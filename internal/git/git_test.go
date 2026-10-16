package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitLifecycle(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.name", "SunBook Test"}, {"config", "user.email", "test@example.invalid"}, {"config", "commit.gpgsign", "false"}} {
		if _, e := Run(ctx, dir, args...); e != nil {
			t.Fatal(e)
		}
	}
	name := "space 中文.txt"
	os.WriteFile(filepath.Join(dir, name), []byte("hello\n"), 0644)
	s, e := ReadStatus(ctx, dir)
	if e != nil || s.Branch == "" || len(s.Files) != 1 || s.Files[0].Path != name {
		t.Fatal(s, e)
	}
	diff, e := Diff(ctx, s.Root, s.Files[0], false)
	if e != nil || !strings.Contains(diff, "+hello") {
		t.Fatal(diff, e)
	}
	if e = Stage(ctx, dir, name); e != nil {
		t.Fatal(e)
	}
	if e = Unstage(ctx, dir, name); e != nil {
		t.Fatal("unborn unstage", e)
	}
	Stage(ctx, dir, name)
	if Commit(ctx, dir, "") == nil {
		t.Fatal("empty message")
	}
	if e = Commit(ctx, dir, "initial"); e != nil {
		t.Fatal(e)
	}
	s, e = ReadStatus(ctx, dir)
	if e != nil || len(s.Files) != 0 {
		t.Fatal(s, e)
	}
	os.WriteFile(filepath.Join(dir, name), []byte("changed\n"), 0644)
	Stage(ctx, dir, name)
	s, _ = ReadStatus(ctx, dir)
	diff, e = Diff(ctx, dir, s.Files[0], true)
	if e != nil || !strings.Contains(diff, "+changed") {
		t.Fatal(diff, e)
	}
	if e = Unstage(ctx, dir, name); e != nil {
		t.Fatal(e)
	}
	if Commit(ctx, dir, "no staged changes") == nil {
		t.Fatal("committed unstaged changes")
	}
	newName := "renamed\n中文.txt"
	os.Rename(filepath.Join(dir, name), filepath.Join(dir, newName))
	Run(ctx, dir, "add", "-A")
	s, e = ReadStatus(ctx, dir)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, f := range s.Files {
		if f.Path == newName {
			found = true
		}
	}
	if !found {
		t.Fatal("NUL parsing lost special path", s.Files)
	}
}
