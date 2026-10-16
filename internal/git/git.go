// Package git wraps Git without passing paths or commit messages through a shell.
package git

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
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

func Run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"--literal-pathspecs", "-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	return string(out), nil
}
func ReadStatus(ctx context.Context, dir string) (Status, error) {
	root, err := Run(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return Status{}, err
	}
	s := Status{Root: strings.TrimSpace(root)}
	branch, err := Run(ctx, s.Root, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		branch, _ = Run(ctx, s.Root, "rev-parse", "--short", "HEAD")
		s.Detached = true
	}
	s.Branch = strings.TrimSpace(branch)
	head, headErr := Run(ctx, s.Root, "rev-parse", "--verify", "HEAD")
	if headErr == nil {
		s.Head = strings.TrimSpace(head)
	}
	s.HasCommits = headErr == nil
	if upstream, e := Run(ctx, s.Root, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}"); e == nil {
		s.Upstream = strings.TrimSpace(upstream)
		if counts, e := Run(ctx, s.Root, "rev-list", "--left-right", "--count", "HEAD...@{upstream}"); e == nil {
			parts := strings.Fields(counts)
			if len(parts) == 2 {
				s.Ahead, _ = strconv.Atoi(parts[0])
				s.Behind, _ = strconv.Atoi(parts[1])
			}
		}
	}
	raw, err := Run(ctx, s.Root, "--no-optional-locks", "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return s, err
	}
	parts := strings.Split(raw, "\x00")
	for i := 0; i < len(parts); i++ {
		p := parts[i]
		if len(p) < 4 {
			continue
		}
		f := File{Index: p[:1], Worktree: p[1:2], Path: p[3:]}
		if f.Index == "R" || f.Index == "C" || f.Worktree == "R" || f.Worktree == "C" {
			if i+1 < len(parts) {
				i++
				f.OldPath = parts[i]
			}
		}
		s.Files = append(s.Files, f)
	}
	return s, nil
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
func Log(ctx context.Context, root, path string) ([]Revision, error) {
	args := []string{"log", "-40", "--format=%H%x00%an%x00%ar%x00%s"}
	if path != "" {
		args = append(args, "--follow")
	}
	args = append(args, "--")
	if path != "" {
		args = append(args, path)
	}
	out, e := Run(ctx, root, args...)
	if e != nil {
		return nil, e
	}
	var result []Revision
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		fields := strings.SplitN(line, "\x00", 4)
		if len(fields) == 4 {
			files, err := CommitFiles(ctx, root, fields[0])
			if err != nil {
				return nil, err
			}
			result = append(result, Revision{Hash: fields[0], Author: fields[1], Age: fields[2], Subject: fields[3], Files: files})
		}
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
