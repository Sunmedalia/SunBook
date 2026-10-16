package ui

import "sunbook/internal/text"

func (a *App) drawButton(x, y int, label, id, hint string, active bool) int {
	label = "[" + label + "]"
	width := text.Width(label, 4)
	style := a.chrome
	if active {
		style = a.selected
	}
	if a.hoverCommand == id {
		style = a.selected.Bold(true)
	}
	a.put(x, y, width, label, style)
	a.buttons = append(a.buttons, button{rect: rect{x, y, width, 1}, Command: id, Hint: hint})
	return width
}
func (a *App) drawToolbar() {
	w, _ := a.screen.Size()
	x := 1
	if w >= 60 {
		a.put(x, 0, 8, "SUNBOOK", a.accent.Bold(true))
		x = 10
	}
	moreX := w - 7
	a.drawButton(moreX, 0, "More", "app.commands", "All commands (F1)", false)
	actions := []struct {
		label, id, hint  string
		active, optional bool
	}{
		{"New", "file.new", "New file (Ctrl+N)", false, false},
		{"Open", "file.open", "Open file (Ctrl+O)", false, false},
		{"Save", "file.save", "Save file (Ctrl+S)", false, false},
		{"Find", "search.find", "Find in file (Ctrl+F)", false, false},
		{"Undo", "edit.undo", "Undo (Ctrl+Z)", false, true},
		{"Redo", "edit.redo", "Redo (Ctrl+Y)", false, true},
		{"Split", "view.split", "Toggle split editor (Ctrl+\\)", a.split, false},
		{"Terminal", "view.showTerminal", "Show terminal (F4 focuses; Ctrl+J hides panel)", a.panelVisible && a.panel == "Terminal", false},
	}
	for _, action := range actions {
		if action.optional && w < 100 {
			continue
		}
		width := text.Width(action.label, 4) + 2
		if x+width >= moreX {
			continue
		}
		x += a.drawButton(x, 0, action.label, action.id, action.hint, action.active) + 1
	}
}
