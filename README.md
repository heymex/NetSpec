# NetSpec: Declarative Network State Monitor

NetSpec is a next-generation, declarative network monitoring system designed for environments where *state correctness matters more than metrics*.

## Quick Start

### Prerequisites

- Docker and Docker Compose
- Cisco IOS-XE devices reachable via SNMP and/or dial-out telemetry

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
- `APPRISE_API_URL` - Apprise-API base URL (required for alerting). With `network_mode: host` for NetSpec, use `http://127.0.0.1:8086` (or your mapped port). Channel targets come from env vars named in `config/alerts.yaml` under `channels.*.url_env` (for example `APPRISE_SLACK_WEBHOOK`).
- Optional: `APPRISE_NOTIFY_TIMEOUT` (HTTP timeout per notify, e.g. `15s`). Troubleshooting: [Apprise alerting](docs/APPRISE_ALERTING.md).
- `NETSPEC_IMAGE_TAG` - optional container image tag override
- Other optional settings as documented in `.env.example`

`config/desired-state.yaml` supports a telemetry mode switch in `global.telemetry_mode`:
- `snmp_validate_only`: targeted SNMP `GET` validation polling for configured interfaces
- `telemetry_ingest_push`: line-delimited JSON push ingest on `tcp/57500` with targeted SNMP confirmation per event

### Running

The docker-compose file uses the container image built by GitHub Actions from GitHub Container Registry.

**Note**: To pull from GitHub Container Registry, you may need to authenticate:
```bash
echo $GITHUB_TOKEN | docker login ghcr.io -u USERNAME --password-stdin
```

Then start the services:
```bash
docker-compose up -d
```

For IOS-XE dial-out telemetry (`grpc-tcp`) into `telemetry_ingest_push`, run the sidecar overlay:

```bash
docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d
```

This starts:
- `telegraf-mdt` to decode Cisco MDT on `tcp/57500`
- `mdt-translator` (image **`netspec-mdt-translator:local`**, built from **`tools/sidecar`**) to convert decoded records into NetSpec newline-delimited JSON ingest events

The sidecar writes runtime artifacts under `${NETSPEC_DATA_DIR}/mdt-sidecar` (`decoded.json`, `forwarder.log`).

To use a specific image tag instead of `latest`, set the `NETSPEC_IMAGE_TAG` environment variable:
```bash
NETSPEC_IMAGE_TAG=v1.0.0 docker-compose up -d
```

### Building from Source

```bash
go mod download
go build -o netspec ./cmd/netspec
./netspec -config ./config/desired-state.yaml
```

### Local Docker build (same images as prod, faster iteration than CI)

Use this when you want the **same container layout as production** (Apprise + NetSpec, optional telemetry sidecar) but built **on your machine** from the current tree:

```bash
export NETSPEC_DATA_DIR=/opt/netspec   # or your config/data root
make docker-rebuild                    # build image netspec:local (after code changes)
make docker-up                         # apprise + netspec (host network)
# optional: same stack + MDT sidecar as docker-compose.dev.yml
make docker-up-telemetry
```

After each Go or translator Python change, run **`make docker-rebuild`** then **`make docker-up`** or **`make docker-up-telemetry`** (or **`docker compose ... up -d --force-recreate`** for the services you changed). **`make docker-up`** alone does not rebuild images.

Compose files: `docker-compose.yml` + `docker-compose.build-local.yml` (and `docker-compose.dev.yml` for telegraf / `mdt-translator`). Stop any host `nohup ./netspec` or old containers first so port **8088** / ingest port are free.

Because `netspec` uses `network_mode: host` in the default compose stack, `APPRISE_API_URL` must target the host-mapped Apprise port (for example `http://127.0.0.1:8086`). In this topology, `depends_on` controls startup order only and does not guarantee Apprise is fully ready before NetSpec starts.

## MVP Features

This MVP includes:

- ✅ SNMP validator with targeted polling
- ✅ Interface state evaluation
- ✅ Basic alerting via Apprise
- ✅ YAML configuration
- ✅ Docker deployment
- ✅ Web status interface

## Web Interface

NetSpec includes a built-in web UI accessible at `http://localhost:8088` (or your configured host/port).

### Features

- **Dashboard** - Overview of devices, interfaces, and active alerts
- **Device List** - All monitored devices with interface counts
- **Active Alerts** - Current firing alerts with severity indicators
- **Live Logs** - Auto-refreshing log stream (updates every 5 seconds)
- **Configuration View** - Collection interval and dedup settings
- **Config Reload** - Button to reload all configuration files from the config directory without restart
- **API Browser** - Interactive OpenAPI documentation at `/api-browser` (Swagger UI with try-it-out; spec served at `/openapi.json`)

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
| `/api/devices/{name}` | DELETE | Remove device from desired state YAML |
| `/api/devices/{name}/interfaces/{iface}` | PATCH | Update interface policy fields |
| `/api/reload` | POST | Reload configuration |
| `/api/telemetry/stats` | GET | Push ingest counters and top talkers |
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
IOS-XE Dial-Out Telemetry → NetSpec Ingest Receiver → SNMP Targeted Validation → Evaluator → Alert Engine
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

NetSpec uses multiple configuration files:

| File | Required | Purpose |
|------|----------|---------|
| `config/desired-state.yaml` | Yes | Global settings plus optional monolithic device/interface definitions |
| `config/alerts.yaml` | No | Alert channels, routing rules, and alert behavior |
| `config/credentials.yaml` | No | Named credential sets for device authentication references |
| `config/maintenance.yaml` | No | Scheduled maintenance windows (currently loaded but not yet enforced for alert suppression) |
| `config/devices/*.yaml` | No | Per-device split config files for larger deployments |

`desired-state.yaml` does not load an `alerts:` block; alert routing lives in `alerts.yaml`.

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

See `config/desired-state.yaml` and `config/devices/example-device.yaml` for configuration examples.

## Bundled Tools

The container image bundles `gnmic` for operator debugging workflows (for example ad-hoc in-container gNMI queries while troubleshooting). The NetSpec application itself does not shell out to `gnmic` at runtime.

### Cisco IOS-XE Telemetry Setup

For detailed instructions on IOS-XE telemetry and validation patterns, see the [Cisco telemetry setup guide](docs/CISCO_GNMI_SETUP.md).

## CI/CD

GitHub Actions automatically:
- Builds and tests on every push and pull request
- Builds and pushes multi-arch Docker images (linux/amd64, linux/arm64) to GitHub Container Registry for **NetSpec** and the **MDT translator** sidecar
- Images are tagged with: `latest`, branch name, commit SHA, and semantic version tags

### Using the Container Image

Images are published to GitHub Container Registry. Replace `OWNER/REPO` with your repository:

```bash
# Pull the latest NetSpec image
docker pull ghcr.io/OWNER/REPO:latest

# MDT → NetSpec ingest translator (optional; dev compose also builds it locally)
docker pull ghcr.io/OWNER/REPO-mdt-translator:latest

# Or use a specific version
docker pull ghcr.io/OWNER/REPO:v1.0.0
```

## Notes

- Use `/wizard` in the web UI to discover and add devices/interfaces.
- Interface policies can be edited inline from each device page (monitor flag, desired/admin state, alert level).
