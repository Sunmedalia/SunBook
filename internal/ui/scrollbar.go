package ui

type contentScrollbar struct {
	rect
	Top   *int
	Total int
	Kind  string
	Group int
}

type scrollbarDrag struct {
	Top  *int
	Grab int
}

func scrollbarThumb(height, total, top int) (int, int) {
	size := min(height, max(1, height*height/max(1, total)))
	limit := max(0, total-height)
	start := (height - size) * max(0, min(top, limit)) / max(1, limit)
	return start, size
}

func (a *App) editorScrollbarWidth(group int) int {
	v, r := a.views[group], a.rects[group]
	if v != nil && r.W > a.gutter(v)+2 && r.H > 0 && v.Doc.Buffer.LineCount() > r.H {
		return 1
	}
	return 0
}

func (a *App) drawContentScrollbar(area rect, top *int, total int, kind string, group int) {
	if area.W <= 1 || area.H <= 0 || total <= area.H {
		return
	}
	bar := contentScrollbar{rect{area.X + area.W - 1, area.Y, 1, area.H}, top, total, kind, group}
	a.scrollbars = append(a.scrollbars, bar)
	start, size := scrollbarThumb(bar.H, total, *top)
	for row := 0; row < bar.H; row++ {
		glyph, style := "│", a.base.Foreground(a.colors.border)
		if kind == "modal" || kind == "search" {
			style = a.chrome.Foreground(a.colors.border)
		}
		if row >= start && row < start+size {
			glyph, style = "▐", style.Foreground(a.colors.muted)
			if a.scrollDrag != nil && a.scrollDrag.Top == top {
				style = style.Foreground(a.accent.GetForeground())
			}
		}
		a.put(bar.X, bar.Y+row, 1, glyph, style)
	}
}

func (a *App) scrollbarMouse(x, y int, pressed, fresh bool) bool {
	if a.scrollDrag != nil {
		drag := a.scrollDrag
		if !pressed {
			a.scrollDrag = nil
			return true
		}
		for _, bar := range a.scrollbars {
			if bar.Top == drag.Top && (a.modal == nil) == (bar.Kind != "modal") {
				a.moveScrollbar(bar, y, drag.Grab)
				return true
			}
		}
		a.scrollDrag = nil
		return true
	}
	if !fresh {
		return false
	}
	for _, bar := range a.scrollbars {
		if !bar.contains(x, y) || (a.modal == nil) != (bar.Kind != "modal") {
			continue
		}
		start, size := scrollbarThumb(bar.H, bar.Total, *bar.Top)
		grab := y - bar.Y - start
		if grab < 0 || grab >= size {
			grab = size / 2
			a.moveScrollbar(bar, y, grab)
		}
		a.scrollDrag = &scrollbarDrag{bar.Top, grab}
		a.drag = ""
		a.completion = nil
		switch bar.Kind {
		case "search":
			a.focus = "search"
		case "editor":
			a.active, a.focus = bar.Group, "editor"
		case "diff":
			a.comparison.Pane, a.focus = bar.Group, "diff"
		case "pluginPreview":
			a.focus = "markdown"
		}
		return true
	}
	return false
}

func (a *App) moveScrollbar(bar contentScrollbar, y, grab int) {
	_, size := scrollbarThumb(bar.H, bar.Total, *bar.Top)
	travel := bar.H - size
	position := max(0, min(travel, y-bar.Y-grab))
	*bar.Top = position * max(0, bar.Total-bar.H) / max(1, travel)
}
