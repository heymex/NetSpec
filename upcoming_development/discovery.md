# NetSpec Feature Spec: Interactive Device & Interface Discovery Wizard

> **Purpose**: This document provides a complete implementation brief for an AI coding agent to build the Device Discovery Wizard feature into NetSpec. It covers every layer of the stack — backend API endpoints, SNMP logic, YAML config patching, and frontend UI — with enough context to write production-quality Go and HTML/JS without needing to ask clarifying questions.

---

## 1. Feature Overview

The Device Discovery Wizard is an interactive tool embedded in the NetSpec web UI (served at `http://localhost:8088`) that allows a user to:

1. Enter an IP address or hostname and an SNMP community string (defaulting to `SNMP_COMMUNITY` from `.env`).
2. Trigger a lightweight SNMP probe to identify the device (sysDescr, sysName, sysObjectID).
3. Optionally walk the `ifTable` / `ifXTable` to enumerate all interfaces.
4. Review each interface in a checklist UI, selecting which to monitor and what the desired operational state should be.
5. Commit the result, which patches `config/desired-state.yaml` (add device if new, patch interfaces if device already exists) and prompts the user to reload.

---

## 2. Codebase Context

### 2.1 Relevant Existing Files

| Path | Purpose |
|---|---|
| `internal/api/server.go` | HTTP server, existing routes (`/health`, `/status`, `/alerts`, `/api/reload`, etc.) |
| `internal/config/loader.go` | Parses `desired-state.yaml` into Go structs |
| `internal/config/validator.go` | Validates config before apply |
| `config/desired-state.yaml` | The file this feature patches |
| `.env` / `.env.example` | Contains `SNMP_COMMUNITY`, `GNMI_USERNAME`, etc. |
| `go.mod` | Module root — add any new dependencies here |

### 2.2 Existing `desired-state.yaml` Schema (relevant excerpt)

```yaml
global:
  gnmi_port: 9339
  collection_interval: 10s
  telemetry_mode: snmp_validate_only   # or gnmi_pull / telemetry_ingest_push

devices:
  <device-key>:                        # arbitrary slug, e.g. "core-sw-01"
    address: <ip-or-hostname>
    description: "<human label>"
    interfaces:
      <IfName>:
        description: "<iface description>"
        desired_state: up              # up | down
        admin_state: enabled           # enabled | disabled
        monitor: true                  # NEW FIELD — whether NetSpec evaluates this iface
        alerts:
          state_mismatch: warning      # or critical / info
```

> **Note**: The `monitor` boolean is introduced by this feature. The agent must add it to the config struct and evaluator so that interfaces with `monitor: false` are stored in YAML (for reference) but silently skipped by the evaluator.

### 2.3 `.env` Variables Relevant to This Feature

```
SNMP_COMMUNITY=public          # Default community; wizard uses this unless overridden
```

---

## 3. SNMP Library Choice

Use **`github.com/gosnmp/gosnmp`** (already a common transitive dependency in the Go SNMP ecosystem; add to `go.mod` if not present).

Version constraint: `v1.37.0` or later (supports SNMPv2c context, bulk walk).

---

## 4. Backend: New API Endpoints

All new endpoints live in `internal/api/server.go` (or a new file `internal/api/discovery.go` registered from the same server setup).

### 4.1 `POST /api/discovery/probe`

**Purpose**: Performs a minimal SNMP GET to identify the device.

**Request body** (JSON):
```json
{
  "address": "10.0.1.1",
  "community": "public",       // optional; falls back to SNMP_COMMUNITY env var
  "port": 161                  // optional; defaults to 161
}
```

**SNMP OIDs to GET**:
| OID | MIB Name | Purpose |
|---|---|---|
| `1.3.6.1.2.1.1.1.0` | `sysDescr` | Full device description string |
| `1.3.6.1.2.1.1.2.0` | `sysObjectID` | Vendor/model OID |
| `1.3.6.1.2.1.1.5.0` | `sysName` | Configured hostname |
| `1.3.6.1.2.1.1.6.0` | `sysLocation` | Physical location (bonus context) |

**Response body** (JSON, `200 OK`):
```json
{
  "address": "10.0.1.1",
  "sys_name": "core-sw-01",
  "sys_descr": "Cisco IOS XE Software, Version 17.09.04a ...",
  "sys_object_id": "1.3.6.1.4.1.9.1.1208",
  "sys_location": "MDF Rack 3",
  "vendor_hint": "Cisco",       // derived heuristically from sysObjectID/sysDescr
  "already_configured": true,   // true if address matches an existing devices entry
  "existing_device_key": "core-sw-01"  // populated when already_configured=true
}
```

