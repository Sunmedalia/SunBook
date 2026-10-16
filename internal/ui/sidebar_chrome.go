package ui

import (
	"strings"

	"github.com/gdamore/tcell/v3"
	"sunbook/internal/text"
)

// Shared surfaces keep the three sidebars visually aligned without consuming
// extra rows needed by search results, file trees, or the resizable Git log.
func (a *App) drawSidebarHeader(title string, reserved int) {
	r := a.sideRect
	st := a.chrome.Background(a.colors.inset).Bold(true)
	a.fill(rect{r.X, r.Y, r.W, 1}, st)
	a.put(r.X, r.Y, 1, "▎", st.Foreground(a.accent.GetForeground()))
	a.put(r.X+1, r.Y, r.W-2-reserved, fitLabel(title, r.W-2-reserved), st)
}
func (a *App) sidebarBand(y int) tcell.Style {
	st := a.chrome.Background(a.colors.line)
	a.fill(rect{a.sideRect.X, y, a.sideRect.W, 1}, st)
	return st
}
func (a *App) sidebarCount(right, y int, count string, style tcell.Style) int {
	width := text.Width(count, 4) + 2
	x := right - width
	st := style.Background(a.colors.raised).Foreground(a.colors.muted)
	a.fill(rect{x, y, width, 1}, st)
	a.put(x+1, y, width-2, count, st.Bold(true))
	return x
}
func (a *App) sidebarScroll(r rect, top, total int) {
	if r.H <= 0 || r.W <= 0 || total <= r.H {
		return
	}
	size := max(1, r.H*r.H/total)
	offset := (r.H - size) * max(0, min(top, total-r.H)) / max(1, total-r.H)
	x := r.X + r.W - 1
	for i := 0; i < r.H; i++ {
		symbol := "│"
		st := a.chrome.Foreground(a.colors.border)
		if i >= offset && i < offset+size {
			symbol = "▐"
			st = st.Foreground(a.colors.muted)
		}
		a.put(x, r.Y+i, 1, symbol, st)
	}
}
func (a *App) sidebarGroup(y int, label, count string, reserved int) {
	st := a.sidebarBand(y).Foreground(a.colors.muted).Bold(true)
	end := a.sideRect.X + a.sideRect.W - 1 - reserved
	countX := a.sidebarCount(end, y, count, st)
	a.put(a.sideRect.X+1, y, countX-a.sideRect.X-2, fitLabel(strings.TrimSpace(label), countX-a.sideRect.X-2), st)
}
