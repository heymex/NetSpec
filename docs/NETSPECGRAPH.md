# NetSpecGraph — bring-up guide

Companion metrics path for NetSpec. Design authority:
[`upcoming_development/NetSpecGraph/NetSpecGraph-DevSpec.md`](../upcoming_development/NetSpecGraph/NetSpecGraph-DevSpec.md).

NetSpec **state** is unchanged: Telegraf → `decoded.json` → mdt-translator → ingest.
NetSpecGraph **metrics** are additive: Telegraf → VictoriaMetrics → `cmd/netspecgraph`.

## Dedicated lab VM (recommended)

Best way to develop Graph: a **new VM** that is not production. Use normal ports
(`8088`, `57500`, `8090`, `8428`). Keep production receivers intact; **add** a second
MDT receiver on each participating switch aimed at this VM.

### 1. VM prerequisites

- Linux + Docker Engine + Compose v2
- Management reachability from IOS-XE dial-out sources to **TCP 57500** (MDT)
- Optional: open **8088** / **8090** to your workstation; keep **8428** localhost-only (compose default)
- Clone this repo on the `feat/netspecgraph` branch (commit/push from your laptop first if needed)

```bash
git clone https://github.com/heymex/NetSpec.git
cd NetSpec
git checkout feat/netspecgraph
```

### 2. Bootstrap config + stack

```bash
# Prefer creating /opt/netspec with sudo, then run setup as your user when possible.
# If you must sudo the script, it now chowns repo .env back to $SUDO_USER.
sudo mkdir -p /opt/netspec && sudo chown "$USER":"$USER" /opt/netspec
./scripts/setup-netspec.sh --data-dir /opt/netspec
# If .env is still root-owned from an earlier sudo run:
#   sudo chown "$USER":"$USER" .env

# Optional: copy prod identity (devices/rules) into the lab — copy, don't symlink:
#   sudo rsync -a /path/to/prod/config/ /opt/netspec/config/
# Then silence alert channels in /opt/netspec/config/alerts.yaml so the lab never pages.

# Build local images (includes ./netspecgraph) and start full stack + Graph
make graph-dev-up
```

| Service | URL / port |
|---|---|
| NetSpec UI | http://\<lab-vm\>:8088 |
| NetSpecGraph | http://\<lab-vm\>:8090 |
| vmui (on the VM) | http://127.0.0.1:8428/vmui |
| MDT dial-out target | \<lab-vm-ip\>:57500 |

Verify:

```bash
curl -sS http://127.0.0.1:8088/health
curl -sS http://127.0.0.1:8090/health
curl -sS http://127.0.0.1:8428/health
docker compose --profile graph ps
```

Rebuild after Go changes:

```bash
make graph-dev-up
```

### 3. Add MDT receivers (do not remove production)

On each switch that should feed the lab, under the **existing** subscription 251
(and later 211 for DOM), **add** a receiver — leave the production receiver alone:

```
telemetry ietf subscription 251
 receiver ip address <LAB_VM_IP> 57500 protocol grpc-tcp
```

Use **`grpc-tcp`** (plaintext), matching this repo’s Telegraf sidecar — same caveat as
[`docs/CISCO_GNMI_SETUP.md`](CISCO_GNMI_SETUP.md).

Optional narrowing in lab `.env`:

```bash
# Only forward these hostnames through mdt-translator → NetSpec state ingest.
# Telegraf still writes whatever MDT it receives into VictoriaMetrics.
MDT_ALLOWED_DEVICES=csw-lab-01,asw-lab-01
```

### 4. Step 1 done-when

After ~5 minutes of dial-out:

1. Open http://127.0.0.1:8428/vmui on the lab VM.
2. Confirm a known device/interface has a **non-zero** `rate()` / `increase()`.
3. Confirm production NetSpec on the old collectors is unchanged.

Metric names will still be Telegraf/YANG-derived until the rename processor is frozen (step 2).

---

## Parallel lab on the *same* Docker host as production

Only needed if you cannot get a separate VM. Uses alternate ports and container
name prefixes so nothing collides with production.

| | Production (typical) | Same-host Graph lab |
|---|---|---|
| Data dir | `/opt/netspec` | `/opt/netspec-graphlab` |
| Env file | `.env` | `.env.graph-lab` |
| Compose project | `netspec` (dir name) | `netspecgraphlab` |
| Containers | `netspec-*` | `netspecgl-*` |
| UI | `:8088` | `:8188` |
| Graph UI | — | `:8190` |
| vmui | — | `127.0.0.1:18428` |
| MDT dial-out | `:57500` | `:57510` |
| Apprise host | `:8086` | `:8186` |

```bash
./scripts/setup-graph-lab.sh
# ./scripts/setup-graph-lab.sh --clone-config-from /opt/netspec/config
make graph-lab-up
```

MDT second receiver in this mode must use the lab host port:

```
receiver ip address <same-host-ip> 57510 protocol grpc-tcp
```

---

## What the Graph branch adds

1. `netspec-victoriametrics` compose service (`-retentionPeriod=400d`, data under `${NETSPEC_DATA_DIR}/victoria-metrics`).
2. Telegraf `outputs.influxdb` writing to VM (file output for the translator untouched).
3. Opt-in `netspec-graph` service (`docker compose --profile graph`) and a skeleton `./netspecgraph` binary in the NetSpec image.

## Freeze the metric schema (step 2)

Observed live Telegraf shape (IETF `/interfaces-state/interface`):

- measurement: `ietf-interfaces:interfaces-state/interface`
- tags: `source` (device), `name` (interface), plus `path` / `subscription`
- fields: `statistics/in_octets`, …, `speed`, `oper_status` (**string** `up`/`down`/…)

`tools/sidecar/telegraf-mdt.conf` clones those into the NetSpecGraph contract for
VictoriaMetrics only (starlark processor). The original metric is unchanged for
`outputs.file` → mdt-translator.

| Contract metric | Source field |
|---|---|
| `if_in_octets_total` / `if_out_octets_total` | `statistics/in_octets` / `out_octets` |
| `if_in_errors_total` / `if_out_errors_total` | `statistics/in_errors` / `out_errors` |
| `if_in_discards_total` / `if_out_discards_total` | `statistics/in_discards` / `out_discards` |
| `if_in_unicast_pkts_total` / `if_out_unicast_pkts_total` | `statistics/in_unicast_pkts` / `out_unicast_pkts` |
| `if_speed_bps` | `speed` |
| `if_oper_status` | `oper_status` mapped `up→1`, else `0` |

Write-time labels: `device`, `interface` (and later `lane` for optics). Routing tag
`_netspecgraph` is stripped before VM. VictoriaMetrics runs with
`-influxSkipSingleField` so `__name__` equals the measurement.

**Done-when:** vmui resolves `if_in_octets_total` (etc.) with a non-zero `rate()`,
and series labels are only `device` / `interface` (plus optional `db` from Influx).

## Invariants (do not violate)

- Do not modify subscription 251’s *existing* production receiver, `decoded.json`, or `mdt-translator` for metrics — only **add** lab receivers.
- Do not write `role` / `alias` / `neighbor_class` / `monitored` / `desired_state` as VM labels.
- Store raw counters; rates and %-utilization are query-time only.
- NetSpecGraph never pages or alerts.
