//go:build !libghostty

package core

import (
	"fmt"
	"io"
)

// Cell is one terminal cell. This is the placeholder definition used when
// the build does not include libghostty.
type Cell struct {
	Text           string
	Fg, Bg         string
	Bold, Italic   bool
	Under, Inverse bool
}

// Snapshot is one rendered screen.
type Snapshot struct {
	Cols, Rows    int
	Cells         [][]Cell
	CursorX       int
	CursorY       int
	CursorVisible bool
}

// Term is unavailable when the build does not include libghostty.
type Term struct{}

// NewTerm reports that this build has no terminal core.
func NewTerm(cols, rows int) (*Term, error) {
	return NewTermWithWriter(cols, rows, nil)
}

// NewTermWithWriter reports that this build has no terminal core.
func NewTermWithWriter(cols, rows int, w io.Writer) (*Term, error) {
	return nil, fmt.Errorf("this build has no terminal core (requires libghostty)")
}

// Feed is a no-op without a terminal core.
func (t *Term) Feed(b []byte) error { return nil }

// Resize is a no-op without a terminal core.
func (t *Term) Resize(cols, rows int) error { return nil }

// Snapshot reports that this build has no terminal core.
func (t *Term) Snapshot() (*Snapshot, error) {
	return nil, fmt.Errorf("this build has no terminal core")
}

// Close is a no-op without a terminal core.
func (t *Term) Close() {}

// TerminalAvailable reports whether libghostty was compiled in.
func TerminalAvailable() bool { return false }
