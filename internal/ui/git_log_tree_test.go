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

func logRowPoint(t *testing.T, a *App, kind, path string) (int, int, int) {
	t.Helper()
	a.draw()
	for i, row := range a.gitLogRows() {
		if row.Kind == kind && row.Path == path {
			if i < a.gitLogTop || i >= a.gitLogTop+a.gitLogRect.H {
				a.gitLogTop = i
				a.draw()
			}
			return a.gitLogRect.X + 4, a.gitLogRect.Y + i - a.gitLogTop, i
		}
	}
	t.Fatal("Git log tree row missing", kind, path)
	return 0, 0, 0
}

func TestGitLogFileTreeShowsCountsAndOpensCommittedSnapshots(t *testing.T) {
	a := gitTestWorkspace(t)
	ctx := context.Background()
	main := filepath.Join(a.root, "src", "main.go")
	writeOpenFixture(t, main, "package old\n")
	writeOpenFixture(t, filepath.Join(a.root, "README.md"), "remove me\n")
	gitservice.StageAll(ctx, a.root)
	if err := gitservice.Commit(ctx, a.root, "base"); err != nil {
		t.Fatal(err)
	}
	writeOpenFixture(t, main, "package committed\n")
	writeOpenFixture(t, filepath.Join(a.root, "src", "lib", "new.py"), "print('new')\n")
	os.Remove(filepath.Join(a.root, "README.md"))
	gitservice.StageAll(ctx, a.root)
	if err := gitservice.Commit(ctx, a.root, "tree change"); err != nil {
		t.Fatal(err)
	}
	writeOpenFixture(t, main, "package worktree\n")
	a.typeInput("unsaved editor")
	original := a.view().Doc
	a.showSidebar("Git")
	waitUpdates(t, a, func() bool { return !a.gitReading && !a.gitLogReading })
	a.gitLogExpanded = true
	a.sidebarWidth = 40
	a.draw()
	hash := a.gitLog[0].Hash
	if len(a.gitLog[0].Files) != 3 || !a.gitLogOpen[hash] {
		t.Fatal("latest commit lacks file count or default expansion")
	}
	var meta strings.Builder
	for x := a.gitLogRect.X; x < a.gitLogRect.X+a.gitLogRect.W; x++ {
		s, _, _ := a.screen.Get(x, a.gitLogRect.Y+1)
		meta.WriteString(s)
	}
	if !strings.Contains(meta.String(), "3 files") {
		t.Fatal("commit file count is not visible", meta.String())
	}
	x, y, index := logRowPoint(t, a, "file", "src/main.go")
	if a.gitLogRows()[index].Depth != 1 || a.gitLogRows()[index].File.Status != "M" {
		t.Fatal("changed file is not nested under its directory")
	}
	click(a, x, y)
	waitUpdates(t, a, func() bool { return a.comparing() })
	if a.comparison.Pair.Old != "package old\n" || a.comparison.Pair.New != "package committed\n" || a.view().Doc != original || original.Buffer.String() != "unsaved editor" {
		t.Fatal("historical file comparison used the working tree or changed a draft")
	}
	if len(a.comparison.Highlights[0]) == 0 || len(a.comparison.Highlights[1]) == 0 {
		t.Fatal("historic comparison has no syntax highlighting")
	}
	x, y, _ = logRowPoint(t, a, "folder", "src")
	click(a, x, y)
	for _, row := range a.gitLogRows() {
		if row.Commit == 0 && strings.HasPrefix(row.Path, "src/") {
			t.Fatal("collapsed folder still shows children")
		}
	}
	a.key(tcell.NewEventKey(tcell.KeyLeft, "", 0))
	if a.gitLogNode != "" || a.gitLogSelected != 0 {
		t.Fatal("Left did not select the collapsed folder's commit")
	}
	a.key(tcell.NewEventKey(tcell.KeyRight, "", 0))
	if a.gitLogNode != "folder:src" {
		t.Fatal("Right did not select the first child directory", a.gitLogNode)
	}
	a.key(tcell.NewEventKey(tcell.KeyRight, "", 0))
	if a.gitLogCollapsed[hash+"\x00src"] {
		t.Fatal("Right did not expand folder")
	}
	// Keep a selected file and a visible row anchored through background refresh.
	_, _, index = logRowPoint(t, a, "file", "src/main.go")
	a.selectGitLogRow(a.gitLogRows()[index], index)
	a.gitLogTop = index
	anchor := a.gitLogRowKey(a.gitLogRows()[index])
	a.refreshGitLog()
	waitUpdates(t, a, func() bool { return !a.gitLogReading })
	if a.gitLogNode != "file:src/main.go" || a.gitLogRowKey(a.gitLogRows()[a.gitLogTop]) != anchor || !a.gitLogOpen[hash] {
		t.Fatal("refresh lost tree selection, expansion, or scroll anchor")
	}
}

func TestGitLogEmptyCommitAndKeyboardTreeNavigation(t *testing.T) {
	a := gitTestWorkspace(t)
	if _, err := gitservice.Run(context.Background(), a.root, "commit", "--allow-empty", "-m", "empty"); err != nil {
		t.Fatal(err)
	}
	a.showSidebar("Git")
	waitUpdates(t, a, func() bool { return !a.gitReading && !a.gitLogReading })
	a.focus = "gitLog"
	a.draw()
	rows := a.gitLogRows()
	if len(rows) != 3 || rows[2].Kind != "empty" || len(a.gitLog[0].Files) != 0 {
		t.Fatal("empty commit lacks a clear empty state", rows)
	}
	a.key(tcell.NewEventKey(tcell.KeyEnd, "", 0))
	if a.gitLogSelected != 0 || a.gitLogNode != "" {
		t.Fatal("End selected a non-actionable metadata/empty row")
	}
	a.key(tcell.NewEventKey(tcell.KeyEnter, "", 0))
	if len(a.gitLogRows()) != 2 || a.modal != nil {
		t.Fatal("Enter did not collapse empty commit")
	}
	a.key(tcell.NewEventKey(tcell.KeyRight, "", 0))
	if len(a.gitLogRows()) != 3 {
		t.Fatal("Right did not expand empty commit")
	}
}
