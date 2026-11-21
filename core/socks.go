package core

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
)

// ServeSOCKS5 listens for SOCKS5 clients and dials every CONNECT through
// the supplied dialer.
//
// This is the entry point for "reach the peer's network": browsers and
// curl point their proxy at 127.0.0.1:1080 and the host dials the target
// on their behalf. The host must allow proxying, otherwise tailcat
// refuses the forward.
func ServeSOCKS5(ctx context.Context, listenAddr string, dial func(ctx context.Context, network, addr string) (net.Conn, error)) (net.Listener, error) {
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", listenAddr, err)
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go socksConn(ctx, conn, dial)
		}
	}()

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	return ln, nil
}

func socksConn(ctx context.Context, c net.Conn, dial func(context.Context, string, string) (net.Conn, error)) {
	defer c.Close()

	// Greeting: VER NMETHODS METHODS...
	head := make([]byte, 2)
	if _, err := io.ReadFull(c, head); err != nil || head[0] != 5 {
		return
	}
	methods := make([]byte, int(head[1]))
	if _, err := io.ReadFull(c, methods); err != nil {
		return
	}
	// Only "no authentication" is offered; the tunnel is already
	// encrypted and authenticated.
	if _, err := c.Write([]byte{5, 0}); err != nil {
		return
	}

	// Request: VER CMD RSV ATYP DST.ADDR DST.PORT
	req := make([]byte, 4)
	if _, err := io.ReadFull(c, req); err != nil {
		return
	}
	if req[0] != 5 || req[1] != 1 { // CONNECT only
		c.Write([]byte{5, 7, 0, 1, 0, 0, 0, 0, 0, 0}) // command not supported
		return
	}

	host, err := readSocksAddr(c, req[3])
	if err != nil {
		return
	}
	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(c, portBuf); err != nil {
		return
	}
	target := net.JoinHostPort(host, fmt.Sprint(binary.BigEndian.Uint16(portBuf)))

	remote, err := dial(ctx, "tcp", target)
	if err != nil {
		c.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0}) // connection refused
		return
	}
	defer remote.Close()

	// Success reply.
	if _, err := c.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}

	pipe(c, remote)
}

// readSocksAddr reads the SOCKS5 address field and returns its host part.
func readSocksAddr(c net.Conn, atyp byte) (string, error) {
	switch atyp {
	case 1: // IPv4
		b := make([]byte, 4)
		if _, err := io.ReadFull(c, b); err != nil {
			return "", err
		}
		return net.IP(b).String(), nil
	case 3: // domain name
		l := make([]byte, 1)
		if _, err := io.ReadFull(c, l); err != nil {
			return "", err
		}
		b := make([]byte, int(l[0]))
		if _, err := io.ReadFull(c, b); err != nil {
			return "", err
		}
		return string(b), nil
	case 4: // IPv6
		b := make([]byte, 16)
		if _, err := io.ReadFull(c, b); err != nil {
			return "", err
		}
		return net.IP(b).String(), nil
	default:
		return "", fmt.Errorf("unsupported address type %d", atyp)
	}
}
