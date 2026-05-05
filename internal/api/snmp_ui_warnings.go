package api

import (
	"fmt"
	"strings"
	"time"

	"github.com/netspec/netspec/internal/config"
)

// SNMPUIWarning is shown as a banner in the Web UI when SNMP-related behavior needs operator visibility.
type SNMPUIWarning struct {
	Class string // "warning" or "info" (CSS modifier)
	Title string
	Body  string
}

func snmpUIWarnings(cfg *config.Config) []SNMPUIWarning {
	if cfg == nil {
		return nil
	}
	mode := strings.ToLower(strings.TrimSpace(cfg.DesiredState.Global.TelemetryMode))
	snmp := cfg.DesiredState.Global.SNMP
	vi := snmp.ValidationInterval
	if vi <= 0 {
		vi = cfg.DesiredState.Global.CollectionInterval
	}
	if vi <= 0 {
		vi = 10 * time.Second
	}
	var out []SNMPUIWarning
	switch mode {
	case "telemetry_ingest_push":
		if snmp.TelemetryFallbackEnabled {
			iv := snmp.TelemetryFallbackInterval
			if iv <= 0 {
				iv = 5 * time.Minute
			}
			out = append(out, SNMPUIWarning{
				Class: "warning",
				Title: "SNMP fallback polling is enabled",
				Body: fmt.Sprintf("NetSpec runs full SNMP polls on each device every %s in addition to push telemetry. This adds significant SNMP, CPU, and network load—use only as a safety net and prefer longer intervals on larger fleets. Set global.snmp.telemetry_fallback_enabled to false when telemetry coverage is reliable.", iv),
			})
		} else {
			out = append(out, SNMPUIWarning{
				Class: "info",
				Title: "SNMP is active alongside telemetry",
				Body: fmt.Sprintf("Reachability pings and per-event confirmations use SNMP (validation interval ~%s). Tiles stay non-green until SNMP contact is proven or telemetry drives a successful read.", vi),
			})
		}
	case "snmp_validate_only":
		out = append(out, SNMPUIWarning{
			Class: "info",
			Title: "SNMP-only monitoring",
			Body: fmt.Sprintf("All observed interface state comes from SNMP polling (~%s). Aggressive intervals or large fleets can overload devices—tune global.snmp and collection_interval accordingly.", vi),
		})
	}
	return out
}
