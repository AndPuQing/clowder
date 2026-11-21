// Package ui implements Clowder's desktop interface.
//
// Layout: a rail of devices on the left, the selected device's detail on
// the right. Visual language: a quiet green-and-cream palette with a
// single gold accent; the tunnel address is the one thing on screen.
package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	myui "github.com/egoist/mygo/ui"
	"github.com/skip2/go-qrcode"

	"github.com/AndPuQing/clowder/core"
)

const (
	fontUI   = "Noto Sans CJK SC"
	fontMono = "Noto Sans Mono CJK SC"

	// The palette is a dark, slightly green-neutral set of surfaces, one warm
	// accent and two semantic colours. Surfaces step up in value so panels
	// read as raised; hairlines are the only ornament.
	colBg      = "#101614"
	colBgTop   = "#151D1A"
	colRail    = "#0B100E"
	colRailTop = "#0F1613"

	colPanel    = "#18211E"
	colPanelHi  = "#1D2925"
	colHover    = "#161F1C"
	colSelected = "#1B2723"

	colLine   = "#2A3732"
	colLineHi = "#35463F"

	colCream = "#ECEAE3"
	colText2 = "#C3CCC7"
	colDim   = "#8A9992"

	colGold   = "#D6A93C"
	colGoldHi = "#E5BB55"
	colGoldLo = "#BE9129"
	colInk    = "#171A12"

	colGood   = "#6FC49A"
	colDanger = "#E06C5B"

	// terminal background, darker than the window
	termBg = "#0A0F0D"

	// Rounded corners: controls 8, cards 12, rows 8, panels 16
	radControl = 8
	radCard    = 12
	radRow     = 8
	radPanel   = 16
)

// demoAddr is a sample address used by the demo/screenshot mode. It is
// well formed but nobody holds its key, so it can never connect.
const demoAddr = "kHBrZxJsdB_CPdGdTQxUzc3JDgTJW_mmxyxbhOtfnPYqAeJZzzlYcBUHIYwlLnEdxTlLWpOWFKufsMabNrWXsBcOYnINedlWyHFCwCrAhDLXhwdbHWSYFVOMRBrkaKGtfsQfTLZLhWLKjPqPwdDlj"

const (
	// saveDelay debounces writes of device and settings changes, so typing
	// in a field does not rewrite the config on every keystroke.
	saveDelay = 400 * time.Millisecond

	// maxFileEntries bounds one directory listing in the UI.
	maxFileEntries = 200
)

// runtime holds per-device state that is never written to disk.
type runtime struct {
	srv  *core.Server
	cli  *core.Client
	addr string

	errMsg string
	busy   bool
	lat    time.Duration

	// clientKey is this peer's node public key, shown so the host can
	// allow-list it.
	clientKey string

	qrAddr string
	qrSrc  myui.ImageSource

	termCmd string
	termOut string
	termErr string

	// files is the SFTP session for browsing the host's shared directory.
	files  *core.Files
	browse browseState

	// gen identifies the current connection attempt. Results from an
	// attempt that was stopped or superseded carry an older value and are
	// discarded (and cleaned up) instead of being published.
	gen uint64
}

// drafts holds the widget bindings for a device's editable fields. Widgets
// keep the pointers they are given across renders, so every pointer must
// point at stable storage rather than into a slice that can be reallocated.
type drafts struct {
	name     *string
	addr     *string
	exposes  []*string
	forwards []*string
	allowed  *string
	termCmd  *string
	files    *string
}

// browseState is the peer's view of the host's shared files.
type browseState struct {
	path    string
	entries []core.FileEntry
	errMsg  string
	note    string
	loading bool
	seq     uint64 // invalidates results of superseded directory loads
}

// App is the interface state.
type App struct {
	mu      sync.Mutex
	window  *mygo.Window
	devices []*core.Device
	sel     string
	rt      map[string]*runtime
	drafts  map[string]*drafts
	copied  time.Time
	theme   *myui.Theme

	tray     *mygo.Tray
	quitting bool

	terms map[string]*termSession

	settings     core.Settings
	showSettings bool
	socksPortIn  string
	derpIn       string
	derpRegionIn string

	// savedDevices/savedSettings mirror what is on disk. The view compares
	// against them so edits made through widgets (which have no change
	// callback of their own) are still written back.
	savedDevices  string
	savedSettings string
	saveTimer     *time.Timer
	saveMu        sync.Mutex // serialises writes to disk
}

// New loads the device list and builds the interface state.
func New() *App {
	initI18n()

	settings := core.LoadSettings()
	if settings.Language != "" {
		mygo.App.SetLocale(settings.Language)
	}

	a := &App{
		rt:           map[string]*runtime{},
		drafts:       map[string]*drafts{},
		settings:     settings,
		socksPortIn:  fmt.Sprint(settings.SocksPort),
		derpIn:       settings.DERPMapURL,
		derpRegionIn: fmt.Sprint(settings.DERPMapRegion),
		theme: func() *myui.Theme {
			t := myui.DarkTheme().WithAccent(myui.Hex(colGold))
			t.Radius = radControl // one radius for buttons, fields and switches
			return t
		}(),
	}
	if list, err := core.LoadDevices(); err == nil {
		for i := range list {
			a.devices = append(a.devices, &list[i])
		}
	}
	if len(a.devices) == 0 {
		a.devices = []*core.Device{newDevice(core.RoleHost, T("device.host"))}
		_ = core.SaveDevices(deviceValues(a.devices))
	}
	if len(a.devices) > 0 {
		a.sel = a.devices[0].ID
	}
	if addr := os.Getenv("CLOWDER_DEMO_ADDR"); addr != "" {
		peer := newDevice(core.RolePeer, T("device.peer")+" 1")
		peer.Addr = addr
		peer.Term = true
		peer.BrowseFiles = true
		a.devices = append(a.devices, peer)
		a.sel = peer.ID
	}
	if os.Getenv("CLOWDER_DEMO") == "1" {
		peer := newDevice(core.RolePeer, T("device.peer")+" 1")
		peer.Addr = demoAddr
		a.devices = append(a.devices, peer)
	}
	a.savedDevices = marshalDevices(a.devices)
	a.savedSettings = marshalSettings(a.settings)
	return a
}

// newDevice returns a device with sensible defaults for its role.
func newDevice(role, name string) *core.Device {
	d := &core.Device{
		ID:        core.NewDeviceID(),
		Name:      name,
		Role:      role,
		CreatedAt: time.Now().UTC(),
	}
	if role == core.RoleHost {
		d.Exposes = []string{"8080"}
	} else {
		d.Forwards = []string{"9000:8080"}
	}
	return d
}

// Run starts the desktop application.
func (a *App) Run() error {
	if os.Getenv("CLOWDER_PAGE") == "settings" {
		a.showSettings = true
	}
	if os.Getenv("CLOWDER_TAB") == "client" {
		// Kept for the old headless screenshot recipe.
		a.mu.Lock()
		for _, d := range a.devices {
			if d.Role == core.RolePeer {
				a.sel = d.ID
				break
			}
		}
		a.mu.Unlock()
	}
	mygo.App.WhenReady(func() {
		a.mu.Lock()
		lang := a.settings.Language
		a.mu.Unlock()
		mygo.App.SetLocale(lang)

		w := mygo.NewWindow(mygo.WindowOptions{
			Title:         "Clowder",
			Width:         1000,
			Height:        720,
			MinWidth:      880,
			MinHeight:     640,
			TitleBarStyle: mygo.TitleBarHidden,
			Content:       myui.View(a.view),
		})
		if pngIcon := trayIcon(); pngIcon != nil {
			_ = w.SetIcon(pngIcon)
		}

		// Publish the window under the lock: background goroutines (the
		// debounced save, refresh, the tray) read it.
		a.mu.Lock()
		a.window = w
		a.mu.Unlock()

		a.setupTray()
		w.OnClose(func(e *mygo.CloseEvent) {
			a.mu.Lock()
			quitting := a.quitting
			hide := a.settings.CloseToTray
			hasTray := a.tray != nil
			a.mu.Unlock()
			// Always write pending edits before the window goes away.
			a.persistNow()
			if quitting || !hasTray || !hide {
				return // no tray, or the user turned hiding off: quit
			}
			e.PreventDefault()
			w.Hide()
		})

		if v := os.Getenv("CLOWDER_AUTOSTART"); v != "" {
			go func() {
				time.Sleep(1200 * time.Millisecond)
				a.debugAutoStart(v)
			}()
		}
	})
	return mygo.App.Run()
}

