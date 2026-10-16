package ui

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/gdamore/tcell/v3"
	"sunbook/internal/editor"
	"sunbook/internal/lsp"
	"sunbook/internal/text"
)

func previewFixtures(t *testing.T, a *App) []string {
	t.Helper()
	var paths []string
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		path := filepath.Join(a.root, name)
		writeOpenFixture(t, path, "中 needle 👩‍💻\n"+name)
		paths = append(paths, path)
		a.tree = append(a.tree, treeEntry{Path: path, Name: name})
	}
	a.draw()
	return paths
}

func clickTreeFile(a *App, index int) {
	a.draw()
	click(a, a.sideRect.X+8, a.sideRect.Y+3+index)
}

func hasDocument(a *App, d *editor.Document) bool {
	for _, open := range a.docs {
		if open == d {
			return true
		}
	}
	return false
}

func TestExplorerClickReusesPreviewAndDoubleClickKeepsIt(t *testing.T) {
	a, _ := testApp(t)
	paths := previewFixtures(t, a)
	clickTreeFile(a, 0)
	first := a.view().Doc
	if first.Path != paths[0] || a.preview != first || len(a.docs) != 2 {
		t.Fatal("single click did not create a preview")
	}
	a.draw()
	tab := a.tabs[1]
	_, style, _ := a.screen.Get(tab.X+2+a.fileIconSpan(first.Path, false, false), tab.Y)
	if !style.HasItalic() {
		t.Fatal("preview filename is not italic")
	}
	a.highlight[first] = highlights{Version: first.Buffer.Version}
	a.diagnostics[lsp.URI(first.Path)] = lsp.Diagnostics{}
	clickTreeFile(a, 1)
	second := a.view().Doc
	if second.Path != paths[1] || a.preview != second || len(a.docs) != 2 || a.docs[1] != second || hasDocument(a, first) {
		t.Fatal("preview did not reuse its tab slot")
	}
	if a.viewCache[0][first] != nil || a.highlightPending[first] != 0 {
		t.Fatal("replaced preview retained view or highlight work")
	}
	if _, ok := a.highlight[first]; ok {
		t.Fatal("replaced preview retained highlight")
	}
	if _, ok := a.diagnostics[lsp.URI(first.Path)]; ok {
		t.Fatal("replaced preview retained diagnostics")
	}
	clickTreeFile(a, 1)
	if a.preview != nil || a.view().Doc != second || len(a.docs) != 2 {
		t.Fatal("double click did not keep the preview")
	}
	a.draw()
	tab = a.tabs[1]
	_, style, _ = a.screen.Get(tab.X+2+a.fileIconSpan(second.Path, false, false), tab.Y)
	if style.HasItalic() {
		t.Fatal("kept tab still looks like a preview")
	}
	clickTreeFile(a, 2)
	third := a.preview
	clickTreeFile(a, 1) // Selecting an already open file must not demote it.
	if a.view().Doc != second || a.preview != third || len(a.docs) != 3 {
		t.Fatal("clicking a permanent file changed its status")
	}
	clickTreeFile(a, 0)
	if a.preview.Path != paths[0] || hasDocument(a, third) || !hasDocument(a, second) || len(a.docs) != 3 {
		t.Fatal("browsing previews closed a permanent file")
	}
}

func TestPreviewKeepsOpenOnUserActions(t *testing.T) {
	for _, action := range []string{"edit", "save", "double-click tab", "drag", "Enter", "open"} {
		t.Run(action, func(t *testing.T) {
			a, _ := testApp(t)
			paths := previewFixtures(t, a)
			clickTreeFile(a, 0)
			d := a.preview
			switch action {
			case "edit":
				a.typeInput("draft")
			case "save":
				a.save(d, nil)
			case "double-click tab":
				x, y := tabPoint(t, a, d)
				click(a, x, y)
				click(a, x, y)
			case "drag":
				x, y := tabPoint(t, a, d)
				a.mouse(tcell.NewEventMouse(x, y, tcell.Button1, 0))
				dragTo(a, a.editorArea.X, y)
			case "Enter":
				a.focus = "tree"
				a.key(tcell.NewEventKey(tcell.KeyEnter, "", 0))
			case "open":
				if err := a.open(paths[0]); err != nil {
					t.Fatal(err)
				}
			}
			if a.preview != nil {
				t.Fatal("user action did not keep the file open")
			}
			if err := a.openFile(paths[1], true); err != nil {
				t.Fatal(err)
			}
			if !hasDocument(a, d) || len(a.docs) != 3 || a.preview.Path != paths[1] {
				t.Fatal("next preview replaced the kept file")
			}
			if action == "edit" && (!d.Dirty() || d.Buffer.String()[:5] != "draft") {
				t.Fatal("preview browsing lost edits")
			}
		})
	}
}

