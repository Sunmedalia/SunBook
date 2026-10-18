package git

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
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

// Changing any single field, or nil versus empty files, must make Equal
// disagree, exactly as reflect.DeepEqual does.
func TestEqualCoversEveryField(t *testing.T) {
	check := func(base any, equal func(a, b any) bool) {
		t.Helper()
		v := reflect.ValueOf(base)
		if !equal(base, base) {
			t.Fatal("value differs from itself")
		}
		for i := range v.NumField() {
			changed := reflect.New(v.Type()).Elem()
			changed.Set(v)
			f := changed.Field(i)
			switch f.Kind() {
			case reflect.String:
				f.SetString(f.String() + "x")
			case reflect.Int:
				f.SetInt(f.Int() + 1)
			case reflect.Bool:
				f.SetBool(!f.Bool())
			case reflect.Slice:
				f.Set(reflect.MakeSlice(f.Type(), 0, 0)) // nil versus empty
			default:
				t.Fatal("unhandled field kind", v.Type().Field(i).Name)
			}
			if equal(base, changed.Interface()) || reflect.DeepEqual(base, changed.Interface()) {
				t.Fatal("Equal ignores field", v.Type().Field(i).Name)
			}
		}
	}
	check(Status{}, func(a, b any) bool { return a.(Status).Equal(b.(Status)) })
	check(Revision{}, func(a, b any) bool { return a.(Revision).Equal(b.(Revision)) })
	files := []File{{Path: "a", Index: "M"}}
	if !(Status{Files: files}).Equal(Status{Files: []File{{Path: "a", Index: "M"}}}) || (Status{Files: files}).Equal(Status{Files: []File{{Path: "a", Index: "A"}}}) {
		t.Fatal("file list comparison")
	}
}
