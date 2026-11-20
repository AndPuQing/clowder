// Command mockups 渲染 Clowder 的界面方向稿（第三轮：带艺术感）。
//
//   CLOWDER_MOCKUP=A  Poster  —— 排版即视觉
//   CLOWDER_MOCKUP=B  Diagram —— 把隧道画出来
//   CLOWDER_MOCKUP=C  Ticket  —— 地址是一张票
package main

import (
	"os"

	"github.com/egoist/mygo"
	myui "github.com/egoist/mygo/ui"
	"github.com/skip2/go-qrcode"
)

const (
	fontUI   = "Noto Sans CJK SC"
	fontMono = "Noto Sans Mono CJK SC"

	ink    = "#101113"
	paper  = "#FAFAF7"
	spot   = "#C03B25" // 朱
	calm   = "#F4F5F3"
	calmIn = "#15181A"
	teal   = "#0F6E63"
	deep   = "#12221E"
	cream  = "#F2EDE3"
	gold   = "#C9A227"
)

// 展示用的样例地址（真实长度）
const sampleAddr = "kHBrZxJsdB_CPdGdTQxUzc3JDgTJW_mmxyxbhOtfnPYqAeJZzzlYcBUHIYwlLnEdxTlLWpOWFKufsMabNrWXsBcOYnINedlWyHFCwCrAhDLXhwdbHWSYFVOMRBrkaKGtfsQfTLZLhWLKjPqPwdDlj"

func main() {
	dir := os.Getenv("CLOWDER_MOCKUP")
	if dir == "" {
		dir = "A"
	}
	qr := mustQR(sampleAddr)

	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:   "Clowder — " + dir,
			Width:   880,
			Height:  600,
			Content: myui.View(viewFor(dir, qr)),
		})
	})
	if err := mygo.App.Run(); err != nil {
		panic(err)
	}
}

func viewFor(dir string, qr myui.ImageSource) func(*myui.Context) {
	switch dir {
	case "B":
		return diagram(qr)
	case "C":
		return ticket(qr)
	default:
		return poster(qr)
	}
}

func mustQR(s string) myui.ImageSource {
	png, err := qrcode.Encode(s, qrcode.Medium, 640)
	if err != nil {
		panic(err)
	}
	src, err := myui.DecodeImage(png)
	if err != nil {
		panic(err)
	}
	return src
}

// dot 画一个实心圆
func dot(c *myui.Context, d float32, col myui.Color) {
	myui.Row(c).Size(d, d).Radius(d / 2).Background(col)
}

// line 画一条细线
func line(c *myui.Context, w, h float32, col myui.Color) {
	myui.Row(c).Width(w).Height(h).Background(col)
}

// ---------------------------------------------------------------------------
// A. Poster —— 排版即视觉：标题巨大，地址当正文，二维码退成印章
// ---------------------------------------------------------------------------

func poster(qr myui.ImageSource) func(*myui.Context) {
	return func(c *myui.Context) {
		bg := myui.Hex(paper)
		fg := myui.Hex(ink)

		myui.Column(c).Fill().Background(bg).Padding(56, 44).Gap(0).Children(func() {
			// 顶部：只有一个动作
			myui.Row(c).Children(func() {
				myui.Text(c, "Clowder").Font(fontUI).FontSize(15).FontWeight(500).
					TextColor(fg)
				myui.Row(c).Grow(1)
				myui.Text(c, "正在等待连接").FontSize(13).TextColor(myui.Hex("#6B6B66"))
			})

			myui.Row(c).Grow(1)

			// 主体：大标题 + 地址正文
			myui.Column(c).Gap(30).Width(560).Children(func() {
				myui.Text(c, "把这串东西\n交给对方").
					Font(fontUI).FontSize(52).FontWeight(700).
					LetterSpacing(-1).TextColor(fg)

				myui.Text(c, sampleAddr).
					Font(fontMono).FontSize(15).Width(560).
					TextColor(myui.Hex("#3A3A36"))
			})

			myui.Row(c).Grow(1)

			// 页脚：印章式二维码 + 一条朱线
			myui.Row(c).Children(func() {
				myui.Column(c).Gap(14).Children(func() {
					line(c, 120, 2, myui.Hex(spot))
					myui.Text(c, "扫码直达").FontSize(12).
						TextColor(myui.Hex("#6B6B66"))
				})

				myui.Row(c).Grow(1)

				myui.Column(c).Padding(10).Background(myui.Hex("#FFFFFF")).
					Radius(4).Children(func() {
					myui.Image(c, qr).Size(124, 124)
				})
			})
		})
	}
}

// ---------------------------------------------------------------------------
// B. Diagram —— 把隧道画出来：两个节点，一条线
// ---------------------------------------------------------------------------

