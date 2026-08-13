# NetSpecGraph — 1.0 Scope & Architecture

**Status:** Design draft · **Target module:** `github.com/netspec/netspec` (`cmd/netspecgraph`) · **Companion to:** NetSpec v2.x

**Implementation branch:** `feat/netspecgraph` — step 4 enrichment done (rules index + filters). Next: seasonality band (step 5). Bring-up: [`docs/NETSPECGRAPH.md`](../../docs/NETSPECGRAPH.md).

## Purpose

NetSpecGraph is the metrics-and-graphing companion to NetSpec. NetSpec answers *"is the network in its desired state, and who do I page when it isn't."* NetSpecGraph answers *"what does utilization / errors / optical health look like over time, and is today busy for this point in the semester."* It is **purely visual** — it never pages, never alerts, never evaluates desired state. NetSpec remains the only thing that alerts.

The same ethos applies as the parent project: this is not a general-purpose NPM. It graphs the ports NetSpec already cares about, using the identity and business rules NetSpec already computes, and nothing more.

## Non-goals for 1.0

- Alerting, paging, deviation routing, or notification of any kind.
- Anomaly detection as an alerting primitive (the seasonality band is a *visual* aid only).
- SNMP metric polling. Metrics come from push telemetry; SNMP stays NetSpec's confirmation tool.
- Downsampling / rollup infrastructure and multi-year retention (see Storage).
- Per-user saved dashboards, RBAC beyond reusing `internal/auth`, Grafana provisioning.
- Re-implementing NetSpec's rules, normalization, or device identity. All of it is imported.

## For the implementing agent

### Read first — reuse, don't reimplement

Before writing any code, inventory the existing `internal/` packages and reuse them. Re-creating any of these silently breaks the design's core premise (metrics identity agreeing with NetSpec state "by construction"):

- **Config loader** — the thing that reads `desired-state.yaml` + `config/devices/*.yaml`. NetSpecGraph loads the *same* files read-only; do not write a second parser.
- **Rules engine + device/interface model** — `port_rules`, `neighbor_rules`, device roles, `monitored`/`desired_state`. This is the source of every query-time label. Import it; never re-encode the rules.
- **Interface-name normalizer** — the SNMP-vs-telemetry name-drift solver. The telemetry ↔ canonical mapping must come from here and nowhere else.
- **`internal/auth`** — session + API-token middleware. Reuse it so login is shared with NetSpec.

Locate the exact package paths in the repo (the tree above is `cmd/`, `internal/`, `tools/sidecar/`); confirm signatures by reading the code, not by assuming.

### Invariants — do not violate

1. VM stores only intrinsic labels (`device`, `interface`, `lane`). **No** `role`/`alias`/`neighbor_class`/`monitored`/`desired_state` on any series — those are query-time joins.
2. Do not modify subscription 251, the Telegraf→file path, `decoded.json`, or `mdt-translator`. Metrics are additive: one new Telegraf output, one optional new subscription (211, DOM).
3. No alerting, paging, notification, or desired-state evaluation. NetSpec owns all of that.
4. Store raw counters; never precompute rates into storage. Rates/utilization are query-time.
5. Metric names in the schema are a **contract enforced by the Telegraf rename processor** — do not hardcode assumed Telegraf field names; observe them, then map.

### Build the vertical slice first

One interface, end to end (VM → query client → enriched → one uPlot page) before any breadth. Do not build the fleet view, DOM page, or baseline overlay until the single per-interface page renders real data with a correct seasonality band.

## Architecture

```
IOS-XE dial-out MDT (gRPC-TCP, 17.12.x)
   · sub 251 (existing): /interfaces-state/interface   [ietf, periodic 10s — carries counters+speed+oper; feeds both]
   · sub 211 (new):      /components/component/transceiver [DOM, periodic 30s]
        │
        ▼
   Telegraf (cisco_telemetry_mdt input — the full-fidelity decoder)
        ├─► decoded.json ──► mdt-translator (Python) ──► NetSpec ingest :57500   [state, unchanged]
        └─► outputs.influxdb ──► VictoriaMetrics                                  [metrics, NEW]
                                        │
                                        ▼
                        NetSpecGraph (cmd/netspecgraph, Go)
                        · VM query client (MetricsQL)
                        · shared rules/normalizer/auth from internal/
                        · html/template + vanilla JS + uPlot
                                        │
                                        ▼
                                  Operator browser
```

Two facts drove the metrics tap to Telegraf rather than `mdt-translator`:

1. **Telegraf is already the decoder** and speaks VM natively, so the tap is a config-only output — no new code, and it carries the full numeric payload before the translator reduces it to state.
2. **`decoded.json` is a rotating 100 MB tail buffer**, explicitly "not long-term storage." Routing metrics through the translator would inherit that lossiness for zero benefit. So the translator is left untouched, owning NetSpec *state* ingest only, and metrics bypass the buffer entirely.

