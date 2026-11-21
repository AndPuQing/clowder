package core

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/tailscale/tailcat"
	"tailscale.com/types/logger"
)

// UDP flow limits: a datagram flood must not be able to create unbounded
// tunnels, and an idle flow should not live forever.
const (
	maxUDPFlows    = 256
	udpIdleTimeout = 2 * time.Minute
)

// Client connects to a tailcat address and maps tunnel ports onto this
// machine.
type Client struct {
	tc *tailcat.Client

	mu         sync.Mutex
	listeners  []net.Listener
	udpConns   []net.PacketConn
	socksAddrs []string
	cancels    []context.CancelFunc
}

// ClientOptions configures a Client.
type ClientOptions struct {
	// Addr is the host's tailcat address.
	Addr string

	// KeyName selects a persistent client key. A host can only
	// allow-list clients whose key does not change between runs.
	KeyName string

	// DERPMapURL overrides the default relay map URL.
	DERPMapURL string

	// Verbose prints the network stack's diagnostic log to stderr.
	Verbose bool
}

// tailcatLogf returns the logger handed to tailcat. The network stack is
// chatty, so it is silent unless the caller asked for diagnostics.
func tailcatLogf(verbose bool) logger.Logf {
	if verbose {
		return log.Printf
	}
	return func(string, ...any) {}
}

// PublicKeyOf returns the public key of the named persistent key.
func PublicKeyOf(name string) (string, error) {
	k, err := LoadKey(name)
	if err != nil {
		return "", err
	}
	return k.PublicKey(), nil
}

// NewClientWithOptions creates a client with optional persistent key and
// relay settings.
func NewClientWithOptions(opts ClientOptions) (*Client, error) {
	c, err := NewClient(opts.Addr)
	if err != nil {
		return nil, err
	}
	if opts.DERPMapURL != "" {
		c.tc.DERPMapURL = opts.DERPMapURL
	}
	if opts.Verbose {
		c.tc.Logf = tailcatLogf(true)
	}
	if opts.KeyName != "" {
		k, _, err := LoadOrCreateKey(opts.KeyName)
		if err != nil {
			// Do not leak the client we already built.
			_ = c.tc.Close()
			return nil, err
		}
		c.tc.Key = k.NodeKey
	}
	return c, nil
}

// NewClient validates the address and returns a client. Nothing is
// dialed yet: the tunnel starts lazily on the first Dial.
func NewClient(addr string) (*Client, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil, fmt.Errorf("address is empty")
	}
	if !strings.HasPrefix(addr, "tc") {
		return nil, fmt.Errorf("not a tailcat address (must start with tc): %.16s", addr)
	}
	// Parse now so a pasted, malformed address fails at paste time rather
	// than on the first dial.
	ci, err := tailcat.ParseAddr(tailcat.Addr(addr))
	if err != nil {
		return nil, fmt.Errorf("invalid tailcat address: %w", err)
	}
	if ci.ServerDiscoPublic.IsZero() {
		return nil, fmt.Errorf("address comes from an old tailcat version; generate a new one")
	}
	tc := tailcat.NewClient(tailcat.Addr(addr))
	tc.Logf = tailcatLogf(false)
	return &Client{tc: tc}, nil
}

// Ping measures one round trip, to check that the address is still live.
func (c *Client) Ping(timeout time.Duration) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	res, err := c.tc.Ping(ctx)
	if err != nil {
		return 0, err
	}
	return res.Latency, nil
}

// PublicKey returns this client's node public key.
func (c *Client) PublicKey() string {
	return c.tc.PublicKey().String()
}

// Dial opens a tunnel connection to one of the host's TCP ports.
func (c *Client) Dial(remotePort uint16, timeout time.Duration) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return c.tc.DialTCPPort(ctx, remotePort)
}

// Forward listens on listenAddr and sends every accepted connection into
// the tunnel to remotePort. The returned listener is already accepting;
// Close stops it.
func (c *Client) Forward(listenAddr string, remotePort uint16) (net.Listener, error) {
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", listenAddr, err)
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

// handle opens a tunnel connection for one local connection and copies
// bytes both ways.
func (c *Client) handle(local net.Conn, remotePort uint16) {
	defer local.Close()

	remote, err := c.Dial(remotePort, 30*time.Second)
	if err != nil {
		return
	}
	defer remote.Close()

	pipe(local, remote)
}

// ForwardUDP listens on listenAddr and sends each source's datagrams into
// the tunnel to remotePort.
func (c *Client) ForwardUDP(listenAddr string, remotePort uint16) (net.PacketConn, error) {
	pc, err := net.ListenPacket("udp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("listen on UDP %s: %w", listenAddr, err)
	}

	c.mu.Lock()
	c.udpConns = append(c.udpConns, pc)
	c.mu.Unlock()

	go func() {
		buf := make([]byte, 64*1024)
		slots := make(chan struct{}, maxUDPFlows)
		for {
			n, src, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			payload := append([]byte(nil), buf[:n]...)
			select {
			case slots <- struct{}{}:
				go func() {
					defer func() { <-slots }()
					c.udpFlow(pc, src, payload, remotePort)
				}()
			default:
				// Too many concurrent flows: drop the datagram rather
				// than grow without bound.
			}
		}
	}()

	return pc, nil
}

// udpFlow opens one tunnel UDP flow per source and relays replies back to
// that source.
//
// The read loop has to stay in this function: cancelling the context
// closes the tunnel connection, so returning early would kill it.
func (c *Client) udpFlow(pc net.PacketConn, src net.Addr, first []byte, remotePort uint16) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn, err := c.tc.DialUDPPort(ctx, remotePort)
	if err != nil {
		return
	}
	defer conn.Close()

	if _, err := conn.Write(first); err != nil {
		return
	}

	buf := make([]byte, 64*1024)
	for {
		_ = conn.SetReadDeadline(time.Now().Add(udpIdleTimeout))
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		if _, err := pc.WriteTo(buf[:n], src); err != nil {
			return
		}
	}
}

// SOCKS5 starts a local SOCKS5 proxy whose connections are carried by the
// tunnel and dialed by the host.
func (c *Client) SOCKS5(listenAddr string) (net.Listener, error) {
	ctx, cancel := context.WithCancel(context.Background())

	c.mu.Lock()
	c.cancels = append(c.cancels, cancel)
	c.mu.Unlock()

	ln, err := ServeSOCKS5(ctx, listenAddr, func(ctx context.Context, network, addr string) (net.Conn, error) {
		ap, err := netip.ParseAddrPort(addr)
		if err != nil {
			return nil, err
		}
		return c.tc.DialTCP(ctx, ap)
	})
	if err != nil {
		cancel()
		return nil, err
	}

	c.mu.Lock()
	c.socksAddrs = append(c.socksAddrs, ln.Addr().String())
	c.mu.Unlock()

	return ln, nil
}

// Close stops every local listener and shuts the tunnel down.
func (c *Client) Close() error {
	c.mu.Lock()
	lns := c.listeners
	c.listeners = nil
	c.mu.Unlock()

	for _, ln := range lns {
		ln.Close()
	}
	c.mu.Lock()
	udps := c.udpConns
	cancels := c.cancels
	c.udpConns = nil
	c.cancels = nil
	c.mu.Unlock()

	for _, pc := range udps {
		pc.Close()
	}
	for _, cancel := range cancels {
		cancel()
	}
	return c.tc.Close()
}
