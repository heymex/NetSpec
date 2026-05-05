# NetSpec: Declarative Network State Monitor

**Current release: [v1.0.0](https://github.com/heymex/NetSpec/releases/tag/v1.0.0)** ([CHANGELOG](CHANGELOG.md)) — pin **`NETSPEC_IMAGE_TAG=v1.0.0`** in `.env` for reproducible deploys, or use **`latest`** to track `main`.

NetSpec is a declarative network monitor: you define how the network *should* behave, and NetSpec **evaluates reality against that desired state** and **raises alerts** when they diverge (SNMP, telemetry ingest, Apprise-backed delivery). It is built for environments where *state correctness matters more than metrics*.

## Quick Start

### Prerequisites

- Docker and Docker Compose v2 (`docker compose`)
- Cisco IOS-XE devices using **dial-out MDT** (grpc-tcp) into the repo’s Telegraf path, plus SNMP for targeted confirmation

### Configuration

1. Edit `config/desired-state.yaml` with global settings.
2. Define devices either in `config/desired-state.yaml` (monolithic) or as split files in `config/devices/*.yaml`.
3. Edit `config/alerts.yaml` for channels and routing (the repo ships a sample; **`./scripts/setup-netspec.sh`** installs it — `alerts:` does not belong in `desired-state.yaml` and is not loaded from there).
4. Copy `.env.example` to `.env` and update with your credentials:

```bash
cp .env.example .env
# Edit .env with your actual values
```

Steps 1–2 define what “correct” means; step 3 defines **where drift alerts are delivered** (channels and routing — repository sample + **`setup-netspec.sh`**). Step 4 supplies Compose/runtime credentials. An `alerts:` block inside `desired-state.yaml` is **not** read (that file unmarshals to only **`global`** and **`devices`** — routing belongs in **`alerts.yaml`** only).

The `.env` file should contain:
- `SNMP_COMMUNITY` - SNMPv2c community (used by SNMP validation and push-confirmation paths)
- `API_PORT` - web UI/API listen port override (default `8088`)
- `APPRISE_API_URL` - Apprise-API **base URL** NetSpec uses to deliver alerts (`{APPRISE_API_URL}/notify/`). With the default compose stack and host-network NetSpec, use **`http://127.0.0.1:8086`**, not `http://netspec-apprise:8000` (that hostname only resolves on the Compose **`netspec`** bridge).
- Channel targets come from env vars named in `config/alerts.yaml` under `channels.*.url_env` (for example `APPRISE_SLACK_WEBHOOK`). See `.env.example` for placeholders.
- Optional: `APPRISE_NOTIFY_TIMEOUT` (HTTP timeout per notify, e.g. `15s`). Troubleshooting: [Apprise alerting](docs/APPRISE_ALERTING.md).
- `NETSPEC_INGEST_HOST` / `NETSPEC_INGEST_PORT` - where **`mdt-translator`** sends NetSpec JSON lines (must match `global.ingest` when `telemetry_mode` is `telemetry_ingest_push`)
- `MDT_ALLOWED_DEVICES` - optional comma-separated device-name allowlist for the translator sidecar
- `NETSPEC_IMAGE_TAG` - optional container image tag override
- `NETSPEC_*`, `APPRISE_*`, `TELEGRAF_*`, `TRANSLATOR_*` runtime knobs - per-service `*_LOG_MAX_SIZE`, `*_LOG_MAX_FILE`, `*_MEM_LIMIT`, `*_CPU_LIMIT`, `*_PIDS_LIMIT` (see `.env.example`)
- Other optional settings as documented in `.env.example`

**Host / local binary:** When you run `./netspec -config /path/to/config/desired-state.yaml`, NetSpec loads environment defaults from **`/path/to/config/.env`** and **`/path/to/config/netspec.env`** if present (same directory as `desired-state.yaml`). Existing process environment variables are **not** overridden. Docker Compose still reads `.env` from the **project directory** (next to `docker-compose.yml`) for `${VAR}` interpolation. The **`netspec-netspec`** service also declares **`env_file: .env`** (optional if the file is missing) so secrets such as **`APPRISE_SLACK_WEBHOOK`** are passed into the container—not only variables listed under `environment:`.

`config/desired-state.yaml` sets `global.telemetry_mode`:
- **`telemetry_ingest_push`** (default in the sample file): line-delimited JSON push ingest on `global.ingest` (default `0.0.0.0:57500`) with targeted SNMP confirmation per event — Telegraf + **`mdt-translator`** decode IOS-XE dial-out into that ingest.
- **`snmp_validate_only`**: SNMP validation only; no push ingest listener.

In `telemetry_ingest_push` mode you can optionally enable `global.snmp.telemetry_fallback_enabled` to run periodic full-device SNMP polling as a safety net when telemetry is missing. This fallback can significantly increase SNMP/device load and slow large deployments; use conservative intervals (for example `5m` or longer).

### First-time setup script

From the repo root, run **`./scripts/setup-netspec.sh`** (use `sudo` if `/opt/netspec` should own persistent data). It:

1. Creates **`NETSPEC_DATA_DIR`** (`config/`, `data/`, `mdt-sidecar/`, `apprise-config/`).
2. Seeds **`config/desired-state.yaml`** and **`config/alerts.yaml`** from the repository samples (drift evaluation + alert routing). Skips existing files unless **`--force`**.
3. Creates or updates **`.env`**: copies from **`.env.example`** when missing; always sets **`NETSPEC_DATA_DIR`** and **`SNMP_COMMUNITY`**.

Non-interactive example:

`sudo ./scripts/setup-netspec.sh --data-dir /opt/netspec --non-interactive`

Then edit devices and notification destinations, **`docker compose pull`** (if using GHCR), **`docker compose up -d`**, and reload after YAML edits (`POST /api/reload` or the dashboard).

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

This starts **NetSpec**, **Apprise-API** (notification relay on host **8086**), **Telegraf MDT** (listen **57500/tcp** for dial-out MDT), and **mdt-translator**. Compose uses a **`netspec-` service prefix** and a **`netspec` bridge network** for Apprise; NetSpec and the sidecars use host networking where configured.

Runtime artifacts: `${NETSPEC_DATA_DIR}/mdt-sidecar` (`decoded.json`, `forwarder.log`).

All services use Docker log rotation via the `json-file` driver with per-service overrides. Tune `NETSPEC_*`, `APPRISE_*`, `TELEGRAF_*`, and `TRANSLATOR_*` limits in `.env` to avoid multi-GB container logs on low-activity stacks.

To pin a specific image tag instead of `latest`:
```bash
NETSPEC_IMAGE_TAG=v1.0.0 docker compose up -d
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

Runtime config lives under **`${NETSPEC_DATA_DIR}`** on the host (default **`/opt/netspec`**): **`config/`** (read-only for NetSpec), **`data/`**, **`mdt-sidecar/`**, **`apprise-config/`**. You can create that tree with **`./scripts/setup-netspec.sh`** on the server once, or by hand. The UI does not replace editing **`desired-state.yaml`** / **`alerts.yaml`** on disk—after changes, **reload** NetSpec (`POST /api/reload` or the dashboard button).

#### 3. Environment: `.env` next to the compose file

Compose loads **`.env`** in the project directory for **`${VAR}`** interpolation, and **`netspec-netspec`** uses **`env_file: .env`** to pass **`APPRISE_SLACK_WEBHOOK`** and other **`url_env`** secrets into the container.

- **Komodo / file-based stacks:** Keep **`compose.yaml`** (or **`docker-compose.yml`**) and **`.env`** in the same stack folder (this matches Docker Compose’s usual layout). Komodo labels expose **`com.docker.compose.project.environment_file`** for the `.env` path—ensure it points at the real file after deploy.
- **Portainer (stack from Git):** Set the **compose path** (e.g. **`docker-compose.yml`**), branch, and **environment variables** in the UI for secrets you do not commit (GHCR pull, **`SNMP_COMMUNITY`**, **`NETSPEC_DATA_DIR`**, Apprise URLs). You can paste the contents of **`.env.example`** and fill in values.
- **Portainer (web editor):** Upload or paste compose **from the repo**, set **Working directory** / bind-mount base if the UI supports it so **`./tools/sidecar`** resolves, and add env vars in the stack’s **Environment** section.

#### 4. Image registry (GHCR)

Published images are **`ghcr.io/heymex/netspec`** and **`ghcr.io/heymex/netspec-mdt-translator`**. The Docker host (or registry settings in the UI) must be able to **`docker pull`**—log in with a GitHub token where required (see **Running** above).

#### 5. Host networking and ports

NetSpec and the sidecars use **`network_mode: host`** where noted. **Apprise** publishes **`8086:8000`**. Expect host listeners on **`8088`** (web/API), **`8086`** (Apprise), and **`57500/tcp`** (ingest) when using the default sample. Ensure the orchestrator allows **host** network mode on Linux (standard for this compose file).

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

Because **`netspec-netspec`** uses `network_mode: host`, **`APPRISE_API_URL`** must target the **host** Apprise port (for example `http://127.0.0.1:8086`), not `http://netspec-apprise:8000` (that DNS name only resolves on the Compose **`netspec`** bridge). For a **remote** Apprise-API only, change **`APPRISE_API_URL`** accordingly and adjust or remove the local **`netspec-apprise`** service for your environment.

## Features

Version **1.0** includes:

- ✅ SNMP validator with targeted polling
- ✅ Interface state evaluation (including **port-channel** members, `member_policy` thresholds, and high-speed interface alias normalization for SNMP vs. telemetry name drift)
- ✅ Push telemetry ingest via **Telegraf MDT + `mdt-translator`** (newline-delimited JSON into NetSpec)
- ✅ **Alerts on desired-state mismatch**, delivered via **Apprise-API** (`/notify/`) and channels in `config/alerts.yaml`
- ✅ YAML configuration (split devices, optional credentials and maintenance files)
- ✅ Docker deployment and **local parity** Makefile workflow
- ✅ Web status interface, discovery wizard, API browser (OpenAPI/Swagger)

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
| `/api/discovery/commit` | POST | Write discovery selection to YAML |
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
| `config/devices/*.yaml` | No | Legacy location for per-device split YAML (still loaded) |
| `data/devices/*.yaml` | No | **Writable** split device files (discovery wizard and API write here when `/config` is mounted read-only) |

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

# Or use a specific version
docker pull ghcr.io/OWNER/REPO:v1.0.0
```

## Notes

- Use `/wizard` in the web UI to discover and add devices/interfaces. Unknown push-telemetry sources appear under **Telemetry** stats with a link into the wizard (address prefill uses the TCP sender when available).
- Interface policies can be edited inline from each device page (monitor flag, desired/admin state, alert severity).
- Prefer **`docker compose`** (v2) over legacy `docker-compose` where possible.
