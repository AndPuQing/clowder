// Command clowderd 是 Clowder 的守护进程。
//
// 两个子命令：
//
//	clowderd serve -expose 8080                     本机 8080 通过隧道暴露出去
//	clowderd dial -addr tc... -forward 9000:8080    隧道里的 8080 映射到本机 9000
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/AndPuQing/clowder/core"
)

// pairList 收集可重复的 -expose / -forward 参数。
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

用法:
  clowderd serve -expose <端口>[=<本机地址>] ... [-v]
  clowderd dial  -addr <tc地址> -forward <本地端口>:<远端端口> ... [-ping]
  clowderd keys  list | gen <名字> | show <名字> | rm <名字> | dir

示例:
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
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "未知子命令: %s\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	var exposes pairList
	fs.Var(&exposes, "expose", "暴露的端口，可写成 端口=本机地址（可重复）")
	verbose := fs.Bool("v", false, "打开调试日志")
	keyName := fs.String("key", "", "持久化密钥名（给定后地址跨重启稳定；留空则用临时密钥）")
	fs.Parse(args)

	if len(exposes.vals) == 0 {
		fmt.Fprintln(os.Stderr, "至少要给一个 -expose")
		os.Exit(2)
	}

	targets := make(map[uint16]string, len(exposes.vals))
	for _, spec := range exposes.vals {
		port, target, err := parseExpose(spec)
		if err != nil {
			fmt.Fprintf(os.Stderr, "解析 -expose %q: %v\n", spec, err)
			os.Exit(2)
		}
		targets[port] = target
	}

	srv, err := core.StartServer(core.ServerOptions{
		Targets: targets,
		Verbose: *verbose,
		KeyName: *keyName,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "启动失败: %v\n", err)
		os.Exit(1)
	}
	defer srv.Close()

	fmt.Println("🐈 服务已启动")
	fmt.Printf("   暴露端口: %s\n", describeTargets(targets))
	if srv.UsingSavedKey() {
		fmt.Printf("   密钥: %s（地址固定）\n", *keyName)
		fmt.Printf("   公钥: %s\n", srv.PublicKey())
	} else {
		fmt.Println("   密钥: 临时（地址仅本次有效）")
	}
	fmt.Printf("   内部地址: %s\n", srv.ListenAddr())
	fmt.Println()
	fmt.Printf("地址（发给客户端）:\n%s\n\n", srv.Address())
	fmt.Println("按 Ctrl-C 停止，地址随即失效。")

	waitForSignal()
}

func runDial(args []string) {
	fs := flag.NewFlagSet("dial", flag.ExitOnError)
	addr := fs.String("addr", "", "服务端的 tailcat 地址")
	var forwards pairList
	fs.Var(&forwards, "forward", "本地端口:远端端口（可重复）")
	pingOnly := fs.Bool("ping", false, "只测连通性，不做映射")
	keyName := fs.String("key", "", "持久化密钥名（固定客户端公钥，便于服务端白名单）")
	fs.Parse(args)

	if *addr == "" {
		fmt.Fprintln(os.Stderr, "必须给 -addr")
		os.Exit(2)
	}

	c, err := core.NewClientWithOptions(core.ClientOptions{Addr: *addr, KeyName: *keyName})
	if err != nil {
		fmt.Fprintf(os.Stderr, "地址无效: %v\n", err)
		os.Exit(2)
	}
	defer c.Close()

	fmt.Printf("本端公钥: %s\n", c.PublicKey())

	lat, err := c.Ping(30 * time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "连不上服务端: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("隧道已通，往返 %v\n", lat.Round(time.Millisecond))

	if *pingOnly {
		return
	}
	if len(forwards.vals) == 0 {
		fmt.Fprintln(os.Stderr, "没有 -forward，隧道建好后直接退出（可加 -ping 只测连通）")
		return
	}

	for _, spec := range forwards.vals {
		local, remote, err := parseForward(spec)
		if err != nil {
			fmt.Fprintf(os.Stderr, "解析 -forward %q: %v\n", spec, err)
			os.Exit(2)
		}
		ln, err := c.Forward(local, remote)
		if err != nil {
			fmt.Fprintf(os.Stderr, "本地监听失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("已映射 本机 %s → 隧道 %d\n", ln.Addr(), remote)
	}

	fmt.Println("\n按 Ctrl-C 断开。")
	waitForSignal()
}

// parseExpose 解析 "8080" 或 "8080=127.0.0.1:8080"。
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

// parseForward 解析 "9000:8080"。
func parseForward(spec string) (string, uint16, error) {
	i := strings.IndexByte(spec, ':')
	if i < 0 {
		return "", 0, fmt.Errorf("要写成 本地端口:远端端口")
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
		return 0, fmt.Errorf("端口无效: %q", s)
	}
	return uint16(n), nil
}

func describeTargets(m map[uint16]string) string {
	parts := make([]string, 0, len(m))
	for port, target := range m {
		parts = append(parts, fmt.Sprintf("%d→%s", port, target))
	}
	return strings.Join(parts, ", ")
}

func waitForSignal() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	<-ch
	fmt.Println("\n已停止。")
}

// runKeys 管理持久化密钥。
func runKeys(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "用法: clowderd keys list|gen <名>|show <名>|rm <名>|dir")
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
			fmt.Println("（还没有密钥）")
			return
		}
		for _, n := range names {
			k, err := core.LoadKey(n)
			if err != nil {
				fmt.Printf("%-16s (读取失败: %v)\n", n, err)
				continue
			}
			fmt.Printf("%-16s %s  建于 %s\n", n, k.PublicKey(), k.CreatedAt.Local().Format("2006-01-02 15:04"))
		}

	case "gen":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "要指定名字: clowderd keys gen <名字>")
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
		fmt.Printf("已生成 %s\n公钥: %s\n", k.Name, k.PublicKey())

	case "show":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "要指定名字: clowderd keys show <名字>")
			os.Exit(2)
		}
		k, err := core.LoadKey(args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		fmt.Printf("名字: %s\n公钥: %s\n创建: %s\n", k.Name, k.PublicKey(), k.CreatedAt.Local().Format(time.RFC3339))

	case "rm":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "要指定名字: clowderd keys rm <名字>")
			os.Exit(2)
		}
		if err := core.DeleteKey(args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		fmt.Printf("已删除 %s\n", args[1])

	default:
		fmt.Fprintf(os.Stderr, "未知的 keys 子命令: %s\n", args[0])
		os.Exit(2)
	}
}
