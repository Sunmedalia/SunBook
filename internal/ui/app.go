// Package ui owns application state on one event loop. Workers only post closures
// back to that loop; they never mutate documents, views or modal dialogs directly.
package ui

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/gdamore/tcell/v3"
	"sunbook/internal/clipboard"
	"sunbook/internal/config"
	"sunbook/internal/editor"
	gitservice "sunbook/internal/git"
	"sunbook/internal/lsp"
	"sunbook/internal/terminal"
	"sunbook/internal/text"
)

type rect struct{ X, Y, W, H int }

func (r rect) contains(x, y int) bool { return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H }

type treeEntry struct {
	Path, Name string
	Depth      int
	Dir        bool
}
type button struct {
	rect
	Command, Hint string
	Disabled      bool
}
type item struct {
	Label, Detail string
	Path          string
	Dir           bool
	Action        func()
	Swatch        [3]string
}
type modal struct {
	Open                         *openPicker
	Subtitle, Placeholder, Empty string
	Accept                       func(bool)
	Complete, Parent             func()
	Title, Input, Hint           string
	InputRect                    rect
	InputOffset                  int
	Cursor                       int
	Items, Visible               []item
	Selected, Top                int
	InputEnabled, Filter         bool
	Body                         []string
	BodyTop, ListY, ListH        int
	BodyLeft                     int
	Submit                       func(string, int)
	Changed                      func(string)
}
type command struct {
	ID, Label, Key string
	Run            func()
}
type tokenSpan struct {
	Start, End int
	Style      tcell.Style
}
type highlights struct {
	Version int
	Spans   []tokenSpan
}
type completionState struct {
	Doc                     *editor.Document
	Version, Head, Selected int
	Items                   []lsp.Completion
}
type App struct {
	screen                                                tcell.Screen
	cfg                                                   config.Config
	root                                                  string
	ctx                                                   context.Context
	cancel                                                context.CancelFunc
	updates                                               chan func()
	docs                                                  []*editor.Document
	preview                                               *editor.Document
	views                                                 [2]*editor.View
	viewCache                                             [2]map[*editor.Document]*editor.View
	active                                                int
	split                                                 bool
	splitRatio                                            float64
	sidebar                                               bool
	sidebarView                                           string
	activityRect                                          rect
	sideHits                                              []sideHit
	sidebarWidth, panelHeight                             int
	panelWidth                                            int
	panelDock                                             string
	editorArea, panelDivider                              rect
	tabDrag                                               *tabDragState
	focus, panel                                          string
	panelVisible                                          bool
	tree                                                  []treeEntry
	treeAll                                               []treeEntry
	treeQuery                                             string
	treeQueryCursor, treeQueryOffset                      int
	treeFilterRect                                        rect
	treeFiles                                             []string
	treeIndexReady, treeIndexLoading                      bool
	treeIndexSeq                                          int
	treeIndexCancel                                       context.CancelFunc
	treeSelected, treeTop                                 int
	expanded                                              map[string]bool
	treeSeq                                               int
	modal                                                 *modal
	commands                                              []command
	bindings                                              map[string]string
	message                                               string
	prefix                                                string
	messageUntil                                          time.Time
	quit                                                  bool
	clipboard                                             clipboard.Clipboard
	clients                                               map[string]*lsp.Client
	starting                                              map[string]bool
	languageSeq                                           int
	serverErrors                                          map[string]string
	opened                                                map[*editor.Document]*lsp.Client
	diagnostics                                           map[string]lsp.Diagnostics
	highlight                                             map[*editor.Document]highlights
	highlightPending                                      map[*editor.Document]int
	highlightDue                                          map[*editor.Document]time.Time
	highlightRunning                                      map[*editor.Document]bool
	completion                                            *completionState
	completionDue                                         time.Time
	git                                                   gitservice.Status
	gitError                                              string
	gitSelected, gitTop, problemSelected, problemTop      int
	gitBusy                                               bool
	gitReading                                            bool
	gitSeq                                                int
	gitMessage, gitFilter                                 string
	gitMessageCursor, gitMessageOffset                    int
	gitFilterCursor, gitFilterOffset                      int
	gitMessageRect, gitFilterRect                         rect
	gitListY                                              int
	gitOperation                                          string
	gitActionError                                        string
	gitViewSeq                                            int
	comparison                                            *gitComparison
	gitLog                                                []gitservice.Revision
	gitLogPath, gitLogError                               string
	gitLogSelected, gitLogTop, gitLogSeq                  int
	gitLogHeight                                          int
	gitLogReading, gitLogLoaded, gitLogExpanded           bool
	gitLogRefreshed                                       time.Time
	gitChangesRect, gitLogRect, gitLogHeader              rect
	term                                                  *terminal.Session
	termOffset                                            int
	termExited                                            bool
	termSeq                                               int
	fileCheck                                             bool
	searchCancel                                          context.CancelFunc
	searchQuery                                           string
	searchCursor, searchOffset, searchSelected, searchTop int
	searchInputRect                                       rect
	searchResults                                         []searchHit
	searchRunning                                         bool
	searchError                                           string
	searchSeq                                             int
	searchDue                                             time.Time
	searchOptions                                         searchOptions
	searchIncludeCursor, searchIncludeOffset              int
	searchExcludeCursor, searchExcludeOffset              int
	searchIncludeRect, searchExcludeRect                  rect
	searchPathsVisible, searchTruncated                   bool
	searchCollapsed                                       map[string]bool
	searchGroupHits                                       []searchGroupHit
	recentFiles                                           []string
	rects                                                 [2]rect
	sideRect, panelRect                                   rect
	tabs                                                  []struct {
		rect
		Doc *editor.Document
	}
	modalRect                             rect
	buttons                               []button
	hoverCommand, hoverHint               string
	pressedCommand                        string
	pressedUntil                          time.Time
	drag                                  string
	dragGroup                             int
	lastButtons                           tcell.ButtonMask
	clickTime                             time.Time
	clickX, clickY, clickCount            int
	mouseClicks                           uint64
	fileClick                             fileClick
	themeSeq                              int
	base, muted, accent, chrome, selected tcell.Style
	colors                                palette
}

