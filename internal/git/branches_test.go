package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBranchLifecycleTrackingAndWorktrees(t *testing.T) {
	ctx, root := repository(t)
	writeFile(t, root, "file", "base\n")
	StageAll(ctx, root)
	if err := Commit(ctx, root, "base"); err != nil {
		t.Fatal(err)
	}
	if err := CreateBranch(ctx, root, "feature/中文", "", true, false); err != nil {
		t.Fatal(err)
	}
	if err := RenameBranch(ctx, root, "feature/中文", "feature/renamed"); err != nil {
		t.Fatal(err)
	}
	branches, err := Branches(ctx, root)
	if err != nil || len(branches) != 2 {
		t.Fatal(branches, err)
	}
	if !branches[0].Current && !branches[1].Current {
		t.Fatal("current marker missing")
	}
	if err := DeleteBranch(ctx, root, "feature/renamed"); err == nil {
		t.Fatal("deleted current branch")
	}
	if err := SwitchBranch(ctx, root, "main"); err != nil {
		t.Fatal(err)
	}
	if err := DeleteBranch(ctx, root, "feature/renamed"); err != nil {
		t.Fatal(err)
	}
	remote := t.TempDir()
	mustRun(t, ctx, remote, "init", "--bare", "-q")
	mustRun(t, ctx, root, "remote", "add", "origin", remote)
	mustRun(t, ctx, root, "push", "-u", "origin", "main")
	if err := CreateBranch(ctx, root, "tracked", "refs/remotes/origin/main", true, true); err != nil {
		t.Fatal(err)
	}
	s, err := ReadStatus(ctx, root)
	if err != nil || s.Upstream != "origin/main" {
		t.Fatal(s, err)
	}
	if err := SetUpstream(ctx, root, "main", "refs/remotes/origin/main"); err != nil {
		t.Fatal(err)
	}
	mustRun(t, ctx, root, "worktree", "add", "-q", filepath.Join(t.TempDir(), "other"), "main")
	branches, err = Branches(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, b := range branches {
		if b.Name == "main" && !b.Remote {
			found = b.Worktree != ""
		}
	}
	if !found {
		t.Fatal("worktree occupancy missing")
	}
	if err := SwitchBranch(ctx, root, "main"); err == nil {
		t.Fatal("switched to occupied branch")
	}
}
func TestMergeConflictAbortAndUnmergedDeletion(t *testing.T) {
	ctx, root := repository(t)
	writeFile(t, root, "file", "base\n")
	StageAll(ctx, root)
	Commit(ctx, root, "base")
	if err := CreateBranch(ctx, root, "feature", "", true, false); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "file", "feature\n")
	StageAll(ctx, root)
	Commit(ctx, root, "feature")
	SwitchBranch(ctx, root, "main")
	if err := DeleteBranch(ctx, root, "feature"); err == nil {
		t.Fatal("deleted unmerged branch")
	}
	writeFile(t, root, "file", "main\n")
	StageAll(ctx, root)
	Commit(ctx, root, "main")
	if err := MergeBranch(ctx, root, "refs/heads/feature"); err == nil || !MergeInProgress(ctx, root) {
		t.Fatal("conflict wasn't preserved", err)
	}
	if err := AbortMerge(ctx, root); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(root, "file"))
	if string(data) != "main\n" || MergeInProgress(ctx, root) {
		t.Fatal("abort failed")
	}
	for _, bad := range []string{"-D", "HEAD", "a..b", "@{-1}", "hello\nworld", "a b", "refs/heads/../../x"} {
		if ValidateBranch(ctx, root, bad) == nil {
			t.Fatal("accepted", bad)
		}
	}
	if err := MergeBranch(ctx, root, "--help"); err == nil || !strings.Contains(err.Error(), "select") {
		t.Fatal(err)
	}
}
func TestCreateUnbornBranch(t *testing.T) {
	ctx, root := repository(t)
	if err := CreateBranch(ctx, root, "new-main", "", true, false); err != nil {
		t.Fatal(err)
	}
	s, err := ReadStatus(ctx, root)
	if err != nil || s.Branch != "new-main" || s.HasCommits {
		t.Fatal(s, err)
	}
}

func TestFastForwardMergePreservesCurrentBranch(t *testing.T) {
	ctx, root := repository(t)
	writeFile(t, root, "base", "base")
	StageAll(ctx, root)
	if err := Commit(ctx, root, "base"); err != nil {
		t.Fatal(err)
	}
	if err := CreateBranch(ctx, root, "feature", "refs/heads/main", true, false); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "feature", "feature")
	StageAll(ctx, root)
	if err := Commit(ctx, root, "feature"); err != nil {
		t.Fatal(err)
	}
	if err := SwitchBranch(ctx, root, "main"); err != nil {
		t.Fatal(err)
	}
	if err := MergeBranch(ctx, root, "refs/heads/feature"); err != nil {
		t.Fatal(err)
	}
	status, err := ReadStatus(ctx, root)
	if err != nil || status.Branch != "main" {
		t.Fatal(status, err)
	}
	if _, err := os.Stat(filepath.Join(root, "feature")); err != nil {
		t.Fatal(err)
	}
	if err := DeleteBranch(ctx, root, "feature"); err != nil {
		t.Fatal("merged branch could not be deleted", err)
	}
}