func TestPreviewFailedOpenAndDirtyFallbackPreserveDocument(t *testing.T) {
	a, _ := testApp(t)
	paths := previewFixtures(t, a)
	clickTreeFile(a, 0)
	d := a.preview
	v := a.view()
	v.Set(4, false)
	bad := filepath.Join(a.root, "binary.dat")
	writeOpenFixture(t, bad, "\x00\x00")
	if err := a.openFile(bad, true); err == nil {
		t.Fatal("binary preview unexpectedly succeeded")
	}
	if a.preview != d || a.view() != v || v.Head() != 4 || len(a.docs) != 2 {
		t.Fatal("failed preview changed the current file")
	}
	// Defend against a buffer changed outside the usual changed() edit hook.
	d.Buffer.Apply([]text.Edit{{Start: 0, End: d.Buffer.Len(), Text: "unsaved"}}, nil, nil)
	if !d.Dirty() {
		t.Fatal("fixture is not dirty")
	}
	if err := a.openFile(paths[1], true); err != nil {
		t.Fatal(err)
	}
	if !hasDocument(a, d) || d.Buffer.String() != "unsaved" || a.modal != nil || len(a.docs) != 3 {
		t.Fatal("dirty preview was replaced or prompted during navigation")
	}
	current := a.preview
	a.closeDocument(current)
	if a.preview != nil || hasDocument(a, current) {
		t.Fatal("closing a preview left a stale tab")
	}
}

func TestPreviewProtectsFilesVisibleInOtherGroup(t *testing.T) {
	a, _ := testApp(t)
	paths := previewFixtures(t, a)
	clickTreeFile(a, 0)
	first := a.preview
	a.splitEditor()
	if a.preview != nil || a.views[0].Doc != first || a.views[1].Doc != first {
		t.Fatal("split did not keep the shared preview")
	}
	if err := a.openFile(paths[1], true); err != nil {
		t.Fatal(err)
	}
	second := a.preview
	a.active = 0
	if err := a.openFile(paths[2], true); err != nil {
		t.Fatal(err)
	}
	if a.views[1].Doc != second || !hasDocument(a, second) || a.preview.Path != paths[2] {
		t.Fatal("previewing in the left group replaced the right group's file")
	}
	third := a.preview
	a.active = 1
	a.activate(third)
	if a.preview != nil || a.views[0].Doc != third || a.views[1].Doc != third {
		t.Fatal("showing a preview in both groups did not keep it")
	}
}

func TestSearchMousePreviewsSelectsMatchAndKeepsOnDoubleClick(t *testing.T) {
	a, _ := testApp(t)
	paths := previewFixtures(t, a)
	clickTreeFile(a, 0)
	first := a.preview
	a.workspaceSearch()
	a.searchResults = []searchHit{
		{Path: paths[1], Text: "中 needle 👩‍💻", Column: len("中 "), End: len("中 needle")},
		{Path: paths[2], Text: "中 needle 👩‍💻", Column: len("中 "), End: len("中 needle")},
	}
	a.draw()
	hit := a.sideHits[0]
	click(a, hit.X+3, hit.Y)
	d := a.view().Doc
	lo, hi := a.view().Selections[0].Range()
	if d.Path != paths[1] || a.preview != d || hasDocument(a, first) || d.Buffer.String()[lo:hi] != "needle" || d.Dirty() {
		t.Fatal("search result did not reuse preview and select match")
	}
	a.draw()
	click(a, hit.X+3, hit.Y)
	if a.preview != nil || a.view().Doc != d {
		t.Fatal("search double click did not keep file")
	}
	a.focus = "search"
	a.searchSelected = 1
	a.key(tcell.NewEventKey(tcell.KeyEnter, "", 0))
	if a.preview != nil || a.view().Doc.Path != paths[2] || !hasDocument(a, d) {
		t.Fatal("search Enter did not open permanently")
	}
}

func TestPreviewDoubleClickRequiresConsecutiveFileClicks(t *testing.T) {
	for _, action := range []string{"other file", "other control", "key", "wheel", "timeout"} {
		t.Run(action, func(t *testing.T) {
			a, _ := testApp(t)
			previewFixtures(t, a)
			clickTreeFile(a, 0)
			switch action {
			case "other file":
				clickTreeFile(a, 1)
			case "other control":
				click(a, a.treeFilterRect.X, a.treeFilterRect.Y)
			case "key":
				a.key(tcell.NewEventKey(tcell.KeyRight, "", 0))
			case "wheel":
				a.mouse(tcell.NewEventMouse(a.sideRect.X+8, a.sideRect.Y+3, tcell.WheelUp, 0))
			case "timeout":
				a.fileClick.when = a.fileClick.when.Add(-time.Second)
			}
			clickTreeFile(a, 0)
			if a.preview == nil {
				t.Fatal("independent clicks were mistaken for a double click")
			}
		})
	}
}
