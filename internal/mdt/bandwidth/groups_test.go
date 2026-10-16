package bandwidth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewRegistryGroupOf(t *testing.T) {
	t.Parallel()
	r := NewRegistry([]Group{
		{Name: "remote_branches", Devices: []string{"br-sw-01", " br-sw-02 "}},
		{Name: "campus_core", Devices: []string{"csw-01"}},
		{Name: "", Devices: []string{"ignored"}},
		{Name: Ungrouped, Devices: []string{"also-ignored"}},
	})
	if got := r.GroupOf("br-sw-01"); got != "remote_branches" {
		t.Fatalf("br-sw-01: %q", got)
	}
	if got := r.GroupOf("br-sw-02"); got != "remote_branches" {
		t.Fatalf("br-sw-02: %q", got)
	}
	if got := r.GroupOf("csw-01"); got != "campus_core" {
		t.Fatalf("csw-01: %q", got)
	}
	if got := r.GroupOf("unknown-sw"); got != Ungrouped {
		t.Fatalf("unknown: %q", got)
	}
	if got := r.GroupOf(""); got != Ungrouped {
		t.Fatalf("empty: %q", got)
	}
	names := r.Names()
	if len(names) != 2 || names[0] != "remote_branches" || names[1] != "campus_core" {
		t.Fatalf("names: %+v", names)
	}
}

func TestLoadFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "groups.yaml")
	if err := os.WriteFile(path, []byte(`
groups:
  - name: wan
    devices: [a, b]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if r.GroupOf("a") != "wan" || r.GroupOf("c") != Ungrouped {
		t.Fatalf("lookup failed: a=%q c=%q", r.GroupOf("a"), r.GroupOf("c"))
	}
	empty, err := LoadFile("")
	if err != nil || empty.GroupOf("x") != Ungrouped {
		t.Fatalf("empty path: %+v %v", empty, err)
	}
}

func TestDeviceLaterGroupWins(t *testing.T) {
	t.Parallel()
	r := NewRegistry([]Group{
		{Name: "first", Devices: []string{"sw-01"}},
		{Name: "second", Devices: []string{"sw-01"}},
	})
	if got := r.GroupOf("sw-01"); got != "second" {
		t.Fatalf("got %q", got)
	}
}
