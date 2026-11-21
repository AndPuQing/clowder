//go:build libghostty

package core

import (
	"fmt"
	"sync"

	"go.mitchellh.com/libghostty"
)

// Cell 是终端里的一格。
type Cell struct {
	Text string
	// Fg / Bg 是 "#rrggbb"；空字符串表示沿用默认色。
	Fg, Bg  string
	Bold    bool
	Italic  bool
	Under   bool
	Inverse bool
}

// Snapshot 是一帧屏幕。
type Snapshot struct {
	Cols, Rows    int
	Cells         [][]Cell
	CursorX       int
	CursorY       int
	CursorVisible bool
}

// Term 是 libghostty 终端内核的封装：喂 VT 字节，取屏幕快照。
type Term struct {
	mu sync.Mutex

	term *libghostty.Terminal
	rs   *libghostty.RenderState
	ri   *libghostty.RenderStateRowIterator
	rc   *libghostty.RenderStateRowCells

	cols, rows int
}

// NewTerm 创建一个终端。
func NewTerm(cols, rows int) (*Term, error) {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}

	term, err := libghostty.NewTerminal(
		libghostty.WithSize(uint16(cols), uint16(rows)),
		libghostty.WithMaxScrollbackLines(5000),
	)
	if err != nil {
		return nil, fmt.Errorf("创建终端: %w", err)
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

// Feed 写入一段 VT 数据（来自 SSH 的输出）。
func (t *Term) Feed(b []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.term == nil {
		return nil
	}
	t.term.VTWrite(b)
	return nil
}

// Resize 改变终端尺寸。
func (t *Term) Resize(cols, rows int) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.term == nil || cols <= 0 || rows <= 0 {
		return nil
	}
	t.cols, t.rows = cols, rows
	return t.term.Resize(uint16(cols), uint16(rows), 0, 0)
}

// Snapshot 取一帧屏幕。
func (t *Term) Snapshot() (*Snapshot, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.term == nil {
		return nil, fmt.Errorf("终端已关闭")
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
				// 一个单元格可能有多个码位（组合字符），拼起来
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
				c.Fg = styleColorHex(st.FgColor(), colors, "")
				c.Bg = styleColorHex(st.BgColor(), colors, "")
			}

			row = append(row, c)
		}
		snap.Cells = append(snap.Cells, row)
	}

	return snap, nil
}

// Close 释放底层资源。
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

// styleColorHex 把样式颜色转成 "#rrggbb"，未设置时返回 fallback。
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

// TerminalAvailable 报告是否编进了 libghostty。
func TerminalAvailable() bool { return true }
