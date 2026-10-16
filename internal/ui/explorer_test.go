package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/vt"
)

func waitUpdates(t *testing.T, a *App, ready func() bool) {
	t.Helper()
	deadline := time.After(4 * time.Second)
	for !ready() {
		select {
		case update := <-a.updates:
			update()
		case <-deadline:
			t.Fatal("background update timed out")
		}
	}
}
func clickCommand(t *testing.T, a *App, id string) {
	t.Helper()
	a.draw()
	for _, b := range a.buttons {
		if b.Command == id {
			click(a, b.X+b.W/2, b.Y)
			return
		}
	}
	t.Fatal("button not visible:", id)
}

func TestExplorerFilterFindsCollapsedFilesAndRestoresTree(t *testing.T) {
	a, _ := testApp(t)
	dir := filepath.Join(a.root, "src", "deep")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "你好.txt")
	os.WriteFile(path, []byte("nested text"), 0644)
	os.WriteFile(filepath.Join(a.root, "notes.txt"), []byte("root"), 0644)
	a.scanTree()
	waitUpdates(t, a, func() bool { return len(a.treeAll) == 2 })
	a.draw()
	for _, e := range a.tree {
		if e.Path == path {
			t.Fatal("fixture directory is already expanded")
		}
	}
	click(a, a.treeFilterRect.X+1, a.treeFilterRect.Y)
	if a.focus != "filter" {
		t.Fatal("filter field cannot receive focus")
	}
	a.paste("你好")
	waitUpdates(t, a, func() bool { return a.treeIndexReady })
	if len(a.tree) != 1 || a.tree[0].Path != path || a.tree[0].Name != "src/deep/你好.txt" {
		t.Fatal("collapsed path not found", a.tree)
	}
	a.key(tcell.NewEventKey(tcell.KeyEnter, "", 0))
	if a.view().Doc.Buffer.String() != "nested text" || a.focus != "editor" {
		t.Fatal("Enter did not open filtered result")
	}
	clickCommand(t, a, "explorer.clearFilter")
	if a.treeQuery != "" || len(a.tree) != 2 || a.expanded[filepath.Join(a.root, "src")] {
		t.Fatal("clear failed to restore collapsed tree", a.tree)
	}
	a.setTreeQuery("NOTES.TXT")
	if len(a.tree) != 1 || a.tree[0].Name != "notes.txt" {
		t.Fatal("case-insensitive path filter", a.tree)
	}
	a.setTreeQuery("src/deep/你")
	if len(a.tree) != 1 {
		t.Fatal("path filter")
	}
	a.setTreeQuery("not-found-xyz")
	a.draw()
	if len(a.tree) != 0 {
		t.Fatal("unmatched filter showed files")
	}
}

func TestExplorerFilterKeyboardKeepsDocumentUntouched(t *testing.T) {
	a, _ := testApp(t)
	insertSource(a, "keep document")
	a.focus = "tree"
	a.key(tcell.NewEventKey(tcell.KeyRune, "/", 0))
	a.key(tcell.NewEventKey(tcell.KeyRune, "中👩‍💻é", 0))
	a.key(tcell.NewEventKey(tcell.KeyBackspace, "", 0))
	if a.treeQuery != "中👩‍💻" {
		t.Fatal("filter backspace split a grapheme", a.treeQuery)
	}
	a.key(tcell.NewEventKey(tcell.KeyLeft, "", 0))
	a.key(tcell.NewEventKey(tcell.KeyRune, "x", 0))
	if a.treeQuery != "中x👩‍💻" {
		t.Fatal("filter cursor insertion", a.treeQuery)
	}
	a.key(tcell.NewEventKey(tcell.KeyEsc, "", 0))
	if a.focus != "tree" || a.treeQuery != "" || a.view().Doc.Buffer.String() != "keep document" {
		t.Fatal("filter leaked into document")
	}
	a.key(tcell.NewEventKey(tcell.KeyCtrlF, "", 0))
	if a.focus != "filter" || a.modal != nil {
		t.Fatal("Explorer Ctrl+F opened file search")
	}
}

