package ui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gdamore/tcell/v3"
	"sunbook/internal/text"
)

// File indexing is lazy and independent of expanded directories. Filtering must
// find a file even if none of its ancestor directories have been expanded.
func workspaceFiles(ctx context.Context, root string) ([]string, error) {
	out, err := exec.CommandContext(ctx, "rg", "--no-config", "--files", "--null", "--hidden", "-g", "!.git", root).Output()
	// Exit 1 means no visible files, not failure; falling back would leak ignored files.
	exit, noFiles := err.(*exec.ExitError)
	if err == nil || (noFiles && exit.ExitCode() == 1) {
		var paths []string
		for _, p := range strings.Split(string(out), "\x00") {
			if p != "" {
				paths = append(paths, p)
			}
		}
		return paths, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	var paths []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, e error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if e != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && (d.Name() == ".git" || d.Name() == "node_modules" || d.Name() == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() || d.Type()&os.ModeSymlink != 0 {
			paths = append(paths, path)
		}
		if len(paths) >= 20000 {
			return filepath.SkipAll
		}
		return nil
	})
	return paths, err
}
func (a *App) focusTreeFilter() {
	a.showSidebar("Explorer")
	a.focus = "filter"
	a.treeQueryCursor = len(a.treeQuery)
	a.completion = nil
}
func (a *App) setTreeQuery(s string) {
	a.treeQuery = strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(s)
	a.treeQueryCursor = len(a.treeQuery)
	a.treeSelected = 0
	a.treePath = ""
	a.treeTop = 0
	a.applyTreeFilter()
	if strings.TrimSpace(a.treeQuery) != "" && !a.treeIndexReady {
		a.loadTreeIndex()
	}
}
func (a *App) insertTreeQuery(s string) {
	p := a.treeQueryCursor
	tail := len(a.treeQuery) - p
	a.setTreeQuery(a.treeQuery[:p] + s + a.treeQuery[p:])
	a.treeQueryCursor = text.Snap(a.treeQuery, len(a.treeQuery)-tail)
}
func (a *App) applyTreeFilter() {
	query := strings.TrimSpace(a.treeQuery)
	if query == "" {
		a.tree = a.treeAll
	} else {
		a.tree = nil
		if a.treeIndexReady {
			a.tree = a.filterTreeIndex(strings.ToLower(query))
		} else {
			for _, e := range a.treeAll {
				if !e.Dir {
					rel, _ := filepath.Rel(a.root, e.Path)
					if fuzzy(filepath.ToSlash(rel), query) {
						e.Name = filepath.ToSlash(rel)
						e.Depth = 0
						a.tree = append(a.tree, e)
					}
				}
			}
		}
	}
	if a.treePath == a.root {
		a.treeSelected = -1
	} else {
		found := false
		for i, entry := range a.tree {
			if entry.Path == a.treePath {
				a.treeSelected, found = i, true
				break
			}
		}
		if !found {
			a.treePath = ""
			a.treeSelected = max(0, min(a.treeSelected, len(a.tree)-1))
		}
	}
	a.treeTop = max(0, min(a.treeTop, max(0, len(a.tree)-1)))
}

