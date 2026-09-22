package decoder

import "testing"

func TestClassifyPath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		path string
		kind string
	}{
		{"", "empty"},
		{"openconfig-interfaces:interfaces/interface", "interface"},
		{"Cisco-IOS-XE-interfaces-oper:interfaces/interface", "interface"},
		{"openconfig-interfaces:interfaces/interface/state/counters", "interface_counters"},
		{"Cisco-IOS-XE-interfaces-oper:interfaces/interface/statistics", "interface_counters"},
		{"Cisco-IOS-XE-transceiver-oper:transceiver-oper-data", "optics"},
		{"openconfig-platform:components/component/transceiver", "optics"},
		{"openconfig-lldp:lldp/interfaces/interface", "lldp"},
		{"Cisco-IOS-XE-device-hardware-oper:device-hardware-data", "hardware"},
		{"Cisco-IOS-XE-bgp-oper:bgp-state-data", "routing"},
		{"Cisco-IOS-XE-qos-oper:qos", "qos"},
		{"Cisco-IOS-XE-arp-oper:arp-data", "other"},
	}
	for _, tc := range cases {
		if got := ClassifyPath(tc.path); got != tc.kind {
			t.Errorf("%q: got %q want %q", tc.path, got, tc.kind)
		}
	}
}
