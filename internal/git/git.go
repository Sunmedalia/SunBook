// Package git wraps Git without passing paths or commit messages through a shell.
package git

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type File struct{ Path, OldPath, Index, Worktree string }
type Status struct {
	Root, Branch string
	Files        []File
}

func Run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
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
	}
	s.Branch = strings.TrimSpace(branch)
	raw, err := Run(ctx, s.Root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
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
		out, err := exec.CommandContext(ctx, "git", "-C", root, "diff", "--no-index", "--no-ext-diff", "--", "/dev/null", f.Path).CombinedOutput()
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
