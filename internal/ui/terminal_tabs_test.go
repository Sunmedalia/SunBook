package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/vt"
)

func TestTerminalSessionsKeepIndependentShellsOutputAndScrollback(t *testing.T) {
	a, mt := testApp(t)
	a.cfg.Shell = "/bin/sh"
	a.showTerminal()
	first := a.terminalActive
	first.session.Paste("SUNBOOK_TAB=first; printf 'FIRST_%s\\n' ready\n")
	waitUpdates(t, a, func() bool { return strings.Contains(first.session.VT.String(), "FIRST_ready") })
	clickCommand(t, a, "terminal.new")
	second := a.terminalActive
	if first == second || len(a.terminals) != 2 {
		t.Fatal("+ did not create an independent shell")
	}
	second.session.Paste("printf 'SECOND_%s\\n' \"${SUNBOOK_TAB-unset}\"\n")
	waitUpdates(t, a, func() bool { return strings.Contains(second.session.VT.String(), "SECOND_unset") })
	// Background shells must keep processing output while another is active.
	first.session.Paste("printf 'BACKGROUND_%s\\n' \"$SUNBOOK_TAB\"\n")
	waitUpdates(t, a, func() bool { return strings.Contains(first.session.VT.String(), "BACKGROUND_first") })
	clickCommand(t, a, "terminal.switch")
	if a.modal != nil || a.terminalMenu == nil {
		t.Fatal("session selector must open a dropdown, not a modal")
	}
	a.draw()
	menu := a.terminalMenu
	click(a, menu.List.X+4, menu.List.Y)
	if a.term != first.session || a.terminalActive != first || a.focus != "terminal" {
		t.Fatal("session selector restarted or failed to switch the shell")
	}
	first.session.Feed([]byte(strings.Repeat("history line\r\n", 60)))
	a.termOffset = 9
	a.nextTerminal(1)
	a.nextTerminal(-1)
	if a.termOffset != 9 {
		t.Fatal("switching sessions lost scroll position")
	}
	// Both the + button and session selector remain available in the narrow dock.
	a.panelDock, a.panelWidth = "right", 28
	mt.SetSize(vt.Coord{X: 140, Y: 32})
	a.screen.Sync()
	clickCommand(t, a, "terminal.switch")
	a.key(tcell.NewEventKey(tcell.KeyEsc, "", 0))
	clickCommand(t, a, "terminal.new")
	third := a.terminalActive
	clickCommand(t, a, "terminal.kill")
	if !third.closed || a.term != second.session || !a.panelVisible || len(a.terminals) != 2 {
		t.Fatal("killing one shell affected other sessions")
	}
	second.session.Paste("exit\n")
	waitUpdates(t, a, func() bool { return second.exited })
	a.selectTerminal(first)
	if a.termExited {
		t.Fatal("another shell's exit marked the running shell as exited")
	}
	a.closeTerminals()
	if !first.closed || !second.closed || a.term != nil || len(a.terminals) != 0 {
		t.Fatal("shutdown did not close every session")
	}
	// Late output and exit callbacks must not touch already-closed emulators.
	for len(a.updates) > 0 {
		(<-a.updates)()
	}
}

func TestTerminalDropdownAnchorsScrollsAndConsumesInput(t *testing.T) {
	a, mt := testApp(t)
	a.cfg.Shell = "/bin/sh"
	for range 3 {
		a.newTerminal()
	}
	first, last := a.terminals[0], a.terminalActive
	a.panelHeight = 3 // Only one dropdown row fits beneath this low panel header.
	clickCommand(t, a, "terminal.switch")
	a.draw()
	menu := a.terminalMenu
	if menu == nil || a.modal != nil || menu.List.H != 1 || menu.Top != 2 {
		t.Fatal("dropdown did not scroll the active session into view")
	}
	for _, button := range a.buttons {
		if button.Command == "terminal.switch" && menu.Y != button.Y+button.H {
			t.Fatal("dropdown is not directly below its button")
		}
	}
	a.paste("must not reach the shell")
	a.key(tcell.NewEventKey(tcell.KeyHome, "", 0))
	a.draw()
	if menu.Top != 0 {
		t.Fatal("keyboard navigation did not scroll the dropdown")
	}
	a.key(tcell.NewEventKey(tcell.KeyEnter, "", 0))
	if a.terminalActive != first || a.terminalMenu != nil {
		t.Fatal("Enter did not switch the session and dismiss the dropdown")
	}
	a.panelDock, a.panelWidth = "right", 28
	mt.SetSize(vt.Coord{X: 140, Y: 32})
	a.screen.Sync()
	clickCommand(t, a, "terminal.switch")
	a.draw()
	menu = a.terminalMenu
	if menu == nil || menu.X < 0 || menu.X+menu.W > 140 || menu.Y != a.panelRect.Y {
		t.Fatal("right-docked dropdown overflowed or lost its button anchor")
	}
	click(a, menu.List.X+4, menu.List.Y+2)
	if a.terminalActive != last {
		t.Fatal("mouse selection did not switch to the chosen terminal")
	}
	clickCommand(t, a, "terminal.switch")
	a.draw()
	click(a, a.rects[0].X+5, a.rects[0].Y+2)
	if a.terminalMenu != nil || a.modal != nil || a.terminalActive != last || a.focus != "terminal" {
		t.Fatal("outside click did not just dismiss the dropdown")
	}
}

func TestTerminalHeaderMovesInstructionsIntoHelp(t *testing.T) {
	a, _ := testApp(t)
	a.panelVisible = true
	a.draw()
	var header strings.Builder
	for x := a.panelRect.X; x < a.panelRect.X+a.panelRect.W; x++ {
		cell, _, _ := a.screen.Get(x, a.panelRect.Y-1)
		header.WriteString(cell)
	}
	if strings.Contains(header.String(), "F4") || strings.Contains(header.String(), "Ctrl+J") {
		t.Fatal("terminal header still shows shortcut instructions")
	}
	a.help()
	body := strings.Join(a.modal.Body, "\n")
	for _, instruction := range []string{"F4", "Ctrl+J", "Terminal +", "Session number / name"} {
		if !strings.Contains(body, instruction) {
			t.Fatal("Help is missing terminal instruction", instruction)
		}
	}
}
