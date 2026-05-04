# NetSpec: Declarative Network State Monitor

NetSpec is a next-generation, declarative network monitoring system designed for environments where *state correctness matters more than metrics*.

## Quick Start

### Prerequisites

- Docker and Docker Compose v2 (`docker compose`)
- Cisco IOS-XE devices using **dial-out MDT** (grpc-tcp) into the repo’s Telegraf path, plus SNMP for targeted confirmation

### Configuration

1. Edit `config/desired-state.yaml` with global settings.
2. Define devices either in `config/desired-state.yaml` (monolithic) or as split files in `config/devices/*.yaml`.
3. Configure notifications: edit **`config/alerts.yaml`** (channels reference env var names such as `APPRISE_SLACK_WEBHOOK`). The repository includes a starter file.
4. Copy `.env.example` to `.env` and update with your credentials:

```bash
cp .env.example .env
# Edit .env with your actual values
```

The `.env` file (repo root for Compose, or see **Host / local binary** below) should contain:
- `SNMP_COMMUNITY` - SNMPv2c community (used by SNMP validation and push-confirmation paths)
- `APPRISE_API_URL` - Apprise-API **base URL** (required for notifications). NetSpec POSTs JSON to `{APPRISE_API_URL}/notify/` (stateless Apprise-API `notify` endpoint). With `network_mode: host` for NetSpec, use `http://127.0.0.1:8086` (or your host-mapped port), not `http://apprise:8000` (that hostname only resolves inside Compose).
- Channel targets come from env vars named in `config/alerts.yaml` under `channels.*.url_env` (for example `APPRISE_SLACK_WEBHOOK`). See `.env.example` for placeholders.
- Optional: `APPRISE_NOTIFY_TIMEOUT` (HTTP timeout per notify, e.g. `15s`). Troubleshooting: [Apprise alerting](docs/APPRISE_ALERTING.md).
- `NETSPEC_INGEST_HOST` / `NETSPEC_INGEST_PORT` - where **`mdt-translator`** sends NetSpec JSON lines (must match `global.ingest` when `telemetry_mode` is `telemetry_ingest_push`)
- `MDT_ALLOWED_DEVICES` - optional comma-separated device-name allowlist for the translator sidecar
- `NETSPEC_IMAGE_TAG` - optional container image tag override
- Other optional settings as documented in `.env.example`

**Host / local binary:** When you run `./netspec -config /path/to/config/desired-state.yaml`, NetSpec loads environment defaults from **`/path/to/config/.env`** and **`/path/to/config/netspec.env`** if present (same directory as `desired-state.yaml`). Existing process environment variables are **not** overridden. Docker Compose still reads `.env` from the **repository root** for variable interpolation in compose files.

`config/desired-state.yaml` sets `global.telemetry_mode`:
- **`telemetry_ingest_push`** (default in the sample file): line-delimited JSON push ingest on `global.ingest` (default `0.0.0.0:57500`) with targeted SNMP confirmation per event — pair with **both** compose files so Telegraf + **`mdt-translator`** decode IOS-XE dial-out into that ingest.
- **`snmp_validate_only`**: SNMP validation only; no push ingest listener (no MDT path on this host).

### Running

Compose is split across two YAML files so **`docker-compose.yml`** can stay Apprise + NetSpec only; **Telegraf** and **`mdt-translator`** live in **`docker-compose.dev.yml`** (name is historical — use the same merge everywhere you run push telemetry). That is **not** an “optional add-on”; it is how the supported MDT → NetSpec path is packaged.

The NetSpec image is built by GitHub Actions and published to GitHub Container Registry.

**Note**: To pull from GitHub Container Registry, you may need to authenticate:
```bash
echo $GITHUB_TOKEN | docker login ghcr.io -u USERNAME --password-stdin
```

**Push telemetry (standard):** IOS-XE dial-out (`grpc-tcp`) with `telemetry_ingest_push`:

```bash
docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d
```

This starts **`telegraf-mdt`** (MDT in on `tcp/57500`) and **`mdt-translator`** (image **`netspec-mdt-translator:local`**, built from **`tools/sidecar`**) writing NetSpec-shaped JSON lines to `NETSPEC_INGEST_HOST:NETSPEC_INGEST_PORT`, plus **`netspec`** and **`apprise`**.

The first time (or after translator changes), build the translator on that host (needs repo checkout with `tools/sidecar/`):

```bash
docker compose -f docker-compose.yml -f docker-compose.dev.yml build mdt-translator
```

Runtime artifacts: `${NETSPEC_DATA_DIR}/mdt-sidecar` (`decoded.json`, `forwarder.log`).

