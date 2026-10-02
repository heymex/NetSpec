package enrichment

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// NetboxEnricher fetches device context from NetBox.
type NetboxEnricher struct {
	apiURL  string
	token   string
	client  *http.Client
	tags    []string
	logger  zerolog.Logger
}

// NewNetboxEnricher creates a new NetboxEnricher from the config.
func NewNetboxEnricher(cfg NetboxConfig, logger zerolog.Logger) *NetboxEnricher {
	if cfg.APIURL == "" {
		return nil
	}
	return &NetboxEnricher{
		apiURL: strings.TrimRight(cfg.APIURL, "/"),
		token:  cfg.ResolvedAPIToken(),
		client: &http.Client{Timeout: 10 * time.Second},
		tags:   cfg.Tags,
		logger: logger.With().Str("enricher", "netbox").Logger(),
	}
}

// Enrich fetches device data from NetBox.
func (e *NetboxEnricher) Enrich(ctx context.Context, deviceName string) (*NetboxContext, error) {
	if e == nil || e.token == "" {
		return nil, nil
	}

	deviceName = strings.TrimSpace(deviceName)
	if deviceName == "" {
		return nil, fmt.Errorf("netbox: no device name")
	}

	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	// Step 1: find the device in NetBox by name.
	devices, err := e.lookupDevices(ctx, deviceName)
	if err != nil {
		return nil, fmt.Errorf("netbox: lookup devices: %w", err)
	}
	if len(devices) == 0 {
		e.logger.Debug().Str("device", deviceName).Msg("netbox: device not found")
		return nil, nil
	}

	// Step 2: apply tag filter if configured.
	if len(e.tags) > 0 {
		devices = filterByTags(devices, e.tags)
	}
	if len(devices) == 0 {
		e.logger.Debug().Str("device", deviceName).Msg("netbox: no matching device after tag filter")
		return nil, nil
	}

	// Use the first matching device.
	best := devices[0]
	if len(devices) > 1 {
		best = pickBestMatch(devices, deviceName)
	}

	// Step 3: extract context.
	enrichCtx := &NetboxContext{
		DeviceName: deviceName,
		Role:       refName(best.Role),
		Site:       refName(best.Site),
		Rack:       refName(best.Rack),
		Tags:       tagNames(best.Tags, best.TagObjects),
	}

	// Extract management IP from interfaces or primary IP.
	for _, iface := range best.Interfaces {
		if len(iface.IPAddresses) > 0 {
			enrichCtx.ManagementIP = iface.IPAddresses[0].Address
			break
		}
	}
	if best.PrimaryIP != nil && enrichCtx.ManagementIP == "" {
		enrichCtx.ManagementIP = best.PrimaryIP.Address
	}
	if best.MANAGEMENT != nil {
		enrichCtx.ManagementIP = best.MANAGEMENT.Address
	}
	if best.Tenant != nil {
		enrichCtx.Tenant = best.Tenant.Name
	}

	e.logger.Debug().
		Str("device", deviceName).
		Str("site", enrichCtx.Site).
		Str("role", enrichCtx.Role).
		Msg("netbox: enrichment complete")

	return enrichCtx, nil
}

// netboxDevice is a minimal representation of a NetBox device response.
type netboxDevice struct {
	ID         int              `json:"id"`
	Name       string           `json:"name"`
	DisplayName string         `json:"display_name"`
	Role       *netboxRef       `json:"role"`
	Site       *netboxRef       `json:"site"`
	Rack       *netboxRef       `json:"rack"`
	Tenant     *netboxRef       `json:"tenant"`
	Interfaces []netboxInterface `json:"interfaces"`
	PrimaryIP  *netboxIP        `json:"primary_ip"`
	MANAGEMENT *netboxIP        `json:"management"` // some setups use this
	Tags       []string         `json:"tags"`
	// Tags as objects (NetBox v3+).
	TagObjects []netboxTagObj `json:"tags_obj"`
}

type netboxRef struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type netboxInterface struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	IPAddresses []netboxIP `json:"ip_addresses"`
}

type netboxIP struct {
	Address string `json:"address"`
}

type netboxTagObj struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type candidate struct {
	Device     netboxDevice
	Role       string
	Site       string
	Rack       string
	Management *netboxIP
	IP4        string
	Tenant     *netboxRef
	Tags       []string
}

// lookupDevices searches NetBox for devices matching the given name.
func (e *NetboxEnricher) lookupDevices(ctx context.Context, name string) ([]netboxDevice, error) {
	url := fmt.Sprintf("%s/api/dcim/devices/?name=%s&limit=10", e.apiURL, url.QueryEscape(name))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Token "+e.token)
	req.Header.Set("Accept", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("netbox API %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result struct {
		Count    int               `json:"count"`
		Next     *string           `json:"next"`
		Previous *string           `json:"previous"`
		Results  []netboxDevice    `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Results, nil
}

// pickBestMatch selects the best device from multiple NetBox results.
func pickBestMatch(devices []netboxDevice, targetName string) netboxDevice {
	best := devices[0]
	bestScore := 0

	targetLower := strings.ToLower(targetName)
	for _, d := range devices {
		score := 0
		if stringsEqualFold(d.Name, targetName) {
			score = 2
		}
		if strings.Contains(strings.ToLower(d.Name), targetLower) {
			score = 1
		}
		// Prefer exact match, then substring match.
		if score > bestScore {
			best = d
			bestScore = score
		}
	}
	return best
}

// parseNetboxDevices transforms raw devices into candidates with extracted fields.
func parseNetboxDevices(devices []netboxDevice) []candidate {
	cands := make([]candidate, 0, len(devices))
	for _, d := range devices {
		c := candidate{
			Device: d,
			Role:   refName(d.Role),
			Site:   refName(d.Site),
			Rack:   refName(d.Rack),
			Tenant: d.Tenant,
			Tags:   tagNames(d.Tags, d.TagObjects),
		}
		// Check for management IP on interfaces.
		for _, iface := range d.Interfaces {
			if len(iface.IPAddresses) > 0 {
				c.IP4 = iface.IPAddresses[0].Address
			}
		}
		// Check for management address on primary IP.
		if d.PrimaryIP != nil {
			c.Management = d.PrimaryIP
		}
		cands = append(cands, c)
	}
	return cands
}

// filterByTags keeps only devices that have at least one of the configured tags.
func filterByTags(devs []netboxDevice, required []string) []netboxDevice {
	out := make([]netboxDevice, 0, len(devs))
	for _, d := range devs {
		if deviceHasTag(d, required) {
			out = append(out, d)
		}
	}
	return out
}

func deviceHasTag(d netboxDevice, required []string) bool {
	tagSet := make(map[string]bool, len(d.Tags)+len(d.TagObjects))
	for _, t := range d.Tags {
		tagSet[t] = true
	}
	for _, t := range d.TagObjects {
		tagSet[t.Slug] = true
		tagSet[t.Name] = true
	}
	for _, r := range required {
		if tagSet[r] || tagSet[slugify(r)] {
			return true
		}
	}
	return false
}

func refName(r *netboxRef) string {
	if r == nil {
		return ""
	}
	return r.Name
}

func tagNames(tags []string, objs []netboxTagObj) []string {
	// Prefer objects (have slug), fall back to raw tags.
	if len(objs) > 0 {
		out := make([]string, len(objs))
		for i, o := range objs {
			out[i] = o.Name
		}
		return out
	}
	return tags
}

// -- Helpers --

func stringsEqualFold(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func slugify(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
