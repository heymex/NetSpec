# NetSpec: Declarative Network State Monitor

NetSpec is a next-generation, declarative network monitoring system designed for environments where *state correctness matters more than metrics*.

## Quick Start

### Prerequisites

- Docker and Docker Compose
- Cisco IOS-XE devices with gNMI enabled
- gNMI credentials

### Configuration

1. Edit `config/desired-state.yaml` with your devices and interfaces
2. Copy `.env.example` to `.env` and update with your credentials:

```bash
cp .env.example .env
# Edit .env with your actual values
```

The `.env` file should contain:
- `GNMI_PASSWORD` - Required password for gNMI connections
- `GNMI_USERNAME` - gNMI username (defaults to `gnmi-monitor`)
- `APPRISE_SLACK_WEBHOOK` - Slack notification URL (optional)
- `APPRISE_TEAMS_WEBHOOK` - Teams notification URL (optional)
- Other optional settings as documented in `.env.example`

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

## Architecture

```
gNMI Stream → State Evaluator → Alert Engine → Apprise
```

## Configuration

See `config/desired-state.yaml` for configuration examples.

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
