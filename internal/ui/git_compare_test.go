package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v3"
	gitservice "sunbook/internal/git"
)

func TestComparisonAlignsInsertionsAndPreservesEditor(t *testing.T) {
	a := gitTestWorkspace(t)
	ctx := context.Background()
	path := filepath.Join(a.root, "file.txt")
	os.WriteFile(path, []byte("first\nold\nkeep\nend\n"), 0644)
	gitservice.StageAll(ctx, a.root)
	if e := gitservice.Commit(ctx, a.root, "base"); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(path, []byte("first\nnew\ninserted\nkeep\nend\n"), 0644)
	a.typeInput("unsaved original 👩‍💻")
	original := a.view().Doc
	a.splitEditor()
	head := a.view().Head()
	a.showSidebar("Git")
	waitUpdates(t, a, func() bool { return !a.gitReading })
	a.draw()
	click(a, a.sideHits[0].X+2, a.sideHits[0].Y)
	waitUpdates(t, a, func() bool { return a.comparing() })
	c := a.comparison
	if a.modal != nil || len(a.docs) != 1 || a.view().Doc != original {
		t.Fatal("comparison replaced editor or opened overlay")
	}
	var left, right []string
	aligned := false
	gap := false
	for _, r := range c.Rows {
		if r.OldNumber > 0 {
			left = append(left, r.Old)
		}
		if r.NewNumber > 0 {
			right = append(right, r.New)
		}
		if r.Old == "keep" && r.New == "keep" {
			aligned = true
		}
		if r.New == "inserted" && r.OldNumber == 0 {
			gap = true
		}
	}
	if strings.Join(left, "\n") != "first\nold\nkeep\nend" || strings.Join(right, "\n") != "first\nnew\ninserted\nkeep\nend" || !aligned || !gap || c.Added != 2 || c.Removed != 1 {
		t.Fatal("unaligned comparison", c.Rows)
	}
	a.draw()
	divider, y := c.Rects[1].X-1, c.Rects[0].Y+1
	a.mouse(tcell.NewEventMouse(divider, y, tcell.Button1, 0))
	a.mouse(tcell.NewEventMouse(divider+6, y, tcell.Button1, 0))
	a.mouse(tcell.NewEventMouse(divider+6, y, 0, 0))
	if c.Ratio <= .5 || a.splitRatio != .5 {
		t.Fatal("comparison divider altered regular editor split")
	}
	a.draw()
	click(a, c.Rects[1].X+5, c.Rects[1].Y)
	a.key(tcell.NewEventKey(tcell.KeyRune, "x", 0))
	a.paste("do not insert")
	a.key(tcell.NewEventKey(tcell.KeyCtrlS, "", 0))
	a.execute("edit.selectAll")
	if original.Buffer.String() != "unsaved original 👩‍💻" || a.view().Head() != head || a.modal != nil {
		t.Fatal("read-only comparison edited or saved original document")
	}
	a.key(tcell.NewEventKey(tcell.KeyCtrlW, "", 0))
	if a.comparison != nil || !a.split || a.views[0].Doc != original || a.views[1].Doc != original || !original.Dirty() {
		t.Fatal("closing diff lost editor layout or draft")
	}
}
func TestGitRefreshDoesNotChangeRenderedState(t *testing.T) {
	a := gitTestWorkspace(t)
	a.showSidebar("Git")
	waitUpdates(t, a, func() bool { return !a.gitReading && !a.gitLogReading })
	a.draw()
	w, h := a.screen.Size()
	snapshot := func() []string {
		var cells []string
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				s, style, _ := a.screen.Get(x, y)
				cells = append(cells, fmt.Sprintf("%s %v", s, style))
			}
		}
		return cells
	}
	before := snapshot()
	message := a.message
	a.refreshGit()
	if !a.gitReading {
		t.Fatal("refresh did not start")
	}
	a.draw()
	after := snapshot()
	if strings.Join(before, "|") != strings.Join(after, "|") || a.message != message {
		t.Fatal("refresh displayed progress or changed chrome")
	}
	waitUpdates(t, a, func() bool { return !a.gitReading })
}

