//go:build libghostty

package core

import (
	"fmt"
	"io"
	"sync"

	"go.mitchellh.com/libghostty"
)

// Cell is one terminal cell.
type Cell struct {
	Text string
	// Fg and Bg are "#rrggbb"; an empty string means the default colour.
	Fg, Bg  string
	Bold    bool
	Italic  bool
	Under   bool
	Inverse bool
}

// Snapshot is one rendered screen.
type Snapshot struct {
	Cols, Rows    int
	Cells         [][]Cell
	CursorX       int
	CursorY       int
	CursorVisible bool
}

// Term wraps libghostty's terminal core: feed it VT bytes and take screen
// snapshots.
type Term struct {
	mu sync.Mutex

	term *libghostty.Terminal
	rs   *libghostty.RenderState
	ri   *libghostty.RenderStateRowIterator
	rc   *libghostty.RenderStateRowCells

	cols, rows int
}

// NewTerm creates a terminal with the given size.
func NewTerm(cols, rows int) (*Term, error) {
	return NewTermWithWriter(cols, rows, nil)
}

// NewTermWithWriter creates a terminal that writes its responses (device
// attributes, cursor position reports and similar) to w, as a real
// terminal emulator would write them back to the pty. Without this, shells
// that probe the terminal wait for replies that never come.
func NewTermWithWriter(cols, rows int, w io.Writer) (*Term, error) {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}

	opts := []libghostty.TerminalOption{
		libghostty.WithSize(uint16(cols), uint16(rows)),
		libghostty.WithMaxScrollbackLines(5000),
	}
	if w != nil {
		opts = append(opts, libghostty.WithWritePty(func(_ *libghostty.Terminal, data []byte) {
			// The slice is only valid for this call.
			_, _ = w.Write(append([]byte(nil), data...))
		}))
	}
	term, err := libghostty.NewTerminal(opts...)
	if err != nil {
		return nil, fmt.Errorf("create terminal: %w", err)
	}

	rs, err := libghostty.NewRenderState()
	if err != nil {
		term.Close()
		return nil, err
	}
	ri, err := libghostty.NewRenderStateRowIterator()
	if err != nil {
		rs.Close()
		term.Close()
		return nil, err
	}
	rc, err := libghostty.NewRenderStateRowCells()
	if err != nil {
		ri.Close()
		rs.Close()
		term.Close()
		return nil, err
	}

	return &Term{term: term, rs: rs, ri: ri, rc: rc, cols: cols, rows: rows}, nil
}

// Feed writes VT data (output read from the SSH session).
func (t *Term) Feed(b []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.term == nil {
		return nil
	}
	t.term.VTWrite(b)
	return nil
}

// Resize changes the terminal size.
func (t *Term) Resize(cols, rows int) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.term == nil || cols <= 0 || rows <= 0 {
		return nil
	}
	t.cols, t.rows = cols, rows
	return t.term.Resize(uint16(cols), uint16(rows), 0, 0)
}

// Snapshot renders the current screen.
func (t *Term) Snapshot() (*Snapshot, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.term == nil {
		return nil, fmt.Errorf("terminal is closed")
	}

	if err := t.rs.Update(t.term); err != nil {
		return nil, err
	}
	colors, err := t.rs.Colors()
	if err != nil {
		return nil, err
	}

	snap := &Snapshot{Cols: t.cols, Rows: t.rows}

	if ok, _ := t.rs.CursorVisible(); ok {
		snap.CursorVisible = true
		if x, err := t.rs.CursorViewportX(); err == nil {
			snap.CursorX = int(x)
		}
		if y, err := t.rs.CursorViewportY(); err == nil {
			snap.CursorY = int(y)
		}
	}

	if err := t.rs.RowIterator(t.ri); err != nil {
		return nil, err
	}

	for t.ri.Next() {
		if err := t.ri.Cells(t.rc); err != nil {
			continue
		}
		row := make([]Cell, 0, t.cols)
		for t.rc.Next() {
			c := Cell{}

			gs, err := t.rc.Graphemes()
			if err == nil && len(gs) > 0 {
				// A cell may hold several code points (combining
				// characters); join them.
				var sb []rune
				for _, g := range gs {
					sb = append(sb, rune(g))
				}
				c.Text = string(sb)
			}

			if st, err := t.rc.Style(); err == nil && st != nil {
				c.Bold = st.Bold()
				c.Italic = st.Italic()
				c.Inverse = st.Inverse()
				c.Under = st.Underline() != libghostty.UnderlineNone
				c.Fg = styleColorHex(st.FgColor(), colors, "")
				c.Bg = styleColorHex(st.BgColor(), colors, "")
			}

			row = append(row, c)
		}
		snap.Cells = append(snap.Cells, row)
	}

	return snap, nil
}

// Close releases the underlying resources.
func (t *Term) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.rc != nil {
		t.rc.Close()
		t.rc = nil
	}
	if t.ri != nil {
		t.ri.Close()
		t.ri = nil
	}
	if t.rs != nil {
		t.rs.Close()
		t.rs = nil
	}
	if t.term != nil {
		t.term.Close()
		t.term = nil
	}
}

// styleColorHex converts a style colour to "#rrggbb", falling back when the
// colour is unset.
func styleColorHex(sc libghostty.StyleColor, colors *libghostty.RenderStateColors, fallback string) string {
	switch sc.Tag {
	case libghostty.StyleColorRGB:
		return rgbHex(sc.RGB)
	case libghostty.StyleColorPalette:
		if colors != nil && int(sc.Palette) < len(colors.Palette) {
			return rgbHex(colors.Palette[sc.Palette])
		}
	}
	return fallback
}

func rgbHex(c libghostty.ColorRGB) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

// TerminalAvailable reports whether libghostty was compiled in.
func TerminalAvailable() bool { return true }
