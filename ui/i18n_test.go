package ui

import (
	"strings"
	"testing"
)

// TestPluralCounts checks the ICU plural in the catalogue, so a count of one
// never reads "1 machines".
func TestPluralCounts(t *testing.T) {
	initI18n()
	if catalog == nil {
		t.Fatal("catalogue did not load")
	}
	msg, _, ok := catalog.Message("en", "tray.idle")
	if !ok {
		t.Fatal("tray.idle is missing")
	}
	one := msg.Format("en", map[string]any{"count": 1})
	many := msg.Format("en", map[string]any{"count": 2})
	if !strings.Contains(one, "1 machine") || strings.Contains(one, "machines") {
		t.Fatalf("one: %q", one)
	}
	if !strings.Contains(many, "2 machines") {
		t.Fatalf("many: %q", many)
	}
}
