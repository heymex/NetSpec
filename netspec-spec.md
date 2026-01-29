# NetSpec: Declarative Network State Monitor

## Executive Summary

**NetSpec** is a next-generation, declarative network monitoring system designed for environments where *state correctness matters more than metrics*. Unlike traditional discovery-and-poll systems, NetSpec operates on a "desired state" paradigm: you define what your network *should* look like, and NetSpec alerts you instantly when reality diverges from intent.

Built on gNMI streaming telemetry for sub-second detection, NetSpec decouples alerting from metrics storage—ensuring you know about problems *before* dashboards update. It's containerized, Linux-native, and integrates with Apprise for notification flexibility across 100+ channels.

---

## The Problem: Why Existing Tools Fall Short

### Pain Points from the Field

Research into user feedback across Zabbix, LibreNMS, SolarWinds NPM, and similar tools reveals consistent frustrations:

**1. Complexity vs. Value Mismatch**
- Zabbix users frequently cite a "steep learning curve" and being "too complex for small to medium setups"
- One Zabbix forum user complained of crashes every 2-3 days and documentation that "sucks"
- LibreNMS struggles when users move beyond pure SNMP/network monitoring: "the further away you move from this, the more jank you encounter"

**2. Polling Latency Creates Blind Spots**
- Standard SNMP polling intervals of 5-10 minutes mean issues can exist for minutes before detection
- One case study found a firewall CPU issue went undetected because 5-minute polling missed intermittent spikes; switching to 30-second polling revealed problems immediately
- Users report "delays caused by stale metrics" as a critical issue

**3. Alert Fatigue and Misconfiguration**
- EMA research shows up to 80% of IT alerts provide little to no operational value
- More than 75% of MSPs experience alert fatigue at least once per month
- Out-of-the-box thresholds don't know what "normal" looks like—yet teams assume tools will "magically figure that out"

**4. Cost and Resource Overhead**
- SolarWinds NPM: Initial costs range from $2,000 to $200,000 with 20% annual maintenance
- "It's not cheap to buy, not cheap for maintenance, and not cheap to run"
- Open-source alternatives often require significant expertise to deploy and maintain

**5. Discovery-First Philosophy is Backwards**
- Traditional tools discover what exists, then try to figure out what matters
- Network engineers already *know* what matters—they built it
- Discovery-based approaches create noise: monitoring things that don't need monitoring while missing things that do

### The Gap in the Market

There is no widely-adopted tool that:
1. Uses gNMI/streaming telemetry as the primary data source (most are SNMP-first)
2. Operates on declarative desired-state definitions
3. Prioritizes alerting speed over visualization
4. Focuses on *state correctness* rather than utilization metrics
5. Is lightweight, containerized, and easy to deploy

---

## NetSpec: Design Philosophy

### Core Principles

1. **Declarative Over Discovery**: You define what your network should look like. NetSpec validates reality against your intent.

2. **State Over Metrics**: Traditional monitors ask "how much bandwidth is used?" NetSpec asks "is this port-channel in the state I expect?"

3. **Alert First, Store Later**: Detection and notification happen on the critical path. Metrics storage is async and secondary.

4. **Speed Matters**: gNMI streaming telemetry enables sub-second detection of state changes—orders of magnitude faster than SNMP polling.

5. **Flexibility in Notification**: Apprise integration provides 100+ notification channels out of the box—Slack, Teams, PagerDuty, OpsGenie, email, SMS, and more.

### Target Environment

