# Changelog

All notable changes to this project are documented here. Release tags follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Features

- **NetSpecGraph (scaffolding):** VictoriaMetrics compose service + Telegraf Influx output for interface metrics; opt-in `netspec-graph` profile and `cmd/netspecgraph` skeleton. See `docs/NETSPECGRAPH.md`.
- **NetSpecGraph metric contract:** Telegraf starlark maps IETF interface counters to `if_*` names with `device`/`interface` labels (translator path unchanged).
- **NetSpecGraph per-interface UI:** uPlot traffic / errors / oper-status page querying contracted VictoriaMetrics series (`/device/{device}/interface/{iface}`).
- Added **OpenClaw webhook** alert channel (`type: openclaw`) that POSTs structured JSON (`event`, `alert`, optional `links`) to an OpenClaw Gateway hook URL. See `docs/OPENCLAW_ALERTING.md`.

### Bug Fixes

- Port-channel member evaluation no longer treats missing/unknown member cache entries as down; evaluation waits until every required member has known oper state, and healthy evaluations emit resolve events so sticky `port_channel_member_down` / `port_channel_down` alerts clear (including after restart).

## [2.0.0] — 2026-05-07

Stable release of the 2.x line, promoting the beta series with bridge-first deployment, hardened ingest, improved operations UX, and expanded operator documentation. Images publish as **`ghcr.io/heymex/netspec:v2.0.0`** and **`ghcr.io/heymex/netspec-mdt-translator:v2.0.0`**.

### Included in 2.0.0

- Bridge-first Docker Compose deployment model with updated migration/runbook guidance.
- Optional web/API authentication (`NETSPEC_ADMIN_PASSWORD_HASH`, `NETSPEC_SESSION_SECRET`, optional `NETSPEC_API_TOKEN`).
- Multi-port telemetry ingest with per-listener source tagging.
- Discovery wizard re-walk/sync behavior for existing devices.
- Dashboard ingest-rate sparkline (last 10m) and dedicated `/noc` high-density operations view.
- Sticky source-aware device navigation (`/` vs `/noc`) for drill-down/back flow.
- Dedicated operator guide for port-channel policy and evaluator behavior: `docs/PORT_CHANNEL_EVALUATOR_GUIDE.md`.

## [2.0.0-beta.3] — 2026-05-07

Pre-release follow-up focused on navigation continuity and operator documentation depth. Images publish as **`ghcr.io/heymex/netspec:v2.0.0-beta.3`** and **`ghcr.io/heymex/netspec-mdt-translator:v2.0.0-beta.3`** on tag CI.

### Features

- Added **sticky return context** for device drill-downs, so Back on `/device/{name}` returns to the originating view (`/noc` vs `/`).
- Added dedicated **port-channel evaluator operations guide**: `docs/PORT_CHANNEL_EVALUATOR_GUIDE.md`.

### Improvements

- Updated release-facing docs to include the new guide and beta.3 tag references.

## [2.0.0-beta.2] — 2026-05-07

Pre-release follow-up focused on operator UX and dashboard signal quality. Images publish as **`ghcr.io/heymex/netspec:v2.0.0-beta.2`** and **`ghcr.io/heymex/netspec-mdt-translator:v2.0.0-beta.2`** on tag CI.

### Features

- **Dashboard telemetry ingest trendline** replaces single-point Events/Sec with a **10-minute ingest-rate sparkline** (with listener-aware stats for multi-port ingest).
- New **NOC view** at **`/noc`** with high-density fleet matrix (device/address/interfaces/alerts/worst severity/SNMP reachability), summary counters, and host overview for faster triage.

### Improvements

- NOC layout tuned for wallboard operation: **SNMP active banner suppressed on `/noc`** and side-pane order optimized for host-overview-first scanning.
- User-facing docs updated for beta.2 behavior, API descriptions, and image tag examples.

## [2.0.0-beta.1] — 2026-05-06

Pre-release (**2.x API / deployment shape**). Use for testing before `v2.0.0`; images publish as **`ghcr.io/heymex/netspec:v2.0.0-beta.1`** and **`ghcr.io/heymex/netspec-mdt-translator:v2.0.0-beta.1`** on tag CI.

