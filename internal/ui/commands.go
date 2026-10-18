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
		{"file.new", "File: New File…", "ctrl+n", a.newFile},
		{"file.newUntitled", "File: New Untitled File", "", func() { a.newUntitled(a.newFileDirectory()) }},
		{"file.open", "File: Open File…", "ctrl+o", a.fileOpen},
		{"file.quickOpen", "File: Quick Open…", "ctrl+p", a.quickOpen},
		{"file.folder", "File: Open Folder…", "", a.switchWorkspace},
		{"file.save", "File: Save", "ctrl+s", func() { a.save(a.view().Doc, nil) }},
		{"file.saveAs", "File: Save As…", "ctrl+shift+s", func() { a.saveAs(a.view().Doc, nil) }},
		{"file.saveAll", "File: Save All", "", a.saveAll},
		{"file.close", "File: Close Tab", "ctrl+w", func() { a.closeDocument(a.view().Doc) }},
		{"file.rename", "File: Rename…", "", a.renameFile},
		{"file.delete", "File: Delete Current File…", "", func() { a.confirmDeleteFile(a.view().Doc.Path) }},
		{"explorer.delete", "Explorer: Delete Selected File…", "", a.deleteExplorerFile},
		{"file.reload", "File: Reload from Disk", "", a.reload},
		{"app.quit", "Quit SunBook", "ctrl+q", a.confirmExit},
		{"app.commands", "Show Command Palette", "ctrl+shift+p", a.palette},
		{"app.help", "Help: Keyboard Shortcuts", "", a.help},
		{"plugins.manage", "Plugins: Manage Installed Plugins", "", func() { a.showSidebar("Plugins") }},
		{"plugins.lastResult", "Plugins: Show Last Result", "", a.showLastPluginResult},
		{"plugins.previewApply", "Plugins: Format Preview Document", "", a.applyPluginSplit},
		{"plugins.previewToggle", "Plugins: Toggle Preview Source / Reading", "", a.togglePluginSplitMode},
		{"plugins.previewClose", "Plugins: Close Formatting Preview", "", func() {
			if a.pluginPreview() != nil {
				a.closePluginSplit()
			}
		}},
		{"plugins.install", "Plugins: Install Local Plugin…", "", a.installPlugin},
		{"plugins.refresh", "Plugins: Refresh Installed Plugins", "", a.refreshPlugins},
		{"activity.plugins", "Activity: Toggle Plugins", "", func() { a.toggleSidebarView("Plugins") }},
		{"app.settings", "Preferences: Settings…", "", a.settings},
		{"view.fileIcons", "Preferences: File Icon Theme…", "", a.chooseFileIcons},
		{"view.actionIcons", "Preferences: Button & Sidebar Icons…", "", a.chooseActionIcons},
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
		{"view.sidebar", "View: Toggle Sidebar", "ctrl+b", a.toggleSidebar},
		{"view.theme", "Preferences: Color Theme…", "ctrl+k ctrl+t", a.chooseTheme},
		{"view.explorer", "View: Explorer", "ctrl+shift+e", func() { a.showSidebar("Explorer") }},
		{"activity.explorer", "Activity: Toggle Explorer", "", func() { a.toggleSidebarView("Explorer") }},
		{"activity.search", "Activity: Toggle Search", "", func() { a.toggleSidebarView("Search") }},
		{"activity.git", "Activity: Toggle Source Control", "", func() { a.toggleSidebarView("Git") }},
		{"explorer.filter", "Explorer: Filter Files", "", a.focusTreeFilter},
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
		{"view.panel", "View: Toggle Panel", "ctrl+j", func() {
			a.panelVisible = !a.panelVisible
			if !a.panelVisible {
				a.focus = "editor"
			}
		}},
		{"view.terminal", "View: Focus Terminal", "f4", a.toggleTerminal},
		{"view.showTerminal", "Terminal: Show Terminal", "", a.showTerminal},
		{"view.panelDock", "View: Toggle Panel Position (Bottom / Right)", "", func() {
			if a.panelDock == "right" {
				a.panelDock = "bottom"
			} else {
				a.panelDock = "right"
			}
		}},
		{"view.hidePanel", "View: Hide Bottom Panel", "", a.hidePanel},
		{"terminal.kill", "Terminal: Kill Shell Session", "", a.killTerminal},
		{"terminal.new", "Terminal: New Terminal", "", a.newTerminal},
		{"terminal.switch", "Terminal: Switch Session…", "", a.chooseTerminal},
		{"terminal.next", "Terminal: Next Session", "", func() { a.nextTerminal(1) }},
		{"terminal.previous", "Terminal: Previous Session", "", func() { a.nextTerminal(-1) }},
		{"view.nextFocus", "View: Focus Next Panel", "f6", a.cycleFocus},
		{"view.problems", "View: Problems", "", func() { a.panel = "Problems"; a.panelVisible = true; a.focus = "problems" }},
		{"view.git", "View: Source Control", "ctrl+shift+g", func() { a.showSidebar("Git") }},
		{"search.run", "Search: Run Workspace Search", "", a.startWorkspaceSearch},
		{"search.clear", "Search: Clear Workspace Search", "", a.clearWorkspaceSearch},
		{"search.stop", "Search: Stop Workspace Search", "", a.stopWorkspaceSearch},
		{"search.case", "Search: Toggle Match Case", "", func() { a.searchOptions.Case = !a.searchOptions.Case; a.rerunSearchOptions() }},
		{"search.word", "Search: Toggle Whole Word", "", func() { a.searchOptions.Word = !a.searchOptions.Word; a.rerunSearchOptions() }},
		{"search.regex", "Search: Toggle Regular Expression", "", func() { a.searchOptions.Regex = !a.searchOptions.Regex; a.rerunSearchOptions() }},
		{"search.paths", "Search: Include / Exclude Files", "", a.toggleSearchPaths},
		{"search.error", "Search: Show Error", "", func() { a.showText("Search Error", a.searchError) }},
		{"language.complete", "Language: Trigger Completion", "ctrl+space", func() { a.requestCompletion(true) }},
		{"language.hover", "Language: Hover", "alt+h", a.hover},
		{"language.definition", "Language: Go to Definition", "f12", func() { a.definition(false) }},
		{"language.references", "Language: Find References", "shift+f12", func() { a.definition(true) }},
		{"language.rename", "Language: Rename Symbol", "f2", a.renameSymbol},
		{"language.format", "Language: Format Document", "alt+shift+f", a.format},
		{"language.restart", "Language: Restart Language Servers", "", a.restartLanguages},
		{"git.branches", "Git: Manage Branches…", "", a.branchManager},
		{"git.refresh", "Git: Refresh", "", a.refreshGit},
		{"git.stage", "Git: Stage Selected File", "", func() { a.gitAction(true) }},
		{"git.unstage", "Git: Unstage Selected File", "", func() { a.gitAction(false) }},
		{"git.diff", "Git: Show Worktree Diff", "", func() { a.gitDiff(false) }},
		{"git.stagedDiff", "Git: Show Staged Diff", "", func() { a.gitDiff(true) }},
		{"git.selectedDiff", "Git: Show Selected Diff", "", a.selectedGitDiff},
		{"git.open", "Git: Open Selected File", "", a.openGitFile},
		{"git.commit", "Git: Commit Staged Changes…", "", a.commit},
		{"git.more", "Git: More Actions", "", a.gitMore},
		{"git.stageAll", "Git: Stage All Repository Changes", "", func() { a.gitAllAction(true) }},
		{"git.unstageAll", "Git: Unstage All Files", "", func() { a.gitAllAction(false) }},
		{"git.stageCHANGES", "Git: Stage Tracked Changes", "", func() { a.gitStageGroup("CHANGES") }},
		{"git.stageUNTRACKED", "Git: Stage Untracked Files", "", func() { a.gitStageGroup("UNTRACKED") }},
		{"git.stageCONFLICTS", "Git: Stage Resolved Conflicts", "", func() { a.gitStageGroup("CONFLICTS") }},
		{"git.toggleCHANGES", "Git: Expand / Collapse Changes", "", func() { a.toggleGitGroup("CHANGES") }},
		{"git.toggleUNTRACKED", "Git: Expand / Collapse Untracked", "", func() { a.toggleGitGroup("UNTRACKED") }},
		{"git.toggleSTAGED", "Git: Expand / Collapse Staged", "", func() { a.toggleGitGroup("STAGED") }},
		{"git.toggleCONFLICTS", "Git: Expand / Collapse Conflicts", "", func() { a.toggleGitGroup("CONFLICTS") }},
		{"git.history", "Git: Recent Commits", "", func() { a.gitHistory(false) }},
		{"git.fileHistory", "Git: Selected File History", "", func() { a.gitHistory(true) }},
		{"git.toggleLog", "Git: Expand / Restore Log", "", func() {
			a.gitLogExpanded = !a.gitLogExpanded
			if a.gitLogExpanded {
				a.focus = "gitLog"
			}
		}},
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
	if a.pluginPreview() != nil && id == "view.other" {
		if a.focus == "markdown" {
			a.focus = "editor"
		} else {
			a.focus = "markdown"
		}
		return
	}
	if a.comparing() {
		if id == "view.other" {
			a.comparison.Pane = 1 - a.comparison.Pane
			a.focus = "diff"
			return
		}
		if id == "file.close" {
			a.closeComparison()
			return
		}
		if comparisonReadOnlyCommand(id) {
			a.notify("Read-only comparison · press o to open the working file")
			return
		}
	}
	for _, cmd := range a.commands {
		if cmd.ID == id {
			a.completion = nil
			a.completionDue = zeroTime
			cmd.Run()
			a.reveal()
			return
		}
	}
	if strings.HasPrefix(id, "plugin.") {
		a.notify("Plugin command unavailable; open Plugins to enable or review it")
		return
	}
	a.notify("Unknown command: " + id)
}
func (a *App) palette() {
	var items []item
	for _, cmd := range a.commands {
		cmd := cmd
		items = append(items, item{Label: a.actionLabel(cmd.ID, cmd.Label, false), Detail: strings.Join(a.commandKeys(cmd.ID), " / "), Action: func() { a.execute(cmd.ID) }})
	}
	a.setModal(&modal{Title: "Command Palette", InputEnabled: true, Filter: true, Items: items, Hint: "Type a command · Enter runs · Esc closes"})
}
func (a *App) commandKeys(id string) []string {
	var keys []string
	for key, bound := range a.bindings {
		if bound == id {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}
func (a *App) help() {
	var body strings.Builder
	body.WriteString("KEYBOARD SHORTCUTS\nCurrent keybindings, including your configured overrides.\n\n")
	for _, c := range a.commands {
		keys := a.commandKeys(c.ID)
		if len(keys) == 0 {
			continue
		}
		fmt.Fprintf(&body, "%-26s %s\n", strings.Join(keys, " / "), c.Label)
	}
	body.WriteString(`
EDITOR & MOUSE
Arrows / Home / End       Move cursor; Shift extends selection
Ctrl+Left / Right        Move by word
Click / drag             Place cursor / select text
Double / triple click    Select word / line
Alt+click                Add cursor
Mouse wheel              Scroll; drag borders to resize panes
Content scrollbar        Click or drag the one-column track to scroll
Drag file tab            Reorder tabs; drop on editor edge to split
Double-click file tab    Keep preview open (italic filename = preview)

OPEN / QUICK OPEN
Ctrl+L                   Edit folder path in Open
Enter / Go               Navigate the edited folder path
Alt+Up / Up button       Browse the parent folder
Tab                      Complete file / folder; leave path field
Ctrl+Enter               Open selected file in the other editor
file:line:column         Jump when opening, e.g. main.go:20:5
Esc                      Cancel path edit, then close the picker

EXPLORER & SEARCH
Single / double click    Preview file / keep file open
Edit / save preview      Keep file open automatically
/ or Ctrl+F (Explorer)   Focus the filename filter
Up / Down + Enter        Select and open a result
Delete (Explorer)        Delete selected file, after confirmation
New / Ctrl+N             Create in the selected folder / file's parent
Click workspace name     Use workspace root for new files
Esc (Explorer filter)    Clear the filter
Tab (Search results)     Return to search input
Click file heading       Expand / collapse search results
Aa / W / .* / ...        Search case / word / regex / file globs

SOURCE CONTROL
s / u                    Stage / unstage selected file
S / U                    Stage / unstage all files
Enter / d                View selected comparison
D                        View staged comparison
o                        Open working file for editing
c                        Focus commit message; Enter commits staged files
Ctrl+J (message)         Insert a newline; arrows navigate wrapped lines
Enter / Space (group)    Expand / collapse group; Left / Right folds / expands
/ or Ctrl+F              Filter changed paths
h / Tab                  Focus log / switch lists
Enter / click (log)      Expand commit files / folders; view file comparison
Left / Right (log)       Collapse / expand the selected tree node
d (log)                  View the full commit patch
b                        Manage local and remote branches
m                        More Git actions

PLUGINS
Enter (Plugins sidebar)  Open plugin actions and commands
i / r (Plugins sidebar)  Install a local folder / refresh installed plugins
Enable plugin            Review permissions and start a trusted local process
Restart plugin           Recover a quarantined plugin; reset retry circuit
Plugin edits             Preview before applying; Ctrl+Z undoes the transaction

COMPARISON
Tab                      Switch comparison pane
Up / Down / wheel        Scroll both sides
Left / Right             Pan the focused pane
n / p                    Next / previous change
o                        Open working file for editing
Esc                      Close comparison

TERMINAL & DIALOGS
F4                       Open terminal / switch shell and editor focus
Ctrl+J                   Show / hide panel; keep all shells running
Terminal +               Create an independent shell session
Session number / name    Dropdown: click or ↑/↓ + Enter to switch
Right / Bottom           Dock panel; drag its divider to resize
Hide                     Hide terminal while keeping shell running
Kill                     Close current session; switch to another if available
Ordinary terminal keys   Sent to the shell, including Ctrl+C
Ctrl+U (input)           Clear input
New / Ctrl+N             Enter filename, or leave blank for Untitled
Save an untitled file    Enter filename; Esc keeps the draft
Up / Down / Enter        Choose / accept a dialog item
Esc                      Close a dialog

All actions are available under More / the command palette.
Settings gear            File icons, button/sidebar icons and color theme
`)
	a.showText("Keyboard Shortcuts", body.String())
	a.modal.Hint = "↑/↓ or wheel scrolls · PgUp/PgDn pages · ←/→ pans · Esc closes"
}
func (a *App) nextTab(dir int) {
	if len(a.docs) == 0 {
		return
	}
	n := len(a.docs)
	index := 0
	for i, d := range a.docs {
		if d == a.view().Doc {
			index = i
		}
	}
	if a.comparison != nil {
		if a.comparing() {
			index = n
		}
		n++
	}
	index = (index + dir + n) % n
	if index == len(a.docs) {
		a.comparison.Active = true
		a.focus = "diff"
	} else {
		a.activate(a.docs[index])
	}
}

func comparisonReadOnlyCommand(id string) bool {
	return strings.HasPrefix(id, "edit.") || strings.HasPrefix(id, "language.") || id == "file.save" || id == "file.saveAs" || id == "file.rename" || id == "file.delete" || id == "file.reload" || id == "search.find" || id == "search.replace" || id == "navigate.line"
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
	editingOpenPath := m != nil && m.Open != nil && m.Open.EditingPath
	go func() {
		s, system := a.clipboard.Read()
		a.post(func() {
			if a.modal != m || a.focus != focus || a.view() != v || v.Doc.Buffer.Version != version || (m != nil && m.Open != nil && m.Open.EditingPath != editingOpenPath) {
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
	if a.terminalMenu != nil {
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
	if a.focus == "searchInclude" || a.focus == "searchExclude" {
		a.insertSearchPath(s)
		return
	}
	if a.focus == "searchInput" {
		a.insertSearchQuery(s)
		return
	}
	if a.focus == "gitMessage" || a.focus == "gitFilter" {
		a.insertGitInput(s)
		return
	}
	if a.comparing() {
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
				c.CloseDocument(old)
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
