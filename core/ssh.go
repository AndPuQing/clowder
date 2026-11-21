package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSHPort is the port the SSH service listens on inside the tunnel.
const SSHPort = 22

// sshConfig is the client configuration used for every SSH session.
//
// Authentication is delegated to the tunnel: only clients holding the
// tailcat address (and its pre-shared key) can reach this port, so no
// password or key material is exchanged. For the same reason the host
// key is not pinned; the tunnel already authenticates the peer.
func sshConfig() *ssh.ClientConfig {
	return &ssh.ClientConfig{
		User:            "clowder",
		Auth:            []ssh.AuthMethod{ssh.Password("")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         15 * time.Second,
	}
}

// ownedConn ties a tunnel connection to the client that owns it: closing
// the connection also shuts the client down, so a one-shot SSH session
// does not leak a WireGuard engine and its DERP connections.
type ownedConn struct {
	net.Conn
	closeClient func() error
}

// Close closes the connection and then the owning client.
func (c *ownedConn) Close() error {
	err := c.Conn.Close()
	if cerr := c.closeClient(); err == nil {
		err = cerr
	}
	return err
}

// DialSSH opens a connection to addr's SSH service through a fresh tunnel
// client. Callers that already have a Client should dial the port directly.
func DialSSH(addr string, timeout time.Duration) (net.Conn, error) {
	c, err := NewClient(addr)
	if err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	conn, err := c.Dial(SSHPort, timeout)
	if err != nil {
		_ = c.Close()
		return nil, err
	}
	return &ownedConn{Conn: conn, closeClient: c.Close}, nil
}

// sshConnect performs the SSH handshake over an established tunnel
// connection. name is only used for logging and host-key verification.
func sshConnect(conn net.Conn, name string) (*ssh.Client, error) {
	cc, chans, reqs, err := ssh.NewClientConn(conn, name, sshConfig())
	if err != nil {
		return nil, fmt.Errorf("ssh handshake: %w", err)
	}
	return ssh.NewClient(cc, chans, reqs), nil
}

// SSHExec runs command on the host behind addr and returns its combined
// stdout and stderr. A non-zero exit status is reported as output plus no
// error; transport and session failures are returned as errors.
func SSHExec(ctx context.Context, addr, command string) (string, error) {
	conn, err := DialSSH(addr, 20*time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	return SSHExecConn(ctx, conn, command)
}

// SSHExecConn is SSHExec over an already established tunnel connection.
// It takes ownership of conn and closes it before returning.
func SSHExecConn(ctx context.Context, conn net.Conn, command string) (string, error) {
	defer conn.Close()

	client, err := sshConnect(conn, "clowder")
	if err != nil {
		return "", err
	}
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("new ssh session: %w", err)
	}
	defer sess.Close()

	// stdout and stderr arrive on separate goroutines, so they share a
	// mutex-protected buffer.
	out := &syncBuffer{}
	sess.Stdout = out
	sess.Stderr = out

	if ctx == nil {
		ctx = context.Background()
	}

	done := make(chan error, 1)
	go func() { done <- sess.Run(command) }()

	select {
	case err := <-done:
		if err != nil {
			var exit *ssh.ExitError
			if errors.As(err, &exit) {
				// The command ran; report its status so callers do not
				// mistake a failure for success.
				return out.String(), fmt.Errorf("command exited with status %d", exit.ExitStatus())
			}
			return out.String(), fmt.Errorf("run command: %w", err)
		}
		return out.String(), nil
	case <-ctx.Done():
		_ = sess.Close()
		<-done
		return out.String(), ctx.Err()
	}
}

// SSHShell opens an interactive shell through a fresh tunnel client.
// Closing the returned stream closes both the SSH session and the client.
func SSHShell(ctx context.Context, addr string, cols, rows int) (io.ReadWriteCloser, func(cols, rows int) error, error) {
	conn, err := DialSSH(addr, 20*time.Second)
	if err != nil {
		return nil, nil, err
	}
	return SSHShellConn(conn, cols, rows)
}

// SSHShellConn opens an interactive shell over an already established
// tunnel connection and takes ownership of it.
func SSHShellConn(conn net.Conn, cols, rows int) (io.ReadWriteCloser, func(cols, rows int) error, error) {
	client, err := sshConnect(conn, "clowder")
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}

	sess, err := client.NewSession()
	if err != nil {
		_ = client.Close()
		_ = conn.Close()
		return nil, nil, fmt.Errorf("new ssh session: %w", err)
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := sess.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		_ = sess.Close()
		_ = client.Close()
		_ = conn.Close()
		return nil, nil, fmt.Errorf("request pty: %w", err)
	}

	stdin, err := sess.StdinPipe()
	if err != nil {
		_ = sess.Close()
		_ = client.Close()
		_ = conn.Close()
		return nil, nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		_ = sess.Close()
		_ = client.Close()
		_ = conn.Close()
		return nil, nil, fmt.Errorf("stdout pipe: %w", err)
	}
	sess.Stderr = sess.Stdout

	if err := sess.Shell(); err != nil {
		_ = sess.Close()
		_ = client.Close()
		_ = conn.Close()
		return nil, nil, fmt.Errorf("start shell: %w", err)
	}

	resize := func(cols, rows int) error {
		return sess.WindowChange(rows, cols)
	}

	stream := &sshStream{
		stdin:  stdin,
		stdout: stdout,
		sess:   sess,
		client: client,
		conn:   conn,
	}
	return stream, resize, nil
}

// syncBuffer is a bytes.Buffer safe for concurrent writers, which SSH
// sessions use for stdout and stderr.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// sshStream bundles stdin/stdout with everything needed to tear the
// session down cleanly.
type sshStream struct {
	stdin  io.WriteCloser
	stdout io.Reader
	sess   *ssh.Session
	client *ssh.Client
	conn   net.Conn

	once sync.Once
	err  error
}

// Read reads shell output.
func (s *sshStream) Read(p []byte) (int, error) { return s.stdout.Read(p) }

// Write sends input to the remote shell.
func (s *sshStream) Write(p []byte) (int, error) { return s.stdin.Write(p) }

// Close closes the session, the SSH client and the tunnel connection.
func (s *sshStream) Close() error {
	s.once.Do(func() {
		_ = s.sess.Close()
		err := s.client.Close()
		if cerr := s.conn.Close(); err == nil {
			err = cerr
		}
		s.err = err
	})
	return s.err
}