**Error responses**:
- `400 Bad Request` — missing/invalid address
- `504 Gateway Timeout` — SNMP timeout (surface the raw error message in `"error"` field)
- `502 Bad Gateway` — SNMP error other than timeout

**Implementation notes**:
- Timeout: 3 seconds, 2 retries.
- Community falls back to `os.Getenv("SNMP_COMMUNITY")` if not supplied in body.
- `vendor_hint` logic: check `sysObjectID` prefix (`1.3.6.1.4.1.9` → Cisco, `.11` → HP/Aruba, `.25461` → Palo Alto, etc.). Also check for keywords in `sysDescr` as fallback. Return `"Unknown"` if no match.
- `already_configured` is determined by iterating the in-memory loaded config for any device whose `address` matches.

---

### 4.2 `POST /api/discovery/walk`

**Purpose**: Walks `ifTable` and `ifXTable` to return all interfaces on the device.

**Request body** (JSON):
```json
{
  "address": "10.0.1.1",
  "community": "public",
  "port": 161
}
```

**OID subtrees to walk** (use `BulkWalk`):
| Subtree OID | MIB Name | Data collected |
|---|---|---|
| `1.3.6.1.2.1.2.2.1.1` | `ifIndex` | Interface index |
| `1.3.6.1.2.1.2.2.1.2` | `ifDescr` | Interface name (e.g. `GigabitEthernet1/0/1`) |
| `1.3.6.1.2.1.2.2.1.3` | `ifType` | IANAifType integer |
| `1.3.6.1.2.1.2.2.1.7` | `ifAdminStatus` | 1=up 2=down 3=testing |
| `1.3.6.1.2.1.2.2.1.8` | `ifOperStatus` | 1=up 2=down … |
| `1.3.6.1.2.1.31.1.1.1.1` | `ifName` | Canonical short name (prefer over ifDescr when present) |
| `1.3.6.1.2.1.31.1.1.1.18` | `ifAlias` | Interface description/alias string |

**Response body** (JSON, `200 OK`):
```json
{
  "address": "10.0.1.1",
  "interfaces": [
    {
      "index": 1,
      "name": "GigabitEthernet1/0/1",
      "alias": "Uplink to Core",
      "type": 6,
      "type_label": "ethernetCsmacd",
      "admin_status": "up",
      "oper_status": "up",
      "already_configured": false
    },
    {
      "index": 3,
      "name": "Loopback0",
      "alias": "",
      "type": 24,
      "type_label": "softwareLoopback",
      "admin_status": "up",
      "oper_status": "up",
      "already_configured": false
    }
  ]
}
```

**Implementation notes**:
- `type_label` should be derived from a small embedded map of common IANAifType values (6=ethernetCsmacd, 24=softwareLoopback, 131=tunnel, 161=ieee8023adLag, 53=propVirtual, etc.).
- `already_configured` is `true` if that interface name already exists under the matching device in the loaded config.
- Walk timeout: 15 seconds total.
- Filter out interfaces of type `softwareLoopback` (24) and `propVirtual` (53) from the default display, but include a flag in the response so the UI can optionally show them. Return a top-level `"filtered_count": N` indicating how many were hidden.
- Sort: physical interfaces first (type 6), then port-channels (type 161), then others.

---

### 4.3 `POST /api/discovery/commit`

**Purpose**: Takes the user's selections and patches `desired-state.yaml`.

**Request body** (JSON):
```json
{
  "address": "10.0.1.1",
  "community": "public",
  "device_key": "core-sw-01",          // user-editable slug; pre-filled from sys_name
  "device_description": "Core Switch", // user-editable
  "existing_device_key": "core-sw-01", // if patching, the key that already exists
  "action": "add",                      // "add" | "patch"
  "interfaces": [
    {
      "name": "GigabitEthernet1/0/1",
      "alias": "Uplink to Core",
      "monitor": true,
      "desired_state": "up",            // "up" | "down"
      "admin_state": "enabled",         // "enabled" | "disabled"
      "alert_severity": "critical"      // "info" | "warning" | "critical"
    },
    {
      "name": "GigabitEthernet1/0/2",
      "alias": "",
      "monitor": false,
      "desired_state": "up",
      "admin_state": "enabled",
      "alert_severity": "warning"
    }
  ]
}
```

