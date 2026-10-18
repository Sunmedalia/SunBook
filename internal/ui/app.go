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
	"sunbook/internal/plugin"
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
	Compact                      bool
	AutoHeight                   bool
	Body                         []string
	BodyTop, BodyH, ListY, ListH int
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
	plugins                                               *plugin.Manager
	pluginStatuses                                        []plugin.Status
	pluginSelected, pluginTop                             int
	pluginSelectID                                        string
	lastPluginResult                                      *pluginOutcome
	markdownPreview                                       *pluginSplitPreview
	pluginRefreshing                                      bool
	pluginRefreshPending                                  bool
	ctx                                                   context.Context
	cancel                                                context.CancelFunc
	updates                                               chan func()
	repaint                                               bool
	lastDraw, nextDisk                                    time.Time
	nextPluginScan                                        time.Time
	maintenanceDue                                        map[*editor.Document]time.Time
	maintenanceRunning                                    map[*text.Buffer]bool
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
	treePath, newFileDir                                  string
	newFileDirs                                           map[*editor.Document]string
	expanded                                              map[string]bool
	treeSeq                                               int
	treeCancel                                            context.CancelFunc
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
	diagnosticCache                                       map[*editor.Document]*diagnosticCache
	highlight                                             map[*editor.Document]highlights
	highlightPending                                      map[*editor.Document]int
	highlightDue                                          map[*editor.Document]time.Time
	highlightRunning                                      map[*editor.Document]bool
	completionCancel                                      context.CancelFunc
	completion                                            *completionState
	completionDue                                         time.Time
	git                                                   gitservice.Status
	branches                                              []gitservice.Branch
	branchBase                                            gitservice.Status
	branchSeq                                             int
	mergeInProgress                                       bool
	gitError                                              string
	gitSelected, gitTop, problemSelected, problemTop      int
	gitBusy                                               bool
	gitReading                                            bool
	gitSeq                                                int
	gitMessage, gitFilter                                 string
	gitMessageCursor, gitMessageTop                       int
	gitGroupFocus                                         string
	gitCollapsed                                          map[string]bool
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
	gitLogNode                                            string
	gitLogOpen, gitLogCollapsed                           map[string]bool
	gitLogHeight                                          int
	gitLogReading, gitLogLoaded, gitLogExpanded           bool
	gitLogRefreshed                                       time.Time
	gitChangesRect, gitLogRect, gitLogHeader              rect
	term                                                  *terminal.Session
	termOffset                                            int
	termExited                                            bool
	termSeq                                               int
	terminals                                             []*terminalTab
	terminalActive                                        *terminalTab
	terminalMenu                                          *terminalMenu
	themeSyntax                                           string
	terminalColors                                        terminal.DefaultColors
	themeColorCache                                       map[tcell.Color]tcell.Color
	fileCheck                                             bool
	fileCheckForce                                        bool
	fileStates                                            map[*editor.Document]filePollState
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
	scrollbars                            []contentScrollbar
	scrollDrag                            *scrollbarDrag
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
	a.plugins = plugin.NewManager(ctx, a.pluginDirectory())
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
	case a.updates <- func() { f(); a.invalidate() }:
	case <-a.ctx.Done():
	}
}
func (a *App) notify(s string) {
	a.message = s
	a.messageUntil = time.Now().Add(8 * time.Second)
	a.invalidate()
}
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
	a.refreshPlugins()
	a.refreshGit()
	for _, d := range a.docs {
		a.startLanguage(d)
		a.scheduleHighlight(d)
	}
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	a.nextDisk = time.Now().Add(2 * time.Second)
	a.nextPluginScan = time.Now().Add(30 * time.Second)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigs)
	a.draw()
	a.repaint = false
	a.lastDraw = time.Now()
	var paste strings.Builder
	inPaste := false
	for !a.quit {
		now := time.Now()
		a.runScheduled(now)
		a.paintIfDue(now)
		timer.Reset(time.Until(a.nextWake(time.Now())))
		select {
		case <-a.ctx.Done():
			return nil
		case <-sigs:
			a.confirmExit()
		case <-a.plugins.Changes:
			a.updatePluginCommands()
		case f := <-a.updates:
			f()
		case <-timer.C:
		case ev, ok := <-a.screen.EventQ():
			if !ok {
				return nil
			}
			switch e := ev.(type) {
			case *tcell.EventResize:
				a.invalidate()
				a.tabDrag = nil
				a.scrollDrag = nil
				a.screen.Sync()
			case *tcell.EventPaste:
				if e.Start() {
					inPaste = true
					paste.Reset()
				}
				if e.End() {
					inPaste = false
					a.paste(paste.String())
					a.invalidate()
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
					a.invalidate()
				}
			case *tcell.EventMouse:
				a.mouse(e)
				a.invalidate()
			case *tcell.EventFocus:
				a.cancelCompletion()
				if e.Focused {
					a.requestTerminalColors()
					a.checkFiles()
				}
			}
		}
		// Keep PTY dimensions in step with layout before the next input event;
		// frame coalescing must not delay shell resize behind subsequent keys.
		a.layout()
		if a.term != nil && a.panel == "Terminal" && a.panelRect.H > 0 {
			_ = a.term.Resize(max(1, a.panelRect.W), max(1, a.panelRect.H))
		}
		a.refreshTerminalTheme()
	}
	return nil
}
func (a *App) Close() {
	a.cancel()
	if a.plugins != nil {
		a.plugins.Close()
	}
	if a.treeCancel != nil {
		a.treeCancel()
	}
	if a.treeIndexCancel != nil {
		a.treeIndexCancel()
	}
	if a.searchCancel != nil {
		a.searchCancel()
	}
	a.closeTerminals()
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
	a.cancelCompletion()
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
	if p := a.markdownPreview; p != nil && p.Outcome.doc == d {
		p.Due = time.Now().Add(100 * time.Millisecond)
	}
	a.scheduleMaintenance(d)
	a.cancelCompletion()
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
		c.SyncBuffer(d.Path, d.Buffer)
	}
	delete(a.diagnostics, lsp.URI(d.Path))
	delete(a.diagnosticCache, d)
	// Keep the previous colours, shifted through this edit, until the new
	// snapshot is tokenized; otherwise every keystroke blanks the file.
	if hl := a.highlight[d]; hl.Version == d.Buffer.Version-1 && len(d.Buffer.LastEdits) > 0 {
		a.highlight[d] = highlights{d.Buffer.Version, shiftSpans(hl.Spans, d.Buffer.LastEdits)}
	}
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
	if a.pluginPreview() != nil {
		a.closePluginSplit()
		return
	}
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
	a.removeTerminal(a.terminalActive)
}
func (a *App) startTerminal() {
	if a.term != nil && !a.termExited {
		return
	}
	a.newTerminal()
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
	delete(a.newFileDirs, d)
	delete(a.fileStates, d)
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
	dir := a.root
	if d.Path == "" && a.newFileDirs[d] != "" {
		dir = a.newFileDirs[d]
	}
	a.compactPrompt("Save As", d.Path, "Filename in "+a.directoryLabel(dir), func(s string) {
		if strings.TrimSpace(s) == "" {
			return
		}
		if !filepath.IsAbs(s) {
			s = filepath.Join(dir, s)
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
				c.CloseDocument(old)
				delete(a.opened, d)
			}
			delete(a.diagnostics, lsp.URI(old))
			delete(a.diagnosticCache, d)
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
			a.modal.Compact = true
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
		if p := a.markdownPreview; p != nil && p.Outcome.doc == d {
			a.closePluginSplit()
		}
		if a.lastPluginResult != nil && a.lastPluginResult.doc == d {
			a.lastPluginResult.doc = nil
			a.lastPluginResult.buffer = nil
		}
		a.keepDocument(d)
		for i, v := range a.docs {
			if v == d {
				a.docs = append(a.docs[:i], a.docs[i+1:]...)
				break
			}
		}
		if c := a.opened[d]; c != nil {
			c.CloseDocument(d.Path)
			delete(a.opened, d)
		}
		delete(a.diagnostics, lsp.URI(d.Path))
		delete(a.diagnosticCache, d)
		delete(a.highlight, d)
		delete(a.highlightPending, d)
		delete(a.highlightDue, d)
		delete(a.maintenanceDue, d)
		delete(a.newFileDirs, d)
		delete(a.fileStates, d)
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
			a.lastPluginResult = nil
			a.markdownPreview = nil
			a.clients = map[string]*lsp.Client{}
			a.starting = map[string]bool{}
			a.serverErrors = map[string]string{}
			a.opened = map[*editor.Document]*lsp.Client{}
			a.diagnostics = map[string]lsp.Diagnostics{}
			a.diagnosticCache = nil
			a.fileStates = nil
			a.maintenanceDue = nil
			a.highlight = map[*editor.Document]highlights{}
			a.highlightPending = map[*editor.Document]int{}
			a.highlightDue = map[*editor.Document]time.Time{}
			a.highlightRunning = map[*editor.Document]bool{}
			a.closeTerminals()
			a.root = path
			a.clearWorkspaceSearch()
			a.git = gitservice.Status{}
			a.gitError = ""
			a.gitSelected = 0
			a.gitTop = 0
			a.gitGroupFocus = ""
			a.gitCollapsed = nil
			a.gitMessageTop = 0
			a.gitMessage, a.gitFilter, a.gitActionError = "", "", ""
			a.gitMessageCursor, a.gitFilterCursor = 0, 0
			a.gitViewSeq++
			a.comparison = nil
			a.gitLogSeq++
			a.gitLog = nil
			a.gitLogNode = ""
			a.gitLogOpen = nil
			a.gitLogCollapsed = nil
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
			a.treePath, a.newFileDir = "", ""
			a.newFileDirs = nil
			a.docs = nil
			a.fileStates = nil
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