- Modern Cisco IOS-XE infrastructure (17.x recommended for full gNMI support)
- Less than 500 devices, ~10 interfaces of interest per device
- Relatively static environments where desired state changes monthly, not daily
- Teams who know what they want to monitor and don't need auto-discovery

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           FAST PATH (Alerting)                          │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│   ┌──────────────┐    ┌──────────────────┐    ┌──────────────────┐     │
│   │              │    │                  │    │                  │     │
│   │  gNMI        │───▶│  State           │───▶│  Alert Engine    │───▶ Apprise
│   │  Collector   │    │  Evaluator       │    │  (Stateful)      │     │
│   │              │    │                  │    │                  │     │
│   └──────────────┘    └──────────────────┘    └──────────────────┘     │
│          │                                                              │
│          │ (async)                                                      │
├──────────┼──────────────────────────────────────────────────────────────┤
│          │                    SLOW PATH (Metrics)                       │
│          ▼                                                              │
│   ┌──────────────┐    ┌──────────────────┐    ┌──────────────────┐     │
│   │              │    │                  │    │                  │     │
│   │  Metrics     │───▶│  VictoriaMetrics │───▶│  Grafana         │     │
│   │  Writer      │    │  (Storage)       │    │  (Visualization) │     │
│   │              │    │                  │    │                  │     │
│   └──────────────┘    └──────────────────┘    └──────────────────┘     │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### Component Responsibilities

**gNMI Collector**
- Maintains persistent gRPC streams to all targets
- Subscribes to relevant OpenConfig paths with on-change notifications
- Handles connection failures, reconnection, and health monitoring
- Exposes collector health metrics

**State Evaluator**
- Compares incoming telemetry against YAML-defined desired state
- Maintains current state cache for context and related-state correlation
- Triggers Alert Engine immediately on state mismatch
- Tracks state history for flap detection

**Alert Engine**
- Manages stateful alerts (fires on transition, clears on recovery)
- Implements deduplication and suppression windows
- Handles escalation tiers with configurable delays
- Provides maintenance window support
- Generates contextual alert messages with related state information

**Metrics Writer (Async)**
- Receives telemetry data via internal queue
- Writes to VictoriaMetrics in Prometheus exposition format
- Decoupled from alerting path—can fall behind without affecting detection

**VictoriaMetrics**
- Chosen for superior storage efficiency (2.5x less disk than Prometheus)
- 7x less CPU and 4x less memory than Prometheus for equivalent workloads
- Native Prometheus compatibility for Grafana integration

---

## Desired State Configuration (YAML)

### Device and Interface State

```yaml
# /config/desired-state.yaml

global:
  default_credentials: vault://network/gnmi-creds
  gnmi_port: 9339
  collection_interval: 10s  # For metrics; state uses on-change subscriptions

devices:
  core-sw-stack:
    address: 10.0.0.1
    description: "Core switch stack - Building A MDF"
    credentials_ref: core_creds  # Override default
    
    interfaces:
      # Port-channel monitoring with member requirements
      Port-channel1:
        description: "Uplink to Distribution"
        desired_state: up
        admin_state: enabled  # Expect it to be admin-up
        members:
          required:
            - GigabitEthernet1/0/49
            - GigabitEthernet1/0/50
            - GigabitEthernet2/0/49
            - GigabitEthernet2/0/50
        member_policy:
          mode: all_active  # or min_active: 2, or per_stack_minimum: 1
        alerts:
          member_down: critical
          channel_down: critical
          admin_down: warning  # Different severity for intentional shutdown
          
      # Simple interface monitoring
      GigabitEthernet1/0/1:
        description: "Server Room UPS"
        desired_state: up
        admin_state: enabled
        alerts:
          state_mismatch: warning
          
    # BGP neighbor discovery and monitoring
    bgp:
      mode: discover  # Learn neighbors, alert on state change
      expected_state: established
      alerts:
        state_change: critical
        new_neighbor: info
        
    # HSRP with explicit active/standby assignment
    hsrp:
      Vlan100:
        group: 1
        expected_role: active
        alerts:
          role_change: critical
          
    # Hardware health
    hardware:
      fans:
        mode: fault  # Alert on any non-OK status
        alerts:
          fault: warning
          
      temperature:
        thresholds:
          warning: 45
          critical: 55
        alerts:
          threshold_exceeded: warning  # Uses threshold name as severity
          
      power:
        mode: fault
        redundancy: required  # Alert if redundancy is lost
        alerts:
          fault: critical
          redundancy_lost: warning

  # Additional device
  dist-sw-01:
    address: 10.0.0.10
    description: "Distribution switch - Building A IDF-1"
    
    interfaces:
      Port-channel10:
        description: "Downlink to Access"
        desired_state: up
        members:
          required:
            - GigabitEthernet1/0/49
            - GigabitEthernet1/0/50
        member_policy:
          mode: min_active
          minimum: 1
          
    hsrp:
      Vlan100:
        group: 1
        expected_role: standby  # This one should be standby
```