The "single source of truth for device identity" goal is preserved — it just moves to the **query layer** (below) instead of the write path.

## Metric ingestion

Add one output to the existing Telegraf config pointing at VM's Influx line-protocol write endpoint (`outputs.influxdb` / `influxdb_v2`; Prometheus remote-write is the equivalent alternative). No collector is added; the one already in the stack gains a second sink.

### Telemetry subscriptions

The live subscription (id 251) filters on `/interfaces-state/interface`, `stream yang-push`, `update-policy periodic 1000` (10s), `encode-kvgpb` → Telegraf MDT receiver on :57500. This is the IETF `ietf-interfaces` operational tree (RFC 7223), and it is **already periodic at 10s**. Its `statistics` container and `speed` leaf hang directly under the subscribed node, so it already streams — right now — in/out octets, unicast/broadcast/multicast pkts, in/out errors, in/out discards (all counter64), plus oper-status and interface speed.

Consequences:

- **Interface metrics need no device change.** Everything the schema needs is on the wire at 10s. No metrics-specific subscription is required; step 1 is purely a Telegraf output.
- **Link speed is free.** `if_speed_bps` comes from `/interfaces-state/interface/speed` — no NetSpec-config lookup.
- **DOM is the only device-side addition.** Transceiver DOM isn't in `ietf-interfaces`; add one subscription against `openconfig-platform-transceiver` (confirmed in the device's NETCONF capabilities):

```
telemetry ietf subscription 211
 encoding encode-kvgpb
 filter xpath /components/component/transceiver
 stream yang-push
 update-policy periodic 3000        ! 30s; optical levels drift slowly
 receiver ip address <telegraf> 57500 protocol grpc-tcp
```

The existing subscription is left untouched; the transceiver sub rides the same Telegraf receiver and VM output.

Telegraf's `cisco_telemetry_mdt` input names measurements/fields off the YANG path, so emitted names won't match the schema out of the box — observe them once in vmui and pin them with a `[[processors.rename]]` block. That is the single empirical task in step 1.

## Metric schema & label contract

The write path is deliberately **dumb and low-cardinality**. Only intrinsic identity goes on the series; everything derived (role, alias, neighbor class, monitored flag) is joined at query time.

**Labels (write time):**

| Label | Example | Notes |
|---|---|---|
| `device` | `csw-mcd-01` | Hostname as telemetry reports it. Required. |
| `interface` | `GigabitEthernet1/0/1` | **Telemetry-native** name, stored as-is. Never normalized in Telegraf. |
| `lane` | `0`–`3` | Optics only; from `physical-channels/channel[index]`. The 4 true 40/100G QSFP report 4 lanes; SFP+ (incl. QSFP-to-SFP+ converters) report one. Omit for single-lane. |

**Explicitly NOT labels:** `role`, `neighbor_class`, `alias`, `monitored`, `desired_state`. These are query-time joins from NetSpec's rules engine.

**Interface metrics (counters unless noted):**

- `if_in_octets_total`, `if_out_octets_total` (64-bit HC counters)
- `if_in_errors_total`, `if_out_errors_total`, `if_in_discards_total`, `if_out_discards_total`
- `if_in_unicast_pkts_total`, `if_out_unicast_pkts_total`
- `if_oper_status` (gauge 0/1 — for uptime bars only; **NetSpec stays authoritative** for state)
- `if_speed_bps` (gauge — from `/interfaces-state/interface/speed`; needed to render %-utilization)

Utilization is derived at query time: `rate(if_*_octets_total) * 8 / if_speed_bps`. Raw counters are stored; rates are never precomputed into storage.

**Optics / DOM metrics (gauges):**

- `transceiver_rx_power_dbm`, `transceiver_tx_power_dbm`
- `transceiver_laser_bias_ma`
- `transceiver_temp_celsius`, `transceiver_voltage_volts`
- Warn/alarm thresholds if the model streams them; otherwise a static per-PID threshold table renders the reference lines.

Rx/tx power and laser bias are reliably present in the OpenConfig transceiver model. The revision here is 2018-05-15 under a Cisco switching deviation, so **temperature and supply voltage may be pruned** — verify on the box (`show interfaces transceiver detail` vs. a NETCONF get on the subtree). If OC comes up short, `Cisco-IOS-XE-transceiver-oper` is the native model carrying the full DOM set; the subscription path changes but nothing downstream does.

**Interface-name normalization** is the one subtle point. NetSpec already solves SNMP-vs-telemetry name drift. VM stores the telemetry-native form; NetSpecGraph owns the telemetry ↔ canonical mapping via the shared `internal/` normalizer whenever it joins metrics to NetSpec state or rules. Nothing else may attempt normalization, or the graphs will disagree with `/noc` about what a port is.

