# Migration guide: bridge networking and authentication

This guide covers upgrading an existing NetSpec deployment to the bridge networking
model introduced in this release and optionally enabling password authentication.

Both changes ship in the same image update. The networking migration **requires action**
for most existing deployments; authentication is opt-in and non-breaking.

---

## What changed

| Area | Before | After |
|------|--------|-------|
| Container networking | `network_mode: host` on netspec, telegraf, translator | All services on `netspec` bridge |
| Web UI port | Bound directly to host `:8088` | Published via `ports: 8088:8088` |
| MDT ingest port | Bound to host `:57500` (telegraf) | Published via `ports: 57500:57500` |
| Apprise URL | `http://127.0.0.1:8086` (loopback) | `http://netspec-apprise:8000` (Docker DNS) |
| Translator → NetSpec | `NETSPEC_INGEST_HOST=127.0.0.1` | `NETSPEC_INGEST_HOST=netspec-netspec` |
| Authentication | None | Optional; disabled by default |

---

## Before you begin

- Pull the new `docker-compose.yml` (or let your orchestrator sync it).
- Note any variables you have **explicitly set** in your `.env`:
  - `APPRISE_API_URL`
  - `NETSPEC_INGEST_HOST`

  If either is set to `127.0.0.1` or a loopback address, it must be updated.
  Variables not present in your `.env` fall through to the new compose defaults automatically.

---

## Step 1 — Update your `.env`

### APPRISE_API_URL

If your `.env` contains this line:

```
APPRISE_API_URL=http://127.0.0.1:8086
```

Change it to:

```
APPRISE_API_URL=http://netspec-apprise:8000
```

If you use an **external** Apprise instance (not the bundled container), leave the URL
as-is — it already points at a real address that bridge containers can route to.

If `APPRISE_API_URL` is **not set** in your `.env`, no change is needed; the compose
default has already been updated.

### NETSPEC_INGEST_HOST

If your `.env` contains:

```
NETSPEC_INGEST_HOST=127.0.0.1
```

Change it to:

```
NETSPEC_INGEST_HOST=netspec-netspec
```

If the variable is absent from your `.env`, no change is needed.

---

## Step 2 — Restart the stack

```bash
cd /path/to/NetSpec
docker compose pull
docker compose up -d
```

The stack will recreate all containers on the bridge network.

### Verify

```bash
# Web UI and API reachable
curl -s http://localhost:8088/health

# Apprise reachable from NetSpec (check logs for delivery errors)
docker logs netspec-netspec 2>&1 | grep -i apprise

# Translator forwarding to NetSpec ingest (should show sent= incrementing)
tail -f /opt/netspec/mdt-sidecar/forwarder.log
```

If telemetry was previously flowing, you should see events resume within one
collection interval (~60 s) once the translator reconnects to `netspec-netspec:57500`.

---

## Step 3 — Enable authentication (optional)

Authentication is disabled by default. Setting `NETSPEC_ADMIN_PASSWORD_HASH` enables it.

### Generate a password hash

```bash
docker run --rm ghcr.io/heymex/netspec:latest hash-password
# Prompts for password, prints bcrypt hash — paste it below.
```

Or pass the password directly (avoid on shared hosts where process list is visible):

```bash
docker run --rm ghcr.io/heymex/netspec:latest hash-password 'my-password'
```

### Add to `.env`

```
NETSPEC_ADMIN_PASSWORD_HASH=$2a$10$...paste-hash-here...
```

### Restart the NetSpec container

```bash
docker compose restart netspec-netspec
```

The login page will appear at `http://your-host:8088/login` on next page load.
All routes except `/health` require a valid session; sessions last 24 hours.

---

## Optional — API token for scripts

If you poll the NetSpec API from scripts or external tools, add a bearer token so they
don't depend on a session cookie:

```
NETSPEC_API_TOKEN=your-secret-token
```

Then authenticate API calls with:

```bash
curl -H "Authorization: Bearer your-secret-token" http://localhost:8088/api/devices
```

Restart `netspec-netspec` after adding the variable. The bearer token and session cookie
are both accepted simultaneously — browser users get the cookie, scripts get the token.

---

## Common failures

| Symptom | Cause | Fix |
|---------|-------|-----|
| Apprise delivery errors in logs after restart | `APPRISE_API_URL` still points at `127.0.0.1` | Update to `http://netspec-apprise:8000` and restart |
| `forwarder.log` stops incrementing | Translator can't reach netspec on `127.0.0.1:57500` | Update `NETSPEC_INGEST_HOST=netspec-netspec` and restart |
| Port `57500` already in use | Another process on the host held the port | `ss -tlnp | grep 57500`; stop the conflicting process |
| Port `8088` already in use | Same as above | `ss -tlnp | grep 8088` |
| Login page appears but auth was not intended | `NETSPEC_ADMIN_PASSWORD_HASH` was set in `.env` unintentionally | Remove or blank the variable and restart |
| Password correct but login fails immediately | Hash was generated for a different password string | Re-run `hash-password` and update the variable |
| Bearer token returns 401 | Token in request doesn't match `NETSPEC_API_TOKEN` | Check for leading/trailing whitespace in the env var |

---

## Notes for existing documentation

- **`docs/APPRISE_ALERTING.md`** references `http://127.0.0.1:8086` as the default
  Apprise URL. That example applied to the old host-network model. The current default
  is `http://netspec-apprise:8000`.
- **`docs/DEV_HOST_RUNBOOK.md`** describes the dev workflow against the host-network
  stack. Update `APPRISE_API_URL` references there when refreshing the dev environment.
