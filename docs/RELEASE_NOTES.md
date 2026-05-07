# Release notes

The authoritative changelog is [**CHANGELOG.md**](../CHANGELOG.md) (semver, dates, migration pointers).

## v2.0.0-beta.2 — 2026-05-07

NetSpec **2.0.0-beta.2** is an operator-focused follow-up to beta.1. It improves dashboard signal quality and introduces a dedicated NOC wallboard view.

### Included in beta.2

- New **`/noc`** route with a high-density fleet matrix for quick triage (device, address, interfaces, alerts, worst severity, SNMP reachability).
- Dashboard ingest telemetry now shows a **10-minute sparkline trend** instead of a single Events/Sec point.
- NOC wallboard tuning: no SNMP active banner noise on `/noc`, with host-overview-first side-pane ordering.
- Added dedicated operator documentation for port-channel management and evaluator behavior: **[PORT_CHANNEL_EVALUATOR_GUIDE.md](PORT_CHANNEL_EVALUATOR_GUIDE.md)**.

### Upgrade path (from v2.0.0-beta.1)

1. Pull beta.2 images.
2. `docker compose up -d` with your existing compose/env.
3. Refresh the UI and verify `/noc` and dashboard ingest trend rendering.

### Image tags

- `ghcr.io/heymex/netspec:v2.0.0-beta.2`
- `ghcr.io/heymex/netspec-mdt-translator:v2.0.0-beta.2`

---

## v2.0.0-beta.1 — 2026-05-06

NetSpec **2.0.0-beta.1** is the first tagged pre-release after **v1.0.0**. It is intended for **staging** upgrades of the Compose stack, translator, and web UI—not for tagging production without your own soak testing.

### Why 2.x

The default **`docker-compose.yml` networking model changed** from host networking on several services to a **Compose bridge**. That affects **every** `.env` that hard-coded **`127.0.0.1`** for Apprise or the translator ingest target.

### Upgrade path (existing v1.0.0 deploys)

1. Read **[MIGRATION_BRIDGE_AND_AUTH.md](MIGRATION_BRIDGE_AND_AUTH.md)** end-to-end.
2. Align **`APPRISE_API_URL`** and **`NETSPEC_INGEST_HOST`** (and **`NETSPEC_INGEST_PORT`** with **`global.ingest.port`** in **`desired-state.yaml`**).
3. Optionally enable password session auth and API bearer token (**`.env.example`** documents the vars).
4. Pull images pinned to **`v2.0.0-beta.1`** (or `latest` tracking `main` after merge) and **`docker compose up -d`** with the refreshed compose file.

### Image tags

- `ghcr.io/heymex/netspec:v2.0.0-beta.1`
- `ghcr.io/heymex/netspec-mdt-translator:v2.0.0-beta.1`

### Feedback

Issues and PRs welcome on [**github.com/heymex/NetSpec**](https://github.com/heymex/NetSpec).