**Data semantics — gotchas that produce silently wrong numbers:**

- **Counter resets.** Device reload or a counter clear makes a monotonic series drop. Derive rates with MetricsQL `rate()`/`increase()` (reset-aware). If any rate is ever computed in Go, handle the decreasing-value case explicitly.
- **`if_speed_bps` can be 0 or absent** on SVIs, port-channels, and admin-down ports. Guard %-utilization against divide-by-zero; fall back to raw bps display rather than rendering `Inf`/`NaN`.
- **oper-status is an enum, not a bool.** `ietf-interfaces` oper-status is `up/down/testing/unknown/dormant/notPresent/lowerLayerDown`. Map `up → 1`, everything else → `0` for the uptime series; keep the mapping in one place.
- **Interface names contain `/`.** Routes carrying an interface must percent-encode/decode it (`GigabitEthernet1%2F0%2F1`), matching NetSpec's existing convention so deep-links line up.
- **New/absent series.** An interface in NetSpec config that telemetry hasn't reported yet must render as empty, not as a flat zero line (zero utilization and "no data" look different and mean different things).

**Cardinality budget (measured, not estimated):** the live NetSpec instance reports 35 devices / 617 monitored interfaces, ~30% optical (~185 fiber ports: 181 single-lane SFP+ plus 4 four-lane QSFP). That gives ~6,200 interface series (617 × ~10) + ~1,000 optic series ≈ **~7,500 active series** fleet-wide. Headroom to roughly double before any of the storage math needs revisiting.

## Storage & retention

Single-node VictoriaMetrics OSS, added as a compose service.

OSS gives **one global retention** and no downsampling (both are Enterprise features), so the "14 days hi-res then rollups" pattern isn't a config knob. The plan sidesteps it: keep **native resolution for ~13 months** (`-retentionPeriod=400d`), which covers one prior fall term for the baseline overlay.

At the measured ~7,500 series and a 10s cadence, 400 days is ~26 billion samples ≈ **~18 GB** at a conservative 0.7 B/sample, likely **~10 GB** in practice. That is small enough that a two-tier rollup setup would cost more operationally than it saves, and it needs no Enterprise license. Cadence is the only knob that moves this materially, and even 10s lands in the tens of GB.

Revisit only if multi-year downsampled retention becomes a hard requirement; that (not performance) is the single condition that would push toward ClickHouse/Timescale, which do tiered retention in OSS. Not a 1.0 concern.

## Seasonality & baseline model

Two independent pieces, both visual.

**Live band (auto-tracking).** A rolling **hour-of-week** aggregate: 168 buckets, computed over a **trailing ~3–4 week window** (`band_window`, configurable). Because the window trails, the band climbs with the fall ramp on its own, so it always means "normal for right now." Per bucket, render **p10/p90** (or median + IQR) as a shaded envelope behind the live line. Computed in Go on demand from a trailing-window query, bucketed, and cached briefly. No statistical rigor is needed because nothing pages off it.

- Window too short (~1 wk) → band chases the signal, nothing ever looks unusual.
- Window too long (a full term) → the summer→fall ramp smears. ~3–4 weeks is the balance.

