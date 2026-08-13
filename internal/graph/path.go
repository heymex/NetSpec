package graph

import (
	"net/url"
	"strings"
)

// parseDeviceInterfacePath extracts device + interface from paths like:
//
//	/device/{device}/interface/{iface}
//	/api/device/{device}/interface/{iface}/series
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
	if trimmed, okCut := strings.CutSuffix(ifaceEsc, "/series"); okCut {
		ifaceEsc = trimmed
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

func interfaceSeriesAPIPath(device, iface string) string {
	return "/api/device/" + url.PathEscape(device) + "/interface/" + url.PathEscape(iface) + "/series"
}
