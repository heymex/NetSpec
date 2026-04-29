# NetSpec: Declarative Network State Monitor

NetSpec is a next-generation, declarative network monitoring system designed for environments where *state correctness matters more than metrics*.

## Quick Start

### Prerequisites

- Docker and Docker Compose
- Cisco IOS-XE devices with gNMI enabled
- gNMI credentials

### Configuration

1. Edit `config/desired-state.yaml` with your devices and interfaces
2. Copy `config/alerts.yaml.example` to `config/alerts.yaml` and configure notification channels:

```bash
cp config/alerts.yaml.example config/alerts.yaml
# Edit config/alerts.yaml with your notification channels
```

3. Copy `.env.example` to `.env` and update with your credentials:

```bash
cp .env.example .env
# Edit .env with your actual values
```

The `.env` file should contain:
- `GNMI_PASSWORD` - Required password for gNMI connections
- `GNMI_USERNAME` - gNMI username (defaults to `gnmi-monitor`)
- `SNMP_COMMUNITY` - SNMPv2c community when using `snmp_validate_only` mode
- `APPRISE_SLACK_WEBHOOK` - Slack notification URL (set in alerts.yaml)
- `APPRISE_TEAMS_WEBHOOK` - Teams notification URL (set in alerts.yaml)
- `APPRISE_API_URL` - Apprise API URL (defaults to `http://apprise:8000`)
- Other optional settings as documented in `.env.example`

The `config/alerts.yaml` file configures:
- Notification channels (Slack, Teams, OpsGenie, Email, etc.)
- Alert routing rules by severity
- Deduplication and flap detection settings
- State persistence configuration

`config/desired-state.yaml` also supports a telemetry mode switch in `global.telemetry_mode`:
- `gnmi_pull` (default): current direct gNMI collection
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
- `mdt-translator` to convert decoded records into NetSpec newline-delimited JSON ingest events

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

## MVP Features

This MVP includes:

- ✅ gNMI collector with connection handling
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
- **Configuration View** - Current gNMI port, collection interval, and dedup settings
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

```
gNMI Stream → State Evaluator → Alert Engine → Apprise
```

### Current Telemetry Modes

NetSpec currently supports three runtime collection modes:

- `gnmi_pull` - NetSpec connects directly to each device's gNMI server and evaluates streamed updates.
- `snmp_validate_only` - NetSpec polls targeted SNMP interface OIDs (`ifAdminStatus`/`ifOperStatus`) for configured interfaces and evaluates those snapshots.
- `telemetry_ingest_push` - NetSpec listens on `global.ingest.listen_address:global.ingest.port` (default `0.0.0.0:57500`) for newline-delimited JSON events. Each event can be SNMP-confirmed before entering the evaluator.

### Push-First Direction (Recommended for IOS-XE 17.12.x)

For unstable IOS-XE gNMI pull behavior, the preferred operating model is:

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

- **`config/desired-state.yaml`** - Device and interface monitoring configuration
- **`config/alerts.yaml`** - Alert routing and notification channel configuration (see `config/alerts.yaml.example`)
- **`config/credentials.yaml`** - (Optional) Credential management
- **`config/maintenance.yaml`** - (Optional) Maintenance window definitions

See `config/desired-state.yaml` and `config/alerts.yaml.example` for configuration examples.

### Cisco IOS-XE gNMI Setup

For detailed instructions on configuring gNMI on Cisco IOS-XE devices, see the [Cisco gNMI Setup Guide](docs/CISCO_GNMI_SETUP.md).

## CI/CD

GitHub Actions automatically:
- Builds and tests on every push and pull request
- Builds and pushes multi-arch Docker images (linux/amd64, linux/arm64) to GitHub Container Registry
- Images are tagged with: `latest`, branch name, commit SHA, and semantic version tags

### Using the Container Image

Images are published to GitHub Container Registry. Replace `OWNER/REPO` with your repository:

```bash
# Pull the latest image
docker pull ghcr.io/OWNER/REPO:latest

# Or use a specific version
docker pull ghcr.io/OWNER/REPO:v1.0.0
```

## License

See LICENSE file for details.
