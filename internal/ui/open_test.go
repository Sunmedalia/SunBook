package ui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/vt"
)

func writeOpenFixture(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}
func openInput(a *App, value string) {
	a.modal.Input = value
	a.modal.Cursor = len(value)
	a.filterModal()
}
func waitPicker(t *testing.T, a *App) {
	t.Helper()
	waitUpdates(t, a, func() bool { return a.modal != nil && !a.modal.Open.Loading })
}

func TestOpenBrowserNavigationCompletionAndUnicodeSplit(t *testing.T) {
	a, _ := testApp(t)
	path := filepath.Join(a.root, "nested", "中文.txt")
	writeOpenFixture(t, path, "first\n中👩‍💻ab\nlast")
	a.typeInput("keep unsaved")
	original := a.view()
	a.workspaceSearch()
	a.key(tcell.NewEventKey(tcell.KeyCtrlO, "", 0))
	waitPicker(t, a)
	m := a.modal
	if len(m.Visible) != 1 || !m.Visible[0].Dir {
		t.Fatal("directory not listed", m.Visible)
	}
	a.key(tcell.NewEventKey(tcell.KeyTab, "", 0))
	waitPicker(t, a)
	if m.Open.Dir != filepath.Dir(path) || len(m.Visible) != 1 {
		t.Fatal("Tab did not browse folder")
	}
	a.key(tcell.NewEventKey(tcell.KeyUp, "", tcell.ModAlt))
	waitPicker(t, a)
	if m.Open.Dir != a.root {
		t.Fatal("parent navigation failed")
	}
	openInput(a, "nested/中")
	waitPicker(t, a)
	a.key(tcell.NewEventKey(tcell.KeyTab, "", 0))
	if m.Input != "nested/中文.txt" {
		t.Fatal("completion lost parent", m.Input)
	}
	openInput(a, m.Input+":2:5")
	a.key(tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModCtrl))
	if a.modal != nil || !a.split || a.views[0] != original || a.view().Doc.Path != path {
		t.Fatal("split open lost editor")
	}
	if a.view().Head() != len("first\n中👩‍💻") || original.Doc.Buffer.String() != "keep unsaved" {
		t.Fatal("Unicode jump or unsaved buffer lost", a.view().Head())
	}
}
func TestOpenBrowserFailureKeepsPickerAndEditors(t *testing.T) {
	a, _ := testApp(t)
	path := filepath.Join(a.root, "binary.bin")
	writeOpenFixture(t, path, "a\x00b")
	a.typeInput("draft")
	original := a.view()
	a.fileOpen()
	waitPicker(t, a)
	m := a.modal
	a.key(tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModCtrl))
	if a.modal != m || !strings.Contains(m.Hint, "binary") || a.split || a.view() != original || original.Doc.Buffer.String() != "draft" {
		t.Fatal("failed open damaged state", m.Hint)
	}
	// Editing remains possible after an error; the picker can create an unsaved new file.
	openInput(a, "new-note.txt")
	a.key(tcell.NewEventKey(tcell.KeyTab, "", 0))
	if m.Input != "new-note.txt" {
		t.Fatal("new-file completion used display label", m.Input)
	}
	a.key(tcell.NewEventKey(tcell.KeyEnter, "", 0))
	if a.modal != nil || a.view().Doc.Path != filepath.Join(a.root, "new-note.txt") {
		t.Fatal("new file not opened")
	}
	if _, err := os.Stat(a.view().Doc.Path); !os.IsNotExist(err) {
		t.Fatal("picker wrote file before save", err)
	}
}
func TestQuickOpenRanksFilesAndReusesIndex(t *testing.T) {
	a, _ := testApp(t)
	exact := filepath.Join(a.root, "notes.txt")
	prefix := filepath.Join(a.root, "notes-more.txt")
	pathMatch := filepath.Join(a.root, "notes", "other.txt")
	for _, p := range []string{exact, prefix, pathMatch} {
		writeOpenFixture(t, p, "first\n中👩‍💻ab")
	}
	if err := a.open(pathMatch); err != nil {
		t.Fatal(err)
	}
	a.workspaceSearch()
	a.key(tcell.NewEventKey(tcell.KeyCtrlP, "", 0))
	waitPicker(t, a)
	if !a.treeIndexReady || a.modal.Visible[0].Path != pathMatch {
		t.Fatal("recent file not first")
	}
	seq := a.treeIndexSeq
	openInput(a, "notes.txt:2:5")
	if len(a.modal.Visible) == 0 || a.modal.Visible[0].Path != exact {
		t.Fatal("filename exact match not first", a.modal.Visible)
	}
	a.key(tcell.NewEventKey(tcell.KeyEnter, "", 0))
	if a.view().Doc.Path != exact || a.view().Head() != len("first\n中👩‍💻") {
		t.Fatal("quick open did not jump")
	}
	a.quickOpen()
	if a.treeIndexSeq != seq || a.modal.Open.Loading || a.modal.Visible[0].Path != exact {
		t.Fatal("quick open reindexed or lost recent file")
	}
	openInput(a, "notes")
	if a.modal.Visible[0].Path != exact || a.modal.Visible[1].Path != prefix {
		t.Fatal("path matches outranked filenames", a.modal.Visible)
	}
}
func TestOpenAbsolutePathAndNumericFilename(t *testing.T) {
	a, mt := testApp(t)
	outside := filepath.Join(t.TempDir(), "report:2")
	writeOpenFixture(t, outside, "start\nlast")
	outside, _ = filepath.EvalSymlinks(outside)
	a.fileOpen()
	waitPicker(t, a)
	openInput(a, outside)
	waitPicker(t, a)
	a.modalAccept()
	if a.modal != nil || a.view().Doc.Path != outside || a.view().Head() != 0 {
		t.Fatal("literal numeric filename treated as location")
	}
	a.fileOpen()
	waitPicker(t, a)
	// Empty and narrow windows still render and allow Escape.
	for _, size := range []vt.Coord{{X: 25, Y: 9}, {X: 6, Y: 3}, {X: 110, Y: 32}} {
		mt.SetSize(size)
		a.screen.Clear()
		a.draw()
	}
	a.key(tcell.NewEventKey(tcell.KeyEsc, "", 0))
	if a.modal != nil {
		t.Fatal("Escape did not close picker")
	}
}
func TestOpenDirectoryResultsCannotReplaceNewDialog(t *testing.T) {
	a, _ := testApp(t)
	writeOpenFixture(t, filepath.Join(a.root, "nested", "file.txt"), "text")
	a.fileOpen()
	old := a.modal
	a.palette()
	current := a.modal
	for len(a.updates) > 0 {
		(<-a.updates)()
	}
	if a.modal == old || a.modal != current || a.modal.Title != "Command Palette" {
		t.Fatal("stale directory read replaced modal")
	}
}

