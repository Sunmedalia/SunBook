package ui

import (
	"sunbook/internal/config"
	"sunbook/internal/editor"
	"sunbook/internal/terminal"

	"github.com/gdamore/tcell/v3"
)

// Surfaces follow the configured background/foreground so custom themes retain
// the same hierarchy as the default SunBook night workbench.
type palette struct {
	border, raised, hover, inset, line, selection tcell.Color
	muted, disabled, folder, success, danger      tcell.Color
	shadow, onAccent                              tcell.Color
}

func mix(a, b tcell.Color, percent int32) tcell.Color {
	if !a.Valid() || !b.Valid() {
		return tcell.ColorDefault
	}
	ar, ag, ab := a.RGB()
	br, bg, bb := b.RGB()
	return tcell.NewRGBColor(ar+(br-ar)*percent/100, ag+(bg-ag)*percent/100, ab+(bb-ab)*percent/100)
}

func (a *App) initTheme() {
	a.themeColorCache = nil
	a.terminalColors = a.readTerminalColors()
	a.themeSyntax = a.cfg.Theme.Syntax
	bg := tcell.GetColor(a.cfg.Theme.Background)
	fg := tcell.GetColor(a.cfg.Theme.Foreground)
	accent := tcell.GetColor(a.cfg.Theme.Accent)
	if a.cfg.Theme.Name == "auto" || a.cfg.Theme.Name == "terminal-default" {
		if a.terminalColors.Background.Valid() {
			bg = a.terminalColors.Background
		}
		if lightBackground(bg) {
			fg, accent, a.themeSyntax = tcell.GetColor("#24292e"), tcell.GetColor("#0969da"), "github"
		}
		if a.terminalColors.Foreground.Valid() {
			fg = a.terminalColors.Foreground
		}
	}
	a.base = tcell.StyleDefault.Background(bg).Foreground(fg)
	a.colors = palette{
		border: mix(bg, fg, 18), raised: mix(bg, fg, 13), hover: mix(bg, fg, 23),
		inset: mix(bg, tcell.ColorBlack, 18), line: mix(bg, fg, 4),
		selection: mix(bg, accent, 24), muted: mix(bg, fg, 64), disabled: mix(bg, fg, 36),
		folder: tcell.GetColor("#8ABEB7"), success: tcell.GetColor("#9BC99A"), danger: tcell.GetColor("#E79A95"),
		shadow: mix(bg, tcell.ColorBlack, 40), onAccent: tcell.GetColor("#161E2B"),
	}
	switch a.cfg.Theme.Name {
	case "tokyo-night":
		a.colors.folder, a.colors.success, a.colors.danger = tcell.GetColor("#7dcfff"), tcell.GetColor("#9ece6a"), tcell.GetColor("#f7768e")
	case "dracula":
		a.colors.folder, a.colors.success, a.colors.danger = tcell.GetColor("#8be9fd"), tcell.GetColor("#50fa7b"), tcell.GetColor("#ff5555")
	case "nord":
		a.colors.folder, a.colors.success, a.colors.danger = tcell.GetColor("#81a1c1"), tcell.GetColor("#a3be8c"), tcell.GetColor("#bf616a")
	case "gruvbox-dark":
		a.colors.folder, a.colors.success, a.colors.danger = tcell.GetColor("#83a598"), tcell.GetColor("#b8bb26"), tcell.GetColor("#fb4934")
	}
	if lightBackground(bg) {
		a.colors.inset = mix(bg, fg, 4)
		a.colors.folder, a.colors.success, a.colors.danger = tcell.GetColor("#0969da"), tcell.GetColor("#1a7f37"), tcell.GetColor("#cf222e")
	}
	r, g, b := accent.RGB()
	if r*299+g*587+b*114 < 145000 {
		a.colors.onAccent = tcell.GetColor("#F5F7FA")
	}
	a.muted = a.base.Foreground(a.colors.muted)
	a.accent = a.base.Foreground(accent)
	a.chrome = a.base.Background(mix(bg, fg, 5))
	a.selected = a.base.Background(a.colors.selection)
	if a.usesTerminalPalette() {
		a.base = a.base.Foreground(tcell.ColorDefault).Background(tcell.ColorDefault)
		a.colors.muted, a.colors.disabled, a.colors.border = tcell.ColorGray, tcell.ColorGray, tcell.ColorGray
		a.colors.folder, a.colors.success, a.colors.danger = tcell.ColorBlue, tcell.ColorGreen, tcell.ColorRed
		a.colors.onAccent = tcell.ColorDefault
		a.muted = a.base.Foreground(a.colors.muted)
		a.accent = a.base.Foreground(tcell.ColorBlue)
		a.chrome = a.chrome.Foreground(tcell.ColorDefault)
		a.selected = a.selected.Foreground(tcell.ColorDefault)
		a.themeSyntax = "terminal-dark"
		if lightBackground(bg) {
			a.themeSyntax = "terminal-light"
		}
	}
}

type terminalColorSource interface {
	DefaultColors() terminal.DefaultColors
	RequestColors()
}

