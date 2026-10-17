package ui

import "sunbook/internal/config"

func (a *App) settings() {
	theme := a.cfg.Theme.Name
	if preset, ok := config.FindTheme(theme); ok {
		theme = preset.Label
	} else if theme == "" {
		theme = "Custom"
	}
	files := "Expressive · 🐍 🦀"
	if a.cfg.FileIcons == "classic" {
		files = "Classic · Py Rs Go"
	}
	buttons := "Nerd Font"
	if a.cfg.ActivityIcons == "unicode" {
		buttons = "Unicode"
	}
	a.setModal(&modal{Title: "Settings", Compact: true, Hint: "↑/↓ choose · Enter opens · Esc closes",
		Items: []item{
			{Label: "Theme…", Detail: theme, Action: a.chooseTheme},
			{Label: "File Icons…", Detail: files, Action: a.chooseFileIcons},
			{Label: "Button & Sidebar Icons…", Detail: buttons, Action: a.chooseActionIcons},
			{Label: "Plugins…", Detail: "Install, enable and inspect health", Action: func() { a.showSidebar("Plugins"); a.refreshPlugins() }},
		}})
}

func (a *App) chooseFileIcons() {
	var choices []item
	selected := 0
	for _, choice := range []struct{ name, label, detail string }{
		{"expressive", "Expressive  🐍 🦀 ☕ 🐹", "Language mascots"},
		{"classic", "Classic  Py Rs Jav Go", "Original text icons"},
	} {
		label := choice.label
		if a.cfg.FileIcons == choice.name {
			label += " · current"
			selected = len(choices)
		}
		choices = append(choices, item{Label: label, Detail: choice.detail, Action: func() {
			a.cfg.FileIcons = choice.name
			a.rememberIcons("File icons: " + choice.name)
		}})
	}
	a.setModal(&modal{Title: "File Icon Theme", Compact: true, Items: choices, Selected: selected,
		Hint: "↑/↓ choose · Enter applies and saves · Esc cancels"})
}

func (a *App) chooseActionIcons() {
	var choices []item
	selected := 0
	for _, choice := range []struct{ name, label, detail string }{
		{"nerd", "Nerd Font", "Original patched-font icons"},
		{"unicode", "Unicode", "Standard characters"},
	} {
		label := choice.label
		if a.cfg.ActivityIcons == choice.name {
			label += " · current"
			selected = len(choices)
		}
		choices = append(choices, item{Label: label, Detail: choice.detail, Action: func() {
			a.cfg.ActivityIcons = choice.name
			a.rememberIcons("Button and sidebar icons: " + choice.label)
		}})
	}
	a.setModal(&modal{Title: "Button & Sidebar Icons", Compact: true, Items: choices, Selected: selected,
		Hint: "↑/↓ choose · Enter applies and saves · Esc cancels"})
}

func (a *App) rememberIcons(message string) {
	a.hoverCommand, a.hoverHint = "", ""
	a.pressedCommand = ""
	a.fileClick = fileClick{}
	if a.cfg.FilePath != "" {
		if err := config.SaveIcons(a.cfg.FilePath, a.cfg.FileIcons, a.cfg.ActivityIcons); err != nil {
			a.notify("Icons applied for this session; could not save: " + err.Error())
			return
		}
	}
	a.notify(message)
}
