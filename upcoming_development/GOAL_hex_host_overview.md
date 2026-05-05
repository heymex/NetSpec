# Development Goal: Hexagonal Host Overview Visualization

**Project:** NetSpec — Declarative Network State Monitor  
**Feature Area:** Web UI (`/` dashboard)  
**Priority:** Medium  
**Status:** In progress (phase 1: honeycomb + polling — landing via PR)

---

## Phase 1 (done)

- **`internal/webui/hexmap.go`** — layout geometry, severity aggregation, SVG HTML rendering helpers (`DefaultHexRadius`, cap **64** tiles).
- **`internal/webui/hexmap_test.go`** — worst-severity aggregation, layout bbox, honeycomb stagger, cap.
- **Dashboard (`internal/webui/templates.go`)** — “Host Overview” card, `#hex-overview-root`, server-rendered SVG on load + **vanilla JS** poll every **10s** via **`/api/devices`** + **`/alerts`** (see note below on `/status`).
- **`internal/api/server.go`** — builds initial honeycomb from configured devices + active alerts (`PageData.HexMapSVG`).

### Follow-ups

- Distinct styling for **down/unreachable** vs **critical**, **maintenance** blue outline (needs runtime maintenance awareness).
- Tooltip “worst interface state” vs alert-derived bucket (could incorporate evaluator snapshot API later).
- Optional: enrich **`/status`** with device summaries so polling matches the original doc literally.

---

## Overview

Add a **hexagonal host overview panel** to the NetSpec web dashboard, inspired by the Checkmk-style "honeycomb" host map. Each monitored device is represented as a hexagonal tile whose color reflects the device's current state. The panel provides an at-a-glance spatial summary of fleet health, complementing the existing tabular device list and active alerts sections.

---

## Motivation

The current web UI at `http://localhost:8088` surfaces state through text lists and log streams. As the number of monitored devices grows, scanning a list becomes slower and less intuitive than a spatial, color-coded overview. A honeycomb panel communicates:

- **How many devices are healthy vs. degraded** — instantly, without reading rows
- **Cluster patterns** — groups of adjacent failing nodes may indicate a physical or segment-level problem
- **Severity gradations** — color encodes OK / Warning / Critical / Down states visually

This aligns with NetSpec's core philosophy: *state correctness matters more than metrics*, and the UI should reflect state at a glance.

---

## Reference Design

The target is modeled on the **Host Overview** panel visible in Checkmk's main dashboard (the hexagonal honeycomb section), where:

- Each hexagon = one monitored host
- **Fill color** = worst active alert severity for that host
- **Border color** = secondary severity or maintenance state
- Hovering a hex reveals the hostname and current state
- Clicking a hex navigates to that device's detail view

### Color Mapping

| State | Color |
|---|---|
| All interfaces OK | Dark / outline only (healthy, no fill) |
| Warning | Yellow / amber |
| Critical | Red (solid fill) |
| Down / Unreachable | Deep red (thick border + fill) |
| In maintenance | Blue outline |

---

## Scope

### In Scope

- SVG-based hexagonal grid rendered in the existing Go HTML template (or as a self-contained `<div>` served by the `/` handler)
- State data sourced from **`/api/devices`** (configured device names) and **`/alerts`** (active alerts with `Device` + `Severity`) — **no new backend endpoints** for this iteration  
  - *Note:* `/status` today returns uptime/version and alert **count** only, not a device list; polling uses `/api/devices` instead.
- Hex count and layout scale dynamically with device count (cap **64** hexes per panel; see code constants)
- Tooltip on hover: device hostname + worst interface state
- Click-through to the existing device detail view (if/when implemented)
- Responsive layout that degrades gracefully on narrow viewports

### Out of Scope (for this iteration)

- Animated transitions between state changes (can be added later)
- User-configurable grid arrangement / drag-and-drop
- Per-interface sub-hexagons within a device hex
- Historical state replay / time-scrubbing

---

## Agent Task Description

> **You are a Go/HTML/SVG agent.** Your task is to implement the hexagonal host overview panel for the NetSpec web dashboard.

### Inputs Available to the Agent

- **`internal/api`** — HTTP handlers (e.g. `/`, `/api/devices`, `/alerts`)
- **`internal/webui`** — HTML templates (`templates.go`) and hex helpers (`hexmap.go`)
- **`/api/devices`** (JSON): configured devices (`name`, `address`, …)
- **`/alerts`** (JSON): `alerts` array with `Device`, `Severity`, …
- The color mapping table above