See also: [Migration: bridge networking and authentication](docs/MIGRATION_BRIDGE_AND_AUTH.md) and [release notes](docs/RELEASE_NOTES.md).

### Breaking changes (upgrade from v1.0.0)

- **Docker Compose networking** defaults to the **`netspec` bridge**. NetSpec (Apprise URLs, ingest targets, translator `NETSPEC_INGEST_HOST`) now uses Docker DNS (**`netspec-apprise:8000`**, **`netspec-netspec`**) instead of `network_mode: host` + **`127.0.0.1`** loopback. Existing `.env` / compose trees need the updates in **`docs/MIGRATION_BRIDGE_AND_AUTH.md`**.
- **Ingest ports**: the sample stack and validator treat **`global.ingest.port`** and **`NETSPEC_INGEST_PORT`** as the same value (typically **57500** on bridge—Telegraf and NetSpec bind **57500 in different containers**). The old **`setup-netspec.sh`** behavior that rewrote **57501** for NetSpec (host-port collision) is removed; `./scripts/validate-netspec-stack.sh` no longer forbids ingest **57500**.

### Features

- **Optional web/API authentication** (`NETSPEC_ADMIN_PASSWORD_HASH`, `NETSPEC_SESSION_SECRET`, optional **`NETSPEC_API_TOKEN`** bearer access). Disabled when unset (`internal/auth`).
- **Multi-port push ingest** with **`global.ingest.source`** / **`additional_listeners`** mapping TCP ports to `PushTelemetryEvent.source` (“sourcetype” style).
- **Discovery wizard re-walk** for existing devices: **`sync_discovered_interfaces`** on patch removes unchecked interfaces seen on SNMP walk from desired state; walk returns **`existing_config`** for YAML prefill. Device page links to **`/wizard?device_key=…&address=…`**.
- **Telemetry/UI**: SNMP-confirmed push events advance **both** `LastSNMPValidation` and **`LastTelemetryValidation`** (`push_snmp` validation source).

### Fixes / improvements

- **Docs / scripts**: README, **`docs/DEV_HOST_RUNBOOK.md`**, **`.env.example`**, **`setup-netspec.sh`**, and **`validate-netspec-stack.sh`** aligned with **bridge** ingest defaults (no forced **57501** rewrite; validator no longer rejects **57500** for `global.ingest.port` when it matches **`.env`**).
- **Canonical interface/Twe** telemetry name matching refinement.
- **CI**: always publish Docker images on **tag** pushes (`v*`), including docs-only tags.
- **Ops**: **`scripts/validate-netspec-stack.sh`** preflight, telemetry **coverage diagnostics** API/page, **`setup-netspec.sh`** / README cleanup, **`mdt-sidecar`** ownership warnings, MDT grpc-tcp vs grpc-tls troubleshooting note.

[2.0.0]: https://github.com/heymex/NetSpec/releases/tag/v2.0.0
[2.0.0-beta.3]: https://github.com/heymex/NetSpec/releases/tag/v2.0.0-beta.3
[2.0.0-beta.2]: https://github.com/heymex/NetSpec/releases/tag/v2.0.0-beta.2
[2.0.0-beta.1]: https://github.com/heymex/NetSpec/releases/tag/v2.0.0-beta.1

---

## [1.0.0] — 2026-05-05

First stable release. NetSpec compares live network state to declarative YAML desired state, evaluates interfaces and telemetry, and delivers drift alerts through **Apprise-API** and `config/alerts.yaml`.

### Included

- SNMP validation (including port-channel and related policies), push telemetry ingest (Telegraf MDT + `mdt-translator`), declarative YAML (split devices, `alerts.yaml`), Docker Compose stack with Apprise-API, web UI (dashboard, wizard, API browser, notification test), `POST /api/reload`, and host deploy notes for Komodo/Portainer-style workflows.

[1.0.0]: https://github.com/heymex/NetSpec/releases/tag/v1.0.0