// refresh repaints the window and rebuilds the tray menu.
func (a *App) refresh() {
	a.mu.Lock()
	w := a.window
	a.mu.Unlock()
	if w == nil {
		return
	}
	mygo.RunOnMain(func() { w.Invalidate() })
	go a.rebuildTrayMenu()
}

// ---------------------------------------------------------------- views

// view draws the whole window.
func (a *App) view(c *myui.Context) {
	c.SetTheme(a.theme)

	a.mu.Lock()
	devices := append([]*core.Device(nil), a.devices...)
	sel := a.sel
	settings := a.showSettings
	a.mu.Unlock()

	myui.Row(c).Fill().
		LinearGradient(myui.LinearGradient{
			From:  myui.Hex(colBgTop),
			To:    myui.Hex(colBg),
			Angle: 150,
		}).
		Children(func() {
			a.rail(c, devices, sel, settings)

			myui.Row(c).Width(1).FillHeight().Background(myui.Hex(colLine))

			myui.Column(c).Grow(1).FillHeight().Children(func() {
				// The top strip is the window's drag handle.
				myui.Row(c).Height(34).FillWidth().DragWindow()

				myui.Column(c).Grow(1).FillWidth().Padding(30, 28).
					Children(func() {
						myui.Column(c).FillWidth().MaxWidth(720).Gap(22).
							Children(func() {
								if settings {
									a.settingsPane(c)
									return
								}

								dev, ok := findDevice(devices, sel)
								if !ok {
									myui.Row(c).Grow(1)
									iconText(c, "mark", T("empty.noSelection"), 18, 13,
										myui.Hex(colDim))
									myui.Row(c).Grow(1)
									return
								}
								// The detail can outgrow the window (terminal,
								// file browser), so it scrolls.
								myui.Scroll(c).Grow(1).FillWidth().Children(func() {
									myui.Column(c).Gap(22).FillWidth().Children(func() {
										if dev.Role == core.RoleHost {
											a.hostDetail(c, dev)
										} else {
											a.peerDetail(c, dev)
										}
										myui.Row(c).Height(12)
									})
								})
							})
					})
			})
		})

	// Widgets write straight into the bound values; catch any change here
	// and schedule a debounced write.
	a.persistIfChanged()
}

// rail draws the device list on the left.
func (a *App) rail(c *myui.Context, devices []*core.Device, sel string, settings bool) {
	cream := myui.Hex(colCream)
	text2 := myui.Hex(colText2)
	dim := myui.Hex(colDim)
	gold := myui.Hex(colGold)

	myui.Column(c).Width(252).FillHeight().
		LinearGradient(myui.LinearGradient{
			From:  myui.Hex(colRailTop),
			To:    myui.Hex(colRail),
			Angle: 180,
		}).
		BorderWidth(0, 1, 0, 0).BorderColor(myui.Hex(colLine)).
		Children(func() {
			myui.Row(c).Height(34).FillWidth().DragWindow()

			// Brand.
			myui.Row(c).FillWidth().Padding(8, 16).Gap(10).AlignItems(myui.Center).
				Children(func() {
					tile := myui.Box(c).Size(26, 26).Radius(7).Center().
						Background(gold).Shrink(0)
					tile.Children(func() { iconEl(c, "mark", 17, myui.Hex(colInk)) })
					myui.Text(c, "Clowder").Font(fontUI).FontSize(15).
						FontWeight(700).LetterSpacing(-0.2).TextColor(cream)
				})

			myui.Column(c).FillWidth().Padding(10, 12).Gap(3).Children(func() {
				myui.Row(c).Padding(4, 8).Children(func() {
					sectionLabel(c, T("nav.machines"))
				})
				for _, d := range devices {
					d := d
					selected := d.ID == sel && !settings
					status, statusColor := a.deviceStatus(d)
					row := myui.Row(c).FillWidth().Gap(10).AlignItems(myui.Center).
						Padding(8, 9).Radius(radRow)
					if selected {
						row.Background(myui.Hex(colSelected)).
							BorderWidth(0, 0, 0, 2).BorderColor(gold)
					} else if row.Hovered() {
						row.Background(myui.Hex(colHover))
					}
					row.Cursor(myui.CursorPointer)
					row.Children(func() {
						tile := myui.Box(c).Size(28, 28).Radius(7).Center().Shrink(0)
						if selected {
							tile.Background(myui.Hex(colPanelHi))
						} else {
							tile.Background(myui.Hex(colPanel))
						}
						tile.Children(func() {
							roleIcon := "host"
							if d.Role == core.RolePeer {
								roleIcon = "peer"
							}
							fg := text2
							if selected {
								fg = cream
							}
							iconEl(c, roleIcon, 16, fg)
						})
						myui.Column(c).Gap(3).Grow(1).Children(func() {
							name := myui.Text(c, d.Name).FontSize(13).
								TextColor(text2).SingleLine().TruncateMiddle()
							if selected {
								name.FontWeight(600).TextColor(cream)
							}
							myui.Row(c).Gap(5).AlignItems(myui.Center).Children(func() {
								myui.Box(c).Size(5, 5).Radius(3).
									Background(statusColor).Shrink(0)
								myui.Text(c, status).FontSize(10.5).
									TextColor(dim).SingleLine()
							})
						})
					})
					row.OnClick(func() {
						a.mu.Lock()
						a.sel = d.ID
						a.showSettings = false
						a.mu.Unlock()
						a.refresh()
					})
				}
			})

			myui.Row(c).Grow(1)

			// Footer: creating a machine, then settings.
			myui.Column(c).FillWidth().Padding(10, 12).Gap(2).Children(func() {
				myui.Row(c).Height(1).FillWidth().
					Background(myui.Hex(colLine)).Padding(0, 8)

				addRow := func(name, label string, fn func()) {
					row := myui.Row(c).FillWidth().Gap(9).AlignItems(myui.Center).
						Padding(7, 9).Radius(radRow)
					row.Cursor(myui.CursorPointer)
					if row.Hovered() {
						row.Background(myui.Hex(colHover))
					}
					row.Children(func() {
						iconEl(c, name, 15, dim)
						myui.Text(c, label).FontSize(12.5).
							TextColor(text2).SingleLine()
					})
					row.OnClick(fn)
				}
				addRow("plus", T("nav.host"), func() { a.addDevice(core.RoleHost) })
				addRow("plus", T("nav.join"), func() { a.addDevice(core.RolePeer) })

				row := myui.Row(c).FillWidth().Gap(9).AlignItems(myui.Center).
					Padding(7, 9).Radius(radRow)
				row.Cursor(myui.CursorPointer)
				fg := text2
				if settings {
					row.Background(myui.Hex(colSelected))
					fg = cream
				} else if row.Hovered() {
					row.Background(myui.Hex(colHover))
				}
				row.Children(func() {
					iconEl(c, "settings", 15, fg)
					myui.Text(c, T("nav.settings")).FontSize(12.5).
						TextColor(fg).SingleLine()
				})
				row.OnClick(func() {
					a.mu.Lock()
					a.showSettings = true
					a.mu.Unlock()
					a.refresh()
				})
			})
		})
}

