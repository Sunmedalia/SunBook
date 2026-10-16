package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type Comparison struct {
	Root, Path, Old, New, OldLabel, NewLabel, Patch string
	Binary                                          bool
}

func Compare(ctx context.Context, root string, f File, staged bool) (Comparison, error) {
	c := Comparison{Root: root, Path: f.Path, OldLabel: "INDEX", NewLabel: "WORKTREE"}
	read := func(spec string) (string, error) { return Run(ctx, root, "show", spec) }
	var err error
	if staged {
		c.OldLabel, c.NewLabel = "HEAD", "INDEX"
		oldPath := f.Path
		if f.OldPath != "" {
			oldPath = f.OldPath
		}
		if f.Index != "A" && !f.Untracked() {
			c.Old, err = read("HEAD:" + oldPath)
			if err != nil {
				return c, err
			}
		}
		if f.Index != "D" && !f.Untracked() {
			c.New, err = read(":0:" + f.Path)
		}
	} else {
		if f.Conflicted() {
			c.OldLabel = "OURS"
			stages, e := Run(ctx, root, "ls-files", "--stage", "-z", "--", f.Path)
			if e != nil {
				return c, e
			}
			for _, entry := range strings.Split(stages, "\x00") {
				meta, _, _ := strings.Cut(entry, "\t")
				fields := strings.Fields(meta)
				if len(fields) == 3 && fields[2] == "2" {
					c.Old, err = read(":2:" + f.Path)
					break
				}
			}
		} else if !f.Untracked() && f.Index != "D" {
			c.Old, err = read(":0:" + f.Path)
		}
		if err != nil {
			return c, err
		}
		path := filepath.Join(root, f.Path)
		info, e := os.Lstat(path)
		if e == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				c.New, err = os.Readlink(path)
			} else {
				var data []byte
				data, err = os.ReadFile(path)
				c.New = string(data)
			}
		} else if !os.IsNotExist(e) {
			err = e
		}
	}
	if err != nil {
		return c, err
	}
	c.Binary = !utf8.ValidString(c.Old) || !utf8.ValidString(c.New) || strings.ContainsRune(c.Old, 0) || strings.ContainsRune(c.New, 0)
	if c.Binary {
		return c, nil
	}
	c.Old = normalizeText(c.Old)
	c.New = normalizeText(c.New)
	dir, e := os.MkdirTemp("", "sunbook-compare-*")
	if e != nil {
		return c, e
	}
	defer os.RemoveAll(dir)
	left, right := filepath.Join(dir, "old"), filepath.Join(dir, "new")
	if e = os.WriteFile(left, []byte(c.Old), 0600); e != nil {
		return c, e
	}
	if e = os.WriteFile(right, []byte(c.New), 0600); e != nil {
		return c, e
	}
	cmd := exec.CommandContext(ctx, "git", "diff", "--no-index", "--no-ext-diff", "--no-color", "--no-textconv", "--unified=3", "--", left, right)
	out, e := cmd.CombinedOutput()
	var exit *exec.ExitError
	if e != nil && !(errors.As(e, &exit) && exit.ExitCode() == 1) {
		return c, fmt.Errorf("git compare: %s (%w)", strings.TrimSpace(string(out)), e)
	}
	c.Patch = string(out)
	return c, nil
}
func normalizeText(s string) string {
	return strings.ReplaceAll(strings.TrimPrefix(s, "\ufeff"), "\r\n", "\n")
}
