// Command clowderd is Clowder's headless daemon.
//
// Two subcommands:
//
//	clowderd serve -expose 8080                     expose this machine's 8080
//	clowderd dial -addr tc... -forward 9000:8080    map the tunnel's 8080 to local 9000
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/AndPuQing/clowder/core"
)

// pairList collects repeatable -expose / -forward flags.
type pairList struct {
	name string
	vals []string
}

func (p *pairList) String() string { return strings.Join(p.vals, ",") }

func (p *pairList) Set(v string) error {
	p.vals = append(p.vals, v)
	return nil
}

func usage() {
	fmt.Fprintf(os.Stderr, `Clowder daemon

Usage:
  clowderd serve -expose <port>[=<local address>] ... [-udp] [-ssh] [-ssh-files <dir>] [-v]
  clowderd dial  -addr <tailcat address> -forward <local>:<remote> ... [-ping] [-socks <addr>] [-v]
  clowderd keys  list | gen <name> | show <name> | rm <name> | dir
  clowderd files -addr <address> [-key <name>] [-list <dir>] [-get <remote> [-out <local>]] [-v]
  clowderd --version

Examples:
  clowderd serve -expose 8080
  clowderd serve -expose 8080=127.0.0.1:8080 -expose 2222=127.0.0.1:22
  clowderd dial -addr tcXXXX -forward 9000:8080
`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "serve":
		runServe(os.Args[2:])
	case "dial":
		runDial(os.Args[2:])
	case "keys":
		runKeys(os.Args[2:])
	case "files":
		runFiles(os.Args[2:])
	case "-version", "--version", "version":
		fmt.Printf("clowderd %s\n", core.Version)
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	var exposes pairList
	fs.Var(&exposes, "expose", "port to expose, optionally as port=local-address (repeatable)")
	verbose := fs.Bool("v", false, "enable debug logging")
	keyName := fs.String("key", "", "persistent key name (stable address across restarts; empty uses an ephemeral key)")
	udp := fs.Bool("udp", false, "also forward UDP")
	allowProxy := fs.Bool("allow-proxy", false, "let the peer reach other addresses through this host")
	sshOn := fs.Bool("ssh", false, "open SSH inside the tunnel (port 22)")
	sshDir := fs.String("ssh-files", "", "also expose SFTP, rooted at this read-only directory")
	var allow pairList
	fs.Var(&allow, "allow", "client public key to allow (repeatable)")
	fs.Parse(args)

	if len(exposes.vals) == 0 {
		fmt.Fprintln(os.Stderr, "at least one -expose is required")
		os.Exit(2)
	}

	// Sharing a directory only makes sense with the SSH service, so
	// -ssh-files implies -ssh rather than silently doing nothing.
	if *sshDir != "" {
		*sshOn = true
	}
	if *sshDir != "" {
		if info, err := os.Stat(*sshDir); err != nil || !info.IsDir() {
			fmt.Fprintf(os.Stderr, "-ssh-files: not a directory: %s\n", *sshDir)
			os.Exit(2)
		}
	}

	targets := make(map[uint16]string, len(exposes.vals))
	for _, spec := range exposes.vals {
		port, target, err := parseExpose(spec)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse -expose %q: %v\n", spec, err)
			os.Exit(2)
		}
		targets[port] = target
	}

	srv, err := core.StartServer(core.ServerOptions{
		Targets:        targets,
		Verbose:        *verbose,
		KeyName:        *keyName,
		UDP:            *udp,
		AllowProxy:     *allowProxy,
		AllowedClients: allow.vals,
		SSH:            *sshOn,
		SSHFileDir:     *sshDir,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "start failed: %v\n", err)
		os.Exit(1)
	}
	defer srv.Close()

	fmt.Println("Sharing")
	fmt.Printf("   exposed ports: %s\n", describeTargets(targets))
	if srv.UsingSavedKey() {
		fmt.Printf("   key: %s (stable address)\n", *keyName)
		fmt.Printf("   public key: %s\n", srv.PublicKey())
	} else {
		fmt.Println("   key: ephemeral (this address dies with the process)")
	}
	fmt.Printf("   internal address: %s\n", srv.ListenAddr())
	fmt.Println()
	fmt.Printf("Address (send this to the peer):\n%s\n\n", srv.Address())
	fmt.Println("Press Ctrl-C to stop; the address stops working immediately.")

	waitForSignal()
}

