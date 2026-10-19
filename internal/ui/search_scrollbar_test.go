package ui

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/vt"
)

func TestSearchScrollbarTrackDragWheelAndCollapse(t *testing.T) {
	a, _ := testApp(t)
	insertSource(a, "unsaved editor content")
	head, version := a.view().Head(), a.view().Doc.Buffer.Version
	a.workspaceSearch()
	a.searchQuery = "needle"
	for file := 0; file < 8; file++ {
		for line := 0; line < 20; line++ {
			a.searchResults = append(a.searchResults, searchHit{Path: filepath.Join(a.root, fmt.Sprintf("%d.txt", file)), Line: line, Text: "needle", End: 6})
		}
	}
	selected := a.searchSelected
	bar := findContentBar(t, a, "search", 0)
	click(a, bar.X, bar.Y+bar.H-1)
	if a.searchTop == 0 || a.focus != "search" || a.searchSelected != selected || a.view().Head() != head || a.view().Doc.Buffer.Version != version || a.modal != nil {
		t.Fatal("track click failed or opened/changed a result", a.searchTop, a.focus)
	}
	bar = findContentBar(t, a, "search", 0)
	start, _ := scrollbarThumb(bar.H, bar.Total, *bar.Top)
	a.mouse(tcell.NewEventMouse(bar.X, bar.Y+start, tcell.Button1, 0))
	a.draw()
	a.mouse(tcell.NewEventMouse(bar.X+10, bar.Y-100, tcell.Button1, 0))
	a.draw()
	a.mouse(tcell.NewEventMouse(bar.X+10, bar.Y-100, 0, 0))
	if a.searchTop != 0 || a.scrollDrag != nil {
		t.Fatal("drag to top failed")
	}
	bar = findContentBar(t, a, "search", 0)
	a.mouse(tcell.NewEventMouse(bar.X, bar.Y, tcell.Button1, 0))
	a.draw()
	a.mouse(tcell.NewEventMouse(bar.X, bar.Y+1000, tcell.Button1, 0))
	a.draw()
	a.mouse(tcell.NewEventMouse(bar.X, bar.Y+1000, 0, 0))
	if a.searchTop != len(a.searchRows())-bar.H {
		t.Fatal("drag to bottom failed", a.searchTop)
	}
	a.mouse(tcell.NewEventMouse(bar.X-1, bar.Y, tcell.WheelDown, 0))
	a.draw()
	if a.searchTop != len(a.searchRows())-bar.H {
		t.Fatal("wheel overscrolled last page")
	}
	a.searchCollapsed = map[string]bool{}
	for _, h := range a.searchResults {
		a.searchCollapsed[h.Path] = true
	}
	a.draw()
	if a.searchTop != 0 {
		t.Fatal("collapse did not clamp scrolling", a.searchTop)
	}
	for _, bar := range a.scrollbars {
		if bar.Kind == "search" {
			t.Fatal("short list has scrollbar")
		}
	}
}

func TestToolbarFindMoreHelpStayRightAligned(t *testing.T) {
	a, mt := testApp(t)
	insertSource(a, "file contents")
	for _, width := range []int{110, 80, 45, 24} {
		mt.SetSize(vt.Coord{X: vt.Col(width), Y: 32})
		a.screen.Sync()
		a.draw()
		buttons := map[string]button{}
		for _, b := range a.buttons {
			if b.Y == 0 {
				buttons[b.Command] = b
			}
		}
		find, more, help := buttons["search.find"], buttons["app.commands"], buttons["app.help"]
		if find.W == 0 || find.X < 0 || find.X+find.W >= more.X || more.X+more.W >= help.X || help.X+help.W > width {
			t.Fatalf("width %d: invalid Find / More / Help layout: %v %v %v", width, find, more, help)
		}
		for id, b := range buttons {
			if id != "search.find" && id != "app.commands" && id != "app.help" && b.X+b.W >= find.X {
				t.Fatalf("width %d: %s overlaps right actions", width, id)
			}
		}
	}
	mt.SetSize(vt.Coord{X: 110, Y: 32})
	a.screen.Sync()
	a.draw()
	clickCommand(t, a, "search.find")
	if a.modal == nil || a.modal.FindNavigate == nil || a.modal.Title != "Find in File" {
		t.Fatal("right Find does not open current-file find")
	}
}

func TestFindButtonFollowsFileContentFocus(t *testing.T) {
	a, _ := testApp(t)
	visible := func() bool {
		t.Helper()
		a.draw()
		for _, b := range a.buttons {
			if b.Command == "search.find" {
				return true
			}
		}
		return false
	}
	if visible() {
		t.Fatal("Find visible in empty startup editor")
	}
	path := filepath.Join(a.root, "notes.txt")
	writeOpenFixture(t, path, "needle")
	if err := a.openFile(path, false); err != nil {
		t.Fatal(err)
	}
	if !visible() {
		t.Fatal("Find missing in active file")
	}
	for _, focus := range []string{"tree", "filter", "searchInput", "search", "git", "gitMessage", "plugins", "terminal", "problems"} {
		a.focus = focus
		// Preserve non-editor focus during layout for panel-related surfaces.
		a.panelVisible = true
		if visible() {
			t.Fatalf("Find visible with %s focus", focus)
		}
	}
	a.focus = "tree"
	a.draw()
	r := a.rects[a.active]
	click(a, r.X+a.gutter(a.view())+1, r.Y)
	if !visible() {
		t.Fatal("clicking file contents did not show Find")
	}
	clickCommand(t, a, "search.find")
	if !visible() {
		t.Fatal("opening Find hid its button")
	}
	a.key(tcell.NewEventKey(tcell.KeyEsc, "", 0))
	a.focus = "tree"
	if visible() {
		t.Fatal("leaving editor did not hide Find")
	}
}
