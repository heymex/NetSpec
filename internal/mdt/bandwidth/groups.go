// Package bandwidth maps MDT node_ids into operator-defined transport bandwidth groups.
package bandwidth

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const Ungrouped = "ungrouped"

// File is the YAML shape for NETSPEC_MDT_BANDWIDTH_GROUPS.
type File struct {
	Groups []Group `yaml:"groups"`
}

// Group is a named set of telemetry node_ids (devices).
type Group struct {
	Name    string   `yaml:"name"`
	Devices []string `yaml:"devices"`
}

// Registry resolves device → group. Devices not listed map to Ungrouped.
type Registry struct {
	deviceToGroup map[string]string
	names         []string
}

// LoadFile reads a bandwidth groups YAML file. An empty path returns an empty
// registry (all devices are Ungrouped).
func LoadFile(path string) (*Registry, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return NewRegistry(nil), nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("bandwidth groups %s: %w", path, err)
	}
	var f File
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("bandwidth groups %s: %w", path, err)
	}
	return NewRegistry(f.Groups), nil
}

// NewRegistry builds a lookup from group definitions. Later device entries win
// if a device appears in more than one group.
func NewRegistry(groups []Group) *Registry {
	r := &Registry{
		deviceToGroup: map[string]string{},
	}
	seen := map[string]struct{}{}
	for _, g := range groups {
		name := strings.TrimSpace(g.Name)
		if name == "" || name == Ungrouped {
			continue
		}
		if _, ok := seen[name]; !ok {
			seen[name] = struct{}{}
			r.names = append(r.names, name)
		}
		for _, d := range g.Devices {
			d = strings.TrimSpace(d)
			if d == "" {
				continue
			}
			r.deviceToGroup[d] = name
		}
	}
	return r
}

// GroupOf returns the configured group for device, or Ungrouped.
func (r *Registry) GroupOf(device string) string {
	device = strings.TrimSpace(device)
	if device == "" {
		return Ungrouped
	}
	if r == nil {
		return Ungrouped
	}
	if g, ok := r.deviceToGroup[device]; ok {
		return g
	}
	return Ungrouped
}

// Names returns configured group names (excluding the implicit ungrouped bucket).
func (r *Registry) Names() []string {
	if r == nil {
		return nil
	}
	out := make([]string, len(r.names))
	copy(out, r.names)
	return out
}
