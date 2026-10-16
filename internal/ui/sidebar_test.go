package ui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v3"
	gitservice "sunbook/internal/git"
)

func TestActivityBarSwitchesPersistentSidebars(t *testing.T) {
	a, _ := testApp(t)
	a.treeQuery = "remember"
	a.treeQueryCursor = len(a.treeQuery)
	clickCommand(t, a, "activity.search")
	if a.sidebarView != "Search" || a.focus != "searchInput" || a.modal != nil || a.panelVisible {
		t.Fatal("Search is not an independent sidebar")
	}
	a.setSearchQuery("needle")
	a.stopWorkspaceSearch()
	a.searchTop = 2
	clickCommand(t, a, "activity.git")
	if a.sidebarView != "Git" || a.panelVisible {
		t.Fatal("Git opened bottom panel")
	}
	clickCommand(t, a, "activity.explorer")
	if a.treeQuery != "remember" {
		t.Fatal("Explorer state lost")
	}
	clickCommand(t, a, "activity.search")
	if a.searchQuery != "needle" || a.searchTop != 2 {
		t.Fatal("Search state lost")
	}
	clickCommand(t, a, "activity.search")
	if a.sidebar || a.focus != "editor" {
		t.Fatal("active activity did not hide sidebar")
	}
	a.draw()
	if a.activityRect.W == 0 || a.sideRect.W != 0 {
		t.Fatal("activity bar must stay visible")
	}
	clickCommand(t, a, "activity.search")
	if !a.sidebar || a.searchQuery != "needle" {
		t.Fatal("activity did not restore sidebar")
	}
}
func TestSearchSidebarRunsAndJumpsWithUnicode(t *testing.T) {
	if _, e := exec.LookPath("rg"); e != nil {
		t.Skip("ripgrep not installed")
	}
	a, _ := testApp(t)
	dir := filepath.Join(a.root, "deep")
	os.Mkdir(dir, 0755)
	path := filepath.Join(dir, "result.txt")
	source := "中 needle needle\nsecond\n"
	os.WriteFile(path, []byte(source), 0644)
	a.workspaceSearch()
	a.paste("needle")
	a.key(tcell.NewEventKey(tcell.KeyEnter, "", 0))
	waitUpdates(t, a, func() bool { return !a.searchRunning })
	if len(a.searchResults) != 2 || a.searchResults[0].Column != len("中 ") || a.modal != nil {
		t.Fatal("search results", a.searchResults)
	}
	a.searchSelected = 1
	a.draw()
	for _, hit := range a.sideHits {
		if hit.Index == 1 {
			click(a, hit.X+1, hit.Y)
			break
		}
	}
	if a.focus != "editor" || a.view().Doc.Buffer.String() != source || a.view().Head() != len("中 needle ") {
		t.Fatal("search result did not navigate by UTF-8 byte column", a.view().Head())
	}
	a.showSidebar("Explorer")
	a.showSidebar("Search")
	if len(a.searchResults) != 2 || a.searchQuery != "needle" {
		t.Fatal("results disappeared after switching")
	}
	a.setSearchQuery("[")
	a.startWorkspaceSearch()
	waitUpdates(t, a, func() bool { return !a.searchRunning })
	if a.searchError == "" {
		t.Fatal("invalid regex did not report error")
	}
	a.setSearchQuery("needle")
	a.startWorkspaceSearch()
	a.clearWorkspaceSearch()
	for len(a.updates) > 0 {
		(<-a.updates)()
	}
	if a.searchQuery != "" || len(a.searchResults) != 0 || a.searchRunning {
		t.Fatal("stale search overwrote clear")
	}
}
func TestSearchSidebarKeyboardInputAndFocus(t *testing.T) {
	a, _ := testApp(t)
	a.workspaceSearch()
	a.paste("中👩‍💻é")
	a.key(tcell.NewEventKey(tcell.KeyBackspace, "", 0))
	if a.searchQuery != "中👩‍💻" {
		t.Fatal("search split grapheme")
	}
	a.stopWorkspaceSearch()
	a.cycleFocus()
	if a.focus != "editor" {
		t.Fatal("F6 could not return to editor")
	}
	a.cycleFocus()
	if a.focus != "searchInput" {
		t.Fatal("F6 focused Explorer instead of active Search")
	}
	a.key(tcell.NewEventKey(tcell.KeyCtrlB, "", 0))
	if a.sidebar || a.focus != "editor" {
		t.Fatal("Ctrl+B failed in input")
	}
	a.key(tcell.NewEventKeyEx(tcell.KeyRune, "E", tcell.ModCtrl|tcell.ModShift, true, tcell.KeyE, 1))
	if a.sidebarView != "Explorer" || a.focus != "tree" {
		t.Fatal("Explorer shortcut")
	}
	a.key(tcell.NewEventKeyEx(tcell.KeyRune, "F", tcell.ModCtrl|tcell.ModShift, true, tcell.KeyF, 1))
	if a.sidebarView != "Search" || a.searchQuery != "中👩‍💻" {
		t.Fatal("Search shortcut lost state")
	}
}
func TestGitSidebarGroupsAndTargetsCorrectVersion(t *testing.T) {
	a, _ := testApp(t)
	ctx := context.Background()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.name", "SunBook Test"}, {"config", "user.email", "test@example.invalid"}, {"config", "commit.gpgsign", "false"}, {"config", "core.hooksPath", "/dev/null"}} {
		if _, e := gitservice.Run(ctx, a.root, args...); e != nil {
			t.Fatal(e)
		}
	}
	path := filepath.Join(a.root, "tracked.txt")
	os.WriteFile(path, []byte("base\n"), 0644)
	gitservice.Stage(ctx, a.root, "tracked.txt")
	if e := gitservice.Commit(ctx, a.root, "fixture"); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(path, []byte("staged\n"), 0644)
	gitservice.Stage(ctx, a.root, "tracked.txt")
	os.WriteFile(path, []byte("unstaged\n"), 0644)
	os.WriteFile(filepath.Join(a.root, "fresh.txt"), []byte("new\n"), 0644)
	a.showSidebar("Git")
	waitUpdates(t, a, func() bool { return !a.gitBusy && !a.gitReading })
	if a.gitError != "" || len(a.gitEntries()) != 3 || a.panelVisible {
		t.Fatal("Git sidebar", a.gitError, a.gitEntries())
	}
	var staged, work int
	for i, e := range a.gitEntries() {
		if a.git.Files[e.File].Path == "tracked.txt" {
			if e.Staged {
				staged = i
			} else {
				work = i
			}
		}
	}
	a.gitSelected = staged
	a.selectedGitDiff()
	waitUpdates(t, a, func() bool { return a.comparison != nil })
	body := a.comparison.Pair.Patch
	if !strings.Contains(body, "+staged") || strings.Contains(body, "+unstaged") {
		t.Fatal("staged row opened worktree diff", body)
	}
	a.closeComparison()
	a.gitSelected = work
	a.selectedGitDiff()
	waitUpdates(t, a, func() bool { return a.comparison != nil })
	body = a.comparison.Pair.Patch
	if !strings.Contains(body, "+unstaged") {
		t.Fatal("worktree row opened staged diff", body)
	}
	a.closeComparison()
	a.gitSelected = work
	a.refreshGit() // A background status read must not swallow Stage.
	if !a.gitReading {
		t.Fatal("fixture did not start background refresh")
	}
	a.gitAction(true)
	waitUpdates(t, a, func() bool { return !a.gitBusy && !a.gitReading })
	if len(a.gitEntries()) != 2 {
		t.Fatal("stage did not merge the two versions", a.gitEntries())
	}
	for i, e := range a.gitEntries() {
		if a.git.Files[e.File].Path == "tracked.txt" && e.Staged {
			a.gitSelected = i
		}
	}
	a.gitAction(false)
	waitUpdates(t, a, func() bool { return !a.gitBusy && !a.gitReading })
	for _, e := range a.gitEntries() {
		if e.Staged {
			t.Fatal("unstage did not update sidebar")
		}
	}
	a.draw()
	found := false
	for _, b := range a.buttons {
		if b.Command == "git.commit" {
			found = true
		}
	}
	if !found {
		t.Fatal("Git controls not clickable")
	}
}
