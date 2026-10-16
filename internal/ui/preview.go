package ui

import (
	"time"

	"sunbook/internal/editor"
)

// Tabs are shared between editor groups, so there is one reusable preview.
// Opening it in another visible group keeps it open before either group can
// replace it with a different file.
func (a *App) keepDocument(d *editor.Document) {
	if a.preview == d {
		a.preview = nil
	}
}

type fileClick struct {
	source, target string
	x, y           int
	serial         uint64
	when           time.Time
}

func (a *App) doubleFileClick(source, target string, x, y int) bool {
	previous := a.fileClick
	now := time.Now()
	a.fileClick = fileClick{source, target, x, y, a.mouseClicks, now}
	if previous.source == source && previous.target == target &&
		previous.serial+1 == a.mouseClicks && now.Sub(previous.when) < 400*time.Millisecond &&
		abs(previous.x-x) <= 1 && previous.y == y {
		a.fileClick = fileClick{}
		return true
	}
	return false
}
