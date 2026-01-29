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
