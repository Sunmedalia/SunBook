package git

import (
	"context"
	"fmt"
	"strings"
)

type CommitFile struct {
	Path, OldPath, Status string
}

// Use NUL delimiters so spaces, tabs and newlines in filenames stay intact.
// Merge commits are compared with their first parent, just like the file view.
func CommitFiles(ctx context.Context, root, hash string) ([]CommitFile, error) {
	out, err := Run(ctx, root, "show", "--format=", "--name-status", "-z", "--find-renames", "--no-ext-diff", "--no-textconv", "--diff-merges=first-parent", hash, "--")
	if err != nil {
		return nil, err
	}
	files, _, err := parseNameStatus(strings.Split(out, "\x00"), 0, hash)
	return files, err
}

// Parse NUL-delimited --name-status entries from parts[i:] until a token that
// is not a status letter, such as the next commit hash in git log output.
func parseNameStatus(parts []string, i int, hash string) ([]CommitFile, int, error) {
	var files []CommitFile
	for i < len(parts) {
		status := strings.TrimLeft(parts[i], "\n")
		if status == "" {
			i++
			continue
		}
		if !isNameStatus(status) {
			break
		}
		i++
		if i >= len(parts) || parts[i] == "" {
			return nil, i, fmt.Errorf("invalid changed-file list for commit %s", hash)
		}
		file := CommitFile{Status: status[:1], Path: parts[i]}
		i++
		if file.Status == "R" || file.Status == "C" {
			if i >= len(parts) || parts[i] == "" {
				return nil, i, fmt.Errorf("invalid rename in commit %s", hash)
			}
			file.OldPath, file.Path = file.Path, parts[i]
			i++
		}
		files = append(files, file)
	}
	return files, i, nil
}

// Status letters are uppercase and may carry a similarity score; commit hashes
// are lowercase hexadecimal, so the two never collide.
func isNameStatus(s string) bool {
	if !strings.ContainsRune("ACDMRTUXB", rune(s[0])) {
		return false
	}
	for _, c := range s[1:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func CompareRevision(ctx context.Context, root, hash string, file CommitFile) (Comparison, error) {
	c := Comparison{Root: root, Path: file.Path, NewLabel: hash[:min(7, len(hash))]}
	parents, err := Run(ctx, root, "rev-list", "--parents", "-n", "1", hash, "--")
	if err != nil {
		return c, err
	}
	fields := strings.Fields(parents)
	c.OldLabel = "EMPTY"
	if len(fields) > 1 {
		parent := fields[1]
		c.OldLabel = parent[:min(7, len(parent))]
		if file.Status != "A" {
			path := file.Path
			if file.OldPath != "" {
				path = file.OldPath
			}
			c.Old, err = Run(ctx, root, "show", parent+":"+path)
			if err != nil {
				return c, err
			}
		}
	}
	if file.Status != "D" {
		c.New, err = Run(ctx, root, "show", hash+":"+file.Path)
		if err != nil {
			return c, err
		}
	}
	return finishComparison(ctx, c)
}
