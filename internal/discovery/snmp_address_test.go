package discovery

import "testing"

func TestValidateAddress(t *testing.T) {
	t.Parallel()
	allow := []string{"10.0.0.1", "192.168.1.10", "2001:db8::1", "switch.lab.example.com", "core-sw01"}
	for _, addr := range allow {
		if err := validateAddress(addr); err != nil {
			t.Errorf("allow %q: %v", addr, err)
		}
	}
	reject := []string{
		"",
		"http://10.0.0.1",
		"10.0.0.1/24",
		"user@10.0.0.1",
		"169.254.169.254",
		"0.0.0.0",
		"224.0.0.1",
		"fe80::1",
	}
	for _, addr := range reject {
		if err := validateAddress(addr); err == nil {
			t.Errorf("expected reject for %q", addr)
		}
	}
}
