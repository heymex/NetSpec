# Distributed MDT ingest sidecars

Run **`netspec-mdt`** at remote sites so IOS-XE dial-out stays on the LAN and only filtered NDJSON crosses the WAN to a central NetSpec. Measured on a full fleet, dial-out can be ~30× larger than the state-only NDJSON the sidecar forwards.

## Topology

```
Remote site                          WAN / VPN                         Central
───────────                          ─────────                         ───────
IOS-XE ──grpc-tcp──► netspec-mdt ──────────NDJSON + token──────────► NetSpec ingest
                     (:57500 LAN)                                    (:57500 published)
                     (:8089 metrics)                                 SNMP confirm → evaluate
```

- **Remote:** `docker-compose.mdt-remote.yml` (sidecar only).
- **Central:** normal `docker-compose.yml` plus `docker-compose.ingest-publish.yml` so remotes can reach ingest.
- **Trust (MVP):** private VPN/overlay path + shared ingest token. NDJSON is plaintext; do not expose ingest to the public Internet.
- **SNMP:** confirmation still runs from central; devices must be SNMP-reachable from the NetSpec host.
- **WAN outage:** failed forwards are logged and dropped (no spool yet).

## Central setup

1. Keep `global.telemetry_mode: telemetry_ingest_push` and `global.ingest.port` (typically `57500`).
2. Enable a shared token in desired-state:

   ```yaml
   global:
     ingest:
       listen_address: 0.0.0.0
       port: 57500
       token_env: NETSPEC_INGEST_TOKEN
   ```

3. Put the same secret in central `.env` as `NETSPEC_INGEST_TOKEN=...` (loaded via `env_file` on `netspec-netspec`).
4. Publish ingest on a VPN-reachable address:

   ```bash
   docker compose -f docker-compose.yml -f docker-compose.ingest-publish.yml up -d
   ```

5. Firewall host `:57500` (or `NETSPEC_INGEST_PUBLISH_PORT`) to VPN peers only.
6. Leave primary ingest `source` empty (or unset) so remote `NETSPEC_MDT_SOURCE` tags are preserved. A non-empty listener `source` overwrites the event field.

Optional: use `additional_listeners` on distinct ports if you prefer port-based sourcetype tags instead of (or in addition to) sidecar source tags.

## Remote setup

1. Copy `.env.example` → `.env` on the remote host and set:

   | Variable | Purpose |
   |----------|---------|
   | `NETSPEC_INGEST_HOST` or `NETSPEC_INGEST_TARGETS` | Central ingest VPN hostname/IP (`host:port` CSV for TARGETS) |
   | `NETSPEC_INGEST_PORT` | Default `57500` when using HOST |
   | `NETSPEC_INGEST_TOKEN` | Same secret as central |
   | `NETSPEC_INGEST_TOKEN_ENV` | Default `NETSPEC_INGEST_TOKEN` in the remote compose file |
   | `NETSPEC_MDT_SOURCE` | Per-site label, e.g. `site-east` |
   | `MDT_ALLOWED_DEVICES` | Optional site allowlist (recommended on thin links) |
   | `MDT_RESEND_INTERVAL` | Unchanged-state resend (default `60s`; raise to cut WAN further) |

2. Start the sidecar only:

   ```bash
   docker compose -f docker-compose.mdt-remote.yml up -d
   ```

3. Point site switches’ dial-out receivers at the **remote host :57500** (LAN), not at central.
4. Keep `MDT_VM_URL` empty at the edge (Graph/VM stays central if used).

## Verification

On the remote sidecar:

```bash
curl -sS http://127.0.0.1:8089/stats
curl -sS http://127.0.0.1:8089/health
```

Expect `receiver.packets_received` climbing and `forward.forward_ok` tracking transformer emissions. On central, `/api/telemetry/stats` should show devices from the site; unknown devices still appear with wizard links.

Device matching uses configured **device names / `node_id`**. Do not rely on TCP `RemoteAddr` (that is the sidecar’s WAN/NAT address).

## Operational notes

- **Colocated stack:** default `docker-compose.yml` still runs a local sidecar on the same host; do not also bind host `:57500` for dial-out on central if remotes own that port for switches—use ingest publish only on central when remotes handle dial-out.
- **Bandwidth:** compare sidecar receive vs forward rates (and optional bandwidth metrics when available) before deciding whether a site needs a local collector.
- **Not in MVP:** TLS/mTLS on NDJSON, durable spool, per-site tokens, first-class `site_id` in the UI.

## Related

- [Cisco IOS-XE telemetry setup](CISCO_GNMI_SETUP.md)
- [Bridge / auth migration](MIGRATION_BRIDGE_AND_AUTH.md)
- Compose files: `docker-compose.mdt-remote.yml`, `docker-compose.ingest-publish.yml`
