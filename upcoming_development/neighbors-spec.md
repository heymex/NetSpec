# LLDP/CDP neighbor integration — specification

Source: [neighbors.md](./neighbors.md). This document scopes delivery into phases; **PR #31 (feat/lldp-cdp-neighbor-discovery)** implements **Phase 1**.

## Goals

1. Use **rules.yaml** to classify endpoints seen via LLDP/CDP and flag likely mislabeled IOS descriptions.
2. **SNMP discovery** walks neighbors with interfaces and surfaces them in the wizard.
3. **Topology export** (Graphviz DOT) from a single-device walk for reporting and future symmetric link mapping.
4. *(Future)* Streaming telemetry for neighbor-change alerts.
5. *(Future)* Multi-device graph, SPoF / redundancy risk analysis.

## Phase 1 — SNMP discovery + neighbor rules + DOT (this PR)

### SNMP

| Protocol | MIB area | OIDs walked |
|----------|----------|-------------|
| LLDP | IEEE 802.1AB `lldpRemTable` | `lldpRemLocalPortNum`, `lldpRemSysName`, `lldpRemSysDesc`, `lldpRemPortId`, `lldpRemPortDesc` |
| CDP | Cisco `cdpCacheEntry` | `cdpCacheDeviceId`, `cdpCacheDevicePort`, `cdpCachePlatform` (best-effort; Cisco only) |

Neighbors attach to interfaces by **local ifIndex** (`lldpRemLocalPortNum` / `cdpCacheIfIndex`). Walk is best-effort: failure to read LLDP/CDP does not fail the interface walk.

### Data model (`discovery.WalkResult`)

- `interfaces[].neighbors[]` — remote sysName, sysDesc, portId, platform, protocol.
- `interfaces[].neighbor_rule_label` — matched `neighbor_rules` label from rules.yaml.
- `interfaces[].neighbor_hint` — human hint when LLDP class and port alias disagree.
- `topology_edges[]` — directed edges for graph export.
- `topology_dot` — Graphviz DOT string for the local device + discovered neighbors.

### rules.yaml — `neighbor_rules` (per device role)

```yaml
neighbor_rules:
  # Order matters: put vendor-specific rules (e.g. Crestron AV) before generic LLDP capability rules.
  - label: AV equipment (Crestron & peers)
    match_sys_name: "*AM3100*"
    expect_alias_glob: "av*"
  - label: IP Phone (LLDP)
    match_lldp_capability: telephone
    expect_alias_glob: "phone*"
  - label: Wireless AP (LLDP capability)
    match_lldp_capability: wlan_ap
    expect_alias_glob: "ap*"
```

Fields:

| Field | Purpose |
|-------|---------|
| `label` | Wizard grouping / display |
| `match_lldp_capability` | LLDP only: enabled capability bitmask (e.g. `telephone`, `wlan_ap`, `bridge`); see IEEE 802.1AB / `lldpRemSysCapEnabled` |
| `match_sys_name` | Glob on LLDP/CDP remote system name |
| `match_sys_desc` | Glob on remote system description |
| `match_platform` | Glob on CDP platform string |
| `expect_alias_glob` | If neighbor matches but interface alias does not, set `neighbor_hint` |

**Note:** On a single rule, `match_*` / `match_lldp_capability` conditions are **AND**ed. Use multiple list entries (often sharing the same `label`) for **OR** behavior (e.g. several Crestron / AirMedia patterns before a generic `telephone` rule).

Evaluation: first matching rule per neighbor entry; port-level label is the first neighbor on that interface that matches.

Sorting: wizard groups by `rule_name` first, then `neighbor_rule_label` when `rule_name` is empty.

### API

No new routes. `POST /api/discovery/walk` response gains neighbor and topology fields.

### Out of scope (Phase 1)

- VLAN / trunk-mode SNMP validation (#1 examples).
- Push telemetry LLDP events (#2).
- Fleet-wide graph DB, SPoF engine (#4 beyond single-device DOT).

## Phase 2 — Streaming telemetry (planned)

- Subscribe to LLDP neighbor add/remove in MDT/gNMI when available.
- Alert on new AP/phone on access ports (compare to `neighbor_rules`).

## Phase 3 — Fleet topology (planned)

- Persist edges from committed devices; merge symmetric links.
- Graphviz / web view; flag single-homed distribution↔core paths.

## Phase 4 — Compliance checks (planned)

- SNMP read of access VLAN / switchport mode vs `neighbor_rules` expectations.
- Actionable alerts in the main alerter pipeline.
