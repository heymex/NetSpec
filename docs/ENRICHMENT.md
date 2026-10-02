# Alert Enrichment

NetSpec can attach contextual data from external sources (NetBox, Taillight, Elastic) to alerts before they are delivered. Enrichment is optional: when disabled or when a source fails, the alert still fires normally.

## Package boundary

`internal/enrichment` is a **leaf** package. It must not import `internal/config` or `internal/types`. Callers pass a minimal `EnrichContext` (`DeviceName`, `AlertID`) and receive an `EnrichedContext` that is stored on `types.Alert.Enriched`.

The config layer embeds `*enrichment.Config` directly — there is no parallel DTO or conversion step.

## Configuration

Enrichment is configured under a top-level `enrichment:` block in `desired-state.yaml` (same file as `global:` / `devices:`). It is **not** loaded from `alerts.yaml`.

```yaml
enrichment:
  enabled: true
  timeout: 15s          # overall pipeline timeout per alert
  max_concurrent: 3     # reserved for future parallel limits

  netbox:
    enabled: true
    api_url: https://netbox.example.com
    api_token_env: NETBOX_API_TOKEN   # prefer env over inline token
    # api_token: "..."                # inline (tests / lab only)
    tags: []                          # optional: only enrich devices with these tags

  taillight:
    enabled: true
    backend: api                      # "api" or "elastic"
    base_url: https://taillight.example.com
    api_key_env: TAILLIGHT_API_KEY
    # When backend: elastic:
    # elastic_url: https://elastic.example.com
    # elastic_api_key_env: ELASTIC_API_KEY
    # syslog_index: "logs-netlog*"
    # summary_index: "taillight-report-*"
    syslog_window:
      enabled: true
      lookback_min: 30
      severity_filter: [error, warning]
      max_entries: 20
      event_kind: netlog              # "netlog" or "srvlog"
    daily_summary:
      enabled: true
      hostname_exact: true
      event_kind: netlog

  elastic:
    enabled: false
    url: https://elastic.example.com
    api_key_env: ELASTIC_API_KEY
    trend:
      enabled: true
      window: 1h
      index_pattern: "logs-*"
      max_hits: 100
```

### Token resolution

Prefer `*_env` fields. At runtime the enricher resolves:

| Field | Fallback |
|-------|----------|
| `api_token` | `os.Getenv(api_token_env)` |
| `api_key` | `os.Getenv(api_key_env)` |
| `elastic_api_key` | `os.Getenv(elastic_api_key_env)` |

Inline tokens are accepted for tests and labs but should not be committed.

## Runtime behavior

1. On alert fire/resolve (and flap/manual close paths), the alerter calls the enrichment pipeline.
2. Enabled enrichers run in parallel under the configured timeout.
3. Partial results are kept: a NetBox success + Taillight timeout still attaches NetBox data.
4. Failures are logged at debug/warn; they never suppress the alert.
5. Apprise notification bodies append compact NetBox / Taillight / Elastic sections when `Enriched.HasAny()` is true.

## Defaults

When the `enrichment:` block is omitted:

- `enabled` defaults to `false` (pipeline is not created)
- `timeout` defaults to `15s`
- `max_concurrent` defaults to `3`