### Credentials Configuration

```yaml
# /config/credentials.yaml (or reference external vault)

credentials:
  default:
    username: gnmi-monitor
    password_env: GNMI_DEFAULT_PASSWORD  # Reference environment variable
    
  core_creds:
    username: gnmi-admin
    password_vault: vault://secrets/network/core-switch
```

### Alert Configuration

```yaml
# /config/alerts.yaml

channels:
  ops-slack:
    type: apprise
    url_env: APPRISE_SLACK_WEBHOOK
    severity_filter: [warning, critical]
    
  ops-teams:
    type: apprise
    url_env: APPRISE_TEAMS_WEBHOOK
    severity_filter: [critical]
    
  opsgenie-critical:
    type: apprise
    url_env: APPRISE_OPSGENIE_URL
    severity_filter: [critical]
    escalation_delay: 600  # Only notify after 10 minutes unresolved

  email-noc:
    type: apprise
    url_env: APPRISE_EMAIL_URL
    severity_filter: [warning, critical]

alert_rules:
  # Default routing
  default:
    channels: [ops-slack]
    
  # Severity-based routing
  critical:
    channels: [ops-slack, ops-teams, opsgenie-critical]
    
  warning:
    channels: [ops-slack]
    
  info:
    channels: [ops-slack]
    
alert_behavior:
  deduplication_window: 300  # 5 minutes
  flap_detection:
    enabled: true
    threshold: 3  # 3 state changes
    window: 300   # within 5 minutes
    action: suppress_and_notify
    
  state_persistence:
    enabled: true
    path: /data/state.json
    on_restart: warn_unknown  # Send warning if state unknown after restart

message_templates:
  interface_down: |
    🔴 **Interface Down**: {{ device }}
    Interface: {{ interface }} ({{ description }})
    Expected: {{ expected_state }} | Actual: {{ actual_state }}
    {{ if related_state }}
    Related: {{ related_state }}
    {{ end }}
    
  interface_recovered: |
    🟢 **Interface Recovered**: {{ device }}
    Interface: {{ interface }} ({{ description }})
    Down duration: {{ duration }}
    
  bgp_state_change: |
    ⚠️ **BGP State Change**: {{ device }}
    Neighbor: {{ neighbor_address }} (AS {{ remote_as }})
    Previous: {{ previous_state }} | Current: {{ current_state }}
    
  hsrp_role_change: |
    🔶 **HSRP Role Change**: {{ device }}
    Interface: {{ interface }} Group: {{ group }}
    Expected: {{ expected_role }} | Actual: {{ actual_role }}
```

### Maintenance Windows

```yaml
# /config/maintenance.yaml

maintenance_windows:
  - name: weekly-core-maintenance
    devices: [core-sw-stack]
    schedule:
      type: recurring
      day: sunday
      start: "02:00"
      end: "04:00"
      timezone: America/Chicago
    suppress_alerts: true
    
  - name: building-a-upgrade
    devices: [core-sw-stack, dist-sw-01]
    schedule:
      type: one-time
      start: "2024-02-15T22:00:00-06:00"
      end: "2024-02-16T06:00:00-06:00"
    suppress_alerts: true
    notify_on_start: true
    notify_on_end: true
```

---

## gNMI Configuration Prerequisites

### Cisco IOS-XE gNMI Setup

