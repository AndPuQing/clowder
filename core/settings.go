package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Settings holds application-wide preferences that do not belong to a
// single device.
type Settings struct {
	// Language is an interface language tag; empty means "follow the
	// system language".
	Language string `json:"language,omitempty"`

	// CloseToTray hides the window instead of quitting when it is closed.
	// Requires a working tray.
	CloseToTray bool `json:"closeToTray"`

	// SocksPort is the local SOCKS5 port used by peers. Defaults to 1080.
	SocksPort int `json:"socksPort,omitempty"`

	// DERPMapURL overrides the default relay map URL.
	DERPMapURL string `json:"derpMapURL,omitempty"`

	// DERPMapRegion pins a preferred relay region ID; 0 picks the nearest.
	DERPMapRegion int `json:"derpMapRegion,omitempty"`
}

// DefaultSettings returns sensible defaults.
func DefaultSettings() Settings {
	return Settings{CloseToTray: true, SocksPort: 1080}
}

// settingsPath returns the settings file path, creating its directory.
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

// LoadSettings reads the settings file. It returns defaults when the file
// is missing or unreadable.
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

// SaveSettings writes the settings file atomically.
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
		return fmt.Errorf("write settings: %w", err)
	}
	return os.Rename(tmp, p)
}
