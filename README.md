# NetSpec: Declarative Network State Monitor

**Current pre-release: [v2.0.0-beta.1](https://github.com/heymex/NetSpec/releases/tag/v2.0.0-beta.1)** ([CHANGELOG](CHANGELOG.md), [release notes](docs/RELEASE_NOTES.md)) — pin **`NETSPEC_IMAGE_TAG=v2.0.0-beta.1`** for this beta; **v1.0.0** remains the last stable line ([tag](https://github.com/heymex/NetSpec/releases/tag/v1.0.0)). Use **`latest`** only to track `main`.

NetSpec is a declarative network monitor: you define how the network *should* behave, and NetSpec **evaluates reality against that desired state** and **raises alerts** when they diverge (SNMP, telemetry ingest, Apprise-backed delivery). It is built for environments where *state correctness matters more than metrics*.

## Quick Start

### Prerequisites

- Docker and Docker Compose v2 (`docker compose`)
- Cisco IOS-XE devices using **dial-out MDT** (grpc-tcp) into the repo’s Telegraf path, plus SNMP for targeted confirmation

### Run a sample stack (recommended first boot)

From a **repo checkout** (so `./tools/sidecar` exists next to `docker-compose.yml`):

```bash
./scripts/setup-netspec.sh
docker compose pull && docker compose up -d
```

Then open **`http://127.0.0.1:8088`** (sample devices + alerting in the YAML; SNMP/Slack wiring is still yours to fix). **`docker compose up` alone is not sufficient**: the compose file mounts **`${NETSPEC_DATA_DIR}/config`**; that tree must contain **`desired-state.yaml`**, **`alerts.yaml`**, and **split devices under `config/devices/*.yaml`** because the shipped **`desired-state.yaml` uses `devices: {}`**. The setup script installs all of that, aligns **`NETSPEC_INGEST_PORT`** with **`global.ingest.port`** when missing (**57500** in the sample bridge stack), aligns **`.env`**, and fixes **mdt-sidecar** ownership for uid **999**. Upgrading from **v1.x** host-network compose: **[docs/MIGRATION_BRIDGE_AND_AUTH.md](docs/MIGRATION_BRIDGE_AND_AUTH.md)**.

If **`NETSPEC_DATA_DIR` is already set in `.env`** (Komodo/Portainer-style), `./scripts/setup-netspec.sh` uses that path automatically — no **`--data-dir`** required.

### Configuration (beyond the sample)

1. Edit `${NETSPEC_DATA_DIR}/config/desired-state.yaml` with global settings (after setup), or rely on defaults.
2. Define devices either in `desired-state.yaml` (monolithic) or as split files in `${NETSPEC_DATA_DIR}/config/devices/*.yaml`.
3. Edit `${NETSPEC_DATA_DIR}/config/alerts.yaml` for channels and routing (the repo ships a sample; **`./scripts/setup-netspec.sh`** installs it — `alerts:` does not belong in `desired-state.yaml` and is not loaded from there).
4. Ensure `.env` next to compose has your secrets ( **`./scripts/setup-netspec.sh`** creates it from **`.env.example`** when missing):

```bash
cp .env.example .env
# Edit .env with your actual values
```

Steps 1–2 define what “correct” means; step 3 defines **where drift alerts are delivered** (channels and routing — repository sample + **`setup-netspec.sh`**). Step 4 supplies Compose/runtime credentials. An `alerts:` block inside `desired-state.yaml` is **not** read (that file unmarshals to only **`global`** and **`devices`** — routing belongs in **`alerts.yaml`** only).

The `.env` file should contain:
- `SNMP_COMMUNITY` - SNMPv2c community (used by SNMP validation and push-confirmation paths)
- `API_PORT` - web UI/API listen port override (default `8088`, published on the host as **`API_PORT`**)
- `APPRISE_API_URL` - Apprise-API **base URL** NetSpec uses to deliver alerts (`{APPRISE_API_URL}/notify/`). Default compose (**bridge** NetSpec ↔ Apprise): **`http://netspec-apprise:8000`** (Docker DNS). The host publishes Apprise UI on **`http://127.0.0.1:8086`** for convenience.
- Channel targets come from env vars named in `config/alerts.yaml` under `channels.*.url_env` (for example `APPRISE_SLACK_WEBHOOK`). See `.env.example` for placeholders.
- Optional: `APPRISE_NOTIFY_TIMEOUT` (HTTP timeout per notify, e.g. `15s`). Troubleshooting: [Apprise alerting](docs/APPRISE_ALERTING.md).
- `NETSPEC_INGEST_HOST` / `NETSPEC_INGEST_PORT` - where **`mdt-translator`** sends NetSpec JSON lines (must match `global.ingest` when `telemetry_mode` is `telemetry_ingest_push`; default compose **`NETSPEC_INGEST_HOST=netspec-netspec`**)
- `NETSPEC_ADMIN_PASSWORD_HASH` / `NETSPEC_SESSION_SECRET` - optional **browser session** login for the web UI and API HTML routes (see **`.env.example`**; use `netspec hash-password` or CI image entrypoint). Omit both (or leave hash empty) for open access.
- `NETSPEC_API_TOKEN` - optional **bearer token** for scripted API access alongside session cookies
- `MDT_ALLOWED_DEVICES` - optional comma-separated device-name allowlist for the translator sidecar
- `NETSPEC_IMAGE_TAG` - optional container image tag override (**`v2.0.0-beta.1`**, **`v1.0.0`**, or **`latest`**)
- `NETSPEC_*`, `APPRISE_*`, `TELEGRAF_*`, `TRANSLATOR_*` runtime knobs - per-service `*_LOG_MAX_SIZE`, `*_LOG_MAX_FILE`, `*_MEM_LIMIT`, `*_CPU_LIMIT`, `*_PIDS_LIMIT` (see `.env.example`)
- Other optional settings as documented in `.env.example`

**Host / local binary:** When you run `./netspec -config /path/to/config/desired-state.yaml`, NetSpec loads environment defaults from **`/path/to/config/.env`** and **`/path/to/config/netspec.env`** if present (same directory as `desired-state.yaml`). Existing process environment variables are **not** overridden. Docker Compose still reads `.env` from the **project directory** (next to `docker-compose.yml`) for `${VAR}` interpolation. The **`netspec-netspec`** service also declares **`env_file: .env`** (optional if the file is missing) so secrets such as **`APPRISE_SLACK_WEBHOOK`** are passed into the container—not only variables listed under `environment:`.

`${NETSPEC_DATA_DIR}/config/desired-state.yaml` sets `global.telemetry_mode`:
- **`telemetry_ingest_push`** (default in the sample file): line-delimited JSON push ingest on **`global.ingest`** (**`NETSPEC_INGEST_PORT`** must match **`global.ingest.port`** — sample **57500** on bridge: Telegraf and NetSpec listen in **different containers**) with targeted SNMP confirmation per event — Telegraf + **`mdt-translator`** decode IOS-XE dial-out into that ingest. **`additional_listeners`** optional for per-port “sourcetype” tagging (same JSON format).
- **`snmp_validate_only`**: SNMP validation only; no push ingest listener.

In `telemetry_ingest_push` mode you can optionally enable `global.snmp.telemetry_fallback_enabled` to run periodic full-device SNMP polling as a safety net when telemetry is missing. This fallback can significantly increase SNMP/device load and slow large deployments; use conservative intervals (for example `5m` or longer).

### First-time setup script

From the repo root, run **`./scripts/setup-netspec.sh`** (no flags needed; use **`sudo`** if the data directory requires root).

1. **`NETSPEC_DATA_DIR`**: taken from **`.env`** if present, otherwise **`/opt/netspec`** or **`~/netspec-data`**.
2. Creates **`NETSPEC_DATA_DIR`** (`config/`, `config/devices/`, `data/`, `mdt-sidecar/`, `apprise-config/`).
3. Seeds **`config/desired-state.yaml`** and **`config/alerts.yaml`**, and **always** installs sample split devices (**`config/devices/*.yaml`**) so NetSpec starts with **`devices: {}`** in **`desired-state`**. Skips replacing existing top-level YAML unless **`--force`**.
4. Creates or updates **`.env`**: copies from **`.env.example`** when missing; syncs **`NETSPEC_DATA_DIR`**, **`SNMP_COMMUNITY`**, appends **`NETSPEC_INGEST_PORT`** when unset (align with sample **`global.ingest.port`**), and attempts **mdt-sidecar** **`chown`** for Telegraf (tries passwordless **`sudo`** if needed).

Optional: **`--interactive`** to prompt for paths; **`--data-dir`** only when you want to override **`.env`**.

Then edit real devices and notification destinations, **`docker compose pull`** (if using GHCR), **`docker compose up -d`**, and reload after YAML edits (`POST /api/reload` or the dashboard).

### Running

GitHub Actions builds and publishes all images (NetSpec and mdt-translator) to GitHub Container Registry on every merge to main.

**Note**: To pull from GitHub Container Registry, you may need to authenticate:
```bash
echo $GITHUB_TOKEN | docker login ghcr.io -u USERNAME --password-stdin
```

**Standard deployment** (published images):

```bash
docker compose up -d
```

This starts **NetSpec** (web/API on host **`API_PORT`**, default **8088**), **Apprise-API** (**`8086:8000`** on the host), **Telegraf MDT** (host **`57500/tcp`** published into the container for MDT dial-out), and **mdt-translator**. All services attach to the Compose **`netspec` bridge** (`docker-compose.yml` uses a **`netspec-` name prefix**).

Runtime artifacts: `${NETSPEC_DATA_DIR}/mdt-sidecar` (`decoded.json`, `forwarder.log`).

All services use Docker log rotation via the `json-file` driver with per-service overrides. Tune `NETSPEC_*`, `APPRISE_*`, `TELEGRAF_*`, and `TRANSLATOR_*` limits in `.env` to avoid multi-GB container logs on low-activity stacks.

To pin a specific image tag instead of `latest`:
```bash
NETSPEC_IMAGE_TAG=v2.0.0-beta.1 docker compose up -d
```

### Komodo, Portainer, and similar UIs

If you manage stacks with **Komodo**, **Portainer**, **Dockge**, or another Compose-based UI instead of typing `docker compose` by hand, the same **`docker-compose.yml`** applies—plus a few constraints these tools hide behind a form.

#### 1. Project directory must include `tools/sidecar/`

**Telegraf** bind-mounts a path **relative to the compose file**:

```yaml
./tools/sidecar/telegraf-mdt.conf:/etc/telegraf/telegraf.conf:ro
```

So the directory the UI treats as the **Compose project root** must be a **NetSpec repo checkout** (or a copy) that still contains **`tools/sidecar/`** next to **`docker-compose.yml`**. A compose file alone in an empty folder **will not** start Telegraf correctly.

#### 2. `NETSPEC_DATA_DIR` and YAML config (outside the repo)

Runtime config lives under **`${NETSPEC_DATA_DIR}`** on the host (default **`/opt/netspec`**): **`config/`**, **`data/`**, **`mdt-sidecar/`**, **`apprise-config/`**. Compose mounts **`config/` read-write** into the NetSpec container so dashboard/API edits (including deleting **`config/devices/*.yaml`**) persist. Operator edits with **`nano`/`vim`** remain valid—after YAML changes outside the UI, **reload** NetSpec (`POST /api/reload` or the dashboard button).

#### 3. Environment: `.env` next to the compose file

Compose loads **`.env`** in the project directory for **`${VAR}`** interpolation, and **`netspec-netspec`** uses **`env_file: .env`** to pass **`APPRISE_SLACK_WEBHOOK`** and other **`url_env`** secrets into the container.

Set **`NETSPEC_DATA_DIR`** explicitly in `.env` (for example `/opt/netspec`) so runtime mounts never silently fall back to defaults. For push ingest, keep **`global.ingest.port`** in `${NETSPEC_DATA_DIR}/config/desired-state.yaml` aligned with **`NETSPEC_INGEST_PORT`** in `.env` (sample and bridge stack commonly use **57500** for both Telegraf publish and NetSpec listen in separate containers).

- **Komodo / file-based stacks:** Keep **`compose.yaml`** (or **`docker-compose.yml`**) and **`.env`** in the same stack folder (this matches Docker Compose’s usual layout). Komodo labels expose **`com.docker.compose.project.environment_file`** for the `.env` path—ensure it points at the real file after deploy.
- **Portainer (stack from Git):** Set the **compose path** (e.g. **`docker-compose.yml`**), branch, and **environment variables** in the UI for secrets you do not commit (GHCR pull, **`SNMP_COMMUNITY`**, **`NETSPEC_DATA_DIR`**, Apprise URLs). You can paste the contents of **`.env.example`** and fill in values.
- **Portainer (web editor):** Upload or paste compose **from the repo**, set **Working directory** / bind-mount base if the UI supports it so **`./tools/sidecar`** resolves, and add env vars in the stack’s **Environment** section.

Preflight before each `docker compose up -d` or UI restart:

```bash
./scripts/validate-netspec-stack.sh --project-dir /etc/komodo/stacks/NetSpec
```

The validator fails fast on common drift: missing `NETSPEC_DATA_DIR`, ingest port mismatch (`desired-state.yaml` vs `.env`), or missing split-device YAML files.

#### 4. Image registry (GHCR)

Published images are **`ghcr.io/heymex/netspec`** and **`ghcr.io/heymex/netspec-mdt-translator`**. The Docker host (or registry settings in the UI) must be able to **`docker pull`**—log in with a GitHub token where required (see **Running** above).

#### 5. Published ports (bridge stack)

**NetSpec**: **`${API_PORT:-8088}:${API_PORT:-8088}`** to the host. **Apprise**: **`8086:8000`**. **Telegraf**: **`57500:57500`** (MDT dial-out target on the host maps to the Telegraf container). **Translator** has no public port; it connects to NetSpec on the bridge. Ensure the host firewall permits MDT devices to reach **57500/tcp** where required.

#### 6. Use **Docker Compose** stacks, not raw Swarm-only manifests

This repository’s file is aimed at **`docker compose`** (Compose spec v2/v3). In Portainer, prefer **Stacks → Add stack → Web editor / Git** using the **Compose** format rather than translating to Swarm Services by hand.

### Building from Source

```bash
go mod download
go build -o netspec ./cmd/netspec
./netspec -config ./config/desired-state.yaml
```

### Local Docker build (same images as prod, faster iteration than CI)

Use this when you want the **same container layout as production** but built **on your machine** from the current tree:

```bash
export NETSPEC_DATA_DIR=/opt/netspec   # or your config/data root
make docker-rebuild                    # build netspec:local + netspec-mdt-translator:local
make docker-up                         # start all four services (local images)
```

After each Go or translator Python change, run **`make docker-rebuild`** then **`make docker-up`** or **`docker compose -f docker-compose.yml -f docker-compose.build-local.yml up -d --force-recreate`**. **`make docker-up`** alone does not rebuild images.

Stop any host `nohup ./netspec` or old containers first so port **8088** / ingest port are free.

| Make target | What it does |
|---------------|----------------|
| `make docker-rebuild` | Build `netspec:local` and `netspec-mdt-translator:local` |
| `make docker-up` | Start full stack (local images) |
| `make docker-down` | Stop the stack |
| `make docker-logs-netspec` | Follow NetSpec container logs |

With the default **bridge** stack, set **`APPRISE_API_URL=http://netspec-apprise:8000`** so NetSpec reaches Apprise over Docker DNS. (**`127.0.0.1:8086`** on the host still works for *your browser* visiting Apprise’s UI.) For a **remote** Apprise-only deployment, point **`APPRISE_API_URL`** at that URL and drop or repoint the bundled **`netspec-apprise`** service.

## Features

As of **v2.0.0-beta.1**, highlights include:

- ✅ SNMP validator with targeted polling
- ✅ Interface state evaluation (including **port-channel** members, `member_policy` thresholds, and high-speed interface alias normalization for SNMP vs. telemetry name drift)
- ✅ Push telemetry ingest via **Telegraf MDT + `mdt-translator`** (newline-delimited JSON into NetSpec)
- ✅ **Alerts on desired-state mismatch**, delivered via **Apprise-API** (`/notify/`) and channels in `config/alerts.yaml`
- ✅ YAML configuration (split devices, optional credentials and maintenance files)
- ✅ Docker deployment and **local parity** Makefile workflow
- ✅ Web status interface, discovery wizard (including **re-walk / sync** monitored interfaces for existing devices), API browser (OpenAPI/Swagger)
- ✅ Optional **session + API token** authentication (`internal/auth`)
- ✅ **Multi-port** push ingest with **`additional_listeners`** / per-port **source** tags
- ✅ Telemetry **coverage diagnostics**, stack **preflight** script, bridge-first **Compose** networking

## Web Interface

NetSpec includes a built-in web UI accessible at `http://localhost:8088` (or your configured host/port).

### Features

- **Dashboard** - Overview of devices, interfaces, active alerts, push telemetry **events/sec**, and a **host overview** honeycomb (up to 64 devices; each cell reflects the worse of **active alerts** and **SNMP reachability** so unreachable or not-yet-polled devices are not shown as healthy; refreshes periodically)
- **Device List** - All monitored devices with interface counts
- **Active Alerts** - Current firing alerts with severity indicators (sorted by severity)
- **Live Logs** - Auto-refreshing log stream (newest entries first; periodic refresh)
- **Configuration View** - Collection interval and dedup settings
- **Config Reload** - Button to reload all configuration files from the config directory without restart
- **Test alerts** - Dashboard button that POSTs to Apprise for every channel in `alerts.yaml` (synthetic **warning**, same URLs and **severity_filter** behavior as production alerts); per-channel results appear in a toast
- **API Browser** - Interactive OpenAPI documentation at `/api-browser` (Swagger UI with try-it-out; machine-readable spec at `/openapi.json`). Interface names in URLs must be **percent-encoded** (for example `GigabitEthernet1%2F0%2F1`).
- **SNMP notices** - When SNMP matters for your deployment (fallback polling, snmp-only mode, or telemetry + SNMP reachability), the dashboard, device pages, wizard, and `/status` surface short **banner warnings** so operators see load/behavior expectations beyond log lines alone.

### API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | Web UI dashboard |
| `/api-browser` | GET | Interactive API browser (Swagger UI) |
| `/openapi.json` | GET | OpenAPI 3.0 document for tooling and the API browser |
| `/health` | GET | Health check |
| `/status` | GET | Status summary (JSON) |
| `/alerts` | GET | Active alerts (JSON) |
| `/api/logs` | GET | Recent log entries (JSON) |
| `/api/devices` | GET | Device configuration (JSON) |
| `/api/devices/{name}` | GET | Single device detail (JSON) |
| `/api/devices/{name}` | DELETE | Remove device from desired-state YAML (and split device file if present), reload when configured, clear active alerts for that device |
| `/api/devices/{name}/interfaces/{iface}` | PATCH | Update interface policy fields (`monitor`, `desired_state`, `admin_state`, `description`, `alert_severity`, etc.) |
| `/api/reload` | POST | Reload configuration |
| `/api/notifications/test` | POST | Optional JSON body `{"channels":["name",...]}`; send synthetic Apprise **warning** to those channels or to **all** when omitted (`all_ok`, per-channel `outcomes` in response; **502**/**503** on prerequisites errors) |
| `/api/telemetry/stats` | GET | Push ingest counters, **events/sec**, last event time, top talkers, and unknown devices (with wizard URLs; telemetry source IP can prefill the wizard when the device is not in config yet) |
| `/api/discovery/probe` | POST | SNMP probe (wizard) |
| `/api/discovery/walk` | POST | SNMP interface walk |
| `/api/discovery/commit` | POST | Write discovery selection to YAML (`sync_discovered_interfaces` on patch = full SNMP walk snapshot: unchecked interfaces are removed from desired state for names present on the walk) |
| `/device/{name}` | GET | Device detail HTML page |
| `/wizard` | GET | Discovery wizard HTML page |

## Architecture

`SNMP Validation / Push Ingest → State Evaluator → Alert Engine → Apprise`

### Current Telemetry Modes

NetSpec currently supports two runtime collection modes:

- `snmp_validate_only` - NetSpec polls targeted SNMP interface OIDs (`ifAdminStatus`/`ifOperStatus`) for configured interfaces and evaluates those snapshots.
- `telemetry_ingest_push` - NetSpec listens on `global.ingest.listen_address:global.ingest.port` (default `0.0.0.0:57500`) for newline-delimited JSON events. Each event can be SNMP-confirmed before entering the evaluator.

### Push-First Direction (Recommended for IOS-XE 17.12.x)

Preferred operating model:

```
IOS-XE Dial-Out Telemetry → Collector (e.g. Telegraf MDT) → mdt-translator → NetSpec ingest → SNMP targeted validation → Evaluator → Alert engine
```

This keeps telemetry event-driven while using targeted SNMP `GET` calls for confirmation. Dial-out telemetry configuration details are documented in `docs/CISCO_GNMI_SETUP.md`.

For `telemetry_ingest_push`, each TCP line must be valid JSON:

```json
{"device":"csw-mcd-01","interface":"GigabitEthernet1/0/1","oper_status":"down","admin_status":"up"}
```

Optional: set `global.ingest.token_env` if you want payload-level shared-token validation.

When using the sidecar overlay, set `NETSPEC_INGEST_PORT` in `.env` to match your
`global.ingest.port` value in `desired-state.yaml`.

## Configuration

NetSpec loads all files from the **`config/`** directory next to `desired-state.yaml`:

| File | Required | Purpose |
|------|----------|---------|
| `config/desired-state.yaml` | Yes | Global settings plus optional monolithic device/interface definitions |
| `config/alerts.yaml` | No (loader skips if missing) | **Default:** use the sample (routing + destinations). Without it, drift is still evaluated but **not delivered** anywhere. |
| `config/credentials.yaml` | No | Named credential sets for device authentication references |
| `config/maintenance.yaml` | No | Scheduled maintenance windows (currently loaded but not yet enforced for alert suppression) |
| `config/devices/*.yaml` | No | Split device YAML loaded from **`config/devices/`**; dashboard/API edits and deletes persist here (default compose mount is RW) |
| `data/devices/*.yaml` | No | Split files created by **Add device** in the wizard (always written under **`data/devices/`**) |

`desired-state.yaml` does not load an `alerts:` block; alert routing lives in `alerts.yaml`.

Split device definitions are merged from **`config/devices/`** first, then **`data/devices/`** (relative to the same config root as `desired-state.yaml`). Duplicate device keys across those trees are rejected at load time.

When using `config/devices/*.yaml`, each file can be either:

```yaml
devices:
  core-sw-01:
    address: 10.0.0.1
    interfaces:
      GigabitEthernet1/0/1:
        desired_state: up
```

or a direct map:

```yaml
core-sw-01:
  address: 10.0.0.1
  interfaces:
    GigabitEthernet1/0/1:
      desired_state: up
```

Device keys must be unique across all files and `desired-state.yaml`.
On startup, NetSpec logs `monolithic_device_count` and `split_device_count` to show how devices were sourced.

See `config/desired-state.yaml`, `config/alerts.yaml`, and `config/devices/example-device.yaml` for configuration examples.

### Port-channel interfaces

For `Port-channel` (or equivalent) interfaces you can declare `members.required` and a `member_policy` with `mode`: `all_active`, `min_active`, or `per_stack_minimum`, plus optional `critical_threshold_pct` / `warning_threshold_pct` for member-down severity escalation. Invalid combinations (for example warning threshold ≥ critical) are rejected at config load time.

### Cisco IOS-XE Telemetry Setup

For detailed instructions on IOS-XE telemetry and validation patterns, see the [Cisco telemetry setup guide](docs/CISCO_GNMI_SETUP.md).

## Operations runbook

For a full dev-host workflow (ports, Apprise URL, sidecar, `curl` checks), see [docs/DEV_HOST_RUNBOOK.md](docs/DEV_HOST_RUNBOOK.md).

## CI/CD

GitHub Actions automatically:
- Builds and tests on every push and pull request
- Builds and pushes multi-arch Docker images (linux/amd64, linux/arm64) to GitHub Container Registry for **NetSpec** and the **MDT translator** sidecar
- Images are tagged with: `latest`, branch name, commit SHA, and semantic version tags (PR builds use corrected metadata tagging)

### Using the Container Image

Images are published to GitHub Container Registry. Replace `OWNER/REPO` with your repository:

```bash
# Pull the latest NetSpec image
docker pull ghcr.io/OWNER/REPO:latest

# MDT → NetSpec ingest translator
docker pull ghcr.io/OWNER/REPO-mdt-translator:latest

# Or pin a semver tag (stable v1.0.0 or pre-release beta)
docker pull ghcr.io/OWNER/REPO:v2.0.0-beta.1
docker pull ghcr.io/OWNER/REPO-mdt-translator:v2.0.0-beta.1
```

## Notes

- Use `/wizard` in the web UI to discover and add devices/interfaces. Unknown push-telemetry sources appear under **Telemetry** stats with a link into the wizard (address prefill uses the TCP sender when available). Existing devices can use **Re-walk interfaces** on the device page (or `/wizard?device_key=...&address=...`): after probe + walk, the wizard prefills monitors from YAML and patch commits sync the walk—unchecked names drop out of desired state (interfaces never seen on the walk are left unchanged).
- Interface policies can be edited inline from each device page (monitor flag, desired/admin state, alert severity).
- Prefer **`docker compose`** (v2) over legacy `docker-compose` where possible.
