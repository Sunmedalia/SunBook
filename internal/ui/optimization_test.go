package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sunbook/internal/lsp"
	"sunbook/internal/text"
	"testing"
	"time"
)

func TestTreeLimitAndCancellation(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"z.txt", "a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "dir"), 0700); err != nil {
		t.Fatal(err)
	}
	entries, truncated := readTree(context.Background(), root, nil, 3)
	if len(entries) != 3 || !truncated || entries[0].Name != "dir" || entries[1].Name != "a.txt" {
		t.Fatal(entries, truncated)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	entries, truncated = readTree(ctx, root, nil, 3)
	if len(entries) != 0 || truncated {
		t.Fatal("cancelled scan returned entries")
	}
}

func TestPaletteUsesEffectiveBindings(t *testing.T) {
	a, _ := testApp(t)
	a.bindings["ctrl+s"] = ""
	a.bindings["f8"] = "file.save"
	a.bindings["f9"] = "file.save"
	a.palette()
	for _, it := range a.modal.Items {
		if it.Detail == "f8 / f9" {
			return
		}
	}
	t.Fatal("palette did not show configured save bindings")
}

func TestSchedulerIdleFramesAndExpiry(t *testing.T) {
	a, _ := testApp(t)
	now := time.Now()
	a.repaint = false
	a.lastDraw = now
	if a.paintIfDue(now.Add(time.Hour)) {
		t.Fatal("idle UI painted")
	}
	a.invalidate()
	if a.paintIfDue(now.Add(frameInterval - time.Nanosecond)) {
		t.Fatal("painted before frame deadline")
	}
	if !a.nextWake(now).Equal(now.Add(frameInterval)) {
		t.Fatal("wrong frame deadline")
	}
	if !a.paintIfDue(now.Add(frameInterval)) || a.paintIfDue(now.Add(frameInterval)) {
		t.Fatal("frame was lost or painted twice")
	}
	a.message, a.messageUntil = "notice", now.Add(time.Second)
	a.pressedCommand, a.pressedUntil = "file.save", now.Add(time.Second)
	a.runScheduled(now.Add(time.Second))
	if a.message != "" || a.pressedCommand != "" || !a.repaint {
		t.Fatal("timed feedback did not expire")
	}
	a.repaint = false
	a.runScheduled(now.Add(2 * time.Second))
	if a.repaint {
		t.Fatal("maintenance without changes repainted")
	}
}

func TestDiagnosticCacheVersionsRowsAndBatches(t *testing.T) {
	a, _ := testApp(t)
	d := a.view().Doc
	d.Path = filepath.Join(a.root, "example.go")
	d.Buffer = text.New("中😀abc\nsecond\nlast")
	uri := lsp.URI(d.Path)
	a.diagnostics[uri] = lsp.Diagnostics{Diagnostics: []lsp.Diagnostic{{Range: lsp.Range{Start: lsp.Position{Line: 0, Character: 1}, End: lsp.Position{Line: 1, Character: 3}}}}}
	c := a.cachedDiagnostics(d)
	if !c.row(0).marker || c.row(1).marker || len(c.row(1).spans) != 1 || len(c.row(2).spans) != 0 {
		t.Fatal("multiline row index")
	}
	if c.spans[0].start != len("中") || c.spans[0].end != len("中😀abc\nsec") {
		t.Fatal("UTF-16 conversion")
	}
	if a.cachedDiagnostics(d) != c {
		t.Fatal("cache wasn't reused")
	}
	d.Buffer.Apply([]text.Edit{{Start: 0, End: 0, Text: "x"}}, nil, nil)
	if a.cachedDiagnostics(d) == c {
		t.Fatal("version did not invalidate cache")
	}
	a.diagnostics[uri] = lsp.Diagnostics{Diagnostics: []lsp.Diagnostic{{Range: lsp.Range{Start: lsp.Position{Line: 2}, End: lsp.Position{Line: 2, Character: 2}}}}}
	if !a.cachedDiagnostics(d).row(2).marker {
		t.Fatal("new diagnostic batch was ignored")
	}
}

func TestPollingMetadataFallbackAndStaleSave(t *testing.T) {
	a, _ := testApp(t)
	path := filepath.Join(a.root, "poll.txt")
	if err := os.WriteFile(path, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := a.open(path); err != nil {
		t.Fatal(err)
	}
	d := a.view().Doc
	a.pollFiles(false)
	waitUpdates(t, a, func() bool { return !a.fileCheck })
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("other"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, st.ModTime(), st.ModTime()); err != nil {
		t.Fatal(err)
	}
	a.repaint = false
	a.pollFiles(false)
	waitUpdates(t, a, func() bool { return !a.fileCheck })
	if d.Buffer.String() != "first" || a.repaint {
		t.Fatal("metadata-only poll unexpectedly reloaded/repainted")
	}
	state := a.fileStates[d]
	state.lastFull = time.Now().Add(-31 * time.Second)
	a.fileStates[d] = state
	a.pollFiles(false)
	waitUpdates(t, a, func() bool { return !a.fileCheck })
	if d.Buffer.String() != "other" {
		t.Fatal("fallback did not detect metadata-preserving change")
	}
	// Capture an inspection, then save before its UI callback is applied.
	if err := os.WriteFile(path, []byte("third"), 0600); err != nil {
		t.Fatal(err)
	}
	d.Buffer.Apply([]text.Edit{{Start: 0, End: d.Buffer.Len(), Text: "saved"}}, nil, nil)
	a.checkFiles()
	var update func()
	select {
	case update = <-a.updates:
	case <-time.After(3 * time.Second):
		t.Fatal("poll timed out")
	}
	if err := d.Save(true); err != nil {
		t.Fatal(err)
	}
	update()
	if d.Buffer.String() != "saved" || d.Dirty() {
		t.Fatal("stale poll replaced newly saved contents")
	}
	// Window-focus checks bypass metadata filtering.
	st, err = os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("focus"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, st.ModTime(), st.ModTime()); err != nil {
		t.Fatal(err)
	}
	a.checkFiles()
	waitUpdates(t, a, func() bool { return !a.fileCheck })
	if d.Buffer.String() != "focus" {
		t.Fatal("focus check wasn't full")
	}
}

func TestHiddenTerminalOutputDoesNotRequestFrame(t *testing.T) {
	a, _ := testApp(t)
	a.cfg.Shell = "/bin/sh"
	a.showTerminal()
	waitUpdates(t, a, func() bool { return a.term.VT.String() != "" })
	a.hidePanel()
	a.repaint = false
	a.term.Paste("printf 'HIDDEN_%s\\n' ready\n")
	waitUpdates(t, a, func() bool { return strings.Contains(a.term.VT.String(), "HIDDEN_ready") })
	if a.repaint {
		t.Fatal("hidden output requested frame")
	}
}

func TestThemeAndSignalDialogInvalidateWithoutKeyboardEvent(t *testing.T) {
	a, _ := testApp(t)
	a.repaint = false
	a.applyTheme(a.cfg.Theme)
	if !a.repaint {
		t.Fatal("theme update didn't request a frame")
	}
	a.view().Insert("dirty")
	a.repaint = false
	a.confirmExit()
	if a.modal == nil || !a.repaint {
		t.Fatal("signal-driven save confirmation didn't request a frame")
	}
}
