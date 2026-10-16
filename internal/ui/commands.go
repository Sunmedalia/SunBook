package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"sunbook/internal/config"
	"sunbook/internal/editor"
	"sunbook/internal/lsp"
	"sunbook/internal/text"
)

func (a *App) registerCommands() {
	a.commands = []command{
		{"file.new", "File: New File", "ctrl+n", func() { a.addDocument(editor.New()) }},
		{"file.open", "File: Open File…", "ctrl+o", func() { a.prompt("Open File", "", func(s string) { a.showError(a.open(s)) }) }},
		{"file.quickOpen", "File: Quick Open…", "ctrl+p", a.quickOpen},
		{"file.folder", "File: Open Folder…", "", a.switchWorkspace},
		{"file.save", "File: Save", "ctrl+s", func() { a.save(a.view().Doc, nil) }},
		{"file.saveAs", "File: Save As…", "ctrl+shift+s", func() { a.saveAs(a.view().Doc, nil) }},
		{"file.saveAll", "File: Save All", "", a.saveAll},
		{"file.close", "File: Close Tab", "ctrl+w", func() { a.closeDocument(a.view().Doc) }},
		{"file.rename", "File: Rename…", "", a.renameFile},
		{"file.reload", "File: Reload from Disk", "", a.reload},
		{"app.quit", "Quit SunBook", "ctrl+q", a.confirmExit},
		{"app.commands", "Show Command Palette", "ctrl+shift+p", a.palette},
		{"app.help", "Help: Keyboard Shortcuts", "", a.help},
		{"app.doctor", "Help: Dependency Check", "", func() { a.showText("Dependencies", config.Doctor(a.cfg)) }},
		{"edit.undo", "Edit: Undo", "ctrl+z", func() {
			if a.view().Undo(false) {
				a.changed(a.view().Doc)
			}
		}},
		{"edit.redo", "Edit: Redo", "ctrl+y", func() {
			if a.view().Undo(true) {
				a.changed(a.view().Doc)
			}
		}},
		{"edit.copy", "Edit: Copy", "ctrl+c", a.copy},
		{"edit.cut", "Edit: Cut", "ctrl+x", a.cut},
		{"edit.paste", "Edit: Paste", "ctrl+v", a.readClipboard},
		{"edit.selectAll", "Edit: Select All", "ctrl+a", func() { a.view().Selections = []text.Selection{{Anchor: 0, Head: a.view().Doc.Buffer.Len()}} }},
		{"edit.selectNext", "Edit: Add Next Match", "ctrl+d", func() { a.view().SelectNext() }},
		{"edit.addCursorBelow", "Edit: Add Cursor Below", "ctrl+alt+down", func() { a.addCursor(1) }},
		{"edit.addCursorAbove", "Edit: Add Cursor Above", "ctrl+alt+up", func() { a.addCursor(-1) }},
		{"edit.comment", "Edit: Toggle Line Comment", "ctrl+/", a.toggleComment},
		{"edit.indent", "Edit: Indent Lines", "", func() { a.indent(false) }},
		{"edit.outdent", "Edit: Outdent Lines", "shift+tab", func() { a.indent(true) }},
		{"edit.moveUp", "Edit: Move Lines Up", "alt+up", func() { a.moveLines(-1) }},
		{"edit.moveDown", "Edit: Move Lines Down", "alt+down", func() { a.moveLines(1) }},
		{"search.find", "Search: Find in File", "ctrl+f", func() { a.find(false) }},
		{"search.replace", "Search: Replace in File", "ctrl+h", func() { a.find(true) }},
		{"search.workspace", "Search: Find in Workspace", "ctrl+shift+f", a.workspaceSearch},
		{"navigate.line", "Go to Line…", "ctrl+g", a.goLine},
		{"view.sidebar", "View: Toggle Sidebar", "ctrl+b", func() { a.sidebar = !a.sidebar }},
		{"explorer.filter", "Explorer: Filter Files", "ctrl+shift+e", a.focusTreeFilter},
		{"explorer.clearFilter", "Explorer: Clear Filter", "", func() { a.setTreeQuery(""); a.focus = "filter" }},
		{"explorer.refresh", "Explorer: Refresh", "", a.scanTree},
		{"view.split", "View: Toggle Split Editor", "ctrl+\\", a.splitEditor},
		{"view.other", "View: Focus Other Editor", "ctrl+k ctrl+o", func() {
			if a.split {
				a.active = 1 - a.active
			}
			a.focus = "editor"
		}},
		{"view.nextTab", "View: Next Tab", "ctrl+tab", func() { a.nextTab(1) }},
		{"view.previousTab", "View: Previous Tab", "ctrl+shift+tab", func() { a.nextTab(-1) }},
		{"view.panel", "View: Toggle Bottom Panel", "ctrl+j", func() {
			a.panelVisible = !a.panelVisible
			if !a.panelVisible {
				a.focus = "editor"
			}
		}},
		{"view.terminal", "View: Focus Terminal", "f4", a.toggleTerminal},
		{"view.showTerminal", "Terminal: Show Terminal", "", a.showTerminal},
		{"view.hidePanel", "View: Hide Bottom Panel", "", a.hidePanel},
		{"terminal.kill", "Terminal: Kill Shell Session", "", a.killTerminal},
		{"view.nextFocus", "View: Focus Next Panel", "f6", a.cycleFocus},
		{"view.problems", "View: Problems", "", func() { a.panel = "Problems"; a.panelVisible = true; a.focus = "problems" }},
		{"view.git", "View: Source Control", "", func() { a.panel = "Git"; a.panelVisible = true; a.focus = "git"; a.refreshGit() }},
		{"language.complete", "Language: Trigger Completion", "ctrl+space", func() { a.requestCompletion(true) }},
		{"language.hover", "Language: Hover", "alt+h", a.hover},
		{"language.definition", "Language: Go to Definition", "f12", func() { a.definition(false) }},
		{"language.references", "Language: Find References", "shift+f12", func() { a.definition(true) }},
		{"language.rename", "Language: Rename Symbol", "f2", a.renameSymbol},
		{"language.format", "Language: Format Document", "alt+shift+f", a.format},
		{"language.restart", "Language: Restart Language Servers", "", a.restartLanguages},
		{"git.refresh", "Git: Refresh", "", a.refreshGit},
		{"git.stage", "Git: Stage Selected File", "", func() { a.gitAction(true) }},
		{"git.unstage", "Git: Unstage Selected File", "", func() { a.gitAction(false) }},
		{"git.diff", "Git: Show Worktree Diff", "", func() { a.gitDiff(false) }},
		{"git.stagedDiff", "Git: Show Staged Diff", "", func() { a.gitDiff(true) }},
		{"git.commit", "Git: Commit Staged Changes…", "", a.commit},
	}
	a.bindings = map[string]string{"f1": "app.commands", "ctrl+shift+z": "edit.redo", "f3": "search.find", "f5": "git.refresh"}
	for _, c := range a.commands {
		if c.Key != "" {
			a.bindings[c.Key] = c.ID
		}
	}
	for key, id := range a.cfg.Keybindings {
		a.bindings[strings.ToLower(key)] = id
	}
}
func (a *App) execute(id string) {
	for _, cmd := range a.commands {
		if cmd.ID == id {
			a.completion = nil
			a.completionDue = zeroTime
			cmd.Run()
			a.reveal()
			return
		}
	}
	a.notify("Unknown command: " + id)
}
func (a *App) palette() {
	var items []item
	for _, cmd := range a.commands {
		cmd := cmd
		items = append(items, item{Label: cmd.Label, Detail: cmd.Key, Action: func() { a.execute(cmd.ID) }})
	}
	a.setModal(&modal{Title: "Command Palette", InputEnabled: true, Filter: true, Items: items, Hint: "Type a command · Enter runs · Esc closes"})
}
func (a *App) help() {
	s := "SUNBOOK\n\n"
	for _, c := range a.commands {
		if c.Key != "" {
			s += fmt.Sprintf("%-24s %s\n", c.Key, c.Label)
		}
	}
	s += "\nF1: command palette · F6: cycle focus · Esc: close overlay / clear extra cursors\nMouse: click / drag / double click / triple click / wheel\nAlt+click: add cursor · drag borders: resize panels\nTerminal: F4 focuses editor; Ctrl+J / [Hide] hides panel; [Kill] ends shell.\nExplorer: click filter or press /; Esc clears; Down selects results.\nGit panel: s stage · u unstage · d diff · D staged diff · c commit\nModal input: Ctrl+U clears · Up/Down choose · Enter accepts\nAll actions are accessible through the command palette.\n"
	a.showText("Keyboard Shortcuts", s)
}
func (a *App) nextTab(dir int) {
	if len(a.docs) == 0 {
		return
	}
	for i, d := range a.docs {
		if d == a.view().Doc {
			a.activate(a.docs[(i+dir+len(a.docs))%len(a.docs)])
			break
		}
	}
}
func (a *App) saveAll() {
	var next func(int)
	next = func(i int) {
		if i >= len(a.docs) {
			a.notify("All files saved")
			return
		}
		d := a.docs[i]
		if d.Dirty() {
			a.save(d, func() { next(i + 1) })
		} else {
			next(i + 1)
		}
	}
	next(0)
}
func (a *App) copy() {
	s := a.view().SelectedText()
	go func() {
		ok := a.clipboard.Write(s)
		a.post(func() {
			if ok {
				a.notify("Copied")
			} else {
				a.notify("Copied to internal clipboard (system clipboard unavailable)")
			}
		})
	}()
}
func (a *App) cut() {
	v := a.view()
	s := v.SelectedText()
	b := v.Doc.Buffer
	ranges := append([]text.Selection(nil), v.Selections...)
	for i, sel := range ranges {
		if sel.Empty() {
			r, _ := b.Position(sel.Head)
			lo, hi := b.LineStart(r), b.LineEnd(r)
			if hi < b.Len() {
				hi++
			}
			ranges[i] = text.Selection{Anchor: lo, Head: hi}
		}
	}
	if v.ReplaceRanges(ranges, "") {
		a.changed(v.Doc)
	}
	go func() {
		ok := a.clipboard.Write(s)
		a.post(func() {
			if !ok {
				a.notify("Cut to internal clipboard")
			}
		})
	}()
}
func (a *App) readClipboard() {
	// Clipboard commands are bounded but may block on a desktop service.
	m := a.modal
	v := a.view()
	version := v.Doc.Buffer.Version
	focus := a.focus
	go func() {
		s, system := a.clipboard.Read()
		a.post(func() {
			if a.modal != m || a.focus != focus || a.view() != v || v.Doc.Buffer.Version != version {
				a.notify("Paste cancelled because focus or document changed")
				return
			}
			a.paste(s)
			if !system {
				a.notify("Pasted internal clipboard")
			}
		})
	}()
}
func (a *App) paste(s string) {
	if s == "" {
		return
	}
	if a.modal != nil {
		if a.modal.InputEnabled {
			a.modalInsert(strings.ReplaceAll(strings.ReplaceAll(s, "\r", ""), "\n", " "))
		}
		return
	}
	if a.focus == "terminal" {
		if a.term != nil && !a.termExited {
			a.term.Paste(s)
		}
		return
	}
	if a.focus == "filter" {
		a.insertTreeQuery(s)
		return
	}
	if a.focus == "editor" && a.view().Insert(s) {
		a.changed(a.view().Doc)
		a.reveal()
	}
}
func (a *App) addCursor(dir int) {
	v := a.view()
	b := v.Doc.Buffer
	p := v.Head()
	r, c := b.Position(p)
	nr := r + dir
	if nr < 0 || nr >= b.LineCount() {
		return
	}
	p = b.LineStart(nr) + text.Offset(b.Line(nr), text.Column(b.Line(r), c, a.cfg.TabSize), a.cfg.TabSize)
	for _, s := range v.Selections {
		if s.Head == p {
			return
		}
	}
	v.Selections = append(v.Selections, text.Selection{Anchor: p, Head: p})
}
func (a *App) selectedRows() []int {
	set := map[int]bool{}
	b := a.view().Doc.Buffer
	for _, sel := range a.view().Selections {
		lo, hi := sel.Range()
		r, _ := b.Position(lo)
		z, _ := b.Position(hi)
		if hi > lo && hi == b.LineStart(z) {
			z--
		}
		for ; r <= z; r++ {
			set[r] = true
		}
	}
	rows := make([]int, 0, len(set))
	for r := range set {
		rows = append(rows, r)
	}
	sort.Ints(rows)
	return rows
}
func (a *App) indent(out bool) {
	b := a.view().Doc.Buffer
	var edits []text.Edit
	pad := "\t"
	if a.cfg.InsertSpaces {
		pad = strings.Repeat(" ", a.cfg.TabSize)
	}
	for _, row := range a.selectedRows() {
		start := b.LineStart(row)
		e := text.Edit{Start: start, End: start, Text: pad}
		if out {
			e.Text = ""
			s := b.Line(row)
			if strings.HasPrefix(s, "\t") {
				e.End++
			} else {
				for i := 0; i < min(a.cfg.TabSize, len(s)) && s[i] == ' '; i++ {
					e.End++
				}
			}
		}
		if e.Start != e.End || e.Text != "" {
			edits = append(edits, e)
		}
	}
	a.apply(edits)
}
func (a *App) toggleComment() {
	b := a.view().Doc.Buffer
	prefix := "//"
	key, _ := config.Language(a.view().Doc.Path)
	if key == "python" || filepath.Ext(a.view().Doc.Path) == ".sh" {
		prefix = "#"
	}
	rows := a.selectedRows()
	all := true
	for _, r := range rows {
		line := strings.TrimLeft(b.Line(r), " \t")
		if line != "" && !strings.HasPrefix(line, prefix) {
			all = false
		}
	}
	var edits []text.Edit
	for _, r := range rows {
		s := b.Line(r)
		trimmed := strings.TrimLeft(s, " \t")
		if trimmed == "" {
			continue
		}
		at := b.LineStart(r) + len(s) - len(trimmed)
		e := text.Edit{Start: at, End: at, Text: prefix + " "}
		if all {
			e.Text = ""
			e.End += len(prefix)
			if len(trimmed) > len(prefix) && trimmed[len(prefix)] == ' ' {
				e.End++
			}
		}
		edits = append(edits, e)
	}
	a.apply(edits)
}
func (a *App) moveLines(dir int) {
	v := a.view()
	b := v.Doc.Buffer
	rows := a.selectedRows()
	if len(rows) == 0 {
		return
	}
	first, last := rows[0], rows[len(rows)-1]
	if dir < 0 && first == 0 || dir > 0 && last == b.LineCount()-1 {
		return
	}
	var start, end int
	var replacement string
	if dir < 0 {
		start = b.LineStart(first - 1)
		end = b.LineEnd(last)
		replacement = b.String()[b.LineStart(first):end] + "\n" + b.Line(first-1)
	} else {
		start = b.LineStart(first)
		end = b.LineEnd(last + 1)
		replacement = b.Line(last+1) + "\n" + b.String()[start:b.LineEnd(last)]
	}
	after := append([]text.Selection(nil), v.Selections...)
	shift := 0
	if dir < 0 {
		shift = -(len(b.Line(first-1)) + 1)
	} else {
		shift = len(b.Line(last+1)) + 1
	}
	for i := range after {
		after[i].Anchor += shift
		after[i].Head += shift
	}
	if b.Apply([]text.Edit{{Start: start, End: end, Text: replacement}}, v.Selections, after) {
		v.Selections = after
		a.changed(v.Doc)
	}
}
func (a *App) find(replace bool) {
	d := a.view().Doc
	initial := ""
	sel := a.view().Selections[0]
	if !sel.Empty() {
		lo, hi := sel.Range()
		initial = d.Buffer.String()[lo:hi]
	}
	a.prompt("Find in File", initial, func(needle string) {
		if needle == "" {
			return
		}
		if !replace {
			a.findNext(needle)
			return
		}
		a.prompt("Replace With", "", func(replacement string) {
			a.choose("Replace "+fmt.Sprintf("%q", needle), []item{{Label: "Replace next", Action: func() {
				if a.findNext(needle) {
					if a.view().Insert(replacement) {
						a.changed(d)
					}
				}
			}}, {Label: "Replace all", Action: func() {
				var edits []text.Edit
				s := d.Buffer.String()
				for pos := 0; pos <= len(s)-len(needle); {
					i := strings.Index(s[pos:], needle)
					if i < 0 {
						break
					}
					i += pos
					edits = append(edits, text.Edit{Start: i, End: i + len(needle), Text: replacement})
					pos = i + len(needle)
				}
				a.apply(edits)
				a.notify(fmt.Sprintf("Replaced %d occurrence(s)", len(edits)))
			}}, {Label: "Cancel"}})
		})
	})
}
func (a *App) findNext(needle string) bool {
	v := a.view()
	s := v.Doc.Buffer.String()
	head := v.Head()
	i := strings.Index(s[head:], needle)
	if i >= 0 {
		i += head
	} else {
		i = strings.Index(s[:head], needle)
	}
	if i < 0 {
		a.notify("No match")
		return false
	}
	v.Selections = []text.Selection{{Anchor: i, Head: i + len(needle)}}
	a.reveal()
	return true
}
func (a *App) goLine() {
	a.prompt("Go to Line (line[:column])", "", func(s string) {
		line, col := 1, 1
		_, _ = fmt.Sscanf(s, "%d:%d", &line, &col)
		b := a.view().Doc.Buffer
		r := max(0, min(line-1, b.LineCount()-1))
		a.view().Set(b.LineStart(r)+text.Offset(b.Line(r), max(0, col-1), a.cfg.TabSize), false)
		a.reveal()
	})
}
func (a *App) reload() {
	d := a.view().Doc
	if d.Path == "" {
		return
	}
	reload := func() {
		fresh, e := editor.Open(d.Path)
		if e != nil {
			a.showError(e)
			return
		}
		version := d.Buffer.Version
		*d = *fresh
		d.Buffer.Version = version + 1
		a.changed(d)
	}
	if d.Dirty() {
		a.choose("Discard changes and reload?", []item{{Label: "Reload", Action: reload}, {Label: "Cancel"}})
	} else {
		reload()
	}
}
func (a *App) renameFile() {
	d := a.view().Doc
	if d.Path == "" {
		a.saveAs(d, nil)
		return
	}
	a.prompt("Rename File", d.Path, func(path string) {
		if !filepath.IsAbs(path) {
			path = filepath.Join(a.root, path)
		}
		path = filepath.Clean(path)
		if path == d.Path {
			return
		}
		if _, e := os.Lstat(path); !os.IsNotExist(e) {
			a.notify("Destination already exists or cannot be accessed")
			return
		}
		for _, other := range a.docs {
			if other != d && other.Path == path {
				a.notify("Destination is already open")
				return
			}
		}
		func() {
			old := d.Path
			if e := os.Rename(old, path); e != nil {
				a.showError(e)
				return
			}
			if c := a.opened[d]; c != nil {
				c.Notify("textDocument/didClose", map[string]any{"textDocument": map[string]string{"uri": lsp.URI(old)}})
				delete(a.opened, d)
			}
			delete(a.diagnostics, lsp.URI(old))
			d.Path = path
			delete(a.highlightPending, d)
			a.scheduleHighlight(d)
			a.startLanguage(d)
			a.scanTree()
			a.refreshGit()
		}()
	})
}
func (a *App) restartLanguages() {
	a.languageSeq++
	for _, c := range a.clients {
		go c.Close()
	}
	a.clients = map[string]*lsp.Client{}
	a.opened = map[*editor.Document]*lsp.Client{}
	a.serverErrors = map[string]string{}
	a.starting = map[string]bool{}
	for _, d := range a.docs {
		a.startLanguage(d)
	}
	a.notify("Restarting language servers")
}
