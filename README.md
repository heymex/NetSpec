# NetSpec: Declarative Network State Monitor

NetSpec is a next-generation, declarative network monitoring system designed for environments where *state correctness matters more than metrics*.

## Quick Start

### Prerequisites

- Docker and Docker Compose
- Cisco IOS-XE devices with gNMI enabled
- gNMI credentials

### Configuration

1. Edit `config/desired-state.yaml` with your devices and interfaces
2. Create `.env` file with credentials:

```bash
GNMI_USERNAME=gnmi-monitor
GNMI_PASSWORD=your-password
APPRISE_SLACK_WEBHOOK=slack://tokenA/tokenB/tokenC
APPRISE_TEAMS_WEBHOOK=msteams://...
```

### Running

```bash
docker-compose up -d
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

## Architecture

```
gNMI Stream → State Evaluator → Alert Engine → Apprise
```

## Configuration

See `config/desired-state.yaml` for configuration examples.

## License

See LICENSE file for details.
