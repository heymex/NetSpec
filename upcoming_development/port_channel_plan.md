# Port-Channel Roadmap

## Goals

- Treat logical port-channels and member interfaces as first-class parent/child objects.
- Improve operator visibility for member degradation vs full channel failure.
- Keep thresholds and severities predictable and configurable.

## Current Gaps

- Port-channel/member relation is inferred but not fully exposed in all UI/API surfaces.
- Severity policy is mostly hard-coded around member counts.
- Interface edit UI currently focuses on basic interface policy, not full channel policy management.

---

## Completed (initial PR — policy + evaluator foundation)

The following landed in the first port-channel PR (config types, `ValidateConfig`, evaluator, CI tag fix, targeted unit tests):

- **Config:** `member_policy.critical_threshold_pct` and `warning_threshold_pct` on YAML model.
- **Validation:** no self-reference in `members.required`, unique member names, empty entries rejected, `min_active.minimum` bounded by member count, threshold ranges and `warning < critical`.
- **Evaluator:** logical channel oper-down always emits **critical** (ignores `alerts.channel_down` override for this path); member degradation uses configurable critical threshold (default 50%); optional **warning threshold** suppresses alerts when down percentage is at or below it; `RelatedState` adds **`down_count`** and **`down_pct`** (alongside existing `total_members`, `active_members`, `down_members`).
- **Tests:** unit coverage for validation edge cases and evaluator threshold / channel-down behavior.

---

## Remaining Work (not in initial PR)

### 1) Data Model and Config — follow-ups

- Document or enforce how **`all_active`** and **`per_stack_minimum`** interact with percentage thresholds (if they should ignore pct fields, require both, etc.).
- Optional: validate that every `members.required` name exists as another interface key on the same device (stricter graph consistency).

### 2) Evaluator Semantics — follow-ups

- Revisit **warning band** wording vs implementation: today, absence of `warning_threshold_pct` means any member loss above zero can alert at **warning** until critical; with `warning_threshold_pct` set, losses at or below that percentage **suppress** the member-down alert entirely (not a separate “info” tier).
- Apply richer metadata consistently anywhere alerts are serialized (API/UI) if not already surfaced from `RelatedState`.
- **`alerts.member_down`:** initial PR removed unconditional override by config severity in favor of threshold logic; decide whether per-interface override should return as an explicit opt-out or severity cap.

### 3) Discovery and Onboarding

- Preserve channel members discovered via IF-MIB stack walks (ensure no regressions vs current discovery).
- Add **confidence flags** when stack data is incomplete.
- During wizard **commit**, allow operator to confirm/edit discovered members before YAML write.

### 4) UI/UX

- **Device page:** group channels and members visually; channel health badge and member summary.
- **Interface edit panel:** channel interfaces — member-policy controls (including new pct fields); member interfaces — show parent channel reference(s).

### 5) Testing and Docs

- **Unit tests:** discovery member-mapping behavior (stack walk → `members.required`).
- **Integration tests:** wizard channel commit + reload; alert behavior with simulated member outages end-to-end.
- **Docs:** operator-facing description of `member_policy` fields, defaults, and examples in README or dedicated doc.

---

## Proposed Work (reference — full scope)

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

1. ~~Policy and validation model update.~~ **Done** (initial PR; follow-ups listed above).
2. ~~Evaluator and alert payload enhancements.~~ **Mostly done** (initial PR; semantics/docs/`member_down` override TBD).
3. **Wizard + device page channel-policy editing.** — not started.
4. **Tests and docs.** — partial (unit only); discovery + integration + operator docs remain.