func TestQuickOpenEmptyIgnoredWorkspace(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep unavailable")
	}
	a, _ := testApp(t)
	writeOpenFixture(t, filepath.Join(a.root, ".ignore"), "*\n")
	writeOpenFixture(t, filepath.Join(a.root, "ignored.txt"), "hidden")
	files, err := workspaceFiles(context.Background(), a.root)
	if err != nil || len(files) != 0 {
		t.Fatal("empty index exposed ignored files", files, err)
	}
	a.quickOpen()
	waitPicker(t, a)
	if len(a.modal.Visible) != 0 || a.modal.Empty != "No matching files" {
		t.Fatal("empty picker did not finish loading")
	}
	a.modalAccept()
	if a.modal == nil {
		t.Fatal("empty picker closed on Enter")
	}
}
func TestQuickOpenNumericFilenameAndDeletedFile(t *testing.T) {
	a, _ := testApp(t)
	path := filepath.Join(a.root, "nested", "report:2")
	writeOpenFixture(t, path, "first\nlast")
	a.quickOpen()
	waitPicker(t, a)
	openInput(a, "report:2")
	a.modalAccept()
	if a.modal != nil || a.view().Doc.Path != path || a.view().Head() != 0 {
		t.Fatal("literal indexed filename treated as jump")
	}
	stale := filepath.Join(a.root, "stale.txt")
	writeOpenFixture(t, stale, "old")
	a.scanTree()
	a.quickOpen()
	waitPicker(t, a)
	openInput(a, "stale.txt")
	if err := os.Remove(stale); err != nil {
		t.Fatal(err)
	}
	original := a.view()
	m := a.modal
	a.modalAccept()
	if a.modal != m || a.view() != original || m.Hint == "" {
		t.Fatal("deleted file opened as new buffer")
	}
}

