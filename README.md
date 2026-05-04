# NetSpec: Declarative Network State Monitor

NetSpec is a next-generation, declarative network monitoring system designed for environments where *state correctness matters more than metrics*.

## Quick Start

### Prerequisites

- Docker and Docker Compose
- Cisco IOS-XE devices reachable via SNMP and/or dial-out telemetry

### Configuration

1. Edit `config/desired-state.yaml` with global settings.
2. Define devices either in `config/desired-state.yaml` (monolithic) or as split files in `config/devices/*.yaml`.
3. Copy `.env.example` to `.env` and update with your credentials:

```bash
cp .env.example .env
# Edit .env with your actual values
```

The `.env` file should contain:
- `SNMP_COMMUNITY` - SNMPv2c community (used by SNMP validation and push-confirmation paths)
- `APPRISE_API_URL` - Apprise-API base URL (required). With `network_mode: host` for NetSpec, use `http://127.0.0.1:8086` (or your mapped port). Channel targets come from env vars named in `alerts.channels.*.url_env` in YAML (for example `APPRISE_SLACK_WEBHOOK`).
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
- **Config Reload** - Button to force re-read of `desired-state.yaml` without restart

### API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | Web UI dashboard |
| `/health` | GET | Health check |
| `/status` | GET | Status summary (JSON) |
| `/alerts` | GET | Active alerts (JSON) |
| `/api/logs` | GET | Recent log entries (JSON) |
| `/api/devices` | GET | Device configuration (JSON) |
| `/api/reload` | POST | Reload configuration |

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

- **`config/desired-state.yaml`** - Global monitoring configuration and optional monolithic device definitions
- **`config/devices/*.yaml`** - (Optional) Split device definitions for large deployments
- **`config/desired-state.yaml`** + **`config/devices/*.yaml`** are the only required runtime config files in the current project state

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
