package ui

import (
	"time"

	myui "github.com/egoist/mygo/ui"
)

// This file holds the small set of controls the interface is built from.
// Keeping them here keeps spacing, radius and colour consistent: a button
// looks the same on every page, and no page invents its own face.

// iconEl draws a vector icon at size, in col.
func iconEl(c *myui.Context, name string, size float32, col myui.Color) myui.Element {
	s := icon(name)
	if s == nil {
		return myui.Box(c).Size(size, size).Shrink(0)
	}
	return myui.Icon(c, s).Size(size, size).TextColor(col).Shrink(0)
}

// iconText is a row of an icon and a label, vertically centred.
func iconText(c *myui.Context, name, label string, size, fontSize float32, col myui.Color) myui.Element {
	return myui.Row(c).Gap(7).AlignItems(myui.Center).Children(func() {
		if name != "" {
			iconEl(c, name, size, col)
		}
		myui.Text(c, label).FontSize(fontSize).TextColor(col).SingleLine()
	})
}

// btnStyle is the weight of an action.
type btnStyle int

const (
	btnQuiet btnStyle = iota
	btnSolid
)

// actionButton is an icon-and-label button. Callers check Clicked as usual.
func actionButton(c *myui.Context, style btnStyle, iconName, label string) myui.Element {
	t := c.Theme()
	b := myui.ButtonBase(c).Padding(7, 12).MinHeight(34).Radius(radControl).Gap(7).
		Label(label).Cursor(myui.CursorPointer).
		Transition(myui.ElementTransition{
			Duration: 120 * time.Millisecond,
			Ease:     myui.EaseOut,
			Colors:   true,
			Shadows:  true,
		})

	base, hover, pressed, fg := t.Surface, t.SurfaceHover, t.SurfacePressed, t.Text
	if style == btnSolid {
		base, hover, pressed, fg = t.Accent, t.AccentHover, t.AccentPressed, t.AccentText
	}

	bg := base
	switch {
	case b.Pressed():
		bg = pressed
		b.Scale(0.97, 0.97)
	case b.Hovered() || b.Focused():
		bg = hover
	}
	b.Background(bg).TextColor(fg)
	if style == btnSolid {
		b.Shadow(0, 3, 8, -3, t.Accent.Alpha(0.45))
	}
	b.Children(func() {
		if iconName != "" {
			iconEl(c, iconName, 15, fg)
		}
		myui.Text(c, label).FontSize(13).FontWeight(600).TextColor(fg).SingleLine()
	})
	return b
}

// iconButton is a square control with only an icon. It shows label as its
// tooltip and its accessible name; MyGo drops the delay once another tip
// has shown, so moving along a row of them stays fast.
func iconButton(c *myui.Context, name, label string, col myui.Color) myui.Element {
	t := c.Theme()
	b := myui.ButtonBase(c).Size(34, 34).Radius(radControl).
		Tooltip(label).Label(label).Cursor(myui.CursorPointer).
		Transition(myui.ElementTransition{
			Duration: 120 * time.Millisecond,
			Ease:     myui.EaseOut,
			Colors:   true,
			Shadows:  true,
		})
	active := b.Hovered() || b.Focused() || b.Pressed()
	if name == "trash" && active {
		col = t.Danger
	}
	switch {
	case b.Pressed():
		b.Background(t.SurfacePressed)
		b.Scale(0.97, 0.97)
	case active:
		b.Background(t.SurfaceHover)
	}
	b.Children(func() { iconEl(c, name, 16, col) })
	return b
}

func navButton(c *myui.Context, selected bool) myui.Element {
	t := c.Theme()
	row := myui.ButtonBase(c).FillWidth().Justify(myui.Start).AlignItems(myui.Center).
		Gap(10).Padding(8, 10).Radius(radRow).Cursor(myui.CursorPointer).
		FocusRingOffset(-2).Selected(selected)
	switch {
	case row.Pressed():
		row.Background(t.SurfacePressed).Scale(0.97, 0.97)
	case selected:
		selectedBackground := currentAppearance(c).panel
		if t.Dark {
			selectedBackground = t.SurfaceHover
		}
		row.Background(selectedBackground)
	case row.Hovered():
		row.Background(t.SurfacePressed.Alpha(0.62))
	}
	return row
}

// sectionLabel is the small heading above a group.
func sectionLabel(c *myui.Context, label string) myui.Element {
	return myui.Text(c, label).FontSize(13).FontWeight(600).
		TextColor(c.Theme().Text).SingleLine()
}

func panel(c *myui.Context) myui.Element {
	return myui.Column(c).FillWidth().Radius(radCard).
		Background(currentAppearance(c).panel).Border(1, separatorColor(c)).
		Shadow(0, 3, 10, -7, myui.Hex("#000000").Alpha(0.18))
}