func TestOpenNavigationButtonsAndEditablePath(t *testing.T) {
	a, _ := testApp(t)
	dir := filepath.Join(a.root, "nested")
	file := filepath.Join(dir, "main.go")
	writeOpenFixture(t, file, "package main\n")
	writeOpenFixture(t, filepath.Join(dir, "readme.md"), "# Hello\n")
	original := a.view()
	a.fileOpen()
	waitPicker(t, a)
	a.draw()
	m := a.modal
	s := m.Open
	click(a, s.PathRect.X+1, s.PathRect.Y)
	a.key(tcell.NewEventKey(tcell.KeyCtrlU, "", 0))
	a.paste("nested")
	a.draw()
	click(a, s.GoRect.X+1, s.GoRect.Y)
	waitPicker(t, a)
	a.draw()
	if s.Dir != dir || s.Path != dir || s.EditingPath || len(m.Visible) != 2 {
		t.Fatal("Go did not browse typed path", s.Dir, s.Path, m.Hint)
	}
	// The file type mark has its own cells/color and leaves the selectable name intact.
	x := a.modalRect.X + 2
	mark, st, _ := a.screen.Get(x, m.ListY)
	want, _ := a.fileIcon(file, false, false)
	if mark != want || st.GetBackground() != a.selected.GetBackground() || m.Visible[0].Label != "main.go" {
		t.Fatal("typed file icon not rendered", mark, want, m.Visible)
	}
	click(a, s.UpRect.X+1, s.UpRect.Y)
	waitPicker(t, a)
	a.draw()
	if s.Dir != a.root || s.Path != a.root || a.view() != original {
		t.Fatal("Up changed document or failed to browse parent")
	}
	// Clicking the name field returns keyboard input to filtering.
	click(a, s.PathRect.X+1, s.PathRect.Y)
	click(a, m.InputRect.X, m.InputRect.Y)
	a.paste("nested")
	if s.EditingPath || m.Input != "nested" {
		t.Fatal("name field did not regain focus")
	}
	a.draw()
	click(a, a.modalRect.X+6, m.ListY)
	waitPicker(t, a)
	if s.Dir != dir {
		t.Fatal("folder row stopped responding after icon added")
	}
}
func TestOpenPathKeyboardRelativeNavigationAndErrorRecovery(t *testing.T) {
	a, _ := testApp(t)
	dir := filepath.Join(a.root, "中文 目录")
	file := filepath.Join(dir, "note.txt")
	writeOpenFixture(t, file, "text")
	a.fileOpen()
	waitPicker(t, a)
	m := a.modal
	s := m.Open
	// A path typed in the file field changes the displayed directory too.
	openInput(a, "中文 目录/")
	waitPicker(t, a)
	a.key(tcell.NewEventKey(tcell.KeyCtrlL, "", 0))
	a.paste("..")
	a.key(tcell.NewEventKey(tcell.KeyEnter, "", 0))
	waitPicker(t, a)
	if s.Dir != a.root || s.EditingPath {
		t.Fatal("relative path resolved against wrong folder", s.Dir)
	}
	a.key(tcell.NewEventKey(tcell.KeyCtrlL, "", 0))
	a.paste("中文 目录")
	a.key(tcell.NewEventKey(tcell.KeyEnter, "", 0))
	waitPicker(t, a)
	if s.Dir != dir || s.Path != dir {
		t.Fatal("Unicode folder path failed", m.Hint)
	}
	a.key(tcell.NewEventKey(tcell.KeyCtrlL, "", 0))
	a.paste("missing/folder")
	a.key(tcell.NewEventKey(tcell.KeyEnter, "", 0))
	if !s.EditingPath || s.Dir != dir || len(m.Visible) != 1 || m.Hint == "" {
		t.Fatal("bad path discarded last directory")
	}
	a.key(tcell.NewEventKey(tcell.KeyEsc, "", 0))
	if a.modal != m || s.EditingPath || s.Path != dir {
		t.Fatal("Escape did not cancel path draft")
	}
	a.key(tcell.NewEventKey(tcell.KeyEsc, "", 0))
	if a.modal != nil {
		t.Fatal("second Escape did not close dialog")
	}
}
func TestOpenPathDraftSurvivesDirectoryReadAndGraphemeEditing(t *testing.T) {
	a, mt := testApp(t)
	a.fileOpen()
	m := a.modal
	s := m.Open
	a.key(tcell.NewEventKey(tcell.KeyCtrlL, "", 0))
	a.paste("中👩‍💻é")
	a.key(tcell.NewEventKey(tcell.KeyBackspace, "", 0))
	waitPicker(t, a)
	if s.Path != "中👩‍💻" || s.PathCursor != len(s.Path) || !s.EditingPath {
		t.Fatal("directory completion replaced draft or split grapheme", s.Path)
	}
	for _, size := range []vt.Coord{{X: 80, Y: 24}, {X: 32, Y: 12}, {X: 6, Y: 3}, {X: 110, Y: 32}} {
		mt.SetSize(size)
		a.screen.Sync()
		a.draw()
		w, _ := a.screen.Size()
		for _, r := range []rect{s.UpRect, s.PathRect, s.GoRect} {
			if r.W > 0 && (r.X < 0 || r.X+r.W > w) {
				t.Fatal("navigation control outside dialog", r, size)
			}
		}
	}
	a.key(tcell.NewEventKey(tcell.KeyCtrlA, "", 0))
	a.paste(a.root)
	a.key(tcell.NewEventKey(tcell.KeyEnter, "", 0))
	waitPicker(t, a)
	if s.Path != a.root || s.EditingPath {
		t.Fatal("whole-path replacement failed")
	}
}
