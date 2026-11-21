package core

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
)

// ServeSOCKS5 在本地监听一个 SOCKS5 代理，所有 CONNECT 都经过隧道。
//
// 这就是「访问对方内网」的入口：浏览器、curl 把代理指向 127.0.0.1:1080，
// 目标地址由服务端按它的网络去拨号。服务端需要打开「允许访问内网」，
// 否则 tailcat 会拒绝转发。
func ServeSOCKS5(ctx context.Context, listenAddr string, dial func(ctx context.Context, network, addr string) (net.Conn, error)) (net.Listener, error) {
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("监听 %s: %w", listenAddr, err)
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

	// 握手：VER NMETHODS METHODS...
	head := make([]byte, 2)
	if _, err := io.ReadFull(c, head); err != nil || head[0] != 5 {
		return
	}
	methods := make([]byte, int(head[1]))
	if _, err := io.ReadFull(c, methods); err != nil {
		return
	}
	// 只支持「无需认证」（隧道本身已经加密）
	if _, err := c.Write([]byte{5, 0}); err != nil {
		return
	}

	// 请求：VER CMD RSV ATYP DST.ADDR DST.PORT
	req := make([]byte, 4)
	if _, err := io.ReadFull(c, req); err != nil {
		return
	}
	if req[0] != 5 || req[1] != 1 { // 只处理 CONNECT
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

	// 成功应答
	if _, err := c.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}

	pipe(c, remote)
}

// readSocksAddr 读取 SOCKS5 的地址字段，返回主机部分。
func readSocksAddr(c net.Conn, atyp byte) (string, error) {
	switch atyp {
	case 1: // IPv4
		b := make([]byte, 4)
		if _, err := io.ReadFull(c, b); err != nil {
			return "", err
		}
		return net.IP(b).String(), nil
	case 3: // 域名
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
		return "", fmt.Errorf("不支持的地址类型 %d", atyp)
	}
}
