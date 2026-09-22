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
- Sidecar: Compose service **`netspec-mdt`** (metrics **:8089**). Host Python/`mdt-sidecar` is legacy only (`docker-compose.legacy-mdt.yml`).

## Recommended: containerized dev (matches prod)

Use the same **`docker-compose.yml`** (plus **`docker-compose.build-local.yml`** for local builds) so volumes and **bridge** service wiring match production `main`. Build **`netspec:local`** and **`netspec-mdt:local`** on the dev host instead of waiting for GHCR.

1. **Stop legacy processes** so ports **8088**, **57500** (MDT gRPC), **8089** (sidecar metrics), and **8086** are not double-bound: `pkill -x netspec` and stop any host `python3 …/mdt_to_netspec.py` (see §6 for `ps`/`grep` that avoids matching `ssh`).
2. **`NETSPEC_DATA_DIR`** should be one tree containing **`config/`**, **`data/`**, **`apprise-config/`** (same layout as prod). Example: `/opt/netspec` with your files symlinked or copied there. The default Go sidecar does **not** need **`mdt-sidecar/`**.
3. **Compose env:** `.env` supplies `${SNMP_COMMUNITY}`, **`APPRISE_API_URL=http://netspec-apprise:8000`**, **`NETSPEC_INGEST_HOST=netspec-netspec`**, **`NETSPEC_INGEST_PORT`** matching **`global.ingest.port`** (sample **57500**), etc. If you still run a **host** NetSpec binary instead of the container, **`APPRISE_API_URL=http://127.0.0.1:8086`** can still work because Apprise is published on the host—but the **containerized** path should use Docker DNS.
4. Build and start:

```bash
cd /home/derek/NetSpec-dev
export NETSPEC_DATA_DIR=/opt/netspec
export NETSPEC_INGEST_PORT=57500
sudo -E make docker-rebuild
sudo -E make docker-up
```

6. Verify: `curl -sS http://127.0.0.1:8088/health`, `curl -sS http://127.0.0.1:8088/api/telemetry/stats`, and `curl -sS http://127.0.0.1:8089/stats`. Optional: open `http://127.0.0.1:8088/api-browser` for the interactive API reference (loads `/openapi.json`).

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

## 6) Sidecar checks

Default path is **`netspec-mdt`**:

```bash
tsh ssh derek@derek-ghrunner "docker ps --filter name=netspec-mdt --format '{{.Names}} {{.Status}}'"
tsh ssh derek@derek-ghrunner "curl -sS http://127.0.0.1:8089/health && echo && curl -sS http://127.0.0.1:8089/stats"
```

Expect `healthy`, `receiver.packets` climbing, `transformer.emitted` equal to `egress.forward_ok`, and `by_kind` showing interface vs optics vs other encoding paths.

If the sidecar is not running, recreate it from the repo with matching **`NETSPEC_DATA_DIR`** and **`NETSPEC_INGEST_PORT`**:

```bash
cd /home/derek/NetSpec-dev
sudo -E make docker-rebuild
sudo -E make docker-up
```

Legacy Telegraf + Python (`docker-compose.legacy-mdt.yml` only — do not bind host **:57500** twice):

```bash
tsh ssh derek@derek-ghrunner "ps aux | grep '[m]dt_to_netspec'"
tsh ssh derek@derek-ghrunner "tail -n 40 /home/derek/mdt-sidecar/forwarder.log"
```

## 7) Known failure patterns

- `listen tcp :8088: bind: address already in use`
  - Cause: stale NetSpec process still running **or** NetSpec container and host binary both bound to 8088.
  - Fix: choose one runtime (`pkill -x netspec` **or** stop the `netspec` service from Compose), then start once.

- Telemetry counters stay at zero while NetSpec is healthy
  - Cause: **`netspec-mdt`** stopped, wrong `NETSPEC_INGEST_PORT` (must match `global.ingest.port` inside the compose network), or switches still using `grpc-tls`.
  - Fix: `curl http://127.0.0.1:8089/stats`; align YAML ingest port with `.env`; `make docker-up`.

- Legacy `${NETSPEC_DATA_DIR}/mdt-sidecar/decoded.json` grows without bound (100GB+)
  - Cause: Telegraf `outputs.file` on **`docker-compose.legacy-mdt.yml`**. The default Go sidecar does **not** write this file.
  - Fix: migrate to default compose (`netspec-mdt`); stop `netspec-telegraf-mdt` / `netspec-mdt-translator` and remove leftover `decoded.json*`.

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
