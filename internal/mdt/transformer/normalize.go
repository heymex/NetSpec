package transformer

import (
	"strings"
)

func NormOper(value string) string {
	if value == "" {
		return ""
	}
	s := strings.ToLower(strings.TrimSpace(value))
	switch s {
	case "up", "if-oper-state-ready", "if-oper-state-up", "if-up":
		return "up"
	case "down", "lower-layer-down", "lower_layer_down", "dormant", "not-present", "not_present",
		"if-oper-state-lower-layer-down", "if-oper-state-down", "if-oper-state-dormant",
		"if-oper-state-not-present", "if-down":
		return "down"
	default:
		return s
	}
}

func NormAdmin(value string) string {
	if value == "" {
		return ""
	}
	s := strings.ToLower(strings.TrimSpace(value))
	switch s {
	case "up", "enabled", "if-state-up", "if-admin-state-up", "if-admin-up":
		return "up"
	case "down", "disabled", "administratively-down", "if-state-down", "if-admin-state-down", "if-admin-down":
		return "down"
	default:
		return s
	}
}

func lookupStatus(fields map[string]string, short string) string {
	if v := strings.TrimSpace(fields[short]); v != "" {
		return v
	}
	suffix := "/" + short
	for k, v := range fields {
		if strings.HasSuffix(k, suffix) && strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
