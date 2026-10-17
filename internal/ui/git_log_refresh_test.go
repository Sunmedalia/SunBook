package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gitservice "sunbook/internal/git"
)

func TestGitCommitRevealsNewHeadInLog(t *testing.T) {
	for _, scope := range []string{"repository", "file history"} {
		t.Run(scope, func(t *testing.T) { testCommitRevealsNewHead(t, scope == "file history") })
	}
}
func testCommitRevealsNewHead(t *testing.T, fileHistory bool) {
	a := gitTestWorkspace(t)
	ctx := context.Background()
	file := filepath.Join(a.root, "file.txt")
	if err := os.WriteFile(file, []byte("first\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := gitservice.StageAll(ctx, a.root); err != nil {
		t.Fatal(err)
	}
	if err := gitservice.Commit(ctx, a.root, "older commit"); err != nil {
		t.Fatal(err)
	}
	a.showSidebar("Git")
	waitUpdates(t, a, func() bool { return !a.gitReading && !a.gitLogReading && a.gitLogLoaded })
	if len(a.gitLog) != 1 || a.gitLogTop != 0 {
		t.Fatal("initial history was not at the newest commit")
	}
	oldHash := a.gitLog[0].Hash
	if fileHistory {
		a.gitLogPath = "file.txt"
		a.refreshGitLog()
		waitUpdates(t, a, func() bool { return !a.gitLogReading })
		a.gitLogTop = 2
		a.gitLogNode = "file:file.txt"
	}
	// The new commit does not touch file.txt, so a lingering file filter would
	// hide it entirely, even after refreshing.
	if err := os.WriteFile(filepath.Join(a.root, "new.txt"), []byte("second\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := gitservice.StageAll(ctx, a.root); err != nil {
		t.Fatal(err)
	}
	a.refreshGit()
	waitUpdates(t, a, func() bool { return !a.gitReading })
	a.gitMessage = "new visible commit"
	a.focusGitMessage()
	a.commit()
	waitUpdates(t, a, func() bool { return !a.gitBusy && !a.gitReading && !a.gitLogReading && len(a.gitLog) == 2 })
	head, err := gitservice.Run(ctx, a.root, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if a.gitLog[0].Hash != strings.TrimSpace(head) || a.gitLog[0].Hash == oldHash {
		t.Fatal("new commit was not loaded")
	}
	if a.gitLogTop != 0 || a.gitLogSelected != 0 || a.gitLogNode != "" || a.gitLogPath != "" {
		t.Fatal("new commit was loaded but hidden behind the previous scroll/selection/filter", a.gitLogTop, a.gitLogSelected, a.gitLogNode, a.gitLogPath)
	}
	if a.focus != "gitMessage" {
		t.Fatal("refresh stole focus from the composer")
	}
	a.draw()
	// The newest commit must be in the visible rows, not just in the data cache.
	if a.gitLogRect.H < 1 || a.gitLogRows()[a.gitLogTop].Commit != 0 || a.gitLogRows()[a.gitLogTop].Kind != "commit" {
		t.Fatal("new commit is outside the visible log")
	}
}

func TestGitLogAtTopFollowsExternalCommit(t *testing.T) {
	a := gitTestWorkspace(t)
	ctx := context.Background()
	commit := func(message string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(a.root, "file.txt"), []byte(message), 0600); err != nil {
			t.Fatal(err)
		}
		if err := gitservice.StageAll(ctx, a.root); err != nil {
			t.Fatal(err)
		}
		if err := gitservice.Commit(ctx, a.root, message); err != nil {
			t.Fatal(err)
		}
	}
	commit("old head")
	a.showSidebar("Git")
	waitUpdates(t, a, func() bool { return !a.gitReading && !a.gitLogReading && a.gitLogLoaded })
	oldHash := a.gitLog[0].Hash
	commit("external head")
	a.refreshGit()
	waitUpdates(t, a, func() bool { return !a.gitReading && !a.gitLogReading && len(a.gitLog) == 2 })
	if a.gitLogTop != 0 || a.gitLog[0].Subject != "external head" {
		t.Fatal("top of history did not follow new commits", a.gitLogTop)
	}
	if a.gitLog[a.gitLogSelected].Hash != oldHash {
		t.Fatal("background refresh lost the historical selection")
	}
}

func TestGitFirstCommitAppearsInInitiallyEmptyLog(t *testing.T) {
	a := gitTestWorkspace(t)
	a.showSidebar("Git")
	waitUpdates(t, a, func() bool { return !a.gitReading && !a.gitLogReading && a.gitLogLoaded })
	if len(a.gitLog) != 0 {
		t.Fatal("fixture already has commits")
	}
	if err := os.WriteFile(filepath.Join(a.root, "first.txt"), []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := gitservice.StageAll(context.Background(), a.root); err != nil {
		t.Fatal(err)
	}
	a.refreshGit()
	waitUpdates(t, a, func() bool { return !a.gitReading })
	a.gitMessage = "initial UI commit"
	a.focusGitMessage()
	a.commit()
	waitUpdates(t, a, func() bool { return !a.gitBusy && !a.gitReading && !a.gitLogReading && len(a.gitLog) == 1 })
	if a.gitLogTop != 0 || a.gitLogSelected != 0 || a.gitLog[0].Subject != "initial UI commit" {
		t.Fatal("first commit did not replace empty history")
	}
}
