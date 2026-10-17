package ui

import "time"

const frameInterval = 16 * time.Millisecond

func (a *App) invalidate() { a.repaint = true }

// Deadlines share one timer. Periodic maintenance does not itself request paint.
func (a *App) nextWake(now time.Time) time.Time {
	var due time.Time
	add := func(t time.Time) {
		if !t.IsZero() && (due.IsZero() || t.Before(due)) {
			due = t
		}
	}
	add(a.nextDisk)
	add(a.nextPluginScan)
	for d, t := range a.maintenanceDue {
		if !a.maintenanceRunning[d.Buffer] {
			add(t)
		}
	}
	add(a.searchDue)
	add(a.completionDue)
	if a.message != "" {
		add(a.messageUntil)
	}
	if a.pressedCommand != "" {
		add(a.pressedUntil)
	}
	for d, t := range a.highlightDue {
		if !a.highlightRunning[d] {
			add(t)
		}
	}
	if a.repaint {
		add(maxTime(now, a.lastDraw.Add(frameInterval)))
	}
	if due.IsZero() {
		return now.Add(2 * time.Second)
	}
	return due
}
func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func (a *App) runScheduled(now time.Time) {
	if !a.nextPluginScan.IsZero() && !now.Before(a.nextPluginScan) {
		a.nextPluginScan = now.Add(30 * time.Second)
		a.refreshPlugins()
	}
	if !a.nextDisk.IsZero() && !now.Before(a.nextDisk) {
		a.nextDisk = now.Add(2 * time.Second)
		a.requestTerminalColors()
		a.refreshTerminalTheme()
		a.pollFiles(false)
		a.refreshGit()
	}
	a.flushHighlights()
	a.flushMaintenance(now)
	if !a.searchDue.IsZero() && !now.Before(a.searchDue) {
		a.startWorkspaceSearch()
	}
	if !a.completionDue.IsZero() && !now.Before(a.completionDue) {
		a.completionDue = time.Time{}
		a.requestCompletion(false)
	}
	if a.message != "" && !now.Before(a.messageUntil) {
		a.message = ""
		a.invalidate()
	}
	if a.pressedCommand != "" && !now.Before(a.pressedUntil) {
		a.pressedCommand = ""
		a.invalidate()
	}
}
func (a *App) paintIfDue(now time.Time) bool {
	if !a.repaint || now.Before(a.lastDraw.Add(frameInterval)) {
		return false
	}
	a.draw()
	a.repaint = false
	a.lastDraw = now
	return true
}

// Quiet posts are used by polling workers; they explicitly invalidate only when
// their result changes visible state. All callbacks still run on the UI thread.
func (a *App) postQuiet(f func()) {
	select {
	case a.updates <- f:
	case <-a.ctx.Done():
	}
}
