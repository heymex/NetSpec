package rules

import (
	"testing"

	"github.com/netspec/netspec/internal/config"
)

func TestGlobMatch(t *testing.T) {
	cases := []struct {
		pattern string
		s       string
		want    bool
	}{
		{"ap*", "ap-floor1-01", true},
		{"ap*", "AP-FLOOR1-01", true}, // case-insensitive
		{"ap*", "bldg-link-01", false},
		{"*nac access port*", "10G NAC ACCESS PORT", true},
		{"*nac access port*", "nac access port", true},
		{"*nac access port*", "regular port", false},
		{"t|*", "t|po1|csw-hcd-01:hu2/0/27|po31.", true},
		{"t|*", "bldg-uplink", false},
		{"bldg*", "bldg-link-01", true},
		{"po*", "po1", true},
		{"po*", "gi1/0/1", false},
		{"*", "anything", true},
		{"exact", "exact", true},
		{"exact", "Exact", true},
		{"exact", "inexact", false},
	}
	for _, c := range cases {
		got := GlobMatch(c.pattern, c.s)
		if got != c.want {
			t.Errorf("GlobMatch(%q, %q) = %v, want %v", c.pattern, c.s, got, c.want)
		}
	}
}

func TestParseTrunkDescription(t *testing.T) {
	cases := []struct {
		desc       string
		wantNil    bool
		wantLocal  string
		wantDevice string
		wantPort   string
		wantRemote string
	}{
		{
			desc:       "t|po1|csw-hcd-01:hu2/0/27|po31.",
			wantLocal:  "po1",
			wantDevice: "csw-hcd-01",
			wantPort:   "hu2/0/27",
			wantRemote: "po31",
		},
		{
			desc:       "t|po2|dsw-main-01:te1/0/1|po5",
			wantLocal:  "po2",
			wantDevice: "dsw-main-01",
			wantPort:   "te1/0/1",
			wantRemote: "po5",
		},
		{desc: "regular port description", wantNil: true},
		{desc: "ap-floor1-01", wantNil: true},
		{desc: "t|only|three", wantNil: true},
	}
	for _, c := range cases {
		got := ParseTrunkDescription(c.desc)
		if c.wantNil {
			if got != nil {
				t.Errorf("ParseTrunkDescription(%q) = %+v, want nil", c.desc, got)
			}
			continue
		}
		if got == nil {
			t.Errorf("ParseTrunkDescription(%q) = nil, want non-nil", c.desc)
			continue
		}
		if got.LocalPortChannel != c.wantLocal {
			t.Errorf("ParseTrunkDescription(%q).LocalPortChannel = %q, want %q", c.desc, got.LocalPortChannel, c.wantLocal)
		}
		if got.RemoteDevice != c.wantDevice {
			t.Errorf("ParseTrunkDescription(%q).RemoteDevice = %q, want %q", c.desc, got.RemoteDevice, c.wantDevice)
		}
		if got.RemotePort != c.wantPort {
			t.Errorf("ParseTrunkDescription(%q).RemotePort = %q, want %q", c.desc, got.RemotePort, c.wantPort)
		}
		if got.RemotePortChannel != c.wantRemote {
			t.Errorf("ParseTrunkDescription(%q).RemotePortChannel = %q, want %q", c.desc, got.RemotePortChannel, c.wantRemote)
		}
	}
}

func TestMatchDevice(t *testing.T) {
	roles := []config.DeviceRole{
		{Prefix: "csw", Name: "Core Switch"},
		{Prefix: "dsw", Name: "Distribution Switch"},
		{Prefix: "asw", Name: "Access Switch"},
	}
	cases := []struct {
		hostname   string
		wantPrefix string
	}{
		{"dsw-mcnorth-01", "dsw"},
		{"DSW-MCNORTH-01", "dsw"}, // case-insensitive
		{"csw-hcd-01", "csw"},
		{"asw-floor2-01", "asw"},
		{"unknown-sw-01", ""},
	}
	for _, c := range cases {
		got := MatchDevice(c.hostname, roles)
		if c.wantPrefix == "" {
			if got != nil {
				t.Errorf("MatchDevice(%q) = %v, want nil", c.hostname, got.Prefix)
			}
		} else {
			if got == nil || got.Prefix != c.wantPrefix {
				prefix := ""
				if got != nil {
					prefix = got.Prefix
				}
				t.Errorf("MatchDevice(%q) = %q, want %q", c.hostname, prefix, c.wantPrefix)
			}
		}
	}
}
