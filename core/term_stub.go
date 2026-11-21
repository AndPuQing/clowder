//go:build !libghostty

package core

import "fmt"

// Cell 是终端里的一格（未启用 libghostty 时的占位定义）。
type Cell struct {
	Text           string
	Fg, Bg         string
	Bold, Italic   bool
	Under, Inverse bool
}

// Snapshot 是一帧屏幕。
type Snapshot struct {
	Cols, Rows    int
	Cells         [][]Cell
	CursorX       int
	CursorY       int
	CursorVisible bool
}

// Term 在未启用 libghostty 的构建里不可用。
type Term struct{}

// NewTerm 返回错误：这个构建没有终端内核。
func NewTerm(cols, rows int) (*Term, error) {
	return nil, fmt.Errorf("这个构建没有启用终端内核（需要 libghostty）")
}

func (t *Term) Feed(b []byte) error         { return nil }
func (t *Term) Resize(cols, rows int) error { return nil }
func (t *Term) Snapshot() (*Snapshot, error) {
	return nil, fmt.Errorf("这个构建没有启用终端内核")
}
func (t *Term) Close() {}

// TerminalAvailable 报告是否编进了 libghostty。
func TerminalAvailable() bool { return false }
