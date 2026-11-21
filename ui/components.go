package ui

import (
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
	btnQuiet  btnStyle = iota // secondary: a surface with a hairline
	btnSolid                  // the page's main action, in the accent
	btnDanger                 // destructive, quiet until hovered
)

// actionButton is an icon-and-label button. Callers check Clicked as usual.
func actionButton(c *myui.Context, style btnStyle, iconName, label string) myui.Element {
	b := myui.ButtonBase(c).Padding(7, 11).Radius(radControl).Gap(7).Shrink(0)

	base := myui.Hex(colPanel)
	hover := myui.Hex(colPanelHi)
	fg := myui.Hex(colCream)
	danger := myui.Hex(colDanger)
	switch style {
	case btnSolid:
		base, hover, fg = myui.Hex(colGold), myui.Hex(colGoldHi), myui.Hex(colInk)
	case btnDanger:
		base, hover, fg = myui.Transparent, myui.Hex("#2A1A17"), danger
	}

	bg := base
	switch {
	case b.Pressed():
		bg = base.Mix(myui.Hex("#000000"), 0.18)
		b.Scale(0.97, 0.97)
	case b.Hovered():
		bg = hover
	}
	b.Background(bg).TextColor(fg)
	if style == btnQuiet {
		b.Border(1, myui.Hex(colLine))
	}
	if style == btnSolid {
		// A short drop below the control; the accent should sit above the
		// surface, not float on it.
		b.Shadow(0, 4, 12, -6, myui.Hex("#000000").Alpha(0.45))
	}
	b.Children(func() {
		if iconName != "" {
			iconEl(c, iconName, 15, fg)
		}
		myui.Text(c, label).FontSize(12.5).FontWeight(600).TextColor(fg).SingleLine()
	})
	return b
}

// iconButton is a square control with only an icon.
func iconButton(c *myui.Context, name string, col myui.Color) myui.Element {
	b := myui.ButtonBase(c).Size(30, 30).Center().Radius(radControl).Shrink(0)
	switch {
	case b.Pressed():
		b.Background(myui.Hex(colLine))
		b.Scale(0.94, 0.94)
	case b.Hovered():
		b.Background(myui.Hex(colPanelHi))
	}
	b.Children(func() { iconEl(c, name, 15, col) })
	return b
}

// sectionLabel is the small heading above a group.
func sectionLabel(c *myui.Context, label string) {
	myui.Text(c, label).FontSize(11).FontWeight(600).
		TextColor(myui.Hex(colDim)).SingleLine()
}

// panel is a raised surface: a subtle top-to-bottom gradient, a hairline
// and a shadow, so it reads as a card rather than a painted rectangle.
func panel(c *myui.Context, width float32) myui.Element {
	p := myui.Column(c).Gap(0).Radius(radCard).
		Border(1, myui.Hex(colLine)).
		LinearGradient(myui.LinearGradient{
			From:  myui.Hex(colPanelHi),
			To:    myui.Hex(colPanel),
			Angle: 180,
		}).
		Shadow(0, 10, 28, -18, myui.Hex("#000000").Alpha(0.55))
	if width > 0 {
		p.Width(width)
	} else {
		p.FillWidth()
	}
	return p
}

// statusChip is a small pill with a status dot and a label.
func statusChip(c *myui.Context, label string, dot myui.Color) myui.Element {
	chip := myui.Row(c).Gap(6).AlignItems(myui.Center).Padding(4, 9).Radius(20).Shrink(0).
		AlignSelf(myui.Start).
		Background(dot.Alpha(0.10)).Border(1, dot.Alpha(0.22))
	chip.Children(func() {
		myui.Box(c).Size(6, 6).Radius(3).Background(dot).Shrink(0)
		myui.Text(c, label).FontSize(11.5).FontWeight(600).TextColor(dot).SingleLine()
	})
	return chip
}

// switchRow is one line of an options card: icon, label, switch.
func switchRow(c *myui.Context, iconName, label string, on *bool) {
	myui.Row(c).FillWidth().AlignItems(myui.Center).Gap(10).Padding(10, 12).Children(func() {
		if iconName != "" {
			iconEl(c, iconName, 16, myui.Hex(colDim))
		}
		myui.Text(c, label).FontSize(12.5).TextColor(myui.Hex(colCream)).
			Grow(1).SingleLine()
		myui.Switch(c, on)
	})
}

// fieldLabel is the caption above an input.
func fieldLabel(c *myui.Context, label string) {
	myui.Text(c, label).FontSize(11).FontWeight(600).
		TextColor(myui.Hex(colDim)).SingleLine()
}

// headerTile is the square mark at the top of a detail page.
func headerTile(c *myui.Context, name string, fg myui.Color) {
	tile := myui.Box(c).Size(44, 44).Radius(11).Center().Shrink(0).
		Border(1, myui.Hex(colLineHi)).
		LinearGradient(myui.LinearGradient{
			From:  myui.Hex(colPanelHi),
			To:    myui.Hex(colPanel),
			Angle: 180,
		}).
		Shadow(0, 6, 16, -10, myui.Hex("#000000").Alpha(0.55))
	tile.Children(func() { iconEl(c, name, 22, fg) })
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
}

// segmentButton is one choice of a small segmented control. The selected
// segment is raised; the others are quiet until hovered.
func segmentButton(c *myui.Context, label string, selected bool) myui.Element {
	b := myui.ButtonBase(c).Padding(4, 10).Radius(6).Shrink(0)
	fg := myui.Hex(colText2)
	switch {
	case selected:
		b.Background(myui.Hex(colPanelHi)).Border(1, myui.Hex(colLineHi))
		fg = myui.Hex(colCream)
	case b.Hovered():
		b.Background(myui.Hex(colHover))
	}
	b.Children(func() {
		myui.Text(c, label).FontSize(11.5).FontWeight(600).
			TextColor(fg).SingleLine()
	})
	return b
}