func (a *App) selectTree(index int) {
	if index < 0 {
		a.treeSelected, a.treePath, a.newFileDir = -1, a.root, a.root
		return
	}
	if index >= len(a.tree) {
		return
	}
	entry := a.tree[index]
	a.treeSelected, a.treePath = index, entry.Path
	a.newFileDir = entry.Path
	if !entry.Dir {
		a.newFileDir = filepath.Dir(entry.Path)
	}
}
func (a *App) loadTreeIndex() {
	if a.treeIndexReady || a.treeIndexLoading {
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.treeIndexCancel = cancel
	a.treeIndexLoading = true
	a.treeIndexSeq++
	seq := a.treeIndexSeq
	root := a.root
	go func() {
		defer cancel()
		files, err := workspaceFiles(ctx, root)
		sort.Strings(files)
		index := newFileIndex(root, files)
		a.post(func() {
			if seq != a.treeIndexSeq || root != a.root {
				return
			}
			a.treeIndexLoading = false
			if m := a.modal; m != nil && m.Open != nil && m.Open.Quick {
				m.Open.Loading = false
				m.Open.Index = index
				if err != nil {
					m.Open.Error = err.Error()
				}
				a.updateOpenModal(m)
			}
			if err != nil {
				a.showError(err)
				return
			}
			a.setTreeIndex(index)
			a.applyTreeFilter()
		})
	}()
}

// fileIndex holds the workspace files with their relative forms computed once,
// off the UI thread, for Explorer filtering and Quick Open ranking.
type fileIndex struct {
	Paths []string // absolute and sorted
	Rel   []string // filepath.Rel from the root; empty if that failed
	Keys  []string // Rel lower-cased with forward slashes
	Names []string // filepath.Base of Paths
	Bases []string // filepath.Base of Keys
}

func newFileIndex(root string, paths []string) fileIndex {
	n := len(paths)
	index := fileIndex{Paths: paths, Rel: make([]string, n), Keys: make([]string, n), Names: make([]string, n), Bases: make([]string, n)}
	for i, path := range paths {
		if rel, err := filepath.Rel(root, path); err == nil {
			index.Rel[i] = rel
			index.Keys[i] = strings.ToLower(filepath.ToSlash(rel))
		}
		index.Names[i] = filepath.Base(path)
		index.Bases[i] = filepath.Base(index.Keys[i])
	}
	return index
}

func (a *App) setTreeIndex(index fileIndex) {
	a.treeFiles = index
	a.treeIndexReady = true
	a.treeMatchQuery, a.treeMatches = "", nil
}

// filterTreeIndex matches a lower-case query against the index. Extending the
// previous query can only remove matches (a fuzzy match of the longer query
// is a subsequence match of its prefix), so typing narrows the last result.
func (a *App) filterTreeIndex(needle string) []treeEntry {
	index := a.treeFiles
	candidates := a.treeMatches
	full := a.treeMatchQuery == "" || !strings.HasPrefix(needle, a.treeMatchQuery)
	if full {
		candidates = nil
	}
	var matches []int
	check := func(i int) {
		if index.Rel[i] != "" && fuzzyLower(index.Keys[i], needle) {
			matches = append(matches, i)
		}
	}
	if full {
		for i := range index.Paths {
			check(i)
		}
	} else {
		for _, i := range candidates {
			check(i)
		}
	}
	a.treeMatchQuery, a.treeMatches = needle, matches
	if len(matches) == 0 {
		return nil
	}
	tree := make([]treeEntry, 0, len(matches))
	for _, i := range matches {
		tree = append(tree, treeEntry{Path: index.Paths[i], Name: filepath.ToSlash(index.Rel[i])})
	}
	return tree
}
func (a *App) invalidateTreeIndex() {
	if a.treeIndexCancel != nil {
		a.treeIndexCancel()
	}
	a.treeIndexSeq++
	a.treeIndexLoading = false
	a.treeIndexReady = false
	if strings.TrimSpace(a.treeQuery) != "" {
		a.loadTreeIndex()
	}
}
func (a *App) treeFilterKey(e *tcell.EventKey, name string) {
	switch name {
	case "escape":
		a.setTreeQuery("")
		a.focus = "tree"
	case "enter":
		a.focus = "tree"
		a.toggleTree()
	case "down", "up":
		a.focus = "tree"
		a.selectTree(a.treeSelected)
	case "left":
		a.treeQueryCursor = text.Prev(a.treeQuery, a.treeQueryCursor)
	case "right":
		a.treeQueryCursor = text.Next(a.treeQuery, a.treeQueryCursor)
	case "home", "ctrl+a":
		a.treeQueryCursor = 0
	case "end", "ctrl+e":
		a.treeQueryCursor = len(a.treeQuery)
	case "ctrl+u":
		a.setTreeQuery("")
	case "ctrl+v":
		a.readClipboard()
	case "backspace", "backspace2":
		p := text.Prev(a.treeQuery, a.treeQueryCursor)
		head := a.treeQueryCursor
		a.setTreeQuery(a.treeQuery[:p] + a.treeQuery[head:])
		a.treeQueryCursor = p
	case "delete":
		head := a.treeQueryCursor
		p := text.Next(a.treeQuery, head)
		a.setTreeQuery(a.treeQuery[:head] + a.treeQuery[p:])
		a.treeQueryCursor = head
	default:
		if e.Str() != "" && e.Modifiers()&(tcell.ModCtrl|tcell.ModAlt|tcell.ModMeta) == 0 {
			a.insertTreeQuery(e.Str())
		}
	}
}
