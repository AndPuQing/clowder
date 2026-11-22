package ui

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	myui "github.com/egoist/mygo/ui"

	"github.com/AndPuQing/clowder/core"
)

// terminalCols and terminalRows are the initial size of a terminal
// session. The pane is wider than it is tall, so a high column count
// keeps long command lines readable.
const (
	terminalCols = 80
	terminalRows = 24

	terminalBackground = "#10141D"
	terminalText       = "#EBEFF7"
	terminalMuted      = "#A5B0C3"
)

// termSession is one interactive terminal session: an SSH shell over the
// tunnel, parsed by libghostty into a cell grid.
type termSession struct {
	term   *core.Term
	stream io.ReadWriteCloser
	resize func(cols, rows int) error

	mu      sync.Mutex
	writeMu sync.Mutex
	snap    *core.Snapshot
	err     string

	closeOnce sync.Once
}

// startTermSession opens a shell through client and feeds its output to
// libghostty. It returns nil when the terminal is unavailable or failed to
// start; the error is recorded on the device runtime so the UI can show it.
func (a *App) startTermSession(id string, client *core.Client, cols, rows int) *termSession {
	if !core.TerminalAvailable() || client == nil {
		return nil
	}

	conn, err := client.Dial(core.SSHPort, 20*time.Second)
	if err != nil {
		a.setTermError(id, "terminal: "+err.Error())
		return nil
	}

	stream, resize, err := core.SSHShellConn(conn, cols, rows)
	if err != nil {
		_ = conn.Close()
		a.setTermError(id, "terminal: "+err.Error())
		return nil
	}

	ts := &termSession{stream: stream, resize: resize}
	// The emulator answers terminal queries (device attributes, cursor
	// position); those replies travel back to the shell.
	term, err := core.NewTermWithWriter(cols, rows, ts)
	if err != nil {
		_ = stream.Close()
		a.setTermError(id, "terminal: "+err.Error())
		return nil
	}
	ts.term = term

	// Debug hook: type a command automatically to verify the whole path
	// from keystrokes to remote shell to rendered cells.
	if os.Getenv("CLOWDER_TERM_DEMO") == "1" {
		go func() {
			time.Sleep(3 * time.Second)
			ts.send("echo hello-from-ghostty; ls /\r")
		}()
	}

	go ts.readLoop(a)
	return ts
}

// readLoop reads shell output, feeds the emulator and repaints.
func (t *termSession) readLoop(a *App) {
	buf := make([]byte, 32*1024)
	for {
		n, err := t.stream.Read(buf)
		if n > 0 {
			_ = t.term.Feed(buf[:n])
			if snap, serr := t.term.Snapshot(); serr == nil {
				t.mu.Lock()
				t.snap = snap
				t.mu.Unlock()
			}
			a.refresh()
		}
		if err != nil {
			t.mu.Lock()
			if err != io.EOF {
				t.err = "connection closed: " + err.Error()
			} else {
				t.err = "connection closed"
			}
			t.mu.Unlock()
			a.refresh()
			return
		}
	}
}

// setTermError records a terminal error on the device runtime.
func (a *App) setTermError(id, msg string) {
	a.mu.Lock()
	rt := a.runtimeLocked(id)
	rt.termErr = msg
	a.mu.Unlock()
	a.refresh()
}

// Close ends the session. It is safe to call more than once.
func (t *termSession) Close() {
	if t == nil {
		return
	}
	t.closeOnce.Do(func() {
		if t.stream != nil {
			_ = t.stream.Close()
		}
		if t.term != nil {
			t.term.Close()
		}
	})
}

// snapshot returns the latest rendered screen.
func (t *termSession) snapshot() *core.Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.snap
}

// errText returns the session error, if any.
func (t *termSession) errText() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.err
}

// Write sends emulator responses (and user input) to the remote shell.
func (t *termSession) Write(b []byte) (int, error) {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	if t.stream == nil {
		return 0, io.ErrClosedPipe
	}
	return t.stream.Write(b)
}

// send writes bytes to the remote shell.
func (t *termSession) send(s string) {
	_, _ = t.Write([]byte(s))
}

// termOf returns the terminal session for a device, if one is running.
func (a *App) termOf(id string) *termSession {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.terms[id]
}

// setTerm records the terminal session for a device, closing any previous
// one. A nil session removes it.
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

