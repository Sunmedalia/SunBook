package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/vt"
	gitservice "sunbook/internal/git"
)

func gitTestWorkspace(t *testing.T) *App {
	t.Helper()
	a, _ := testApp(t)
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.name", "SunBook Test"}, {"config", "user.email", "test@example.invalid"}, {"config", "commit.gpgsign", "false"}, {"config", "core.hooksPath", "/dev/null"}} {
		if _, err := gitservice.Run(context.Background(), a.root, args...); err != nil {
			t.Fatal(err)
		}
	}
	return a
}
func TestGitInlineCommitPreservesDraftOnFailureAndCommitsOnlyStaged(t *testing.T) {
	a := gitTestWorkspace(t)
	ctx := context.Background()
	os.WriteFile(filepath.Join(a.root, "staged.txt"), []byte("staged\n"), 0644)
	os.WriteFile(filepath.Join(a.root, "untouched.txt"), []byte("leave untracked\n"), 0644)
	if err := gitservice.Stage(ctx, a.root, "staged.txt"); err != nil {
		t.Fatal(err)
	}
	hooks := t.TempDir()
	hook := filepath.Join(hooks, "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho fixture-rejected >&2\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := gitservice.Run(ctx, a.root, "config", "core.hooksPath", hooks); err != nil {
		t.Fatal(err)
	}
	a.showSidebar("Git")
	waitUpdates(t, a, func() bool { return !a.gitReading })
	a.draw()
	click(a, a.gitMessageRect.X+1, a.gitMessageRect.Y)
	a.paste("fix 中👩‍💻\nmessage")
	expected := "fix 中👩‍💻 message"
	if a.gitMessage != expected || a.view().Doc.Buffer.String() != "" {
		t.Fatal("message paste went to editor", a.gitMessage)
	}
	a.key(tcell.NewEventKey(tcell.KeyEnter, "", 0))
	waitUpdates(t, a, func() bool { return !a.gitBusy && !a.gitReading })
	if a.gitMessage != expected || !strings.Contains(a.gitActionError, "fixture-rejected") {
		t.Fatal("failed commit lost draft or error", a.gitMessage, a.gitActionError)
	}
	os.Remove(hook)
	a.key(tcell.NewEventKey(tcell.KeyEnter, "", 0))
	waitUpdates(t, a, func() bool { return !a.gitBusy && !a.gitReading })
	message, err := gitservice.Run(ctx, a.root, "log", "-1", "--format=%s")
	if err != nil || strings.TrimSpace(message) != expected || a.gitMessage != "" || a.gitActionError != "" {
		t.Fatal("successful inline commit", message, err, a.gitMessage, a.gitActionError)
	}
	files, err := gitservice.Run(ctx, a.root, "ls-tree", "-r", "--name-only", "HEAD")
	if err != nil || strings.TrimSpace(files) != "staged.txt" {
		t.Fatal("commit included unstaged content", files, err)
	}
}
func TestGitFilterKeepsRepositoryCountsAndConflictGuard(t *testing.T) {
	a, _ := testApp(t)
	a.sidebarView = "Git"
	a.focus = "git"
	a.git = gitservice.Status{Root: a.root, Branch: "main", Files: []gitservice.File{
		{Path: "dir/a.go", Index: "M", Worktree: "M"},
		{Path: "dir/中👩‍💻.go", Index: "?", Worktree: "?"},
		{Path: "conflict.go", Index: "U", Worktree: "U"},
	}}
	if len(a.gitEntries()) != 4 {
		t.Fatal("conflict duplicated or mixed versions lost", a.gitEntries())
	}
	a.focusGitFilter()
	a.paste("中👩‍💻")
	if len(a.gitEntries()) != 1 || a.git.Files[a.gitEntries()[0].File].Path != "dir/中👩‍💻.go" {
		t.Fatal("unicode filter", a.gitEntries())
	}
	if a.buttonEnabled("git.commit") {
		t.Fatal("filter hid a conflict from commit validation")
	}
	a.commit()
	if a.gitBusy || !strings.Contains(a.message, "conflicts") {
		t.Fatal("commit ignored conflict")
	}
	a.git.Files = a.git.Files[:2]
	if !a.buttonEnabled("git.commit") {
		t.Fatal("filter hid staged files from commit button")
	}
	a.key(tcell.NewEventKey(tcell.KeyBackspace, "", 0))
	if a.gitFilter != "中" {
		t.Fatal("filter split emoji grapheme", a.gitFilter)
	}
	a.key(tcell.NewEventKey(tcell.KeyEsc, "", 0))
	if a.gitFilter != "" || a.focus != "git" {
		t.Fatal("Esc did not clear Git filter")
	}
}
func TestGitGroupButtonsAndBatchUnstage(t *testing.T) {
	a := gitTestWorkspace(t)
	ctx := context.Background()
	os.WriteFile(filepath.Join(a.root, "tracked.txt"), []byte("base\n"), 0644)
	gitservice.StageAll(ctx, a.root)
	if err := gitservice.Commit(ctx, a.root, "fixture"); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(a.root, "tracked.txt"), []byte("changed\n"), 0644)
	os.WriteFile(filepath.Join(a.root, "fresh.txt"), []byte("fresh\n"), 0644)
	a.showSidebar("Git")
	waitUpdates(t, a, func() bool { return !a.gitReading })
	clickCommand(t, a, "git.stageCHANGES")
	waitUpdates(t, a, func() bool { return !a.gitBusy && !a.gitReading })
	staged, changed, _ := a.gitCounts()
	if staged != 1 || changed != 1 {
		t.Fatal("tracked group staged untracked files", a.git.Files)
	}
	clickCommand(t, a, "git.stageUNTRACKED")
	waitUpdates(t, a, func() bool { return !a.gitBusy && !a.gitReading })
	staged, changed, _ = a.gitCounts()
	if staged != 2 || changed != 0 {
		t.Fatal("untracked group not staged", a.git.Files)
	}
	clickCommand(t, a, "git.unstageAll")
	waitUpdates(t, a, func() bool { return !a.gitBusy && !a.gitReading })
	staged, changed, _ = a.gitCounts()
	if staged != 0 || changed != 2 {
		t.Fatal("batch unstage changed working files", a.git.Files)
	}
	if data, _ := os.ReadFile(filepath.Join(a.root, "tracked.txt")); string(data) != "changed\n" {
		t.Fatal("unstage changed content")
	}
}
func TestGitLogSidebarOpensCommitDiff(t *testing.T) {
	a := gitTestWorkspace(t)
	ctx := context.Background()
	os.WriteFile(filepath.Join(a.root, "file.txt"), []byte("history line\n"), 0644)
	gitservice.StageAll(ctx, a.root)
	if e := gitservice.Commit(ctx, a.root, "history fixture"); e != nil {
		t.Fatal(e)
	}
	a.showSidebar("Git")
	waitUpdates(t, a, func() bool { return !a.gitReading })
	a.gitHistory(false)
	waitUpdates(t, a, func() bool { return !a.gitLogReading })
	if a.modal != nil || len(a.gitLog) != 1 || a.focus != "gitLog" {
		t.Fatal("log is not an inline sidebar", a.gitLog)
	}
	a.draw()
	a.key(tcell.NewEventKey(tcell.KeyEnter, "", 0))
	if a.modal != nil || a.gitLogOpen[a.gitLog[0].Hash] {
		t.Fatal("Enter did not collapse commit files")
	}
	a.key(tcell.NewEventKey(tcell.KeyRune, "d", 0))
	waitUpdates(t, a, func() bool { return a.modal != nil })
	if !strings.HasPrefix(a.modal.Title, "Diff: Commit") || !strings.Contains(strings.Join(a.modal.Body, "\n"), "+history line") {
		t.Fatal("history did not open commit patch", a.modal)
	}
	a.modal.Body = []string{strings.Repeat("wide ", 60)}
	a.draw()
	a.modalKey(tcell.NewEventKey(tcell.KeyRight, "", 0), "right")
	if a.modal.BodyLeft != 8 {
		t.Fatal("diff cannot pan horizontally")
	}
	a.modalKey(tcell.NewEventKey(tcell.KeyHome, "", 0), "home")
	if a.modal.BodyLeft != 0 || a.modal.Top != 0 {
		t.Fatal("Home did not reset diff")
	}
}
func TestGitControlsFitResizedSidebar(t *testing.T) {
	a, mt := testApp(t)
	a.sidebarView = "Git"
	a.git = gitservice.Status{Root: a.root, Branch: "a-long-feature-branch", Files: []gitservice.File{{Path: "a.go", Index: "M", Worktree: "M"}, {Path: "b.go", Index: "?", Worktree: "?"}, {Path: "c.go", Index: "U", Worktree: "U"}}}
	for _, size := range []vt.Coord{{X: 110, Y: 32}, {X: 60, Y: 24}, {X: 60, Y: 14}, {X: 60, Y: 10}, {X: 45, Y: 20}} {
		mt.SetSize(size)
		a.screen.Sync()
		a.draw()
		for i, b := range a.buttons {
			if b.X < 0 || b.Y < 0 || b.X+b.W > int(size.X) || b.Y >= int(size.Y) {
				t.Fatal("outside screen", size, b)
			}
			for _, other := range a.buttons[:i] {
				if b.Y == other.Y && b.X < other.X+other.W && b.X+b.W > other.X {
					t.Fatal("overlapping actions", size, b, other)
				}
			}
		}
	}
}