// hostDetail draws the "share a port" side of a device.
func (a *App) hostDetail(c *myui.Context, d *core.Device) {
	cream := myui.Hex(colCream)
	text2 := myui.Hex(colText2)
	dim := myui.Hex(colDim)
	gold := myui.Hex(colGold)

	// Sharing files is part of the SSH service; keep the two switches
	// consistent when SSH is turned off.
	if !d.SSH && d.Files {
		d.Files = false
	}

	rt := a.runtimeSnapshot(d.ID)
	running := rt.srv != nil
	addr := rt.addr
	dr := a.deviceDrafts(d)
	status, statusColor := a.deviceStatus(d)

	// Header.
	myui.Row(c).Gap(16).AlignItems(myui.Center).Children(func() {
		headerTile(c, "host", gold)
		myui.Column(c).Gap(6).Grow(1).Children(func() {
			if myui.TextInputBase(c, dr.name).Font(fontUI).FontSize(22).
				FontWeight(700).LetterSpacing(-0.4).TextColor(cream).FillWidth().MaxWidth(300).Changed() {
				a.renameDevice(d, dr, *dr.name)
			}
			statusChip(c, status, statusColor)
		})

		if running {
			if actionButton(c, btnQuiet, "stop", T("action.stop")).Clicked() {
				a.stopHost(d.ID)
			}
		} else if !rt.busy {
			if actionButton(c, btnSolid, "share", T("action.ticket")).Clicked() {
				a.startHost(d.ID)
			}
		}
		if actionButton(c, btnDanger, "trash", T("action.remove")).Clicked() {
			a.removeDevice(d.ID)
		}
	})

	if rt.errMsg != "" {
		notice(c, "alert", rt.errMsg, myui.Hex(colDanger))
	}

	// The address, once the host is up.
	if running && addr != "" {
		ticket := appear("ticket", panel(c, 0).Padding(18))
		ticket.Children(func() {
			myui.Row(c).Gap(18).Children(func() {
				box := myui.Column(c).Padding(8).Background(myui.Hex("#FFFFFF")).
					Radius(radControl).Shrink(0)
				box.Children(func() {
					if src := a.qrImage(d.ID, addr); src != nil {
						myui.Image(c, src).Size(136, 136)
					}
				})

				myui.Column(c).Gap(10).Grow(1).Children(func() {
					sectionLabel(c, T("ticket.title"))
					myui.Text(c, hardWrap(addr, 66)).Font(fontMono).FontSize(10.5).
						TextColor(text2)
					myui.Row(c).Height(2)
					label := T("action.copy")
					if time.Since(a.copiedTime()) < 2*time.Second {
						label = T("action.copied")
					}
					myui.Row(c).Children(func() {
						if actionButton(c, btnSolid, "copy", label).Clicked() {
							_ = mygo.Clipboard.Write(transfer.TextData(addr))
							a.markCopied()
						}
					})
				})
			})
		})
	}

	// Ports.
	ports := panel(c, 0).Padding(16).Gap(10)
	ports.Children(func() {
		myui.Row(c).AlignItems(myui.Center).Children(func() {
			sectionLabel(c, T("section.ports"))
			myui.Row(c).Grow(1)
			if !running {
				if iconButton(c, "plus", T("action.addPort"), dim).Clicked() {
					a.addExpose(d.ID)
				}
			}
		})
		for j := range d.Exposes {
			j := j
			myui.Row(c).FillWidth().Gap(8).AlignItems(myui.Center).Children(func() {
				iconEl(c, "hash", 15, dim)
				if running {
					myui.Text(c, d.Exposes[j]).Font(fontMono).FontSize(12.5).
						TextColor(text2).Grow(1)
					return
				}
				if myui.TextInput(c, dr.exposes[j]).Width(300).
					Placeholder(T("placeholder.expose")).Changed() {
					d.Exposes[j] = *dr.exposes[j]
					a.persistSoon()
				}
				myui.Row(c).Grow(1)
				if len(d.Exposes) > 1 {
					if iconButton(c, "x", T("action.removeItem"), dim).Clicked() {
						a.removeExpose(d.ID, j)
					}
				}
			})
		}
		if !running {
			myui.Row(c).Height(1).FillWidth().Background(myui.Hex(colLine))
			switchRow(c, "key", T("opt.persistent"), &d.Persistent)
		}
	})

	if !running {
		opts := panel(c, 0).Padding(6, 0).Gap(1).
			Dividers(1, myui.Hex(colLine))
		opts.Children(func() {
			switchRow(c, "radio", T("opt.udp"), &d.UDP)
			switchRow(c, "globe", T("opt.allowProxy"), &d.AllowProxy)
			switchRow(c, "terminal", T("opt.ssh"), &d.SSH)
			if d.SSH {
				switchRow(c, "folder", T("opt.files"), &d.Files)
				if d.Files {
					myui.Column(c).Gap(6).Padding(10, 36).Children(func() {
						fieldLabel(c, T("opt.filesDir"))
						if myui.TextInput(c, dr.files).FillWidth().
							Placeholder(T("placeholder.filesDir")).Changed() {
							d.FileDir = *dr.files
							a.persistSoon()
						}
					})
				}
			}
			myui.Column(c).Gap(6).Padding(12, 12).Children(func() {
				fieldLabel(c, T("opt.allowedClients"))
				if myui.TextInput(c, dr.allowed).FillWidth().
					Placeholder(T("placeholder.allowedClients")).Changed() {
					d.AllowedClients = splitKeys(*dr.allowed)
					a.persistSoon()
				}
			})
		})
	}
}

