# Splunk and Elasticsearch alerting (NetSpec)

NetSpec can ship the same structured alert JSON used by webhooks into **Splunk HEC** and **Elasticsearch**. Configure a channel of `type: splunk` or `type: elasticsearch` (alias `elastic`) in `config/alerts.yaml`, then route it from `alert_rules` like any other channel.

## Configuration

| Piece | Where |
|--------|--------|
| Splunk channel | `channels.*.type: splunk` |
| Elasticsearch channel | `channels.*.type: elasticsearch` (or `elastic`) |
| URL | `url_env` (full HEC or ES URL). Or `endpoints` list — do not also set top-level `url_env` |
| Token | `token_env` (required for Splunk; optional for Elasticsearch) |
| Index | `index` and/or `index_env` (optional Splunk HEC index; ES index when URL is cluster base only) |
| Splunk fields | `sourcetype` (default `netspec:alert`), `source` (default `netspec`) |
| ES auth | `api_key` (default when `token_env` is set), `bearer`, or `none` |
| UI links | **`NETSPEC_PUBLIC_URL`** — when set, nested `links.alert` / `links.device` are included |
| Routing | Add the channel name under `alert_rules` |

Example `alerts.yaml` fragment:

```yaml
channels:
  ops-splunk:
    type: splunk
    url_env: SPLUNK_HEC_URL
    token_env: SPLUNK_HEC_TOKEN
    index: netops
    sourcetype: netspec:alert
    source: netspec
    severity_filter: [warning, critical]

  ops-elastic:
    type: elasticsearch
    url_env: ELASTIC_URL
    token_env: ELASTIC_API_KEY
    index: netspec-alerts
    severity_filter: [warning, critical]

alert_rules:
  critical:
    channels: [ops-slack, ops-splunk, ops-elastic]
  warning:
    channels: [ops-splunk, ops-elastic]
```

### Splunk HEC

- `url_env` should be the HEC collector URL, for example `https://splunk.example:8088/services/collector/event`.
- NetSpec always sends `Authorization: Splunk <token>` from `token_env`.
- Do not set `auth` or `header_name` on Splunk channels.

HEC envelope (inner `event` matches the [webhook payload](WEBHOOK_ALERTING.md)):

```json
{
  "time": 1723312800.0,
  "host": "core-sw-01",
  "source": "netspec",
  "sourcetype": "netspec:alert",
  "index": "netops",
  "event": {
    "event": "alert.firing",
    "alert": { "...": "..." },
    "links": { "...": "..." }
  }
}
```

`time` is the alert `fired_at` as a Unix timestamp (fractional seconds). `host` is the alert device name.

### Elasticsearch

- Point `url_env` at a **full document URL** (`https://es.example:9200/netspec-alerts/_doc`) **or** at the cluster base (`https://es.example:9200`) and set `index` / `index_env` so NetSpec appends `/{index}/_doc`.
- Default auth with a token is `Authorization: ApiKey <token>`. Use `auth: bearer` for Elastic Cloud-style bearer tokens. Omit `token_env` (or set `auth: none`) when the cluster is already network-isolated.

Indexed document:

```json
{
  "@timestamp": "2026-08-10T20:00:00Z",
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

`@timestamp` is `fired_at` (RFC3339Nano). `event` is `alert.firing`, `alert.acked`, or `alert.resolved`.

## Verifying

Splunk (replace URL/token):

```bash
curl -sS -k -X POST "$SPLUNK_HEC_URL" \
  -H "Authorization: Splunk $SPLUNK_HEC_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"event":{"event":"alert.firing","alert":{"id":"test","device":"lab-sw","entity":"Gi1/0/1","alert_type":"interface_state_mismatch","severity":"warning","state":"firing","fired_at":"2026-08-10T20:00:00Z","message":"manual test","related_state":{}}},"sourcetype":"netspec:alert","source":"netspec"}'
```

Elasticsearch:

```bash
curl -sS -X POST "${ELASTIC_URL%/}/netspec-alerts/_doc" \
  -H "Authorization: ApiKey $ELASTIC_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"@timestamp":"2026-08-10T20:00:00Z","event":"alert.firing","alert":{"id":"test","device":"lab-sw","entity":"Gi1/0/1","alert_type":"interface_state_mismatch","severity":"warning","state":"firing","fired_at":"2026-08-10T20:00:00Z","message":"manual test","related_state":{}}}'
```

Then fire or resolve a real NetSpec alert and check logs for `splunk notification sent` / `elasticsearch notification sent` (`LOG_LEVEL=debug` helps).

The dashboard **Test alerts** button only exercises **Apprise** channels. Splunk and Elasticsearch channels are reported as skipped there.

If you only need a raw JSON POST without HEC/`@timestamp` wrapping, use a generic [`type: webhook`](WEBHOOK_ALERTING.md) channel instead.
