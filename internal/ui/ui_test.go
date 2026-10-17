package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/vt"
	"sunbook/internal/config"
	"sunbook/internal/editor"
	"sunbook/internal/lsp"
	"sunbook/internal/text"
)

func testApp(t testing.TB) (*App, vt.MockTerm) {
	t.Helper()
	mt := vt.NewMockTerm(vt.MockOptSize{X: 110, Y: 32})
	screen, e := tcell.NewTerminfoScreenFromTty(mt)
	if e != nil {
		t.Fatal(e)
	}
	if e = screen.Init(); e != nil {
		t.Fatal(e)
	}
	a, e := New(screen, config.Default(), t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { a.Close(); screen.Fini() })
	a.draw()
	return a, mt
}
func insertSource(a *App, s string) {
	a.view().Doc.Buffer = text.New(s)
	a.view().Selections = []text.Selection{{}}
	a.draw()
}
func click(a *App, x, y int) {
	a.mouse(tcell.NewEventMouse(x, y, tcell.Button1, tcell.ModNone))
	a.mouse(tcell.NewEventMouse(x, y, 0, tcell.ModNone))
}
func TestUnicodeMouseAndScrolling(t *testing.T) {
	a, _ := testApp(t)
	insertSource(a, "first\nsecond\n中👩‍💻ab\nlast")
	a.view().Top = 2
	a.draw()
	r := a.rects[0]
	x := r.X + a.gutter(a.view())
	click(a, x+2, r.Y)
	if a.view().Head() != len("first\nsecond\n中") {
		t.Fatal("wide click", a.view().Head())
	}
	a.clickTime = time.Time{}
	a.mouse(tcell.NewEventMouse(x, r.Y, tcell.Button1, 0))
	a.mouse(tcell.NewEventMouse(x+4, r.Y, tcell.Button1, 0))
	a.mouse(tcell.NewEventMouse(x+4, r.Y, 0, 0))
	lo, hi := a.view().Selections[0].Range()
	if a.view().Doc.Buffer.String()[lo:hi] != "中👩‍💻" {
		t.Fatal("drag split grapheme", lo, hi)
	}
	a.mouse(tcell.NewEventMouse(x, r.Y, tcell.WheelDown, 0))
	if a.view().Top != 3 {
		t.Fatal("wheel", a.view().Top)
	}
}
func TestSplitSyncUndoAndHiddenResize(t *testing.T) {
	a, mt := testApp(t)
	insertSource(a, "hello\nworld")
	d := a.view().Doc
	a.splitEditor()
	a.view().Set(7, false)
	other := a.view()
	a.active = 0
	a.view().Set(0, false)
	a.typeInput("中")
	if other.Doc != d || other.Head() != 10 || other.Doc.Buffer.String() != "中hello\nworld" {
		t.Fatal("split sync", other.Head())
	}
	a.execute("edit.undo")
	if other.Head() != 7 || d.Buffer.String() != "hello\nworld" {
		t.Fatal("undo sync", other.Head())
	}
	for _, size := range []vt.Coord{{X: 25, Y: 9}, {X: 6, Y: 3}, {X: 100, Y: 35}} {
		mt.SetSize(size)
		a.screen.Clear()
		a.draw()
	}
	if a.active != 0 {
		t.Fatal("hidden editor kept focus")
	}
}
func TestCommandsAndDirtyClose(t *testing.T) {
	a, _ := testApp(t)
	a.typeInput("unsaved")
	d := a.view().Doc
	a.execute("file.close")
	if a.modal == nil || len(a.modal.Visible) != 3 {
		t.Fatal("missing save prompt")
	}
	a.modal.Selected = 2
	a.modalAccept()
	if a.view().Doc != d || d.Buffer.String() != "unsaved" {
		t.Fatal("cancel lost text")
	}
	a.execute("file.save")
	if a.modal == nil || a.modal.Title != "Save As" {
		t.Fatal("no Save As")
	}
	path := filepath.Join(a.root, "saved.txt")
	a.modal.Input = path
	a.modal.Cursor = len(path)
	a.modalAccept()
	got, e := os.ReadFile(path)
	if e != nil || string(got) != "unsaved" || d.Dirty() {
		t.Fatal("save", string(got), e)
	}
	a.palette()
	a.modalInsert("split")
	if len(a.modal.Visible) != 1 || !strings.Contains(a.modal.Visible[0].Label, "Split") {
		t.Fatal(a.modal.Visible)
	}
	a.modalAccept()
	if !a.split {
		t.Fatal("command not executed")
	}
}
func TestEditingOperationsAndCompletion(t *testing.T) {
	a, _ := testApp(t)
	insertSource(a, "one one")
	a.execute("edit.selectNext")
	a.execute("edit.selectNext")
	a.typeInput("中")
	if a.view().Doc.Buffer.String() != "中 中" {
		t.Fatal(a.view().Doc.Buffer.String())
	}
	a.execute("edit.undo")
	if a.view().Doc.Buffer.String() != "one one" {
		t.Fatal("multi undo")
	}
	a.view().Selections = []text.Selection{{Anchor: 0, Head: 7}}
	a.indent(false)
	if a.view().Doc.Buffer.String() != "    one one" {
		t.Fatal("indent")
	}
	a.indent(true)
	if a.view().Doc.Buffer.String() != "one one" {
		t.Fatal("outdent")
	}
	insertSource(a, "fmt.")
	a.view().Set(4, false)
	d := a.view().Doc
	a.completion = &completionState{Doc: d, Version: d.Buffer.Version, Head: 4, Items: []lsp.Completion{{Label: "Println"}}}
	a.acceptCompletion()
	if d.Buffer.String() != "fmt.Println" {
		t.Fatal("completion ate dot", d.Buffer.String())
	}
	insertSource(a, "{")
	a.view().Set(1, false)
	a.typeInput("\n")
	if a.view().Doc.Buffer.String() != "{\n    " {
		t.Fatal("autoindent", a.view().Doc.Buffer.String())
	}
}
func TestPanelHeaderMouseAndPreview(t *testing.T) {
	a, _ := testApp(t)
	a.cfg.Shell = "/bin/sh"
	a.panelVisible = true
	a.panel = "Problems"
	a.draw()
	r := a.panelRect
	click(a, r.X+16, r.Y-1)
	if a.panel != "Terminal" || a.drag != "" {
		t.Fatal("panel click interpreted as drag", a.panel, a.drag)
	}
	a.choose("Preview", []item{{Label: "Apply"}, {Label: "Cancel"}})
	for range 20 {
		a.modal.Body = append(a.modal.Body, "edit")
	}
	a.draw()
	a.modalKey(tcell.NewEventKey(tcell.KeyDown, "", tcell.ModCtrl), "ctrl+down")
	if a.modal.BodyTop != 1 {
		t.Fatal("preview won't scroll")
	}
	a.draw()
}
func TestKeyProtocols(t *testing.T) {
	if pasteText(tcell.NewEventKey(tcell.KeyCtrlJ, "", tcell.ModCtrl)) != "\n" || pasteText(tcell.NewEventKeyEx(tcell.KeyRune, "j", tcell.ModCtrl, true, tcell.KeyJ, 1)) != "\n" {
		t.Fatal("paste dropped LF")
	}
	for _, test := range []struct {
		e    *tcell.EventKey
		want string
	}{{tcell.NewEventKey(tcell.KeyCtrlS, "", 0), "ctrl+s"}, {tcell.NewEventKeyEx(tcell.KeyRune, "s", tcell.ModCtrl, true, tcell.KeyS, 1), "ctrl+s"}, {tcell.NewEventKeyEx(tcell.KeyRune, "P", tcell.ModCtrl|tcell.ModShift, true, tcell.KeyP, 1), "ctrl+shift+p"}, {tcell.NewEventKey(tcell.KeyBacktab, "", 0), "shift+tab"}} {
		if got := keyName(test.e); got != test.want {
			t.Fatalf("%s want %s", got, test.want)
		}
	}
	key := terminalKey(tcell.NewEventKey(tcell.KeyCtrlC, "", 0))
	if key.Code != 'c' || key.Text != "" {
		t.Fatal("terminal Ctrl+C encoding", key)
	}
}
func TestDiskCheckDoesNotRaceWithEditing(t *testing.T) {
	a, _ := testApp(t)
	p := filepath.Join(a.root, "file.txt")
	os.WriteFile(p, []byte("old"), 0644)
	if e := a.open(p); e != nil {
		t.Fatal(e)
	}
	d := a.view().Doc
	os.WriteFile(p, []byte("external"), 0644)
	a.checkFiles()
	a.typeInput("keep ")
	deadline := time.After(2 * time.Second)
	for a.fileCheck {
		select {
		case f := <-a.updates:
			f()
		case <-deadline:
			t.Fatal("file check timeout")
		}
	}
	if d.Buffer.String() != "keep old" {
		t.Fatal("overwrote dirty buffer", d.Buffer.String())
	}
}
func TestRejectStaleRenameAndShowEdits(t *testing.T) {
	a, _ := testApp(t)
	p := filepath.Join(a.root, "file.txt")
	os.WriteFile(p, []byte("old"), 0644)
	a.open(p)
	ws := lsp.WorkspaceEdit{Changes: map[string][]lsp.TextEdit{lsp.URI(p): {{Range: lsp.Range{Start: lsp.Position{Line: 0, Character: 0}, End: lsp.Position{Line: 0, Character: 3}}, NewText: "new"}}}}
	a.previewWorkspaceEdit(ws)
	if a.modal == nil || len(a.modal.Body) < 2 {
		t.Fatal("missing edit preview")
	}
	a.view().Insert("x")
	a.modalAccept()
	if a.view().Doc.Buffer.String() != "xold" {
		t.Fatal("stale rename applied")
	}
}
func TestClampRetainsGraphemeBoundary(t *testing.T) {
	d := editor.New()
	d.Buffer = text.New("中👩‍💻")
	v := editor.NewView(d)
	v.Selections = []text.Selection{{Anchor: 1, Head: 6}}
	v.Clamp()
	if v.Selections[0].Anchor != 0 || v.Selections[0].Head != 3 {
		t.Fatal(v.Selections)
	}
}

