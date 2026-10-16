package ui

import (
	"strings"

	"github.com/rivo/uniseg"
	"sunbook/internal/editor"
	"sunbook/internal/text"
)

func fitLabel(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if text.Width(s, 4) <= width {
		return s
	}
	end, used := 0, 0
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		if used+max(1, g.Width()) > width-1 {
			break
		}
		used += max(1, g.Width())
		_, end = g.Positions()
	}
	return s[:end] + "…"
}

func tabWidth(d *editor.Document) int {
	return min(28, text.Width(d.Name(), 4)+6)
}

func (a *App) drawTabs(start, width int) {
	a.tabs = nil
	activeIndex := 0
	for i, d := range a.docs {
		if d == a.view().Doc {
			activeIndex = i
		}
	}
	first, used := 0, 0
	for i := activeIndex; i >= 0; i-- {
		tw := min(width, tabWidth(a.docs[i]))
		if used+tw > width {
			first = i + 1
			break
		}
		used += tw
	}
	x := start
	for _, d := range a.docs[first:] {
		tw := min(width, tabWidth(d))
		if x+tw > start+width {
			break
		}
		style := a.chrome.Foreground(a.colors.muted)
		active := d == a.view().Doc
		if active {
			style = a.base.Bold(true)
		}
		a.fill(rect{x, 1, tw, 1}, style)
		if active {
			a.put(x, 1, 1, "▎", a.accent)
		}
		a.put(x+2, 1, tw-6, fitLabel(d.Name(), tw-6), style)
		if d.Dirty() {
			a.put(x+tw-4, 1, 1, "•", style.Foreground(a.accent.GetForeground()))
		}
		a.put(x+tw-2, 1, 1, "×", style.Foreground(a.colors.muted))
		a.tabs = append(a.tabs, struct {
			rect
			Doc *editor.Document
		}{rect{x, 1, tw, 1}, d})
		x += tw
	}
}

func (a *App) drawStatus(width, y int) {
	style := a.chrome
	a.fill(rect{0, y, width, 1}, style)
	branch := a.git.Branch
	if branch == "" {
		branch = "workspace"
	}
	right := a.status()
	if text.Width(right, 4) > width-20 {
		v := a.view()
		row, col := v.Doc.Buffer.Position(v.Head())
		right = positionLabel(row, text.Column(v.Doc.Buffer.Line(row), col, a.cfg.TabSize))
	}
	right = fitLabel(right, max(0, width-2))
	rightX := max(1, width-text.Width(right, 4)-1)
	left := "git: " + branch + "   " + strings.ToUpper(a.focus)
	a.put(1, y, max(0, rightX-3), fitLabel(left, rightX-3), style.Foreground(a.colors.muted))
	a.put(rightX, y, width-rightX-1, right, style)
	a.put(0, y, 1, "▎", style.Foreground(a.accent.GetForeground()))
}
