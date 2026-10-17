package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

type Branch struct {
	Name, Ref, Upstream, Hash, Subject, Worktree string
	Current, Remote                              bool
}

func Branches(ctx context.Context, root string) ([]Branch, error) {
	out, err := Run(ctx, root, "for-each-ref", "--sort=-committerdate", "--format=%(refname)%00%(HEAD)%00%(upstream:short)%00%(objectname:short)%00%(subject)%00%(worktreepath)%00%(symref)%00", "refs/heads/", "refs/remotes/")
	if err != nil {
		return nil, err
	}
	fields := strings.Split(out, "\x00")
	var branches []Branch
	for i := 0; i+6 < len(fields); i += 7 {
		ref := strings.TrimPrefix(fields[i], "\n")
		if fields[i+6] != "" {
			continue
		}
		remote := strings.HasPrefix(ref, "refs/remotes/")
		name := strings.TrimPrefix(strings.TrimPrefix(ref, "refs/heads/"), "refs/remotes/")
		branches = append(branches, Branch{Name: name, Ref: ref, Current: fields[i+1] == "*", Remote: remote, Upstream: fields[i+2], Hash: fields[i+3], Subject: fields[i+4], Worktree: fields[i+5]})
	}
	return branches, nil
}
func ValidateBranch(ctx context.Context, root, name string) error {
	if name == "" || name == "HEAD" || strings.HasPrefix(name, "-") || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return errors.New("enter a valid branch name")
	}
	out, err := Run(ctx, root, "check-ref-format", "--branch", name)
	if err != nil {
		return err
	}
	if strings.TrimSuffix(out, "\n") != name {
		return errors.New("branch shortcuts are not accepted; use a literal name")
	}
	return nil
}
func resolveBranch(ctx context.Context, root, ref string) (string, error) {
	if !strings.HasPrefix(ref, "refs/heads/") && !strings.HasPrefix(ref, "refs/remotes/") {
		return "", errors.New("select a local or remote branch")
	}
	out, err := Run(ctx, root, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
	return strings.TrimSpace(out), err
}
func CreateBranch(ctx context.Context, root, name, from string, switchTo, track bool) error {
	if err := ValidateBranch(ctx, root, name); err != nil {
		return err
	}
	if track && (!switchTo || !strings.HasPrefix(from, "refs/remotes/")) {
		return errors.New("tracking requires a remote branch and checkout")
	}
	start := ""
	if from != "" {
		var err error
		start, err = resolveBranch(ctx, root, from)
		if err != nil {
			return err
		}
		if track {
			start = from
		}
	}
	args := []string{"branch", name}
	if switchTo {
		args = []string{"switch", "-c", name}
		if track {
			args = append(args, "--track")
		}
	}
	if start != "" {
		args = append(args, start)
	}
	_, err := Run(ctx, root, args...)
	return err
}
func SwitchBranch(ctx context.Context, root, name string) error {
	if err := ValidateBranch(ctx, root, name); err != nil {
		return err
	}
	if _, err := resolveBranch(ctx, root, "refs/heads/"+name); err != nil {
		return err
	}
	_, err := Run(ctx, root, "switch", "--no-guess", name)
	return err
}
func RenameBranch(ctx context.Context, root, old, name string) error {
	if err := ValidateBranch(ctx, root, old); err != nil {
		return err
	}
	if err := ValidateBranch(ctx, root, name); err != nil {
		return err
	}
	_, err := Run(ctx, root, "branch", "-m", "--", old, name)
	return err
}
func DeleteBranch(ctx context.Context, root, name string) error {
	if err := ValidateBranch(ctx, root, name); err != nil {
		return err
	}
	_, err := Run(ctx, root, "branch", "-d", "--", name)
	return err
}
func MergeBranch(ctx context.Context, root, ref string) error {
	hash, err := resolveBranch(ctx, root, ref)
	if err != nil {
		return err
	}
	_, err = Run(ctx, root, "merge", "--no-edit", hash)
	return err
}
func AbortMerge(ctx context.Context, root string) error {
	_, err := Run(ctx, root, "merge", "--abort")
	return err
}
func MergeInProgress(ctx context.Context, root string) bool {
	path, err := Run(ctx, root, "rev-parse", "--git-path", "MERGE_HEAD")
	if err != nil {
		return false
	}
	path = strings.TrimSpace(path)
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	_, err = os.Stat(path)
	return err == nil
}
func SetUpstream(ctx context.Context, root, name, upstream string) error {
	if err := ValidateBranch(ctx, root, name); err != nil {
		return err
	}
	if _, err := resolveBranch(ctx, root, upstream); err != nil {
		return err
	}
	_, err := Run(ctx, root, "branch", "--set-upstream-to="+upstream, "--", name)
	return err
}
