package types

import "time"

// Alert represents an active or resolved alert.
// State transitions: firing → acked → resolved, or firing → resolved.
type Alert struct {
	ID           string
	Device       string
	Entity       string
	AlertType    string
	Severity     string
	State        string // "firing", "acked", or "resolved"
	FiredAt      time.Time
	AckedAt      *time.Time
	AckedBy      string
	AckNote      string
	ResolvedAt   *time.Time
	Message      string
	RelatedState map[string]string

	// Slack ChatOps tracking — set when a Block Kit message is posted.
	SlackMsgTS     string
	SlackChannelID string
}