func (a *App) readTerminalColors() terminal.DefaultColors {
	colors := terminal.ColorsFromEnv()
	if tty, ok := a.screen.Tty(); ok {
		if source, ok := tty.(terminalColorSource); ok {
			actual := source.DefaultColors()
			if actual.Foreground.Valid() {
				colors.Foreground = actual.Foreground
			}
			if actual.Background.Valid() {
				colors.Background = actual.Background
			}
		}
	}
	return colors
}

func (a *App) requestTerminalColors() {
	if a.cfg.Theme.Name != "auto" && a.cfg.Theme.Name != "terminal-default" {
		return
	}
	if tty, ok := a.screen.Tty(); ok {
		if source, ok := tty.(terminalColorSource); ok {
			source.RequestColors()
		}
	}
}

func (a *App) refreshTerminalTheme() {
	if (a.cfg.Theme.Name == "auto" || a.cfg.Theme.Name == "terminal-default") && a.readTerminalColors() != a.terminalColors {
		a.applyTheme(a.cfg.Theme)
	}
}

func (a *App) usesTerminalPalette() bool {
	return a.cfg.Theme.Name == "terminal-default" || (a.cfg.Theme.Name == "auto" && !a.terminalColors.Background.Valid())
}

var ansiPalette = []tcell.Color{
	tcell.ColorBlack, tcell.ColorMaroon, tcell.ColorGreen, tcell.ColorOlive,
	tcell.ColorNavy, tcell.ColorPurple, tcell.ColorTeal, tcell.ColorSilver,
	tcell.ColorGray, tcell.ColorRed, tcell.ColorLime, tcell.ColorYellow,
	tcell.ColorBlue, tcell.ColorFuchsia, tcell.ColorAqua, tcell.ColorWhite,
}

// Route all workbench surfaces through this function so dialogs, selections,
// diff rows and gutters cannot accidentally paint a background in default mode.
// Child terminal cells bypass it: explicit shell/application colors stay intact.
func (a *App) workbenchStyle(style tcell.Style) tcell.Style {
	if !a.usesTerminalPalette() {
		return style
	}
	bg := style.GetBackground()
	if bg == a.colors.selection || bg == a.colors.hover {
		style = style.Reverse(true)
	}
	fg := style.GetForeground()
	if fg.IsRGB() {
		if a.themeColorCache == nil {
			a.themeColorCache = map[tcell.Color]tcell.Color{}
		}
		if cached, ok := a.themeColorCache[fg]; ok {
			fg = cached
		} else {
			converted := tcell.FindColor(fg, ansiPalette)
			a.themeColorCache[fg] = converted
			fg = converted
		}
	}
	return style.Background(tcell.ColorDefault).Foreground(fg)
}

func lightBackground(color tcell.Color) bool {
	r, g, b := color.RGB()
	return r*299+g*587+b*114 >= 145000
}

func (a *App) chooseTheme() {
	var choices []item
	selected := 0
	for _, preset := range config.ThemePresets() {
		label := preset.Label
		if a.cfg.Theme == preset.Theme {
			label += " · current"
			selected = len(choices)
		}
		swatch := [3]string{preset.Theme.Background, preset.Theme.Foreground, preset.Theme.Accent}
		if preset.Theme.Name == "auto" || preset.Theme.Name == "terminal-default" {
			swatch = [3]string{}
		}
		choices = append(choices, item{Label: label, Detail: preset.Description,
			Swatch: swatch, Action: func() {
				a.applyTheme(preset.Theme)
				if a.cfg.FilePath != "" {
					if err := config.SaveTheme(a.cfg.FilePath, preset.Theme.Name); err != nil {
						a.notify("Theme applied for this session; could not save: " + err.Error())
						return
					}
				}
				a.notify("Theme: " + preset.Label)
			}})
	}
	a.setModal(&modal{Title: "Color Theme", InputEnabled: true, Filter: true, Items: choices, Selected: selected,
		Placeholder: "Filter themes…", Hint: "↑/↓ choose · Enter applies and remembers · Esc cancels"})
}

func (a *App) applyTheme(theme config.Theme) {
	a.cfg.Theme = theme
	a.themeSeq++
	a.initTheme()
	a.requestTerminalColors()
	a.screen.SetStyle(a.base)
	a.highlight = map[*editor.Document]highlights{}
	a.highlightPending = map[*editor.Document]int{}
	for _, d := range a.docs {
		a.scheduleHighlight(d)
	}
	if c := a.comparison; c != nil {
		c.Highlights = [2][]tokenSpan{}
		a.rehighlightComparison(c)
	}
}

func (a *App) rehighlightComparison(c *gitComparison) {
	pair, base, syntax, seq := c.Pair, a.base, a.themeSyntax, a.themeSeq
	go func() {
		view := &gitComparison{Pair: pair}
		view.highlight(base, syntax)
		a.post(func() {
			if a.comparison == c && a.themeSeq == seq {
				c.Highlights = view.Highlights
			}
		})
	}()
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
