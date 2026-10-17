package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/vt"
	gitservice "sunbook/internal/git"
	"sunbook/internal/text"
)

func TestGitGroupsFoldWithMouseKeyboardAndKeepCounts(t *testing.T) {
	a, _ := testApp(t)
	a.sidebarView, a.focus = "Git", "git"
	a.git = gitservice.Status{Root: a.root, Branch: "main", Files: []gitservice.File{
		{Path: "staged.go", Index: "M", Worktree: " "},
		{Path: "tracked.go", Index: " ", Worktree: "M"},
		{Path: "one.md", Index: "?", Worktree: "?"},
		{Path: "two.md", Index: "?", Worktree: "?"},
	}}
	counts := func() (int, int, int) { return a.gitCounts() }
	staged, changed, conflicts := counts()
	clickCommand(t, a, "git.toggleCHANGES")
	if !a.gitCollapsed["CHANGES"] || a.gitGroupFocus != "CHANGES" {
		t.Fatal("Changes header did not collapse")
	}
	for _, row := range a.gitRows() {
		if !row.Heading && row.Group == "CHANGES" {
			t.Fatal("collapsed file still visible")
		}
	}
	if _, ok := a.selectedGitEntry(); ok || a.buttonEnabled("git.stage") || a.buttonEnabled("git.selectedDiff") {
		t.Fatal("collapsed header targets a hidden file")
	}
	s, c, f := counts()
	if s != staged || c != changed || f != conflicts {
		t.Fatal("folding changed repository counts")
	}
	a.key(tcell.NewEventKey(tcell.KeyDown, "", 0))
	if a.gitGroupFocus != "UNTRACKED" {
		t.Fatal("navigation entered hidden group contents")
	}
	a.key(tcell.NewEventKey(tcell.KeyEnter, "", 0))
	if !a.gitCollapsed["UNTRACKED"] {
		t.Fatal("Enter did not collapse Untracked")
	}
	a.key(tcell.NewEventKey(tcell.KeyRight, "", 0))
	if a.gitCollapsed["UNTRACKED"] {
		t.Fatal("Right did not expand")
	}
	a.key(tcell.NewEventKey(tcell.KeyRight, "", 0))
	entry, ok := a.selectedGitEntry()
	if !ok || a.git.Files[entry.File].Path != "one.md" {
		t.Fatal("Right did not enter visible files")
	}
	a.key(tcell.NewEventKey(tcell.KeyLeft, "", 0))
	if a.gitGroupFocus != "UNTRACKED" {
		t.Fatal("Left did not select parent")
	}
	a.key(tcell.NewEventKey(tcell.KeyLeft, "", 0))
	if !a.gitCollapsed["UNTRACKED"] {
		t.Fatal("Left did not collapse parent")
	}
	// Filtering retains fold choices and displays matched/total counts.
	a.setGitFilter("two")
	rows := a.gitRows()
	if len(rows) != 1 || rows[0].Label != "UNTRACKED 1/2" {
		t.Fatal("filter lost collapsed heading counts", rows)
	}
	a.execute("git.toggleUNTRACKED")
	if a.gitFilter != "two" || len(a.gitRows()) != 2 {
		t.Fatal("expanding cleared the filter")
	}
	a.setGitFilter("")
	if !a.gitCollapsed["CHANGES"] {
		t.Fatal("filtering lost another group's fold state")
	}
	a.sidebarView = "Explorer"
	a.draw()
	a.sidebarView = "Git"
	a.draw()
	if !a.gitCollapsed["CHANGES"] {
		t.Fatal("switching sidebars reset groups")
	}
}

func TestGitComposerWrapsAndEditsMultilineUnicode(t *testing.T) {
	a, mt := testApp(t)
	a.sidebarView = "Git"
	a.focusGitMessage()
	a.git.Root = a.root
	a.draw()
	if a.gitMessageRect.H != 3 {
		t.Fatal("large sidebar did not get a multiline composer")
	}
	a.paste("fix: 中文 👩‍💻 e\u0301\r\n\r\nMore detail")
	if a.gitMessage != "fix: 中文 👩‍💻 e\u0301\n\nMore detail" {
		t.Fatal("paste lost line breaks", a.gitMessage)
	}
	a.key(tcell.NewEventKey(tcell.KeyCtrlJ, "", 0))
	a.paste("最后一行")
	if !strings.HasSuffix(a.gitMessage, "More detail\n最后一行") {
		t.Fatal("Ctrl+J did not insert a newline")
	}
	a.draw()
	lines := gitMessageLines(a.gitMessage, a.gitMessageRect.W)
	row := lines[a.gitMessageTop+1]
	click(a, a.gitMessageRect.X, a.gitMessageRect.Y+1)
	if a.gitMessageCursor != row.Start {
		t.Fatal("mouse ignored composer row")
	}
	a.gitMessageCursor = len("fix: 中文 👩‍💻")
	a.key(tcell.NewEventKey(tcell.KeyBackspace, "", 0))
	if strings.Contains(a.gitMessage, "👩") || strings.Contains(a.gitMessage, "💻") {
		t.Fatal("backspace split an emoji")
	}
	// Cursor and draft survive compact height and soft wrapping after resize.
	a.gitMessageCursor = len(a.gitMessage)
	before := a.gitMessage
	for _, size := range []vt.Coord{{X: 60, Y: 14}, {X: 110, Y: 32}} {
		mt.SetSize(size)
		a.screen.Sync()
		a.draw()
		lines = gitMessageLines(a.gitMessage, a.gitMessageRect.W)
		cursorRow := gitMessageRow(lines, a.gitMessageCursor)
		if cursorRow < a.gitMessageTop || cursorRow >= a.gitMessageTop+a.gitMessageRect.H {
			t.Fatal("resize hid message cursor")
		}
		if a.gitMessage != before || text.Snap(a.gitMessage, a.gitMessageCursor) != a.gitMessageCursor {
			t.Fatal("resize modified draft or grapheme boundary")
		}
	}
	a.focusGitFilter()
	a.paste("one\ntwo")
	if a.gitFilter != "one two" {
		t.Fatal("path filter accidentally became multiline")
	}
}
