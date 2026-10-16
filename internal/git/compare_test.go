package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComparisonUsesHeadIndexAndWorktree(t *testing.T) {
	ctx, dir := repository(t)
	name := "space [中].txt"
	writeFile(t, dir, name, "base\nkeep\n")
	StageAll(ctx, dir)
	if e := Commit(ctx, dir, "base"); e != nil {
		t.Fatal(e)
	}
	writeFile(t, dir, name, "staged\nkeep\n")
	Stage(ctx, dir, name)
	writeFile(t, dir, name, "working\ninsert\nkeep\n")
	s, e := ReadStatus(ctx, dir)
	if e != nil {
		t.Fatal(e)
	}
	f := s.Files[0]
	staged, e := Compare(ctx, dir, f, true)
	if e != nil || staged.Old != "base\nkeep\n" || staged.New != "staged\nkeep\n" || staged.OldLabel != "HEAD" || staged.NewLabel != "INDEX" {
		t.Fatal("staged pair", staged, e)
	}
	work, e := Compare(ctx, dir, f, false)
	if e != nil || work.Old != staged.New || work.New != "working\ninsert\nkeep\n" || !strings.Contains(work.Patch, "+insert") {
		t.Fatal("worktree pair", work, e)
	}
	writeFile(t, dir, "new.txt", "new\n")
	fresh, e := Compare(ctx, dir, File{Path: "new.txt", Index: "?", Worktree: "?"}, false)
	if e != nil || fresh.Old != "" || fresh.New != "new\n" {
		t.Fatal("new file", fresh, e)
	}
	os.Remove(filepath.Join(dir, name))
	deleted, e := Compare(ctx, dir, File{Path: name, Index: "M", Worktree: "D"}, false)
	if e != nil || deleted.New != "" || deleted.Old != staged.New {
		t.Fatal("deleted file", deleted, e)
	}
}
func TestComparisonInitialRenameBinaryAndSymlink(t *testing.T) {
	ctx, dir := repository(t)
	writeFile(t, dir, "old.txt", "initial\n")
	StageAll(ctx, dir)
	initial, e := Compare(ctx, dir, File{Path: "old.txt", Index: "A", Worktree: " "}, true)
	if e != nil || initial.Old != "" || initial.New != "initial\n" {
		t.Fatal("initial index", initial, e)
	}
	if e := Commit(ctx, dir, "base"); e != nil {
		t.Fatal(e)
	}
	mustRun(t, ctx, dir, "mv", "--", "old.txt", "renamed.txt")
	pair, e := Compare(ctx, dir, File{Path: "renamed.txt", OldPath: "old.txt", Index: "R", Worktree: " "}, true)
	if e != nil || pair.Old != "initial\n" || pair.New != pair.Old {
		t.Fatal("rename pair", pair, e)
	}
	writeFile(t, dir, "binary.bin", "a\x00b")
	pair, e = Compare(ctx, dir, File{Path: "binary.bin", Index: "?", Worktree: "?"}, false)
	if e != nil || !pair.Binary {
		t.Fatal("binary detection", pair, e)
	}
	if e := os.Symlink("renamed.txt", filepath.Join(dir, "link")); e != nil {
		t.Fatal(e)
	}
	pair, e = Compare(ctx, dir, File{Path: "link", Index: "?", Worktree: "?"}, false)
	if e != nil || pair.New != "renamed.txt" {
		t.Fatal("symlink was followed", pair, e)
	}
}
