# NetSpec Dev Host Runbook

This runbook documents the standard operational flow for `derek-ghrunner` so telemetry and UI behavior are reproducible and recoverable.

## Scope

- Host: `derek-ghrunner`
- App checkout: `/home/derek/NetSpec-dev`
- Runtime config: `/home/derek/netspec-config/desired-state.yaml`
- NetSpec process mode: host process (not dockerized app runtime)
- Sidecar files: `/home/derek/mdt-sidecar`

## 1) Sync code to dev host

```bash
tsh ssh derek@derek-ghrunner "cd /home/derek/NetSpec-dev && git pull --ff-only origin main && git rev-parse --short HEAD"
```

## 2) Build NetSpec binary

```bash
tsh ssh derek@derek-ghrunner "cd /home/derek/NetSpec-dev && go build -o netspec ./cmd/netspec"
```

## 3) Restart NetSpec host process

Stop old process:

```bash
tsh ssh derek@derek-ghrunner "pkill -f '^./netspec -config /home/derek/netspec-config/desired-state.yaml -log-level info$' || true"
```

Start process:

```bash
tsh ssh derek@derek-ghrunner "cd /home/derek/NetSpec-dev && nohup env SNMP_COMMUNITY=public APPRISE_API_URL=http://localhost:8086 ./netspec -config /home/derek/netspec-config/desired-state.yaml -log-level info > /home/derek/NetSpec-dev/netspec-dev.log 2>&1 < /dev/null &"
```

## 4) Verify NetSpec health

```bash
tsh ssh derek@derek-ghrunner "pgrep -af '^./netspec -config /home/derek/netspec-config/desired-state.yaml -log-level info$'"
tsh ssh derek@derek-ghrunner "curl -sS http://localhost:8088/health && echo && curl -sS http://localhost:8088/status"
```

## 5) Telemetry ingest checks

Check listener and stats:

```bash
tsh ssh derek@derek-ghrunner "ss -ltnp | sed -n '1,120p'"
tsh ssh derek@derek-ghrunner "curl -sS http://localhost:8088/api/telemetry/stats"
```

Expected:
- NetSpec listens on `:8088` and ingest port from config (currently `:57501`).
- `received` and `accepted` counters increase.

## 6) Sidecar forwarder checks

Verify sidecar process:

```bash
tsh ssh derek@derek-ghrunner "pgrep -af mdt_to_netspec.py"
```

Verify forwarder activity:

```bash
tsh ssh derek@derek-ghrunner "tail -n 40 /home/derek/mdt-sidecar/forwarder.log"
```

If forwarder is not running, restart it against the current NetSpec ingest port:

```bash
tsh ssh derek@derek-ghrunner "cd /home/derek/mdt-sidecar && nohup env MDT_DECODED_FILE=/home/derek/mdt-sidecar/decoded.json NETSPEC_INGEST_HOST=127.0.0.1 NETSPEC_INGEST_PORT=57501 MDT_FORWARDER_LOG=/home/derek/mdt-sidecar/forwarder.log python3 /home/derek/mdt-sidecar/mdt_to_netspec.py > /home/derek/mdt-sidecar/forwarder.stdout.log 2>&1 < /dev/null &"
```

## 7) Known failure patterns

- `listen tcp :8088: bind: address already in use`
  - Cause: stale NetSpec process still running.
  - Fix: kill old process, then restart once.

- Telemetry counters stay at zero while NetSpec is healthy
  - Cause: forwarder process stopped or wrong ingest port.
  - Fix: restart forwarder with matching `NETSPEC_INGEST_PORT`.

- Container restart confusion
  - Current operational runtime uses host process (`./netspec`), not the containerized app process.
  - If testing with docker compose, ensure port/config paths are correct and avoid dual-running.
