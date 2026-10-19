package ui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltinWorkspaceSearchWithoutRipgrep(t *testing.T) {
	a, _ := testApp(t)
	t.Setenv("PATH", t.TempDir())
	writeOpenFixture(t, filepath.Join(a.root, "src", "code.txt"), "cat CAT cats [x] 中文 中文字\n")
	writeOpenFixture(t, filepath.Join(a.root, "src", "skip.txt"), "cat\n")
	writeOpenFixture(t, filepath.Join(a.root, "other.md"), "cat\n")
	writeOpenFixture(t, filepath.Join(a.root, "binary.bin"), "cat\x00cat")
	writeOpenFixture(t, filepath.Join(a.root, ".hidden.txt"), "cat")
	for _, tc := range []struct {
		name, query string
		options     searchOptions
		count       int
	}{
		{"smart case", "cat", searchOptions{}, 5},
		{"uppercase", "CAT", searchOptions{}, 1},
		{"match case", "cat", searchOptions{Case: true}, 4},
		{"whole word", "cat", searchOptions{Word: true}, 4},
		{"unicode word", "中文", searchOptions{Word: true}, 1},
		{"literal", "[x]", searchOptions{}, 1},
		{"regex smart case", "ca[tT]s?", searchOptions{Regex: true}, 4},
		{"paths", "cat", searchOptions{Include: "src/**", Exclude: "**/skip.txt"}, 3},
		{"multiple includes", "cat", searchOptions{Word: true, Include: "*.md,**/skip.txt"}, 2},
		{"directory exclude", "cat", searchOptions{Exclude: "src/"}, 1},
		{"no matches", "missing", searchOptions{}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a.searchOptions = tc.options
			a.setSearchQuery(tc.query)
			a.startWorkspaceSearch()
			waitUpdates(t, a, func() bool { return !a.searchRunning })
			if a.searchError != "" || len(a.searchResults) != tc.count {
				t.Fatal(a.searchError, len(a.searchResults), a.searchResults)
			}
		})
	}
	a.searchOptions = searchOptions{Regex: true}
	a.setSearchQuery("[")
	a.startWorkspaceSearch()
	waitUpdates(t, a, func() bool { return !a.searchRunning })
	if a.searchError == "" {
		t.Fatal("invalid regex silently accepted")
	}
}

func TestBuiltinSearchIgnoresLimitAndCancellation(t *testing.T) {
	a, _ := testApp(t)
	t.Setenv("PATH", t.TempDir())
	writeOpenFixture(t, filepath.Join(a.root, ".gitignore"), "ignored/\n*.log\n!keep.log\n")
	writeOpenFixture(t, filepath.Join(a.root, ".ignore"), "skip.txt\n")
	writeOpenFixture(t, filepath.Join(a.root, "ignored", "a.txt"), "needle")
	writeOpenFixture(t, filepath.Join(a.root, "skip.txt"), "needle")
	writeOpenFixture(t, filepath.Join(a.root, "drop.log"), "needle")
	writeOpenFixture(t, filepath.Join(a.root, "keep.log"), "needle")
	writeOpenFixture(t, filepath.Join(a.root, "nested", ".gitignore"), "local.txt\n")
	writeOpenFixture(t, filepath.Join(a.root, "nested", "local.txt"), "needle")
	writeOpenFixture(t, filepath.Join(a.root, "nested", "good.txt"), "中 needle needle\n")
	a.setSearchQuery("needle")
	a.startWorkspaceSearch()
	waitUpdates(t, a, func() bool { return !a.searchRunning })
	if a.searchError != "" || len(a.searchResults) != 3 {
		t.Fatal(a.searchError, a.searchResults)
	}
	a.searchSelected = 1
	a.openSearchResult()
	lo, hi := a.view().Selections[0].Range()
	if a.view().Doc.Buffer.String()[lo:hi] != "needle" || lo != len("中 ") {
		t.Fatal("incorrect byte offsets", lo, hi)
	}
	writeOpenFixture(t, filepath.Join(a.root, "many.txt"), strings.Repeat("needle\n", 2100))
	a.startWorkspaceSearch()
	waitUpdates(t, a, func() bool { return !a.searchRunning })
	if len(a.searchResults) != searchLimit || !a.searchTruncated || a.searchError != "" {
		t.Fatal("limit", len(a.searchResults), a.searchError)
	}
	seq := a.searchSeq
	a.clearWorkspaceSearch()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a.builtinWorkspaceSearch(ctx, seq, a.root, "needle", searchOptions{})
	waitUpdates(t, a, func() bool { return len(a.updates) == 0 })
	if len(a.searchResults) != 0 || a.searchRunning {
		t.Fatal("cancelled search revived results")
	}
}
