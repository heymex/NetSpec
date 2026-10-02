package types

import (
	"github.com/netspec/netspec/internal/enrichment"
	"time"
)

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

	// Enriched contains contextual data from external sources (NetBox,
	// Taillight, Elastic). Set by the enrichment pipeline before the alert
	// is delivered to notifiers. Nil when enrichment is disabled or not
	// configured.
	Enriched *enrichment.EnrichedContext
}