func TestConfiguredBindingsAndHighlightCoalescing(t *testing.T) {
	a, _ := testApp(t)
	a.bindings["f1"] = "file.new"
	old := a.view().Doc
	a.key(tcell.NewEventKey(tcell.KeyF1, "", 0))
	if a.modal == nil || a.modal.Title != "New File" {
		t.Fatal("function key override ignored")
	}
	a.modalAccept()
	if a.view().Doc == old {
		t.Fatal("new-file command did not create an untitled document for empty input")
	}
	a.bindings["ctrl+e ctrl+x"] = "view.split"
	a.key(tcell.NewEventKey(tcell.KeyCtrlE, "", 0))
	a.key(tcell.NewEventKey(tcell.KeyCtrlX, "", 0))
	if !a.split {
		t.Fatal("custom chord ignored")
	}
	d := a.view().Doc
	for range 20 {
		a.typeInput("x")
	}
	if len(a.highlightRunning) != 0 {
		t.Fatal("unbounded highlight workers")
	}
	a.highlightDue[d] = time.Time{}
	a.flushHighlights()
	deadline := time.After(3 * time.Second)
	for a.highlight[d].Version != d.Buffer.Version {
		select {
		case f := <-a.updates:
			f()
		case <-deadline:
			t.Fatal("no highlight result")
		}
	}
}
