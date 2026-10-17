package ui

import (
	"errors"
	"os"
	"time"

	"sunbook/internal/editor"
	"sunbook/internal/text"
)

type filePollState struct {
	disk       editor.DiskState
	lastFull   time.Time
	generation uint64
}
type fileCheckResult struct {
	doc      *editor.Document
	snapshot editor.Document
	buffer   *text.Buffer
	version  int
	poll     filePollState
	fresh    *editor.Document
	changed  bool
	err      error
}

func (a *App) checkFiles() { a.pollFiles(true) }
func (a *App) pollFiles(force bool) {
	if a.fileCheck {
		a.fileCheckForce = a.fileCheckForce || force
		return
	}
	a.fileCheck = true
	now := time.Now()
	checks := make([]fileCheckResult, 0, len(a.docs))
	for _, d := range a.docs {
		if d.Path == "" {
			continue
		}
		previous, ok := a.fileStates[d]
		if !ok || previous.generation != d.SaveGeneration() {
			previous = filePollState{disk: d.DiskState(), lastFull: now, generation: d.SaveGeneration()}
		}
		checks = append(checks, fileCheckResult{doc: d, snapshot: *d, buffer: d.Buffer, version: d.Buffer.Version, poll: previous})
	}
	go func() {
		for i := range checks {
			c := &checks[i]
			full := force || now.Sub(c.poll.lastFull) >= 30*time.Second
			state, err := editor.InspectDisk(c.snapshot.Path, c.poll.disk, full)
			if full {
				c.poll.lastFull = now
			}
			c.poll.disk = state
			if errors.Is(err, os.ErrNotExist) {
				c.changed = c.snapshot.Exists
			} else if err != nil {
				c.err = err
				continue
			} else {
				c.changed = !c.snapshot.MatchesDisk(state)
			}
			if !c.changed {
				continue
			}
			c.fresh, c.err = editor.Open(c.snapshot.Path)
			if c.err != nil {
				continue
			}
			// A second metadata check validates the reload entirely off the UI thread.
			final, err := editor.InspectDisk(c.snapshot.Path, state, false)
			if errors.Is(err, os.ErrNotExist) && !c.fresh.Exists {
				continue
			}
			if err != nil || !state.SameMetadata(final) || !c.fresh.MatchesDisk(state) {
				c.err = editor.ErrConflict
			}
		}
		a.postQuiet(func() {
			a.fileCheck = false
			if a.fileStates == nil {
				a.fileStates = make(map[*editor.Document]filePollState)
			}
			for _, c := range checks {
				if c.doc.Buffer != c.buffer || c.doc.Buffer.Version != c.version || c.doc.Path != c.snapshot.Path || c.doc.SaveGeneration() != c.snapshot.SaveGeneration() {
					continue
				}
				open := false
				for _, d := range a.docs {
					if d == c.doc {
						open = true
						break
					}
				}
				if !open {
					continue
				}
				if c.err != nil {
					// Concurrent disk writers are retried next poll; don't attach a hash to
					// metadata that may belong to another version of the file.
					if !errors.Is(c.err, editor.ErrConflict) {
						a.notify("External change: " + c.err.Error())
					}
					continue
				}
				a.fileStates[c.doc] = c.poll
				if !c.changed {
					continue
				}
				if c.doc.Dirty() {
					a.notify(c.doc.Name() + " changed on disk. Save will offer conflict resolution.")
					continue
				}
				if !c.fresh.Exists {
					a.notify(c.doc.Name() + " was removed on disk")
					continue
				}
				*c.doc = *c.fresh
				c.doc.Buffer.Version = c.version + 1
				// A fresh document starts a new storage/history lineage; don't reuse polls
				// from the previous buffer, even when its save generation happens to match.
				delete(a.fileStates, c.doc)
				for _, cache := range a.viewCache {
					if v := cache[c.doc]; v != nil {
						v.Clamp()
					}
				}
				a.changed(c.doc)
				a.notify(c.doc.Name() + " reloaded")
			}
			if a.fileCheckForce {
				a.fileCheckForce = false
				a.pollFiles(true)
			}
		})
	}()
}
