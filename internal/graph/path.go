package graph

import (
	"net/url"
	"strings"
)

// parseDeviceInterfacePath extracts device + interface from paths like:
//
//	/device/{device}/interface/{iface}
//	/device/{device}/interface/{iface}/optics
//	/api/device/{device}/interface/{iface}/series
//	/api/device/{device}/interface/{iface}/meta
//	/api/device/{device}/interface/{iface}/optics/series
//
// iface may contain slashes (literal or as %2F). Prefer EscapedPath semantics
// by passing the escaped path string so %2F survives as one interface name.
func parseDeviceInterfacePath(escapedPath string) (device, iface string, ok bool) {
	path := escapedPath
	if path == "" {
		return "", "", false
	}
	if i := strings.Index(path, "/device/"); i >= 0 {
		path = path[i:]
	}
	rest, found := strings.CutPrefix(path, "/device/")
	if !found {
		return "", "", false
	}
	deviceEsc, after, found := strings.Cut(rest, "/interface/")
	if !found || deviceEsc == "" || after == "" {
		return "", "", false
	}
	ifaceEsc := after
	for _, suffix := range []string{"/optics/series", "/optics", "/series", "/meta"} {
		if trimmed, okCut := strings.CutSuffix(ifaceEsc, suffix); okCut {
			ifaceEsc = trimmed
			break
		}
	}
	if ifaceEsc == "" {
		return "", "", false
	}
	var err error
	device, err = url.PathUnescape(deviceEsc)
	if err != nil || device == "" {
		return "", "", false
	}
	iface, err = url.PathUnescape(ifaceEsc)
	if err != nil || iface == "" {
		return "", "", false
	}
	return device, iface, true
}

func interfacePagePath(device, iface string) string {
	return "/device/" + url.PathEscape(device) + "/interface/" + url.PathEscape(iface)
}

// InterfacePagePath is the Graph UI path for one interface (slashes → %2F).
func InterfacePagePath(device, iface string) string {
	return interfacePagePath(device, iface)
}

// AbsoluteInterfaceURL joins an optional Graph public origin with InterfacePagePath.
// Empty base yields "".
func AbsoluteInterfaceURL(base, device, iface string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" || device == "" || iface == "" {
		return ""
	}
	return base + interfacePagePath(device, iface)
}

// AbsoluteFleetDeviceURL joins an optional Graph public origin with /fleet?device=.
func AbsoluteFleetDeviceURL(base, device string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" || device == "" {
		return ""
	}
	return base + "/fleet?device=" + url.QueryEscape(device)
}

func opticsPagePath(device, iface string) string {
	return interfacePagePath(device, iface) + "/optics"
}

func interfaceSeriesAPIPath(device, iface string) string {
	return "/api/device/" + url.PathEscape(device) + "/interface/" + url.PathEscape(iface) + "/series"
}

func opticsSeriesAPIPath(device, iface string) string {
	return "/api/device/" + url.PathEscape(device) + "/interface/" + url.PathEscape(iface) + "/optics/series"
}

func pathIsOptics(escapedPath string) bool {
	return strings.Contains(escapedPath, "/optics")
}
