package ui

import (
	"strings"

	"github.com/gdamore/tcell/v3"
	"sunbook/internal/text"
)

func (a *App) focusGitFilter() {
	a.focus = "gitFilter"
	a.gitFilterCursor = len(a.gitFilter)
	// Keep the field accessible when the terminal is too short for its inline row.
	if a.sideRect.H < 18 {
		a.prompt("Filter Git Changes", a.gitFilter, func(s string) { a.setGitFilter(s); a.focus = "git" })
		a.focus = "git"
	}
}
func (a *App) setGitFilter(s string) {
	a.gitFilter = s
	a.gitFilterCursor = len(s)
	a.gitSelected = 0
	a.gitTop = 0
}
func (a *App) insertGitInput(s string) {
	s = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\t", " ").Replace(s)
	value, cursor := &a.gitMessage, &a.gitMessageCursor
	filter := a.focus == "gitFilter"
	if filter {
		value, cursor = &a.gitFilter, &a.gitFilterCursor
	}
	*value = (*value)[:*cursor] + s + (*value)[*cursor:]
	*cursor += len(s)
	if filter {
		a.gitSelected = 0
		a.gitTop = 0
	}
}
func (a *App) gitInputKey(e *tcell.EventKey, name string) {
	filter := a.focus == "gitFilter"
	value, cursor := &a.gitMessage, &a.gitMessageCursor
	if filter {
		value, cursor = &a.gitFilter, &a.gitFilterCursor
	}
	switch name {
	case "escape":
		if filter {
			a.setGitFilter("")
		}
		a.focus = "git"
	case "enter", "ctrl+enter":
		if filter {
			a.focus = "git"
			a.moveGitSelection(0)
		} else {
			a.commit()
		}
	case "down":
		a.focus = "git"
		a.moveGitSelection(0)
	case "tab":
		if filter {
			a.focus = "git"
		} else {
			a.focusGitFilter()
		}
	case "f5":
		a.refreshGit()
	case "ctrl+q":
		a.confirmExit()
	case "left":
		*cursor = text.Prev(*value, *cursor)
	case "right":
		*cursor = text.Next(*value, *cursor)
	case "home", "ctrl+a":
		*cursor = 0
	case "end", "ctrl+e":
		*cursor = len(*value)
	case "ctrl+u":
		*value = ""
		*cursor = 0
	case "ctrl+v":
		a.readClipboard()
	case "backspace", "backspace2":
		p := text.Prev(*value, *cursor)
		*value = (*value)[:p] + (*value)[*cursor:]
		*cursor = p
	case "delete":
		p := text.Next(*value, *cursor)
		*value = (*value)[:*cursor] + (*value)[p:]
	default:
		if e.Str() != "" && e.Modifiers()&(tcell.ModCtrl|tcell.ModAlt|tcell.ModMeta) == 0 {
			a.insertGitInput(e.Str())
		}
	}
	if filter && name != "up" && name != "down" {
		a.gitSelected = max(0, min(a.gitSelected, len(a.gitEntries())-1))
		a.gitTop = 0
	}
}