func TestRenamedFileHistoryOpensPatchBeforeRename(t *testing.T) {
	a := gitTestWorkspace(t)
	ctx := context.Background()
	os.WriteFile(filepath.Join(a.root, "old.txt"), []byte("before rename\n"), 0644)
	gitservice.StageAll(ctx, a.root)
	if e := gitservice.Commit(ctx, a.root, "before rename"); e != nil {
		t.Fatal(e)
	}
	if _, e := gitservice.Run(ctx, a.root, "mv", "--", "old.txt", "new.txt"); e != nil {
		t.Fatal(e)
	}
	if e := gitservice.Commit(ctx, a.root, "rename file"); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(a.root, "new.txt"), []byte("edited\n"), 0644)
	a.showSidebar("Git")
	waitUpdates(t, a, func() bool { return !a.gitReading })
	a.gitHistory(true)
	waitUpdates(t, a, func() bool { return !a.gitLogReading })
	if len(a.gitLog) != 2 {
		t.Fatal("history did not follow rename", a.gitLog)
	}
	a.gitLogSelected = 1
	a.openGitRevision()
	waitUpdates(t, a, func() bool { return a.modal != nil })
	if !strings.Contains(strings.Join(a.modal.Body, "\n"), "+before rename") {
		t.Fatal("older renamed revision has no patch", a.modal.Body)
	}
}