func runDial(args []string) {
	fs := flag.NewFlagSet("dial", flag.ExitOnError)
	addr := fs.String("addr", "", "the host's tailcat address")
	var forwards pairList
	fs.Var(&forwards, "forward", "local:remote port mapping (repeatable)")
	pingOnly := fs.Bool("ping", false, "only test connectivity, do not map ports")
	keyName := fs.String("key", "", "persistent client key (stable public key for the host's allow-list)")
	udp := fs.Bool("udp", false, "also forward UDP")
	socks := fs.String("socks", "", "start a local SOCKS5 proxy (for example 127.0.0.1:1080)")
	sshExec := fs.String("ssh-exec", "", "run one command on the host through the tunnel, then exit")
	verbose := fs.Bool("v", false, "print the network stack's diagnostic log")
	fs.Parse(args)

	if *addr == "" {
		fmt.Fprintln(os.Stderr, "-addr is required")
		os.Exit(2)
	}

	c, err := core.NewClientWithOptions(core.ClientOptions{
		Addr:    *addr,
		KeyName: *keyName,
		Verbose: *verbose,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid address: %v\n", err)
		os.Exit(2)
	}
	defer c.Close()

	fmt.Printf("local public key: %s\n", c.PublicKey())

	if *sshExec != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		// Reuse this client so -key applies: a host with an allow-list
		// sees the same public key that was printed above.
		conn, derr := c.Dial(core.SSHPort, 20*time.Second)
		var out string
		err := derr
		if derr == nil {
			out, err = core.SSHExecConn(ctx, conn, *sshExec)
		}
		// Print whatever the command produced, even when it failed.
		fmt.Print(out)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ssh failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	lat, err := c.Ping(30 * time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot reach the host: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("tunnel is up, round trip %v\n", lat.Round(time.Millisecond))

	if *pingOnly {
		return
	}
	if len(forwards.vals) == 0 {
		fmt.Fprintln(os.Stderr, "no -forward given; exiting after the tunnel came up (use -ping to only test)")
		return
	}

	for _, spec := range forwards.vals {
		local, remote, err := parseForward(spec)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse -forward %q: %v\n", spec, err)
			os.Exit(2)
		}
		ln, err := c.Forward(local, remote)
		if err != nil {
			fmt.Fprintf(os.Stderr, "local listener failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("mapped local %s -> tunnel %d\n", ln.Addr(), remote)

		if *udp {
			if _, err := c.ForwardUDP(local, remote); err != nil {
				fmt.Fprintf(os.Stderr, "UDP listener failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("mapped local UDP %s -> tunnel %d\n", local, remote)
		}
	}

	if *socks != "" {
		ln, err := c.SOCKS5(*socks)
		if err != nil {
			fmt.Fprintf(os.Stderr, "SOCKS5 failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("SOCKS5 proxy listening on %s\n", ln.Addr())
	}

	fmt.Println("\nPress Ctrl-C to disconnect.")
	waitForSignal()
}

// parseExpose parses "8080" or "8080=127.0.0.1:8080".
func parseExpose(spec string) (uint16, string, error) {
	if i := strings.IndexByte(spec, '='); i >= 0 {
		port, err := parsePort(spec[:i])
		if err != nil {
			return 0, "", err
		}
		return port, spec[i+1:], nil
	}
	port, err := parsePort(spec)
	if err != nil {
		return 0, "", err
	}
	return port, fmt.Sprintf("127.0.0.1:%d", port), nil
}

// parseForward parses "9000:8080".
func parseForward(spec string) (string, uint16, error) {
	i := strings.IndexByte(spec, ':')
	if i < 0 {
		return "", 0, fmt.Errorf("mapping must be written local:remote")
	}
	local := spec[:i]
	if _, err := parsePort(local); err != nil {
		return "", 0, err
	}
	remote, err := parsePort(spec[i+1:])
	if err != nil {
		return "", 0, err
	}
	return "127.0.0.1:" + local, remote, nil
}

func parsePort(s string) (uint16, error) {
	n, err := strconv.ParseUint(strings.TrimSpace(s), 10, 16)
	if err != nil || n == 0 {
		return 0, fmt.Errorf("invalid port: %q", s)
	}
	return uint16(n), nil
}

func describeTargets(m map[uint16]string) string {
	parts := make([]string, 0, len(m))
	for port, target := range m {
		parts = append(parts, fmt.Sprintf("%d->%s", port, target))
	}
	return strings.Join(parts, ", ")
}

func waitForSignal() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	<-ch
	fmt.Println("\nStopped.")
}

// runFiles lists or downloads files from a host that shares a directory.
func runFiles(args []string) {
	fs := flag.NewFlagSet("files", flag.ExitOnError)
	addr := fs.String("addr", "", "the host's tailcat address")
	keyName := fs.String("key", "", "persistent client key (needed when the host has an allow-list)")
	dir := fs.String("list", "/", "directory to list")
	get := fs.String("get", "", "remote file to download")
	out := fs.String("out", "", "local path for -get (default: the remote base name)")
	verbose := fs.Bool("v", false, "print the network stack's diagnostic log")
	fs.Parse(args)

	if *addr == "" {
		fmt.Fprintln(os.Stderr, "-addr is required")
		os.Exit(2)
	}

	c, err := core.NewClientWithOptions(core.ClientOptions{
		Addr:    *addr,
		KeyName: *keyName,
		Verbose: *verbose,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid address: %v\n", err)
		os.Exit(2)
	}
	defer c.Close()

	conn, err := c.Dial(core.SSHPort, 30*time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	files, err := core.OpenFiles(conn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	defer files.Close()

	if *get != "" {
		local := *out
		if local == "" {
			local = path.Base(*get)
		}
		if err := files.Download(*get, local); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		fmt.Printf("saved %s\n", local)
		return
	}

	entries, err := files.List(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	for _, e := range entries {
		kind := "file"
		if e.Dir {
			kind = "dir "
		}
		fmt.Printf("%s %10d  %s\n", kind, e.Size, e.Name)
	}
}

// runKeys manages persistent keys.
func runKeys(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: clowderd keys list|gen <name>|show <name>|rm <name>|dir")
		os.Exit(2)
	}

	switch args[0] {
	case "dir":
		d, err := core.ConfigDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		fmt.Println(d)

	case "list":
		names, err := core.ListKeys()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		if len(names) == 0 {
			fmt.Println("(no keys yet)")
			return
		}
		for _, n := range names {
			k, err := core.LoadKey(n)
			if err != nil {
				fmt.Printf("%-16s (read failed: %v)\n", n, err)
				continue
			}
			fmt.Printf("%-16s %s  created %s\n", n, k.PublicKey(),
				k.CreatedAt.Local().Format("2006-01-02 15:04"))
		}

	case "gen":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "name required: clowderd keys gen <name>")
			os.Exit(2)
		}
		k, err := core.GenerateKey(args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		if err := k.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		fmt.Printf("generated %s\npublic key: %s\n", k.Name, k.PublicKey())

	case "show":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "name required: clowderd keys show <name>")
			os.Exit(2)
		}
		k, err := core.LoadKey(args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		fmt.Printf("name: %s\npublic key: %s\ncreated: %s\n", k.Name,
			k.PublicKey(), k.CreatedAt.Local().Format(time.RFC3339))

	case "rm":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "name required: clowderd keys rm <name>")
			os.Exit(2)
		}
		if err := core.DeleteKey(args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		fmt.Printf("deleted %s\n", args[1])

	default:
		fmt.Fprintf(os.Stderr, "unknown keys subcommand: %s\n", args[0])
		os.Exit(2)
	}
}
