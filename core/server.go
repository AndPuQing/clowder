package core

import (
	"fmt"
	"io"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/tailscale/tailcat"
	"go4.org/mem"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
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

	// UDP 为真时同时转发 UDP（用同样的端口号）。
	UDP bool

	// AllowProxy 为真时允许对方经本机访问其它地址（内网 / 任意站点）。
	AllowProxy bool

	// AllowedClients 是允许连接的客户端公钥（nodekey:...），为空则不限制。
	AllowedClients []string

	// DERPMapURL 覆盖默认中继列表地址。
	DERPMapURL string

	// RegionID 指定优先使用的中继区域。
	RegionID int

	// SSH 为真时在隧道里开放 SSH（端口 22），对方可拿到 shell。
	SSH bool

	// SSHFileDir 非空时开放 SFTP，目录为根，只读。
	SSHFileDir string

	// SSHAuthorizedKeys 是允许登录的 OpenSSH 公钥，空表示只靠隧道身份。
	SSHAuthorizedKeys []string
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
	var sshOnce sync.Once
	var sshHandler func(net.Conn)

	var ts *tailcat.Server
	ts = &tailcat.Server{
		OnTCP: func(port uint16) func(net.Conn) {
			if opts.SSH && port == SSHPort {
				sshOnce.Do(func() {
					so := tailcat.SSHOptions{
						Shell:          true,
						AuthorizedKeys: opts.SSHAuthorizedKeys,
					}
					if opts.SSHFileDir != "" {
						so.Files = &tailcat.FileService{Dir: opts.SSHFileDir, Mode: tailcat.FileServeRO}
					}
					sshHandler = ts.SSHConnHandler(so)
				})
				return sshHandler
			}
			target, ok := opts.Targets[port]
			if !ok {
				return nil // 未暴露的端口：RST
			}
			return func(c net.Conn) {
				pipeToTarget(c, target)
			}
		},
	}

	if opts.DERPMapURL != "" {
		ts.DERPMapURL = opts.DERPMapURL
	}
	if opts.RegionID > 0 {
		ts.RegionID = tailcfg.DERPRegionID(opts.RegionID)
	}

	if opts.UDP {
		ts.OnUDP = func(port uint16) func(tailcat.ConnPacketConn) {
			target, ok := opts.Targets[port]
			if !ok {
				return nil
			}
			return func(conn tailcat.ConnPacketConn) {
				pipeUDPToTarget(conn, target)
			}
		}
	}

	if opts.AllowProxy {
		// 对方可以指定任意目标地址，由本机替它拨号
		ts.AllowProxy = func(netip.AddrPort) bool { return true }

		ts.OnTCPForward = func(ap netip.AddrPort) func(net.Conn) {
			return func(c net.Conn) {
				up, err := net.DialTimeout("tcp", ap.String(), 10*time.Second)
				if err != nil {
					return
				}
				defer up.Close()
				pipe(c, up)
			}
		}

		if opts.UDP {
			ts.OnUDPForward = func(ap netip.AddrPort) func(tailcat.ConnPacketConn) {
				return func(conn tailcat.ConnPacketConn) {
					pipeUDPToTarget(conn, ap.String())
				}
			}
		}
	}

	if len(opts.AllowedClients) > 0 {
		for _, raw := range opts.AllowedClients {
			raw = strings.TrimSpace(raw)
			if raw == "" {
				continue
			}
			// 用户通常直接粘贴 "nodekey:..."，解析函数要的是裸十六进制
			raw = strings.TrimPrefix(raw, "nodekey:")
			raw = strings.TrimPrefix(raw, "nodekey(")
			raw = strings.TrimSuffix(raw, ")")
			pub, err := key.ParseNodePublicUntyped(mem.S(raw))
			if err != nil {
				return nil, fmt.Errorf("客户端公钥无效: %s", raw)
			}
			ts.AllowedClients = append(ts.AllowedClients, pub)
		}
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

// pipeUDPToTarget 把隧道里的一条 UDP 流转发到本机 UDP 目标。
func pipeUDPToTarget(conn tailcat.ConnPacketConn, target string) {
	defer conn.Close()

	up, err := net.Dial("udp", target)
	if err != nil {
		return
	}
	defer up.Close()

	done := make(chan struct{}, 2)
	go func() {
		buf := make([]byte, 64*1024)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				break
			}
			if _, err := up.Write(buf[:n]); err != nil {
				break
			}
		}
		done <- struct{}{}
	}()
	go func() {
		buf := make([]byte, 64*1024)
		for {
			n, err := up.Read(buf)
			if err != nil {
				break
			}
			if _, err := conn.Write(buf[:n]); err != nil {
				break
			}
		}
		done <- struct{}{}
	}()

	<-done
}
