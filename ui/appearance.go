package ui

import myui "github.com/egoist/mygo/ui"

const (
	lightBackground = "#FAF9F7"
	darkBackground  = "#171719"

	radControl = 10
	radCard    = 18
	radRow     = 11

	railWidth      = 218
	railInset      = 10
	detailMaxWidth = 780
	detailPadding  = 30

	fontMono = "monospace"
)

type appearance struct {
	theme   *myui.Theme
	sidebar myui.Color
	panel   myui.Color
	divider myui.Color
	inset   myui.Color
}

var lightAppearance = func() appearance {
	t := myui.LightTheme().WithAccent(myui.Hex("#3047D8"))
	t.Background = myui.Hex(lightBackground)
	t.Surface = myui.Hex("#F3F2F0")
	t.SurfaceHover = myui.Hex("#EBEAE8")
	t.SurfacePressed = myui.Hex("#E1E0DE")
	t.Border = myui.Hex("#DFDEDC")
	t.Text = myui.Hex("#1B1B20")
	t.TextMuted = myui.Hex("#64646D")
	t.Success = myui.Hex("#1A7951")
	t.Danger = myui.Hex("#BB343D")
	t.Scrollbar = t.TextMuted.Alpha(0.42)
	t.Radius = radControl
	t.FontSize = 13
	return appearance{
		theme:   t,
		sidebar: myui.Hex("#EFEEEB"),
		panel:   myui.Hex("#FFFFFF"),
		divider: myui.Hex("#E9E7E4"),
		inset:   myui.Hex("#F5F4F2"),
	}
}()

var darkAppearance = func() appearance {
	t := myui.DarkTheme().WithAccent(myui.Hex("#A6B2FF"))
	t.Background = myui.Hex(darkBackground)
	t.Surface = myui.Hex("#303033")
	t.SurfaceHover = myui.Hex("#39393D")
	t.SurfacePressed = myui.Hex("#45454A")
	t.Border = myui.Hex("#3B3B40")
	t.Text = myui.Hex("#F3F2F5")
	t.TextMuted = myui.Hex("#A9A8B3")
	t.Success = myui.Hex("#6BD39C")
	t.Danger = myui.Hex("#FF8C94")
	t.Scrollbar = t.TextMuted.Alpha(0.4)
	t.Radius = radControl
	t.FontSize = 13
	return appearance{
		theme:   t,
		sidebar: myui.Hex("#212124"),
		panel:   myui.Hex("#252528"),
		divider: myui.Hex("#353539"),
		inset:   myui.Hex("#1E1E21"),
	}
}()

func currentAppearance(c *myui.Context) *appearance {
	if c.Theme().Dark {
		return &darkAppearance
	}
	return &lightAppearance
}

func separatorColor(c *myui.Context) myui.Color {
	if c.Preferences().HighContrast {
		return c.Theme().Border
	}
	return currentAppearance(c).divider
}
