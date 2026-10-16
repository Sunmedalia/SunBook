package ui

import (
	"time"

	"sunbook/internal/text"
)

func (a *App) buttonEnabled(id string) bool {
	if a.comparing() && comparisonReadOnlyCommand(id) {
		return false
	}
	switch id {
	case "edit.undo":
		return a.view().Doc.Buffer.CanUndo()
	case "edit.redo":
		return a.view().Doc.Buffer.CanRedo()
	case "search.stop":
		return a.searchRunning || !a.searchDue.IsZero()
	case "search.run", "search.clear":
		return a.searchQuery != ""
	case "git.stage", "git.unstage", "git.selectedDiff", "git.open":
		entry, ok := a.selectedGitEntry()
		if !ok || a.gitBusy {
			return false
		}
		f := a.git.Files[entry.File]
		if id == "git.open" && (f.Worktree == "D" || f.Index == "D" && f.Worktree == " ") {
			return false
		}
		return id != "git.stage" && id != "git.unstage" || (id == "git.unstage") == entry.Staged
	case "git.commit":
		staged, _, conflicts := a.gitCounts()
		return !a.gitBusy && staged > 0 && conflicts == 0
	case "git.stageAll":
		_, changed, _ := a.gitCounts()
		return !a.gitBusy && changed > 0
	case "git.unstageAll":
		staged, _, _ := a.gitCounts()
		return !a.gitBusy && staged > 0
	case "git.stageCHANGES", "git.stageUNTRACKED", "git.stageCONFLICTS":
		if a.gitBusy {
			return false
		}
		for _, f := range a.git.Files {
			if f.Changed() && "git.stage"+gitGroup(f, false) == id {
				return true
			}
		}
		return false
	case "git.more":
		return a.git.Root != ""
	case "git.refresh":
		return !a.gitBusy
	}
	return true
}

func (a *App) drawButton(x, y int, label, id, hint string, active bool) int {
	width := text.Width(label, 4) + 2
	enabled := a.buttonEnabled(id)
	style := a.base.Background(a.colors.raised)
	primary := id == "file.save" || id == "git.commit" || id == "search.run"
	if primary {
		style = style.Background(a.accent.GetForeground()).Foreground(a.colors.onAccent).Bold(true)
	}
	if active {
		style = a.selected.Foreground(a.accent.GetForeground()).Bold(true)
	}
	if a.hoverCommand == id && enabled {
		style = style.Background(a.colors.hover).Bold(true)
		if primary {
			style = style.Background(mix(a.accent.GetForeground(), a.base.GetForeground(), 20))
		}
	}
	pressed := enabled && a.pressedCommand == id && time.Now().Before(a.pressedUntil)
	if pressed {
		style = a.selected.Foreground(a.accent.GetForeground()).Bold(true)
	}
	if !enabled {
		style = a.chrome.Foreground(a.colors.disabled)
		hint += " · unavailable"
	}
	a.fill(rect{x, y, width, 1}, style)
	a.put(x+1, y, width-2, label, style)
	// Thin opposing edges suggest a raised face without taking another row.
	if enabled {
		left, right := mix(style.GetBackground(), a.base.GetForeground(), 26), a.colors.shadow
		if pressed {
			left, right = right, left
		}
		a.put(x, y, 1, "▏", style.Foreground(left))
		a.put(x+width-1, y, 1, "▕", style.Foreground(right))
	}
	a.buttons = append(a.buttons, button{rect: rect{x, y, width, 1}, Command: id, Hint: hint, Disabled: !enabled})
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
