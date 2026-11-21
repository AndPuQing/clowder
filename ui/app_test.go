package ui

import (
	"testing"
	"time"

	"github.com/AndPuQing/clowder/core"
)

func TestParsePort(t *testing.T) {
	valid := map[string]uint16{
		"1":     1,
		"8080":  8080,
		" 443 ": 443,
		"65535": 65535,
	}
	for in, want := range valid {
		got, err := parsePort(in)
		if err != nil {
			t.Fatalf("parsePort(%q): %v", in, err)
		}
		if got != want {
			t.Fatalf("parsePort(%q) = %d, want %d", in, got, want)
		}
	}

	for _, in := range []string{"", "0", "65536", "-1", "abc", "80.5", "0x50"} {
		if _, err := parsePort(in); err == nil {
			t.Fatalf("parsePort(%q) should fail", in)
		}
	}
}

func TestParseExpose(t *testing.T) {
	cases := []struct {
		in     string
		port   uint16
		target string
	}{
		{"8080", 8080, "127.0.0.1:8080"},
		{"8080=127.0.0.1:18080", 8080, "127.0.0.1:18080"},
		{" 2222 = 127.0.0.1:22 ", 2222, "127.0.0.1:22"},
		{"9000=192.168.1.10:80", 9000, "192.168.1.10:80"},
	}
	for _, c := range cases {
		port, target, err := parseExpose(c.in)
		if err != nil {
			t.Fatalf("parseExpose(%q): %v", c.in, err)
		}
		if port != c.port || target != c.target {
			t.Fatalf("parseExpose(%q) = %d,%q; want %d,%q", c.in, port, target, c.port, c.target)
		}
	}

	for _, in := range []string{"", "abc", "8080=", "=127.0.0.1:1", "0", "70000"} {
		if _, _, err := parseExpose(in); err == nil {
			t.Fatalf("parseExpose(%q) should fail", in)
		}
	}
}

func TestParseForward(t *testing.T) {
	cases := []struct {
		in            string
		local, remote uint16
	}{
		{"9000:8080", 9000, 8080},
		{" 1 : 65535 ", 1, 65535},
	}
	for _, c := range cases {
		local, remote, err := parseForward(c.in)
		if err != nil {
			t.Fatalf("parseForward(%q): %v", c.in, err)
		}
		if local != c.local || remote != c.remote {
			t.Fatalf("parseForward(%q) = %d,%d; want %d,%d", c.in, local, remote, c.local, c.remote)
		}
	}

	for _, in := range []string{"", "9000", ":8080", "9000:", "a:b", "0:8080"} {
		if _, _, err := parseForward(in); err == nil {
			t.Fatalf("parseForward(%q) should fail", in)
		}
	}
}

func TestHardWrap(t *testing.T) {
	if got := hardWrap("abcdef", 0); got != "abcdef" {
		t.Fatalf("hardWrap n=0 = %q", got)
	}
	if got, want := hardWrap("abcdef", 2), "ab\ncd\nef"; got != want {
		t.Fatalf("hardWrap = %q, want %q", got, want)
	}
	if got, want := hardWrap("abcdef", 4), "abcd\nef"; got != want {
		t.Fatalf("hardWrap = %q, want %q", got, want)
	}
	if got, want := hardWrap("abc", 10), "abc"; got != want {
		t.Fatalf("hardWrap = %q, want %q", got, want)
	}
	// Multi-byte input must be wrapped by rune, not by byte.
	if got, want := hardWrap("éééé", 2), "éé\néé"; got != want {
		t.Fatalf("hardWrap = %q, want %q", got, want)
	}
}