func TestPanelHideKeepsSessionAndKillEndsIt(t *testing.T) {
	a, _ := testApp(t)
	a.cfg.Shell = "/bin/sh"
	a.showTerminal()
	session := a.term
	if session == nil {
		t.Fatal("shell not started")
	}
	a.key(tcell.NewEventKey(tcell.KeyCtrlJ, "", 0))
	if a.panelVisible || a.focus != "editor" || a.term != session {
		t.Fatal("Ctrl+J was sent to the shell instead of hiding panel")
	}
	a.key(tcell.NewEventKey(tcell.KeyCtrlJ, "", 0))
	if !a.panelVisible || a.term != session {
		t.Fatal("toggle lost shell session")
	}
	clickCommand(t, a, "view.showTerminal")
	if a.focus != "terminal" || a.term != session {
		t.Fatal("terminal button restarted shell")
	}
	clickCommand(t, a, "view.hidePanel")
	if a.panelVisible || a.term != session {
		t.Fatal("Hide killed shell")
	}
	a.showTerminal()
	clickCommand(t, a, "terminal.kill")
	if a.term != nil || a.panelVisible || a.focus != "editor" {
		t.Fatal("Kill did not close session")
	}
	a.key(tcell.NewEventKey(tcell.KeyF4, "", 0))
	if a.term == nil || a.term == session {
		t.Fatal("F4 did not start a fresh shell")
	}
}

func TestToolbarClicksSaveUndoRedoFindAndSplit(t *testing.T) {
	a, _ := testApp(t)
	old := a.view().Doc
	clickCommand(t, a, "file.new")
	if a.view().Doc == old {
		t.Fatal("New button")
	}
	a.typeInput("draft")
	clickCommand(t, a, "file.save")
	if a.modal == nil || a.modal.Title != "Save As" {
		t.Fatal("Save button")
	}
	path := filepath.Join(a.root, "draft.txt")
	a.modal.Input = path
	a.modalAccept()
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "draft" {
		t.Fatal("toolbar save", string(data), err)
	}
	a.typeInput("!")
	clickCommand(t, a, "edit.undo")
	if a.view().Doc.Buffer.String() != "draft" {
		t.Fatal("Undo button")
	}
	clickCommand(t, a, "edit.redo")
	if a.view().Doc.Buffer.String() != "draft!" {
		t.Fatal("Redo button")
	}
	clickCommand(t, a, "search.find")
	if a.modal == nil || a.modal.Title != "Find in File" {
		t.Fatal("Find button")
	}
	a.key(tcell.NewEventKey(tcell.KeyEsc, "", 0))
	clickCommand(t, a, "view.split")
	if !a.split || a.views[0].Doc != a.views[1].Doc {
		t.Fatal("Split button")
	}
	clickCommand(t, a, "app.commands")
	if a.modal == nil || a.modal.Title != "Command Palette" {
		t.Fatal("More button")
	}
}

func TestButtonsStayWithinScreenAndAvoidOverlap(t *testing.T) {
	a, mt := testApp(t)
	a.panelVisible = true
	for _, size := range []vt.Coord{{X: 110, Y: 32}, {X: 80, Y: 24}, {X: 45, Y: 20}, {X: 24, Y: 10}, {X: 6, Y: 3}} {
		mt.SetSize(size)
		a.screen.Sync()
		a.draw()
		w, h := a.screen.Size()
		for i, b := range a.buttons {
			if b.X < 0 || b.Y < 0 || b.X+b.W > w || b.Y >= h {
				t.Fatal("button outside screen", size, b)
			}
			for _, other := range a.buttons[:i] {
				if b.Y == other.Y && b.X < other.X+other.W && b.X+b.W > other.X {
					t.Fatal("overlapping click targets", size, b, other)
				}
			}
		}
	}
}
