package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tailscale/tailcat"
	"tailscale.com/types/key"
)

// SavedKey is a persistent node identity.
//
// A tailcat address is derived from the node private key and the
// pre-shared key, so both are stored together: keeping them makes the
// address stable across restarts.
type SavedKey struct {
	Name         string               `json:"name"`
	NodeKey      key.NodePrivate      `json:"nodeKey"`
	PresharedKey tailcat.PresharedKey `json:"presharedKey"`
	CreatedAt    time.Time            `json:"createdAt"`
}

// PublicKey returns the matching node public key, used for logs and for a
// host's client allow-list.
func (k *SavedKey) PublicKey() string {
	return k.NodeKey.Public().String()
}

// ConfigDir returns the configuration directory, following the XDG spec.
func ConfigDir() (string, error) {
	if d := os.Getenv("CLOWDER_CONFIG_DIR"); d != "" {
		return d, nil
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "clowder"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, ".config", "clowder"), nil
}

// keysDir returns the key directory, ensuring it is private (0700).
func keysDir() (string, error) {
	base, err := ConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "keys")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create key directory: %w", err)
	}
	return dir, nil
}

// keyPath returns the file path for a named key.
func keyPath(name string) (string, error) {
	if name == "" {
		name = "default"
	}
	if strings.ContainsAny(name, "/\\.") {
		return "", fmt.Errorf("key name must not contain a path separator or dot: %q", name)
	}
	dir, err := keysDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name+".json"), nil
}

// GenerateKey creates a new key without writing it to disk.
func GenerateKey(name string) (*SavedKey, error) {
	if name == "" {
		name = "default"
	}
	return &SavedKey{
		Name:         name,
		NodeKey:      key.NewNode(),
		PresharedKey: tailcat.NewPresharedKey(),
		CreatedAt:    time.Now().UTC(),
	}, nil
}

// Save writes the key to disk with mode 0600.
func (k *SavedKey) Save() error {
	p, err := keyPath(k.Name)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(k, "", "  ")
	if err != nil {
		return fmt.Errorf("encode key: %w", err)
	}
	// Write a temporary file and rename it, so a crash cannot leave a
	// half-written key behind.
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write key: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("store key: %w", err)
	}
	return nil
}

// LoadKey reads an existing key.
func LoadKey(name string) (*SavedKey, error) {
	p, err := keyPath(name)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("read key %q: %w", name, err)
	}
	var k SavedKey
	if err := json.Unmarshal(data, &k); err != nil {
		return nil, fmt.Errorf("parse key %q: %w", name, err)
	}
	if k.Name == "" {
		k.Name = name
	}
	return &k, nil
}

// LoadOrCreateKey reads a key, generating and saving one if it does not
// exist. The second result reports whether a new key was created.
func LoadOrCreateKey(name string) (*SavedKey, bool, error) {
	k, err := LoadKey(name)
	if err == nil {
		return k, false, nil
	}
	if !os.IsNotExist(unwrapPathErr(err)) {
		return nil, false, err
	}
	k, genErr := GenerateKey(name)
	if genErr != nil {
		return nil, false, genErr
	}
	if err := k.Save(); err != nil {
		return nil, false, err
	}
	return k, true, nil
}

// ListKeys returns the names of all stored keys.
func ListKeys() ([]string, error) {
	dir, err := keysDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".json"))
	}
	sort.Strings(names)
	return names, nil
}

// DeleteKey removes a stored key.
func DeleteKey(name string) error {
	p, err := keyPath(name)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil {
		return fmt.Errorf("delete key %q: %w", name, err)
	}
	return nil
}

// unwrapPathErr unwraps an error down to an *os.PathError so callers can
// use os.IsNotExist on it.
func unwrapPathErr(err error) error {
	for {
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return err
		}
		next := u.Unwrap()
		if next == nil {
			return err
		}
		err = next
	}
}
