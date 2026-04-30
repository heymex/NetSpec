# NetSpec Discovery: Current State and Next Steps

This document tracks the actual behavior of the discovery wizard and related APIs as implemented.

## Current State

- Wizard UI is served at `/wizard`.
- Discovery APIs:
  - `POST /api/discovery/probe`
  - `POST /api/discovery/walk`
  - `POST /api/discovery/commit`
- Probe and walk use SNMP (`gosnmp`) and include vendor hints and existing-device detection.
- Walk includes port-channel detection and membership inference from IF-MIB stack data.
- Commit writes device changes to split files under `config/devices/*.yaml` (monolithic fallback/migration supported).
- Device page supports inline interface policy edits through:
  - `PATCH /api/devices/{device}/interfaces/{iface}`
- Unknown telemetry source handling supports address hints and reconciliation when a source becomes known.

## Design Notes

- `desired-state.yaml` should hold globals and optional legacy monolithic devices.
- `config/devices/*.yaml` is the preferred structure for per-device changes.
- Interface-level fields supported by GUI and API:
  - `monitor`
  - `desired_state`
  - `admin_state`
  - `alerts.state_mismatch`

## Validation Checklist

- Add a new device via wizard and verify a new file appears in `config/devices/`.
- Patch an existing device and verify only that device file changes.
- Confirm unknown telemetry entries clear after device/address match.
- Verify port-channel interfaces appear in walk results.
- Verify inline interface edits persist and survive reload.

## Backlog

- Improve inline edit UX from prompt-based fallbacks to richer row forms (description/member policy).
- Add unit tests for split-file patching and interface PATCH endpoint.
- Add explicit API docs for discovery and interface edit endpoints.