NetSpec requires gNMI to be enabled on target devices. Here's a reference configuration for IOS-XE 17.x:

```
! Enable gNMI server
gnmi-yang
 gnmi-yang server
 gnmi-yang port 9339
 
! Optional: Enable TLS (recommended for production)
gnmi-yang secure-server
gnmi-yang secure-trustpoint <trustpoint-name>
gnmi-yang secure-port 9339

! Create dedicated monitoring user
username gnmi-monitor privilege 15 secret <password>

! Enable NETCONF/YANG (required for OpenConfig models)
netconf-yang
```

### Verify gNMI is Working

```bash
# Using gnmic (https://gnmic.openconfig.net/)
gnmic -a 10.0.0.1:9339 -u gnmi-monitor -p <password> --insecure \
  capabilities

# Subscribe to interface state
gnmic -a 10.0.0.1:9339 -u gnmi-monitor -p <password> --insecure \
  subscribe --path "/interfaces/interface/state"
```

### OpenConfig Paths Used

NetSpec subscribes to these OpenConfig paths:

```yaml
# Interface operational state
/interfaces/interface[name=*]/state/oper-status
/interfaces/interface[name=*]/state/admin-status

# LAG/Port-channel members
/interfaces/interface[name=*]/aggregation/state/member
/interfaces/interface[name=*]/aggregation/state/lag-type

# BGP neighbors
/network-instances/network-instance/protocols/protocol/bgp/neighbors/neighbor/state

# HSRP (Cisco-specific model)
/Cisco-IOS-XE-hsrp-oper:hsrp-oper-data/hsrp-entry

# Hardware sensors
/components/component[class=FAN]/state
/components/component[class=TEMPERATURE_SENSOR]/state
/components/component[class=POWER_SUPPLY]/state
```

---

## Docker Compose Deployment

```yaml
# docker-compose.yml

version: '3.8'

services:
  netspec:
    image: netspec:latest
    container_name: netspec
    restart: unless-stopped
    volumes:
      - ./config:/config:ro
      - ./data:/data
    environment:
      - GNMI_DEFAULT_PASSWORD=${GNMI_DEFAULT_PASSWORD}
      - APPRISE_SLACK_WEBHOOK=${APPRISE_SLACK_WEBHOOK}
      - APPRISE_TEAMS_WEBHOOK=${APPRISE_TEAMS_WEBHOOK}
      - APPRISE_OPSGENIE_URL=${APPRISE_OPSGENIE_URL}
      - APPRISE_EMAIL_URL=${APPRISE_EMAIL_URL}
      - LOG_LEVEL=info
      - METRICS_ENABLED=true
      - METRICS_PUSH_URL=http://victoriametrics:8428/api/v1/write
    ports:
      - "8080:8080"  # Health/status API
    depends_on:
      - victoriametrics
      - apprise
    networks:
      - monitoring

  victoriametrics:
    image: victoriametrics/victoria-metrics:latest
    container_name: victoriametrics
    restart: unless-stopped
    volumes:
      - vmdata:/victoria-metrics-data
    command:
      - "-storageDataPath=/victoria-metrics-data"
      - "-retentionPeriod=90d"
      - "-httpListenAddr=:8428"
    ports:
      - "8428:8428"
    networks:
      - monitoring

  grafana:
    image: grafana/grafana:latest
    container_name: grafana
    restart: unless-stopped
    volumes:
      - grafana-data:/var/lib/grafana
      - ./grafana/provisioning:/etc/grafana/provisioning:ro
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=${GRAFANA_ADMIN_PASSWORD}
    ports:
      - "3000:3000"
    depends_on:
      - victoriametrics
    networks:
      - monitoring

  apprise:
    image: lscr.io/linuxserver/apprise-api:latest
    container_name: apprise
    restart: unless-stopped
    environment:
      - PUID=1000
      - PGID=1000
      - TZ=America/Chicago
    volumes:
      - ./apprise-config:/config
    ports:
      - "8000:8000"
    networks:
      - monitoring

volumes:
  vmdata:
  grafana-data:

networks:
  monitoring:
    driver: bridge
```

