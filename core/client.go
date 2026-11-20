package core

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/tailscale/tailcat"
)

// Client 通过 tailcat 地址连到服务端，并把隧道里的端口映射到本机。
type Client struct {
	tc *tailcat.Client

	mu        sync.Mutex
	listeners []net.Listener
}

// ClientOptions 是创建客户端的参数。
type ClientOptions struct {
	// Addr 是服务端的 tailcat 地址。
	Addr string

	// KeyName 指定持久化密钥的名字。设置后客户端公钥固定，
	// 服务端才能用 AllowedClients 做白名单。
	KeyName string
}

// PublicKeyOf 返回指定名字的持久化密钥对应的公钥。
func PublicKeyOf(name string) (string, error) {
	k, err := LoadKey(name)
	if err != nil {
		return "", err
	}
	return k.PublicKey(), nil
}

// NewClientWithOptions 按选项创建客户端，支持持久化密钥。
func NewClientWithOptions(opts ClientOptions) (*Client, error) {
	c, err := NewClient(opts.Addr)
	if err != nil {
		return nil, err
	}
	if opts.KeyName != "" {
		k, _, err := LoadOrCreateKey(opts.KeyName)
		if err != nil {
			return nil, err
		}
		c.tc.Key = k.NodeKey
	}
	return c, nil
}

// NewClient 校验地址格式后创建客户端。此时不会建立连接，
// 隧道在第一次 Dial 时惰性建立。
func NewClient(addr string) (*Client, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil, fmt.Errorf("地址为空")
	}
	if !strings.HasPrefix(addr, "tc") {
		return nil, fmt.Errorf("不是 tailcat 地址（应以 tc 开头）: %.16s", addr)
	}
	return &Client{tc: tailcat.NewClient(tailcat.Addr(addr))}, nil
}

// Ping 测一次往返，用来确认地址是否还有效。
func (c *Client) Ping(timeout time.Duration) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	res, err := c.tc.Ping(ctx)
	if err != nil {
		return 0, err
	}
	return res.Latency, nil
}

// PublicKey 返回本客户端的节点公钥（服务端可据此做白名单）。
func (c *Client) PublicKey() string {
	return c.tc.PublicKey().String()
}

// Dial 通过隧道连到服务端的某个 TCP 端口。
func (c *Client) Dial(remotePort uint16, timeout time.Duration) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return c.tc.DialTCPPort(ctx, remotePort)
}

// Forward 在本机 listenAddr 上监听，把收到的连接转进隧道送到 remotePort。
// 返回的 listener 已经启动接受循环，调用 Close 收工。
func (c *Client) Forward(listenAddr string, remotePort uint16) (net.Listener, error) {
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("监听 %s: %w", listenAddr, err)
	}

	c.mu.Lock()
	c.listeners = append(c.listeners, ln)
	c.mu.Unlock()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go c.handle(conn, remotePort)
		}
	}()

	return ln, nil
}

// handle 为一条本地连接开一条隧道连接，然后双向搬运。
func (c *Client) handle(local net.Conn, remotePort uint16) {
	defer local.Close()

	remote, err := c.Dial(remotePort, 30*time.Second)
	if err != nil {
		return
	}
	defer remote.Close()

	pipe(local, remote)
}

// Close 关掉所有本地监听和隧道。
func (c *Client) Close() error {
	c.mu.Lock()
	lns := c.listeners
	c.listeners = nil
	c.mu.Unlock()

	for _, ln := range lns {
		ln.Close()
	}
	return c.tc.Close()
}
