package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v3"
)

func TestFindLiveCompactNavigationAndRepeat(t *testing.T) {
	a, _ := testApp(t)
	insertSource(a, "first needle\n中文 needle\nlast")
	a.focus = "tree"
	a.find(false)
	a.paste("needle")
	a.draw()
	assertMatch := func(want string, start int) {
		t.Helper()
		lo, hi := a.view().Selections[0].Range()
		if a.view().Doc.Buffer.String()[lo:hi] != want || lo != start {
			t.Fatalf("selection %d:%d", lo, hi)
		}
	}
	assertMatch("needle", 6)
	row, _ := a.view().Doc.Buffer.Position(a.view().Head())
	matchY := a.rects[a.active].Y + row - a.view().Top
	if matchY >= a.modalRect.Y && matchY < a.modalRect.Y+a.modalRect.H {
		t.Fatal("find covers its live match")
	}
	if a.modalRect.H != 4 || a.modalRect.W > 64 || !strings.Contains(a.modal.Hint, "1/2") || a.focus != "editor" {
		t.Fatal("find layout/state", a.modalRect, a.modal.Hint, a.focus)
	}
	a.key(tcell.NewEventKey(tcell.KeyEnter, "", 0))
	assertMatch("needle", len("first needle\n中文 "))
	if a.modal == nil || !strings.Contains(a.modal.Hint, "2/2") {
		t.Fatal("find closed or wrong count")
	}
	a.key(tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModShift))
	assertMatch("needle", 6)
	a.key(tcell.NewEventKey(tcell.KeyEsc, "", 0))
	a.key(tcell.NewEventKey(tcell.KeyF3, "", 0))
	assertMatch("needle", len("first needle\n中文 "))
	a.key(tcell.NewEventKey(tcell.KeyF3, "", tcell.ModShift))
	assertMatch("needle", 6)
	a.key(tcell.NewEventKey(tcell.KeyF3, "", tcell.ModShift))
	assertMatch("needle", len("first needle\n中文 "))
}

func TestFindNoMatchEmptyAndCombiningGrapheme(t *testing.T) {
	a, _ := testApp(t)
	insertSource(a, "é\nneedle")
	a.find(false)
	a.paste("missing")
	if !strings.Contains(a.modal.Hint, "No matches") {
		t.Fatal(a.modal.Hint)
	}
	a.key(tcell.NewEventKey(tcell.KeyCtrlU, "", 0))
	if !strings.Contains(a.modal.Hint, "Type to find") {
		t.Fatal(a.modal.Hint)
	}
	a.paste("e")
	lo, hi := a.view().Selections[0].Range()
	if a.view().Doc.Buffer.String()[lo:hi] != "é" {
		t.Fatal("split combining grapheme")
	}
	a.key(tcell.NewEventKey(tcell.KeyEsc, "", 0))
	if a.findNext("") {
		t.Fatal("empty find matched")
	}
}

func TestFindWrapIncludesMatchSpanningOriginalCursor(t *testing.T) {
	a, _ := testApp(t)
	insertSource(a, "needle tail")
	a.view().Set(3, false)
	if !a.findNext("needle") || a.view().Head() != 6 {
		t.Fatal("wrap lost a match crossing the original cursor")
	}
}

func TestTextPromptsUseCompactLayout(t *testing.T) {
	a, _ := testApp(t)
	a.prompt("Replace With", "", func(string) {})
	a.draw()
	if a.modalRect.H != 4 || a.modalRect.W > 64 {
		t.Fatal(a.modalRect)
	}
}
