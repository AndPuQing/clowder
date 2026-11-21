// Package ui 是 Clowder 的桌面界面。
//
// 结构：左侧设备列表，右侧所选设备的详情。
// 视觉：票根——深墨绿底、米色字、金色点缀；地址是一张可以交给对方的票。
package ui

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	myui "github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/transfer"
	"github.com/skip2/go-qrcode"

	"github.com/AndPuQing/clowder/core"
)

const (
	fontUI   = "Noto Sans CJK SC"
	fontMono = "Noto Sans Mono CJK SC"

	colBg     = "#12221E"
	colRail   = "#0E1B18"
	colPanel  = "#182C27"
	colLine   = "#2A443E"
	colCream  = "#F2EDE3"
	colDim    = "#93A9A2"
	colGold   = "#C9A227"
	colDanger = "#E0765E"

	// Apple 式圆角：控件 11、卡片 14、面板 20
	radControl = 11
	radCard    = 14
	radRow     = 10
	radPanel   = 20
)

// runtime 是一台设备的运行时状态，不落盘。
// demoAddr 是演示/截图用的样例地址（不可连接）。
const demoAddr = "kHBrZxJsdB_CPdGdTQxUzc3JDgTJW_mmxyxbhOtfnPYqAeJZzzlYcBUHIYwlLnEdxTlLWpOWFKufsMabNrWXsBcOYnINedlWyHFCwCrAhDLXhwdbHWSYFVOMRBrkaKGtfsQfTLZLhWLKjPqPwdDlj"

type runtime struct {
	srv    *core.Server
	cli    *core.Client
	addr   string
	errMsg string
	note   string
	busy   bool
	lat    time.Duration
	qrAddr string
	qrSrc  myui.ImageSource
}

// App 是界面状态。
type App struct {
	mu      sync.Mutex
	window  *mygo.Window
	devices []core.Device
	sel     string
	rt      map[string]*runtime
	copied  time.Time
	theme   *myui.Theme

	tray     *mygo.Tray
	quitting bool
}

// New 读取设备清单并构造界面状态。
func New() *App {
	a := &App{
		rt:    map[string]*runtime{},
		theme: func() *myui.Theme {
			t := myui.DarkTheme().WithAccent(myui.Hex(colGold))
			t.Radius = radControl // 让按钮、输入框、开关统一成同一种圆角
			return t
		}(),
	}
	if list, err := core.LoadDevices(); err == nil {
		a.devices = list
	}
	if len(a.devices) == 0 {
		list := []core.Device{newDevice(core.RoleHost, "本机")}
		a.devices = list
		_ = core.SaveDevices(list)
	}
	if len(a.devices) > 0 {
		a.sel = a.devices[0].ID
	}
	if os.Getenv("CLOWDER_DEMO") == "1" {
		peer := newDevice(core.RolePeer, "对方 1")
		peer.Addr = demoAddr
		a.devices = append(a.devices, peer)
	}
	return a
}