// ensureTerm starts a terminal session if the device wants one and none is
// running yet. gen is the connection attempt this session belongs to; a
// session that finishes after the attempt was stopped is discarded.
func (a *App) ensureTerm(id string, gen uint64, client *core.Client) {
	if a.termOf(id) != nil {
		return
	}
	ts := a.startTermSession(id, client, terminalCols, terminalRows)
	if ts == nil {
		return
	}

	a.mu.Lock()
	rt := a.rt[id]
	current := rt != nil && rt.gen == gen
	a.mu.Unlock()
	if !current {
		ts.Close()
		return
	}

	a.setTerm(id, ts)
	a.refresh()
}

// terminalPane renders the terminal screen as a grid of styled runs.
func (a *App) terminalPane(c *myui.Context, id string) {
	ts := a.termOf(id)
	if ts == nil {
		return
	}

	pane := myui.Column(c).FillWidth().Radius(radControl).
		Background(myui.Hex(terminalBackground)).Border(1, separatorColor(c)).
		Padding(10).Gap(0)
	pane.Focusable().HandleInput(func(ev myui.InputEvent) bool {
		b := terminalInput(ev)
		if len(b) == 0 {
			return false
		}
		ts.send(string(b))
		return true
	})

	snap := ts.snapshot()
	if snap == nil {
		pane.Children(func() {
			myui.Text(c, "…").Font(fontMono).FontSize(12).
				TextColor(myui.Hex(terminalMuted))
		})
		return
	}

	cols := snap.Cols
	if cols <= 0 {
		cols = terminalCols
	}
	fontSize := float32(11)
	if cols > 90 {
		fontSize = 10.5
	}

	maxRows := 12
	if snap.Rows < maxRows {
		maxRows = snap.Rows
	}

	pane.Children(func() {
		for y, row := range snap.Cells {
			if y >= maxRows {
				break
			}
			y := y
			line := row
			myui.Row(c).Children(func() {
				a.terminalRow(c, line, y, snap, fontSize)
			})
		}
		if msg := ts.errText(); msg != "" {
			myui.Text(c, msg).FontSize(11).TextColor(myui.Hex(terminalMuted)).
				Padding(4, 0)
		}
	})
}

// terminalRow draws one row, merging adjacent cells that share a style and
// giving the cursor cell its own inverted run.
func (a *App) terminalRow(c *myui.Context, line []core.Cell, y int, snap *core.Snapshot, fontSize float32) {
	cursorAt := func(i int) bool {
		return snap.CursorVisible && y == snap.CursorY && i == snap.CursorX
	}

	i := 0
	for i < len(line) {
		if cursorAt(i) {
			fg, bg := cellColors(line[i], true)
			a.drawRun(c, []core.Cell{line[i]}, fg, bg, fontSize)
			i++
			continue
		}

		start := i
		for i < len(line) && sameStyle(line[i], line[start]) && !cursorAt(i) {
			i++
		}
		fg, bg := cellColors(line[start], false)
		a.drawRun(c, line[start:i], fg, bg, fontSize)
	}
}