// peerDetail draws the "connect to a peer" side of a device.
func (a *App) peerDetail(c *myui.Context, d *core.Device) {
	cream := myui.Hex(colCream)
	text2 := myui.Hex(colText2)
	dim := myui.Hex(colDim)
	gold := myui.Hex(colGold)

	rt := a.runtimeSnapshot(d.ID)
	connected := rt.cli != nil
	dr := a.deviceDrafts(d)
	status, statusColor := a.deviceStatus(d)

	// Header.
	myui.Row(c).Gap(16).AlignItems(myui.Center).Children(func() {
		headerTile(c, "peer", gold)
		myui.Column(c).Gap(6).Grow(1).Children(func() {
			if myui.TextInputBase(c, dr.name).Font(fontUI).FontSize(22).
				FontWeight(700).LetterSpacing(-0.4).TextColor(cream).FillWidth().MaxWidth(300).Changed() {
				a.renameDevice(d, dr, *dr.name)
			}
			myui.Row(c).Gap(10).AlignItems(myui.Center).Children(func() {
				statusChip(c, status, statusColor)
				if connected {
					myui.Text(c, rt.lat.Round(time.Millisecond).String()).
						Font(fontMono).FontSize(10.5).FontFeatures("tnum").TextColor(dim)
				}
			})
		})

		if connected {
			if actionButton(c, btnQuiet, "x", T("action.disconnect")).Clicked() {
				a.stopPeer(d.ID)
			}
		} else if !rt.busy {
			if actionButton(c, btnSolid, "connect", T("action.connect")).Clicked() {
				a.startPeer(d.ID)
			}
		}
		if actionButton(c, btnDanger, "trash", T("action.remove")).Clicked() {
			a.removeDevice(d.ID)
		}
	})

	if rt.errMsg != "" {
		notice(c, "alert", rt.errMsg, myui.Hex(colDanger))
	}

	// Address.
	address := panel(c, 0).Padding(16).Gap(9)
	address.Children(func() {
		row := myui.Row(c).Gap(8).AlignItems(myui.Center)
		row.Children(func() {
			iconEl(c, "connect", 15, dim)
			sectionLabel(c, T("section.peerAddr"))
			myui.Row(c).Grow(1)
			if connected {
				if iconButton(c, "copy", T("action.copy"), dim).Clicked() {
					_ = mygo.Clipboard.Write(transfer.TextData(d.Addr))
					a.markCopied()
				}
			}
		})
		if connected {
			myui.Text(c, hardWrap(d.Addr, 92)).Font(fontMono).FontSize(10.5).
				TextColor(text2)
		} else if myui.TextInput(c, dr.addr).FillWidth().
			Placeholder(T("placeholder.addr")).Changed() {
			d.Addr = *dr.addr
			a.persistSoon()
		}
	})

	// Client key, so the host can allow-list this peer.
	if !connected {
		if key := a.ensureClientKey(d.ID); key != "" {
			keyCard := panel(c, 0).Padding(16).Gap(8)
			keyCard.Children(func() {
				myui.Row(c).AlignItems(myui.Center).Children(func() {
					iconText(c, "key", T("peer.clientKey"), 15, 11, dim)
					myui.Row(c).Grow(1)
					if iconButton(c, "copy", T("action.copyKey"), dim).Clicked() {
						_ = mygo.Clipboard.Write(transfer.TextData(key))
						a.markCopied()
					}
				})
				myui.Text(c, hardWrap(key, 64)).Font(fontMono).FontSize(10.5).
					TextColor(text2)
			})
		}
	}

	// Port mapping.
	mapping := panel(c, 0).Padding(16).Gap(10)
	mapping.Children(func() {
		myui.Row(c).AlignItems(myui.Center).Children(func() {
			sectionLabel(c, T("section.mapping"))
			myui.Row(c).Grow(1)
			if !connected {
				if iconButton(c, "plus", T("action.addMapping"), dim).Clicked() {
					a.addForward(d.ID)
				}
			}
		})
		for j := range d.Forwards {
			j := j
			myui.Row(c).FillWidth().Gap(8).AlignItems(myui.Center).Children(func() {
				iconEl(c, "connect", 15, dim)
				if connected {
					myui.Text(c, d.Forwards[j]).Font(fontMono).FontSize(12.5).
						TextColor(text2).Grow(1)
					return
				}
				if myui.TextInput(c, dr.forwards[j]).Width(300).
					Placeholder(T("placeholder.forward")).Changed() {
					d.Forwards[j] = *dr.forwards[j]
					a.persistSoon()
				}
				myui.Row(c).Grow(1)
				if len(d.Forwards) > 1 {
					if iconButton(c, "x", T("action.removeItem"), dim).Clicked() {
						a.removeForward(d.ID, j)
					}
				}
			})
		}
	})

	if !connected {
		opts := panel(c, 0).Padding(6, 0).Gap(1).
			Dividers(1, myui.Hex(colLine))
		opts.Children(func() {
			switchRow(c, "radio", T("opt.udp"), &d.UDP)
			switchRow(c, "globe", T("opt.socks"), &d.Socks)
			if d.Socks {
				myui.Row(c).Gap(9).AlignItems(myui.Center).Padding(2, 38).Children(func() {
					iconEl(c, "network", 14, dim)
					myui.Text(c, a.socksAddr()).Font(fontMono).FontSize(10.5).
						TextColor(text2)
				})
			}
			switchRow(c, "folder", T("opt.browse"), &d.BrowseFiles)
			if core.TerminalAvailable() {
				switchRow(c, "terminal", T("opt.term"), &d.Term)
			}
		})
	}

	// Interactive terminal.
	if connected && a.termOf(d.ID) != nil {
		term := appear("terminal", panel(c, 0).Padding(16).Gap(10))
		term.Children(func() {
			iconText(c, "terminal", T("term.live"), 15, 11, dim)
			a.terminalPane(c, d.ID)
			myui.Row(c).Gap(8).Children(func() {
				myui.TextInput(c, dr.termCmd).Grow(1).
					Placeholder(T("term.placeholder"))
				if actionButton(c, btnSolid, "send", T("term.send")).Clicked() {
					a.sendTermLine(d.ID)
				}
			})
		})
	}

	if connected && d.BrowseFiles {
		a.filesPanel(c, d.ID)
	}
	// Remote shell, when the tunnel is up and no interactive terminal runs.
	if connected && a.termOf(d.ID) == nil {
		shell := appear("shell", panel(c, 0).Padding(16).Gap(10))
		shell.Children(func() {
			iconText(c, "terminal", T("term.title"), 15, 11, dim)
			myui.Row(c).Gap(8).Children(func() {
				myui.TextInput(c, dr.termCmd).Grow(1).
					Placeholder(T("term.placeholder"))
				if actionButton(c, btnSolid, "play", T("term.run")).Clicked() {
					a.runRemoteCommand(d.ID)
				}
			})
			body := T("term.empty")
			if rt.termOut != "" {
				body = rt.termOut
			}
			myui.Column(c).FillWidth().Padding(12).Radius(radControl).
				Background(myui.Hex(termBg)).Border(1, myui.Hex(colLine)).
				Children(func() {
					myui.Text(c, body).Font(fontMono).FontSize(11).
						TextColor(text2)
				})
			if rt.termErr != "" {
				notice(c, "alert", rt.termErr, myui.Hex(colDanger))
			}
		})
	}

	if connected && d.Socks {
		line := appear("socks", myui.Row(c).Gap(9).AlignItems(myui.Center).Padding(0, 2))
		line.Children(func() {
			iconEl(c, "network", 14, dim)
			myui.Text(c, "SOCKS5").FontSize(11.5).TextColor(dim)
			myui.Text(c, a.socksAddr()).Font(fontMono).FontSize(10.5).
				TextColor(text2)
		})
	}
}

// ---------------------------------------------------------------- state helpers

// deviceValues copies the pointer list into values for persistence.
func deviceValues(list []*core.Device) []core.Device {
	out := make([]core.Device, len(list))
	for i, d := range list {
		out[i] = *d
		// Copy the slices too: the returned values must not share storage
		// with fields the widgets write to.
		out[i].Exposes = append([]string(nil), d.Exposes...)
		out[i].Forwards = append([]string(nil), d.Forwards...)
		out[i].AllowedClients = append([]string(nil), d.AllowedClients...)
	}
	return out
}

// findDevice looks a device up by id in a snapshot.
func findDevice(list []*core.Device, id string) (*core.Device, bool) {
	for _, d := range list {
		if d.ID == id {
			return d, true
		}
	}
	return nil, false
}

// deviceByID returns the live device record, if it exists.
func (a *App) deviceByID(id string) *core.Device {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.deviceByIDLocked(id)
}

func (a *App) deviceByIDLocked(id string) *core.Device {
	for _, d := range a.devices {
		if d.ID == id {
			return d
		}
	}
	return nil
}

// runtimeLocked returns (creating if needed) the runtime for a device.
// The caller must hold a.mu.
func (a *App) runtimeLocked(id string) *runtime {
	if a.rt == nil {
		a.rt = map[string]*runtime{}
	}
	rt := a.rt[id]
	if rt == nil {
		rt = &runtime{}
		a.rt[id] = rt
	}
	return rt
}

// runtimeSnapshot copies a runtime for rendering, so the view never reads
// fields that a background goroutine may be writing.
func (a *App) runtimeSnapshot(id string) runtime {
	a.mu.Lock()
	defer a.mu.Unlock()
	if rt := a.rt[id]; rt != nil {
		return *rt
	}
	return runtime{}
}

// deviceDrafts returns the stable widget bindings for a device.
func (a *App) deviceDrafts(d *core.Device) *drafts {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.draftsLocked(d)
}

// draftsLocked returns the drafts for d, growing or shrinking the row lists
// to match the device. The caller must hold a.mu.
func (a *App) draftsLocked(d *core.Device) *drafts {
	dr := a.drafts[d.ID]
	if dr == nil {
		dr = &drafts{
			name:    new(string),
			addr:    new(string),
			allowed: new(string),
			termCmd: new(string),
			files:   new(string),
		}
		*dr.name = d.Name
		*dr.addr = d.Addr
		*dr.allowed = strings.Join(d.AllowedClients, " ")
		*dr.files = d.FileDir
		a.drafts[d.ID] = dr
	}
	dr.exposes = resizeDrafts(dr.exposes, d.Exposes)
	dr.forwards = resizeDrafts(dr.forwards, d.Forwards)
	return dr
}

