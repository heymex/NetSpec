# Codebase Consistency Fixes

**Project:** NetSpec — Declarative Network State Monitor  
**Source:** Internal review — doc / functionality / design audit  
**Date:** 2026-05-04  

---

## Overview

This plan addresses inconsistencies found between documentation, configuration examples, and actual runtime behavior. Issues are ordered by user-facing impact: silent misconfigurations first, then bugs, then dead code, then documentation.

---

## Priority 1 — Silent Misconfiguration (Fix Immediately)

### 1.1 Move `alerts:` out of `desired-state.yaml`

**Problem:** `config/desired-state.yaml` contains a full `alerts:` block, but the loader reads that file into `cfg.DesiredState` (`DesiredStateConfig` struct), which has no `Alerts` field. YAML drops unknown keys silently. Any user following the example has non-functional alerting with no error or warning.

**Fix:**
- Remove the `alerts:` block from `config/desired-state.yaml`
- Create `config/alerts.yaml` containing that block (this is where the loader actually reads it from)
- Update `README.md` to clearly state alerts belong in `alerts.yaml`, not `desired-state.yaml`
- Add a comment to the top of `desired-state.yaml` referencing `alerts.yaml`

**Files:** `config/desired-state.yaml`, `config/alerts.yaml` (new), `README.md`

---

## Priority 2 — Code Bug

### 2.1 Fix `formatDuration` for uptimes ≥ 10 days or hours

**Problem:** `internal/api/server.go:747-749` uses `string(rune('0'+days))` to convert integers to strings. This is correct only for 0–9. For 10+, the rune overflows into punctuation (e.g., day 10 → `':'`, day 12 → `'<'`). The same pattern is used for hours on the same lines.

**Fix:** Replace the rune arithmetic with `strconv.Itoa()`:

```go
// Before
return string(rune('0'+days)) + "d"

// After
return strconv.Itoa(days) + "d"
```

Apply the same fix to the hours branch on line 749.

**Files:** `internal/api/server.go:747-749`

---

## Priority 3 — Dead Code Cleanup

### 3.1 Remove duplicate `AlertConfig` type

**Problem:** `internal/config/types.go:152` defines `AlertConfig`, which is structurally identical to `AlertsConfig` (line 28) and is never referenced anywhere. It will cause confusion about which type is authoritative.

**Fix:** Delete the `AlertConfig` type and its comment block (lines 151–156 approximately).

**Files:** `internal/config/types.go`

---

### 3.2 Decide on credentials and maintenance features — implement or remove

Both features are partially scaffolded:

- **Credentials:** `credentials.yaml` is loaded, `credentials_ref` is validated, `ResolveCredentials()` is implemented — but it is never called. Devices can reference credentials that are never actually used for authentication.
- **Maintenance windows:** `maintenance.yaml` is loaded and the `MaintenanceWindow` type is defined, but there is no suppression logic anywhere that checks them. Configuring a maintenance window does nothing.

**Options (pick one per feature):**

| Option | Credentials | Maintenance |
|--------|------------|-------------|
| A | Wire `ResolveCredentials()` into the SNMP/gNMI collectors | Implement alert suppression checks against active windows |
| B | Remove the loading, types, and validation entirely until the feature is ready | Same |

Until a decision is made, add a `// TODO: not yet wired` comment to `ResolveCredentials()` and the maintenance loader so the intent is visible to contributors.

**Files:** `internal/config/loader.go:194`, `internal/config/types.go:193`, `internal/config/loader.go:54`

---

## Priority 4 — Documentation

### 4.1 Document all optional config files in README

**Problem:** `README.md` does not mention `credentials.yaml` or `maintenance.yaml`. Users have no way to discover these files exist.

**Fix:** Add a "Configuration Files" reference table to `README.md`:

| File | Required | Purpose |
|------|----------|---------|
| `config/desired-state.yaml` | Yes | Global settings, devices, interfaces |
| `config/alerts.yaml` | No | Alert channels and routing rules |
| `config/credentials.yaml` | No | Named credential sets for device auth |
| `config/maintenance.yaml` | No | Scheduled maintenance windows |
| `config/devices/*.yaml` | No | Per-device split config files |

**Files:** `README.md`

---

### 4.2 Document `API_PORT` environment variable

**Problem:** `cmd/netspec/main.go` reads `API_PORT` to override the default port 8088, but this variable does not appear in `.env.example` or `README.md`.

**Fix:** Add `API_PORT=8088` (with comment) to `.env.example` and to the environment variable reference section in `README.md`.

**Files:** `.env.example`, `README.md`

---

### 4.3 Correct the config reload description in README

**Problem:** `README.md` says the reload button re-reads `desired-state.yaml`. It actually calls `LoadConfigDir()`, which reloads all config files in the directory.

**Fix:** Update the description to: "Reloads all configuration files from the config directory without restarting the process."

**Files:** `README.md`

---

### 4.4 Clarify `gnmic` in the Docker image

**Problem:** `Dockerfile:34-49` installs the `gnmic` CLI tool, but the Go application never invokes it. The README mentions it only in passing. New contributors will wonder why it's there.

**Fix:** Either:
- **A (preferred):** Add a comment in the Dockerfile explaining it is bundled for operator use (ad-hoc CLI queries from inside the container), and add a brief note in `README.md` under a "Bundled Tools" or "Debugging" section.
- **B:** Remove it from the image and document it as an optional external tool.

**Files:** `Dockerfile`, `README.md`

---

## Priority 5 — Infrastructure

### 5.1 Resolve Docker Compose networking ambiguity

**Problem:** `netspec` uses `network_mode: host` while `apprise` uses a custom bridge network (`monitoring`). They are on different network planes. The `depends_on: apprise` has no health-check condition, so `netspec` can start before Apprise is ready.

**Fix options:**

| Option | Trade-off |
|--------|-----------|
| A | Keep host networking for `netspec`; document clearly that `APPRISE_API_URL` must use `127.0.0.1` and explain why `depends_on` is best-effort in this topology |
| B | Move both services onto the same bridge network; replace `127.0.0.1` with the `apprise` service DNS name; add a `healthcheck` to the apprise service and use `depends_on: condition: service_healthy` |

Option B is cleaner long-term. Option A is a lower-risk minimal change.

**Files:** `docker-compose.yml`, `README.md`

---

## Completion Checklist

- [x] 1.1 — Move alerts block to `config/alerts.yaml`, update README
- [x] 2.1 — Fix `formatDuration` rune bug
- [x] 3.1 — Delete duplicate `AlertConfig` type
- [x] 3.2 — Decide on credentials / maintenance: implement or mark TODO
- [x] 4.1 — Document all optional config files in README
- [x] 4.2 — Add `API_PORT` to `.env.example` and README
- [x] 4.3 — Correct reload description in README
- [x] 4.4 — Clarify `gnmic` usage in Dockerfile and README
- [x] 5.1 — Resolve Docker Compose networking (choose option A or B)
