package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRevisionFilesAndSnapshotsPreserveLiteralPaths(t *testing.T) {
	ctx, dir := repository(t)
	odd := "[中]\tline\nname.go"
	for name, body := range map[string]string{odd: "package old\n", "old.txt": strings.Repeat("keep\n", 10), "delete.txt": "deleted\n"} {
		writeFile(t, dir, name, body)
	}
	StageAll(ctx, dir)
	if err := Commit(ctx, dir, "initial"); err != nil {
		t.Fatal(err)
	}
	initial := mustRun(t, ctx, dir, "rev-parse", "HEAD")
	files, err := CommitFiles(ctx, dir, initial)
	if err != nil || len(files) != 3 {
		t.Fatal("root commit file list", files, err)
	}
	for _, file := range files {
		pair, err := CompareRevision(ctx, dir, initial, file)
		if err != nil || pair.Old != "" || pair.New == "" || file.Status != "A" {
			t.Fatal("initial snapshot", file, pair, err)
		}
	}
	mustRun(t, ctx, dir, "mv", "--", "old.txt", "renamed.txt")
	os.Remove(filepath.Join(dir, "delete.txt"))
	writeFile(t, dir, odd, "package changed\n")
	writeFile(t, dir, "binary.bin", "\x00binary")
	StageAll(ctx, dir)
	if err := Commit(ctx, dir, "changes"); err != nil {
		t.Fatal(err)
	}
	hash := mustRun(t, ctx, dir, "rev-parse", "HEAD")
	// A dirty working tree must never supply the content of a historic diff.
	writeFile(t, dir, odd, "uncommitted\n")
	files, err = CommitFiles(ctx, dir, hash)
	if err != nil || len(files) != 4 {
		t.Fatal("changed file list", files, err)
	}
	seen := map[string]bool{}
	for _, file := range files {
		seen[file.Path] = true
		pair, err := CompareRevision(ctx, dir, hash, file)
		if err != nil {
			t.Fatal(file, err)
		}
		switch file.Path {
		case odd:
			if pair.Old != "package old\n" || pair.New != "package changed\n" || !strings.Contains(pair.Patch, "+package changed") {
				t.Fatal("history used working content or lost literal filename", pair)
			}
		case "renamed.txt":
			if file.Status != "R" || file.OldPath != "old.txt" || pair.Old != pair.New {
				t.Fatal("rename snapshot", file, pair)
			}
		case "delete.txt":
			if file.Status != "D" || pair.Old != "deleted\n" || pair.New != "" {
				t.Fatal("deleted snapshot", file, pair)
			}
		case "binary.bin":
			if !pair.Binary {
				t.Fatal("binary snapshot was treated as text")
			}
		}
	}
	if !seen[odd] || !seen["renamed.txt"] {
		t.Fatal("NUL-delimited filename or rename was lost")
	}
	log, err := Log(ctx, dir, "")
	if err != nil || len(log) != 2 || len(log[0].Files) != 4 || len(log[1].Files) != 3 {
		t.Fatal("log lacks per-commit file counts", log, err)
	}
}

func TestMergeRevisionUsesFirstParent(t *testing.T) {
	ctx, dir := repository(t)
	writeFile(t, dir, "base.txt", "base\n")
	StageAll(ctx, dir)
	Commit(ctx, dir, "base")
	mustRun(t, ctx, dir, "checkout", "-q", "-b", "feature")
	writeFile(t, dir, "feature.txt", "feature\n")
	StageAll(ctx, dir)
	Commit(ctx, dir, "feature")
	mustRun(t, ctx, dir, "checkout", "-q", "main")
	writeFile(t, dir, "main.txt", "main\n")
	StageAll(ctx, dir)
	Commit(ctx, dir, "main")
	mustRun(t, ctx, dir, "merge", "--no-ff", "-m", "merge", "feature")
	hash := mustRun(t, ctx, dir, "rev-parse", "HEAD")
	files, err := CommitFiles(ctx, dir, hash)
	if err != nil || len(files) != 1 || files[0].Path != "feature.txt" || files[0].Status != "A" {
		t.Fatal("merge file list does not match first parent", files, err)
	}
	pair, err := CompareRevision(ctx, dir, hash, files[0])
	if err != nil || pair.Old != "" || pair.New != "feature\n" {
		t.Fatal("merge snapshot", pair, err)
	}
}
