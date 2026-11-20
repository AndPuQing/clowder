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

// SavedKey 是一份持久化的节点身份。
//
// tailcat 的地址由「节点私钥 + 预共享密钥」决定，两者一起保存，
// 地址才能在重启后保持不变。
type SavedKey struct {
	Name         string               `json:"name"`
	NodeKey      key.NodePrivate      `json:"nodeKey"`
	PresharedKey tailcat.PresharedKey `json:"presharedKey"`
	CreatedAt    time.Time            `json:"createdAt"`
}

// PublicKey 返回对应的节点公钥，用于日志和客户端白名单。
func (k *SavedKey) PublicKey() string {
	return k.NodeKey.Public().String()
}

// ConfigDir 返回配置目录，遵循 XDG 规范。
func ConfigDir() (string, error) {
	if d := os.Getenv("CLOWDER_CONFIG_DIR"); d != "" {
		return d, nil
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "clowder"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("定位家目录: %w", err)
	}
	return filepath.Join(home, ".config", "clowder"), nil
}

// keysDir 返回密钥目录，并确保权限是 0700。
func keysDir() (string, error) {
	base, err := ConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "keys")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("创建密钥目录: %w", err)
	}
	return dir, nil
}

// keyPath 返回某个名字的密钥文件路径。
func keyPath(name string) (string, error) {
	if name == "" {
		name = "default"
	}
	if strings.ContainsAny(name, "/\\.") {
		return "", fmt.Errorf("密钥名不能包含路径分隔符或点: %q", name)
	}
	dir, err := keysDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name+".json"), nil
}

// GenerateKey 生成一份新密钥（不落盘前可先看地址）。
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

// Save 把密钥写入磁盘，权限 0600。
func (k *SavedKey) Save() error {
	p, err := keyPath(k.Name)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(k, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化密钥: %w", err)
	}
	// 先写临时文件再改名，避免写一半断电留下坏文件
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("写入密钥: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("落盘密钥: %w", err)
	}
	return nil
}

// LoadKey 读取一份已有密钥。
func LoadKey(name string) (*SavedKey, error) {
	p, err := keyPath(name)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("读取密钥 %q: %w", name, err)
	}
	var k SavedKey
	if err := json.Unmarshal(data, &k); err != nil {
		return nil, fmt.Errorf("解析密钥 %q: %w", name, err)
	}
	if k.Name == "" {
		k.Name = name
	}
	return &k, nil
}

// LoadOrCreateKey 读取密钥，不存在就生成并保存。
// 第二个返回值表示是否是新建的。
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

// ListKeys 列出磁盘上所有密钥名。
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

// DeleteKey 删除一份密钥。
func DeleteKey(name string) error {
	p, err := keyPath(name)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil {
		return fmt.Errorf("删除密钥 %q: %w", name, err)
	}
	return nil
}

// unwrapPathErr 把包了一层的错误还原成 *os.PathError，便于 os.IsNotExist 判断。
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
