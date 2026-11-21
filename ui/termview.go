package ui

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/egoist/mygo"
	myui "github.com/egoist/mygo/ui"

	"github.com/AndPuQing/clowder/core"
)

// termSession 是一条交互式终端会话。
type termSession struct {
	term   *core.Term
	stream io.ReadWriteCloser
	resize func(cols, rows int) error

	mu   sync.Mutex
	snap *core.Snapshot
	err  string
}

// startTermSession 通过隧道打开 shell，并把输出喂给 libghostty 内核。
func (a *App) startTermSession(id, addr string, cols, rows int) *termSession {
	if !core.TerminalAvailable() {
		return nil
	}

	ctx := context.Background()
	stream, resize, err := core.SSHShell(ctx, addr, cols, rows)
	if err != nil {
		a.mu.Lock()
		rt := a.ensureRuntime(id)
		rt.termErr = "终端启动失败：" + err.Error()
		a.mu.Unlock()
		return nil
	}

	term, err := core.NewTerm(cols, rows)
	if err != nil {
		stream.Close()
		return nil
	}

	ts := &termSession{term: term, stream: stream, resize: resize}

	// 调试：自动敲一条命令，验证「输入 → 远端 shell → 渲染」整条链路
	if os.Getenv("CLOWDER_TERM_DEMO") == "1" {
		go func() {
			time.Sleep(3 * time.Second)
			ts.send("echo hello-from-ghostty; ls /\r")
		}()
	}

	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := stream.Read(buf)
			if n > 0 {
				_ = ts.term.Feed(buf[:n])
				if snap, serr := ts.term.Snapshot(); serr == nil {
					ts.mu.Lock()
					ts.snap = snap
					ts.mu.Unlock()
				}
				a.refresh()
			}
			if err != nil {
				ts.mu.Lock()
				ts.err = "连接已结束"
				ts.mu.Unlock()
				a.refresh()
				return
			}
		}
	}()

	return ts
}

// closeTermSession 结束会话。
func (t *termSession) Close() {
	if t == nil {
		return
	}
	if t.stream != nil {
		t.stream.Close()
	}
	if t.term != nil {
		t.term.Close()
	}
}

// snapshot 取当前屏幕。
func (t *termSession) snapshot() *core.Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.snap
}

// send 把按键数据写进会话。
func (t *termSession) send(s string) {
	if t.stream == nil {
		return
	}
	_, _ = t.stream.Write([]byte(s))
}

// termOf 返回该设备的终端会话。
func (a *App) termOf(id string) *termSession {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.terms[id]
}

// setTerm 记录终端会话。
func (a *App) setTerm(id string, ts *termSession) {
	a.mu.Lock()
	if a.terms == nil {
		a.terms = map[string]*termSession{}
	}
	old := a.terms[id]
	if ts == nil {
		delete(a.terms, id)
	} else {
		a.terms[id] = ts
	}
	a.mu.Unlock()
	if old != nil {
		old.Close()
	}
}

// terminalPane 画一个终端。
func (a *App) terminalPane(c *myui.Context, d core.Device) {
	ts := a.termOf(d.ID)
	if ts == nil {
		return
	}

	snap := ts.snapshot()
	if snap == nil {
		myui.Text(c, "…").Font(fontMono).FontSize(12).TextColor(myui.Hex(colDim))
		return
	}

	cols := snap.Cols
	if cols <= 0 {
		cols = 80
	}
	fontSize := float32(12)
	if cols > 90 {
		fontSize = 10.5
	}

	maxRows := 11
	if snap.Rows < maxRows {
		maxRows = snap.Rows
	}

	myui.Column(c).Padding(10).Radius(radCard).
		Background(myui.Hex("#0B1512")).Width(620).Gap(0).Children(func() {
		for y, row := range snap.Cells {
			if y >= maxRows {
				break
			}
			y := y
			line := row
			_ = y
			myui.Row(c).Children(func() {
				// 把同样式的连续单元格合并成一段，减少元素数量
				i := 0
				for i < len(line) {
					start := i
					style := line[i]
					for i < len(line) && sameStyle(line[i], style) {
						i++
					}

					text := ""
					for _, cell := range line[start:i] {
						if cell.Text == "" {
							text += " "
						} else {
							text += cell.Text
						}
					}

					fg := style.Fg
					bg := style.Bg
					if fg == "" {
						fg = colCream
					}
					if snap.CursorVisible && y == snap.CursorY &&
						snap.CursorX >= start && snap.CursorX < i {
						// 光标所在段整段反色；底色为空时给一个兜底色，
						// 否则 Hex("") 会 panic。
						if bg == "" {
							bg = colCream
							fg = "#0B1512"
						} else {
							fg, bg = bg, fg
						}
					}

					e := myui.Text(c, text).Font(fontMono).FontSize(fontSize).
						TextColor(myui.Hex(fg))
					if bg != "" {
						e.Background(myui.Hex(bg))
					}
					if style.Bold {
						e.Bold()
					}
					if style.Italic {
						e.Italic()
					}
					if style.Under {
						e.Underline()
					}
				}
			})
		}
	})

	if msg := ts.err; msg != "" {
		myui.Text(c, msg).FontSize(11.5).TextColor(myui.Hex(colDim)).
			Padding(6, 0)
	}
}

// sameStyle 判断两格能否合并成同一段文本。
func sameStyle(a, b core.Cell) bool {
	return a.Fg == b.Fg && a.Bg == b.Bg && a.Bold == b.Bold &&
		a.Italic == b.Italic && a.Under == b.Under
}

// ensureTerm 在需要时创建终端会话。
func (a *App) ensureTerm(id, addr string) {
	if a.termOf(id) != nil {
		return
	}
	ts := a.startTermSession(id, addr, 86, 24)
	if ts != nil {
		a.setTerm(id, ts)
	}
}

var _ = fmt.Sprintf
var _ = mygo.RunOnMain
