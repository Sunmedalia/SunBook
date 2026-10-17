package ui

import (
	"time"

	"sunbook/internal/editor"
	"sunbook/internal/text"
)

func (a *App) scheduleMaintenance(d *editor.Document) {
	if !d.Buffer.NeedsCompaction() {
		return
	}
	if a.maintenanceDue == nil {
		a.maintenanceDue = make(map[*editor.Document]time.Time)
	}
	a.maintenanceDue[d] = time.Now().Add(time.Second)
}
func (a *App) flushMaintenance(now time.Time) {
	for d, due := range a.maintenanceDue {
		if now.Before(due) || a.maintenanceRunning[d.Buffer] {
			continue
		}
		delete(a.maintenanceDue, d)
		b := d.Buffer
		task := b.CompactionTask()
		if task == nil {
			continue
		}
		if a.maintenanceRunning == nil {
			a.maintenanceRunning = make(map[*text.Buffer]bool)
		}
		a.maintenanceRunning[b] = true
		go func() {
			result := task()
			a.postQuiet(func() {
				delete(a.maintenanceRunning, b)
				if d.Buffer != b {
					return
				}
				open := false
				for _, doc := range a.docs {
					if doc == d {
						open = true
						break
					}
				}
				if open && !b.InstallCompaction(result) {
					b.RetryCompaction()
					a.scheduleMaintenance(d)
				}
			})
		}()
	}
}
