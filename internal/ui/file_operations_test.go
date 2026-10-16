package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v3"
	gitservice "sunbook/internal/git"
)

func clickExplorerPath(t *testing.T, a *App, path string) {
	t.Helper()
	a.draw()
	for i, entry := range a.tree {
		if entry.Path == path {
			a.treeTop = max(0, i-(a.sideRect.H-4))
			a.draw()
			click(a, a.sideRect.X+8, a.sideRect.Y+3+i-a.treeTop)
			return
		}
	}
	t.Fatal("Explorer path is not visible", path, a.tree)
}

func treeContains(a *App, path string) bool {
	for _, entry := range a.tree {
		if entry.Path == path {
			return true
		}
	}
	return false
}

func TestNewUsesClickedFolderAndUntitledSaveKeepsItsDirectory(t *testing.T) {
	a, _ := testApp(t)
	parent := filepath.Join(a.root, "src")
	dir := filepath.Join(parent, "nested")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	a.scanTree()
	waitUpdates(t, a, func() bool { return treeContains(a, parent) })
	clickExplorerPath(t, a, parent)
	waitUpdates(t, a, func() bool { return treeContains(a, dir) })
	clickExplorerPath(t, a, dir)
	clickCommand(t, a, "file.new")
	if !strings.Contains(a.modal.Placeholder, "src/nested") {
		t.Fatal("New input does not identify the selected directory")
	}
	a.paste("first.txt")
	a.modalAccept()
	first := filepath.Join(dir, "first.txt")
	if a.view().Doc.Path != first {
		t.Fatal("New ignored the clicked folder", a.view().Doc.Path)
	}
	waitUpdates(t, a, func() bool { return treeContains(a, first) })
	clickExplorerPath(t, a, first)
	clickCommand(t, a, "file.new")
	a.paste("beside.txt")
	a.modalAccept()
	if a.view().Doc.Path != filepath.Join(dir, "beside.txt") {
		t.Fatal("selecting a file did not use its parent directory")
	}
	clickCommand(t, a, "file.new")
	a.modalAccept() // An untitled file also remembers where it was created.
	draft := a.view().Doc
	a.typeInput("remember the folder")
	a.draw()
	click(a, a.sideRect.X+4, a.sideRect.Y+1) // Workspace heading selects root.
	a.save(draft, nil)
	a.paste("draft.txt")
	a.modalAccept()
	if draft.Path != filepath.Join(dir, "draft.txt") || draft.Dirty() {
		t.Fatal("untitled Save lost the original creation directory", draft.Path)
	}
	clickCommand(t, a, "file.new")
	a.paste("root.txt")
	a.modalAccept()
	if a.view().Doc.Path != filepath.Join(a.root, "root.txt") {
		t.Fatal("clicking workspace name did not reset the creation directory")
	}
}

func TestExplorerDeleteCancelsAndClosesCleanSplitFile(t *testing.T) {
	a := gitTestWorkspace(t)
	path := filepath.Join(a.root, "delete-me.txt")
	writeOpenFixture(t, path, "tracked contents")
	if err := gitservice.StageAll(context.Background(), a.root); err != nil {
		t.Fatal(err)
	}
	if err := gitservice.Commit(context.Background(), a.root, "deletion fixture"); err != nil {
		t.Fatal(err)
	}
	a.scanTree()
	waitUpdates(t, a, func() bool { return treeContains(a, path) })
	clickExplorerPath(t, a, path)
	doc := a.view().Doc
	a.splitEditor()
	clickCommand(t, a, "explorer.delete")
	a.draw()
	if a.modal == nil || !a.modal.Compact || !strings.Contains(a.modal.Title, "delete-me.txt") {
		t.Fatal("Delete did not show a compact confirmation for the selected file")
	}
	a.key(tcell.NewEventKey(tcell.KeyEsc, "", 0))
	if data, err := os.ReadFile(path); err != nil || string(data) != "tracked contents" || !hasDocument(a, doc) {
		t.Fatal("cancelling Delete changed the file or its tab", err)
	}
	a.focus = "tree"
	a.key(tcell.NewEventKey(tcell.KeyDelete, "", 0))
	a.modalAccept()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("Delete did not remove the selected file", err)
	}
	if hasDocument(a, doc) || a.preview == doc || a.views[0].Doc == doc || a.views[1].Doc == doc {
		t.Fatal("deleted clean file remained open in an editor group")
	}
	waitUpdates(t, a, func() bool {
		if treeContains(a, path) || a.gitReading {
			return false
		}
		for _, file := range a.git.Files {
			if file.Path == "delete-me.txt" && file.Worktree == "D" {
				return true
			}
		}
		return false
	})
}

func TestDeleteKeepsUnsavedEditsAndUnlinksOnlySelectedSymlink(t *testing.T) {
	a, _ := testApp(t)
	path := filepath.Join(a.root, "draft.txt")
	writeOpenFixture(t, path, "original")
	if err := a.open(path); err != nil {
		t.Fatal(err)
	}
	a.typeInput("edited ")
	doc := a.view().Doc
	a.execute("file.delete")
	a.modalAccept()
	if !hasDocument(a, doc) || !doc.Dirty() || doc.Exists || doc.Buffer.String() != "edited original" {
		t.Fatal("deleting a dirty file discarded its unsaved edits")
	}
	a.save(doc, nil)
	if data, err := os.ReadFile(path); err != nil || string(data) != "edited original" {
		t.Fatal("retained edits could not restore the deleted file", err)
	}
	link := filepath.Join(a.root, "alias.txt")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	a.confirmDeleteFile(link)
	a.modalAccept()
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatal("selected symlink was not removed", err)
	}
	if _, err := os.Stat(path); err != nil || !hasDocument(a, doc) {
		t.Fatal("deleting a symlink affected its target or target tab", err)
	}
}
