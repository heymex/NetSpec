# NetSpec Dev Host Runbook

> **v2.x compose** uses **Docker bridge** networking by default (`APPRISE_API_URL=http://netspec-apprise:8000`, `NETSPEC_INGEST_HOST=netspec-netspec`, ingest **57500** in the sample). Legacy **host-network** notes below are marked where they still apply to **bare-metal `./netspec`** debugging. See **[MIGRATION_BRIDGE_AND_AUTH.md](MIGRATION_BRIDGE_AND_AUTH.md)** for the production cutover story.

This runbook documents the standard operational flow for `derek-ghrunner` so telemetry and UI behavior are reproducible and recoverable.

## Scope

- Host: `derek-ghrunner`
- App checkout: `/home/derek/NetSpec-dev`
- Runtime config: `/home/derek/netspec-config/desired-state.yaml`
- Alert routing: `/home/derek/netspec-config/alerts.yaml` (required for Apprise delivery; the loader does **not** read a top-level `alerts:` key from `desired-state.yaml`)
- Host env for NetSpec: `/home/derek/netspec-config/netspec.env` (sourced by `restart-netspec-dev.sh`; the Go binary also auto-loads `netspec.env` and `.env` in the **config directory** when started directly, without overriding variables already set in the process environment)
- NetSpec process mode: **prefer Docker** (see below); legacy option was host `./netspec` for fast Go iteration
- Sidecar files: `/home/derek/mdt-sidecar` (or `${NETSPEC_DATA_DIR}/mdt-sidecar` when using Compose)

## Recommended: containerized dev (matches prod)

Use the same **`docker-compose.yml`** (plus **`docker-compose.build-local.yml`** for local builds) so volumes and **bridge** service wiring match production `main`. Build **`netspec:local`** and **`netspec-mdt-translator:local`** on the dev host instead of waiting for GHCR.

1. **Stop legacy processes** so ports **8088**, **57500** (default MDT / ingest publish), and **8086** are not double-bound: `pkill -x netspec` and stop any host `python3 …/mdt_to_netspec.py` (see §6 for `ps`/`grep` that avoids matching `ssh`).
2. **`NETSPEC_DATA_DIR`** should be one tree containing **`config/`**, **`data/`**, **`apprise-config/`**, **`mdt-sidecar/`** (same layout as prod). Example: `/opt/netspec` with your files symlinked or copied there.
3. **Compose env:** `.env` supplies `${SNMP_COMMUNITY}`, **`APPRISE_API_URL=http://netspec-apprise:8000`**, **`NETSPEC_INGEST_HOST=netspec-netspec`**, **`NETSPEC_INGEST_PORT`** matching **`global.ingest.port`** (sample **57500**), etc. If you still run a **host** NetSpec binary instead of the container, **`APPRISE_API_URL=http://127.0.0.1:8086`** can still work because Apprise is published on the host—but the **containerized** path should use Docker DNS.
4. Build and start:

```bash
cd /home/derek/NetSpec-dev
export NETSPEC_DATA_DIR=/opt/netspec
export NETSPEC_INGEST_PORT=57500
sudo -E make docker-rebuild
sudo -E make docker-up
```

6. Verify: `curl -sS http://127.0.0.1:8088/health` and `curl -sS http://127.0.0.1:8088/api/telemetry/stats`. Optional: open `http://127.0.0.1:8088/api-browser` for the interactive API reference (loads `/openapi.json`).

Do **not** run **`restart-netspec-dev.sh`** at the same time as the NetSpec container (both would bind **8088**).

## 0) Apprise and `APPRISE_API_URL`

**Compose (default):** NetSpec containers use **`APPRISE_API_URL=http://netspec-apprise:8000`** (bridge DNS). Operators can still **`curl http://127.0.0.1:8086`** from the host because Apprise publishes **`8086:8000`**.

**Legacy bare-metal NetSpec** on the host: `APPRISE_API_URL` in `netspec.env` must reach the published Apprise port on **localhost** (e.g. `http://127.0.0.1:8086`). A Docker-only hostname like `http://netspec-apprise:8000` will fail unless the host participates in that network namespace.

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
- NetSpec listens on `:8088` (inside the container; mapped to host) and ingest port from **`global.ingest`** (sample **57500** on bridge).
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

If forwarder is not running, prefer the **containerized translator**: `make docker-up` or `docker compose -f docker-compose.yml -f docker-compose.build-local.yml up -d netspec-mdt-translator` from the NetSpec repo with matching **`NETSPEC_DATA_DIR`** and **`NETSPEC_INGEST_PORT`**.

Legacy host fallback (only if you are not using Compose for the translator):

```bash
tsh ssh derek@derek-ghrunner "cd /home/derek/mdt-sidecar && nohup env MDT_DECODED_FILE=/home/derek/mdt-sidecar/decoded.json NETSPEC_INGEST_HOST=127.0.0.1 NETSPEC_INGEST_PORT=57501 MDT_FORWARDER_LOG=/home/derek/mdt-sidecar/forwarder.log python3 /home/derek/mdt-sidecar/mdt_to_netspec.py > /home/derek/mdt-sidecar/forwarder.stdout.log 2>&1 < /dev/null &"
```

## 7) Known failure patterns

- `listen tcp :8088: bind: address already in use`
  - Cause: stale NetSpec process still running **or** NetSpec container and host binary both bound to 8088.
  - Fix: choose one runtime (`pkill -x netspec` **or** stop the `netspec` service from Compose), then start once.

- Telemetry counters stay at zero while NetSpec is healthy
  - Cause: `mdt-translator` / forwarder stopped, wrong `NETSPEC_INGEST_PORT`, or Telegraf not writing `decoded.json`.
  - Fix: align `global.ingest.port` in YAML with `NETSPEC_INGEST_PORT`; restart `make docker-up` or the translator container.

- Container vs host binary
  - Prefer **one** runtime: containerized NetSpec (this runbook § “Recommended: containerized dev”) **or** host `./netspec` for quick Go iteration—not both on the same ports. This host has often run the **host** `./netspec` process operationally; if you switch to Compose, ensure port/config paths are correct.
  - If testing with Docker Compose, ensure `NETSPEC_DATA_DIR` contains `config/`, `data/`, and `apprise-config/` as in production, and avoid dual-running with a legacy host NetSpec on **8088** / the ingest port.

## 8) Opening a GitHub PR from the dev host (`gh`)

**Best practice:** create the PR from the **branch already pushed to `origin`**, without checking that branch out in a dirty working tree. That avoids losing or merging local-only edits on the server (e.g. tar-patched files, experiments).

1. Push your branch from your laptop (or merge via GitHub UI) so **`origin/<branch>`** exists.
2. On **`derek-ghrunner`**, use **`gh pr create --head <branch>`** from any directory in the clone; **`git checkout`** of the feature branch is **not** required.

```bash
cd /home/derek/NetSpec-dev
git fetch origin
gh pr create --repo heymex/NetSpec --base main --head feature/your-branch \
  --title "Your title" --body "Your description."
```

Or use the repo helper (same behavior, resolves clone path automatically when run inside the repo):

```bash
cd /home/derek/NetSpec-dev
./scripts/gh-pr-create.sh feature/your-branch "Your title" "Your description."
```

**Avoid:** `git checkout feature/your-branch` when you have uncommitted changes in **`NetSpec-dev`** unless you intend to carry or discard them (**`git stash`** / commit first).

**Alternative:** run **`gh pr create`** from your **laptop** clone after **`git push`** (same model; no server needed).
