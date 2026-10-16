package ui

import "github.com/gdamore/tcell/v3"

// Surfaces follow the configured background/foreground so custom themes retain
// the same hierarchy as the default SunBook night workbench.
type palette struct {
	border, raised, hover, inset, line, selection tcell.Color
	muted, disabled, folder, success, danger      tcell.Color
	shadow, onAccent                              tcell.Color
}

func mix(a, b tcell.Color, percent int32) tcell.Color {
	ar, ag, ab := a.RGB()
	br, bg, bb := b.RGB()
	return tcell.NewRGBColor(ar+(br-ar)*percent/100, ag+(bg-ag)*percent/100, ab+(bb-ab)*percent/100)
}

func (a *App) initTheme() {
	bg := tcell.GetColor(a.cfg.Theme.Background)
	fg := tcell.GetColor(a.cfg.Theme.Foreground)
	accent := tcell.GetColor(a.cfg.Theme.Accent)
	a.base = tcell.StyleDefault.Background(bg).Foreground(fg)
	a.colors = palette{
		border: mix(bg, fg, 18), raised: mix(bg, fg, 13), hover: mix(bg, fg, 23),
		inset: mix(bg, tcell.ColorBlack, 18), line: mix(bg, fg, 4),
		selection: mix(bg, accent, 24), muted: mix(bg, fg, 64), disabled: mix(bg, fg, 36),
		folder: tcell.GetColor("#8ABEB7"), success: tcell.GetColor("#9BC99A"), danger: tcell.GetColor("#E79A95"),
		shadow: mix(bg, tcell.ColorBlack, 40), onAccent: tcell.GetColor("#161E2B"),
	}
	r, g, b := accent.RGB()
	if r*299+g*587+b*114 < 145000 {
		a.colors.onAccent = tcell.GetColor("#F5F7FA")
	}
	a.muted = a.base.Foreground(a.colors.muted)
	a.accent = a.base.Foreground(accent)
	a.chrome = a.base.Background(mix(bg, fg, 5))
	a.selected = a.base.Background(a.colors.selection)
}

func (a *App) inputStyle(focused bool) tcell.Style {
	style := a.base.Background(a.colors.inset).Underline(tcell.UnderlineStyleSolid, a.colors.border)
	if focused {
		style = style.Underline(tcell.UnderlineStyleSolid, a.accent.GetForeground())
	}
	return style
}

// A file stays visibly selected when opening it transfers focus to the editor.
// Keep the marker separate from the icon so both remain readable in narrow rows.
func (a *App) drawFileSelection(r rect, focused bool) tcell.Style {
	style := a.selected
	if focused {
		style = style.Bold(true)
	} else {
		style = a.chrome.Background(mix(a.chrome.GetBackground(), a.accent.GetForeground(), 16))
	}
	a.fill(r, style)
	a.put(r.X, r.Y, 1, "▎", style.Foreground(a.accent.GetForeground()).Bold(true))
	return style
}
