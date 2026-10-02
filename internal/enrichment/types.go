package enrichment

import "encoding/json"

// EnrichedContext is attached to a types.Alert when enrichment is enabled.
// Zero values on any field mean that source was either not configured,
// not enabled, or failed to return data — the alert fires normally either way.
type EnrichedContext struct {
	// Sources lists which enrichers returned data (e.g. ["netbox", "taillight"]).
	Sources []string `json:"sources,omitempty"`

	Netbox    *NetboxContext    `json:"netbox,omitempty"`
	Taillight *TaillightContext `json:"taillight,omitempty"`
	Elastic   *ElasticContext   `json:"elastic,omitempty"`

	// Raw is an extensible stash for future enrichers that don't need
	// a dedicated struct yet. Keyed by the enricher's Name().
	Raw map[string]json.RawMessage `json:"raw,omitempty"`
}

// HasAny reports whether at least one enricher returned data.
func (e *EnrichedContext) HasAny() bool {
	return e != nil && (len(e.Sources) > 0 || e.Netbox != nil || e.Taillight != nil || e.Elastic != nil)
}

// -- NetBox --

// NetboxContext holds device context from NetBox.
type NetboxContext struct {
	DeviceName   string   `json:"device_name"`
	Role         string   `json:"role"`
	Site         string   `json:"site"`
	Rack         string   `json:"rack"`
	ManagementIP string   `json:"management_ip"`
	Tenant       string   `json:"tenant"`
	Tags         []string `json:"tags,omitempty"`
}

// -- Taillight --

// TaillightContext holds Taillight enrichment results.
// Taillight is the data source; it may be reached via the Taillight HTTP API
// or via Elastic (backend is transparent to this struct).
type TaillightContext struct {
	// SyslogWindow is the direct syslog query mode — raw entries for the device
	// in the lookback window around the alert.
	SyslogWindow *SyslogWindow `json:"syslog_window,omitempty"`

	// DailySummary is the daily analysis/summary report for the device.
	DailySummary *DailySummary `json:"daily_summary,omitempty"`
}

// SyslogWindow contains raw log entries from a Taillight syslog query.
type SyslogWindow struct {
	Device    string         `json:"device"`
	Lookback  string         `json:"lookback"` // human-readable duration
	Entries   []SyslogEntry  `json:"entries,omitempty"`
	TotalHits int            `json:"total_hits"`
}

// SyslogEntry mirrors a Taillight netlog/srvlog event.
type SyslogEntry struct {
	Timestamp string `json:"timestamp"`
	Hostname  string `json:"hostname"`
	Facility  string `json:"facility,omitempty"`
	Severity  string `json:"severity,omitempty"`
	Program   string `json:"program,omitempty"`
	Message   string `json:"message"`
}

// DailySummary is the Taillight daily analysis report for a device.
type DailySummary struct {
	ReportDate   string         `json:"report_date"`
	TotalEvents  int            `json:"total_events"`
	ErrorCount   int            `json:"error_count"`
	WarningCount int            `json:"warning_count"`
	TopMessages  []MessageCount `json:"top_messages,omitempty"`
	HasAnomalies bool           `json:"has_anomalies"`
	SummaryText  string         `json:"summary_text,omitempty"`
}

// -- Elastic --

// ElasticContext holds enrichment results from a standalone Elastic query.
// This is used for cross-source correlation / trend analysis from data that
// lives in Elastic but may not originate from Taillight.
type ElasticContext struct {
	// Trend is a time-windowed volume and severity breakdown.
	Trend *TrendAnalysis `json:"trend,omitempty"`
}

// TrendAnalysis provides volume and severity data over a lookback window.
type TrendAnalysis struct {
	Window      string         `json:"window"`
	TotalHits   int            `json:"total_hits"`
	BySeverity  map[string]int `json:"by_severity,omitempty"`
	TopMessages []MessageCount `json:"top_messages,omitempty"`
}

// MessageCount is a {message, count} pair for top-N aggregations.
type MessageCount struct {
	Message string `json:"message"`
	Count   int    `json:"count"`
}

// SyslogFilter filters syslog results by severity or other criteria.
type SyslogFilter struct {
	Severity string
}

// QueryResult is the common return type for a Taillight query.
type QueryResult struct {
	Entries []SyslogEntry
	Total   int
}

// DailyResult is the result of a Taillight daily summary query.
type DailyResult struct {
	ReportDate   string
	TotalEvents  int
	ErrorCount   int
	WarningCount int
	TopMessages  []MessageCount
	HasAnomalies bool
	SummaryText  string
}
