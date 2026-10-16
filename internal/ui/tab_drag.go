package ui

import (
	"sunbook/internal/editor"
)

type tabDragState struct {
	Doc, Previous  *editor.Document
	StartX, StartY int
	SourceGroup    int
	Moving         bool
	Slot, Group    int
	Edge           string
	Preview        rect
}

func (a *App) moveTabDrag(x, y int, pressed bool) {
	d := a.tabDrag
	if d == nil {
		return
	}
	if !d.Moving && abs(x-d.StartX)+abs(y-d.StartY) >= 2 {
		a.keepDocument(d.Doc)
		a.fileClick = fileClick{}
		d.Moving = true
	}
	if !d.Moving {
		if !pressed {
			a.tabDrag = nil
		}
		return
	}
	d.Slot, d.Group, d.Edge, d.Preview = -1, -1, "", rect{}
	if a.editorArea.contains(x, y) && y == 1 {
		d.Slot = len(a.docs)
		caret := a.editorArea.X + a.editorArea.W - 1
		for _, tab := range a.tabs {
			index := len(a.docs)
			if tab.Doc != nil {
				for i, doc := range a.docs {
					if doc == tab.Doc {
						index = i
						break
					}
				}
			}
			if x < tab.X+tab.W/2 {
				d.Slot = index
				caret = tab.X
				break
			}
			if tab.Doc != nil {
				d.Slot = index + 1
				caret = min(a.editorArea.X+a.editorArea.W-1, tab.X+tab.W-1)
			}
		}
		d.Preview = rect{caret, 1, 1, 1}
	} else {
		for i, r := range a.rects {
			if r.W == 0 || !r.contains(x, y) {
				continue
			}
			if a.rects[1].W > 0 {
				d.Group = i
				d.Preview = r
				break
			}
			if r.W < 30 {
				break
			}
			half := (r.W - 1) / 2
			if x < r.X+r.W/4 {
				d.Edge = "left"
				d.Preview = rect{r.X, r.Y, half, r.H}
			}
			if x >= r.X+r.W-r.W/4 {
				d.Edge = "right"
				d.Preview = rect{r.X + half + 1, r.Y, r.W - half - 1, r.H}
			}
			break
		}
	}
	a.hoverHint = "Drag a tab to reorder · drop on an editor edge to split · Esc cancels"
	if pressed {
		return
	}
	a.tabDrag = nil
	a.hoverHint = ""
	if d.Slot >= 0 {
		a.reorderDocument(d.Doc, d.Slot)
		return
	}
	if d.Group >= 0 {
		if d.Group != d.SourceGroup && d.Previous != nil {
			a.active = d.SourceGroup
			a.activate(d.Previous)
		}
		a.active = d.Group
		a.activate(d.Doc)
		a.reveal()
		return
	}
	if d.Edge != "" {
		other := d.Previous
		if other == nil || other == d.Doc {
			for _, doc := range a.docs {
				if doc != d.Doc {
					other = doc
					break
				}
			}
		}
		if !a.split {
			a.splitEditor()
		}
		target := 0
		if d.Edge == "right" {
			target = 1
		}
		if other != nil && other != d.Doc {
			a.active = 1 - target
			a.activate(other)
		}
		a.active = target
		a.activate(d.Doc)
		a.reveal()
	}
}
func (a *App) reorderDocument(doc *editor.Document, slot int) {
	from := -1
	for i, d := range a.docs {
		if d == doc {
			from = i
			break
		}
	}
	if from < 0 {
		return
	}
	slot = max(0, min(slot, len(a.docs)))
	if slot > from {
		slot--
	}
	if slot == from {
		return
	}
	a.docs = append(a.docs[:from], a.docs[from+1:]...)
	a.docs = append(a.docs, nil)
	copy(a.docs[slot+1:], a.docs[slot:len(a.docs)-1])
	a.docs[slot] = doc
}
func (a *App) drawTabDrop() {
	d := a.tabDrag
	if d == nil || !d.Moving || d.Preview.W <= 0 || d.Preview.H <= 0 {
		return
	}
	r := d.Preview
	st := a.accent.Background(a.colors.inset).Bold(true)
	if d.Slot >= 0 {
		a.put(r.X, r.Y, 1, "┃", st)
		return
	}
	for y := r.Y; y < r.Y+r.H; y++ {
		a.put(r.X, y, 1, "│", st.Bold(false))
		a.put(r.X+r.W-1, y, 1, "│", st.Bold(false))
	}
	a.put(r.X, r.Y, r.W, " Drop to open in split ", st)
}