**Behavior**:

- **`action: "add"`** — creates a new top-level key under `devices:` in the YAML. Error if `device_key` already exists.
- **`action: "patch"`** — merges interface entries into an existing device. For each interface in the request:
  - If the interface already exists in YAML: overwrite its fields.
  - If it does not exist: append it.
  - Interfaces in YAML but *not* in the request payload are left untouched.

**YAML write strategy**:
1. Read the current `desired-state.yaml` into memory using `gopkg.in/yaml.v3` (preserving comments is nice-to-have; use `yaml.Node` tree if feasible, otherwise a clean marshal is acceptable).
2. Apply mutations to the Go struct / `yaml.Node` tree.
3. Write back atomically: write to a `.tmp` file, then `os.Rename` to the target path.
4. Do **not** trigger a live reload automatically — that is a separate user action.

**Response body** (JSON, `200 OK`):
```json
{
  "success": true,
  "action": "add",
  "device_key": "core-sw-01",
  "interfaces_written": 12,
  "interfaces_monitored": 4,
  "message": "Device added successfully. Click 'Reload Config' to apply changes."
}
```

**Error responses**:
- `409 Conflict` — `action: "add"` but device key already exists.
- `404 Not Found` — `action: "patch"` but device key does not exist.
- `400 Bad Request` — validation failure (empty device_key, no interfaces, invalid desired_state value, etc.)
- `500 Internal Server Error` — file write failure.

---

## 5. Frontend: Discovery Wizard UI

The wizard is added as a new page within the existing NetSpec web UI. The existing UI is Go `html/template`-rendered HTML with minimal JavaScript (the live logs page uses a `setInterval` fetch). The wizard requires more interactive JS — use **vanilla JS with `fetch()`**; no external framework dependencies.

### 5.1 Navigation

Add a **"Add Device"** link to the existing nav bar in the base template. Route: `/wizard`.

The `/wizard` GET handler returns the wizard HTML page (Go template, same base layout as other pages).

### 5.2 Wizard Step Structure

The wizard is a single HTML page that shows/hides steps. Do **not** use separate page navigations between steps — all state lives in JS variables.

```
Step 1: Connect & Probe
Step 2: Review Device Info
Step 3: Interface Selection
Step 4: Confirm & Commit
Step 5: Success / Reload Prompt
```

---

### Step 1 — Connect & Probe

**Fields**:
| Field | Type | Default | Notes |
|---|---|---|---|
| IP / Hostname | text input | — | Required |
| SNMP Community | text input | `(from .env)` | Placeholder shows env var hint |
| SNMP Port | number input | `161` | Advanced — collapsed by default |

The SNMP community placeholder text should be `"Uses SNMP_COMMUNITY from .env"` when the field is empty, making it clear the default comes from config.

**"Probe Device" button**:
- Shows a spinner.
- Calls `POST /api/discovery/probe`.
- On success → advance to Step 2.
- On error → display inline error message in red (include the raw error from the API response).

---

### Step 2 — Review Device Info

Displays a read-only info card:

```
┌─────────────────────────────────────────────┐
│  🟢  Device Identified                      │
│                                             │
│  Hostname:    core-sw-01                    │
│  Address:     10.0.1.1                      │
│  Vendor:      Cisco                         │
│  Description: Cisco IOS XE Software, ...    │
│  Location:    MDF Rack 3                    │
│                                             │
│  ⚠️  This device is already configured.     │
│     Proceeding will patch existing entries. │
└─────────────────────────────────────────────┘
```

Show the "already configured" warning prominently if `already_configured: true`.

**Editable fields** (pre-filled from probe response):
- **Device Key** (slug) — pre-filled from `sys_name`, lowercased, spaces→hyphens. User can edit. Validate: alphanumeric + hyphens only, no spaces.
- **Device Description** — pre-filled from `sys_name` or `sys_descr` (truncated). User can edit.

**Buttons**:
- "Walk Interfaces" — triggers Step 3 walk.
- "Back" — returns to Step 1.

The "Walk Interfaces" button calls `POST /api/discovery/walk` and shows a spinner with status text: *"Walking interface table, this may take a few seconds…"*

---

### Step 3 — Interface Selection

Display a filterable, scrollable table of interfaces returned from the walk.

