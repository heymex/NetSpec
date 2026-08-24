# Port-Channel Evaluator Guide

This guide documents how NetSpec currently models port-channels, validates their configuration, and emits alerts from evaluator logic.

Use this alongside `config/desired-state.yaml` (or split device files under `config/devices/*.yaml`).

> Current-state note: `member_policy.mode` and count fields (`minimum`, `per_stack_minimum`) are validated at load time, but member alert severity in the current evaluator is driven by member down-percentage thresholds (`warning_threshold_pct`, `critical_threshold_pct`) plus logical channel oper status.

## 1) Port-channel model in desired state

Port-channel policy is defined per logical interface in each device's `interfaces` map:

- `members.required`: required member interfaces for the logical channel.
- `member_policy`: policy metadata validated by config loader.
- `alerts`: per-interface severity overrides for evaluator alert types.

Example:

```yaml
devices:
  dsw-core-01:
    address: 10.0.10.10
    interfaces:
      Port-channel10:
        description: Uplink LAG to distribution stack
        desired_state: up
        admin_state: enabled
        monitor: true
        members:
          required:
            - TenGigabitEthernet1/0/49
            - TenGigabitEthernet1/0/50
            - TenGigabitEthernet2/0/49
            - TenGigabitEthernet2/0/50
        member_policy:
          mode: min_active
          minimum: 2
          warning_threshold_pct: 20
          critical_threshold_pct: 50
        alerts:
          state_mismatch: warning
          member_down: warning
          channel_down: critical
```

## 2) Config validation rules

Validation is performed in `internal/config/loader.go`.

When `members.required` is present, NetSpec enforces:

- `member_policy` must exist.
- `members.required` entries must be non-empty.
- no self-reference (channel name cannot list itself as member).
- no duplicate member entries.
- `member_policy.mode` must be one of:
  - `all_active`
  - `min_active`
  - `per_stack_minimum`
- for `min_active`:
  - `minimum > 0`
  - `minimum <= len(members.required)`
- optional thresholds:
  - `critical_threshold_pct` must be `> 0` and `<= 100`
  - `warning_threshold_pct` must be `> 0` and `< 100`
  - when both are set: `warning_threshold_pct < critical_threshold_pct`

## 3) Evaluator behavior and alert types

Port-channel logic is evaluated in `internal/evaluator/evaluator.go`.

NetSpec can emit three relevant alert types:

- `interface_state_mismatch`
- `port_channel_member_down`
- `port_channel_down`

### 3.1 Logical channel down (`port_channel_down`)

If the logical channel operational state is `down`, evaluator emits:

- alert type: `port_channel_down`
- severity: `critical` (always)
- message: `port-channel <name> is down`

Important: current evaluator forces this to `critical` even if `alerts.channel_down` is set differently.

When the logical channel operational state is `up`, evaluator emits a **resolve** for `port_channel_down` (no-op if none is active).

### 3.2 Member degradation (`port_channel_member_down`)

When one or more members are down while channel oper state is not down:

- each required member is classified as **up**, **down**, or **unknown**
  - missing cache entries and non-`up`/`down` oper values (including SNMP `unknown`) are **unknown**
  - unknown members are **never** counted as down
  - member-policy evaluation is **deferred** until every required member has known oper state (avoids cold-start / partial-hydration false positives)
- when all members are known, evaluator calculates:
  - `total_members`
  - `active_members`
  - `down_members`
  - `down_count`
  - `down_pct`
- default critical threshold is `50%` down members.
- if `member_policy.critical_threshold_pct` is set, it overrides that default.
- if `member_policy.warning_threshold_pct` is set, alerts are suppressed when `down_pct <= warning_threshold_pct`.
- severity is:
  - `critical` when `down_pct >= critical_threshold_pct`
  - otherwise `warning`
- when the member policy is healthy again (all known members up, or `down_pct` at/below the warning threshold), evaluator emits an explicit **resolve** event so sticky `port_channel_member_down` alerts clear (including alerts restored from `alert-state.json` after restart).

### 3.3 Interface mismatch (`interface_state_mismatch`)

This is the standard per-interface mismatch alert (`expected up/down` vs actual). For channels, it can coexist with member-related alerts.

## 4) Severity override behavior

Per-interface alert severity fields live under:

```yaml
alerts:
  state_mismatch: info|warning|critical
  member_down: info|warning|critical
  channel_down: info|warning|critical
  admin_down: info|warning|critical
```

Current evaluator behavior:

- `state_mismatch` override is honored.
- `member_down` and `channel_down` values are not used to override the computed/fixed severity in current channel-member code paths.
  - `channel_down` is always emitted as `critical`.
  - `member_down` severity is derived from threshold logic.

## 5) How member alerts are triggered

Evaluator updates can come from SNMP and telemetry ingest paths.

For each updated interface snapshot, evaluator checks:

1. Is this interface itself a channel with `members.required`?
2. Is this interface listed as a member in any configured channel?

If either is true, channel member policy is reevaluated for affected channel(s).

This means a single member flap can recalculate alert state for one or more channels referencing it.

## 6) Recommended operator patterns

- Use `mode`/`minimum` as declarative intent and validation guardrails today; tune live alert behavior primarily with `warning_threshold_pct` and `critical_threshold_pct`.
- For strict bundles, use low warning threshold (or none) and a lower `critical_threshold_pct` to escalate early.
- For resilient bundles (N+1/N+2), use higher warning/critical thresholds aligned to your tolerated member loss.
- Keep member names exactly aligned with interface names NetSpec sees from SNMP/telemetry normalization.
- Start with warning-level routing for `port_channel_member_down` in `config/alerts.yaml` channels, then tighten after baseline.

## 7) Quick checklist

- [ ] Channel interface has `desired_state`, `monitor`, `members.required`, and `member_policy`.
- [ ] Member list has no duplicates and no self-reference.
- [ ] `minimum` and threshold percentages pass loader constraints.
- [ ] Alert routing (`config/alerts.yaml`) includes the severities you expect to receive.
- [ ] Device/member names match normalized interface naming in runtime.

