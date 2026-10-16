package ui

import (
	"github.com/gdamore/tcell/v3"
	"path/filepath"
	"testing"
)

func TestFileIconsKeepExplorerExpansionAndTabCloseWorking(t *testing.T) {
	a, _ := testApp(t)
	dir := filepath.Join(a.root, "src")
	file := filepath.Join(dir, "中文.go")
	writeOpenFixture(t, file, "package sample\n")
	a.scanTree()
	waitUpdates(t, a, func() bool { return len(a.treeAll) == 1 })
	a.draw()
	r := a.sideRect
	y := r.Y + 3
	s, _, _ := a.screen.Get(r.X+3, y)
	if s != "▰" {
		t.Fatal("folder icon absent", s)
	}
	click(a, r.X+7, y)
	waitUpdates(t, a, func() bool { return len(a.treeAll) == 2 })
	a.draw()
	s, _, _ = a.screen.Get(r.X+3, y)
	if s != "▱" {
		t.Fatal("expanded folder icon absent", s)
	}
	click(a, r.X+9, y+1)
	if a.view().Doc.Path != file {
		t.Fatal("icon changed Explorer click targets")
	}
	a.draw()
	tab := a.tabs[len(a.tabs)-1]
	s, _, _ = a.screen.Get(tab.X+2, tab.Y)
	if s != "G" {
		t.Fatal("file tab icon absent", s)
	}
	click(a, tab.X+tab.W-2, tab.Y)
	if a.view().Doc.Path == file || a.modal != nil {
		t.Fatal("icon displaced tab close target")
	}
}

func TestExplorerSelectionRemainsVisibleAfterOpeningFile(t *testing.T) {
	a, _ := testApp(t)
	first := filepath.Join(a.root, "first.go")
	second := filepath.Join(a.root, "second.py")
	writeOpenFixture(t, first, "package sample\n")
	writeOpenFixture(t, second, "pass\n")
	a.scanTree()
	waitUpdates(t, a, func() bool { return len(a.treeAll) == 2 })
	a.draw()
	r := a.sideRect
	firstY := r.Y + 3
	click(a, r.X+8, firstY)
	a.draw()
	assertMarker := func(y int, want bool) {
		t.Helper()
		s, st, _ := a.screen.Get(r.X, y)
		if (s == "▎") != want {
			t.Fatal("selection marker", y, s, want)
		}
		if want && (st.GetForeground() != a.accent.GetForeground() || st.GetBackground() == a.chrome.GetBackground()) {
			t.Fatal("selection is not visible after focus change")
		}
	}
	if a.focus != "editor" || a.view().Doc.Path != first {
		t.Fatal("file did not open")
	}
	assertMarker(firstY, true)
	assertMarker(firstY+1, false)
	click(a, r.X+8, firstY+1)
	a.draw()
	assertMarker(firstY, false)
	assertMarker(firstY+1, true)
	a.key(tcell.NewEventKey(tcell.KeyF6, "", 0))
	a.draw()
	assertMarker(firstY+1, true)
}
func TestOpenSelectionMarkerMovesWithoutObscuringLanguageIcons(t *testing.T) {
	a, _ := testApp(t)
	for _, name := range []string{"same.java", "same.kt", "same.swift", "same.cs", "same.rb", "same.php", "same.lua", "same.jsx", "same.tsx", "same.cpp"} {
		writeOpenFixture(t, filepath.Join(a.root, name), "source\n")
	}
	a.fileOpen()
	waitPicker(t, a)
	m := a.modal
	a.draw()
	readMark := func(y int) string {
		var value string
		for i := 0; i < fileIconWidth-1; i++ {
			s, _, _ := a.screen.Get(a.modalRect.X+2+i, y)
			value += s
		}
		return value
	}
	marks := map[string]bool{}
	for i := 0; i < len(m.Visible); i++ {
		mark := readMark(m.ListY + i)
		if mark == "" || marks[mark] {
			t.Fatal("different languages share the same visible mark", mark, m.Visible[i].Path)
		}
		marks[mark] = true
	}
	x := a.modalRect.X + 1
	s, _, _ := a.screen.Get(x, m.ListY)
	if s != "▎" {
		t.Fatal("Open selection marker missing")
	}
	a.key(tcell.NewEventKey(tcell.KeyDown, "", 0))
	a.draw()
	s, _, _ = a.screen.Get(x, m.ListY)
	if s == "▎" {
		t.Fatal("previous row remains marked")
	}
	s, st, _ := a.screen.Get(x, m.ListY+1)
	if s != "▎" || st.GetBackground() != a.selected.GetBackground() {
		t.Fatal("selection marker did not follow arrow key")
	}
	a.key(tcell.NewEventKey(tcell.KeyCtrlL, "", 0))
	a.draw()
	s, _, _ = a.screen.Get(x, m.ListY+1)
	if s != "▎" {
		t.Fatal("path editing hid selected file")
	}
}