func newDevice(role, name string) core.Device {
	d := core.Device{
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

// Run 启动主循环。
func (a *App) Run() error {
	if os.Getenv("CLOWDER_TAB") == "client" {
		// 兼容旧的调试开关：选中第一台 peer 设备
		for _, d := range a.devices {
			if d.Role == core.RolePeer {
				a.sel = d.ID
				break
			}
		}
	}
	mygo.App.WhenReady(func() {
		a.window = mygo.NewWindow(mygo.WindowOptions{
			Title:         "Clowder",
			Width:         1000,
			Height:        680,
			MinWidth:      880,
			MinHeight:     600,
			TitleBarStyle: mygo.TitleBarHidden,
			Content: myui.View(a.view),
		})
		a.setupTray()
		a.window.OnClose(func(e *mygo.CloseEvent) {
			a.mu.Lock()
			quitting := a.quitting
			a.mu.Unlock()
			if quitting || a.tray == nil {
				return // 没有托盘就正常退出
			}
			e.PreventDefault()
			a.window.Hide()
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

func (a *App) refresh() {
	w := a.window
	if w == nil {
		return
	}
	mygo.RunOnMain(func() { w.Invalidate() })
	go a.rebuildTrayMenu()
}

// ---------------------------------------------------------------- 视图

func (a *App) view(c *myui.Context) {
	c.SetTheme(a.theme)

	bg := myui.Hex(colBg)
	cream := myui.Hex(colCream)

	a.mu.Lock()
	devices := append([]core.Device(nil), a.devices...)
	sel := a.sel
	a.mu.Unlock()

	myui.Row(c).Fill().Background(bg).Children(func() {
		a.rail(c, devices, sel)

		myui.Row(c).Width(1).FillHeight().Background(myui.Hex(colLine))

		myui.Column(c).Grow(1).Fill().Padding(34, 30).Gap(0).Children(func() {
			myui.Row(c).Height(18).FillWidth().DragWindow()
			dev, ok := findDevice(devices, sel)
			if !ok {
				myui.Row(c).Grow(1)
				myui.Text(c, "左边选一台设备").FontSize(14).TextColor(myui.Hex(colDim))
				myui.Row(c).Grow(1)
				return
			}
			if dev.Role == core.RoleHost {
				a.hostDetail(c, dev)
			} else {
				a.peerDetail(c, dev)
			}
			myui.Text(c, "关掉窗口不影响正在进行的连接").
				FontSize(11.5).TextColor(myui.Hex(colDim)).Padding(0, 18, 0, 0)
			_ = cream
		})
	})
}

// rail 左侧设备列表
func (a *App) rail(c *myui.Context, devices []core.Device, sel string) {
	cream := myui.Hex(colCream)
	dim := myui.Hex(colDim)

	myui.Column(c).Width(232).Fill().Background(myui.Hex(colRail)).
		Padding(26, 26).Gap(0).Children(func() {
		myui.Row(c).Height(18).FillWidth().DragWindow()
		myui.Text(c, "Clowder").Font(fontUI).FontSize(14).FontWeight(500).
			TextColor(cream)

		myui.Column(c).Gap(2).Padding(0, 22, 0, 0).Children(func() {
			for _, d := range devices {
				d := d
				selected := d.ID == sel
				status, statusColor := a.deviceStatus(d)
				row := myui.Column(c).Gap(3).Padding(9, 11).Radius(radRow).
					Children(func() {
						name := myui.Text(c, d.Name).FontSize(13.5)
						label := myui.Text(c, status).FontSize(11).TextColor(statusColor)
						if selected {
							name.FontWeight(600).TextColor(cream)
						} else {
							name.TextColor(myui.Hex("#C6D2CE"))
							label.TextColor(dim)
						}
					})
				row.Cursor(myui.CursorPointer)
				if row.Hovered() && !selected {
					row.Background(myui.Hex("#15231F"))
				}
				if selected {
					row.Background(myui.Hex("#1B2E29"))
				}
				row.OnClick(func() {
					a.mu.Lock()
					a.sel = d.ID
					a.mu.Unlock()
					a.refresh()
				})
			}
		})

		myui.Row(c).Grow(1)

		// 新建：两行，左边一条金线作为动作标记
		addRow := func(label string, role string) {
			row := myui.Row(c).Gap(8).Padding(6, 0).Children(func() {
				myui.Row(c).Width(2).Height(12).Background(myui.Hex(colGold))
				myui.Text(c, label).FontSize(12.5).TextColor(myui.Hex("#C6D2CE"))
			})
			row.Cursor(myui.CursorPointer)
			if row.Hovered() {
				myui.Text(c, "").FontSize(0)
			}
			row.OnClick(func() { a.addDevice(role) })
		}
		addRow("开票（暴露本机端口）", core.RoleHost)
		addRow("加入（连接对方）", core.RolePeer)
	})
}

// hostDetail 服务端详情
func (a *App) hostDetail(c *myui.Context, d core.Device) {
	cream := myui.Hex(colCream)
	dim := myui.Hex(colDim)
	gold := myui.Hex(colGold)

	rt := a.runtimeOf(d.ID)
	if rt == nil {
		rt = &runtime{}
	}
	running := rt.srv != nil
	addr := rt.addr
	if addr == "" && running {
		addr = rt.srv.Address()
	}

	// 标题行 + 动作
	myui.Row(c).Gap(12).Children(func() {
		myui.Text(c, d.Name).Font(fontUI).FontSize(24).FontWeight(700).
			TextColor(cream)
		myui.Row(c).Grow(1)
		if running {
			myui.Toolbar(c, func() {
				if myui.Button(c, "停止").Clicked() {
					a.stopHost(d.ID)
				}
			})
		} else if !rt.busy {
			if myui.PrimaryButton(c, "开票").Width(96).Clicked() {
				a.startHost(d.ID)
			}
		}
		myui.Toolbar(c, func() {
			if myui.Button(c, "删除").Clicked() {
				a.removeDevice(d.ID)
			}
		})
	})

	myui.Column(c).Padding(0, 14, 0, 14).Children(func() {
		myui.Row(c).Height(1).FillWidth().Background(myui.Hex(colLine))
	})

	if running && addr != "" {
		myui.Row(c).Padding(0, 22, 0, 0).Children(func() {
			myui.Row(c).Padding(24).Background(myui.Hex(colPanel)).Radius(radPanel).
				Gap(22).Width(600).Children(func() {
				myui.Column(c).Padding(10).Background(myui.Hex("#FFFFFF")).
					Radius(radCard).Children(func() {
						if src := a.qrImage(d.ID, addr); src != nil {
							myui.Image(c, src).Size(150, 150)
						}
					})

				myui.Row(c).Width(1).Height(150).Background(myui.Hex(colLine))

				myui.Column(c).Gap(9).Grow(1).Children(func() {
					myui.Text(c, "单向通行").FontSize(23).FontWeight(700).
						TextColor(cream)
					myui.Text(c, "把这张票交给对方").FontSize(12).TextColor(dim)
					myui.Column(c).FillWidth().Padding(0, 0, 12, 0).
						Children(func() {
							myui.Text(c, addr).Font(fontMono).FontSize(10.5).
								FillWidth().TextColor(myui.Hex("#C6D2CE"))
						})

					label := "复制地址"
					if time.Since(a.copiedTime()) < 2*time.Second {
						label = "已复制"
					}
					if myui.PrimaryButton(c, label).Width(112).Clicked() {
						_ = mygo.Clipboard.Write(transfer.TextData(addr))
						a.markCopied()
					}
					myui.Text(c, "停止后失效").FontSize(11).TextColor(gold)
				})
			})
		})
	} else {
		myui.Column(c).Gap(6).Padding(0, 20, 0, 0).Children(func() {
			myui.Text(c, "还没开票").FontSize(20).FontWeight(600).TextColor(cream)
			myui.Text(c, "填好端口，点右上角「开票」生成地址。").
				FontSize(12.5).TextColor(dim)
		})
	}

	// 端口
	myui.Column(c).Gap(8).FillWidth().Padding(0, 22, 0, 0).Children(func() {
		myui.Text(c, "暴露的端口").FontSize(11).FontWeight(600).TextColor(dim)
		for i := range a.devices {
			if a.devices[i].ID != d.ID {
				continue
			}
			for j := range a.devices[i].Exposes {
				i, j := i, j
				if running {
					myui.Text(c, a.devices[i].Exposes[j]).Font(fontMono).
						FontSize(12.5).TextColor(myui.Hex("#C6D2CE"))
					continue
				}
				myui.Row(c).Gap(8).Children(func() {
					myui.TextInput(c, &a.devices[i].Exposes[j]).Width(260)
					if len(a.devices[i].Exposes) > 1 {
						myui.Toolbar(c, func() {
							if myui.Button(c, "移除").Clicked() {
								a.removeExpose(i, j)
							}
						})
					}
				})
			}
		}
		if !running {
			myui.Toolbar(c, func() {
				if myui.Button(c, "添加端口").Clicked() {
					a.addExpose(d.ID)
				}
			})
			myui.Row(c).Gap(10).Children(func() {
				myui.Switch(c, a.persistentOf(d.ID))
				myui.Text(c, "固定地址").FontSize(12).TextColor(cream)
				myui.Text(c, "关闭则每次开票地址都不同").FontSize(11).
					TextColor(dim)
			})
		}
	})

	if rt.errMsg != "" {
		myui.Text(c, rt.errMsg).FontSize(12).TextColor(myui.Hex(colDanger)).
			Padding(0, 14, 0, 0)
	}
	if rt.busy {
		myui.Text(c, "正在连接中继…").FontSize(12).TextColor(gold).
			Padding(0, 14, 0, 0)
	}
}

// peerDetail 客户端详情
func (a *App) peerDetail(c *myui.Context, d core.Device) {
	cream := myui.Hex(colCream)
	dim := myui.Hex(colDim)
	gold := myui.Hex(colGold)
	green := myui.Hex("#5FBF8F")

	rt := a.runtimeOf(d.ID)
	if rt == nil {
		rt = &runtime{}
	}
	connected := rt.cli != nil

	myui.Row(c).Gap(12).Children(func() {
		myui.Text(c, d.Name).Font(fontUI).FontSize(24).FontWeight(700).
			TextColor(cream)
		if connected {
			myui.Text(c, fmt.Sprintf("已连接 %v", rt.lat.Round(time.Millisecond))).
				FontSize(11.5).TextColor(green)
		}
		myui.Row(c).Grow(1)
		if connected {
			myui.Toolbar(c, func() {
				if myui.Button(c, "断开").Clicked() {
					a.stopPeer(d.ID)
				}
			})
		} else if !rt.busy {
			if myui.PrimaryButton(c, "连接").Width(96).Clicked() {
				a.startPeer(d.ID)
			}
		}
		myui.Toolbar(c, func() {
			if myui.Button(c, "删除").Clicked() {
				a.removeDevice(d.ID)
			}
		})
	})

	myui.Column(c).Padding(0, 14, 0, 14).Children(func() {
		myui.Row(c).Height(1).FillWidth().Background(myui.Hex(colLine))
	})

	myui.Column(c).Gap(10).FillWidth().Padding(0, 24, 0, 0).Children(func() {
		myui.Text(c, "对方的票面").FontSize(11).FontWeight(600).TextColor(dim)
		if connected {
			myui.Text(c, d.Addr).Font(fontMono).FontSize(10.5).
				FillWidth().TextColor(myui.Hex("#C6D2CE"))
		} else {
			myui.TextInput(c, a.addressOf(d.ID))
		}
	})

	myui.Column(c).Gap(8).FillWidth().Padding(0, 22, 0, 0).Children(func() {
		myui.Text(c, "端口映射").FontSize(11).FontWeight(600).TextColor(dim)
		for i := range a.devices {
			if a.devices[i].ID != d.ID {
				continue
			}
			for j := range a.devices[i].Forwards {
				i, j := i, j
				if connected {
					myui.Text(c, a.devices[i].Forwards[j]).Font(fontMono).
						FontSize(12.5).TextColor(myui.Hex("#C6D2CE"))
					continue
				}
				myui.Row(c).Gap(8).Children(func() {
					myui.TextInput(c, &a.devices[i].Forwards[j]).Width(260)
					if len(a.devices[i].Forwards) > 1 {
						myui.Toolbar(c, func() {
							if myui.Button(c, "移除").Clicked() {
								a.removeForward(i, j)
							}
						})
					}
				})
			}
		}
		if !connected {
			myui.Toolbar(c, func() {
				if myui.Button(c, "添加映射").Clicked() {
					a.addForward(d.ID)
				}
			})
		}
	})

	if rt.errMsg != "" {
		myui.Text(c, rt.errMsg).FontSize(12).TextColor(myui.Hex(colDanger)).
			Padding(0, 14, 0, 0)
	}
	if rt.note != "" {
		myui.Text(c, rt.note).FontSize(12).TextColor(gold).
			Padding(0, 10, 0, 0)
	}
}

// ---------------------------------------------------------------- 行为

func findDevice(list []core.Device, id string) (core.Device, bool) {
	for _, d := range list {
		if d.ID == id {
			return d, true
		}
	}
	return core.Device{}, false
}

func (a *App) runtimeOf(id string) *runtime {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.rt[id]
}

func (a *App) ensureRuntime(id string) *runtime {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.rt[id] == nil {
		a.rt[id] = &runtime{}
	}
	return a.rt[id]
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

// persistentOf 返回设备 persistent 字段的指针，供 Switch 修改。
func (a *App) persistentOf(id string) *bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.devices {
		if a.devices[i].ID == id {
			return &a.devices[i].Persistent
		}
	}
	// 兜底：返回一个临时值，避免 panic
	return new(bool)
}

// addressOf 返回设备地址字段的指针，供 TextInput 修改。
func (a *App) addressOf(id string) *string {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.devices {
		if a.devices[i].ID == id {
			return &a.devices[i].Addr
		}
	}
	return new(string)
}

func (a *App) addDevice(role string) {
	name := "本机"
	if role == core.RolePeer {
		name = "对方"
	}
	d := newDevice(role, fmt.Sprintf("%s %d", name, len(a.devices)+1))

	a.mu.Lock()
	a.devices = append(a.devices, d)
	a.sel = d.ID
	list := append([]core.Device(nil), a.devices...)
	a.mu.Unlock()

	_ = core.SaveDevices(list)
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
	delete(a.rt, id)
	if a.sel == id && len(a.devices) > 0 {
		a.sel = a.devices[0].ID
	}
	list := append([]core.Device(nil), a.devices...)
	a.mu.Unlock()

	_ = core.SaveDevices(list)
	a.refresh()
}

func (a *App) save() {
	a.mu.Lock()
	list := append([]core.Device(nil), a.devices...)
	a.mu.Unlock()
	_ = core.SaveDevices(list)
}

func (a *App) addExpose(id string) {
	a.mu.Lock()
	for i := range a.devices {
		if a.devices[i].ID == id {
			a.devices[i].Exposes = append(a.devices[i].Exposes, "")
		}
	}
	a.mu.Unlock()
	a.refresh()
}

func (a *App) removeExpose(i, j int) {
	a.mu.Lock()
	if i < len(a.devices) && j < len(a.devices[i].Exposes) {
		e := a.devices[i].Exposes
		a.devices[i].Exposes = append(e[:j], e[j+1:]...)
	}
	a.mu.Unlock()
	a.save()
	a.refresh()
}

func (a *App) addForward(id string) {
	a.mu.Lock()
	for i := range a.devices {
		if a.devices[i].ID == id {
			a.devices[i].Forwards = append(a.devices[i].Forwards, "")
		}
	}
	a.mu.Unlock()
	a.refresh()
}

func (a *App) removeForward(i, j int) {
	a.mu.Lock()
	if i < len(a.devices) && j < len(a.devices[i].Forwards) {
		f := a.devices[i].Forwards
		a.devices[i].Forwards = append(f[:j], f[j+1:]...)
	}
	a.mu.Unlock()
	a.save()
	a.refresh()
}

// deviceStatus 返回列表里显示的状态文字与颜色。
func (a *App) deviceStatus(d core.Device) (string, myui.Color) {
	a.mu.Lock()
	rt := a.rt[d.ID]
	a.mu.Unlock()

	idle := myui.Hex(colDim)
	if rt == nil {
		if d.Role == core.RoleHost {
			return "未开票", idle
		}
		return "未连接", idle
	}
	if rt.errMsg != "" {
		return "出错", myui.Hex(colDanger)
	}
	if rt.busy {
		return "连接中…", myui.Hex(colGold)
	}
	if rt.srv != nil {
		return "已开票", myui.Hex("#5FBF8F")
	}
	if rt.cli != nil {
		return "已连接", myui.Hex("#5FBF8F")
	}
	if d.Role == core.RoleHost {
		return "未开票", idle
	}
	return "未连接", idle
}

// startHost 按设备配置开票。
func (a *App) startHost(id string) {
	a.mu.Lock()
	var dev core.Device
	for _, d := range a.devices {
		if d.ID == id {
			dev = d
		}
	}
	rt := a.rt[id]
	if rt == nil {
		rt = &runtime{}
		a.rt[id] = rt
	}
	rt.errMsg = ""
	rt.busy = true
	a.mu.Unlock()
	a.refresh()

	targets := map[uint16]string{}
	for _, spec := range dev.Exposes {
		port, target, err := parseExpose(spec)
		if err != nil {
			a.finishHost(id, nil, err)
			return
		}
		targets[port] = target
	}
	if len(targets) == 0 {
		a.finishHost(id, nil, fmt.Errorf("至少填一个端口"))
		return
	}

	opts := core.ServerOptions{Targets: targets}
	if dev.Persistent {
		opts.KeyName = "device-" + id
	}

	go func() {
		srv, err := core.StartServer(opts)
		a.finishHost(id, srv, err)
	}()
}

func (a *App) finishHost(id string, srv *core.Server, err error) {
	rt := a.ensureRuntime(id)
	a.mu.Lock()
	rt.busy = false
	if err != nil {
		rt.errMsg = err.Error()
	} else {
		rt.srv = srv
		rt.addr = srv.Address()
		fmt.Fprintf(os.Stderr, "clowder: 设备 %s 地址 %s\n", id, rt.addr)
	}
	a.mu.Unlock()
	a.refresh()
}

func (a *App) stopHost(id string) {
	a.mu.Lock()
	rt := a.rt[id]
	var srv *core.Server
	if rt != nil {
		srv = rt.srv
		rt.srv = nil
		rt.addr = ""
		rt.errMsg = ""
	}
	a.mu.Unlock()

	if srv != nil {
		_ = srv.Close()
	}
	a.refresh()
}

// startPeer 连接对方并建立映射。
func (a *App) startPeer(id string) {
	a.mu.Lock()
	var dev core.Device
	for _, d := range a.devices {
		if d.ID == id {
			dev = d
		}
	}
	rt := a.rt[id]
	if rt == nil {
		rt = &runtime{}
		a.rt[id] = rt
	}
	rt.errMsg = ""
	rt.note = ""
	rt.busy = true
	a.mu.Unlock()
	a.refresh()

	addr := strings.TrimSpace(dev.Addr)
	if addr == "" {
		a.finishPeer(id, nil, 0, fmt.Errorf("先粘贴对方的地址"))
		return
	}

	parsed := make([][2]uint16, 0, len(dev.Forwards))
	for _, spec := range dev.Forwards {
		local, remote, err := parseForward(spec)
		if err != nil {
			a.finishPeer(id, nil, 0, err)
			return
		}
		parsed = append(parsed, [2]uint16{local, remote})
	}

	go func() {
		c, err := core.NewClient(addr)
		if err != nil {
			a.finishPeer(id, nil, 0, err)
			return
		}
		lat, err := c.Ping(30 * time.Second)
		if err != nil {
			_ = c.Close()
			a.finishPeer(id, nil, 0, fmt.Errorf("连不上对方：%w", err))
			return
		}
		for _, m := range parsed {
			if _, err := c.Forward(fmt.Sprintf("127.0.0.1:%d", m[0]), m[1]); err != nil {
				_ = c.Close()
				a.finishPeer(id, nil, lat, fmt.Errorf("本地端口 %d 被占用：%w", m[0], err))
				return
			}
		}
		a.finishPeer(id, c, lat, nil)
	}()
}

func (a *App) finishPeer(id string, c *core.Client, lat time.Duration, err error) {
	rt := a.ensureRuntime(id)
	a.mu.Lock()
	rt.busy = false
	if err != nil {
		rt.errMsg = err.Error()
	} else {
		rt.cli = c
		rt.lat = lat
		rt.note = "已建立映射，访问本地端口即可"
		fmt.Fprintf(os.Stderr, "clowder: 设备 %s 已连接 %v\n", id, lat)
	}
	a.mu.Unlock()
	a.refresh()
}

func (a *App) stopPeer(id string) {
	a.mu.Lock()
	rt := a.rt[id]
	var c *core.Client
	if rt != nil {
		c = rt.cli
		rt.cli = nil
		rt.note = ""
		rt.errMsg = ""
		rt.lat = 0
	}
	a.mu.Unlock()

	if c != nil {
		_ = c.Close()
	}
	a.refresh()
}

// qrImage 生成二维码，按设备与地址缓存。
func (a *App) qrImage(id, addr string) myui.ImageSource {
	rt := a.ensureRuntime(id)
	if rt.qrAddr == addr && rt.qrSrc != nil {
		return rt.qrSrc
	}
	png, err := qrcode.Encode(addr, qrcode.Medium, 640)
	if err != nil {
		return nil
	}
	src, err := myui.DecodeImage(png)
	if err != nil {
		return nil
	}
	rt.qrAddr = addr
	rt.qrSrc = src
	return src
}

// debugAutoStart 供无头截图使用。
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

// ---------------------------------------------------------------- 解析

func parseExpose(spec string) (uint16, string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return 0, "", fmt.Errorf("有空端口")
	}
	if i := strings.IndexByte(spec, '='); i >= 0 {
		port, err := parsePort(spec[:i])
		if err != nil {
			return 0, "", err
		}
		return port, strings.TrimSpace(spec[i+1:]), nil
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
		return 0, 0, fmt.Errorf("映射要写成 本地端口:远端端口")
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
		return 0, fmt.Errorf("端口无效: %q", s)
	}
	return uint16(n), nil
}

// sortDevices 保持插入顺序，仅作为稳定排序的占位。
func sortDevices(list []core.Device) []core.Device {
	sort.SliceStable(list, func(i, j int) bool { return list[i].CreatedAt.Before(list[j].CreatedAt) })
	return list
}