**Table columns**:
| Column | Notes |
|---|---|
| ☑ Monitor | Checkbox. Default: `true` for physical (type 6) and LAG (type 161) interfaces; `false` for everything else. Already-configured interfaces shown with their existing `monitor` value. |
| Interface Name | `ifName` value |
| Alias / Description | `ifAlias` — editable inline text input |
| Current Admin | Badge: `up` (green) / `down` (red) |
| Current Oper | Badge: `up` (green) / `down` (grey) |
| Desired State | Dropdown: `up` / `down`. Default: match current oper_status. |
| Alert Severity | Dropdown: `info` / `warning` / `critical`. Default: `warning`. |

**Filter bar** above the table:
- Text search (filters by interface name or alias).
- Toggle: "Show virtual/loopback interfaces" (default: hidden, matching the API filter).
- Toggle: "Show only monitored" — filters to rows where Monitor checkbox is checked.

**Select All / Deselect All** buttons that affect only the currently visible (filtered) rows.

**Already-configured badge**: Interfaces where `already_configured: true` should display a small grey `configured` badge. Their rows should be pre-filled with the existing YAML values.

**Buttons**:
- "Review & Commit" — advances to Step 4.
- "Back" — returns to Step 2 (does not re-probe, uses cached data).

---

### Step 4 — Confirm & Commit

Display a summary before writing:

```
┌──────────────────────────────────────────────────────┐
│  Ready to write config                               │
│                                                      │
│  Device key:    core-sw-01                           │
│  Action:        Add new device                       │  ← or "Patch existing device"
│  Address:       10.0.1.1                             │
│  Interfaces:    12 total · 4 monitored               │
│                                                      │
│  Monitored interfaces:                               │
│    ✔ GigabitEthernet1/0/1  — desired: up  (critical) │
│    ✔ GigabitEthernet1/0/2  — desired: up  (warning)  │
│    ✔ Port-channel1         — desired: up  (critical) │
│    ✔ GigabitEthernet1/0/48 — desired: down (warning) │
│                                                      │
│  [ Back ]  [ Write to desired-state.yaml ]           │
└──────────────────────────────────────────────────────┘
```

"Write to desired-state.yaml" calls `POST /api/discovery/commit`.

On success → Step 5.
On error → display inline error, stay on Step 4.

---

### Step 5 — Success & Reload Prompt

```
┌──────────────────────────────────────────────────────┐
│  ✅  Config written successfully                     │
│                                                      │
│  core-sw-01 has been added to desired-state.yaml.    │
│  NetSpec is still running with the previous config.  │
│                                                      │
│  [ Reload Config Now ]   [ Add Another Device ]      │
│  [ Go to Dashboard ]                                 │
└──────────────────────────────────────────────────────┘
```

**"Reload Config Now"** calls the existing `POST /api/reload` endpoint. On success, show a toast/banner: *"Config reloaded. NetSpec is now monitoring your updated device list."*

**"Add Another Device"** resets the wizard state and returns to Step 1.

---

## 6. Go Struct Changes

### 6.1 Config Loader (`internal/config/loader.go`)

Add `Monitor` field to the interface struct:

```go
type InterfaceConfig struct {
    Description  string            `yaml:"description"`
    DesiredState string            `yaml:"desired_state"`
    AdminState   string            `yaml:"admin_state"`
    Monitor      bool              `yaml:"monitor"`          // NEW
    Members      *MemberConfig     `yaml:"members,omitempty"`
    MemberPolicy *MemberPolicy     `yaml:"member_policy,omitempty"`
    Alerts       map[string]string `yaml:"alerts"`
}
```

Default `Monitor` to `true` when not present in YAML (use a custom `UnmarshalYAML` or post-parse pass).

### 6.2 Evaluator (`internal/evaluator/interface.go`)

Add a guard at the top of interface evaluation:

```go
if !ifaceCfg.Monitor {
    continue  // skip evaluation for this interface
}
```

### 6.3 New Package: `internal/discovery/`

Create a new package with these files:

```
internal/discovery/
├── snmp.go        // ProbeDevice(), WalkInterfaces() — wraps gosnmp
├── types.go       // ProbeResult, Interface, WalkResult structs
└── yaml_patch.go  // PatchDesiredState() — reads/mutates/writes YAML
```

**`snmp.go`** — key function signatures:

```go
func ProbeDevice(address string, port uint16, community string, timeout time.Duration) (*ProbeResult, error)

func WalkInterfaces(address string, port uint16, community string, timeout time.Duration) (*WalkResult, error)
```

**`yaml_patch.go`** — key function signature:

