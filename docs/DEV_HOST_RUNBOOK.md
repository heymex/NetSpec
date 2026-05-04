# NetSpec Dev Host Runbook

This runbook documents the standard operational flow for `derek-ghrunner` so telemetry and UI behavior are reproducible and recoverable.

## Scope

- Host: `derek-ghrunner`
- App checkout: `/home/derek/NetSpec-dev`
- Runtime config: `/home/derek/netspec-config/desired-state.yaml`
- Optional alert routing: `/home/derek/netspec-config/alerts.yaml` (required for Apprise delivery; not read from `desired-state.yaml`)
- Host env for NetSpec: `/home/derek/netspec-config/netspec.env` (sourced by `restart-netspec-dev.sh`)
- NetSpec process mode: **prefer Docker** (see below); legacy option was host `./netspec` for fast Go iteration
- Sidecar files: `/home/derek/mdt-sidecar` (or `${NETSPEC_DATA_DIR}/mdt-sidecar` when using Compose)

## Recommended: containerized dev (matches prod)

Use the same **`docker-compose.yml`** (plus **`docker-compose.build-local.yml`**) so Apprise port mapping, volumes, and NetSpec **host networking** match production. Build **`netspec:local`** on the dev host instead of waiting for GHCR.

1. **Stop legacy processes** so ports **8088**, **57501** (or your ingest port), and **8086** are not double-bound: `pkill -x netspec` and stop any host `python3 …/mdt_to_netspec.py` (see §6 for `ps`/`grep` that avoids matching `ssh`).
2. **`NETSPEC_DATA_DIR`** should be one tree containing **`config/`**, **`data/`**, **`apprise-config/`**, **`mdt-sidecar/`** (same layout as prod). Example: `/opt/netspec` with your files symlinked or copied there.
3. **Compose env interpolation:** from the repo directory, Docker Compose reads **`.env`** in that directory for `${SNMP_COMMUNITY}`, `${APPRISE_API_URL}`, etc. Either copy/link `netspec.env` → `.env` in the checkout or `export` those variables before `make`. For host-network NetSpec talking to Apprise on the host, use **`APPRISE_API_URL=http://127.0.0.1:8086`** (not `http://apprise:8000`).
4. **Ingest port for the sidecar:** set **`NETSPEC_INGEST_PORT`** (and ingest in `desired-state.yaml`) consistently, e.g. `57501`.
5. Build and start (telemetry overlay = telegraf + `mdt-translator` in containers):

```bash
cd /home/derek/NetSpec-dev
export NETSPEC_DATA_DIR=/opt/netspec
export NETSPEC_INGEST_PORT=57501
sudo -E make docker-rebuild
sudo -E make docker-up-telemetry
```

6. Verify: `curl -sS http://127.0.0.1:8088/health` and `curl -sS http://127.0.0.1:8088/api/telemetry/stats`.

Do **not** run **`restart-netspec-dev.sh`** at the same time as the NetSpec container (both would bind **8088**).

## 0) Apprise and `APPRISE_API_URL`

NetSpec runs on the **host**, so `APPRISE_API_URL` in `netspec.env` must reach the published Apprise port on **localhost** (e.g. `http://127.0.0.1:8086`). A value like `http://apprise:8000` only works **inside** Docker DNS and will fail DNS resolution on the host.

If `curl http://127.0.0.1:8086/status` returns `Connection reset by peer`, the host port may be mapped to the **wrong container port**. The linuxserver `apprise-api` image serves uWSGI on **8000** inside the container; the publish mapping must be **`8086:8000`** (not `8086:8086`). Fix with sudo: `sudo docker stop apprise && sudo docker rm apprise` then `docker run ... -p 8086:8000 ...` (see repo `docker-compose.yml`).

Use **`sudo docker ...`** when your user cannot access `/var/run/docker.sock`.

## 1) Sync code to dev host

```bash
tsh ssh derek@derek-ghrunner "cd /home/derek/NetSpec-dev && git pull --ff-only origin main && git rev-parse --short HEAD"
```

Until changes are merged to `main`, you can sync specific files from a local checkout:

```bash
cd /path/to/NetSpec
tar czf - cmd/netspec/main.go internal/notifier/apprise.go internal/notifier/apprise_test.go internal/alerter/engine.go \
  | tsh ssh derek@derek-ghrunner 'cd /home/derek/NetSpec-dev && tar xzf -'
```

## 2) Build NetSpec binary

```bash
tsh ssh derek@derek-ghrunner "cd /home/derek/NetSpec-dev && go build -o netspec ./cmd/netspec"
```

## 3) Restart NetSpec host process

Prefer the checked-in script (sources `netspec.env`, rebuilds binary, manages `pkill`):

```bash
tsh ssh derek@derek-ghrunner "bash /home/derek/netspec-config/restart-netspec-dev.sh"
```

Manual stop/start (only if not using the script):

```bash
tsh ssh derek@derek-ghrunner "pkill -x netspec || true"
# then start with the same env pattern as restart-netspec-dev.sh
```

## 4) Verify NetSpec health

```bash
tsh ssh derek@derek-ghrunner "pgrep -x netspec && pgrep -af netspec | head -3"
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

Verify sidecar process (avoid `pgrep -f mdt_to_netspec.py` alone — it can match the wrapping `ssh` command; prefer `ps aux | grep '[m]dt_to_netspec'`):

```bash
tsh ssh derek@derek-ghrunner "ps aux | grep '[m]dt_to_netspec'"
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
