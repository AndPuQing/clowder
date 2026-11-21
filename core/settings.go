package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Settings 是应用级配置，与具体设备无关。
type Settings struct {
	// Language 是界面语言标签；空表示跟随系统。
	Language string `json:"language,omitempty"`

	// CloseToTray 为真时关窗口只是隐藏（需要托盘）。
	CloseToTray bool `json:"closeToTray"`

	// SocksPort 是客户端本地 SOCKS5 的端口，默认 1080。
	SocksPort int `json:"socksPort,omitempty"`

	// DERPMapURL 覆盖默认的中继列表地址，空表示用官方默认。
	DERPMapURL string `json:"derpMapURL,omitempty"`

	// DERPMapRegion 指定优先使用的 DERP 区域 ID，0 表示自动挑最近的。
	DERPMapRegion int `json:"derpMapRegion,omitempty"`
}

// DefaultSettings 返回一份合理的默认值。
func DefaultSettings() Settings {
	return Settings{CloseToTray: true, SocksPort: 1080}
}

func settingsPath() (string, error) {
	base, err := ConfigDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(base, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(base, "settings.json"), nil
}

// LoadSettings 读取配置，文件不存在时返回默认值。
func LoadSettings() Settings {
	s := DefaultSettings()
	p, err := settingsPath()
	if err != nil {
		return s
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return s
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return DefaultSettings()
	}
	if s.SocksPort == 0 {
		s.SocksPort = 1080
	}
	return s
}

// SaveSettings 写回配置。
func SaveSettings(s Settings) error {
	p, err := settingsPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("写配置: %w", err)
	}
	return os.Rename(tmp, p)
}
