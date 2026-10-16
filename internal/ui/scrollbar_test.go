package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v3"
	gitservice "sunbook/internal/git"
)

func findContentBar(t *testing.T, a *App, kind string, group int) contentScrollbar {
	t.Helper()
	a.draw()
	for _, bar := range a.scrollbars {
		if bar.Kind == kind && bar.Group == group {
			return bar
		}
	}
	t.Fatal("content scrollbar missing", kind, group)
	return contentScrollbar{}
}

func TestEditorScrollbarClickAndDragPreserveDocumentAndSelection(t *testing.T) {
	a, _ := testApp(t)
	source := strings.Repeat("中 👩‍💻 é content\n", 150)
	a.typeInput(source)
	v := a.view()
	v.Set(4, false)
	v.Top = 0
	head, version := v.Head(), v.Doc.Buffer.Version
	bar := findContentBar(t, a, "editor", 0)
	if bar.W != 1 || bar.X != a.rects[0].X+a.rects[0].W-1 {
		t.Fatal("scrollbar did not use exactly the last column")
	}
	click(a, bar.X, bar.Y+bar.H-1)
	if v.Top == 0 || v.Head() != head || v.Doc.Buffer.Version != version {
		t.Fatal("clicking track did not scroll, or changed the selection")
	}
	bar = findContentBar(t, a, "editor", 0)
	start, _ := scrollbarThumb(bar.H, bar.Total, *bar.Top)
	a.mouse(tcell.NewEventMouse(bar.X, bar.Y+start, tcell.Button1, 0))
	a.draw()
	a.mouse(tcell.NewEventMouse(bar.X, bar.Y-100, tcell.Button1, 0))
	a.draw()
	a.mouse(tcell.NewEventMouse(bar.X, bar.Y-100, 0, 0))
	if v.Top != 0 || a.scrollDrag != nil {
		t.Fatal("drag to start did not clamp and release")
	}
	bar = findContentBar(t, a, "editor", 0)
	a.mouse(tcell.NewEventMouse(bar.X, bar.Y, tcell.Button1, 0))
	a.draw()
	a.mouse(tcell.NewEventMouse(bar.X, bar.Y+1000, tcell.Button1, 0))
	a.draw()
	a.mouse(tcell.NewEventMouse(bar.X, bar.Y+1000, 0, 0))
	if v.Top != v.Doc.Buffer.LineCount()-bar.H || v.Doc.Buffer.String() != source || v.Head() != head {
		t.Fatal("drag to end corrupted the document or scrolled past its bounds")
	}
	insertSource(a, "short")
	a.draw()
	if len(a.scrollbars) != 0 {
		t.Fatal("short document has an unnecessary scrollbar")
	}
}

func TestScrollbarsRespectSplitGroupsAndSynchronizedComparison(t *testing.T) {
	a, _ := testApp(t)
	a.typeInput(strings.Repeat("line\n", 120))
	a.splitEditor()
	a.views[0].Top, a.views[1].Top = 0, 0
	right := findContentBar(t, a, "editor", 1)
	click(a, right.X, right.Y+right.H-1)
	if a.views[0].Top != 0 || a.views[1].Top == 0 || a.active != 1 {
		t.Fatal("scrolling one split moved the other split")
	}
	before := a.views[1].Top
	a.comparison = newGitComparison(gitservice.Comparison{Path: "file.txt", Old: strings.Repeat("old\n", 150), New: strings.Repeat("new\n", 150)})
	right = findContentBar(t, a, "diff", 1)
	click(a, right.X, right.Y+right.H-1)
	if a.comparison.Top == 0 || a.comparison.Pane != 1 || a.views[1].Top != before {
		t.Fatal("comparison scrollbar altered the editor view")
	}
	left := findContentBar(t, a, "diff", 0)
	if left.Top != right.Top || *left.Top != a.comparison.Top {
		t.Fatal("comparison sides do not share scrolling")
	}
}

func TestTextDialogScrollbarDoesNotScrollUnderlyingEditor(t *testing.T) {
	a, _ := testApp(t)
	a.typeInput(strings.Repeat("editor\n", 120))
	before := a.view().Top
	a.showText("Details", strings.Repeat("detail\n", 200))
	bar := findContentBar(t, a, "modal", 0)
	click(a, bar.X, bar.Y+bar.H-1)
	if a.modal.Top == 0 || a.view().Top != before {
		t.Fatal("dialog scrollbar moved the underlying editor")
	}
}

func TestSplitButtonKeepsItsTextLabel(t *testing.T) {
	a, _ := testApp(t)
	for _, button := range a.buttons {
		if button.Command == "view.split" {
			var label strings.Builder
			for x := button.X; x < button.X+button.W; x++ {
				s, _, _ := a.screen.Get(x, button.Y)
				label.WriteString(s)
			}
			if !strings.Contains(label.String(), "Split") {
				t.Fatal("Split button has only an icon", label.String())
			}
			return
		}
	}
	t.Fatal("Split button is not visible")
}
