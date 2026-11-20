package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Device 是一台被管理的机器。
//
// 两个方向对立看待：
//   - RoleHost：本机开票，把自己端口暴露出去
//   - RolePeer：拿着别人的票，把对方的端口映射到本机
type Device struct {
	ID   string `json:"id"`
	Name string `json:"name"`

	// Role 是 "host" 或 "peer"。
	Role string `json:"role"`

	// Addr 对 peer 是对方给的地址；对 host 运行后写入自己的地址。
	Addr string `json:"addr,omitempty"`

	// Exposes 是 host 要暴露的端口，形如 "8080" / "8080=127.0.0.1:8080"。
	Exposes []string `json:"exposes,omitempty"`

	// Forwards 是 peer 的端口映射，形如 "9000:8080"。
	Forwards []string `json:"forwards,omitempty"`

	// Persistent 为真时 host 使用固定密钥，地址跨重启稳定。
	Persistent bool `json:"persistent,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
}

const (
	RoleHost = "host"
	RolePeer = "peer"
)

// devicesPath 返回设备清单文件路径。
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

// LoadDevices 读取设备清单，文件不存在时返回空列表。
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
		return nil, fmt.Errorf("读取设备清单: %w", err)
	}
	var out []Device
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("解析设备清单: %w", err)
	}
	return out, nil
}

// SaveDevices 写回设备清单。
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
		return fmt.Errorf("写设备清单: %w", err)
	}
	return os.Rename(tmp, p)
}

// NewDeviceID 生成一个足够短的设备标识。
func NewDeviceID() string {
	return fmt.Sprintf("d%d", time.Now().UnixNano())
}