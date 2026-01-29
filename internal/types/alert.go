package types

import "time"

// Alert represents an active or resolved alert
type Alert struct {
	ID          string
	Device      string
	Entity      string
	AlertType   string
	Severity    string
	State       string // "firing" or "resolved"
	FiredAt     time.Time
	ResolvedAt  *time.Time
	Message     string
	RelatedState map[string]string
}
