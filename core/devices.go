package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Device is one managed machine.
//
// Two roles read the same record from opposite sides:
//   - RoleHost shares ports from this machine
//   - RolePeer connects to someone else's shared address
type Device struct {
	ID   string `json:"id"`
	Name string `json:"name"`

	// Role is RoleHost or RolePeer.
	Role string `json:"role"`

	// Addr is the peer's address for RolePeer; a host writes its own
	// address here while sharing.
	Addr string `json:"addr,omitempty"`

	// Exposes lists the host's ports, as "8080" or "8080=127.0.0.1:8080".
	Exposes []string `json:"exposes,omitempty"`

	// Forwards lists the peer's port mappings, as "9000:8080".
	Forwards []string `json:"forwards,omitempty"`

	// Persistent keeps a saved key for the host, so the address survives
	// restarts.
	Persistent bool `json:"persistent,omitempty"`

	// UDP also relays UDP on the same port.
	UDP bool `json:"udp,omitempty"`

	// AllowProxy lets the peer reach addresses this machine can reach.
	AllowProxy bool `json:"allowProxy,omitempty"`

	// SSH opens an SSH service inside the tunnel so the peer can run
	// commands and, with libghostty, open an interactive terminal.
	SSH bool `json:"ssh,omitempty"`

	// Term opens an interactive terminal on the peer after connecting.
	Term bool `json:"term,omitempty"`

	// Files shares FileDir read-only over SFTP (host side). Requires SSH.
	Files bool `json:"files,omitempty"`

	// FileDir is the directory shared over SFTP.
	FileDir string `json:"fileDir,omitempty"`

	// BrowseFiles shows the host's shared files on the peer.
	BrowseFiles bool `json:"browseFiles,omitempty"`

	// Socks starts a local SOCKS5 proxy on the peer.
	Socks bool `json:"socks,omitempty"`

	// AllowedClients limits the host to these client public keys. Empty
	// means any client holding the address may connect.
	AllowedClients []string `json:"allowedClients,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
}

const (
	// RoleHost shares local ports.
	RoleHost = "host"
	// RolePeer connects to a shared address.
	RolePeer = "peer"
)

// devicesPath returns the device list path, creating its directory.
func devicesPath() (string, error) {
	base, err := ConfigDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(base, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(base, "devices.json"), nil
}

// LoadDevices reads the device list. A missing file yields no devices.
func LoadDevices() ([]Device, error) {
	p, err := devicesPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read device list: %w", err)
	}
	var out []Device
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parse device list: %w", err)
	}
	return out, nil
}

// SaveDevices writes the device list atomically.
func SaveDevices(list []Device) error {
	p, err := devicesPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write device list: %w", err)
	}
	return os.Rename(tmp, p)
}

// NewDeviceID returns an ID that is unique enough for a device list.
func NewDeviceID() string {
	return fmt.Sprintf("d%d", time.Now().UnixNano())
}
