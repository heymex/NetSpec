# Release notes

The authoritative changelog is [**CHANGELOG.md**](../CHANGELOG.md) (semver, dates, migration pointers).

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
