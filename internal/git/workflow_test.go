package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repository(t *testing.T) (context.Context, string) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.name", "SunBook Test"}, {"config", "user.email", "test@example.invalid"}, {"config", "commit.gpgsign", "false"}, {"config", "core.hooksPath", "/dev/null"}} {
		if _, e := Run(ctx, dir, args...); e != nil {
			t.Fatal(e)
		}
	}
	return ctx, dir
}
func writeFile(t *testing.T, dir, name, value string) {
	t.Helper()
	if e := os.WriteFile(filepath.Join(dir, name), []byte(value), 0644); e != nil {
		t.Fatal(e)
	}
}
func mustRun(t *testing.T, ctx context.Context, dir string, args ...string) string {
	t.Helper()
	s, e := Run(ctx, dir, args...)
	if e != nil {
		t.Fatal(e)
	}
	return strings.TrimSpace(s)
}
func TestBatchOperationsLiteralPathsAndRenameHistory(t *testing.T) {
	ctx, dir := repository(t)
	for _, name := range []string{"[draft].txt", "d.txt", "-option.txt", ":(glob)*.go", "中文 space.txt"} {
		writeFile(t, dir, name, "base\nstable line one\nstable line two\n")
	}
	if e := StageFiles(ctx, dir, []File{{Path: "[draft].txt"}, {Path: ":(glob)*.go"}}); e != nil {
		t.Fatal(e)
	}
	staged := mustRun(t, ctx, dir, "diff", "--cached", "--name-only", "-z")
	if strings.Contains(staged, "d.txt\x00") || !strings.Contains(staged, "[draft].txt\x00") || !strings.Contains(staged, ":(glob)*.go\x00") {
		t.Fatal("pathspec expanded or lost a literal file", staged)
	}
	if e := StageAll(ctx, dir); e != nil {
		t.Fatal(e)
	}
	if e := UnstageAll(ctx, dir); e != nil {
		t.Fatal("initial repository unstage", e)
	}
	if got := mustRun(t, ctx, dir, "ls-files"); got != "" {
		t.Fatal("index not empty", got)
	}
	if _, e := os.Stat(filepath.Join(dir, "[draft].txt")); e != nil {
		t.Fatal("unstage removed working file", e)
	}
	StageAll(ctx, dir)
	if e := Commit(ctx, dir, "initial"); e != nil {
		t.Fatal(e)
	}
	mustRun(t, ctx, dir, "mv", "--", "[draft].txt", "renamed 中文.txt")
	writeFile(t, dir, "renamed 中文.txt", "base\nstable line one\nstable line two\nadditional\n")
	status, e := ReadStatus(ctx, dir)
	if e != nil {
		t.Fatal(e)
	}
	var rename File
	for _, f := range status.Files {
		if f.Path == "renamed 中文.txt" {
			rename = f
		}
	}
	if rename.OldPath != "[draft].txt" {
		t.Fatal("rename missing source", status.Files)
	}
	if e := StageFile(ctx, dir, rename); e != nil {
		t.Fatal("staged rename with further edits", e)
	}
	if e := UnstageFile(ctx, dir, rename); e != nil {
		t.Fatal(e)
	}
	if got := mustRun(t, ctx, dir, "diff", "--cached", "--name-only"); got != "" {
		t.Fatal("rename only partially unstaged", got)
	}
	StageAll(ctx, dir)
	if e := Commit(ctx, dir, "rename"); e != nil {
		t.Fatal(e)
	}
	revisions, e := Log(ctx, dir, "renamed 中文.txt")
	if e != nil || len(revisions) != 2 || revisions[0].Subject != "rename" {
		t.Fatal("rename history", revisions, e)
	}
	patch, e := Show(ctx, dir, revisions[0].Hash, "renamed 中文.txt")
	if e != nil || !strings.Contains(patch, "+additional") {
		t.Fatal("commit patch", patch, e)
	}
	writeFile(t, dir, "d.txt", "changed\n")
	os.Remove(filepath.Join(dir, "-option.txt"))
	StageAll(ctx, dir)
	if e := UnstageAll(ctx, dir); e != nil {
		t.Fatal(e)
	}
	if mustRun(t, ctx, dir, "diff", "--cached", "--name-only") != "" {
		t.Fatal("batch unstage left index changes")
	}
	if _, e := os.Stat(filepath.Join(dir, "-option.txt")); !os.IsNotExist(e) {
		t.Fatal("unstage restored a deleted working file")
	}
}
func TestLocalTrackingCountsDetachedAndConflicts(t *testing.T) {
	ctx, dir := repository(t)
	writeFile(t, dir, "file.txt", "base\n")
	StageAll(ctx, dir)
	if e := Commit(ctx, dir, "base"); e != nil {
		t.Fatal(e)
	}
	base := mustRun(t, ctx, dir, "rev-parse", "HEAD")
	mustRun(t, ctx, dir, "remote", "add", "origin", t.TempDir())
	mustRun(t, ctx, dir, "update-ref", "refs/remotes/origin/main", base)
	mustRun(t, ctx, dir, "branch", "--set-upstream-to=origin/main", "main")
	writeFile(t, dir, "file.txt", "local\n")
	StageAll(ctx, dir)
	if e := Commit(ctx, dir, "local"); e != nil {
		t.Fatal(e)
	}
	mustRun(t, ctx, dir, "checkout", "-q", "-b", "remote-work", base)
	writeFile(t, dir, "file.txt", "remote\n")
	StageAll(ctx, dir)
	if e := Commit(ctx, dir, "remote"); e != nil {
		t.Fatal(e)
	}
	remote := mustRun(t, ctx, dir, "rev-parse", "HEAD")
	mustRun(t, ctx, dir, "update-ref", "refs/remotes/origin/main", remote)
	mustRun(t, ctx, dir, "checkout", "-q", "main")
	s, e := ReadStatus(ctx, dir)
	if e != nil || s.Upstream != "origin/main" || s.Ahead != 1 || s.Behind != 1 || !s.HasCommits {
		t.Fatal("local upstream counts", s, e)
	}
	mustRun(t, ctx, dir, "checkout", "-q", "--detach", "HEAD")
	s, e = ReadStatus(ctx, dir)
	if e != nil || !s.Detached || s.Branch == "" {
		t.Fatal("detached status", s, e)
	}
	mustRun(t, ctx, dir, "checkout", "-q", "main")
	if _, e := Run(ctx, dir, "merge", "--no-edit", "remote-work"); e == nil {
		t.Fatal("fixture did not conflict")
	}
	s, e = ReadStatus(ctx, dir)
	if e != nil || len(s.Files) != 1 || !s.Files[0].Conflicted() || s.Files[0].Staged() || !s.Files[0].Changed() {
		t.Fatal("conflict status", s, e)
	}
}

