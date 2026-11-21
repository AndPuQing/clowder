package ui

import (
	"embed"
	"io/fs"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
)

//go:embed catalog/*.json
var catalogFS embed.FS

// 界面语言：跟随系统。MyGo 的 i18n 会按 CLDR 的距离挑最合适的一种。
const (
	langSource = "en"
	langZh     = "zh-Hans"
)

var catalog *i18n.Catalog

// initI18n 读取语言目录，并把应用声明为「写了哪几种语言」。
// 必须在 Run 之前调用。
func initI18n() {
	sub, err := fs.Sub(catalogFS, "catalog")
	if err == nil {
		catalog, _ = i18n.NewCatalog(sub, langSource)
	}
	mygo.App.SetLanguages(langSource, langZh)
}

// T 按当前界面语言取一条文案。args 是 ICU 里 {name} 的取值，成对给出。
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

// IsEnglish 报告当前界面是否是英文，个别排版上的差异可以据此微调。
func IsEnglish() bool {
	return i18n.Base(i18n.Locale()) == "en"
}
