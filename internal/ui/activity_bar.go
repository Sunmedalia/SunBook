package ui

import (
	"time"

	"sunbook/internal/text"
)

// Nerd Font symbols keep the activities compact and visually consistent.
// Unicode mode is available for terminals using an unpatched font.
func (a *App) drawActivityBar() {
	r := a.activityRect
	if r.W == 0 {
		return
	}
	a.fill(r, a.base.Background(a.colors.inset))
	entries := []struct{ view, id, hint, icon, fallback string }{
		{"Explorer", "activity.explorer", "Explorer (Ctrl+Shift+E)", "\uf07b", "▤"},
		{"Search", "activity.search", "Search workspace (Ctrl+Shift+F)", "\uf002", "⌕"},
		{"Git", "activity.git", "Source Control (Ctrl+Shift+G)", "\uf126", "⎇"},
		{"Plugins", "activity.plugins", "Plugins: install and manage extensions", "\uf12e", "◇"},
	}
	for i, entry := range entries {
		tile := rect{r.X, r.Y + i*3, r.W, 3}
		if tile.Y+tile.H > r.Y+r.H {
			break
		}
		active := a.sidebar && a.sidebarView == entry.view
		style := a.base.Background(a.colors.inset).Foreground(a.colors.muted)
		// Only the center row has a face; vertical breathing room stays quiet.
		face := rect{tile.X + 1, tile.Y + 1, tile.W - 2, 1}
		if a.hoverCommand == entry.id {
			style = style.Background(a.colors.hover).Foreground(a.base.GetForeground())
		}
		if active {
			style = style.Background(mix(a.colors.inset, a.accent.GetForeground(), 14)).Foreground(a.accent.GetForeground())
		}
		if a.pressedCommand == entry.id && time.Now().Before(a.pressedUntil) {
			style = style.Background(a.colors.selection).Foreground(a.accent.GetForeground())
		}
		a.fill(face, style)
		icon := entry.icon
		if a.cfg.ActivityIcons == "unicode" {
			icon = entry.fallback
		}
		iconWidth := text.Width(icon, 4)
		x := face.X + max(0, (face.W-iconWidth)/2)
		a.put(x, face.Y, iconWidth, icon, style)
		if active {
			a.put(r.X, face.Y, 1, "▎", a.accent.Background(a.colors.inset))
		}
		a.buttons = append(a.buttons, button{rect: tile, Command: entry.id, Hint: entry.hint})
	}
	tile := rect{r.X, r.Y + r.H - 3, r.W, 3}
	if tile.Y >= r.Y+len(entries)*3 {
		style := a.base.Background(a.colors.inset).Foreground(a.colors.muted)
		if a.hoverCommand == "app.settings" {
			style = style.Background(a.colors.hover).Foreground(a.accent.GetForeground())
		}
		if a.modal != nil && (a.modal.Title == "Settings" || a.modal.Title == "Color Theme" || a.modal.Title == "File Icon Theme" || a.modal.Title == "Button & Sidebar Icons") {
			style = style.Background(a.colors.selection).Foreground(a.accent.GetForeground())
		}
		face := rect{tile.X + 1, tile.Y + 1, tile.W - 2, 1}
		a.fill(face, style)
		icon := a.actionIcon("app.settings", "Settings")
		width := text.Width(icon, 4)
		a.put(face.X+max(0, (face.W-width)/2), face.Y, width, icon, style)
		a.buttons = append(a.buttons, button{rect: tile, Command: "app.settings", Hint: "Settings: file icons, button icons, color theme"})
	}
}