### Environment File

```bash
# .env

# gNMI Credentials
GNMI_DEFAULT_PASSWORD=your-secure-password

# Apprise Notification URLs
APPRISE_SLACK_WEBHOOK=slack://tokenA/tokenB/tokenC
APPRISE_TEAMS_WEBHOOK=msteams://...
APPRISE_OPSGENIE_URL=opsgenie://apikey/...
APPRISE_EMAIL_URL=mailto://user:pass@smtp.example.com

# Grafana
GRAFANA_ADMIN_PASSWORD=your-grafana-password
```

---

## Operational Features

### Self-Monitoring

NetSpec monitors its own health and reports issues:

```yaml
# Built-in self-monitoring alerts
self_monitoring:
  telemetry_gap:
    description: "No telemetry received from device"
    threshold: 60s
    severity: warning
    
  connection_failure:
    description: "gNMI connection failed"
    severity: critical
    
  config_validation_error:
    description: "Invalid desired-state configuration"
    severity: critical
    
  high_event_rate:
    description: "Unusually high state change rate (possible flapping)"
    threshold: 100/minute
    severity: warning
```

### Configuration Validation

On startup and config reload, NetSpec validates:
- YAML syntax
- Device reachability (optional ping check)
- Interface/entity existence via gNMI get
- Credential validity

```bash
# Validate configuration without starting
docker exec netspec netspec validate --config /config/desired-state.yaml

# Force configuration reload
docker exec netspec netspec reload
```

### API Endpoints

```
GET  /health           # Service health check
GET  /status           # Current state summary
GET  /alerts           # Active alerts
GET  /alerts/history   # Alert history
POST /reload           # Reload configuration
GET  /metrics          # Prometheus-format metrics (self-monitoring)
```

---

## Market Analysis and Differentiation

### Competitive Positioning

| Feature | NetSpec | Zabbix | LibreNMS | SolarWinds NPM |
|---------|----------|--------|----------|----------------|
| Primary Protocol | gNMI | SNMP | SNMP | SNMP |
| Detection Speed | Sub-second | Minutes | Minutes | Minutes |
| Configuration Model | Declarative | Discovery | Discovery | Discovery |
| Alert-First Architecture | Yes | No | No | No |
| Complexity | Low | High | Medium | High |
| Cost | Open Source | Open Source | Open Source | $2K-$200K |
| Setup Time | Hours | Days-Weeks | Days | Days-Weeks |

### Target Users

**Primary**: Network engineers in SMB and mid-market enterprises who:
- Have modern Cisco IOS-XE infrastructure
- Value correctness over exhaustive metrics
- Are frustrated with alert fatigue from traditional tools
- Want sub-second detection without enterprise costs

**Secondary**: 
- MSPs managing multiple customer networks
- DevOps teams needing network state verification
- Organizations with compliance requirements for state monitoring

### Market Demand Signals

1. **Streaming telemetry adoption is accelerating**: Netflix, Yahoo, and Nokia have all released open-source gNMI tools, indicating industry movement away from SNMP

2. **GitOps/IaC mindset is spreading to networking**: Tools like ArgoCD have proven the value of declarative desired-state configuration; network teams want similar approaches

3. **Alert fatigue is a recognized crisis**: With 80% of alerts providing no value, there's clear demand for tools that alert on what matters

4. **Traditional tools are aging**: Zabbix's complexity and LibreNMS's SNMP-centricity leave a gap for modern, focused solutions

---

## Appendix: Claude Code Instruction Set

The following section provides the complete instruction set for Claude Code or similar AI agents to implement NetSpec.

---

# Claude Code Instructions: NetSpec Implementation

## Project Overview

