package ui

import (
	"fmt"
	"math/rand/v2"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestExplorerFilterNarrowingMatchesFullScan(t *testing.T) {
	a, _ := testApp(t)
	rng := rand.New(rand.NewPCG(4, 4))
	parts := []string{"src", "Internal", "ui", "render.go", "中文", "a b", "README.md", "ü"}
	var files []string
	for i := range 2000 {
		files = append(files, filepath.Join(a.root, parts[i%len(parts)], parts[(i/8)%len(parts)], fmt.Sprintf("%s%d", parts[(i/64)%len(parts)], i)))
	}
	sort.Strings(files)
	a.setTreeIndex(newFileIndex(a.root, files))
	alphabet := []rune("srcIUnde/ 中ü.1")
	query := ""
	matched := 0
	for step := range 400 {
		switch {
		// Short queries keep most steps matching something.
		case query != "" && (rng.IntN(4) == 0 || len([]rune(query)) >= 3):
			r := []rune(query)
			query = string(r[:len(r)-1])
		case rng.IntN(10) == 0:
			// A new index must not reuse matches from the previous one.
			files = files[1:]
			a.setTreeIndex(newFileIndex(a.root, files))
		default:
			query += string(alphabet[rng.IntN(len(alphabet))])
		}
		a.setTreeQuery(query)
		var want []string
		if strings.TrimSpace(query) == "" {
			continue
		}
		for _, path := range files {
			rel, _ := filepath.Rel(a.root, path)
			if fuzzy(filepath.ToSlash(rel), strings.TrimSpace(query)) {
				want = append(want, path)
			}
		}
		var got []string
		for _, e := range a.tree {
			got = append(got, e.Path)
		}
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Fatalf("step %d query %q: %d matches, want %d", step, query, len(got), len(want))
		}
		if len(want) > 0 {
			matched++
		}
	}
	if matched < 200 {
		t.Fatal("too few steps had matches to exercise narrowing", matched)
	}
}

func TestQuickOpenKeepsBestTwoHundred(t *testing.T) {
	a, _ := testApp(t)
	var files []string
	for i := range 3000 {
		files = append(files, filepath.Join(a.root, fmt.Sprintf("dir%d", i%13), fmt.Sprintf("render_%d.go", i)))
	}
	sort.Strings(files)
	a.setTreeIndex(newFileIndex(a.root, files))
	a.recentFiles = []string{files[2999], files[7]}
	a.quickOpen()
	m := a.modal
	m.Input = "rend 1"
	a.updateOpenModal(m)
	type ranked struct {
		path  string
		score int
	}
	var all []ranked
	for _, path := range files {
		rel, _ := filepath.Rel(a.root, path)
		score, ok := fileRank(rel, m.Input)
		if !ok {
			continue
		}
		for i, recent := range a.recentFiles {
			if recent == path {
				score += max(1, 100-i)
				break
			}
		}
		all = append(all, ranked{path, score})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].score != all[j].score {
			return all[i].score > all[j].score
		}
		return all[i].path < all[j].path
	})
	if len(all) <= 200 || len(m.Visible) != 200 {
		t.Fatal("fixture should overflow the visible limit", len(all), len(m.Visible))
	}
	for i, it := range m.Visible {
		if it.Path != all[i].path {
			t.Fatalf("rank %d: %s, want %s", i, it.Path, all[i].path)
		}
	}
	if !strings.HasPrefix(m.Visible[0].Detail, "recent · ") {
		t.Fatal("recent bonus or detail lost", m.Visible[0])
	}
}
