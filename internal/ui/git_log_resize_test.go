package ui

import (
	"fmt"
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/vt"
	gitservice "sunbook/internal/git"
)

func TestGitLogDividerDragsAndKeepsScrollState(t *testing.T) {
	a, mt := testApp(t)
	a.sidebarView = "Git"
	a.focus = "git"
	a.git.Root = a.root
	for i := 0; i < 10; i++ {
		a.git.Files = append(a.git.Files, gitservice.File{Path: fmt.Sprintf("file%d.go", i), Index: " ", Worktree: "M"})
		a.gitLog = append(a.gitLog, gitservice.Revision{Hash: fmt.Sprintf("abcdef%d", i), Subject: "commit", Author: "author"})
	}
	a.gitLogLoaded = true
	a.gitTop, a.gitLogTop, a.gitSelected, a.gitLogSelected = 2, 3, 4, 5
	a.draw()
	initialY, initialHeight := a.gitLogHeader.Y, a.gitLogRect.H
	x := a.gitLogHeader.X + a.gitLogHeader.W/2
	drag := func(target int) {
		t.Helper()
		a.mouse(tcell.NewEventMouse(x, a.gitLogHeader.Y, tcell.Button1, 0))
		a.draw()
		a.mouse(tcell.NewEventMouse(x, target, tcell.Button1, 0))
		a.draw()
		a.mouse(tcell.NewEventMouse(x, target, 0, 0))
		a.draw()
	}
	drag(initialY - 5)
	if a.gitLogHeader.Y != initialY-5 || a.gitLogRect.H != initialHeight+5 {
		t.Fatal("dragging up did not enlarge log", a.gitLogHeader, a.gitLogRect)
	}
	if a.modal != nil || a.drag != "" || a.gitTop != 2 || a.gitLogTop != 3 || a.gitSelected != 4 || a.gitLogSelected != 5 {
		t.Fatal("divider drag opened a commit or changed list state")
	}
	drag(a.gitLogHeader.Y + 3)
	if a.gitLogHeader.Y != initialY-2 {
		t.Fatal("dragging down did not shrink log", a.gitLogHeader)
	}
	desiredHeight := a.gitLogHeight
	a.execute("git.toggleLog")
	a.draw()
	if a.gitChangesRect.H != 0 {
		t.Fatal("expand no longer fills sidebar body")
	}
	a.execute("git.toggleLog")
	a.draw()
	if a.gitLogHeight != desiredHeight || a.gitLogHeader.Y != initialY-2 {
		t.Fatal("restore lost chosen height")
	}
	a.sidebarView = "Explorer"
	a.draw()
	a.sidebarView = "Git"
	a.draw()
	if a.gitLogHeight != desiredHeight || a.gitLogHeader.Y != initialY-2 {
		t.Fatal("sidebar switch lost chosen height")
	}
	drag(-100)
	if a.gitChangesRect.H < 2 || a.gitLogHeader.Y < a.gitBodyStart() {
		t.Fatal("log dragged over controls or hid change list")
	}
	drag(10000)
	if a.gitLogRect.H < 2 || a.gitLogRect.Y+a.gitLogRect.H != a.sideRect.Y+a.sideRect.H {
		t.Fatal("log dragged below sidebar")
	}
	desiredHeight = a.gitLogHeight
	mt.SetSize(vt.Coord{X: 60, Y: 10})
	a.screen.Sync()
	a.draw()
	if a.gitLogHeader.Y < a.gitBodyStart() || a.gitLogRect.Y+a.gitLogRect.H > a.sideRect.Y+a.sideRect.H {
		t.Fatal("small terminal has invalid log bounds")
	}
	mt.SetSize(vt.Coord{X: 110, Y: 32})
	a.screen.Sync()
	a.draw()
	if a.gitLogHeight != desiredHeight || a.gitLogRect.H != desiredHeight-1 {
		t.Fatal("resize lost preferred log height")
	}
}