You are implementing **NetSpec**, a declarative network state monitoring system. The user has provided detailed requirements through a Q&A process. This instruction set captures all decisions and requirements.

## Technology Stack

- **Language**: Go (Golang) - chosen for gRPC/protobuf support, performance, and single-binary deployment
- **Primary Protocol**: gNMI (gRPC Network Management Interface) for streaming telemetry
- **Configuration Format**: YAML exclusively
- **Alerting**: Apprise library/API integration
- **Metrics Storage**: VictoriaMetrics (Prometheus-compatible)
- **Containerization**: Docker Compose
- **Target Platform**: Linux (Ubuntu 24.04 LTS recommended)

## Core Architecture Requirements

### 1. Two-Path Processing

**CRITICAL**: Implement strict separation between alerting and metrics paths.

```
gNMI Stream → State Evaluator → Alert Engine → Apprise (FAST PATH)
           ↘ Metrics Queue → VictoriaMetrics (SLOW PATH)
```

- Alert path MUST NOT wait for metrics writes
- Use Go channels with buffering for async metrics delivery
- If metrics queue fills, drop metrics (not alerts)

### 2. gNMI Collector

**Requirements**:
- Use `github.com/openconfig/gnmi` as the base library
- Implement on-change subscriptions for state monitoring (fast path)
- Implement periodic sampling for metrics collection (slow path, configurable interval)
- Handle connection failures with exponential backoff
- Support TLS with certificate validation (make insecure mode available for testing)
- Expose collector health as Prometheus metrics

**Subscriptions to implement**:
```go
// State subscriptions (on-change)
"/interfaces/interface/state/oper-status"
"/interfaces/interface/state/admin-status"
"/interfaces/interface/aggregation/state"
"/network-instances/network-instance/protocols/protocol/bgp/neighbors/neighbor/state"

// Cisco-specific (IOS-XE)
"/Cisco-IOS-XE-hsrp-oper:hsrp-oper-data"

// Hardware (OpenConfig)
"/components/component/state" // filtered by class: FAN, POWER_SUPPLY, TEMPERATURE_SENSOR
```

### 3. State Evaluator

**Responsibilities**:
- Load and validate desired-state YAML
- Compare incoming telemetry against desired state
- Maintain current state cache for context
- Calculate related state information
- Track state change history for flap detection

**Port-Channel Logic**:
```go
type PortChannelPolicy struct {
    Mode string // "all_active", "min_active", "per_stack_minimum"
    Minimum int
    PerStackMinimum int
}

// Evaluation must handle:
// 1. Channel itself down
// 2. Member count below threshold
// 3. Specific required members down
// 4. Stack distribution requirements (e.g., at least one from each stack member)
```

**Admin-Down Handling**:
```go
// If admin_status changes to "DOWN" (intentional shutdown):
// - Generate WARNING severity alert (not critical)
// - Include context: "Interface administratively disabled"
// - Do NOT generate oper-status DOWN alert (expected consequence)
```

### 4. Alert Engine

**Stateful Alert Model**:
```go
type Alert struct {
    ID            string
    Device        string
    Entity        string    // interface name, BGP neighbor, etc.
    AlertType     string    // "interface_down", "bgp_state_change", etc.
    Severity      string    // "info", "warning", "critical"
    State         string    // "firing", "resolved"
    FiredAt       time.Time
    ResolvedAt    *time.Time
    Message       string
    RelatedState  map[string]string
    Acknowledged  bool
    Suppressed    bool
    SuppressionReason string
}
```

**Alert Lifecycle**:
1. State mismatch detected → Check deduplication window → Create alert
2. Alert created → Apply severity routing → Send to channels
3. State recovers → Update alert to "resolved" → Send recovery notification
4. If escalation configured → Start escalation timer → Escalate if unresolved

**Deduplication**:
- Same (device, entity, alert_type) within deduplication_window (default 5 min) = skip
- Implemented via in-memory map with TTL cleanup