// resizeDrafts makes room for values, keeping existing pointers stable.
func resizeDrafts(list []*string, values []string) []*string {
	for len(list) < len(values) {
		p := new(string)
		*p = values[len(list)]
		list = append(list, p)
	}
	return list[:len(values)]
}

// ensureClientKey returns (creating on first use) this peer's persistent
// node public key, so the host can allow-list it.
func (a *App) ensureClientKey(id string) string {
	a.mu.Lock()
	if rt := a.rt[id]; rt != nil && rt.clientKey != "" {
		key := rt.clientKey
		a.mu.Unlock()
		return key
	}
	a.mu.Unlock()

	k, _, err := core.LoadOrCreateKey("device-" + id)
	if err != nil {
		return ""
	}
	key := k.PublicKey()

	a.mu.Lock()
	a.runtimeLocked(id).clientKey = key
	a.mu.Unlock()
	return key
}

func (a *App) socksAddr() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return fmt.Sprintf("127.0.0.1:%d", a.settings.SocksPort)
}

func (a *App) copiedTime() time.Time {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.copied
}

func (a *App) markCopied() {
	a.mu.Lock()
	a.copied = time.Now()
	a.mu.Unlock()
	a.refresh()
	time.AfterFunc(2*time.Second, a.refresh)
}

// sendTermLine sends the command field as a line to the remote shell.
func (a *App) sendTermLine(id string) {
	d := a.deviceByID(id)
	if d == nil {
		return
	}
	dr := a.deviceDrafts(d)
	line := *dr.termCmd
	*dr.termCmd = ""
	if ts := a.termOf(id); ts != nil && line != "" {
		ts.send(line + "\r")
	}
	a.refresh()
}

// runRemoteCommand runs one command through the established tunnel client.
func (a *App) runRemoteCommand(id string) {
	d := a.deviceByID(id)
	if d == nil {
		return
	}
	dr := a.deviceDrafts(d)

	a.mu.Lock()
	cmd := strings.TrimSpace(*dr.termCmd)
	rt := a.runtimeLocked(id)
	client := rt.cli
	rt.termErr = ""
	if cmd != "" {
		rt.termOut = "…"
	}
	a.mu.Unlock()

	if cmd == "" {
		return
	}
	if client == nil {
		a.mu.Lock()
		rt := a.runtimeLocked(id)
		rt.termErr = "connect to the peer first"
		a.mu.Unlock()
		a.refresh()
		return
	}
	a.refresh()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		var out string
		var err error
		conn, derr := client.Dial(core.SSHPort, 20*time.Second)
		if derr != nil {
			err = derr
		} else {
			out, err = core.SSHExecConn(ctx, conn, cmd)
		}

		a.mu.Lock()
		if rt := a.rt[id]; rt != nil {
			rt.termOut = out
			if err != nil {
				rt.termErr = err.Error()
			}
		}
		a.mu.Unlock()
		a.refresh()
	}()
}

// splitKeys splits the allow-list text into individual public keys.
func splitKeys(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == ' ' || r == '\t'
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// ---------------------------------------------------------------- mutations

// renameDevice stores a new display name for a device. An empty name falls
// back to a placeholder so the rail never shows a blank row.
func (a *App) renameDevice(d *core.Device, dr *drafts, name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Untitled"
		if dr != nil {
			*dr.name = name
		}
	}
	a.mu.Lock()
	if cur := a.deviceByIDLocked(d.ID); cur != nil {
		cur.Name = name
	}
	a.mu.Unlock()

	a.persistSoon()
	a.refresh()
}

func (a *App) addDevice(role string) {
	a.mu.Lock()
	name := T("device.host")
	if role == core.RolePeer {
		name = T("device.peer")
	}
	n := 0
	for _, d := range a.devices {
		if d.Role == role {
			n++
		}
	}
	d := newDevice(role, fmt.Sprintf("%s %d", name, n+1))
	a.devices = append(a.devices, d)
	a.sel = d.ID
	a.mu.Unlock()

	a.persistNow()
	a.refresh()
}

func (a *App) removeDevice(id string) {
	a.stopHost(id)
	a.stopPeer(id)

	a.mu.Lock()
	out := a.devices[:0]
	for _, d := range a.devices {
		if d.ID != id {
			out = append(out, d)
		}
	}
	a.devices = out
	if rt := a.rt[id]; rt != nil {
		rt.gen++ // invalidate any in-flight attempt
		delete(a.rt, id)
	}
	delete(a.drafts, id)
	if a.sel == id && len(a.devices) > 0 {
		a.sel = a.devices[0].ID
	}
	a.mu.Unlock()

	a.persistNow()
	a.refresh()
}

// persistSoon schedules a debounced save.
func (a *App) persistSoon() {
	a.mu.Lock()
	if a.saveTimer != nil {
		a.saveTimer.Stop()
	}
	a.saveTimer = time.AfterFunc(saveDelay, a.persistOnMain)
	a.mu.Unlock()
}

// persistOnMain runs persistNow on the UI thread when there is one.
//
// Widgets write bound fields directly, so those fields are only safe to
// read from the UI thread. The timer fires on a background goroutine, so
// marshalling keeps the snapshot from racing a keystroke. Before the
// window exists (startup, tests) there is no loop to marshal to.
func (a *App) persistOnMain() {
	a.mu.Lock()
	ready := a.window != nil
	a.mu.Unlock()
	if !ready {
		a.persistNow()
		return
	}
	mygo.RunOnMain(a.persistNow)
}

// persistIfChanged schedules a save when the in-memory state differs from
// what was last written. Widgets mutate bound values directly, so the view
// is the place where those edits become visible to us.
func (a *App) persistIfChanged() {
	a.mu.Lock()
	devices := marshalDevices(a.devices)
	settings := marshalSettings(a.settings)
	changed := devices != a.savedDevices || settings != a.savedSettings
	a.mu.Unlock()
	if changed {
		a.persistSoon()
	}
}

// persistNow writes devices and settings to disk and records the snapshot.
func (a *App) persistNow() {
	a.saveMu.Lock()
	defer a.saveMu.Unlock()

	a.mu.Lock()
	list := deviceValues(a.devices)
	settings := a.settings
	a.savedDevices = marshalDevices(a.devices)
	a.savedSettings = marshalSettings(settings)
	a.mu.Unlock()

	_ = core.SaveDevices(list)
	_ = core.SaveSettings(settings)
}

func marshalDevices(list []*core.Device) string {
	data, err := json.Marshal(deviceValues(list))
	if err != nil {
		return ""
	}
	return string(data)
}

func marshalSettings(s core.Settings) string {
	data, err := json.Marshal(s)
	if err != nil {
		return ""
	}
	return string(data)
}

func (a *App) addExpose(id string) {
	a.mu.Lock()
	if d := a.deviceByIDLocked(id); d != nil {
		d.Exposes = append(d.Exposes, "")
		a.draftsLocked(d) // grow the draft list
	}
	a.mu.Unlock()
	a.persistSoon()
	a.refresh()
}

func (a *App) removeExpose(id string, j int) {
	a.mu.Lock()
	if d := a.deviceByIDLocked(id); d != nil && j < len(d.Exposes) {
		d.Exposes = append(d.Exposes[:j], d.Exposes[j+1:]...)
		if dr := a.drafts[id]; dr != nil && j < len(dr.exposes) {
			dr.exposes = append(dr.exposes[:j], dr.exposes[j+1:]...)
		}
	}
	a.mu.Unlock()
	a.persistNow()
	a.refresh()
}

func (a *App) addForward(id string) {
	a.mu.Lock()
	if d := a.deviceByIDLocked(id); d != nil {
		d.Forwards = append(d.Forwards, "")
		a.draftsLocked(d)
	}
	a.mu.Unlock()
	a.persistSoon()
	a.refresh()
}