func TestSplitKeys(t *testing.T) {
	got := splitKeys("nodekey:a, nodekey:b; nodekey:c\nnodekey:d")
	want := []string{"nodekey:a", "nodekey:b", "nodekey:c", "nodekey:d"}
	if len(got) != len(want) {
		t.Fatalf("splitKeys = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("splitKeys[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if got := splitKeys("  ,, ; \n "); len(got) != 0 {
		t.Fatalf("splitKeys(blank) = %v, want empty", got)
	}
}

func TestResizeDraftsKeepsPointers(t *testing.T) {
	first := new(string)
	*first = "8080"
	list := []*string{first}

	list = resizeDrafts(list, []string{"8080", "9090"})
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2", len(list))
	}
	if list[0] != first {
		t.Fatal("existing pointer was replaced")
	}
	if *list[1] != "9090" {
		t.Fatalf("new draft = %q, want 9090", *list[1])
	}

	list = resizeDrafts(list, []string{"1"})
	if len(list) != 1 || list[0] != first {
		t.Fatal("shrinking should keep the first pointer")
	}
}

func TestDeviceValuesDoesNotAlias(t *testing.T) {
	d := &core.Device{
		ID:             "d1",
		Name:           "host",
		Exposes:        []string{"8080"},
		Forwards:       []string{"9000:8080"},
		AllowedClients: []string{"nodekey:a"},
	}
	list := deviceValues([]*core.Device{d})
	if len(list) != 1 || list[0].ID != "d1" {
		t.Fatalf("deviceValues = %+v", list)
	}
	list[0].Name = "changed"
	list[0].Exposes[0] = "9090"
	list[0].Forwards[0] = "1:2"
	list[0].AllowedClients[0] = "nodekey:b"
	if d.Name != "host" || d.Exposes[0] != "8080" ||
		d.Forwards[0] != "9000:8080" || d.AllowedClients[0] != "nodekey:a" {
		t.Fatal("deviceValues must deep-copy, not alias")
	}
}

func TestRenameDevice(t *testing.T) {
	t.Setenv("CLOWDER_CONFIG_DIR", t.TempDir())
	d := &core.Device{ID: "d1", Name: "host", Role: core.RoleHost}
	a := &App{
		rt:      map[string]*runtime{},
		drafts:  map[string]*drafts{},
		devices: []*core.Device{d},
	}
	// renameDevice debounces a write. Stop it before the test ends so it
	// cannot fire during the next test, whose config directory differs.
	defer func() {
		a.mu.Lock()
		if a.saveTimer != nil {
			a.saveTimer.Stop()
		}
		a.mu.Unlock()
	}()
	dr := a.deviceDrafts(d)

	a.renameDevice(d, dr, "  build box  ")
	if d.Name != "build box" {
		t.Fatalf("name = %q, want %q", d.Name, "build box")
	}

	a.renameDevice(d, dr, "   ")
	if d.Name != "Untitled" {
		t.Fatalf("empty name = %q, want Untitled", d.Name)
	}
	if *dr.name != "Untitled" {
		t.Fatalf("draft = %q, want Untitled", *dr.name)
	}
}

func TestPersistIfChangedWritesDeviceChanges(t *testing.T) {
	t.Setenv("CLOWDER_CONFIG_DIR", t.TempDir())

	d := &core.Device{ID: "d1", Name: "host", Role: core.RoleHost, Exposes: []string{"8080"}}
	a := &App{
		rt:      map[string]*runtime{},
		drafts:  map[string]*drafts{},
		devices: []*core.Device{d},
	}
	a.savedDevices = marshalDevices(a.devices)
	a.savedSettings = marshalSettings(a.settings)

	// Widgets change bound values directly; the view notices afterwards.
	a.mu.Lock()
	d.UDP = true
	a.mu.Unlock()
	a.persistIfChanged()

	deadline := time.Now().Add(3 * time.Second)
	for {
		list, err := core.LoadDevices()
		if err == nil && len(list) == 1 && list[0].UDP {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("device change was not persisted (err=%v, list=%+v)", err, list)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestHumanSize(t *testing.T) {
	cases := map[int64]string{
		0:           "0 B",
		33:          "33 B",
		2048:        "2.0 KiB",
		1024 * 1024: "1.0 MiB",
	}
	for in, want := range cases {
		if got := humanSize(in); got != want {
			t.Fatalf("humanSize(%d) = %q, want %q", in, got, want)
		}
	}
}
