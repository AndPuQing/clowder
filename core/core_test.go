package core

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// isolateConfig points the configuration directory at a temporary dir.
func isolateConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CLOWDER_CONFIG_DIR", dir)
	return dir
}

func TestKeyRoundTrip(t *testing.T) {
	dir := isolateConfig(t)

	k, created, err := LoadOrCreateKey("node-a")
	if err != nil {
		t.Fatalf("LoadOrCreateKey: %v", err)
	}
	if !created {
		t.Fatal("first call should create the key")
	}

	loaded, created, err := LoadOrCreateKey("node-a")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if created {
		t.Fatal("second call should reuse the key")
	}
	if got, want := loaded.PublicKey(), k.PublicKey(); got != want {
		t.Fatalf("public key changed: %s != %s", got, want)
	}

	info, err := os.Stat(filepath.Join(dir, "keys", "node-a.json"))
	if err != nil {
		t.Fatalf("stat key: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("key file mode = %o, want 600", perm)
	}

	names, err := ListKeys()
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if !reflect.DeepEqual(names, []string{"node-a"}) {
		t.Fatalf("ListKeys = %v", names)
	}

	if err := DeleteKey("node-a"); err != nil {
		t.Fatalf("DeleteKey: %v", err)
	}
	if _, err := LoadKey("node-a"); err == nil {
		t.Fatal("key should be gone")
	}
}

func TestKeyNameRejectsPathSeparators(t *testing.T) {
	isolateConfig(t)
	for _, name := range []string{"../escape", "a/b", "a.b", `a\b`} {
		if _, err := GenerateKey(name); err != nil {
			continue // generation itself does not validate
		}
		if _, err := keyPath(name); err == nil {
			t.Fatalf("keyPath(%q) should fail", name)
		}
	}
}

func TestDevicesRoundTrip(t *testing.T) {
	isolateConfig(t)

	want := []Device{{
		ID:             "d1",
		Name:           "This machine",
		Role:           RoleHost,
		Exposes:        []string{"8080", "2222=127.0.0.1:22"},
		UDP:            true,
		AllowProxy:     true,
		SSH:            true,
		Persistent:     true,
		AllowedClients: []string{"nodekey:abc"},
	}}
	if err := SaveDevices(want); err != nil {
		t.Fatalf("SaveDevices: %v", err)
	}

	got, err := LoadDevices()
	if err != nil {
		t.Fatalf("LoadDevices: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", got, want)
	}
}

func TestLoadDevicesMissingFile(t *testing.T) {
	isolateConfig(t)
	got, err := LoadDevices()
	if err != nil {
		t.Fatalf("LoadDevices: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no devices, got %v", got)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	isolateConfig(t)

	want := DefaultSettings()
	want.Language = "en"
	want.SocksPort = 1081
	want.DERPMapURL = "https://example.invalid/derp"
	want.DERPMapRegion = 7
	if err := SaveSettings(want); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	got := LoadSettings()
	if got != want {
		t.Fatalf("settings mismatch: got %+v want %+v", got, want)
	}
}

func TestSettingsDefaults(t *testing.T) {
	isolateConfig(t)
	got := LoadSettings()
	if got.SocksPort != 1080 {
		t.Fatalf("default SOCKS port = %d, want 1080", got.SocksPort)
	}
	if !got.CloseToTray {
		t.Fatal("close-to-tray should default to true")
	}
}

func TestNewClientRejectsBadAddress(t *testing.T) {
	for _, addr := range []string{"", "http://example.com", "  tc"} {
		if _, err := NewClient(addr); err == nil {
			t.Fatalf("NewClient(%q) should fail", addr)
		}
	}
}

func TestStartServerNeedsTargets(t *testing.T) {
	if _, err := StartServer(ServerOptions{}); err == nil {
		t.Fatal("StartServer without targets should fail before touching the network")
	}
}
