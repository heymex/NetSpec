# Apprise alerting (NetSpec)

NetSpec sends firing and resolved alerts to [Apprise-API](https://github.com/caronc/apprise-api) using JSON `POST {APPRISE_API_URL}/notify` (stateless, with `urls`) or `POST …/notify/{key}/` (pre-stored config keys).

## Configuration

| Piece | Where |
|--------|--------|
| API base URL | Environment **`APPRISE_API_URL`** (required), e.g. `http://127.0.0.1:8086` when NetSpec uses host networking and Apprise publishes `8086:8000`. |
| Per-destination URLs | Environment variables named in **`alerts.yaml`** → `channels.*.url_env` (e.g. `APPRISE_SLACK_WEBHOOK=slack://…`). |
| Routing | **`config/alerts.yaml`** (loaded from the config directory next to `desired-state.yaml`). |
| HTTP timeout | Optional **`APPRISE_NOTIFY_TIMEOUT`** (Go duration, default `10s`), e.g. `15s`, `30s`. |

## Verifying Apprise before NetSpec

1. **Status** (should return `OK`):

   ```bash
   curl -sS "${APPRISE_API_URL%/}/status"
   ```

2. **Stateless notify** (replace `urls` with a real Apprise URL you own; some schemes may return HTTP **424** if delivery from inside the container fails—that still proves routing to Apprise works):

   ```bash
   curl -sS -X POST "${APPRISE_API_URL%/}/notify" \
     -H "Content-Type: application/json" \
     -d '{"title":"t","body":"b","format":"text","type":"info","urls":"json://localhost"}'
   ```

## Verifying NetSpec → Apprise

1. Set **`LOG_LEVEL=debug`** for the NetSpec process so the notifier logs each attempt (`delivery`, `path`, `url_env_target`).

2. Trigger or wait for an alert, then search logs for **`component":"notifier"`** or **`apprise notify`**.

3. On failure, errors include **`apprise-api HTTP …`** with Apprise’s JSON **`error`** / **`details`** when present.

## Common failures

| Symptom | Likely cause |
|---------|----------------|
| `Connection reset` on `curl` to `:8086` | Docker published **host 8086 → container 8086** but linuxserver **apprise-api** listens on **8000** inside the image. Use **`8086:8000`** (see repo `docker-compose.yml`). |
| `APPRISE_API_URL is not set` | Missing env in the NetSpec process (compose `.env`, `netspec.env`, or systemd `Environment=`). |
| `environment variable X is not set or empty` | Variable is absent in the NetSpec **process**. With Docker Compose you must declare **`env_file`** and/or **`environment`** entries so each `url_env` name reaches the container; presence in compose `.env` alone only interpolates **`${VAR}`** lines in the YAML—see **`netspec-netspec`** in `docker-compose.yml`. |
| `unknown alert channel` | `alert_rules` references a name missing from `alerts.channels`. |
| HTTP **424** from `/notify` | Apprise accepted the HTTP request but could not deliver to the given `urls` (bad token, network from container, etc.). Inspect **`details`** in the JSON body. |
| Channel skipped (debug only) | **`severity_filter`** on that channel excludes the alert’s severity. |

## Docker vs host NetSpec

- **Host-network NetSpec** (`netspec-netspec` in Compose) must use **`APPRISE_API_URL` pointing at localhost** (or the host’s published Apprise port), not `http://netspec-apprise:8000` (that hostname only resolves for containers on the Compose **`netspec`** bridge, not in the host network namespace).

- **Container NetSpec with `network_mode: host`** is the same: use **localhost** for Apprise on the host.