```go
// action: "add" | "patch"
func PatchDesiredState(configPath string, req *CommitRequest) (*CommitResult, error)
```

`PatchDesiredState` must:
1. Read the YAML file.
2. Unmarshal into the existing `Config` struct.
3. Apply adds/patches.
4. Marshal back to YAML with `gopkg.in/yaml.v3`.
5. Atomic write via temp file + rename.

---

## 7. New HTTP Handler Wiring

In `internal/api/server.go` (or `discovery.go`), register:

```go
mux.HandleFunc("GET /wizard",                   s.handleWizardPage)
mux.HandleFunc("POST /api/discovery/probe",     s.handleDiscoveryProbe)
mux.HandleFunc("POST /api/discovery/walk",      s.handleDiscoveryWalk)
mux.HandleFunc("POST /api/discovery/commit",    s.handleDiscoveryCommit)
```

Each handler:
1. Decodes JSON request body.
2. Calls the corresponding `discovery` package function.
3. Encodes JSON response.
4. Returns appropriate HTTP status code on error.

All handlers should log the action at `INFO` level with structured fields (device address, action type, outcome).

---

## 8. Security Considerations

- The SNMP community string in request bodies is **never** logged. Redact it in all log lines.
- Validate `address` to prevent SSRF-style abuse: accept only valid IPv4, IPv6, or hostname formats. Reject anything with a scheme prefix (`http://`, etc.).
- Validate `device_key` to alphanumeric + hyphens + underscores, max 64 characters.
- The `desired-state.yaml` path is **not** configurable via the API — it is hardcoded to the server's known config path. Do not accept a path parameter from the client.
- File writes use `0644` permissions; no world-executable bit.

---

## 9. Error Handling & Edge Cases

| Scenario | Expected Behavior |
|---|---|
| SNMP timeout | `504` with `"error": "SNMP timeout after 3s"` |
| Device unreachable (port closed) | `502` with descriptive message |
| Walk returns 0 interfaces | `200` with empty array + `"message": "No interfaces found"` |
| Device already in config, `action: "add"` | `409 Conflict` |
| YAML file not writable | `500` with error; do not leave partial file |
| User submits 0 monitored interfaces | Allow it — store all interfaces with `monitor: false`; add a UI warning but do not block commit |
| `ifName` missing for an interface | Fall back to `ifDescr` |
| `ifAlias` missing | Use empty string; do not show column as blank, show `—` in UI |

---

## 10. Testing

### Unit Tests (`internal/discovery/`)

- `TestProbeDevice_Success` — mock gosnmp, verify struct population.
- `TestProbeDevice_Timeout` — mock timeout, verify error type.
- `TestWalkInterfaces_FilterLoopback` — verify loopback interfaces are excluded from primary list.
- `TestWalkInterfaces_SortOrder` — physical first, then LAG, then other.
- `TestPatchDesiredState_Add` — write to temp YAML, verify new device key appears.
- `TestPatchDesiredState_Patch` — write to temp YAML with existing device, verify interface merge.
- `TestPatchDesiredState_AtomicWrite` — simulate write failure mid-way, verify original file intact.

### Integration / Manual Test Checklist

- [ ] Probe a live Cisco IOS-XE device; verify `vendor_hint: "Cisco"`.
- [ ] Probe a device with wrong community; verify `502` error in UI.
- [ ] Walk a device with 48 physical ports; verify table renders and scrolls.
- [ ] Commit an add; verify YAML contains new device block.
- [ ] Commit a patch; verify existing interfaces are overwritten, others preserved.
- [ ] Reload after commit; verify NetSpec begins monitoring new interfaces.
- [ ] Probe a device already in config; verify "already configured" warning.

---

## 11. Suggested Commit / PR Structure

| PR | Scope |
|---|---|
| #1 | `internal/discovery/` package — SNMP probe + walk + YAML patch + unit tests |
| #2 | `internal/api/` — three new endpoints wired to discovery package |
| #3 | `internal/config/` — add `Monitor` field + evaluator guard |
| #4 | Web UI — wizard page template + JS |
| #5 | Integration test + documentation update |

---

## 12. Documentation Updates Required

- **`README.md`**: Add "Adding Devices" section pointing to the wizard at `/wizard`.
- **`docs/CONFIGURATION.md`** (create if not present): Document the new `monitor` field under interface config.
- **`netspec-spec.md`**: Add the Device Discovery Wizard to the Web Interface section and API Endpoints table.

---

*End of Feature Spec*