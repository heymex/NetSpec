# NetSpec Development Guide

This guide explains how to run NetSpec as a native Go application for rapid development and debugging.

## Prerequisites

- Go 1.21 or later
- Access to Cisco IOS-XE devices with gNMI enabled
- gNMI credentials

## Quick Start

### 1. Install Dependencies

```bash
go mod download
```

### 2. Set Up Configuration

Ensure you have your configuration files in place:

```bash
# Copy example configs if needed
cp config/alerts.yaml.example config/alerts.yaml

# Edit your desired-state.yaml
# Edit your alerts.yaml
```

### 3. Set Environment Variables

```bash
export GNMI_PASSWORD="your-password"
export GNMI_USERNAME="netspec-monitor"  # Optional, defaults to "gnmi-monitor"
export API_PORT="8088"  # Optional, defaults to 8088
export LOG_LEVEL="debug"  # Optional: debug, info, warn, error (default: info)
```

### 4. Run NetSpec

```bash
# Build and run
go run ./cmd/netspec -config ./config/desired-state.yaml -log-level debug

# Or build first, then run
go build -o netspec ./cmd/netspec
./netspec -config ./config/desired-state.yaml -log-level debug
```

### 5. Access the Web UI

Open your browser to: `http://localhost:8088`

## Development Workflow

### Hot Reloading

For rapid development, use a file watcher like `air` or `nodemon`:

```bash
# Install air (Go hot reload tool)
go install github.com/cosmtrek/air@latest

# Run with air (auto-reloads on file changes)
air
```

Or use `nodemon` with a custom command:

```bash
# Install nodemon
npm install -g nodemon

# Run with nodemon
nodemon --watch . --ext go --exec "go run ./cmd/netspec -config ./config/desired-state.yaml -log-level debug"
```

### Debugging

#### Using Delve (Go Debugger)

```bash
# Install delve
go install github.com/go-delve/delve/cmd/dlv@latest

# Run with delve
dlv debug ./cmd/netspec -- -config ./config/desired-state.yaml -log-level debug

# In delve console:
# (dlv) break main.main
# (dlv) continue
# (dlv) next
# (dlv) print variableName
```

#### Using VS Code

1. Install the Go extension
2. Create `.vscode/launch.json`:
```json
{
    "version": "0.2.0",
    "configurations": [
        {
            "name": "Launch NetSpec",
            "type": "go",
            "request": "launch",
            "mode": "debug",
            "program": "${workspaceFolder}/cmd/netspec",
            "args": [
                "-config", "./config/desired-state.yaml",
                "-log-level", "debug"
            ],
            "env": {
                "GNMI_PASSWORD": "your-password",
                "GNMI_USERNAME": "netspec-monitor",
                "API_PORT": "8088"
            }
        }
    ]
}
```

3. Press F5 to start debugging

### Logging

NetSpec uses `zerolog` for structured logging. Log levels:

- `debug` - Verbose output, includes all gNMI messages
- `info` - Normal operation messages (default)
- `warn` - Warnings and non-fatal errors
- `error` - Errors only

Set log level via:
- Command line: `-log-level debug`
- Environment: `LOG_LEVEL=debug`

### Testing Changes

#### Unit Tests

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests for a specific package
go test ./internal/collector
```

#### Integration Testing

1. Start NetSpec with a test device
2. Monitor logs for connection status
3. Check web UI at `http://localhost:8088`
4. Test config reload via UI or API:
   ```bash
   curl -X POST http://localhost:8088/api/reload
   ```

### Common Development Tasks

#### Adding a New Feature

1. Create feature branch: `git checkout -b feature/my-feature`
2. Make changes
3. Run tests: `go test ./...`
4. Test manually: `go run ./cmd/netspec -config ./config/desired-state.yaml`
5. Commit and push

#### Testing gNMI Collector Changes

1. Set log level to `debug` to see all gNMI messages
2. Monitor connection status in logs
3. Check device health via API:
   ```bash
   curl http://localhost:8088/api/devices/device-name
   ```

#### Testing Configuration Changes

1. Edit `config/desired-state.yaml`
2. Use the "Reload Config" button in the web UI, or:
   ```bash
   curl -X POST http://localhost:8088/api/reload
   ```
3. Check logs to verify collectors restarted

## Configuration File Locations

When running natively, NetSpec looks for config files relative to the `-config` path:

- `desired-state.yaml` - Path specified by `-config` flag
- `alerts.yaml` - Same directory as `desired-state.yaml`
- `credentials.yaml` - Same directory as `desired-state.yaml` (optional)
- `maintenance.yaml` - Same directory as `desired-state.yaml` (optional)

Example:
```bash
./netspec -config ./config/desired-state.yaml
# Will look for:
# - ./config/desired-state.yaml
# - ./config/alerts.yaml
# - ./config/credentials.yaml
# - ./config/maintenance.yaml
```

## Troubleshooting

### Connection Issues

1. Check gNMI port is correct (default: 9338 for non-TLS, 9339 for TLS)
2. Verify credentials: `gnmic -a <device-ip>:9338 -u <username> -p <password> --insecure capabilities`
3. Check firewall rules
4. Enable debug logging: `-log-level debug`

### Build Errors

```bash
# Clean module cache
go clean -modcache

# Update dependencies
go get -u ./...
go mod tidy
```

### Port Already in Use

Change the API port:
```bash
export API_PORT="8089"
go run ./cmd/netspec -config ./config/desired-state.yaml
```

## Performance Tips

- Use `info` log level in production (debug is verbose)
- Monitor memory usage with `go tool pprof` if needed
- Use `-race` flag to detect race conditions during development:
  ```bash
  go run -race ./cmd/netspec -config ./config/desired-state.yaml
  ```

## Next Steps

- See [README.md](README.md) for deployment instructions
- See [docs/CISCO_GNMI_SETUP.md](docs/CISCO_GNMI_SETUP.md) for gNMI configuration
- See [config/alerts.yaml.example](config/alerts.yaml.example) for alert configuration