### Deliverables

1. **`internal/webui/hexmap.go`** — helpers that build `HexMapLayout` / `HexTile` from device names + worst severity per device, and render SVG (`RenderHexMapSVG`).

2. **Updated dashboard template** — add a host-overview section with **`#hex-overview-root`** containing an inline SVG honeycomb. The SVG must:
   - Use `pointy-top` hex orientation
   - Pack hexes in a spiral or row-offset grid layout
   - Apply fill colors from the state → color mapping
   - Include `<title>` elements per hex for native browser tooltip support
   - Poll **`/api/devices`** and **`/alerts`** via `fetch()` on a 10-second interval and re-render without full page reload

3. **CSS additions** — dark-theme styles consistent with the existing UI (`background: #1a1a2e` palette), hover highlight, and hex border rendering.

4. **Unit tests** (`internal/webui/hexmap_test.go`) — cover grid coordinate generation and severity aggregation logic.

### Constraints

- No new npm/node dependencies; plain JavaScript only (no React, no D3)
- Must not break existing API endpoints or introduce new required config fields
- SVG rendering must work without a canvas element (pure SVG path-based hexagons)
- The panel must degrade to a simple device count summary if zero devices are configured

### Acceptance Criteria

- [x] Hexagonal grid renders on the dashboard with at least one device
- [x] Colors update within 15 seconds of an alert state change (10s poll interval)
- [x] Hovering a hex shows hostname and worst state string (`<title>` + native tooltip)
- [ ] No JavaScript console errors in Chrome / Firefox (manual QA)
- [x] Existing dashboard sections (device list, alerts, logs) remain intact
- [x] `go test ./internal/webui/...` passes

---

## Implementation Notes

### Hex Grid Geometry

For a pointy-top hexagon with circumradius `R`:

```
width  = R * sqrt(3)
height = R * 2
horiz_spacing = width
vert_spacing  = height * 0.75
```

Offset rows by `width / 2` for even rows to produce the honeycomb stagger.

Center coordinates for hex at grid position `(col, row)`:

```
x = col * horiz_spacing + (row % 2) * (width / 2)
y = row * vert_spacing
```

SVG path for a single pointy-top hex centered at `(cx, cy)` with circumradius `R`:

```
M cx, cy-R
L cx+R*sin(60°), cy-R*cos(60°)
L cx+R*sin(60°), cy+R*cos(60°)
L cx, cy+R
L cx-R*sin(60°), cy+R*cos(60°)
L cx-R*sin(60°), cy-R*cos(60°)
Z
```

### State Aggregation Logic

For each device, derive its display color by:

1. Collecting all active alerts for that device from `/alerts`
2. Taking the maximum severity: `critical > warning > ok`
3. If no alerts exist, the device is **OK** (dark/outline hex)
4. If the device itself is absent from `/status`, treat as **unknown** (gray)

### Polling Pattern (Vanilla JS)

```javascript
async function refreshHexMap() {
  const [devicesRes, alertsRes] = await Promise.all([
    fetch('/api/devices').then(r => r.json()),
    fetch('/alerts').then(r => r.json())
  ]);
  const names = (devicesRes.devices || []).map(d => d.name);
  renderHexGrid(names, alertsRes.alerts || []);
}
setInterval(refreshHexMap, 10000);
refreshHexMap();
```

---

## Open Questions

- Should the hex grid be server-side rendered (Go template, static on page load) and then updated client-side, or fully client-side rendered from the start? **Recommendation:** fully client-side for simplicity; the Go template only emits the container `<div>`.
- What is the maximum practical device count before the grid needs pagination or zoom? Suggest capping at 64 hexes per "page" and adding a scroll affordance beyond that.
- Should maintenance windows (from `config/maintenance.yaml`) suppress color changes during scheduled windows? **Recommendation:** yes, render as blue outline if device is in an active maintenance window.

---

## Related Files

| File | Relevance |
|---|---|
| `internal/webui/templates.go` | Dashboard markup + hex poll script |
| `internal/webui/hexmap.go` | Layout + SVG rendering helpers |
| `internal/api/server.go` | `/`, `/api/devices`, `/alerts` handlers; seeds `HexMapSVG` |
| `config/desired-state.yaml` | Source of device list |
| `config/alerts.yaml` | Alert routing (informs severity model) |
| `/api/devices` | Configured devices JSON |
| `/alerts` | Active alerts JSON |
| `docs/CISCO_GNMI_SETUP.md` | Context on device telemetry sources |
