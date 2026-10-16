package ui

import (
	"fmt"
	"path/filepath"

	"sunbook/internal/terminal"
)

// Session callbacks retain their tab, so background shells keep receiving output
// and a late exit from a closed shell cannot affect its replacement.
type terminalTab struct {
	id      int
	session *terminal.Session
	offset  int
	exited  bool
	closed  bool
}

func (a *App) newTerminal() {
	a.panelVisible, a.panel, a.focus = true, "Terminal", "terminal"
	a.layout()
	r := a.panelRect
	a.termSeq++
	tab := &terminalTab{id: a.termSeq}
	s, err := terminal.Start(a.cfg.Shell, a.root, max(1, r.W), max(1, r.H), func(data []byte) {
		a.post(func() {
			if !tab.closed {
				tab.session.Feed(data)
			}
		})
	}, func(err error) {
		a.post(func() {
			if tab.closed {
				return
			}
			tab.exited = true
			if a.terminalActive == tab {
				a.termExited = true
				if err != nil {
					a.notify("Shell exited: " + err.Error())
				} else {
					a.notify("Shell exited")
				}
			}
		})
	})
	if err != nil {
		tab.closed = true
		a.showError(err)
		a.focus = "editor"
		return
	}
	tab.session = s
	a.terminals = append(a.terminals, tab)
	a.selectTerminal(tab)
}

func (a *App) selectTerminal(tab *terminalTab) {
	if tab == nil || tab.closed {
		return
	}
	if a.terminalActive != nil {
		a.terminalActive.offset = a.termOffset
	}
	a.terminalActive = tab
	a.term, a.termOffset, a.termExited = tab.session, tab.offset, tab.exited
	a.panelVisible, a.panel, a.focus = true, "Terminal", "terminal"
}

func (a *App) removeTerminal(tab *terminalTab) {
	if tab == nil {
		return
	}
	tab.closed = true
	for i, candidate := range a.terminals {
		if candidate != tab {
			continue
		}
		a.terminals = append(a.terminals[:i], a.terminals[i+1:]...)
		if a.terminalActive == tab {
			a.terminalActive = nil
			a.term, a.termOffset, a.termExited = nil, 0, false
			if len(a.terminals) > 0 {
				a.selectTerminal(a.terminals[min(i, len(a.terminals)-1)])
			} else if a.panel == "Terminal" {
				a.hidePanel()
			}
		}
		break
	}
	tab.session.Close()
	a.notify(fmt.Sprintf("Terminal %d closed", tab.id))
}

func (a *App) closeTerminals() {
	for _, tab := range a.terminals {
		tab.closed = true
		tab.session.Close()
	}
	a.terminals, a.terminalActive = nil, nil
	a.term, a.termOffset, a.termExited = nil, 0, false
}

func (a *App) chooseTerminal() {
	if len(a.terminals) == 0 {
		a.newTerminal()
		return
	}
	var choices []item
	selected := 0
	for i, tab := range a.terminals {
		label := fmt.Sprintf("Terminal %d · %s", tab.id, filepath.Base(a.cfg.Shell))
		if tab == a.terminalActive {
			selected = i
			label += " · current"
		}
		state := "Running"
		if tab.exited {
			state = "Exited · output retained"
		}
		choices = append(choices, item{Label: label, Detail: state, Action: func() { a.selectTerminal(tab) }})
	}
	a.setModal(&modal{Title: "Switch Terminal", InputEnabled: true, Filter: true, Items: choices, Selected: selected,
		Placeholder: "Filter sessions…", Hint: "↑/↓ choose · Enter switches · Esc cancels"})
}

func (a *App) nextTerminal(dir int) {
	for i, tab := range a.terminals {
		if tab == a.terminalActive {
			a.selectTerminal(a.terminals[(i+len(a.terminals)+dir)%len(a.terminals)])
			return
		}
	}
	a.showTerminal()
}
