package webui

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

func TestWorstSeverityPerDevice(t *testing.T) {
	t.Parallel()
	alerts := []HexAlert{
		{Device: "a", Severity: "warning"},
		{Device: "a", Severity: "critical"},
		{Device: "b", Severity: "info"},
	}
	got := WorstSeverityPerDevice(alerts)
	if got["a"] != "critical" {
		t.Fatalf("device a: want critical, got %q", got["a"])
	}
	if got["b"] != "info" {
		t.Fatalf("device b: want info, got %q", got["b"])
	}
}

func TestDisplayBucket(t *testing.T) {
	t.Parallel()
	if DisplayBucket("critical") != "critical" {
		t.Fatal()
	}
	if DisplayBucket("") != "ok" {
		t.Fatal()
	}
}

func TestBuildHexMapLayout_Empty(t *testing.T) {
	t.Parallel()
	layout := BuildHexMapLayout(nil, nil, DefaultHexRadius)
	if !layout.Empty || len(layout.Tiles) != 0 {
		t.Fatalf("expected empty layout")
	}
}

func TestBuildHexMapLayout_GridAndBBox(t *testing.T) {
	t.Parallel()
	names := []string{"z", "a", "b"}
	layout := BuildHexMapLayout(names, map[string]string{"b": "warning"}, DefaultHexRadius)
	if layout.Empty {
		t.Fatal("expected non-empty")
	}
	if len(layout.Tiles) != 3 {
		t.Fatalf("tiles: got %d", len(layout.Tiles))
	}
	// Sorted order: a, b, z
	if layout.Tiles[0].DeviceName != "a" || layout.Tiles[0].WorstSev != "ok" {
		t.Fatalf("first tile: %+v", layout.Tiles[0])
	}
	if layout.Tiles[1].DeviceName != "b" || layout.Tiles[1].WorstSev != "warning" {
		t.Fatalf("second tile: %+v", layout.Tiles[1])
	}
	if layout.ViewWidth <= 0 || layout.ViewHeight <= 0 {
		t.Fatalf("bad viewBox: %v x %v", layout.ViewWidth, layout.ViewHeight)
	}
}

func TestHexPathD_Closed(t *testing.T) {
	t.Parallel()
	d := HexPathD(10, 20, 5)
	if !strings.HasPrefix(d, "M ") || !strings.HasSuffix(d, "Z") {
		t.Fatalf("path: %q", d)
	}
}

func TestBuildHexMapLayout_Cap64(t *testing.T) {
	t.Parallel()
	names := make([]string, 70)
	for i := range names {
		names[i] = fmt.Sprintf("dev%d", i)
	}
	layout := BuildHexMapLayout(names, nil, 10)
	if len(layout.Tiles) != HexMaxTiles {
		t.Fatalf("want cap %d, got %d", HexMaxTiles, len(layout.Tiles))
	}
}

func TestHexPositions_Stagger(t *testing.T) {
	t.Parallel()
	layout := BuildHexMapLayout([]string{"d1", "d2"}, nil, DefaultHexRadius)
	if len(layout.Tiles) != 2 {
		t.Fatal()
	}
	w := DefaultHexRadius * math.Sqrt(3)
	// row 0 col 0 vs row 0 col 1
	x0 := layout.Tiles[0].CX
	x1 := layout.Tiles[1].CX
	if math.Abs((x1-x0)-w) > 1e-6 {
		t.Fatalf("horizontal spacing: %v vs want %v", x1-x0, w)
	}
}
