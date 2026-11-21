package ui

import (
	"bytes"
	"testing"

	myui "github.com/egoist/mygo/ui"
)

func TestTerminalInput(t *testing.T) {
	cases := []struct {
		name string
		ev   myui.InputEvent
		want []byte
	}{
		{"text", myui.InputEvent{Kind: myui.InputText, Text: "ls -l"}, []byte("ls -l")},
		{"enter", myui.InputEvent{Kind: myui.InputKeyDown, Key: myui.KeyEnter}, []byte("\r")},
		{"backspace", myui.InputEvent{Kind: myui.InputKeyDown, Key: myui.KeyBackspace}, []byte{0x7f}},
		{"tab", myui.InputEvent{Kind: myui.InputKeyDown, Key: myui.KeyTab}, []byte("\t")},
		{"shift+tab", myui.InputEvent{Kind: myui.InputKeyDown, Key: myui.KeyTab, Mods: myui.Shift}, []byte("\x1b[Z")},
		{"escape", myui.InputEvent{Kind: myui.InputKeyDown, Key: myui.KeyEscape}, []byte{0x1b}},
		{"up", myui.InputEvent{Kind: myui.InputKeyDown, Key: myui.KeyUp}, []byte("\x1b[A")},
		{"alt+left", myui.InputEvent{Kind: myui.InputKeyDown, Key: myui.KeyLeft, Mods: myui.Alt}, []byte("\x1b[1;3D")},
		{"shift+up", myui.InputEvent{Kind: myui.InputKeyDown, Key: myui.KeyUp, Mods: myui.Shift}, []byte("\x1b[1;2A")},
		{"ctrl+c", myui.InputEvent{Kind: myui.InputKeyDown, Key: myui.KeyC, Mods: myui.Ctrl}, []byte{0x03}},
		{"ctrl+space", myui.InputEvent{Kind: myui.InputKeyDown, Key: myui.KeySpace, Mods: myui.Ctrl}, []byte{0x00}},
		{"home", myui.InputEvent{Kind: myui.InputKeyDown, Key: myui.KeyHome}, []byte("\x1b[H")},
		{"delete", myui.InputEvent{Kind: myui.InputKeyDown, Key: myui.KeyDelete}, []byte("\x1b[3~")},
		{"f5", myui.InputEvent{Kind: myui.InputKeyDown, Key: myui.KeyF5}, []byte("\x1b[15~")},
		{"plain letter waits for text", myui.InputEvent{Kind: myui.InputKeyDown, Key: myui.KeyA}, nil},
		{"pointer ignored", myui.InputEvent{Kind: myui.InputPointerDown}, nil},
		{"empty text", myui.InputEvent{Kind: myui.InputText}, nil},
	}
	for _, c := range cases {
		got := terminalInput(c.ev)
		if !bytes.Equal(got, c.want) {
			t.Fatalf("%s: terminalInput = %q, want %q", c.name, got, c.want)
		}
	}
}