func TestStatusGoneUpstreamAndConflictOrder(t *testing.T) {
	ctx, dir := repository(t)
	writeFile(t, dir, "b.txt", "base\n")
	StageAll(ctx, dir)
	Commit(ctx, dir, "base")
	base := mustRun(t, ctx, dir, "rev-parse", "HEAD")
	mustRun(t, ctx, dir, "remote", "add", "origin", t.TempDir())
	mustRun(t, ctx, dir, "update-ref", "refs/remotes/origin/main", base)
	mustRun(t, ctx, dir, "branch", "--set-upstream-to=origin/main", "main")
	mustRun(t, ctx, dir, "update-ref", "-d", "refs/remotes/origin/main")
	s, e := ReadStatus(ctx, dir)
	if e != nil || s.Upstream != "" || s.Head != base || s.Branch != "main" || s.Detached {
		t.Fatal("gone upstream", s, e)
	}
	mustRun(t, ctx, dir, "checkout", "-q", "-b", "other")
	writeFile(t, dir, "b.txt", "other\n")
	mustRun(t, ctx, dir, "commit", "-q", "-am", "other")
	mustRun(t, ctx, dir, "checkout", "-q", "main")
	writeFile(t, dir, "b.txt", "main\n")
	mustRun(t, ctx, dir, "commit", "-q", "-am", "main")
	if _, e := Run(ctx, dir, "merge", "--no-edit", "other"); e == nil {
		t.Fatal("fixture did not conflict")
	}
	writeFile(t, dir, "c.txt", "c\n")
	writeFile(t, dir, "a.txt", "a\n")
	mustRun(t, ctx, dir, "add", "--", "c.txt", "a.txt")
	writeFile(t, dir, "untracked.txt", "u\n")
	s, e = ReadStatus(ctx, dir)
	var got []string
	for _, f := range s.Files {
		got = append(got, f.Index+f.Worktree+" "+f.Path)
	}
	if e != nil || strings.Join(got, ",") != "A  a.txt,UU b.txt,A  c.txt,?? untracked.txt" {
		t.Fatal("status order", got, e)
	}
}
