package ui

import (
	"fmt"
	"path/filepath"
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

func (a *App) drawTabs(start, width int) {
	a.tabs = nil
	docs := append([]*editor.Document(nil), a.docs...)
	if a.comparison != nil {
		docs = append(docs, nil)
	}
	name := func(d *editor.Document) string {
		if d == nil {
			return "Diff · " + filepath.Base(a.comparison.Pair.Path)
		}
		return d.Name()
	}
	path := func(d *editor.Document) string {
		if d != nil {
			return d.Path
		}
		return a.comparison.Pair.Path
	}
	twidth := func(d *editor.Document) int {
		return min(width, 28, text.Width(name(d), 4)+6+a.fileIconSpan(path(d), false, false))
	}
	activeIndex := 0
	for i, d := range docs {
		if d == nil && a.comparing() || d != nil && !a.comparing() && d == a.view().Doc {
			activeIndex = i
		}
	}
	first, used := 0, 0
	for i := activeIndex; i >= 0; i-- {
		tw := twidth(docs[i])
		if used+tw > width {
			first = i + 1
			break
		}
		used += tw
	}
	x := start
	for _, d := range docs[first:] {
		tw := twidth(d)
		if x+tw > start+width {
			break
		}
		active := d == nil && a.comparing() || d != nil && !a.comparing() && d == a.view().Doc
		style := a.chrome.Foreground(a.colors.muted)
		if active {
			style = a.base.Bold(true)
		}
		a.fill(rect{x, 1, tw, 1}, style)
		if active {
			a.put(x, 1, 1, "▎", a.accent)
		}
		n := a.drawFileIcon(x+2, 1, max(0, tw-6), path(d), false, false, style)
		a.put(x+2+n, 1, tw-6-n, fitLabel(name(d), tw-6-n), style)
		if d != nil && d.Dirty() {
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
	if a.comparing() {
		right = fmt.Sprintf("+%d / −%d lines   Read-only diff", a.comparison.Added, a.comparison.Removed)
	}
	if text.Width(right, 4) > width-20 {
		v := a.view()
		row, col := v.Doc.Buffer.Position(v.Head())
		right = positionLabel(row, text.Column(v.Doc.Buffer.Line(row), col, a.cfg.TabSize))
		if a.comparing() {
			right = "Read-only diff"
		}
	}
	right = fitLabel(right, max(0, width-2))
	rightX := max(1, width-text.Width(right, 4)-1)
	focus := strings.ToUpper(a.focus)
	switch a.focus {
	case "gitMessage":
		focus = "COMMIT"
	case "gitFilter":
		focus = "GIT FILTER"
	case "searchInput":
		focus = "SEARCH"
	case "tree":
		focus = "EXPLORER"
	case "filter":
		focus = "FILE FILTER"
	}
	left := "git: " + branch + "   " + focus
	a.put(1, y, max(0, rightX-3), fitLabel(left, rightX-3), style.Foreground(a.colors.muted))
	a.put(rightX, y, width-rightX-1, right, style)
	a.put(0, y, 1, "▎", style.Foreground(a.accent.GetForeground()))
}
