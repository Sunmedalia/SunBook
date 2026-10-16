package ui

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v3"
)

func TestSearchOptionsUseLiteralCaseWordsAndGlobs(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep unavailable")
	}
	a, _ := testApp(t)
	writeOpenFixture(t, filepath.Join(a.root, "src", "code.txt"), "cat CAT cats [x]\n")
	writeOpenFixture(t, filepath.Join(a.root, "src", "skip.txt"), "cat\n")
	writeOpenFixture(t, filepath.Join(a.root, "other.md"), "cat\n")
	for _, tc := range []struct {
		name, query string
		opts        searchOptions
		count       int
	}{
		{"smart case", "cat", searchOptions{Regex: true}, 5},
		{"match case", "cat", searchOptions{Case: true}, 4},
		{"whole word", "cat", searchOptions{Word: true}, 4},
		{"literal", "[x]", searchOptions{}, 1},
		{"include and exclude", "cat", searchOptions{Include: "src/**", Exclude: "**/skip.txt"}, 3},
		{"multiple includes", "cat", searchOptions{Word: true, Include: "*.md,**/skip.txt"}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a.searchOptions = tc.opts
			a.setSearchQuery(tc.query)
			a.startWorkspaceSearch()
			waitUpdates(t, a, func() bool { return !a.searchRunning })
			if a.searchError != "" || len(a.searchResults) != tc.count {
				t.Fatal("search", a.searchError, len(a.searchResults), a.searchResults)
			}
		})
	}
}
func TestSearchHighlightsLateMatchCollapsesGroupAndSelectsRange(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep unavailable")
	}
	a, _ := testApp(t)
	path := filepath.Join(a.root, "中文.txt")
	prefix := strings.Repeat("context ", 30) + "中 "
	writeOpenFixture(t, path, prefix+"needle\n")
	a.workspaceSearch()
	a.setSearchQuery("needle")
	a.startWorkspaceSearch()
	waitUpdates(t, a, func() bool { return !a.searchRunning })
	a.draw()
	if len(a.searchResults) != 1 || a.searchResults[0].End != len(prefix)+len("needle") {
		t.Fatal("match end missing")
	}
	h := a.sideHits[0]
	found := false
	for x := h.X; x < h.X+h.W; x++ {
		s, style, _ := a.screen.Get(x, h.Y)
		if s == "n" && style.GetForeground() == a.accent.GetForeground() {
			found = true
		}
	}
	if !found {
		t.Fatal("late match not visible or highlighted")
	}
	group := a.searchGroupHits[0]
	click(a, group.X+1, group.Y)
	a.draw()
	if len(a.sideHits) != 0 || !a.searchCollapsed[path] {
		t.Fatal("group did not collapse")
	}
	a.key(tcell.NewEventKey(tcell.KeyDown, "", 0))
	a.draw()
	if len(a.sideHits) != 1 || a.searchCollapsed[path] {
		t.Fatal("keyboard navigation did not reveal result")
	}
	a.key(tcell.NewEventKey(tcell.KeyEnter, "", 0))
	lo, hi := a.view().Selections[0].Range()
	if a.view().Head() != len(prefix) || a.view().Doc.Buffer.String()[lo:hi] != "needle" {
		t.Fatal("match not selected")
	}
}
func TestSearchPathFieldsMousePasteAndOptions(t *testing.T) {
	a, _ := testApp(t)
	a.workspaceSearch()
	a.setSearchQuery("needle")
	clickCommand(t, a, "search.paths")
	a.draw()
	click(a, a.searchIncludeRect.X+1, a.searchIncludeRect.Y)
	a.paste("*.go")
	a.key(tcell.NewEventKey(tcell.KeyTab, "", 0))
	a.paste("vendor/**")
	if a.searchOptions.Include != "*.go" || a.searchOptions.Exclude != "vendor/**" || a.searchDue.IsZero() {
		t.Fatal("path fields", a.searchOptions)
	}
	a.key(tcell.NewEventKey(tcell.KeyCtrlU, "", 0))
	if a.searchOptions.Exclude != "" || a.searchExcludeCursor != 0 {
		t.Fatal("exclude clear")
	}
	clickCommand(t, a, "search.case")
	clickCommand(t, a, "search.word")
	clickCommand(t, a, "search.regex")
	if !a.searchOptions.Case || !a.searchOptions.Word || a.searchOptions.Regex {
		t.Fatal("toggle actions not wired")
	}
	clickCommand(t, a, "search.paths")
	if a.focus != "searchInput" {
		t.Fatal("hidden input retained focus")
	}
	a.stopWorkspaceSearch()
}
func TestSearchLimitAndStaleStreamingResults(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep unavailable")
	}
	a, _ := testApp(t)
	writeOpenFixture(t, filepath.Join(a.root, "many.txt"), strings.Repeat("needle\n", 2100))
	a.setSearchQuery("needle")
	a.startWorkspaceSearch()
	waitUpdates(t, a, func() bool { return !a.searchRunning })
	if len(a.searchResults) != 2000 || !a.searchTruncated || a.searchError != "" {
		t.Fatal("limit not reported", len(a.searchResults), a.searchTruncated, a.searchError)
	}
	seq := a.searchSeq
	a.clearWorkspaceSearch()
	a.publishSearch(seq, a.root, []searchHit{{Path: "stale"}}, "", false, false)
	waitUpdates(t, a, func() bool { return len(a.updates) == 0 })
	if a.searchRunning || len(a.searchResults) != 0 {
		t.Fatal("stale streaming update revived search")
	}
}

func TestSearchSelectionIncludesWholeCombiningGrapheme(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep unavailable")
	}
	a, _ := testApp(t)
	writeOpenFixture(t, filepath.Join(a.root, "accent.txt"), "é")
	for _, query := range []string{"e", "́"} {
		a.setSearchQuery(query)
		a.startWorkspaceSearch()
		waitUpdates(t, a, func() bool { return !a.searchRunning })
		a.openSearchResult()
		lo, hi := a.view().Selections[0].Range()
		if a.view().Doc.Buffer.String()[lo:hi] != "é" {
			t.Fatal("match selection split combining grapheme", query, lo, hi)
		}
	}
}