// drawRun renders a run of cells that share one visual style.
func (a *App) drawRun(c *myui.Context, cells []core.Cell, fg, bg string, fontSize float32) {
	if len(cells) == 0 {
		return
	}
	var text []rune
	for _, cell := range cells {
		if cell.Text == "" {
			text = append(text, ' ')
			continue
		}
		text = append(text, []rune(cell.Text)...)
	}
	if len(text) == 0 {
		return
	}

	style := cells[0]
	// NoWrap and PreserveSpaces keep a screen row from reflowing: a terminal
	// line is positioned by cell, so wrapping it would move every cell that
	// follows, and dropping a run's trailing spaces would collapse columns.
	e := myui.Text(c, string(text)).Font(fontMono).FontSize(fontSize).
		NoWrap().PreserveSpaces().TextColor(myui.Hex(fg))
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

// cellColors resolves the foreground and background colours for a run,
// applying inverse video and the cursor block if needed.
func cellColors(cell core.Cell, cursor bool) (fg, bg string) {
	fg, bg = cell.Fg, cell.Bg
	if fg == "" {
		fg = terminalText
	}
	if cell.Inverse {
		// Swap the two inks. An unset background means "the terminal
		// background", so the inverse block takes the foreground colour.
		if bg == "" {
			bg, fg = fg, terminalBackground
		} else {
			fg, bg = bg, fg
		}
	}
	if cursor {
		// The cursor block is the cell's ink, its text the cell's background.
		text := bg
		if text == "" {
			text = terminalBackground
		}
		fg, bg = text, fg
	}
	return fg, bg
}

// sameStyle reports whether two cells can be merged into one run.
func sameStyle(a, b core.Cell) bool {
	return a.Fg == b.Fg && a.Bg == b.Bg && a.Bold == b.Bold &&
		a.Italic == b.Italic && a.Under == b.Under && a.Inverse == b.Inverse
}

// fnKeySeqs are the escape sequences for the function keys.
var fnKeySeqs = map[myui.Key]string{
	myui.KeyF1:  "\x1bOP",
	myui.KeyF2:  "\x1bOQ",
	myui.KeyF3:  "\x1bOR",
	myui.KeyF4:  "\x1bOS",
	myui.KeyF5:  "\x1b[15~",
	myui.KeyF6:  "\x1b[17~",
	myui.KeyF7:  "\x1b[18~",
	myui.KeyF8:  "\x1b[19~",
	myui.KeyF9:  "\x1b[20~",
	myui.KeyF10: "\x1b[21~",
	myui.KeyF11: "\x1b[23~",
	myui.KeyF12: "\x1b[24~",
}

// terminalInput turns one UI input event into the bytes a VT expects, or nil
// when the event is not keyboard input the terminal should take.
func terminalInput(ev myui.InputEvent) []byte {
	switch ev.Kind {
	case myui.InputText:
		if ev.Text == "" {
			return nil
		}
		return []byte(ev.Text)
	case myui.InputKeyDown:
	default:
		return nil
	}

	// Control combinations map onto the C0 bytes.
	if ev.Mods&myui.Ctrl != 0 {
		if b, ok := controlByte(ev.Key); ok {
			return []byte{b}
		}
	}

	if seq, ok := fnKeySeqs[ev.Key]; ok {
		return []byte(seq)
	}

	switch ev.Key {
	case myui.KeyEnter:
		return []byte("\r")
	case myui.KeyBackspace:
		return []byte{0x7f}
	case myui.KeyTab:
		if ev.Mods&myui.Shift != 0 {
			return []byte("\x1b[Z")
		}
		return []byte("\t")
	case myui.KeyEscape:
		return []byte{0x1b}
	case myui.KeyDelete:
		return []byte("\x1b[3~")
	case myui.KeyInsert:
		return []byte("\x1b[2~")
	case myui.KeyPageUp:
		return []byte("\x1b[5~")
	case myui.KeyPageDown:
		return []byte("\x1b[6~")
	case myui.KeyHome:
		return cursorSeq('H', ev.Mods)
	case myui.KeyEnd:
		return cursorSeq('F', ev.Mods)
	case myui.KeyUp:
		return cursorSeq('A', ev.Mods)
	case myui.KeyDown:
		return cursorSeq('B', ev.Mods)
	case myui.KeyRight:
		return cursorSeq('C', ev.Mods)
	case myui.KeyLeft:
		return cursorSeq('D', ev.Mods)
	}
	return nil
}

// cursorSeq returns a cursor movement sequence, with the modifier
// parameter xterm expects (2 shift, 3 alt, 4 both).
func cursorSeq(final byte, mods myui.Modifiers) []byte {
	m := 0
	shift := mods&myui.Shift != 0
	alt := mods&myui.Alt != 0
	switch {
	case shift && alt:
		m = 4
	case alt:
		m = 3
	case shift:
		m = 2
	}
	if m == 0 {
		return []byte{0x1b, '[', final}
	}
	return []byte(fmt.Sprintf("\x1b[1;%d%c", m, final))
}

// controlByte maps Ctrl+letter and a few Ctrl+punctuation keys onto the
// corresponding C0 byte.
func controlByte(k myui.Key) (byte, bool) {
	switch {
	case k >= myui.KeyA && k <= myui.KeyZ:
		return byte(1 + int(k-myui.KeyA)), true
	case k == myui.KeySpace:
		return 0, true
	case k == myui.KeyBracketLeft:
		return 27, true
	case k == myui.KeyBackslash:
		return 28, true
	case k == myui.KeyBracketRight:
		return 29, true
	}
	return 0, false
}
