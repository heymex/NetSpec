# Changelog

All notable changes to this project are documented here. Release tags follow [Semantic Versioning](https://semver.org/).

## [1.0.0] — 2026-05-05

First stable release. NetSpec compares live network state to declarative YAML desired state, evaluates interfaces and telemetry, and delivers drift alerts through **Apprise-API** and `config/alerts.yaml`.

### Included

- SNMP validation (including port-channel and related policies), push telemetry ingest (Telegraf MDT + `mdt-translator`), declarative YAML (split devices, `alerts.yaml`), Docker Compose stack with Apprise-API, web UI (dashboard, wizard, API browser, notification test), `POST /api/reload`, and host deploy notes for Komodo/Portainer-style workflows.

[1.0.0]: https://github.com/heymex/NetSpec/releases/tag/v1.0.0
