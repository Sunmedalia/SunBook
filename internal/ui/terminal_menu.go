package ui

import (
	"fmt"
	"path/filepath"
	"strings"
)

// The session list belongs to its header button, rather than the modal layer.
type terminalMenu struct {
	rect
	List          rect
	Selected, Top int
}

func (a *App) drawTerminalMenu() {
	m := a.terminalMenu
	if m == nil {
		return
	}
	var anchor rect
	for _, button := range a.buttons {
		if button.Command == "terminal.switch" {
			anchor = button.rect
			break
		}
	}
	w, h := a.screen.Size()
	y := anchor.Y + anchor.H
	rows := min(8, len(a.terminals), h-2-y-2)
	if !a.panelVisible || a.panel != "Terminal" || anchor.W == 0 || rows < 1 {
		a.terminalMenu = nil
		return
	}
	width := min(36, w-2)
	x := max(0, min(anchor.X, w-width-1))
	m.rect = rect{x, y, width, rows + 2}
	m.List = rect{x + 1, y + 1, width - 2, rows}
	m.Selected = max(0, min(m.Selected, len(a.terminals)-1))
	m.Top = max(0, min(m.Top, len(a.terminals)-rows))
	if m.Selected < m.Top {
		m.Top = m.Selected
	} else if m.Selected >= m.Top+rows {
		m.Top = m.Selected - rows + 1
	}
	a.screen.HideCursor()
	a.fill(m.rect, a.chrome)
	border := a.chrome.Foreground(a.colors.border)
	a.put(x, y, width, "╭"+strings.Repeat("─", width-2)+"╮", border)
	a.put(x, y+rows+1, width, "╰"+strings.Repeat("─", width-2)+"╯", border)
	if m.Top > 0 {
		a.put(x+width-3, y, 1, "↑", a.accent)
	}
	if m.Top+rows < len(a.terminals) {
		a.put(x+width-3, y+rows+1, 1, "↓", a.accent)
	}
	for row := 0; row < rows; row++ {
		idx := m.Top + row
		tab := a.terminals[idx]
		style := a.chrome
		if idx == m.Selected {
			style = a.selected
		}
		a.fill(rect{x + 1, y + 1 + row, width - 2, 1}, style)
		a.put(x, y+row+1, 1, "│", border)
		a.put(x+width-1, y+row+1, 1, "│", border)
		marker := " "
		if tab == a.terminalActive {
			marker = "✓"
		}
		labelWidth := width - 6
		if tab.exited {
			labelWidth -= 8
			a.put(x+width-9, y+row+1, 6, "exited", style.Foreground(a.colors.muted))
		}
		a.put(x+2, y+row+1, 1, marker, style.Foreground(a.accent.GetForeground()))
		a.put(x+4, y+row+1, labelWidth, fitLabel(fmt.Sprintf("%d · %s", tab.id, filepath.Base(a.cfg.Shell)), labelWidth), style)
	}
}

func (a *App) terminalMenuKey(name string) bool {
	m := a.terminalMenu
	if len(a.terminals) == 0 {
		a.terminalMenu = nil
		return false
	}
	switch name {
	case "escape":
		a.terminalMenu = nil
	case "enter":
		a.selectTerminal(a.terminals[max(0, min(m.Selected, len(a.terminals)-1))])
	case "up", "shift+tab":
		m.Selected = max(0, m.Selected-1)
	case "down", "tab":
		m.Selected = min(len(a.terminals)-1, m.Selected+1)
	case "home":
		m.Selected = 0
	case "end":
		m.Selected = len(a.terminals) - 1
	case "pageup":
		m.Selected = max(0, m.Selected-max(1, m.List.H))
	case "pagedown":
		m.Selected = min(len(a.terminals)-1, m.Selected+max(1, m.List.H))
	default:
		if name == "f1" || name == "f4" || name == "f6" || a.bindings[name] == "view.panel" || a.bindings[name] == "view.hidePanel" {
			a.terminalMenu = nil
			return false
		}
	}
	return true
}

func (a *App) terminalMenuMouse(x, y int, fresh bool, wheel int) {
	m := a.terminalMenu
	if wheel != 0 {
		m.Selected = max(0, min(len(a.terminals)-1, m.Selected+wheel))
		return
	}
	if m.List.contains(x, y) {
		idx := m.Top + y - m.List.Y
		if idx >= 0 && idx < len(a.terminals) {
			m.Selected = idx
			if fresh {
				a.selectTerminal(a.terminals[idx])
			}
		}
		return
	}
	if fresh && !m.contains(x, y) {
		a.terminalMenu = nil
	}
}
