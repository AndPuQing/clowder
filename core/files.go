package core

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// FileEntry describes one entry in a host's shared directory.
type FileEntry struct {
	Name    string
	Dir     bool
	Size    int64
	Mode    string
	ModTime time.Time
}

// Files is a read-only SFTP session over the tunnel.
//
// The host exposes one directory read-only (ServerOptions.SSHFileDir); it
// becomes this session's root.
type Files struct {
	sc   *sftp.Client
	ssh  *ssh.Client
	conn net.Conn
}

// OpenFiles opens an SFTP session over an established tunnel connection.
// It takes ownership of conn.
func OpenFiles(conn net.Conn) (*Files, error) {
	client, err := sshConnect(conn, "clowder")
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	sc, err := sftp.NewClient(client)
	if err != nil {
		_ = client.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("start sftp: %w", err)
	}
	return &Files{sc: sc, ssh: client, conn: conn}, nil
}

// List returns the entries of dir, directories first and then by name.
func (f *Files) List(dir string) ([]FileEntry, error) {
	infos, err := f.sc.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", dir, err)
	}
	out := make([]FileEntry, 0, len(infos))
	for _, fi := range infos {
		out = append(out, FileEntry{
			Name:    fi.Name(),
			Dir:     fi.IsDir(),
			Size:    fi.Size(),
			Mode:    fi.Mode().String(),
			ModTime: fi.ModTime(),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Dir != out[j].Dir {
			return out[i].Dir
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// Download copies remote to local. The file appears at local only once it
// is complete.
func (f *Files) Download(remote, local string) error {
	src, err := f.sc.Open(remote)
	if err != nil {
		return fmt.Errorf("open %s: %w", remote, err)
	}
	defer src.Close()

	tmp := local + ".part"
	if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
		return err
	}
	dst, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		os.Remove(tmp)
		return fmt.Errorf("copy %s: %w", remote, err)
	}
	if err := dst.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	// Windows does not replace an existing file with Rename.
	_ = os.Remove(local)
	return os.Rename(tmp, local)
}

// Close ends the session.
func (f *Files) Close() error {
	if f == nil {
		return nil
	}
	if f.sc != nil {
		_ = f.sc.Close()
	}
	if f.ssh != nil {
		_ = f.ssh.Close()
	}
	if f.conn != nil {
		return f.conn.Close()
	}
	return nil
}