**Flap Detection**:
```go
type FlapDetector struct {
    threshold int           // Number of state changes
    window    time.Duration // Time window
    history   map[string][]time.Time // Key: device+entity
}

// If flapping detected:
// 1. Suppress individual alerts
// 2. Send single "flapping detected" alert
// 3. Continue monitoring, send "flapping stopped" when stable
```

**Escalation**:
```go
type Escalation struct {
    Delay    time.Duration
    Channels []string
}

// Implementation:
// 1. Start goroutine with timer on alert fire
// 2. If alert still firing after delay, send to escalation channels
// 3. Cancel timer if alert resolves
```

**Maintenance Windows**:
```go
type MaintenanceWindow struct {
    Name      string
    Devices   []string
    Schedule  Schedule
    Active    bool
}

// Before generating alert:
// 1. Check if device is in active maintenance window
// 2. If yes, suppress alert and set SuppressionReason
// 3. Log suppressed alert for audit
```

### 5. BGP Monitoring (Discover Mode)

Since user wants discovery mode for BGP:

```go
// On first telemetry:
// 1. Store discovered neighbors in state cache
// 2. Expected state: Established (from config)

// On subsequent telemetry:
// 1. If neighbor state changes from Established → alert
// 2. If new neighbor appears → info alert "New BGP neighbor discovered"
// 3. If neighbor disappears → critical alert "BGP neighbor lost"
```

### 6. HSRP Monitoring (Explicit Role)

```go
type HSRPConfig struct {
    Interface    string
    Group        int
    ExpectedRole string // "active" or "standby"
}

// Alert triggers:
// 1. Actual role != expected role → critical
// 2. State not "Active" or "Standby" (e.g., "Init", "Learn") → critical
```

### 7. Hardware Monitoring (Hybrid)

**Fans**: Fault-based (on/off)
```go
// Alert if status != "OK"
```

**Temperature**: Threshold-based
```go
type TempThresholds struct {
    Warning  float64
    Critical float64
}

// Check against thresholds, use threshold name as severity
```

**Power**: Fault + Redundancy
```go
// Alert if:
// 1. Any PSU status != "OK" → critical
// 2. Redundancy lost (only one PSU operational) → warning
```

### 8. Configuration Validation

On load/reload:
```go
func ValidateConfig(cfg *Config) error {
    // 1. YAML syntax (handled by parser)
    
    // 2. Logical validation:
    //    - Referenced credentials exist
    //    - Port-channel members are valid interface names
    //    - Severity values are valid
    //    - Threshold values are sane
    
    // 3. Optional device validation (if enabled):
    //    - gNMI connection test
    //    - Interface existence check via gNMI Get
    
    return nil // or error with specific details
}
```

### 9. State Persistence

```go
type PersistedState struct {
    Devices   map[string]DeviceState
    UpdatedAt time.Time
}

// On startup:
// 1. Load persisted state if exists
// 2. Compare with incoming telemetry
// 3. If state unknown (file missing/corrupt), send WARNING:
//    "NetSpec restarted, state unknown for device X, re-learning..."
// 4. Only alert on NEW state changes, not on re-learning existing state
```

### 10. Configuration Reload

Support two modes:
1. **Manual**: API call to `/reload` endpoint
2. **File watch**: Optional, can be enabled in config

```go
// Reload process:
// 1. Parse new config
// 2. Validate new config (fail fast if invalid)
// 3. Diff against current config
// 4. Apply changes:
//    - New devices: add gNMI subscriptions
//    - Removed devices: close connections
//    - Changed devices: update state evaluator
// 5. Log changes
```

### 11. Self-Monitoring

Implement internal health checks:

```go
type HealthStatus struct {
    Status    string            // "healthy", "degraded", "unhealthy"
    Details   map[string]string
    Metrics   map[string]float64
}

// Checks:
// 1. gNMI connection status per device
// 2. Last telemetry received timestamp per device
// 3. Alert queue depth
// 4. Metrics queue depth
// 5. Memory usage
// 6. Goroutine count
```