**Bucket in site-local time, not UTC.** The 168 buckets key off day-of-week + hour-of-day as humans experience them (classes at 9am local). Bucketing in UTC smears each local hour across two buckets seasonally as DST shifts. Use a configurable IANA zone (default the site's, e.g. `America/Chicago`); the tz database handles the two DST discontinuities per year.

**Baseline overlay (cross-regime).** The band cannot answer "busy fall day vs. busy summer-gap day" — that's a deliberate comparison across regimes. So the user picks a comparison period (same ISO week last year, or a named baseline window) and NetSpecGraph queries VM for that historical range and renders it as a time-shifted ghost line on the current axis. This is what the 13-month retention buys.

## Query & enrichment layer

This is where "one source of truth for identity" lives. On load, NetSpecGraph imports NetSpec's config + rules loader and builds an in-memory index:

```
(device, canonical_interface) → { role, alias, neighbor_class, monitored, desired_state }
```

A user filter like "AP-uplink ports in building HB1" resolves through that index to a concrete set of `(device, telemetry_interface)` pairs, which becomes a MetricsQL label filter or an explicit series list. VM never sees a business-rule label; the rules engine stays defined once, in Go, and graphs agree with state by construction. At campus cardinality, even enumerating series explicitly per query is fine.

## UI

Server-rendered `html/template` + vanilla JS + **uPlot**, no build step — the same stack NetSpec's UI already uses. Adopt the `/noc` visual language so it reads as one product, and reuse `internal/auth` so login is shared. Deep-link both directions with NetSpec's `/device/{name}` pages.

1.0 views:

- **Per-interface / per-device:** utilization (bps or %), errors/discards, oper-status uptime bar, with the seasonality band behind the live line and an optional baseline overlay.
- **Per-optic DOM:** rx/tx power, temp, bias, voltage, with warn/alarm thresholds as reference lines.
- **Fleet/aggregate:** top-talkers and aggregate uplink utilization in the `/noc` idiom (a utilization-heat honeycomb is a natural reuse).

## Configuration surface

NetSpecGraph follows NetSpec's conventions (env for secrets/runtime, optional YAML in the config dir). Proposed knobs for 1.0 — a starting struct, not a final list:

- `vm_url` — e.g. `http://netspec-victoriametrics:8428`.
- `listen_addr` — host:port for the UI/API.
- `netspec_config_path` — read-only path to NetSpec's `config/` (device + rules identity).
- `timezone` — IANA zone for band bucketing (default site-local).
- `band_window` — trailing window for the live band (default `21d`).
- `band_percentiles` — envelope bounds (default `p10`/`p90`).
- `baseline_presets` — named comparison windows ("same ISO week last year", named baseline weeks).
- Auth: reuse NetSpec's `internal/auth` env (`NETSPEC_ADMIN_PASSWORD_HASH`, `NETSPEC_SESSION_SECRET`, `NETSPEC_API_TOKEN`) so login is shared.

## Deployment

Two new services on the existing `netspec` bridge in `docker-compose.yml`:

- `netspec-victoriametrics` — one container, `-retentionPeriod=400d`, a persistent volume under `${NETSPEC_DATA_DIR}`.
- `netspec-graph` — `cmd/netspecgraph`, mounts NetSpec `config/` **read-only** to share device + rules identity, published on its own `API_PORT`-style host port.

Same GHCR + multi-arch GitHub Actions pattern as NetSpec and the translator; in-module means one `docker compose pull` upgrades everything together. Telegraf gains the VM output; no other stack changes.

VM is an internal store: publish `:8428` to `127.0.0.1` only during bring-up (vmui), and not at all in production. NetSpecGraph is the sole VM client, reaching it over the `netspec` bridge.

## Resolved inputs

- **Fleet:** 35 devices / 617 monitored interfaces, ~185 optical (181 SFP+, 4 four-lane QSFP). → ~7,500 series.
- **Cadence:** 10s.
- **Retention:** 400d native, single global retention (~10–18 GB).
- **Interface counters + speed + oper-status:** already streaming via existing sub 251 (`ietf-interfaces` `/interfaces-state/interface`, periodic 10s). No metrics subscription needed; link speed sourced from telemetry.
- **DOM:** `openconfig-platform-transceiver` confirmed supported; needs its own subscription (211).

## Open questions

1. **DOM leaf coverage** — does the OC transceiver model expose temperature/voltage on this platform, or do we move DOM to `Cisco-IOS-XE-transceiver-oper`?
2. **Per-PID threshold table** — needed as a fallback for optics that don't stream warn/alarm thresholds?

## Rough build sequence

Each step lists its **done-when** so progress is verifiable, not vibes.

1. VM service + Telegraf VM output. *Done when:* vmui shows interface counters for a known device/interface with a non-zero `rate()` over 5 min. — **done** on lab (live IETF series + non-zero `in_octets` rates).
2. Freeze metric schema + label contract via the Telegraf rename processor; interface counters + oper-status first, optics second. *Done when:* the schema's exact metric names resolve in vmui and carry only `device`/`interface`/`lane` labels. — **done** for interface counters (optics TBD).
3. `cmd/netspecgraph` skeleton: reuse config/rules/normalizer/auth, VM query client, one per-interface uPlot page. *Done when:* the page renders real utilization + errors for one interface behind `internal/auth`, and `go build ./... && go test ./...` passes. — **done** (per-interface traffic/errors/oper uPlot; seasonality band is step 5).
4. Query-time enrichment: rules index + role/neighbor filters. *Done when:* a role filter (e.g. AP-uplinks) resolves to the correct series set, matching what `/noc` considers those ports — with a unit test on the join. — **done** (`internal/graph` Index via `rules.MatchDevice`/`MatchPort` + `ifname`; `GET /api/interfaces`, `GET /api/roles`, `/meta`; compose mounts `/data`).
5. Seasonality band + baseline overlay. *Done when:* the band renders in site-local buckets and a baseline preset overlays a prior period; band math has unit tests on synthetic series.
6. Optics/DOM page (needs subscription 211). *Done when:* per-lane rx/tx/bias render with threshold reference lines.
7. Fleet/aggregate view + NetSpec deep-links. *Done when:* deep-links round-trip with NetSpec's `/device/{name}` and interface-name encoding matches.
