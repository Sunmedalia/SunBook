package ui

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/vt"
	"sunbook/internal/editor"
	gitservice "sunbook/internal/git"
)

func layoutDocs(t *testing.T, a *App) []*editor.Document {
	t.Helper()
	a.docs[0].Path = filepath.Join(a.root, "a.go")
	a.typeInput("draft A")
	docs := []*editor.Document{a.docs[0]}
	for _, name := range []string{"b.py", "c.ts"} {
		d := editor.New()
		d.Path = filepath.Join(a.root, name)
		a.addDocument(d)
		a.typeInput("draft " + name)
		docs = append(docs, d)
	}
	a.draw()
	return docs
}
func tabPoint(t *testing.T, a *App, doc *editor.Document) (int, int) {
	t.Helper()
	a.draw()
	for _, tab := range a.tabs {
		if tab.Doc == doc {
			return tab.X + 2, tab.Y
		}
	}
	t.Fatal("file tab not visible", doc.Name())
	return 0, 0
}
func dragTo(a *App, x, y int) {
	a.mouse(tcell.NewEventMouse(x, y, tcell.Button1, 0))
	a.draw()
	a.mouse(tcell.NewEventMouse(x, y, 0, 0))
	a.draw()
}
func TestTabDragReordersWithoutLosingBuffersOrSelection(t *testing.T) {
	a, _ := testApp(t)
	docs := layoutDocs(t, a)
	head := a.view().Head()
	x, y := tabPoint(t, a, docs[2])
	target, _ := tabPoint(t, a, docs[0])
	a.mouse(tcell.NewEventMouse(x, y, tcell.Button1, 0))
	a.draw()
	a.mouse(tcell.NewEventMouse(target, 1, tcell.Button1, 0))
	a.draw()
	if a.tabDrag == nil || !a.tabDrag.Moving || a.tabDrag.Slot != 0 {
		t.Fatal("tab reorder target not shown")
	}
	a.mouse(tcell.NewEventMouse(target, 1, 0, 0))
	a.draw()
	if a.tabDrag != nil || a.docs[0] != docs[2] || a.docs[1] != docs[0] || a.docs[2] != docs[1] || a.view().Head() != head {
		t.Fatal("tab reorder changed editor state")
	}
	for _, d := range docs {
		if !d.Dirty() || d.Buffer.Len() == 0 {
			t.Fatal("drag lost draft", d.Path)
		}
	}
	// Dragging back to the end shifts the intervening tabs in the other direction.
	x, y = tabPoint(t, a, docs[2])
	a.mouse(tcell.NewEventMouse(x, y, tcell.Button1, 0))
	a.draw()
	dragTo(a, a.editorArea.X+a.editorArea.W-2, 1)
	if a.docs[0] != docs[0] || a.docs[1] != docs[1] || a.docs[2] != docs[2] {
		t.Fatal("drag to end corrupted tab order")
	}
	// Escape cancels a second drag without applying its insertion target.
	x, y = tabPoint(t, a, docs[2])
	target, _ = tabPoint(t, a, docs[0])
	a.mouse(tcell.NewEventMouse(x, y, tcell.Button1, 0))
	a.mouse(tcell.NewEventMouse(target, 1, tcell.Button1, 0))
	a.key(tcell.NewEventKey(tcell.KeyEsc, "", 0))
	a.mouse(tcell.NewEventMouse(target, 1, 0, 0))
	if a.docs[0] != docs[0] || a.docs[2] != docs[2] || a.tabDrag != nil {
		t.Fatal("Escape did not cancel tab drag")
	}

}
func TestTabDropCreatesSplitAndOpensInExistingGroup(t *testing.T) {
	a, _ := testApp(t)
	docs := layoutDocs(t, a)
	a.activate(docs[0])
	left := a.view()
	head := left.Head()
	x, y := tabPoint(t, a, docs[2])
	r := a.rects[0]
	a.mouse(tcell.NewEventMouse(x, y, tcell.Button1, 0))
	a.draw()
	dragTo(a, r.X+r.W-2, r.Y+2)
	if !a.split || a.active != 1 || a.views[1].Doc != docs[2] || a.views[0] != left || left.Head() != head {
		t.Fatal("edge drop did not preserve original editor")
	}
	x, y = tabPoint(t, a, docs[1])
	r = a.rects[0]
	a.mouse(tcell.NewEventMouse(x, y, tcell.Button1, 0))
	a.draw()
	dragTo(a, r.X+r.W/2, r.Y+2)
	if a.active != 0 || a.view().Doc != docs[1] || a.views[1].Doc != docs[2] || len(a.docs) != 3 {
		t.Fatal("drop into existing group damaged split")
	}
}
func TestTerminalDockResizesWithoutRestartAndKeepsComparisonInsideEditor(t *testing.T) {
	a, mt := testApp(t)
	a.cfg.Shell = "/bin/sh"
	a.showTerminal()
	a.draw()
	session := a.term
	if session == nil {
		t.Fatal("shell did not start")
	}
	a.term.Feed([]byte("remember shell output\r\n"))
	clickCommand(t, a, "view.panelDock")
	a.draw()
	if a.panelDock != "right" || a.panelRect.Y != 2 || a.panelDivider.W != 1 || a.term != session {
		t.Fatal("dock did not preserve shell")
	}
	if a.editorArea.X+a.editorArea.W >= a.panelRect.X || session.VT.Width() != a.panelRect.W || session.VT.Height() != a.panelRect.H {
		t.Fatal("right terminal overlaps editor or wrong PTY size")
	}
	for _, tab := range a.tabs {
		if tab.X+tab.W > a.panelRect.X {
			t.Fatal("tab overlaps right terminal")
		}
	}
	// Right divider is independently draggable and leaves both editor groups usable.
	x, y := a.panelDivider.X, a.panelDivider.Y+5
	oldW := a.panelRect.W
	a.mouse(tcell.NewEventMouse(x, y, tcell.Button1, 0))
	a.mouse(tcell.NewEventMouse(x-4, y, tcell.Button1, 0))
	a.mouse(tcell.NewEventMouse(x-4, y, 0, 0))
	a.draw()
	if a.panelRect.W <= oldW || a.term != session {
		t.Fatal("right divider did not resize existing terminal")
	}
	a.comparison = &gitComparison{Active: true, Ratio: .5, Pair: gitservice.Comparison{Path: "file.go", OldLabel: "HEAD", NewLabel: "WORKTREE"}}
	a.draw()
	c := a.comparison
	if c.Rects[1].X+c.Rects[1].W > a.panelDivider.X {
		t.Fatal("comparison covered docked terminal")
	}
	a.closeComparison()
	// Small windows temporarily fall back to bottom, preserving the preferred dock.
	mt.SetSize(vt.Coord{X: 75, Y: 24})
	a.screen.Sync()
	a.draw()
	if a.panelDock != "right" || a.panelDivider.H != 1 || a.term != session {
		t.Fatal("small window lost dock preference or shell")
	}
	mt.SetSize(vt.Coord{X: 110, Y: 32})
	a.screen.Sync()
	a.draw()
	if a.panelDivider.W != 1 {
		t.Fatal("right dock not restored after widening")
	}
	clickCommand(t, a, "view.panelDock")
	a.draw()
	if a.panelDock != "bottom" || a.panelDivider.H != 1 || a.term != session {
		t.Fatal("bottom switch restarted shell")
	}
	a.hidePanel()
	a.showTerminal()
	a.draw()
	if a.term != session {
		t.Fatal("dock switch broke Hide/Show session retention")
	}
}
func TestSidebarAndDockButtonsStayWithinBounds(t *testing.T) {
	a, mt := testApp(t)
	a.panelVisible = true
	a.panel = "Problems"
	a.panelDock = "right"
	a.git.Root = a.root
	for i := 0; i < 24; i++ {
		a.git.Files = append(a.git.Files, gitservice.File{Path: fmt.Sprintf("src/file%d.go", i), Index: " ", Worktree: "M"})
	}
	for _, size := range []vt.Coord{{X: 140, Y: 40}, {X: 110, Y: 32}, {X: 80, Y: 24}, {X: 60, Y: 14}, {X: 45, Y: 10}, {X: 24, Y: 7}} {
		mt.SetSize(size)
		a.screen.Sync()
		for _, view := range []string{"Explorer", "Search", "Git"} {
			a.sidebarView = view
			a.draw()
			w, h := a.screen.Size()
			for i, b := range a.buttons {
				if b.X < 0 || b.Y < 0 || b.X+b.W > w || b.Y >= h {
					t.Fatal("control outside screen", view, size, b)
				}
				for _, other := range a.buttons[:i] {
					if b.Y == other.Y && b.X < other.X+other.W && b.X+b.W > other.X {
						t.Fatal("overlapping controls", view, size, b, other)
					}
				}
			}
		}
	}
}
