package ui

import (
	"bytes"
	"image/png"
	"testing"
)

// TestIconAssetsLoad parses every icon the interface uses. A broken SVG
// would otherwise only fail when a page is drawn.
func TestIconAssetsLoad(t *testing.T) {
	names := []string{
		"alert", "chevron-left", "chevron-right", "chevron-up", "connect",
		"copy", "download", "edit", "file", "folder", "globe", "hash", "host", "key",
		"mark", "network", "peer", "play", "plus", "radio", "refresh", "send",
		"settings", "share", "stop", "terminal", "trash", "tray", "x",
	}
	for _, name := range names {
		if icon(name) == nil {
			t.Errorf("icon %q did not load", name)
		}
	}
}

// TestTrayIcon is a PNG of the mark, at the size the tray asks for.
func TestTrayIcon(t *testing.T) {
	data := trayIcon()
	if len(data) == 0 {
		t.Fatal("trayIcon returned nothing")
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode tray icon: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 32 || b.Dy() != 32 {
		t.Fatalf("tray icon is %dx%d, want 32x32", b.Dx(), b.Dy())
	}
}
