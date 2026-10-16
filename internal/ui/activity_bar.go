package ui

import "time"

// Draw small line icons directly in terminal cells. They need no icon font and
// give Explorer, Search, and Source Control recognizable shapes at the same size.
func (a *App) drawActivityBar() {
	r := a.activityRect
	if r.W == 0 {
		return
	}
	a.fill(r, a.base.Background(a.colors.inset))
	entries := []struct {
		view, id, hint string
		icon           [3]string
	}{
		{"Explorer", "activity.explorer", "Explorer (Ctrl+Shift+E)", [3]string{" ┌┐", "┌┤│", "└┴┘"}},
		{"Search", "activity.search", "Search workspace (Ctrl+Shift+F)", [3]string{"╭╮ ", "╰╯╲", "   "}},
		{"Git", "activity.git", "Source Control (Ctrl+Shift+G)", [3]string{"○ ○", "│/ ", "○  "}},
	}
	for i, entry := range entries {
		tile := rect{r.X, r.Y + i*3, r.W, 3}
		if tile.Y+tile.H > r.Y+r.H {
			break
		}
		active := a.sidebar && a.sidebarView == entry.view
		style := a.base.Background(a.colors.inset).Foreground(a.colors.muted)
		if active {
			style = style.Background(a.colors.selection).Foreground(a.accent.GetForeground())
		}
		if a.hoverCommand == entry.id {
			style = style.Background(a.colors.hover).Foreground(a.base.GetForeground())
		}
		if a.pressedCommand == entry.id && time.Now().Before(a.pressedUntil) {
			style = style.Background(a.colors.selection).Foreground(a.accent.GetForeground())
		}
		a.fill(tile, style)
		for row, line := range entry.icon {
			a.put(tile.X+1, tile.Y+row, tile.W-2, line, style)
			if active {
				a.put(tile.X, tile.Y+row, 1, "▎", style.Foreground(a.accent.GetForeground()))
			}
		}
		a.buttons = append(a.buttons, button{rect: tile, Command: entry.id, Hint: entry.hint})
	}
}
