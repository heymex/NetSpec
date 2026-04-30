# Port-Channel Roadmap

## Goals

- Treat logical port-channels and member interfaces as first-class parent/child objects.
- Improve operator visibility for member degradation vs full channel failure.
- Keep thresholds and severities predictable and configurable.

## Current Gaps

- Port-channel/member relation is inferred but not fully exposed in all UI/API surfaces.
- Severity policy is mostly hard-coded around member counts.
- Interface edit UI currently focuses on basic interface policy, not full channel policy management.

## Proposed Work

### 1) Data Model and Config

- Keep `members.required` on channel interfaces as the parent-child definition.
- Expand `member_policy` support:
  - `mode: min_active`
  - `minimum` (absolute)
  - optional `critical_threshold_pct` and `warning_threshold_pct` for percentage-based policies.
- Add validation:
  - channel must not reference itself as a member
  - member names must be unique
  - policy values must be in valid ranges.

### 2) Evaluator Semantics

- Channel oper down:
  - always `critical`.
- Member degradation:
  - `critical` when down-members percentage >= critical threshold (default 50%).
  - `warning` when down-members percentage > 0 and below critical threshold.
- Include richer alert metadata:
  - `total_members`, `active_members`, `down_members`, `down_pct`.

### 3) Discovery and Onboarding

- Preserve channel members discovered via IF-MIB stack walks.
- Add confidence flags when stack data is incomplete.
- During commit, allow operator to confirm/edit discovered members before write.

### 4) UI/UX

- Device page:
  - group channels and members visually.
  - show channel health badge and member summary.
- Interface edit panel:
  - for channel interfaces expose member-policy controls.
  - for member interfaces show parent channel references.

### 5) Testing

- Unit tests:
  - evaluator thresholds and severity transitions.
  - config validation edge cases.
  - discovery member-mapping behavior.
- Integration tests:
  - wizard channel commit + reload.
  - alert behavior with simulated member outages.

## Milestones

1. Policy and validation model update.
2. Evaluator and alert payload enhancements.
3. Wizard + device page channel-policy editing.
4. Tests and docs.
