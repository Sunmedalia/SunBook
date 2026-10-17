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
	a.gitGroupFocus = ""
	a.gitTop = 0
}
func (a *App) insertGitInput(s string) {
	s = normalizeGitMessage(s)
	value, cursor := &a.gitMessage, &a.gitMessageCursor
	filter := a.focus == "gitFilter"
	if filter {
		s = strings.ReplaceAll(s, "\n", " ")
		value, cursor = &a.gitFilter, &a.gitFilterCursor
	}
	*cursor = text.Snap(*value, *cursor)
	*value = (*value)[:*cursor] + s + (*value)[*cursor:]
	*cursor += len(s)
	if filter {
		a.gitSelected = 0
		a.gitGroupFocus = ""
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
	case "up":
		if !filter {
			a.moveGitMessageCursor(-1)
		}
	case "down":
		if !filter {
			a.moveGitMessageCursor(1)
		} else {
			a.focus = "git"
			a.moveGitSelection(0)
		}
	case "ctrl+j", "shift+enter":
		if !filter {
			a.insertGitInput("\n")
		}
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
	case "ctrl+a", "ctrl+home":
		*cursor = 0
	case "ctrl+e", "ctrl+end":
		*cursor = len(*value)
	case "home", "end":
		if filter {
			if name == "home" {
				*cursor = 0
			} else {
				*cursor = len(*value)
			}
		} else {
			lines := gitMessageLines(*value, a.gitMessageRect.W)
			line := lines[gitMessageRow(lines, *cursor)]
			if name == "home" {
				*cursor = line.Start
			} else {
				*cursor = line.End
			}
		}
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
		a.gitGroupFocus = ""
		a.gitSelected = max(0, min(a.gitSelected, len(a.gitEntries())-1))
		a.gitTop = 0
	}
}