func New(screen tcell.Screen, cfg config.Config, path string) (*App, error) {
	root, e := os.Getwd()
	if e != nil {
		return nil, e
	}
	var file string
	if path != "" {
		path, e = filepath.Abs(path)
		if e != nil {
			return nil, e
		}
		st, err := os.Stat(path)
		if err == nil && st.IsDir() {
			root = path
		} else if err == nil || os.IsNotExist(err) {
			root = filepath.Dir(path)
			file = path
		} else {
			return nil, err
		}
	}
	// Match the canonical paths used by editor.Open so recent files and indexes agree.
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	ctx, cancel := context.WithCancel(context.Background())
	a := &App{screen: screen, cfg: cfg, root: root, ctx: ctx, cancel: cancel, updates: make(chan func(), 256), sidebar: true, sidebarWidth: 26, panelHeight: 12, panelWidth: 40, focus: "editor", panel: "Terminal", splitRatio: .5, expanded: map[string]bool{root: true}, clients: map[string]*lsp.Client{}, starting: map[string]bool{}, serverErrors: map[string]string{}, opened: map[*editor.Document]*lsp.Client{}, diagnostics: map[string]lsp.Diagnostics{}, highlight: map[*editor.Document]highlights{}, highlightPending: map[*editor.Document]int{}}
	for i := range a.viewCache {
		a.viewCache[i] = map[*editor.Document]*editor.View{}
	}
	a.highlightDue = map[*editor.Document]time.Time{}
	a.sidebarView = "Explorer"
	a.searchOptions.Regex = true
	a.highlightRunning = map[*editor.Document]bool{}
	a.initTheme()
	a.screen.SetStyle(a.base)
	a.registerCommands()
	if file != "" {
		if e = a.open(file); e != nil {
			cancel()
			return nil, e
		}
	} else {
		a.addDocument(editor.New())
	}
	return a, nil
}
func (a *App) view() *editor.View { return a.views[a.active] }
func (a *App) post(f func()) {
	select {
	case a.updates <- f:
	case <-a.ctx.Done():
	}
}
func (a *App) notify(s string) { a.message = s; a.messageUntil = time.Now().Add(8 * time.Second) }
func (a *App) showError(err error) {
	if err != nil {
		a.notify(err.Error())
	}
}
func (a *App) Run() error {
	defer a.Close()
	a.screen.EnableMouse(tcell.MouseMotionEvents, tcell.MouseButtonEvents, tcell.MouseDragEvents)
	a.screen.EnablePaste()
	a.screen.EnableFocus()
	a.screen.SetTitle("SunBook — " + filepath.Base(a.root))
	a.screen.SetCursorStyle(tcell.CursorStyleSteadyBar)
	a.scanTree()
	a.refreshGit()
	for _, d := range a.docs {
		a.startLanguage(d)
		a.scheduleHighlight(d)
	}
	tick := time.NewTicker(80 * time.Millisecond)
	defer tick.Stop()
	diskTick := time.NewTicker(2 * time.Second)
	defer diskTick.Stop()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigs)
	a.draw()
	var paste strings.Builder
	inPaste := false
	for !a.quit {
		select {
		case <-a.ctx.Done():
			return nil
		case <-sigs:
			a.confirmExit()
		case f := <-a.updates:
			f()
		case <-diskTick.C:
			a.checkFiles()
			a.refreshGit()
		case <-tick.C:
			a.flushHighlights()
			if !a.searchDue.IsZero() && !time.Now().Before(a.searchDue) {
				a.startWorkspaceSearch()
			}
			if !a.completionDue.IsZero() && time.Now().After(a.completionDue) {
				a.completionDue = time.Time{}
				a.requestCompletion(false)
			}
		case ev, ok := <-a.screen.EventQ():
			if !ok {
				return nil
			}
			switch e := ev.(type) {
			case *tcell.EventResize:
				a.tabDrag = nil
				a.screen.Sync()
			case *tcell.EventPaste:
				if e.Start() {
					inPaste = true
					paste.Reset()
				}
				if e.End() {
					inPaste = false
					a.paste(paste.String())
					paste.Reset()
				}
			case *tcell.EventKey:
				if !e.Pressed() {
					break
				}
				if inPaste {
					paste.WriteString(pasteText(e))
				} else {
					a.key(e)
				}
			case *tcell.EventMouse:
				a.mouse(e)
			case *tcell.EventFocus:
				if e.Focused {
					a.checkFiles()
				}
			}
		}
		a.draw()
	}
	return nil
}
func (a *App) Close() {
	a.cancel()
	if a.treeIndexCancel != nil {
		a.treeIndexCancel()
	}
	if a.searchCancel != nil {
		a.searchCancel()
	}
	if a.term != nil {
		a.term.Close()
	}
	for _, c := range a.clients {
		c.Close()
	}
}
func (a *App) addDocument(d *editor.Document) {
	a.docs = append(a.docs, d)
	a.activate(d)
	a.scheduleHighlight(d)
}
func (a *App) activate(d *editor.Document) {
	if a.split && a.views[1-a.active] != nil && a.views[1-a.active].Doc == d {
		a.keepDocument(d)
	}
	a.rememberFile(d.Path)
	a.gitViewSeq++
	if a.comparison != nil {
		a.comparison.Active = false
	}
	v := a.viewCache[a.active][d]
	if v == nil {
		v = editor.NewView(d)
		a.viewCache[a.active][d] = v
	}
	a.views[a.active] = v
	a.focus = "editor"
	a.completion = nil
}
func (a *App) open(path string) error {
	return a.openFile(path, false)
}
func (a *App) openFile(path string, preview bool) error {
	if !filepath.IsAbs(path) {
		path = filepath.Join(a.root, path)
	}
	path = filepath.Clean(path)
	if real, e := filepath.EvalSymlinks(path); e == nil {
		path = real
	}
	for _, d := range a.docs {
		if d.Path == path {
			if !preview {
				a.keepDocument(d)
			}
			a.activate(d)
			return nil
		}
	}
	d, e := editor.Open(path)
	if e != nil {
		return e
	}
	// Read successfully before replacing a preview. A failed open must leave
	// the current document and its selection intact.
	old, slot := a.preview, len(a.docs)
	if preview && old != nil {
		if old.Dirty() || a.split && a.views[1-a.active] != nil && a.views[1-a.active].Doc == old {
			a.keepDocument(old)
			old = nil
		} else {
			for i, doc := range a.docs {
				if doc == old {
					slot = i
					break
				}
			}
		}
	}
	a.addDocument(d)
	if preview {
		if old != nil {
			// The new document is already active, so closing the clean preview
			// neither prompts nor creates an intermediate untitled document.
			a.closeDocument(old)
			a.reorderDocument(d, slot)
		}
		a.preview = d
	}
	a.startLanguage(d)
	return nil
}
func (a *App) changed(d *editor.Document) {
	a.keepDocument(d)
	for _, cache := range a.viewCache {
		if v := cache[d]; v != nil {
			if v != a.view() {
				for i, s := range v.Selections {
					v.Selections[i] = text.Selection{Anchor: mapOffset(s.Anchor, d.Buffer.LastEdits), Head: mapOffset(s.Head, d.Buffer.LastEdits)}
				}
			}
			v.Clamp()
		}
	}
	if c := a.opened[d]; c != nil {
		c.Change(d.Path, d.Buffer.String(), d.Buffer.Version)
	}
	delete(a.diagnostics, lsp.URI(d.Path))
	a.scheduleHighlight(d)
	a.completion = nil
	if a.focus == "editor" {
		a.completionDue = time.Now().Add(450 * time.Millisecond)
	}
}
func (a *App) apply(edits []text.Edit) bool {
	edits = append([]text.Edit(nil), edits...)
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].Start < edits[j].Start })
	v := a.view()
	before := append([]text.Selection(nil), v.Selections...)
	after := append([]text.Selection(nil), before...)
	// Map every caret through each edit using offsets in the original document.
	for i, s := range after {
		after[i] = text.Selection{Anchor: mapOffset(s.Anchor, edits), Head: mapOffset(s.Head, edits)}
	}
	if v.Doc.Buffer.Apply(edits, before, after) {
		v.Selections = after
		a.changed(v.Doc)
		return true
	}
	return false
}
func mapOffset(p int, edits []text.Edit) int {
	delta := 0
	for _, e := range edits {
		if p < e.Start {
			continue
		}
		if p <= e.End {
			return e.Start + delta + len(e.Text)
		}
		delta += len(e.Text) - (e.End - e.Start)
	}
	return p + delta
}
func (a *App) splitEditor() {
	if a.comparison != nil {
		a.comparison.Active = false
	}
	if a.split {
		a.split = false
		a.active = 0
	} else {
		a.keepDocument(a.views[0].Doc)
		a.split = true
		a.active = 1
		a.activate(a.views[0].Doc)
	}
	a.focus = "editor"
}
func (a *App) toggleTerminal() {
	a.panelVisible = true
	a.panel = "Terminal"
	if a.focus == "terminal" {
		a.focus = "editor"
	} else {
		a.focus = "terminal"
		a.startTerminal()
	}
}
func (a *App) showTerminal() {
	a.panelVisible = true
	a.panel = "Terminal"
	a.focus = "terminal"
	a.startTerminal()
}
func (a *App) hidePanel() {
	a.panelVisible = false
	if a.focus == "terminal" || a.focus == "problems" {
		a.focus = "editor"
	}
}
func (a *App) killTerminal() {
	s := a.term
	a.termSeq++
	a.term = nil
	a.termOffset = 0
	a.termExited = false
	if s != nil {
		s.Close()
	}
	if a.panel == "Terminal" {
		a.hidePanel()
	}
	a.notify("Shell session ended. F4 opens a new session.")
}
func (a *App) startTerminal() {
	if a.term != nil && !a.termExited {
		return
	}
	if a.term != nil {
		a.term.Close()
	}
	a.termSeq++
	seq := a.termSeq
	a.layout()
	r := a.panelRect
	s, e := terminal.Start(a.cfg.Shell, a.root, max(1, r.W), max(1, r.H), func(data []byte) {
		a.post(func() {
			if a.term != nil && seq == a.termSeq {
				a.term.Feed(data)
			}
		})
	}, func(err error) {
		a.post(func() {
			if seq != a.termSeq {
				return
			}
			a.termExited = true
			if err != nil {
				a.notify("Shell exited: " + err.Error())
			} else {
				a.notify("Shell exited. Press F4 to start a new session.")
			}
		})
	})
	if e != nil {
		a.showError(e)
		a.focus = "editor"
		return
	}
	a.term = s
	a.termExited = false
}
func (a *App) checkFiles() {
	if a.fileCheck {
		return
	}
	a.fileCheck = true
	type check struct {
		doc      *editor.Document
		snapshot editor.Document
		version  int
		reloaded *editor.Document
		changed  bool
		err      error
	}
	checks := make([]check, 0, len(a.docs))
	for _, d := range a.docs {
		if d.Path != "" {
			checks = append(checks, check{doc: d, snapshot: *d, version: d.Buffer.Version})
		}
	}
	go func() {
		for i := range checks {
			c := &checks[i]
			c.changed = c.snapshot.DiskChanged()
			if c.changed {
				c.reloaded, c.err = editor.Open(c.snapshot.Path)
			}
		}
		a.post(func() {
			a.fileCheck = false
			for _, c := range checks {
				if !c.changed || c.doc.Buffer.Version != c.version || c.doc.Path != c.snapshot.Path || !c.doc.DiskChanged() {
					continue
				}
				stillOpen := false
				for _, d := range a.docs {
					if d == c.doc {
						stillOpen = true
						break
					}
				}
				if !stillOpen {
					continue
				}
				if c.err != nil {
					a.notify("External change: " + c.err.Error())
					continue
				}
				if c.doc.Dirty() {
					a.notify(c.doc.Name() + " changed on disk. Save will offer conflict resolution.")
					continue
				}
				if !c.reloaded.Exists {
					a.notify(c.doc.Name() + " was removed on disk")
					continue
				}
				oldVersion := c.doc.Buffer.Version
				*c.doc = *c.reloaded
				c.doc.Buffer.Version = oldVersion + 1
				for _, v := range a.views {
					if v != nil && v.Doc == c.doc {
						v.Clamp()
					}
				}
				a.changed(c.doc)
				a.notify(c.doc.Name() + " reloaded")
			}
		})
	}()
}
func (a *App) save(d *editor.Document, done func()) {
	if d.Path == "" {
		a.saveAs(d, done)
		return
	}
	if e := d.Save(false); e == editor.ErrConflict {
		a.choose("File changed on disk", []item{{Label: "Reload disk version (discard buffer)", Action: func() {
			fresh, err := editor.Open(d.Path)
			if err != nil {
				a.showError(err)
				return
			}
			version := d.Buffer.Version
			*d = *fresh
			d.Buffer.Version = version + 1
			a.changed(d)
		}}, {Label: "Overwrite disk with editor version", Action: func() { a.saveForce(d, done) }}, {Label: "Cancel"}})
		return
	} else if e != nil {
		a.showError(e)
		return
	}
	a.saved(d)
	if done != nil {
		done()
	}
}
func (a *App) saveForce(d *editor.Document, done func()) {
	if e := d.Save(true); e != nil {
		a.showError(e)
		return
	}
	a.saved(d)
	if done != nil {
		done()
	}
}
func (a *App) saved(d *editor.Document) {
	a.keepDocument(d)
	a.rememberFile(d.Path)
	a.notify("Saved " + d.Name())
	if c := a.opened[d]; c != nil {
		c.Notify("textDocument/didSave", map[string]any{"textDocument": map[string]string{"uri": lsp.URI(d.Path)}, "text": d.Buffer.String()})
	} else {
		a.startLanguage(d)
	}
	a.scanTree()
	a.refreshGit()
}
func (a *App) saveAs(d *editor.Document, done func()) {
	a.prompt("Save As", d.Path, func(s string) {
		if strings.TrimSpace(s) == "" {
			return
		}
		if !filepath.IsAbs(s) {
			s = filepath.Join(a.root, s)
		}
		s = filepath.Clean(s)
		for _, other := range a.docs {
			if other != d && other.Path == s {
				a.notify("That file is already open")
				return
			}
		}
		finish := func() {
			old := d.Path
			oldExists := d.Exists
			d.Path = s
			d.Exists = false
			if e := d.Save(true); e != nil {
				d.Path = old
				d.Exists = oldExists
				a.showError(e)
				return
			}
			if c := a.opened[d]; c != nil {
				c.Notify("textDocument/didClose", map[string]any{"textDocument": map[string]string{"uri": lsp.URI(old)}})
				delete(a.opened, d)
			}
			delete(a.diagnostics, lsp.URI(old))
			delete(a.highlightPending, d)
			a.scheduleHighlight(d)
			a.saved(d)
			if done != nil {
				done()
			}
		}
		if st, e := os.Lstat(s); e == nil {
			if st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
				a.notify("Choose a regular file path")
				return
			}
			a.choose("Overwrite "+filepath.Base(s)+"?", []item{{Label: "Overwrite", Action: finish}, {Label: "Cancel"}})
		} else {
			finish()
		}
	})
}
func (a *App) confirmDocument(d *editor.Document, done func()) {
	if !d.Dirty() {
		done()
		return
	}
	a.choose("Save changes to "+d.Name()+"?", []item{{Label: "Save", Action: func() { a.save(d, done) }}, {Label: "Discard", Action: done}, {Label: "Cancel"}})
}
func (a *App) closeDocument(d *editor.Document) {
	comparison := a.comparison
	wasComparing := a.comparing()
	a.confirmDocument(d, func() {
		a.keepDocument(d)
		for i, v := range a.docs {
			if v == d {
				a.docs = append(a.docs[:i], a.docs[i+1:]...)
				break
			}
		}
		if c := a.opened[d]; c != nil {
			c.Notify("textDocument/didClose", map[string]any{"textDocument": map[string]string{"uri": lsp.URI(d.Path)}})
			delete(a.opened, d)
		}
		delete(a.diagnostics, lsp.URI(d.Path))
		delete(a.highlight, d)
		delete(a.highlightPending, d)
		delete(a.highlightDue, d)
		for i := range a.viewCache {
			delete(a.viewCache[i], d)
		}
		if len(a.docs) == 0 {
			a.docs = append(a.docs, editor.New())
		}
		old := a.active
		for i, v := range a.views {
			if v != nil && v.Doc == d {
				a.active = i
				a.activate(a.docs[0])
			}
		}
		a.active = old
		if wasComparing && comparison == a.comparison {
			a.comparison.Active = true
		}
	})
}
func (a *App) confirmExit() {
	var next func(int)
	next = func(i int) {
		if i >= len(a.docs) {
			a.quit = true
			return
		}
		a.confirmDocument(a.docs[i], func() { next(i + 1) })
	}
	next(0)
}
func (a *App) switchWorkspace() {
	a.prompt("Open Folder", a.root, func(path string) {
		if !filepath.IsAbs(path) {
			path = filepath.Join(a.root, path)
		}
		path = filepath.Clean(path)
		st, e := os.Stat(path)
		if e != nil {
			a.showError(e)
			return
		}
		if !st.IsDir() {
			a.notify("Choose a directory")
			return
		}
		if real, err := filepath.EvalSymlinks(path); err == nil {
			path = real
		}
		var next func(int)
		next = func(i int) {
			if i < len(a.docs) {
				a.confirmDocument(a.docs[i], func() { next(i + 1) })
				return
			}
			for _, c := range a.clients {
				go c.Close()
			}
			a.languageSeq++
			a.clients = map[string]*lsp.Client{}
			a.starting = map[string]bool{}
			a.serverErrors = map[string]string{}
			a.opened = map[*editor.Document]*lsp.Client{}
			a.diagnostics = map[string]lsp.Diagnostics{}
			a.highlight = map[*editor.Document]highlights{}
			a.highlightPending = map[*editor.Document]int{}
			a.highlightDue = map[*editor.Document]time.Time{}
			a.highlightRunning = map[*editor.Document]bool{}
			if a.term != nil {
				a.termSeq++
				a.term.Close()
				a.term = nil
			}
			a.root = path
			a.clearWorkspaceSearch()
			a.git = gitservice.Status{}
			a.gitError = ""
			a.gitSelected = 0
			a.gitTop = 0
			a.gitMessage, a.gitFilter, a.gitActionError = "", "", ""
			a.gitMessageCursor, a.gitFilterCursor = 0, 0
			a.gitViewSeq++
			a.comparison = nil
			a.gitLogSeq++
			a.gitLog = nil
			a.gitLogPath = ""
			a.gitLogError = ""
			a.gitLogSelected = 0
			a.gitLogTop = 0
			a.gitLogReading = false
			a.gitLogLoaded = false
			a.gitLogRefreshed = zeroTime
			a.gitLogExpanded = false
			if a.treeIndexCancel != nil {
				a.treeIndexCancel()
			}
			a.treeIndexSeq++
			a.treeIndexReady = false
			a.treeIndexLoading = false
			a.treeFiles = nil
			a.treeAll = nil
			a.tree = nil
			a.treeQuery = ""
			a.treeQueryCursor = 0
			a.treeTop = 0
			a.treeSelected = 0
			a.docs = nil
			a.preview = nil
			a.fileClick = fileClick{}
			a.views = [2]*editor.View{}
			for j := range a.viewCache {
				a.viewCache[j] = map[*editor.Document]*editor.View{}
			}
			a.split = false
			a.active = 0
			a.expanded = map[string]bool{path: true}
			a.addDocument(editor.New())
			a.scanTree()
			a.gitSeq++
			a.gitBusy = false
			a.gitReading = false
			a.refreshGit()
		}
		next(0)
	})
}
func (a *App) status() string {
	v := a.view()
	r, c := v.Doc.Buffer.Position(v.Head())
	eol := "LF"
	if v.Doc.CRLF {
		eol = "CRLF"
	}
	mode := "UTF-8"
	key, _ := config.Language(v.Doc.Path)
	if err := a.serverErrors[key]; err != "" {
		mode += " · LSP unavailable"
	}
	position := positionLabel(r, text.Column(v.Doc.Buffer.Line(r), c, a.cfg.TabSize))
	if len(v.Selections) > 1 {
		position += fmt.Sprintf(" · %d cursors", len(v.Selections))
	}
	_, language := config.Language(v.Doc.Path)
	if language == "" {
		language = "Plain text"
	}
	return fmt.Sprintf("%s   %s · %s   %s", position, mode, eol, language)
}

func positionLabel(row, col int) string { return fmt.Sprintf("Ln %d, Col %d", row+1, col+1) }
