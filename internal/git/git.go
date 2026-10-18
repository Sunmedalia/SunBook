// Package git wraps Git without passing paths or commit messages through a shell.
package git

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"sort"
	"strconv"
	"strings"
)

type File struct{ Path, OldPath, Index, Worktree string }

func (f File) Conflicted() bool {
	switch f.Index + f.Worktree {
	case "DD", "AU", "UD", "UA", "DU", "AA", "UU":
		return true
	}
	return false
}
func (f File) Staged() bool {
	return !f.Conflicted() && f.Index != "" && f.Index != " " && f.Index != "?"
}
func (f File) Untracked() bool { return f.Index == "?" }
func (f File) Changed() bool {
	return f.Conflicted() || f.Untracked() || f.Worktree != "" && f.Worktree != " "
}

type Status struct {
	Root, Branch         string
	Head                 string
	Upstream             string
	Ahead, Behind        int
	Detached, HasCommits bool
	Files                []File
}

type Revision struct {
	Hash, Author, Age, Subject string
	Files                      []CommitFile
}

// Equal matches reflect.DeepEqual, including nil versus empty file lists,
// without reflection on every periodic refresh. TestEqualCoversEveryField
// fails when a new field is not compared here.
func (s Status) Equal(o Status) bool {
	return s.Root == o.Root && s.Branch == o.Branch && s.Head == o.Head && s.Upstream == o.Upstream &&
		s.Ahead == o.Ahead && s.Behind == o.Behind && s.Detached == o.Detached && s.HasCommits == o.HasCommits &&
		sameSlice(s.Files, o.Files)
}

// Equal matches reflect.DeepEqual for revisions.
func (r Revision) Equal(o Revision) bool {
	return r.Hash == o.Hash && r.Author == o.Author && r.Age == o.Age && r.Subject == o.Subject && sameSlice(r.Files, o.Files)
}

func sameSlice[T comparable](a, b []T) bool {
	return (a == nil) == (b == nil) && slices.Equal(a, b)
}

func Run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"--literal-pathspecs", "-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	return string(out), nil
}

// One porcelain v2 status reports the branch, upstream counts and files, so
// periodic refreshes start two Git processes instead of six.
func ReadStatus(ctx context.Context, dir string) (Status, error) {
	root, err := Run(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return Status{}, err
	}
	s := Status{Root: strings.TrimSpace(root)}
	raw, err := Run(ctx, s.Root, "--no-optional-locks", "status", "--porcelain=v2", "--branch", "--ahead-behind", "-z", "--untracked-files=all")
	if err != nil {
		return s, err
	}
	upstream := ""
	parts := strings.Split(raw, "\x00")
	for i := 0; i < len(parts); i++ {
		p := parts[i]
		if header, ok := strings.CutPrefix(p, "# "); ok {
			key, value, _ := strings.Cut(header, " ")
			switch key {
			case "branch.oid":
				if value != "(initial)" {
					s.Head, s.HasCommits = value, true
				}
			case "branch.head":
				s.Branch, s.Detached = value, value == "(detached)"
			case "branch.upstream":
				upstream = value
			case "branch.ab":
				// Git omits counts when the configured upstream ref is gone.
				s.Upstream = upstream
				if fields := strings.Fields(value); len(fields) == 2 {
					s.Ahead, _ = strconv.Atoi(strings.TrimPrefix(fields[0], "+"))
					s.Behind, _ = strconv.Atoi(strings.TrimPrefix(fields[1], "-"))
				}
			}
			continue
		}
		var f File
		switch {
		case strings.HasPrefix(p, "? "):
			f = File{Index: "?", Worktree: "?", Path: p[2:]}
		case strings.HasPrefix(p, "1 "):
			fields := strings.SplitN(p, " ", 9)
			if len(fields) != 9 {
				continue
			}
			f = statusFile(fields[1], fields[8])
		case strings.HasPrefix(p, "2 "):
			fields := strings.SplitN(p, " ", 10)
			if len(fields) != 10 {
				continue
			}
			f = statusFile(fields[1], fields[9])
			if i+1 < len(parts) {
				i++
				f.OldPath = parts[i]
			}
		case strings.HasPrefix(p, "u "):
			fields := strings.SplitN(p, " ", 11)
			if len(fields) != 11 {
				continue
			}
			f = statusFile(fields[1], fields[10])
		default:
			continue
		}
		s.Files = append(s.Files, f)
	}
	// v2 lists conflicts after ordinary changes; keep v1's single path order.
	sort.SliceStable(s.Files, func(i, j int) bool {
		a, b := s.Files[i], s.Files[j]
		if a.Untracked() != b.Untracked() {
			return b.Untracked()
		}
		return !a.Untracked() && a.Path < b.Path
	})
	if s.Detached {
		s.Branch = ""
		if short, e := Run(ctx, s.Root, "rev-parse", "--short", "HEAD"); e == nil {
			s.Branch = strings.TrimSpace(short)
		}
	}
	return s, nil
}