func (a *App) removeForward(id string, j int) {
	a.mu.Lock()
	if d := a.deviceByIDLocked(id); d != nil && j < len(d.Forwards) {
		d.Forwards = append(d.Forwards[:j], d.Forwards[j+1:]...)
		if dr := a.drafts[id]; dr != nil && j < len(dr.forwards) {
			dr.forwards = append(dr.forwards[:j], dr.forwards[j+1:]...)
		}
	}
	a.mu.Unlock()
	a.persistNow()
	a.refresh()
}

// deviceStatus returns the rail label and colour for a device.
func (a *App) deviceStatus(d *core.Device) (string, myui.Color) {
	a.mu.Lock()
	rt := a.rt[d.ID]
	var errMsg string
	var busy, srvOn, cliOn bool
	if rt != nil {
		errMsg = rt.errMsg
		busy = rt.busy
		srvOn = rt.srv != nil
		cliOn = rt.cli != nil
	}
	a.mu.Unlock()

	idle := myui.Hex(colDim)
	if rt == nil {
		if d.Role == core.RoleHost {
			return T("state.idleHost"), idle
		}
		return T("state.idlePeer"), idle
	}
	if errMsg != "" {
		return T("state.error"), myui.Hex(colDanger)
	}
	if busy {
		return T("state.connecting"), myui.Hex(colGold)
	}
	if srvOn {
		return T("state.running"), myui.Hex(colGood)
	}
	if cliOn {
		return T("state.connected"), myui.Hex(colGood)
	}
	if d.Role == core.RoleHost {
		return T("state.idleHost"), idle
	}
	return T("state.idlePeer"), idle
}

// ---------------------------------------------------------------- connections

// startHost starts sharing the configured ports.
func (a *App) startHost(id string) {
	a.mu.Lock()
	d := a.deviceByIDLocked(id)
	if d == nil {
		a.mu.Unlock()
		return
	}
	dev := *d
	// Copy the slices: the goroutine below reads them while the widgets
	// may be editing the originals.
	dev.Exposes = append([]string(nil), d.Exposes...)
	dev.AllowedClients = append([]string(nil), d.AllowedClients...)
	rt := a.runtimeLocked(id)
	rt.errMsg = ""
	rt.busy = true
	rt.gen++
	gen := rt.gen
	st := a.settings
	a.mu.Unlock()
	a.refresh()

	targets := map[uint16]string{}
	for _, spec := range dev.Exposes {
		port, target, err := parseExpose(spec)
		if err != nil {
			a.finishHost(id, gen, nil, err)
			return
		}
		targets[port] = target
	}
	if len(targets) == 0 {
		a.finishHost(id, gen, nil, fmt.Errorf("at least one port is required"))
		return
	}

	if dev.Files {
		if strings.TrimSpace(dev.FileDir) == "" {
			a.finishHost(id, gen, nil, fmt.Errorf("choose a directory to share"))
			return
		}
		info, err := os.Stat(dev.FileDir)
		if err != nil || !info.IsDir() {
			a.finishHost(id, gen, nil, fmt.Errorf("shared directory is not usable: %s", dev.FileDir))
			return
		}
	}

	opts := core.ServerOptions{
		DERPMapURL:     st.DERPMapURL,
		RegionID:       st.DERPMapRegion,
		Targets:        targets,
		UDP:            dev.UDP,
		AllowProxy:     dev.AllowProxy,
		SSH:            dev.SSH,
		AllowedClients: dev.AllowedClients,
	}
	if dev.Files {
		opts.SSH = true
		opts.SSHFileDir = dev.FileDir
	}
	if dev.Persistent {
		opts.KeyName = "device-" + id
	}

	go func() {
		srv, err := core.StartServer(opts)
		a.finishHost(id, gen, srv, err)
	}()
}

func (a *App) finishHost(id string, gen uint64, srv *core.Server, err error) {
	var closeSrv *core.Server
	var addr string

	a.mu.Lock()
	rt := a.rt[id]
	if rt == nil || rt.gen != gen {
		// The attempt was stopped or the device removed while starting.
		closeSrv = srv
	} else {
		rt.busy = false
		if err != nil {
			rt.errMsg = err.Error()
		} else {
			rt.srv = srv
			rt.addr = srv.Address()
			addr = rt.addr
		}
	}
	a.mu.Unlock()

	if closeSrv != nil {
		_ = closeSrv.Close()
	}
	if addr != "" {
		fmt.Fprintf(os.Stderr, "clowder: %s is sharing at %s\n", id, addr)
	}
	a.refresh()
}

func (a *App) stopHost(id string) {
	a.mu.Lock()
	rt := a.rt[id]
	var srv *core.Server
	if rt != nil {
		rt.gen++
		srv = rt.srv
		rt.srv = nil
		rt.addr = ""
		rt.errMsg = ""
		rt.busy = false
	}
	a.mu.Unlock()

	if srv != nil {
		_ = srv.Close()
	}
	a.refresh()
}

// startPeer connects to the peer and installs the port mappings.
func (a *App) startPeer(id string) {
	a.mu.Lock()
	d := a.deviceByIDLocked(id)
	if d == nil {
		a.mu.Unlock()
		return
	}
	dev := *d
	dev.Forwards = append([]string(nil), d.Forwards...)
	rt := a.runtimeLocked(id)
	rt.errMsg = ""
	rt.busy = true
	rt.gen++
	gen := rt.gen
	st := a.settings
	a.mu.Unlock()
	a.refresh()

	addr := strings.TrimSpace(dev.Addr)
	if addr == "" {
		a.finishPeer(id, gen, nil, dev, 0, fmt.Errorf("paste the peer's address first"))
		return
	}

	parsed := make([][2]uint16, 0, len(dev.Forwards))
	for _, spec := range dev.Forwards {
		local, remote, err := parseForward(spec)
		if err != nil {
			a.finishPeer(id, gen, nil, dev, 0, err)
			return
		}
		parsed = append(parsed, [2]uint16{local, remote})
	}
	if len(parsed) == 0 {
		a.finishPeer(id, gen, nil, dev, 0, fmt.Errorf("at least one port mapping is required"))
		return
	}

	socksAddr := fmt.Sprintf("127.0.0.1:%d", st.SocksPort)

	go func() {
		c, err := core.NewClientWithOptions(core.ClientOptions{
			Addr:       addr,
			DERPMapURL: st.DERPMapURL,
			// A stable key per peer makes the host's allow-list usable.
			KeyName: "device-" + id,
		})
		if err != nil {
			a.finishPeer(id, gen, nil, dev, 0, err)
			return
		}

		a.mu.Lock()
		if rt := a.rt[id]; rt != nil && rt.gen == gen {
			rt.clientKey = c.PublicKey()
		}
		a.mu.Unlock()
		a.refresh()

		lat, err := c.Ping(30 * time.Second)
		if err != nil {
			_ = c.Close()
			a.finishPeer(id, gen, nil, dev, 0,
				fmt.Errorf("cannot reach the peer: %w", err))
			return
		}

		for _, m := range parsed {
			if _, err := c.Forward(fmt.Sprintf("127.0.0.1:%d", m[0]), m[1]); err != nil {
				_ = c.Close()
				a.finishPeer(id, gen, nil, dev, lat,
					fmt.Errorf("local port %d is busy: %w", m[0], err))
				return
			}
			if dev.UDP {
				if _, err := c.ForwardUDP(fmt.Sprintf("127.0.0.1:%d", m[0]), m[1]); err != nil {
					_ = c.Close()
					a.finishPeer(id, gen, nil, dev, lat,
						fmt.Errorf("local UDP port %d is busy: %w", m[0], err))
					return
				}
			}
		}

		if dev.Term && core.TerminalAvailable() {
			a.ensureTerm(id, gen, c)
		}

		if dev.Socks {
			if _, err := c.SOCKS5(socksAddr); err != nil {
				_ = c.Close()
				a.finishPeer(id, gen, nil, dev, lat,
					fmt.Errorf("start the SOCKS5 proxy: %w", err))
				return
			}
		}

		a.finishPeer(id, gen, c, dev, lat, nil)
	}()
}

