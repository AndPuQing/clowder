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

// Server exposes this machine's TCP ports through a tailcat tunnel to
// whoever holds the address.
//
// By default it uses an ephemeral key: the address changes on every start
// and dies when the process exits.
type Server struct {
	ts  *tailcat.Server
	key *SavedKey // non-nil when a persistent key is used

	mu     sync.Mutex
	closed bool
}

// ServerOptions configures a Server.
type ServerOptions struct {
	// Targets maps tunnel ports to local addresses, for example
	//   map[uint16]string{8080: "127.0.0.1:8080"}
	// Ports that are not listed are refused (the client sees a reset).
	Targets map[uint16]string

	// Verbose enables tailcat's debug logging.
	Verbose bool

	// KeyName selects a persistent key. Empty means an ephemeral key:
	// every start gets a new address and nothing survives the process.
	KeyName string

	// UDP also forwards UDP on the same port numbers.
	UDP bool

	// AllowProxy lets the peer reach other addresses through this host
	// (its LAN, or any address this machine can dial).
	AllowProxy bool

	// AllowedClients limits connections to these client public keys
	// ("nodekey:..."). Empty accepts any client holding the address.
	AllowedClients []string

	// DERPMapURL overrides the default relay map URL.
	DERPMapURL string

	// RegionID pins a preferred relay region.
	RegionID int

	// SSH opens an SSH service inside the tunnel (port 22), giving the
	// peer a shell.
	SSH bool

	// SSHFileDir, when non-empty, exposes SFTP rooted at this directory,
	// read-only.
	SSHFileDir string

	// SSHAuthorizedKeys lists OpenSSH public keys allowed to log in.
	// Empty means the tunnel identity is sufficient.
	SSHAuthorizedKeys []string
}

// StartServer starts the server and connects it to a DERP relay.
func StartServer(opts ServerOptions) (*Server, error) {
	if len(opts.Targets) == 0 {
		return nil, fmt.Errorf("at least one port to expose is required")
	}
	if opts.Verbose {
		tailcat.Verbose = true
	}

	var saved *SavedKey
	var sshOnce sync.Once
	var sshHandler func(net.Conn)

	var ts *tailcat.Server
	ts = &tailcat.Server{
		// The network stack logs every handshake; keep it quiet unless the
		// caller asked for diagnostics.
		Logf: tailcatLogf(opts.Verbose),
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
				return nil // port not exposed: reset
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
		// The peer may name any target; this host dials it.
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
			// People usually paste "nodekey:..."; the parser wants the
			// bare hex string.
			raw = strings.TrimPrefix(raw, "nodekey:")
			raw = strings.TrimPrefix(raw, "nodekey(")
			raw = strings.TrimSuffix(raw, ")")
			pub, err := key.ParseNodePublicUntyped(mem.S(raw))
			if err != nil {
				return nil, fmt.Errorf("invalid client public key: %s", raw)
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
		return nil, fmt.Errorf("start tailcat server: %w", err)
	}
	return &Server{ts: ts, key: saved}, nil
}

// Address returns the address to hand to clients (it starts with tc).
func (s *Server) Address() string {
	return string(s.ts.TailcatAddr())
}

// ListenAddr returns the server's internal address; it is only useful for
// logs.
func (s *Server) ListenAddr() string {
	return s.ts.Addr().String()
}

// PublicKey returns the server's node public key. With an ephemeral key
// there is no persistent identity, so the result is empty.
func (s *Server) PublicKey() string {
	if s.key != nil {
		return s.key.PublicKey()
	}
	return ""
}

// UsingSavedKey reports whether a persistent key is in use.
func (s *Server) UsingSavedKey() bool {
	return s.key != nil
}

// Close shuts the server down; the address stops working immediately.
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.ts.Close()
}

// pipeToTarget connects an incoming tunnel connection to a local target
// and copies bytes both ways.
func pipeToTarget(c net.Conn, target string) {
	defer c.Close()

	up, err := net.DialTimeout("tcp", target, 10*time.Second)
	if err != nil {
		return
	}
	defer up.Close()

	pipe(c, up)
}

// pipe copies in both directions and closes both sides when either ends.
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

// pipeUDPToTarget relays one tunnel UDP flow to a local UDP target.
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