Expose as Prometheus metrics AND send alerts for issues.

### 12. Apprise Integration

```go
import "github.com/caronc/apprise-api" // or HTTP client to Apprise API

func SendAlert(alert *Alert, channels []Channel) error {
    // For each channel:
    // 1. Render message template with alert data
    // 2. POST to Apprise API endpoint
    // 3. Handle failures with retry
    
    return nil
}
```

**Message Template Rendering**:
Use Go's `text/template` with alert struct as data.

### 13. Metrics Export

Export to VictoriaMetrics via Prometheus remote write protocol:

```go
// Metrics to export:
// - Interface state (gauge: 1=up, 0=down)
// - BGP neighbor state (gauge)
// - Hardware sensor values
// - NetSpec internal metrics (collector latency, alert count, etc.)

// Use labels:
// - device
// - interface (or entity)
// - description
```

### 14. Logging

Use structured logging (zerolog or zap):

```go
log.Info().
    Str("device", device).
    Str("interface", iface).
    Str("state", state).
    Msg("Interface state change detected")
```

Levels: debug, info, warn, error

### 15. Project Structure

```
netspec/
├── cmd/
│   └── netspec/
│       └── main.go
├── internal/
│   ├── collector/
│   │   └── gnmi.go
│   ├── evaluator/
│   │   ├── evaluator.go
│   │   ├── interface.go
│   │   ├── bgp.go
│   │   ├── hsrp.go
│   │   └── hardware.go
│   ├── alerter/
│   │   ├── engine.go
│   │   ├── dedup.go
│   │   ├── flap.go
│   │   └── escalation.go
│   ├── notifier/
│   │   └── apprise.go
│   ├── metrics/
│   │   └── writer.go
│   ├── config/
│   │   ├── loader.go
│   │   └── validator.go
│   ├── state/
│   │   └── persistence.go
│   └── api/
│       └── server.go
├── config/
│   ├── desired-state.yaml
│   ├── alerts.yaml
│   └── maintenance.yaml
├── Dockerfile
├── docker-compose.yml
├── go.mod
└── README.md
```

### 16. Testing Requirements

- Unit tests for state evaluation logic
- Integration tests with mock gNMI server
- End-to-end tests with containerized test environment

### 17. Documentation Requirements

Generate:
- README with quick start
- Configuration reference
- Alerting playbook (what each alert means, how to respond)
- Grafana dashboard JSON

---

## Implementation Order

1. **Phase 1**: Core collector and state evaluator
   - gNMI connection handling
   - Interface state evaluation
   - Basic alerting (no escalation)
   
2. **Phase 2**: Full alerting
   - Apprise integration
   - Deduplication
   - Flap detection
   - Escalation
   
3. **Phase 3**: Extended monitoring
   - BGP
   - HSRP
   - Hardware
   
4. **Phase 4**: Operational features
   - State persistence
   - Configuration reload
   - Self-monitoring
   - Metrics export
   
5. **Phase 5**: Production readiness
   - Docker packaging
   - Documentation
   - Grafana dashboards

---

## User-Specific Decisions Summary

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Protocol | gNMI only | SNMP too slow for sub-second detection |
| Config format | YAML only | User preference |
| BGP monitoring | Discover mode | Easier to maintain |
| HSRP monitoring | Explicit role | User knows expected state |
| Admin-down handling | Lower severity alert | Intentional != emergency |
| On restart | Warn and re-learn | Avoid alert storm |
| Config reload | Manual + optional file watch | User wanted explicit reload option |
| YAML validation | Before apply | Fail fast on errors |
| Self-monitoring | Yes + Prometheus metrics | Important for reliability |
| Metrics storage | VictoriaMetrics | Efficiency, lower resource usage |
| Distributed deployment | Optional | User open to it if needed |

---

*This instruction set provides complete context for implementing NetSpec. Reference specific sections as needed during development.*
