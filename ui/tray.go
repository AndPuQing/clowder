package ui

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"

	"github.com/egoist/mygo"

	"github.com/AndPuQing/clowder/core"
)

// trayIcon 生成托盘图标：猫头剪影（圆脸 + 两只耳朵）。
//
// macOS 的 template 图标只看 alpha 通道，所以同一张图在三个平台都能用。
func trayIcon() []byte {
	const size = 32
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	fill := color.RGBA{R: 0xC9, G: 0xA2, B: 0x27, A: 0xFF}

	// 圆点（抗锯齿靠超采样）
	circle := func(cx, cy, r float64) {
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				const sub = 4
				hit := 0
				for sy := 0; sy < sub; sy++ {
					for sx := 0; sx < sub; sx++ {
						px := float64(x) + (float64(sx)+0.5)/sub
						py := float64(y) + (float64(sy)+0.5)/sub
						dx, dy := px-cx, py-cy
						if dx*dx+dy*dy <= r*r {
							hit++
						}
					}
				}
				if hit == 0 {
					continue
				}
				alpha := uint8(255 * hit / (sub * sub))
				old := img.RGBAAt(x, y)
				if alpha > old.A {
					img.SetRGBA(x, y, color.RGBA{fill.R, fill.G, fill.B, alpha})
				}
			}
		}
	}

	// 耳朵
	circle(10, 9, 4.6)
	circle(22, 9, 4.6)
	// 脸
	circle(16, 19, 10)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil
	}
	return buf.Bytes()
}

// setupTray 创建托盘图标；没有托盘的环境（如无头容器）直接跳过。
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
		fmt.Fprintf(os.Stderr, "clowder: 托盘不可用：%v\n", err)
		return
	}

	a.mu.Lock()
	a.tray = tray
	a.mu.Unlock()

	tray.OnClick(func() { a.showWindow() })
	a.rebuildTrayMenu()
}

// showWindow 把窗口叫回前台。
func (a *App) showWindow() {
	mygo.RunOnMain(func() {
		if a.window == nil {
			return
		}
		a.window.Show()
		a.window.Focus()
	})
}

// rebuildTrayMenu 按当前设备与运行状态重建托盘菜单。
func (a *App) rebuildTrayMenu() {
	a.mu.Lock()
	tray := a.tray
	devices := append([]core.Device(nil), a.devices...)
	running := 0
	for _, d := range devices {
		rt := a.rt[d.ID]
		if rt != nil && (rt.srv != nil || rt.cli != nil) {
			running++
		}
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
		status, _ := a.deviceStatus(d)
		rt := a.runtimeOf(d.ID)
		busy := rt != nil && rt.busy
		srvOn := rt != nil && rt.srv != nil
		cliOn := rt != nil && rt.cli != nil

		sub := []*mygo.MenuItem{}
		if d.Role == core.RoleHost {
			if srvOn {
				sub = append(sub, &mygo.MenuItem{Label: T("action.stop"), Click: func(*mygo.MenuItem, *mygo.Window) { a.stopHost(d.ID) }})
			} else if !busy {
				sub = append(sub, &mygo.MenuItem{Label: T("action.ticket"), Click: func(*mygo.MenuItem, *mygo.Window) { a.startHost(d.ID) }})
			}
		} else {
			if cliOn {
				sub = append(sub, &mygo.MenuItem{Label: T("action.disconnect"), Click: func(*mygo.MenuItem, *mygo.Window) { a.stopPeer(d.ID) }})
			} else if !busy {
				sub = append(sub, &mygo.MenuItem{Label: T("action.connect"), Click: func(*mygo.MenuItem, *mygo.Window) { a.startPeer(d.ID) }})
			}
		}
		if rt != nil && rt.errMsg != "" {
			sub = append(sub, &mygo.MenuItem{Label: "ⓘ " + truncate(rt.errMsg, 40), Disabled: true})
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
		_ = menu
		tray.SetMenu(menu)
		tray.SetToolTip(tip)
	})
}

// quit 真正退出（托盘菜单里调用）。
func (a *App) quit() {
	a.mu.Lock()
	a.quitting = true
	a.mu.Unlock()
	mygo.RunOnMain(func() { mygo.App.Quit() })
}

// truncate 截断过长文案，菜单里不展开长错误。
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
