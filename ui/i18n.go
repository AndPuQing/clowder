package ui

import (
	"embed"
	"io/fs"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"

	// Widget strings built into MyGo need the same locale data.
	_ "github.com/egoist/mygo/locales/zh-Hans"
)

//go:embed catalog/*.json
var catalogFS embed.FS

// Interface languages. English is the source language; MyGo's i18n picks
// the closest catalog to the system locale.
const (
	langSource = "en"
	langZh     = "zh-Hans"
)

var catalog *i18n.Catalog

// initI18n loads the message catalogs and declares which languages the
// application ships. It must run before Run.
func initI18n() {
	sub, err := fs.Sub(catalogFS, "catalog")
	if err == nil {
		catalog, _ = i18n.NewCatalog(sub, langSource)
	}
	mygo.App.SetLanguages(langSource, langZh)
}

// T returns a message in the current interface language. args are values
// for ICU placeholders, given as name/value pairs:
//
//	T("status.latency", "latency", "12ms")
func T(key string, kv ...any) string {
	if catalog == nil {
		return key
	}
	locale := i18n.Locale()
	m, _, ok := catalog.Message(locale, key)
	if !ok {
		return key
	}
	if len(kv) == 0 {
		return m.Format(locale, nil)
	}
	args := make(map[string]any, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		name, _ := kv[i].(string)
		args[name] = kv[i+1]
	}
	return m.Format(locale, args)
}
