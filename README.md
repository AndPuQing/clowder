# Clowder

基于 [tailcat](https://github.com/tailscale/tailcat) 的无控制平面内网穿透工具，跨平台 GUI。

名字取自 "a clowder of cats"——一群猫，正好是 mesh 里节点彼此对等、互相连接的隐喻。

## 安装

从 [Releases](https://github.com/AndPuQing/clowder/releases) 下载对应平台的包。

| 平台 | 包 | 说明 |
|---|---|---|
| macOS | `clowder-darwin-arm64.zip` / `clowder-darwin-amd64.zip` | 解压后拖进「应用程序」 |
| Windows | `clowder-windows-amd64.zip` | 需要 WebView2（Win11 自带） |
| Linux | `clowder-linux-amd64.tar.gz` / `clowder-linux-arm64.tar.gz` | 需要 `libgtk-3-0`、`libwebkit2gtk-4.1-0` |

```bash
# Linux 运行
sudo apt install libgtk-3-0 libwebkit2gtk-4.1-0
tar -xzf clowder-linux-amd64.tar.gz
./clowder
```

## tailcat 是什么

Tailscale 官方开源项目（`github.com/tailscale/tailcat`，v0.7.0，Go 1.27.1），
"Tailscale 的数据平面，去掉 Tailscale 的控制平面"。

它复用的是 magicsock：**WireGuard 加密 + NAT 打洞 + DERP 中继兜底**。但没有账号、
没有登录、没有中心调度。

几个关键特性决定了本项目的形态：

| 特性 | 影响 |
|---|---|
| **无需 root / 管理员** | 全程 userspace（gVisor netstack），不开 TUN，不装驱动 |
| **无 IP 地址** | 节点不是"IP:端口"，而是一个 `tc...` 地址串 |
| **带外交换元数据** | 连接信息必须通过其他渠道传递——正好对应手动交换方案 |
| **DERP 兜底** | 打洞失败时走中继，可用官方 DERP 或自建 |
| **临时密钥为默认** | 每次启动生成新地址，进程退出即失效；需要固定地址得 genkey |

### 地址格式

`tc` 前缀 + base64 编码的 CBOR，内含：

- 服务端 WireGuard 公钥（Curve25519，32 字节）
- 独立的路径发现公钥
- 预共享密钥（PSK）——防 DERP 运营方旁观入网
- DERP 信息：区域 ID，或内嵌完整服务器元数据（`resolve` 后的长地址）

两种形式：**短地址**（引用 DERP 区域，客户端需拉 derpmap）、**长地址**（自包含，连接更快）。

## 本项目做什么

tailcat 是 CLI，本项目给它套一层桌面 GUI，把"复制粘贴地址串"这件事做得像产品：

- 服务端：一键监听，地址渲染成**二维码**，扫码即配对
- 客户端：扫码或粘贴地址，连接、断开、状态可视化
- 多节点：保存已连接节点，命名、分组、快速重连
- 端口转发：把本机端口暴露到隧道（对应 `tailcat serve`）
- 托盘常驻：关窗口不断连

## 技术栈

| 层 | 选型 |
|---|---|
| 语言 | Go 1.27.1 |
| 网络 | `github.com/tailscale/tailcat` v0.7.0 |
| GUI | [MyGo](https://mygo.egoist.dev/docs)（EGOIST，Go 原生桌面框架） |

MyGo 双模式：

- 主窗口用 **Web 前端**（节点列表、二维码、实时流量曲线）
- 托盘与状态小窗用 **原生 UI**（GPU 直绘，秒开、省内存）

## 结构

```
clowder/
├── core/          tailcat 封装 + 密钥存储（GUI 与 CLI 共用）
├── cmd/clowderd/  命令行前端（headless / 测试用）
└── ui/            MyGo 窗口，直接 import core
```

**tailcat 是内嵌的**：`core` 直接 `import "github.com/tailscale/tailcat"`，
编译进二进制，不调用外部 `tailcat` 命令，也不起 sidecar 进程。

因此不需要常驻守护进程。GUI 进程内直接持有隧道；关窗口只是隐藏窗口，
进程留在托盘，连接自然不断。`cmd/clowderd` 只是同一套 core 的另一个入口。

## 依赖的 tailcat API

```go
// 服务端
s := &tailcat.Server{
    OnTCP: func(port uint16) func(net.Conn) { ... },
}
s.Start()
addr := s.TailcatAddr()   // tc...

// 客户端
cl := tailcat.NewClient(tailcat.Addr(addrStr))
conn, err := cl.Dial("tcp", "127.0.0.1:8080")
```

零值即可工作：未设置的字段走默认（临时密钥、最近 DERP 区域、`log.Printf`）。

## 节点发现：手动交换

这是 tailcat 的原生模式，不是妥协。首版只做这个：

```
节点 A（服务端）                节点 B（客户端）
  │ 启动，得到 tc... 地址            │
  │ 渲染成二维码                     │
  │                                 │
  │←──────── 扫码 / 粘贴 ───────────│
  │                                 │
  └────── 打洞成功 → 直连 ──────────┘
         失败 → DERP 中继
```

明确不做：集中式信令服务器、账号体系、自动发现（mDNS/DHT 可后续作为可选插件）。

## 密钥与地址

tailcat 的地址由「节点私钥 + 预共享密钥」决定。两者不变，地址就不变。

| 模式 | 地址 | 适合 |
|---|---|---|
| 临时（默认） | 每次启动都变，进程退出即失效 | 一次性传输、临时排障 |
| 持久化（`-key <名字>`） | 跨重启稳定 | 常用节点、长期服务 |

持久化密钥存在 `~/.config/clowder/keys/<名字>.json`，目录 0700、文件 0600。

```bash
clowderd keys list              # 列出已有密钥及公钥
clowderd keys gen my-node       # 生成
clowderd keys show my-node      # 查看
clowderd keys rm my-node        # 删除
clowderd keys dir               # 配置目录
```

**风险**：持久化意味着"曾经分享过地址的人，以后都能连进来"。
GUI 里默认用临时密钥，切换持久化时必须让用户明确知情。
配合 `Server.AllowedClients`（客户端公钥白名单）可进一步收窄。

客户端也可以指定密钥（`-key`），这样公钥固定，服务端才能做白名单。

## 竞争项目

同赛道已有几个开源项目，立项前需要正面对比：

| 项目 | 星数 | 定位 |
|---|---|---|
| [Tailcat-Box](https://github.com/mushroom11s/Tailcat-Box) | 93 | 三平台桌面客户端，功能完整 |
| [whiskerlink](https://github.com/DavidCarliez/whiskerlink) | 76 | GUI，主打文件与服务分享 |
| [tailcat-webui](https://github.com/ca-x/tailcat-webui) | 47 | 多用户 Web 控制台 |

**Clowder 的差异点**要在这里找：Tailcat-Box 已覆盖"三平台桌面客户端"，
单纯再做一遍意义有限。可能的方向：更专注的扫码配对体验、更好的多节点管理、
或者面向团队的自建 DERP 集成。

## 从源码构建

```bash
go build -o clowder ./cmd/clowder     # 图形界面
go build -o clowderd ./cmd/clowderd   # 命令行
```

Linux 构建需要 `libgtk-3-dev libwebkit2gtk-4.1-dev`（cgo）。
CI 会在三个平台各原生构建一次，见 `.github/workflows/release.yml`。

## 开发环境

### 纯 Go 部分（core / cmd/clowderd）

不需要任何系统依赖。Go 工具链不在 PATH 里，用脚本引入：

```bash
source scripts/env.sh      # GOROOT/PATH/缓存/GOPROXY 一次搞定
go build ./...
```

### GUI 部分（ui / cmd/clowder）

宿主机是 **Ubuntu 20.04**，源里只有 WebKitGTK 4.0，而 MyGo 需要 **4.1**，
且 `libgtk-3-dev` 在本机有依赖冲突装不上。因此 GUI 一律在容器里构建：

```bash
docker build -t clowder-dev -f dev/Dockerfile .
dev/run.sh go build -o bin/clowder ./cmd/clowder   # 构建
dev/run.sh gui ./bin/clowder                       # Xvfb 无头运行并截图
```

容器基于 ubuntu:24.04，装了：

- `libgtk-3-dev` 3.24.41（原生 UI 必需）
- `libwebkit2gtk-4.1-dev` 2.52.6（Web 前端模式需要）
- `fonts-noto-cjk`（否则中文全是空白）
- `xvfb` + `imagemagick`（无头验证）

Go 工具链和 gocache/gomodcache 从宿主机挂载，容器无状态。
脚本以宿主机 uid 运行，构建产物不会变成 root 所有。

## 已有功能

多设备

- 左侧设备列表，右侧详情；每台设备独立运行互不影响
- 设备清单存 `~/.config/clowder/devices.json`，重启后恢复
- 一次点击新建「开票」或「加入」，也可删除

服务端

- 多端口暴露，每行可写 `8080` 或 `8080=127.0.0.1:8080`，可增删
- 一键开票，生成 `tc...` 地址并渲染成二维码
- 复制地址到系统剪贴板
- 临时密钥 / 固定地址（持久密钥）切换
- 停止即失效

客户端

- 粘贴地址，握手后显示往返延迟
- 多条端口映射（`本地端口:远端端口`），可增删
- 断开时回收本地监听

## 界面

方向为「票根」：深墨绿底、米色字、金色点缀，地址是一张可以交给对方的票。
设计规范在 `docs/DESIGN.md`，含三刻度设定和对照 AI-tells 清单的自查。

## 路线

- [x] **1. 最小闭环**：Server 起隧道打印地址，Client 连上并转发端口，端到端跑通
- [x] **2. 密钥持久化**：同一密钥重启后地址不变，已验证
- [x] **2.5 开发容器**：容器内构建 + 无头运行 MyGo 窗口，截图确认渲染正常
- [x] **3. MyGo 主窗口**：启动服务端、状态显示、地址渲染成二维码
- [ ] 4. 客户端侧：粘贴地址 / 扫码连接
- [ ] 5. 托盘常驻
- [ ] 6. 跨平台打包

## 已验证

```
服务端  clowderd serve -expose 18081
客户端  clowderd dial -addr tc... -forward 19081:18081
结果    curl 127.0.0.1:19081 → hello from clowder   ✅

持久化  同一 -key 两次启动，地址（233 字符）完全一致   ✅
临时    不加 -key 时每次地址都不同                    ✅
GUI     容器内 Xvfb 启动，窗口与中文渲染正常           ✅
二维码  截图裁剪后解码，内容与界面地址完全一致（232 字符）✅
端到端  GUI 当服务端开票 → clowderd 当客户端接入 → HTTP 穿透返回内容 ✅
```

## 无头调试开关

```bash
CLOWDER_AUTOSTART=server CLOWDER_GUI_WAIT=30 dev/run.sh gui ./bin/clowder  # 自动开票
CLOWDER_TAB=client dev/run.sh gui ./bin/clowder                            # 直接打开客户端页
CLOWDER_MOCKUP=A dev/run.sh gui ./bin/mockups                              # 渲染设计方向稿
```

## 调试开关

```bash
CLOWDER_AUTOSTART=1 CLOWDER_GUI_WAIT=35 dev/run.sh gui ./bin/clowder
```

- `CLOWDER_AUTOSTART=1`：窗口创建后自动启动服务端（省得手点按钮）
- `CLOWDER_GUI_WAIT=35`：截图前等待秒数（等隧道建好）
