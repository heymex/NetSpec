# AGENTS.md – Topology Awareness & Dependency Mapping

## Overview

**Feature Name:** Topology Awareness & Dependency Mapping  
**Purpose:** Introduce awareness of device and interface dependencies to improve alert correlation, reduce noise, and enable root cause identification.

---

## Architecture

```
[Telemetry/SNMP]
        ↓
[State Evaluator]
        ↓
[Dependency Evaluator]
        ↓
[Alert Engine]
```

---

## Core Concepts

### Dependency Model
A dependency represents:

> "This interface/device depends on another for upstream connectivity"

### Types
- Physical Uplink
- Port-channel membership
- Logical dependencies (AP → switch port)

---

## Topology Graph Model

```go
type NodeID string

type DependencyGraph struct {
    Upstream   map[NodeID][]NodeID
    Downstream map[NodeID][]NodeID
}
```

---

## Configuration

### Example YAML

```yaml
interfaces:
  GigabitEthernet1/0/48:
    depends_on:
      device: dist-sw-01
      interface: Port-channel10
```

---

## Evaluation Logic

### Root Cause Rule
- If upstream is DOWN → downstream alerts suppressed

### Example
| Condition | Output |
|----------|--------|
| Access port down | Alert normally |
| Uplink down | Suppress downstream alerts |

---

## API Endpoints

### Get Full Topology
```
GET /api/topology
```

### Per Device
```
GET /api/topology/{device}
```

---

## UI Changes

- Show upstream/downstream relationships
- Add root cause indicator in alerts

---

## Testing

### Unit
- Graph traversal
- Cycle detection

### Integration
- Multi-tier outage simulation

---

## Implementation Phases

### Phase 1
- Graph model
- YAML parsing
- Basic suppression

### Phase 2
- Severity adjustment
- UI support

### Phase 3
- Auto discovery
- Visualization

---

## Constraints

- Must not increase alert latency
- Must remain deterministic
- Must not overcomplicate config

---

## Success Metrics

- Reduction in alert volume
- Time to root cause
- Operator feedback

---

## Code Review Notes

*Added after review against current codebase (main + PR #25 — alert state persistence).*

### Overlap with PR #25

PR #25 adds a `suppressedUntil map[string]time.Time` check inside `process()` in `internal/alerter/engine.go`. The topology suppression ("upstream is down → suppress downstream") would insert a logically distinct but mechanically identical early-exit guard in the same function. The two can coexist cleanly, but `process()` is accumulating suppression logic fast enough that a `shouldSuppress()` helper method should be extracted before this work starts.

Planned suppression check order after both features land:

1. Flap detection
2. Acked check
3. Suppressed-until check (PR #25 — manual close persistence)
4. **Upstream-down check (this feature)**
5. Dedup check
6. Fire

No direct code conflict with PR #25.

### Feasibility by Phase

**Phase 1 — Graph model, YAML parsing, basic suppression**
Highly feasible. The `map[NodeID][]NodeID` graph is trivial Go. Extending `InterfaceConfig` in `internal/config/types.go` with a `depends_on` field is straightforward. However, the existing `Members` / `MemberPolicy` fields already model a form of port-channel dependency — Phase 1 must either unify with that model or explicitly decouple from it and document why. Do not silently duplicate it.

**Phase 2 — Severity adjustment, UI root cause indicator**
Feasible. The UI root cause indicator (showing *why* an alert is suppressed in the dashboard) is the highest-value deliverable and not complex. Severity propagation logic needs a defined behaviour spec before implementation starts.

**Phase 3 — Auto-discovery, visualization**
Ambitious. LLDP MIB polling is doable given NetSpec already has SNMP collection infrastructure, but it is a significant collector addition. Browser-side graph visualization (d3.js / Cytoscape.js) is out of character with the current minimal UI approach and is substantial frontend work. Treat Phase 3 as a separate project.

### Gaps to Resolve Before Implementation

1. **`NodeID` encoding** — The YAML shows `device` and `interface` as separate fields but the graph needs a single composite key. The codebase already uses `device|entity` as a key pattern throughout the alert engine. `NodeID` should follow the same convention and be explicitly specified in the design.

2. **Cycle detection** — Mentioned only in the testing section. A cycle in the dependency graph causes infinite traversal. Cycle detection must be part of Phase 1 graph construction, enforced at config load time (fail fast in `ValidateConfig`), not deferred to a test.

3. **Suppression lift behaviour** — When an upstream recovers, what happens to downstream alerts that were suppressed while it was down? Do they automatically re-fire if the condition still exists on the downstream device? This must be explicitly defined. The PR #25 pattern (lift suppression when a resolve event is received for the suppressing entity) is a reasonable model to follow.

4. **Interaction with port-channel members** — `Members.Required` + `MemberPolicy` already model member-level dependency. The topology graph should either fold port-channel members in as first-class edges or document the intentional boundary between the two systems.

5. **Multi-hop behaviour** — If A depends on B which depends on C, and C goes down, is A suppressed? The graph traversal implies yes but this should be stated explicitly in the spec, including the maximum traversal depth.

### Config Burden Risk

The biggest practical risk is not technical — it is the cost of manually declaring `depends_on` for every interface in a mid-sized network. Without Phase 3 auto-discovery, operator adoption may be low. Consider whether Phase 1 could auto-populate dependency edges from the existing port-channel `Members` config as a zero-config starting point, delivering immediate value before manual topology entry is required.