func diagram(qr myui.ImageSource) func(*myui.Context) {
	return func(c *myui.Context) {
		bg := myui.Hex(calm)
		fg := myui.Hex(calmIn)
		ac := myui.Hex(teal)
		weak := myui.Hex("#7C8388")

		myui.Column(c).Fill().Background(bg).Padding(52, 44).Gap(0).Children(func() {
			myui.Text(c, "Clowder").Font(fontUI).FontSize(15).FontWeight(500).
				TextColor(fg)

			myui.Row(c).Grow(1)

			// 图：两个节点连成一条轴
			myui.Row(c).Gap(12).Children(func() {
				dot(c, 9, myui.Hex(teal))
				myui.Text(c, "本机").FontSize(14).FontWeight(600).TextColor(fg)
				myui.Text(c, "8080").Font(fontMono).FontSize(13).TextColor(weak)

				myui.Row(c).Grow(1).Height(1).Background(myui.Hex("#C9CFD2"))

				dot(c, 9, myui.Hex("#C3C9CB"))
				myui.Text(c, "对方").FontSize(14).FontWeight(600).TextColor(weak)
			})

			myui.Column(c).Padding(0, 22, 0, 0).Gap(18).Children(func() {
				myui.Row(c).Gap(34).Children(func() {
					myui.Column(c).Padding(10).Background(myui.Hex("#FFFFFF")).
						Radius(2).Children(func() {
						myui.Image(c, qr).Size(184, 184)
					})

					myui.Column(c).Gap(10).Width(340).Children(func() {
						myui.Text(c, "把地址或二维码给他").FontSize(13).
							TextColor(fg)
						myui.Text(c, sampleAddr).Font(fontMono).FontSize(11).
							Width(340).TextColor(weak)
						myui.Text(c, "对方在 Clowder 里粘贴即可连接").FontSize(12).
							TextColor(weak)
					})
				})
			})

			myui.Row(c).Grow(1)

			myui.Row(c).Gap(16).Children(func() {
				myui.Text(c, "停止").FontSize(13.5).FontWeight(500).TextColor(ac)
				myui.Text(c, "关掉窗口不影响连接").FontSize(12.5).TextColor(weak)
			})
		})
	}
}

// ---------------------------------------------------------------------------
// C. Ticket —— 地址是一张可以撕下来的票
// ---------------------------------------------------------------------------

func ticket(qr myui.ImageSource) func(*myui.Context) {
	return func(c *myui.Context) {
		bg := myui.Hex(deep)
		fg := myui.Hex(cream)
		g := myui.Hex(gold)

		myui.Column(c).Fill().Background(bg).Padding(44, 36).Gap(0).Children(func() {
			myui.Row(c).Children(func() {
				myui.Text(c, "Clowder").Font(fontUI).FontSize(15).FontWeight(500).
					TextColor(fg)
				myui.Row(c).Grow(1)
				myui.Text(c, "临时").Font(fontMono).FontSize(12).TextColor(g)
			})

			myui.Row(c).Grow(1)

			// 票根：左联二维码，右联地址与有效期，中间一条竖虚线
			myui.Row(c).Children(func() {
				myui.Row(c).Padding(26).Background(myui.Hex("#182C27")).
					Radius(6).Gap(24).Width(640).Children(func() {
					myui.Column(c).Padding(8).Background(myui.Hex("#FFFFFF")).
						Children(func() {
							myui.Image(c, qr).Size(150, 150)
						})

					myui.Row(c).Width(1).Height(150).
						Background(myui.Hex("#33504A"))

					myui.Column(c).Gap(10).Width(360).Children(func() {
						myui.Text(c, "单向通行").FontSize(26).FontWeight(700).
							TextColor(fg)
						myui.Text(c, "本机 8080 已开放").FontSize(13).
							TextColor(myui.Hex("#9DB3AC"))

						myui.Text(c, sampleAddr).Font(fontMono).FontSize(11).
							Width(360).TextColor(myui.Hex("#CBD8D3"))

						myui.Row(c).Gap(8).Children(func() {
							myui.Text(c, "有效期").FontSize(12).
								TextColor(myui.Hex("#9DB3AC"))
							myui.Text(c, "至停止时").FontSize(12).TextColor(g)
						})
					})
				})

				myui.Row(c).Grow(1)
			})

			myui.Row(c).Grow(1)

			myui.Row(c).Children(func() {
				myui.Text(c, "停止").FontSize(13.5).FontWeight(500).TextColor(g)
				myui.Row(c).Grow(1)
				myui.Text(c, "关掉窗口不影响连接").FontSize(12.5).
					TextColor(myui.Hex("#9DB3AC"))
			})
		})
	}
}