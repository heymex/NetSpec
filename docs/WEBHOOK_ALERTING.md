# Webhook alerting (NetSpec)

NetSpec can POST the same structured alert JSON to one or more HTTP endpoints. Use a channel of `type: webhook` when a provider accepts a JSON body (OpenClaw mapped hooks, an internal receiver, PagerDuty-style routing headers, and similar).

Each endpoint has its own URL and optional token. Endpoints on one channel share that channel's `severity_filter` and receive every alert routed to the channel. Use separate channels when providers should see different severities.

## Configuration

| Piece | Where |
|--------|--------|
| Channel | `config/alerts.yaml` → `channels.*.type: webhook` |
| One endpoint | `url_env` (full URL). Optional `token_env`, `auth`, `header_name` |
| Several endpoints | `endpoints` list. Do not also set top-level `url_env` |
| Auth | `bearer` (default when `token_env` is set), `header`, `bearer_and_header`, or `none` |
| UI links | **`NETSPEC_PUBLIC_URL`** (optional) — when set, payload includes `links.alert` and `links.device` |
| Routing | Add the channel name under `alert_rules` (same as Apprise) |

`type: openclaw` is still accepted as a single-endpoint alias. With `token_env` set and no explicit `auth`, the token is sent as `Authorization: Bearer` and `x-openclaw-token`.

Example `alerts.yaml` fragment:

```yaml
channels:
  ops-webhooks:
    type: webhook
    severity_filter: [warning, critical]
    endpoints:
      - name: openclaw
        url_env: OPENCLAW_WEBHOOK_URL
        token_env: OPENCLAW_HOOK_TOKEN
        auth: bearer_and_header
        header_name: x-openclaw-token
      - name: internal
        url_env: INTERNAL_WEBHOOK_URL

  ops-pager:
    type: webhook
    url_env: PAGER_WEBHOOK_URL
    token_env: PAGER_ROUTING_KEY
    auth: header
    header_name: X-Routing-Key
    severity_filter: [critical]

alert_rules:
  critical:
    channels: [ops-slack, ops-webhooks, ops-pager]
  warning:
    channels: [ops-webhooks]
```

A single endpoint can omit the list:

```yaml
channels:
  ops-hook:
    type: webhook
    url_env: GENERIC_WEBHOOK_URL
    token_env: GENERIC_WEBHOOK_TOKEN
```

That sends `Authorization: Bearer <token>`. Leave `token_env` unset to POST with no auth header.

## Payload shape

Every endpoint receives the same body. Firing example:

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

One failing endpoint does not cancel the others. NetSpec logs `webhook notification sent` per endpoint and returns an error that names the endpoint that failed.

## OpenClaw

Point `url_env` at a mapped hook (`hooks.enabled` + shared token), for example `http://openclaw:18789/hooks/netspec`, and use `auth: bearer_and_header` with `header_name: x-openclaw-token`. A transform on the OpenClaw side should read `event` / `alert.*` and produce a `wake` or `agent` action.

Built-in `/hooks/wake` and `/hooks/agent` expect `{ "text": … }` / `{ "message": … }` — they will not accept this payload as-is without a mapping/transform. Keep the endpoint on loopback, a tailnet, or a trusted reverse proxy.

## Verifying

```bash
curl -sS -X POST "$OPENCLAW_WEBHOOK_URL" \
  -H "Authorization: Bearer $OPENCLAW_HOOK_TOKEN" \
  -H "x-openclaw-token: $OPENCLAW_HOOK_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"event":"alert.firing","alert":{"id":"test","device":"lab-sw","entity":"Gi1/0/1","alert_type":"interface_state_mismatch","severity":"warning","state":"firing","fired_at":"2026-08-10T20:00:00Z","message":"manual test","related_state":{}}}'
```

Then fire a real NetSpec alert (or resolve one) and check NetSpec logs for `webhook notify` / `webhook notification sent` (`LOG_LEVEL=debug` helps).

The dashboard **Test alerts** button only exercises **Apprise** channels. Webhook channels are reported as skipped there.