func (a *App) finishPeer(id string, gen uint64, c *core.Client, dev core.Device,
	lat time.Duration, err error) {

	var closeClient *core.Client
	stale := false

	a.mu.Lock()
	rt := a.rt[id]
	if rt == nil || rt.gen != gen {
		// Stopped, removed, or superseded while connecting.
		closeClient = c
		stale = true
	} else {
		rt.busy = false
		if err != nil {
			rt.errMsg = err.Error()
		} else {
			rt.cli = c
			rt.lat = lat
		}
	}
	a.mu.Unlock()

	if closeClient != nil {
		_ = closeClient.Close()
	}
	if !stale && err != nil {
		a.setTerm(id, nil)
	}
	if !stale && err == nil {
		fmt.Fprintf(os.Stderr, "clowder: %s connected in %v\n", id, lat)
		if dev.BrowseFiles {
			a.openBrowser(id)
		}
	}
	a.refresh()
}

func (a *App) stopPeer(id string) {
	a.setTerm(id, nil)

	a.mu.Lock()
	rt := a.rt[id]
	var c *core.Client
	var files *core.Files
	if rt != nil {
		rt.gen++
		c = rt.cli
		files = rt.files
		rt.cli = nil
		rt.files = nil
		rt.browse = browseState{}
		rt.errMsg = ""
		rt.lat = 0
		rt.busy = false
	}
	a.mu.Unlock()

	if files != nil {
		_ = files.Close()
	}
	if c != nil {
		_ = c.Close()
	}
	a.refresh()
}

// qrImage renders (and caches) the QR code for an address.
func (a *App) qrImage(id, addr string) myui.ImageSource {
	a.mu.Lock()
	if rt := a.rt[id]; rt != nil && rt.qrAddr == addr && rt.qrSrc != nil {
		src := rt.qrSrc
		a.mu.Unlock()
		return src
	}
	a.mu.Unlock()

	png, err := qrcode.Encode(addr, qrcode.Medium, 640)
	if err != nil {
		return nil
	}
	src, err := myui.DecodeImage(png)
	if err != nil {
		return nil
	}

	a.mu.Lock()
	if rt := a.rt[id]; rt != nil {
		rt.qrAddr = addr
		rt.qrSrc = src
	}
	a.mu.Unlock()
	return src
}

// debugAutoStart starts a device for headless screenshots.
func (a *App) debugAutoStart(what string) {
	a.mu.Lock()
	var host, peer string
	for _, d := range a.devices {
		if d.Role == core.RoleHost && host == "" {
			host = d.ID
		}
		if d.Role == core.RolePeer && peer == "" {
			peer = d.ID
		}
	}
	a.mu.Unlock()

	switch what {
	case "server", "host":
		if host != "" {
			a.startHost(host)
		}
	case "client", "peer":
		if peer != "" {
			a.startPeer(peer)
		}
	}
}

// ---------------------------------------------------------------- parsing

func parseExpose(spec string) (uint16, string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return 0, "", fmt.Errorf("empty port")
	}
	if i := strings.IndexByte(spec, '='); i >= 0 {
		port, err := parsePort(spec[:i])
		if err != nil {
			return 0, "", err
		}
		target := strings.TrimSpace(spec[i+1:])
		if target == "" {
			return 0, "", fmt.Errorf("missing local address after '='")
		}
		return port, target, nil
	}
	port, err := parsePort(spec)
	if err != nil {
		return 0, "", err
	}
	return port, fmt.Sprintf("127.0.0.1:%d", port), nil
}

func parseForward(spec string) (uint16, uint16, error) {
	spec = strings.TrimSpace(spec)
	i := strings.IndexByte(spec, ':')
	if i < 0 {
		return 0, 0, fmt.Errorf("mapping must be written local:remote")
	}
	local, err := parsePort(spec[:i])
	if err != nil {
		return 0, 0, err
	}
	remote, err := parsePort(spec[i+1:])
	if err != nil {
		return 0, 0, err
	}
	return local, remote, nil
}

func parsePort(s string) (uint16, error) {
	n, err := strconv.ParseUint(strings.TrimSpace(s), 10, 16)
	if err != nil || n == 0 {
		return 0, fmt.Errorf("invalid port: %q", s)
	}
	return uint16(n), nil
}

// hardWrap inserts a newline every n characters.
func hardWrap(s string, n int) string {
	if n <= 0 {
		return s
	}
	r := []rune(s)
	var b strings.Builder
	for i := 0; i < len(r); i += n {
		if i > 0 {
			b.WriteByte('\n')
		}
		end := i + n
		if end > len(r) {
			end = len(r)
		}
		b.WriteString(string(r[i:end]))
	}
	return b.String()
}

// settingsPane draws application-wide settings.
func (a *App) settingsPane(c *myui.Context) {
	cream := myui.Hex(colCream)

	a.mu.Lock()
	st := a.settings
	a.mu.Unlock()

	// Header.
	myui.Row(c).Gap(16).AlignItems(myui.Center).Children(func() {
		headerTile(c, "settings", myui.Hex(colGold))
		myui.Column(c).Gap(4).Grow(1).Children(func() {
			myui.Text(c, T("settings.title")).Font(fontUI).FontSize(22).
				FontWeight(700).LetterSpacing(-0.4).TextColor(cream)
		})
		if actionButton(c, btnQuiet, "chevron-left", T("nav.back")).Clicked() {
			a.mu.Lock()
			a.showSettings = false
			a.mu.Unlock()
			a.refresh()
		}
	})

	// General.
	general := panel(c, 0).Padding(6, 0).Gap(1).
		Dividers(1, myui.Hex(colLine))
	general.Children(func() {
		myui.Row(c).FillWidth().AlignItems(myui.Center).Padding(10, 12).Gap(12).Children(func() {
			iconText(c, "globe", T("settings.language"), 16, 12.5, cream).Grow(1)
			group := myui.Row(c).Gap(2).Padding(2).Radius(8).
				Background(myui.Hex(colRail)).Border(1, myui.Hex(colLine)).
				Shrink(0)
			group.Children(func() {
				langs := []struct{ label, tag string }{
					{T("settings.langAuto"), ""},
					{"English", "en"},
					{"简体中文", "zh-Hans"},
				}
				for _, l := range langs {
					l := l
					if segmentButton(c, l.label, st.Language == l.tag).Clicked() {
						a.applyLanguage(l.tag)
					}
				}
			})
		})

		switchRow(c, "tray", T("settings.closeToTray"), a.closeToTrayPtr())

		myui.Row(c).FillWidth().AlignItems(myui.Center).Padding(10, 12).Gap(10).Children(func() {
			iconText(c, "network", T("settings.socksPort"), 16, 12.5, cream).Grow(1)
			if myui.TextInput(c, &a.socksPortIn).Width(110).Changed() {
				a.saveNumericSettings()
			}
		})
	})

	// Relay.
	relay := panel(c, 0).Padding(6, 0).Gap(1).
		Dividers(1, myui.Hex(colLine))
	relay.Children(func() {
		myui.Row(c).FillWidth().AlignItems(myui.Center).Padding(10, 12).Gap(10).Children(func() {
			iconText(c, "globe", T("settings.derp"), 16, 12.5, cream).Grow(1)
			if myui.TextInput(c, &a.derpIn).Width(330).Changed() {
				a.saveNumericSettings()
			}
		})
		myui.Row(c).FillWidth().AlignItems(myui.Center).Padding(10, 12).Gap(10).Children(func() {
			iconText(c, "radio", T("settings.derpRegion"), 16, 12.5, cream).Grow(1)
			if myui.TextInput(c, &a.derpRegionIn).Width(110).Changed() {
				a.saveNumericSettings()
			}
		})
	})
}

