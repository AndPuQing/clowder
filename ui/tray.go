package ui

import (
	"bytes"
	"fmt"
	"image/color"
	"image/png"
	"os"

	"github.com/egoist/mygo"

	"github.com/AndPuQing/clowder/core"
)

// trayIcon renders the app mark to a PNG for the tray and the window icon.
// It comes from the same SVG as the mark in the sidebar, so the two never
// drift apart.
func trayIcon() []byte {
	mark := icon("mark")
	if mark == nil {
		return nil
	}
	img := mark.Image(32, 32, color.White)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil
	}
	return buf.Bytes()
}

// setupTray creates the tray icon, if the desktop provides a tray.
func (a *App) setupTray() {
	icon := trayIcon()
	if icon == nil {
		return
	}

	tray, err := mygo.NewTray(mygo.TrayOptions{
		ID:   "clowder",
		Icon: icon,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "clowder: tray unavailable: %v\n", err)
		return
	}

	a.mu.Lock()
	a.tray = tray
	a.mu.Unlock()

	tray.OnClick(func() { a.showWindow() })
	a.rebuildTrayMenu()
}

// showWindow brings the window back to the front.
func (a *App) showWindow() {
	mygo.RunOnMain(func() {
		a.mu.Lock()
		w := a.window
		a.mu.Unlock()
		if w == nil {
			return
		}
		w.Show()
		w.Focus()
	})
}

// rebuildTrayMenu rebuilds the tray menu from the current devices and
// their runtime state.
func (a *App) rebuildTrayMenu() {
	a.mu.Lock()
	tray := a.tray
	devices := append([]*core.Device(nil), a.devices...)
	type flags struct {
		busy, srv, cli bool
		errMsg         string
	}
	state := make(map[string]flags, len(devices))
	running := 0
	for _, d := range devices {
		rt := a.rt[d.ID]
		f := flags{}
		if rt != nil {
			f = flags{
				busy:   rt.busy,
				srv:    rt.srv != nil,
				cli:    rt.cli != nil,
				errMsg: rt.errMsg,
			}
			if f.srv || f.cli {
				running++
			}
		}
		state[d.ID] = f
	}
	a.mu.Unlock()

	if tray == nil {
		return
	}

	items := []*mygo.MenuItem{
		{Label: T("tray.show"), Click: func(*mygo.MenuItem, *mygo.Window) { a.showWindow() }},
		mygo.Separator(),
	}

	for _, d := range devices {
		d := d
		f := state[d.ID]
		status, _ := a.deviceStatus(d)

		sub := []*mygo.MenuItem{}
		if d.Role == core.RoleHost {
			if f.srv {
				sub = append(sub, &mygo.MenuItem{Label: T("action.stop"), Click: func(*mygo.MenuItem, *mygo.Window) { a.stopHost(d.ID) }})
			} else if !f.busy {
				sub = append(sub, &mygo.MenuItem{Label: T("action.ticket"), Click: func(*mygo.MenuItem, *mygo.Window) { a.startHost(d.ID) }})
			}
		} else {
			if f.cli {
				sub = append(sub, &mygo.MenuItem{Label: T("action.disconnect"), Click: func(*mygo.MenuItem, *mygo.Window) { a.stopPeer(d.ID) }})
			} else if !f.busy {
				sub = append(sub, &mygo.MenuItem{Label: T("action.connect"), Click: func(*mygo.MenuItem, *mygo.Window) { a.startPeer(d.ID) }})
			}
		}
		if f.errMsg != "" {
			sub = append(sub, &mygo.MenuItem{Label: "Error: " + truncate(f.errMsg, 40), Disabled: true})
		}

		items = append(items, &mygo.MenuItem{
			Label:   fmt.Sprintf("%s — %s", d.Name, status),
			Submenu: sub,
		})
	}

	items = append(items,
		mygo.Separator(),
		&mygo.MenuItem{Label: T("tray.quit"), Click: func(*mygo.MenuItem, *mygo.Window) { a.quit() }},
	)

	menu := mygo.NewMenu(items)
	tip := T("tray.idle", "count", len(devices))
	if running > 0 {
		tip = T("tray.running", "count", running)
	}

	mygo.RunOnMain(func() {
		tray.SetMenu(menu)
		tray.SetToolTip(tip)
	})
}

// quit writes pending changes and exits for real.
func (a *App) quit() {
	a.persistNow()
	a.mu.Lock()
	a.quitting = true
	a.mu.Unlock()
	mygo.RunOnMain(func() { mygo.App.Quit() })
}

// truncate shortens text for menu labels.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