// Porcelain v2 marks an unchanged side with "." where v1 used a space.
func statusFile(xy, path string) File {
	if len(xy) != 2 {
		xy = "  "
	}
	side := func(c byte) string {
		if c == '.' {
			return " "
		}
		return string(c)
	}
	return File{Index: side(xy[0]), Worktree: side(xy[1]), Path: path}
}
func Stage(ctx context.Context, root, path string) error {
	_, e := Run(ctx, root, "add", "--", path)
	return e
}
func StageFile(ctx context.Context, root string, f File) error {
	return StageFiles(ctx, root, []File{f})
}
func StageFiles(ctx context.Context, root string, files []File) error {
	args := []string{"add", "-A", "--"}
	seen := map[string]bool{}
	for _, f := range files {
		paths := []string{f.Path}
		if f.OldPath != "" && f.Worktree == "R" {
			paths = append(paths, f.OldPath)
		}
		for _, path := range paths {
			if !seen[path] {
				args = append(args, path)
				seen[path] = true
			}
		}
	}
	if len(args) == 3 {
		return nil
	}
	_, e := Run(ctx, root, args...)
	return e
}
func StageAll(ctx context.Context, root string) error {
	_, e := Run(ctx, root, "add", "-A", "--", ".")
	return e
}
func UnstageAll(ctx context.Context, root string) error {
	if _, e := Run(ctx, root, "rev-parse", "--verify", "HEAD"); e != nil {
		_, e = Run(ctx, root, "rm", "-r", "--cached", "--", ".")
		return e
	}
	_, e := Run(ctx, root, "restore", "--staged", "--", ".")
	return e
}
func UnstageFile(ctx context.Context, root string, f File) error {
	if f.OldPath == "" {
		return Unstage(ctx, root, f.Path)
	}
	_, e := Run(ctx, root, "restore", "--staged", "--", f.Path, f.OldPath)
	return e
}
func Unstage(ctx context.Context, root, path string) error {
	if _, e := Run(ctx, root, "rev-parse", "--verify", "HEAD"); e != nil {
		_, e = Run(ctx, root, "rm", "--cached", "--", path)
		return e
	}
	_, e := Run(ctx, root, "restore", "--staged", "--", path)
	return e
}
func Diff(ctx context.Context, root string, f File, staged bool) (string, error) {
	if f.Index == "?" {
		out, err := exec.CommandContext(ctx, "git", "--literal-pathspecs", "-C", root, "diff", "--no-index", "--no-ext-diff", "--no-color", "--", "/dev/null", f.Path).CombinedOutput()
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return string(out), nil
		}
		return string(out), err
	}
	args := []string{"diff", "--no-ext-diff", "--no-color"}
	if staged {
		args = append(args, "--cached")
	}
	args = append(args, "--", f.Path)
	if f.OldPath != "" {
		args = append(args, f.OldPath)
	}
	return Run(ctx, root, args...)
}

// Read the history and every commit's changed files with one git log. File
// history first lists the followed commits, since --follow would otherwise
// limit each file list to the followed path.
func Log(ctx context.Context, root, path string) ([]Revision, error) {
	args := []string{"log", "--format=%H%x00%an%x00%ar%x00%s", "-z", "--name-status", "--root", "--find-renames", "--no-ext-diff", "--no-textconv", "--diff-merges=first-parent"}
	if path == "" {
		args = append(args, "-40", "--")
	} else {
		hashes, e := Run(ctx, root, "log", "-40", "--format=%H", "--follow", "--", path)
		if e != nil {
			return nil, e
		}
		if strings.TrimSpace(hashes) == "" {
			return nil, nil
		}
		args = append(append(args, "--no-walk=unsorted"), strings.Fields(hashes)...)
		args = append(args, "--")
	}
	out, e := Run(ctx, root, args...)
	if e != nil {
		return nil, e
	}
	parts := strings.Split(out, "\x00")
	var result []Revision
	for i := 0; i < len(parts); {
		if strings.TrimLeft(parts[i], "\n") == "" {
			i++
			continue
		}
		if i+4 > len(parts) {
			return nil, errors.New("invalid git log output")
		}
		revision := Revision{Hash: strings.TrimLeft(parts[i], "\n"), Author: parts[i+1], Age: parts[i+2], Subject: parts[i+3]}
		files, next, err := parseNameStatus(parts, i+4, revision.Hash)
		if err != nil {
			return nil, err
		}
		revision.Files, i = files, next
		result = append(result, revision)
	}
	return result, nil
}
func Show(ctx context.Context, root, hash, path string) (string, error) {
	args := []string{"show", "--format=medium", "--stat", "--patch", "--no-ext-diff", "--no-color", hash, "--"}
	if path != "" {
		args = append(args, path)
	}
	return Run(ctx, root, args...)
}
func Commit(ctx context.Context, root, message string) error {
	if strings.TrimSpace(message) == "" {
		return errors.New("commit message is empty")
	}
	out, e := Run(ctx, root, "diff", "--cached", "--name-only", "-z")
	if e != nil {
		return e
	}
	if out == "" {
		return errors.New("nothing staged to commit")
	}
	_, e = Run(ctx, root, "commit", "-m", message)
	return e
}
