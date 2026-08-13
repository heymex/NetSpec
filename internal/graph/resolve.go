package graph

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/netspec/netspec/internal/graph/vm"
	"github.com/netspec/netspec/internal/ifname"
)

// ifaceResolveCache maps device → telemetry interface names for a short TTL so
// short SNMP keys (Gi1/0/1) can be joined to MDT-native labels (GigabitEthernet…).
type ifaceResolveCache struct {
	mu    sync.Mutex
	byDev map[string]ifaceCacheEntry
}

type ifaceCacheEntry struct {
	names   []string
	expires time.Time
}

var telemetryIfaceCache = &ifaceResolveCache{byDev: map[string]ifaceCacheEntry{}}

const ifaceCacheTTL = 2 * time.Minute

// ResolveTelemetryInterface returns the VictoriaMetrics interface label that
// matches query (exact or ifname.Canonical). Falls back to query unchanged.
func ResolveTelemetryInterface(ctx context.Context, client *vm.Client, device, query string) (string, error) {
	if client == nil || device == "" || query == "" {
		return query, nil
	}
	names, err := telemetryIfaceCache.interfaces(ctx, client, device)
	if err != nil {
		return query, err
	}
	for _, n := range names {
		if n == query {
			return n, nil
		}
	}
	for _, n := range names {
		if ifname.Match(n, query) {
			return n, nil
		}
	}
	return query, nil
}

func (c *ifaceResolveCache) interfaces(ctx context.Context, client *vm.Client, device string) ([]string, error) {
	now := time.Now()
	c.mu.Lock()
	if e, ok := c.byDev[device]; ok && now.Before(e.expires) {
		names := e.names
		c.mu.Unlock()
		return names, nil
	}
	c.mu.Unlock()

	match := fmt.Sprintf(`{device="%s"}`, vm.EscapeLabel(device))
	names, err := client.LabelValues(ctx, "interface", match)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.byDev[device] = ifaceCacheEntry{names: names, expires: now.Add(ifaceCacheTTL)}
	c.mu.Unlock()
	return names, nil
}

// selectorForDeviceIface builds a MetricsQL selector using the telemetry-native
// interface label when it can be resolved.
func selectorForDeviceIface(ctx context.Context, client *vm.Client, device, iface string) (string, string, error) {
	resolved, err := ResolveTelemetryInterface(ctx, client, device, iface)
	if err != nil {
		return "", iface, err
	}
	return vm.Selector(device, resolved), resolved, nil
}
