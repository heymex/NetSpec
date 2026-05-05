# NetSpec: Declarative Network State Monitor

NetSpec is a next-generation, declarative network monitoring system designed for environments where *state correctness matters more than metrics*.

## Quick Start

### Prerequisites

- Docker and Docker Compose v2 (`docker compose`)
- Cisco IOS-XE devices using **dial-out MDT** (grpc-tcp) into the repo’s Telegraf path, plus SNMP for targeted confirmation

### Configuration

1. Edit `config/desired-state.yaml` with global settings.
2. Define devices either in `config/desired-state.yaml` (monolithic) or as split files in `config/devices/*.yaml`.
3. Optional: edit `config/alerts.yaml` for alert channels and routing (`alerts:` does not belong in `desired-state.yaml`; it is not loaded from there).
4. Copy `.env.example` to `.env` and update with your credentials:

```bash
cp .env.example .env
# Edit .env with your actual values
```

Steps 1–2 are required for a minimal deployment; step 3 only if you use Apprise alerting; step 4 supplies runtime credentials for Compose or local runs.

The `.env` file should contain:
- `SNMP_COMMUNITY` - SNMPv2c community (used by SNMP validation and push-confirmation paths)
- `API_PORT` - web UI/API listen port override (default `8088`)
- `APPRISE_API_URL` - Apprise-API **base URL** (required for notifications). NetSpec POSTs JSON to `{APPRISE_API_URL}/notify/` (stateless Apprise-API `notify` endpoint). With `network_mode: host` for NetSpec, use `http://127.0.0.1:8086` (or your host-mapped port), not `http://netspec-apprise:8000` (that hostname only resolves inside the Compose **`netspec`** bridge, not on the host network namespace).
- Channel targets come from env vars named in `config/alerts.yaml` under `channels.*.url_env` (for example `APPRISE_SLACK_WEBHOOK`). See `.env.example` for placeholders.
- Optional: `APPRISE_NOTIFY_TIMEOUT` (HTTP timeout per notify, e.g. `15s`). Troubleshooting: [Apprise alerting](docs/APPRISE_ALERTING.md).
- `NETSPEC_INGEST_HOST` / `NETSPEC_INGEST_PORT` - where **`mdt-translator`** sends NetSpec JSON lines (must match `global.ingest` when `telemetry_mode` is `telemetry_ingest_push`)
- `MDT_ALLOWED_DEVICES` - optional comma-separated device-name allowlist for the translator sidecar
- `NETSPEC_IMAGE_TAG` - optional container image tag override
- `NETSPEC_*`, `APPRISE_*`, `TELEGRAF_*`, `TRANSLATOR_*` runtime knobs - per-service `*_LOG_MAX_SIZE`, `*_LOG_MAX_FILE`, `*_MEM_LIMIT`, `*_CPU_LIMIT`, `*_PIDS_LIMIT` (see `.env.example`)
- Other optional settings as documented in `.env.example`

**Host / local binary:** When you run `./netspec -config /path/to/config/desired-state.yaml`, NetSpec loads environment defaults from **`/path/to/config/.env`** and **`/path/to/config/netspec.env`** if present (same directory as `desired-state.yaml`). Existing process environment variables are **not** overridden. Docker Compose still reads `.env` from the **repository root** for variable interpolation in compose files.

`config/desired-state.yaml` sets `global.telemetry_mode`:
- **`telemetry_ingest_push`** (default in the sample file): line-delimited JSON push ingest on `global.ingest` (default `0.0.0.0:57500`) with targeted SNMP confirmation per event — Telegraf + **`mdt-translator`** decode IOS-XE dial-out into that ingest.
- **`snmp_validate_only`**: SNMP validation only; no push ingest listener.

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

This starts all four services: **`netspec-netspec`**, **`netspec-apprise`**, **`netspec-telegraf-mdt`** (MDT in on `tcp/57500`), and **`netspec-mdt-translator`** (forwards NetSpec-shaped JSON lines to `NETSPEC_INGEST_HOST:NETSPEC_INGEST_PORT`). Compose uses a **`netspec-` service prefix** and a **`netspec` bridge network** so names stay unique alongside other stacks on the same host.

Runtime artifacts: `${NETSPEC_DATA_DIR}/mdt-sidecar` (`decoded.json`, `forwarder.log`).

All services use Docker log rotation via the `json-file` driver with per-service overrides. Tune `NETSPEC_*`, `APPRISE_*`, `TELEGRAF_*`, and `TRANSLATOR_*` limits in `.env` to avoid multi-GB container logs on low-activity stacks.

To pin a specific image tag instead of `latest`:
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
make docker-up                         # start all four services (local images)
```

After each Go or translator Python change, run **`make docker-rebuild`** then **`make docker-up`** or **`docker compose -f docker-compose.yml -f docker-compose.build-local.yml up -d --force-recreate`**. **`make docker-up`** alone does not rebuild images.

Stop any host `nohup ./netspec` or old containers first so port **8088** / ingest port are free.

| Make target | What it does |
|---------------|----------------|
| `make docker-rebuild` | Build `netspec:local` and `netspec-mdt-translator:local` |
| `make docker-up` | Start all four services (local images) |
| `make docker-down` | Stop the stack |
| `make docker-logs-netspec` | Follow NetSpec container logs |

Because **`netspec-netspec`** uses `network_mode: host` in the default compose stack, `APPRISE_API_URL` must target the host-mapped Apprise port (for example `http://127.0.0.1:8086`), not `http://netspec-apprise:8000` (that DNS name only resolves for containers attached to the **`netspec`** bridge). In this topology, `depends_on` controls startup order only and does not guarantee Apprise is fully ready before NetSpec starts.

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

- **Dashboard** - Overview of devices, interfaces, active alerts, push telemetry **events/sec**, and a **host overview** honeycomb (up to 64 devices; each cell reflects the worse of **active alerts** and **SNMP reachability** so unreachable or not-yet-polled devices are not shown as healthy; refreshes periodically)
- **Device List** - All monitored devices with interface counts
- **Active Alerts** - Current firing alerts with severity indicators (sorted by severity)
- **Live Logs** - Auto-refreshing log stream (newest entries first; periodic refresh)
- **Configuration View** - Collection interval and dedup settings
- **Config Reload** - Button to reload all configuration files from the config directory without restart
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

| File | Required | Purpose |
|------|----------|---------|
| `config/desired-state.yaml` | Yes | Global settings plus optional monolithic device/interface definitions |
| `config/alerts.yaml` | No | Alert channels, routing rules, and alert behavior |
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

## Bundled Tools

The container image bundles `gnmic` for operator debugging workflows (for example ad-hoc in-container gNMI queries while troubleshooting). The NetSpec application itself does not shell out to `gnmic` at runtime.

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
