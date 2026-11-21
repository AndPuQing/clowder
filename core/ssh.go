package core

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSHPort 是隧道里 SSH 服务使用的端口。
const SSHPort = 22

// dialTunnelTCP 建立一条到对方的隧道连接。
func dialTunnelTCP(ctx context.Context, addr string, port uint16) (net.Conn, error) {
	c, err := NewClient(addr)
	if err != nil {
		return nil, err
	}
	return c.Dial(port, 20*time.Second)
}

// SSHExec 通过隧道在对方机器上执行一条命令，返回 stdout/stderr。
func SSHExec(ctx context.Context, addr string, command string) (string, error) {
	conn, err := dialTunnelTCP(ctx, addr, SSHPort)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	cfg := &ssh.ClientConfig{
		User:            "clowder",
		Auth:            []ssh.AuthMethod{ssh.Password("")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         15 * time.Second,
	}
	cc, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		return "", fmt.Errorf("SSH 握手失败: %w", err)
	}
	client := ssh.NewClient(cc, chans, reqs)
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("建立会话: %w", err)
	}
	defer sess.Close()

	var out bytes.Buffer
	sess.Stdout = &out
	sess.Stderr = &out
	if err := sess.Run(command); err != nil {
		if _, ok := err.(*ssh.ExitError); !ok {
			return out.String(), fmt.Errorf("执行失败: %w", err)
		}
	}
	return out.String(), nil
}

// SSHShell 打开一个交互式会话，返回双向流与终端尺寸设置函数。
func SSHShell(ctx context.Context, addr string, cols, rows int) (io.ReadWriteCloser, func(cols, rows int) error, error) {
	conn, err := dialTunnelTCP(ctx, addr, SSHPort)
	if err != nil {
		return nil, nil, err
	}

	cfg := &ssh.ClientConfig{
		User:            "clowder",
		Auth:            []ssh.AuthMethod{ssh.Password("")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         15 * time.Second,
	}
	cc, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("SSH 握手失败: %w", err)
	}
	client := ssh.NewClient(cc, chans, reqs)

	sess, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, nil, err
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := sess.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		sess.Close()
		client.Close()
		return nil, nil, fmt.Errorf("申请 PTY: %w", err)
	}

	stdin, err := sess.StdinPipe()
	if err != nil {
		sess.Close()
		client.Close()
		return nil, nil, err
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		sess.Close()
		client.Close()
		return nil, nil, err
	}
	sess.Stderr = sess.Stdout

	if err := sess.Shell(); err != nil {
		sess.Close()
		client.Close()
		return nil, nil, fmt.Errorf("启动 shell: %w", err)
	}

	resize := func(cols, rows int) error {
		return sess.WindowChange(rows, cols)
	}

	stream := &sshStream{stdin: stdin, stdout: stdout, sess: sess, client: client}
	_ = ctx
	return stream, resize, nil
}

// sshStream 把 stdin/stdout 与底层会话打包成一个读写关闭器。
type sshStream struct {
	stdin  io.WriteCloser
	stdout io.Reader
	sess   *ssh.Session
	client *ssh.Client
}

func (s *sshStream) Read(p []byte) (int, error)  { return s.stdout.Read(p) }
func (s *sshStream) Write(p []byte) (int, error) { return s.stdin.Write(p) }

func (s *sshStream) Close() error {
	_ = s.sess.Close()
	return s.client.Close()
}