**Core stack only** (no Telegraf/translator on this host — e.g. `snmp_validate_only` and no dial-out to this box):

```bash
docker compose up -d
```

To use a specific image tag instead of `latest`, set the `NETSPEC_IMAGE_TAG` environment variable:
```bash
NETSPEC_IMAGE_TAG=v1.0.0 docker compose up -d
```

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
make docker-up-telemetry               # apprise + netspec + telegraf + mdt-translator (host network)
# Apprise + NetSpec only (no MDT containers on this host):
make docker-up
```

After each Go or translator Python change, run **`make docker-rebuild`** then **`make docker-up-telemetry`** (or **`make docker-up`**) or **`docker compose ... up -d --force-recreate`**. **`make docker-up`** / **`docker-up-telemetry`** alone does not rebuild images.

Compose files: `docker-compose.yml` + `docker-compose.build-local.yml`, and **`docker-compose.dev.yml`** whenever you run the push pipeline locally. Stop any host `nohup ./netspec` or old containers first so port **8088** / ingest port are free.

| Make target | What it does |
|---------------|----------------|
| `make docker-rebuild` | Build `netspec:local` and `netspec-mdt-translator:local` |
| `make docker-up` | Start Apprise + NetSpec (local images) |
| `make docker-down` | Stop that stack |
| `make docker-up-telemetry` | Same + Telegraf MDT + `mdt-translator` |
| `make docker-down-telemetry` | Stop telemetry stack |
| `make docker-logs-netspec` | Follow NetSpec container logs |

## MVP Features

This MVP includes:

- ✅ SNMP validator with targeted polling
- ✅ Interface state evaluation (including **port-channel** members, `member_policy` thresholds, and high-speed interface alias normalization for SNMP vs. telemetry name drift)
- ✅ Push telemetry ingest via **Telegraf MDT + `mdt-translator`** (newline-delimited JSON into NetSpec)
- ✅ Alerting via **Apprise-API** (`/notify/`) using channels defined in `config/alerts.yaml`
- ✅ YAML configuration (split devices, optional credentials and maintenance files)
- ✅ Docker deployment and **local parity** Makefile workflow
- ✅ Web status interface, discovery wizard, API browser (OpenAPI/Swagger)

## Web Interface

NetSpec includes a built-in web UI accessible at `http://localhost:8088` (or your configured host/port).

### Features

- **Dashboard** - Overview of devices, interfaces, active alerts, push telemetry **events/sec**, and a **host overview** honeycomb (up to 64 devices, worst alert severity per cell, links to device pages; refreshes periodically)
- **Device List** - All monitored devices with interface counts
- **Active Alerts** - Current firing alerts with severity indicators (sorted by severity)
- **Live Logs** - Auto-refreshing log stream (newest entries first; periodic refresh)
- **Configuration View** - Collection interval and dedup settings
- **Config Reload** - Button to force re-read of YAML from the config directory without restart
- **API Browser** - Interactive OpenAPI documentation at `/api-browser` (Swagger UI with try-it-out; machine-readable spec at `/openapi.json`). Interface names in URLs must be **percent-encoded** (for example `GigabitEthernet1%2F0%2F1`).

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

| File | Purpose |
|------|--------|
| **`desired-state.yaml`** | Required. `global` settings and optional inline `devices` map |
| **`devices/*.yaml`** | Optional. Split per-device definitions (see formats below) |
| **`alerts.yaml`** | Optional but **required for Apprise delivery**. Defines `channels`, `alert_rules`, and `alert_behavior`. A top-level `alerts:` block inside `desired-state.yaml` is **not** read by the loader—use this file (see `config/alerts.yaml` in the repo) |
| **`credentials.yaml`** | Optional. Named credential sets referenced by `credentials_ref` on devices |
| **`maintenance.yaml`** | Optional. Maintenance windows |

At minimum you need **`config/desired-state.yaml`**; add **`config/alerts.yaml`** for real notifications.

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

# MDT → NetSpec ingest translator (same image `docker-compose.dev.yml` builds locally)
docker pull ghcr.io/OWNER/REPO-mdt-translator:latest

# Or use a specific version
docker pull ghcr.io/OWNER/REPO:v1.0.0
```

## Notes

- Use `/wizard` in the web UI to discover and add devices/interfaces. Unknown push-telemetry sources appear under **Telemetry** stats with a link into the wizard (address prefill uses the TCP sender when available).
- Interface policies can be edited inline from each device page (monitor flag, desired/admin state, alert severity).
- Prefer **`docker compose`** (v2) over legacy `docker-compose` where possible.