// applyLanguage switches the interface language and remembers it.
func (a *App) applyLanguage(tag string) {
	a.mu.Lock()
	a.settings.Language = tag
	a.mu.Unlock()

	a.persistNow()
	mygo.RunOnMain(func() { mygo.App.SetLocale(tag) })
	a.refresh()
}

// closeToTrayPtr returns the binding for the close-to-tray switch.
func (a *App) closeToTrayPtr() *bool {
	return &a.settings.CloseToTray
}

// saveNumericSettings parses the settings fields and writes them back.
func (a *App) saveNumericSettings() {
	a.mu.Lock()
	if n, err := strconv.Atoi(strings.TrimSpace(a.socksPortIn)); err == nil && n > 0 && n < 65536 {
		a.settings.SocksPort = n
	}
	if n, err := strconv.Atoi(strings.TrimSpace(a.derpRegionIn)); err == nil && n >= 0 {
		a.settings.DERPMapRegion = n
	}
	a.settings.DERPMapURL = strings.TrimSpace(a.derpIn)
	a.mu.Unlock()

	a.persistNow()
	a.refresh()
}

// openBrowser opens an SFTP session to the host and lists the shared root.
func (a *App) openBrowser(id string) {
	a.mu.Lock()
	rt := a.rt[id]
	if rt == nil || rt.cli == nil {
		a.mu.Unlock()
		return
	}
	if rt.files != nil {
		a.mu.Unlock()
		a.navigateFiles(id, "/")
		return
	}
	cli := rt.cli
	gen := rt.gen
	rt.browse = browseState{path: "/", loading: true}
	a.mu.Unlock()
	a.refresh()

	go func() {
		conn, err := cli.Dial(core.SSHPort, 20*time.Second)
		if err != nil {
			a.browseFailed(id, gen, err)
			return
		}
		files, err := core.OpenFiles(conn)
		if err != nil {
			a.browseFailed(id, gen, err)
			return
		}

		a.mu.Lock()
		rt := a.rt[id]
		if rt == nil || rt.gen != gen {
			a.mu.Unlock()
			_ = files.Close()
			return
		}
		rt.files = files
		a.mu.Unlock()

		a.navigateFiles(id, "/")
	}()
}

// browseFailed records a file browser error on the runtime.
func (a *App) browseFailed(id string, gen uint64, err error) {
	msg := err.Error()
	if strings.Contains(msg, "subsystem") {
		msg = T("files.notShared") + " (" + msg + ")"
	}

	a.mu.Lock()
	if rt := a.rt[id]; rt != nil && rt.gen == gen {
		rt.browse.loading = false
		rt.browse.errMsg = msg
	}
	a.mu.Unlock()
	a.refresh()
}

// navigateFiles lists dir in the open SFTP session.
func (a *App) navigateFiles(id, dir string) {
	dir = path.Clean(dir)
	if !strings.HasPrefix(dir, "/") {
		dir = "/" + dir
	}

	a.mu.Lock()
	rt := a.rt[id]
	if rt == nil || rt.files == nil {
		a.mu.Unlock()
		return
	}
	files := rt.files
	gen := rt.gen
	rt.browse.seq++
	seq := rt.browse.seq
	rt.browse.path = dir
	rt.browse.loading = true
	rt.browse.errMsg = ""
	rt.browse.note = ""
	a.mu.Unlock()
	a.refresh()

	go func() {
		entries, err := files.List(dir)

		a.mu.Lock()
		if rt := a.rt[id]; rt != nil && rt.gen == gen && rt.browse.seq == seq {
			rt.browse.loading = false
			if err != nil {
				rt.browse.errMsg = err.Error()
			} else {
				if total := len(entries); total > maxFileEntries {
					entries = entries[:maxFileEntries]
					rt.browse.note = T("files.truncated", "count", total)
				}
				rt.browse.entries = entries
			}
		}
		a.mu.Unlock()
		a.refresh()
	}()
}

// downloadFile copies one shared file into ~/Downloads/Clowder/<device>/.
func (a *App) downloadFile(id, dir, name string) {
	a.mu.Lock()
	rt := a.rt[id]
	if rt == nil || rt.files == nil {
		a.mu.Unlock()
		return
	}
	files := rt.files
	gen := rt.gen
	deviceName := "peer"
	if d := a.deviceByIDLocked(id); d != nil {
		deviceName = d.Name
	}
	rt.browse.note = T("files.loading")
	a.mu.Unlock()
	a.refresh()

	remote := path.Join(dir, name)
	base := filepath.Base(name)
	if base == "." || base == ".." || base == "" || base == string(filepath.Separator) {
		base = "download"
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	safe := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == 0 {
			return '-'
		}
		return r
	}, deviceName)
	local := filepath.Join(home, "Downloads", "Clowder", safe, base)

	go func() {
		err := files.Download(remote, local)

		a.mu.Lock()
		if rt := a.rt[id]; rt != nil && rt.gen == gen {
			if err != nil {
				rt.browse.note = err.Error()
			} else {
				rt.browse.note = T("files.saved", "path", local)
			}
		}
		a.mu.Unlock()
		a.refresh()
	}()
}

// filesPanel draws the peer's browser for the host's shared directory.
func (a *App) filesPanel(c *myui.Context, id string) {
	cream := myui.Hex(colCream)
	text2 := myui.Hex(colText2)
	dim := myui.Hex(colDim)
	gold := myui.Hex(colGold)

	rt := a.runtimeSnapshot(id)
	b := rt.browse

	card := appear("files", panel(c, 0).Padding(16).Gap(10))
	card.Children(func() {
		myui.Row(c).AlignItems(myui.Center).Gap(9).Children(func() {
			iconEl(c, "folder", 15, dim)
			sectionLabel(c, T("files.title"))
			myui.Row(c).Grow(1)
			myui.Text(c, b.path).Font(fontMono).FontSize(10.5).
				TextColor(text2).SingleLine()
			if b.path != "/" {
				if iconButton(c, "chevron-up", T("files.up"), dim).Clicked() {
					a.navigateFiles(id, path.Dir(b.path))
				}
			}
			if iconButton(c, "refresh", T("files.refresh"), dim).Clicked() {
				if rt.files != nil {
					a.navigateFiles(id, b.path)
				} else {
					a.openBrowser(id)
				}
			}
		})

		switch {
		case b.loading:
			myui.Text(c, T("files.loading")).FontSize(11.5).TextColor(dim)
		case b.errMsg != "":
			notice(c, "alert", b.errMsg, myui.Hex(colDanger))
			myui.Row(c).Children(func() {
				if actionButton(c, btnQuiet, "refresh", T("files.retry")).Clicked() {
					a.openBrowser(id)
				}
			})
		case len(b.entries) == 0:
			myui.Text(c, T("files.empty")).FontSize(11.5).TextColor(dim)
		default:
			for _, e := range b.entries {
				e := e
				row := myui.Row(c).FillWidth().Gap(9).AlignItems(myui.Center).
					Padding(6, 8).Radius(radRow)
				if row.Hovered() {
					row.Background(myui.Hex(colHover))
				}
				row.Children(func() {
					name := "file"
					if e.Dir {
						name = "folder"
					}
					iconEl(c, name, 15, dim)
					myui.Text(c, e.Name).FontSize(12).TextColor(cream).
						Grow(1).SingleLine().TruncateStart()
					if e.Dir {
						iconEl(c, "chevron-right", 14, dim)
					} else {
						myui.Text(c, humanSize(e.Size)).Font(fontMono).
							FontSize(10.5).FontFeatures("tnum").TextColor(dim)
						if iconButton(c, "download", T("files.download"), dim).Clicked() {
							a.downloadFile(id, b.path, e.Name)
						}
					}
				})
				if e.Dir {
					row.Cursor(myui.CursorPointer)
					row.OnClick(func() {
						a.navigateFiles(id, path.Join(b.path, e.Name))
					})
				}
			}
		}

		if b.note != "" {
			myui.Text(c, b.note).FontSize(11).TextColor(gold)
		}
	})
}

// humanSize formats a byte count for the file list.
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
