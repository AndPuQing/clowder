package ui

import (
	"embed"
	"strings"
	"sync"

	myui "github.com/egoist/mygo/ui"
)

// Icons are real SVG files, parsed once and drawn as vectors. They take the
// colour of the text they sit next to, so one file works on every surface.
//
//go:embed icons/*.svg
var iconFS embed.FS

var (
	iconsOnce sync.Once
	icons     map[string]*myui.SVG
)

// icon returns a named icon, or nil when the asset is missing.
func icon(name string) *myui.SVG {
	iconsOnce.Do(func() {
		icons = make(map[string]*myui.SVG)
		entries, err := iconFS.ReadDir("icons")
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".svg") {
				continue
			}
			data, err := iconFS.ReadFile("icons/" + e.Name())
			if err != nil {
				continue
			}
			icons[strings.TrimSuffix(e.Name(), ".svg")] = myui.MustParseSVG(data)
		}
	})
	return icons[name]
}