func formGroup(c *myui.Context, label string, rows func()) {
	myui.Column(c).FillWidth().Gap(8).Children(func() {
		sectionLabel(c, label).Padding(0, 3).TextColor(c.Theme().TextMuted)
		panel(c).Dividers(1, separatorColor(c)).Children(rows)
	})
}

func statusLine(c *myui.Context, status string, size float32) {
	t := c.Theme()
	dot := t.TextMuted.Alpha(0.65)
	switch status {
	case "state.starting", "state.connecting":
		dot = t.Accent
	case "state.running", "state.connected":
		dot = t.Success
	case "state.error":
		dot = t.Danger
	}
	myui.Row(c).Gap(6).AlignItems(myui.Center).Shrink(0).Children(func() {
		myui.Box(c).Size(7, 7).Radius(4).Background(dot).Shrink(0)
		myui.Text(c, T(status)).FontSize(size).TextColor(t.TextMuted).SingleLine()
	})
}

// switchRow is one line of an options card: icon, label, switch.
func switchRow(c *myui.Context, iconName, label string, on *bool) {
	t := c.Theme()
	myui.Row(c).FillWidth().AlignItems(myui.Center).Gap(10).Padding(11, 16).Children(func() {
		if iconName != "" {
			iconEl(c, iconName, 16, t.TextMuted)
		}
		myui.Text(c, label).FontSize(13).TextColor(t.Text).
			Grow(1).SingleLine()
		myui.Switch(c, on).Label(label).Cursor(myui.CursorPointer).Shrink(0)
	})
}

// fieldLabel is the caption above an input.
func fieldLabel(c *myui.Context, label string) {
	myui.Text(c, label).FontSize(12).FontWeight(600).
		TextColor(c.Theme().TextMuted).SingleLine()
}

// headerTile is the square mark at the top of a detail page.
func headerTile(c *myui.Context, name string) {
	t := c.Theme()
	myui.Box(c).Size(62, 62).Radius(18).Center().Shrink(0).
		Background(currentAppearance(c).panel).Border(1, separatorColor(c)).
		Shadow(0, 5, 12, -5, myui.Hex("#000000").Alpha(0.16)).Children(func() {
		iconEl(c, name, 30, t.Text.Mix(t.Accent, 0.78))
	})
}

// appear gives a panel that comes and goes a short entry and a faster
// exit: a state change should not look like a cut. It starts near its
// place, not from nothing. Key identifies it among its siblings.
func appear(key string, e myui.Element) myui.Element {
	return e.Key(key).Transition(myui.ElementTransition{
		Duration:     160 * time.Millisecond,
		Ease:         myui.CubicBezier(0.23, 1, 0.32, 1),
		Colors:       true,
		ExitDuration: 110 * time.Millisecond,
		Enter:        &myui.Motion{Y: 6, Opacity: 0, Scale: 0.99},
		Exit:         &myui.Motion{Y: 4, Opacity: 0, Scale: 0.99},
	})
}

// notice is an inline banner for an error or a warning.
func notice(c *myui.Context, name, msg string, col myui.Color) {
	n := myui.Row(c).FillWidth().Gap(9).AlignItems(myui.Center).Padding(10, 12).
		Radius(radControl).Background(col.Alpha(0.08)).
		Border(1, col.Alpha(0.22))
	n.Children(func() {
		iconEl(c, name, 15, col)
		myui.Text(c, msg).FontSize(12).TextColor(col).Grow(1)
	})
	appear("notice", n)
}

// segmentButton is one choice of a small segmented control. The selected
// segment is raised; the others are quiet until hovered.
func segmentButton(c *myui.Context, label string, selected bool) myui.Element {
	t := c.Theme()
	b := myui.ButtonBase(c).Padding(4, 10).Radius(6).
		Selected(selected).Label(label).Cursor(myui.CursorPointer).
		Transition(myui.ElementTransition{
			Duration: 120 * time.Millisecond,
			Ease:     myui.EaseOut,
			Colors:   true,
		})
	fg := t.TextMuted
	switch {
	case selected:
		bg := currentAppearance(c).panel
		if t.Dark {
			bg = t.SurfacePressed
		}
		b.Background(bg).Border(1, separatorColor(c)).
			Shadow(0, 1, 3, -1, myui.Hex("#000000").Alpha(0.18))
		fg = t.Text
	case b.Pressed():
		b.Background(t.SurfacePressed).Scale(0.97, 0.97)
	case b.Hovered():
		b.Background(t.SurfaceHover)
	}
	b.Children(func() {
		myui.Text(c, label).FontSize(11.5).FontWeight(600).
			TextColor(fg).SingleLine()
	})
	return b
}
