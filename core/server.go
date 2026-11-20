package core

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/tailscale/tailcat"
)

// Server 把本机的 TCP 端口通过 tailcat 隧道暴露给拿着地址的人。
//
// 默认使用临时密钥：每次启动地址都会变，进程退出即失效。
type Server struct {
	ts  *tailcat.Server
	key *SavedKey // 非 nil 表示用了持久化密钥

	mu     sync.Mutex
	closed bool
}

// ServerOptions 是启动服务端的参数。
type ServerOptions struct {
	// Targets 把隧道里的端口映射到本机地址，例如
	//   map[uint16]string{8080: "127.0.0.1:8080"}
	// 未列出的端口一律拒绝（返回 nil handler，客户端收到 RST）。
	Targets map[uint16]string

	// Verbose 打开 tailcat 的调试日志。
	Verbose bool

	// KeyName 指定持久化密钥的名字。为空则用临时密钥——
	// 每次启动地址不同，进程退出即失效。
	KeyName string
}

// StartServer 启动服务端并连上 DERP 中继。
func StartServer(opts ServerOptions) (*Server, error) {
	if len(opts.Targets) == 0 {
		return nil, fmt.Errorf("至少需要一个要暴露的端口")
	}
	if opts.Verbose {
		tailcat.Verbose = true
	}

	var saved *SavedKey
	ts := &tailcat.Server{
		OnTCP: func(port uint16) func(net.Conn) {
			target, ok := opts.Targets[port]
			if !ok {
				return nil // 未暴露的端口：RST
			}
			return func(c net.Conn) {
				pipeToTarget(c, target)
			}
		},
	}

	if opts.KeyName != "" {
		k, _, err := LoadOrCreateKey(opts.KeyName)
		if err != nil {
			return nil, err
		}
		saved = k
		ts.Key = k.NodeKey
		ts.PresharedKey = k.PresharedKey
	}

	if err := ts.Start(); err != nil {
		return nil, fmt.Errorf("启动 tailcat 服务端: %w", err)
	}
	return &Server{ts: ts, key: saved}, nil
}

// Address 返回可以交给客户端的 tailcat 地址（tc... 开头）。
func (s *Server) Address() string {
	return string(s.ts.TailcatAddr())
}

// ListenAddr 返回服务端在隧道里的内部地址，仅用于日志。
func (s *Server) ListenAddr() string {
	return s.ts.Addr().String()
}

// PublicKey 返回服务端的节点公钥。
// 使用临时密钥时返回空字符串（公钥只存在于本次进程内）。
func (s *Server) PublicKey() string {
	if s.key != nil {
		return s.key.PublicKey()
	}
	return ""
}

// UsingSavedKey 报告是否用了持久化密钥。
func (s *Server) UsingSavedKey() bool {
	return s.key != nil
}

// Close 关闭服务端，地址随即失效。
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.ts.Close()
}

// pipeToTarget 把隧道来的一条连接接到本机 target 上，双向搬运字节。
func pipeToTarget(c net.Conn, target string) {
	defer c.Close()

	up, err := net.DialTimeout("tcp", target, 10*time.Second)
	if err != nil {
		return
	}
	defer up.Close()

	pipe(c, up)
}

// pipe 双向复制，任一侧结束后两边都关掉。
func pipe(a, b net.Conn) {
	var once sync.Once
	closeBoth := func() {
		once.Do(func() {
			a.Close()
			b.Close()
		})
	}

	done := make(chan struct{}, 2)
	go func() { io.Copy(a, b); done <- struct{}{} }()
	go func() { io.Copy(b, a); done <- struct{}{} }()

	<-done
	closeBoth()
	<-done
}
