# OpenClaw alerting (NetSpec)

NetSpec can POST structured alert events to an [OpenClaw](https://github.com/openclaw/openclaw) Gateway webhook (`hooks.enabled` + shared token). Use a **mapped hook** so OpenClaw can turn the NetSpec payload into a `wake` or `agent` action.

## Configuration

| Piece | Where |
|--------|--------|
| Channel | `config/alerts.yaml` → `channels.*.type: openclaw` |
| Webhook URL | Env named by `url_env` (full URL, e.g. `http://openclaw:18789/hooks/netspec`) |
| Hook token | Env named by `token_env` (optional but recommended) → sent as `Authorization: Bearer` and `x-openclaw-token` |
| UI links | **`NETSPEC_PUBLIC_URL`** (optional) — when set, payload includes `links.alert` and `links.device` |
| Routing | Add the channel name under `alert_rules` (same as Apprise) |

Example `alerts.yaml` fragment:

```yaml
channels:
  ops-openclaw:
    type: openclaw
    url_env: OPENCLAW_WEBHOOK_URL
    token_env: OPENCLAW_HOOK_TOKEN
    severity_filter: [warning, critical]

alert_rules:
  critical:
    channels: [ops-slack, ops-openclaw]
  warning:
    channels: [ops-openclaw]
```

## Payload shape

Firing example:

```json
{
  "event": "alert.firing",
  "alert": {
    "id": "core-sw-01|Port-channel10|port_channel_degraded-1723312800000",
    "device": "core-sw-01",
    "entity": "Port-channel10",
    "alert_type": "port_channel_degraded",
    "severity": "critical",
    "state": "firing",
    "fired_at": "2026-08-10T20:00:00Z",
    "message": "…",
    "related_state": {}
  },
  "links": {
    "alert": "https://netspec.example/alerts",
    "device": "https://netspec.example/device/core-sw-01"
  }
}
```

`event` is `alert.firing`, `alert.acked`, or `alert.resolved` based on `alert.state`. `links` is omitted when `NETSPEC_PUBLIC_URL` is unset.

## OpenClaw side

1. Enable hooks with a dedicated token (`hooks.token`).
2. Prefer a **mapped** path such as `/hooks/netspec` with a transform/template that reads `event` / `alert.*` and produces a `wake` or `agent` action.
3. Keep the endpoint on loopback, tailnet, or a trusted reverse proxy.

Built-in `/hooks/wake` and `/hooks/agent` expect `{ "text": … }` / `{ "message": … }` — they will not accept this payload as-is without a mapping/transform.

## Verifying

```bash
curl -sS -X POST "$OPENCLAW_WEBHOOK_URL" \
  -H "Authorization: Bearer $OPENCLAW_HOOK_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"event":"alert.firing","alert":{"id":"test","device":"lab-sw","entity":"Gi1/0/1","alert_type":"interface_state_mismatch","severity":"warning","state":"firing","fired_at":"2026-08-10T20:00:00Z","message":"manual test","related_state":{}}}'
```

Then fire a real NetSpec alert (or resolve one) and check NetSpec logs for `openclaw webhook notify` / `openclaw notification sent` (`LOG_LEVEL=debug` helps).

The dashboard **Test alerts** button only exercises **Apprise** channels; OpenClaw channels are reported as skipped there.