func TestInlineGitLogScrollAndSelectionSurviveRefresh(t *testing.T) {
	a := gitTestWorkspace(t)
	ctx := context.Background()
	path := filepath.Join(a.root, "file.txt")
	for i := 0; i < 4; i++ {
		os.WriteFile(path, []byte(fmt.Sprintf("version %d\n", i)), 0644)
		gitservice.StageAll(ctx, a.root)
		if e := gitservice.Commit(ctx, a.root, fmt.Sprintf("commit %d", i)); e != nil {
			t.Fatal(e)
		}
	}
	os.WriteFile(path, []byte("working\n"), 0644)
	a.showSidebar("Git")
	waitUpdates(t, a, func() bool { return !a.gitReading && !a.gitLogReading })
	a.draw()
	if len(a.gitLog) != 4 || a.gitLogRect.H == 0 || a.modal != nil {
		t.Fatal("Git log was not displayed inline")
	}
	a.gitTop = 1
	a.mouse(tcell.NewEventMouse(a.gitLogRect.X+2, a.gitLogRect.Y, tcell.WheelDown, 0))
	a.mouse(tcell.NewEventMouse(a.gitLogRect.X+2, a.gitLogRect.Y, 0, 0))
	if a.gitLogTop != 3 || a.gitTop != 1 {
		t.Fatal("log scroll affected changes", a.gitLogTop, a.gitTop)
	}
	anchor := a.gitLog[a.gitLogTop].Hash
	a.gitLogSelected = 2
	selected := a.gitLog[2].Hash
	gitservice.StageAll(ctx, a.root)
	if e := gitservice.Commit(ctx, a.root, "new head"); e != nil {
		t.Fatal(e)
	}
	a.refreshGit()
	waitUpdates(t, a, func() bool { return !a.gitReading && !a.gitLogReading })
	if a.gitLog[0].Subject != "new head" || a.gitLog[a.gitLogSelected].Hash != selected || a.gitLog[a.gitLogTop].Hash != anchor {
		t.Fatal("log refresh jumped selection or viewport", a.gitLogSelected, a.gitLogTop)
	}
	a.draw()
	click(a, a.gitLogRect.X+2, a.gitLogRect.Y)
	waitUpdates(t, a, func() bool { return a.modal != nil })
	if !strings.Contains(a.modal.Title, anchor) {
		t.Fatal("clicked log opened wrong commit", a.modal.Title)
	}
}

func TestComparisonRowsReconstructEmptyFilesAndSeparateHunks(t *testing.T) {
	a := gitTestWorkspace(t)
	ctx := context.Background()
	path := filepath.Join(a.root, "case.txt")
	for _, pair := range [][2]string{
		{"", "new\n"}, {"old\n", ""}, {"same\n", "same\n"}, {"no newline", "no newline\n"},
		{"head\n" + strings.Repeat("stable\n", 30) + "tail\n", "changed head\n" + strings.Repeat("stable\n", 30) + "changed tail\n"},
		{"中\t👩‍💻\n\nend\n", "中\t👩‍💻\ninsert\n\nend\n"},
	} {
		os.WriteFile(path, []byte(pair[0]), 0644)
		if e := gitservice.Stage(ctx, a.root, "case.txt"); e != nil {
			t.Fatal(e)
		}
		os.WriteFile(path, []byte(pair[1]), 0644)
		snapshot, e := gitservice.Compare(ctx, a.root, gitservice.File{Path: "case.txt", Index: "A", Worktree: "M"}, false)
		if e != nil {
			t.Fatal(e)
		}
		view := newGitComparison(snapshot)
		var old, new []string
		for _, row := range view.Rows {
			if row.OldNumber > 0 {
				old = append(old, row.Old)
			}
			if row.NewNumber > 0 {
				new = append(new, row.New)
			}
		}
		if strings.Join(old, "\n") != strings.Join(comparisonLines(pair[0]), "\n") || strings.Join(new, "\n") != strings.Join(comparisonLines(pair[1]), "\n") {
			t.Fatal("diff rows lost or duplicated file content", pair, view.Rows)
		}
	}
}
