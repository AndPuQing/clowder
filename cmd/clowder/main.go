// Command clowder 是 Clowder 的桌面程序。
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/AndPuQing/clowder/core"
	"github.com/AndPuQing/clowder/ui"
)

func main() {
	version := flag.Bool("version", false, "打印版本号后退出")
	flag.Parse()

	if *version {
		fmt.Printf("clowder %s\n", core.Version)
		return
	}

	if err := ui.New().Run(); err != nil {
		fmt.Fprintf(os.Stderr, "clowder: %v\n", err)
		os.Exit(1)
	}
}