package ui

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/gdamore/tcell/v3"
	"sunbook/internal/editor"
	"sunbook/internal/text"
)

var zeroTime time.Time

// LF in a paste is reported as Ctrl+J by traditional terminal parsers. It
// must be restored as data instead of being discarded or dispatched as a key.
func pasteText(e *tcell.EventKey) string {
	switch e.Key() {
	case tcell.KeyEnter:
		return "\r"
	case tcell.KeyTab:
		return "\t"
	case tcell.KeyEsc:
		return "\x1b"
	}
	name := keyName(e)
	if len(name) == 6 && strings.HasPrefix(name, "ctrl+") && name[5] >= 'a' && name[5] <= 'z' {
		return string([]byte{name[5] - 'a' + 1})
	}
	return e.Str()
}

func keyName(e *tcell.EventKey) string {
	k := e.Key()
	m := e.Modifiers()
	name := ""
	if k >= tcell.KeyCtrlA && k <= tcell.KeyCtrlZ {
		name = string(rune('a' + k - tcell.KeyCtrlA))
		m |= tcell.ModCtrl
	} else if k == tcell.KeyRune {
		name = strings.ToLower(e.Str())
	} else if k >= 32 && k < 127 {
		name = strings.ToLower(string(rune(k)))
	} else {
		switch k {
		case tcell.KeyNUL:
			name = "space"
			m |= tcell.ModCtrl
		case tcell.KeyBacktab:
			name = "tab"
			m |= tcell.ModShift
		case tcell.KeyPgUp:
			name = "pageup"
		case tcell.KeyPgDn:
			name = "pagedown"
		case tcell.KeyEsc:
			name = "escape"
		default:
			name = strings.ToLower(tcell.KeyNames[k])
		}
	}
	if name == " " {
		name = "space"
	}
	prefix := ""
	if m&tcell.ModCtrl != 0 {
		prefix += "ctrl+"
	}
	if m&tcell.ModAlt != 0 {
		prefix += "alt+"
	}
	if m&tcell.ModShift != 0 {
		prefix += "shift+"
	}
	return prefix + name
}
func (a *App) key(e *tcell.EventKey) {
	name := keyName(e)
	if a.modal != nil {
		a.modalKey(e, name)
		return
	}
	if name == "f1" || name == "f4" || name == "f6" {
		if id := a.bindings[name]; id != "" {
			a.execute(id)
			return
		}
	}
	if id := a.bindings[name]; id == "view.panel" || id == "view.hidePanel" || id == "view.sidebar" || id == "view.explorer" || id == "view.git" || id == "search.workspace" {
		a.execute(id)
		return
	}
	if a.focus == "terminal" {
		if a.term != nil && !a.termExited {
			a.termOffset = 0
			a.term.Key(terminalKey(e))
		}
		return
	}
	if id := a.bindings[name]; id == "file.open" || id == "file.quickOpen" {
		a.execute(id)
		return
	}
	if a.focus == "searchInput" {
		a.searchInputKey(e, name)
		return
	}
	if a.focus == "searchInclude" || a.focus == "searchExclude" {
		a.searchPathKey(e, name)
		return
	}
	if a.focus == "search" {
		a.searchResultsKey(e, name)
		return
	}
	if a.focus == "gitMessage" || a.focus == "gitFilter" {
		a.gitInputKey(e, name)
		return
	}
	if a.focus == "gitLog" {
		a.gitLogKey(e, name)
		return
	}
	if a.comparing() && (a.focus == "diff" || a.focus == "editor") {
		a.comparisonKey(e, name)
		return
	}
	if a.focus == "git" && (name == "ctrl+f" || name == "/") {
		a.focusGitFilter()
		return
	}
	if a.focus == "filter" {
		a.treeFilterKey(e, name)
		return
	}
	if a.focus == "tree" && (name == "ctrl+f" || name == "/") {
		a.focusTreeFilter()
		return
	}
	if a.completion != nil {
		switch name {
		case "escape":
			a.completion = nil
			return
		case "up":
			a.completion.Selected = (a.completion.Selected - 1 + len(a.completion.Items)) % len(a.completion.Items)
			return
		case "down":
			a.completion.Selected = (a.completion.Selected + 1) % len(a.completion.Items)
			return
		case "enter", "tab":
			a.acceptCompletion()
			a.reveal()
			return
		}
		a.completion = nil
	}
	if a.prefix != "" {
		id := a.bindings[a.prefix+" "+name]
		a.prefix = ""
		if id != "" {
			a.execute(id)
			return
		}
	}
	if id := a.bindings[name]; id != "" {
		a.execute(id)
		return
	}
	for key, id := range a.bindings {
		if id != "" && strings.HasPrefix(key, name+" ") {
			a.prefix = name
			a.notify(name + " …")
			return
		}
	}
	if a.focus == "tree" {
		switch name {
		case "up":
			a.treeSelected = max(0, a.treeSelected-1)
		case "down":
			a.treeSelected = min(len(a.tree)-1, a.treeSelected+1)
		case "enter", "right":
			a.toggleTree()
		case "left":
			if a.treeSelected >= 0 && a.treeSelected < len(a.tree) {
				p := a.tree[a.treeSelected].Path
				a.expanded[p] = false
				a.scanTree()
			}
		case "escape":
			if a.treeQuery != "" {
				a.setTreeQuery("")
			} else {
				a.focus = "editor"
			}
		default:
			if e.Str() != "" && e.Modifiers()&(tcell.ModCtrl|tcell.ModAlt|tcell.ModMeta) == 0 {
				a.focusTreeFilter()
				a.insertTreeQuery(e.Str())
			}
		}
		return
	}
	if a.focus == "git" {
		switch name {
		case "up":
			a.moveGitSelection(-1)
		case "down":
			a.moveGitSelection(1)
		case "pageup":
			a.moveGitSelection(-max(1, a.gitChangesRect.H))
		case "pagedown":
			a.moveGitSelection(max(1, a.gitChangesRect.H))
		case "home":
			a.gitSelected = 0
			a.moveGitSelection(0)
		case "end":
			a.gitSelected = len(a.gitEntries()) - 1
			a.moveGitSelection(0)
		case "s", "shift+s":
			if e.Str() == "S" || name == "shift+s" {
				a.gitAllAction(true)
			} else {
				a.gitAction(true)
			}
		case "u", "shift+u":
			if e.Str() == "U" || name == "shift+u" {
				a.gitAllAction(false)
			} else {
				a.gitAction(false)
			}
		case "d", "shift+d", "enter":
			if e.Str() == "D" || name == "shift+d" {
				a.gitDiff(true)
			} else {
				a.selectedGitDiff()
			}
		case "o":
			a.openGitFile()
		case "c":
			a.focusGitMessage()
		case "r":
			a.refreshGit()
		case "h":
			a.gitHistory(false)
		case "tab":
			a.focus = "gitLog"
			a.moveGitLog(0)
		case "m":
			a.gitMore()
		case "escape":
			a.gitViewSeq++
			if a.gitFilter != "" {
				a.setGitFilter("")
			} else {
				a.focus = "editor"
			}
		}
		return
	}
	if a.focus == "problems" {
		switch name {
		case "up":
			a.problemSelected = max(0, a.problemSelected-1)
		case "down":
			a.problemSelected = min(len(a.problems())-1, a.problemSelected+1)
		case "enter":
			p := a.problems()
			if a.problemSelected >= 0 && a.problemSelected < len(p) {
				p[a.problemSelected].Action()
			}
		case "escape":
			a.focus = "editor"
		}
		return
	}
	v := a.view()
	selecting := e.Modifiers()&tcell.ModShift != 0
	word := e.Modifiers()&(tcell.ModCtrl|tcell.ModAlt) != 0
	changed := false
	switch e.Key() {
	case tcell.KeyLeft:
		v.Move(-1, 0, selecting, word, a.cfg.TabSize)
	case tcell.KeyRight:
		v.Move(1, 0, selecting, word, a.cfg.TabSize)
	case tcell.KeyUp:
		v.Move(0, -1, selecting, false, a.cfg.TabSize)
	case tcell.KeyDown:
		v.Move(0, 1, selecting, false, a.cfg.TabSize)
	case tcell.KeyPgUp:
		v.Move(0, -max(1, a.rects[a.active].H-2), selecting, false, a.cfg.TabSize)
	case tcell.KeyPgDn:
		v.Move(0, max(1, a.rects[a.active].H-2), selecting, false, a.cfg.TabSize)
	case tcell.KeyHome:
		r, _ := v.Doc.Buffer.Position(v.Head())
		p := v.Doc.Buffer.LineStart(r)
		if e.Modifiers()&tcell.ModCtrl != 0 {
			p = 0
		}
		v.Set(p, selecting)
	case tcell.KeyEnd:
		r, _ := v.Doc.Buffer.Position(v.Head())
		p := v.Doc.Buffer.LineEnd(r)
		if e.Modifiers()&tcell.ModCtrl != 0 {
			p = v.Doc.Buffer.Len()
		}
		v.Set(p, selecting)
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		changed = v.Delete(true, word)
	case tcell.KeyDelete:
		changed = v.Delete(false, word)
	case tcell.KeyEnter:
		a.typeInput("\n")
		a.reveal()
		return
	case tcell.KeyTab:
		hasSelection := false
		for _, s := range v.Selections {
			if !s.Empty() {
				hasSelection = true
			}
		}
		if hasSelection {
			a.indent(selecting)
		} else {
			pad := "\t"
			if a.cfg.InsertSpaces {
				pad = strings.Repeat(" ", a.cfg.TabSize)
			}
			changed = v.Insert(pad)
		}
	case tcell.KeyBacktab:
		a.indent(true)
	case tcell.KeyEsc:
		v.Selections = []text.Selection{{Anchor: v.Head(), Head: v.Head()}}
	default:
		if e.Str() != "" && e.Modifiers()&(tcell.ModCtrl|tcell.ModAlt|tcell.ModMeta) == 0 {
			a.typeInput(e.Str())
			a.reveal()
			return
		}
	}
	if changed {
		a.changed(v.Doc)
	}
	a.reveal()
}
func (a *App) typeInput(s string) {
	v := a.view()
	b := v.Doc.Buffer
	source := b.String()
	v.Selections = text.NormalizeSelections(v.Selections)
	before := append([]text.Selection(nil), v.Selections...)
	var edits []text.Edit
	var after []text.Selection
	delta := 0
	pairs := map[string]string{"(": ")", "[": "]", "{": "}", "\"": "\"", "'": "'"}
	pad := "\t"
	if a.cfg.InsertSpaces {
		pad = strings.Repeat(" ", a.cfg.TabSize)
	}
	for _, sel := range before {
		lo, hi := sel.Range()
		insert := s
		caret := len(s)
		if sel.Empty() && strings.Contains(")]}\"'", s) && len(s) == 1 && lo < len(source) && source[lo:lo+1] == s {
			after = append(after, text.Selection{Anchor: lo + delta + 1, Head: lo + delta + 1})
			continue
		}
		if close, ok := pairs[s]; ok && sel.Empty() {
			nextSpace := lo == len(source)
			if !nextSpace {
				r, _ := utf8.DecodeRuneInString(source[lo:])
				nextSpace = unicode.IsSpace(r) || strings.ContainsRune(")]},;", r)
			}
			if nextSpace {
				insert += close
			}
		}
		if s == "\n" {
			row, _ := b.Position(lo)
			line := b.Line(row)
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			if lo < b.LineStart(row)+len(indent) {
				indent = source[b.LineStart(row):lo]
			}
			insert = "\n" + indent
			beforePart := strings.TrimSpace(source[b.LineStart(row):lo])
			if strings.HasSuffix(beforePart, "{") || strings.HasSuffix(beforePart, "[") || strings.HasSuffix(beforePart, "(") {
				insert += pad
			}
			caret = len(insert)
			if lo < len(source) && strings.ContainsRune("}])", rune(source[lo])) && len(insert) > 1 && strings.HasSuffix(insert, pad) {
				insert += "\n" + indent
			}
		}
		edits = append(edits, text.Edit{Start: lo, End: hi, Text: insert})
		p := lo + delta + caret
		after = append(after, text.Selection{Anchor: p, Head: p})
		delta += len(insert) - (hi - lo)
	}
	if len(edits) > 0 && b.Apply(edits, before, after) {
		v.Selections = after
		a.changed(v.Doc)
	} else if len(edits) == 0 {
		v.Selections = after
	}
}
func (a *App) cycleFocus() {
	if a.focus == "editor" || a.focus == "diff" {
		if a.sidebar && a.sideRect.W > 0 {
			a.focusSidebar()
			return
		}
		if a.panelVisible {
			a.focus = strings.ToLower(a.panel)
			if a.focus == "terminal" {
				a.startTerminal()
			}
		}
		return
	}
	if sidebarFocus(a.focus) && a.panelVisible {
		a.focus = strings.ToLower(a.panel)
		if a.focus == "terminal" {
			a.startTerminal()
		}
		return
	}
	a.focus = "editor"
}
func terminalKey(e *tcell.EventKey) uv.Key {
	k := uv.Key{Text: e.Str()}
	m := e.Modifiers()
	if m&tcell.ModCtrl != 0 {
		k.Mod |= uv.ModCtrl
	}
	if m&tcell.ModShift != 0 {
		k.Mod |= uv.ModShift
	}
	if m&tcell.ModAlt != 0 {
		k.Mod |= uv.ModAlt
	}
	if e.Key() >= tcell.KeyCtrlA && e.Key() <= tcell.KeyCtrlZ {
		k.Code = rune('a' + e.Key() - tcell.KeyCtrlA)
		k.Mod |= uv.ModCtrl
		k.Text = ""
		return k
	}
	keys := map[tcell.Key]rune{tcell.KeyEnter: uv.KeyEnter, tcell.KeyTab: uv.KeyTab, tcell.KeyBacktab: uv.KeyTab, tcell.KeyBackspace: uv.KeyBackspace, tcell.KeyBackspace2: uv.KeyBackspace, tcell.KeyEsc: uv.KeyEscape, tcell.KeyUp: uv.KeyUp, tcell.KeyDown: uv.KeyDown, tcell.KeyLeft: uv.KeyLeft, tcell.KeyRight: uv.KeyRight, tcell.KeyHome: uv.KeyHome, tcell.KeyEnd: uv.KeyEnd, tcell.KeyPgUp: uv.KeyPgUp, tcell.KeyPgDn: uv.KeyPgDown, tcell.KeyDelete: uv.KeyDelete, tcell.KeyInsert: uv.KeyInsert}
	if code, ok := keys[e.Key()]; ok {
		k.Code = code
		k.Text = ""
		if e.Key() == tcell.KeyBacktab {
			k.Mod |= uv.ModShift
		}
	} else if e.Key() >= tcell.KeyF1 && e.Key() <= tcell.KeyF63 {
		k.Code = uv.KeyF1 + rune(e.Key()-tcell.KeyF1)
		k.Text = ""
	} else if e.Key() == tcell.KeyNUL {
		k.Code = ' '
		k.Mod |= uv.ModCtrl
		k.Text = ""
	} else {
		r, _ := utf8.DecodeRuneInString(e.Str())
		k.Code = r
		if e.Str() == "" && e.Key() >= 32 && e.Key() < 127 {
			k.Code = rune(e.Key())
		}
		if k.Mod&uv.ModCtrl != 0 {
			k.Text = ""
		}
	}
	return k
}

func (a *App) setModal(m *modal) {
	a.gitViewSeq++
	a.completion = nil
	a.completionDue = zeroTime
	a.modal = m
	m.Cursor = len(m.Input)
	a.filterModal()
}
func (a *App) prompt(title, initial string, submit func(string)) {
	a.setModal(&modal{Title: title, Input: initial, InputEnabled: true, Hint: "Enter confirms · Ctrl+U clears · Esc cancels", Submit: func(s string, _ int) { submit(s) }})
}
func (a *App) choose(title string, items []item) {
	a.setModal(&modal{Title: title, Items: items, Hint: "↑/↓ choose · Enter accepts · Esc cancels"})
}
func (a *App) showText(title, body string) {
	a.setModal(&modal{Title: title, Body: strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n"), Hint: "↑/↓ or wheel scrolls · Esc closes"})
}
func fuzzy(hay, needle string) bool {
	hay = strings.ToLower(hay)
	needle = strings.ToLower(needle)
	if strings.Contains(hay, needle) {
		return true
	}
	r := []rune(needle)
	i := 0
	for _, c := range hay {
		if i < len(r) && c == r[i] {
			i++
		}
	}
	return i == len(r)
}
func (a *App) filterModal() {
	m := a.modal
	if m == nil {
		return
	}
	if m.Open != nil {
		a.updateOpenModal(m)
		return
	}
	m.Visible = nil
	for _, it := range m.Items {
		if !m.Filter || fuzzy(it.Label, m.Input) {
			m.Visible = append(m.Visible, it)
		}
	}
	m.Selected = max(0, min(m.Selected, len(m.Visible)-1))
	m.Top = 0
	if m.Changed != nil {
		m.Changed(m.Input)
	}
}
func (a *App) modalInsert(s string) {
	m := a.modal
	if m == nil || !m.InputEnabled {
		return
	}
	if m.Open != nil && m.Open.EditingPath {
		a.insertOpenPath(m, s)
		return
	}
	m.Input = m.Input[:m.Cursor] + s + m.Input[m.Cursor:]
	m.Cursor += len(s)
	a.filterModal()
}
func (a *App) modalAccept() {
	m := a.modal
	if m == nil {
		return
	}
	if m.Accept != nil {
		m.Accept(false)
		return
	}
	s := m.Input
	idx := m.Selected
	a.modal = nil
	if m.Submit != nil {
		m.Submit(s, idx)
	} else if idx >= 0 && idx < len(m.Visible) && m.Visible[idx].Action != nil {
		m.Visible[idx].Action()
	}
	a.reveal()
}
func (a *App) modalKey(e *tcell.EventKey, name string) {
	m := a.modal
	if a.openPathKey(m, e, name) {
		return
	}
	switch name {
	case "ctrl+enter":
		if m.Accept != nil {
			m.Accept(true)
			return
		}
	case "tab":
		if m.Complete != nil {
			m.Complete()
			return
		}
	case "alt+up":
		if m.Parent != nil {
			m.Parent()
			return
		}
	}
	if !m.InputEnabled && len(m.Body) > 0 {
		switch name {
		case "left":
			m.BodyLeft = max(0, m.BodyLeft-8)
			return
		case "right":
			maxWidth := 0
			for _, line := range m.Body {
				maxWidth = max(maxWidth, text.Width(line, a.cfg.TabSize))
			}
			m.BodyLeft = min(max(0, maxWidth-(a.modalRect.W-4)), m.BodyLeft+8)
			return
		case "home":
			m.Top = 0
			m.BodyLeft = 0
			return
		case "end":
			m.Top = max(0, len(m.Body)-max(1, m.ListH))
			return
		}
	}
	switch name {
	case "ctrl+up":
		m.BodyTop = max(0, m.BodyTop-1)
		return
	case "ctrl+down":
		m.BodyTop = min(max(0, len(m.Body)-5), m.BodyTop+1)
		return
	case "escape":
		a.modal = nil
		return
	case "enter":
		if len(m.Items) > 0 || m.Submit != nil || m.Accept != nil {
			a.modalAccept()
		} else {
			a.modal = nil
		}
		return
	case "ctrl+v":
		a.readClipboard()
		return
	case "ctrl+u":
		if m.InputEnabled {
			m.Input = ""
			m.Cursor = 0
			a.filterModal()
		}
		return
	case "up":
		if len(m.Visible) > 0 {
			m.Selected = max(0, m.Selected-1)
		} else {
			m.Top = max(0, m.Top-1)
		}
		return
	case "down":
		if len(m.Visible) > 0 {
			m.Selected = min(len(m.Visible)-1, m.Selected+1)
		} else {
			m.Top = min(max(0, len(m.Body)-1), m.Top+1)
		}
		return
	case "pageup":
		if len(m.Visible) > 0 {
			m.Selected = max(0, m.Selected-10)
		} else {
			m.Top = max(0, m.Top-10)
		}
		return
	case "pagedown":
		if len(m.Visible) > 0 {
			m.Selected = min(len(m.Visible)-1, m.Selected+10)
		} else {
			m.Top = min(max(0, len(m.Body)-1), m.Top+10)
		}
		return
	}
	if !m.InputEnabled {
		return
	}
	switch name {
	case "left":
		m.Cursor = text.Prev(m.Input, m.Cursor)
	case "right":
		m.Cursor = text.Next(m.Input, m.Cursor)
	case "home", "ctrl+a":
		m.Cursor = 0
	case "end", "ctrl+e":
		m.Cursor = len(m.Input)
	case "backspace", "backspace2":
		p := text.Prev(m.Input, m.Cursor)
		m.Input = m.Input[:p] + m.Input[m.Cursor:]
		m.Cursor = p
		a.filterModal()
	case "delete":
		p := text.Next(m.Input, m.Cursor)
		m.Input = m.Input[:m.Cursor] + m.Input[p:]
		a.filterModal()
	default:
		if e.Str() != "" && e.Modifiers()&(tcell.ModCtrl|tcell.ModAlt) == 0 {
			a.modalInsert(e.Str())
		}
	}
}

func (a *App) mouse(e *tcell.EventMouse) {
	x, y := e.Position()
	buttons := e.Buttons()
	previous := a.lastButtons
	press := buttons&tcell.Button1 != 0
	fresh := press && a.lastButtons&tcell.Button1 == 0
	wheel := 0
	if buttons&tcell.WheelUp != 0 {
		wheel = -3
	}
	if buttons&tcell.WheelDown != 0 {
		wheel = 3
	}
	a.lastButtons = buttons
	if a.modal != nil {
		m := a.modal
		if a.openNavigationMouse(m, x, y, fresh) {
			return
		}
		if wheel != 0 {
			if len(m.Body) > 0 && len(m.Items) > 0 {
				m.BodyTop = max(0, min(max(0, len(m.Body)-5), m.BodyTop+wheel))
			} else if len(m.Visible) > 0 {
				m.Selected = max(0, min(m.Selected+wheel, len(m.Visible)-1))
			} else {
				m.Top = max(0, min(m.Top+wheel, max(0, len(m.Body)-1)))
			}
		}
		if fresh && x > a.modalRect.X && x < a.modalRect.X+a.modalRect.W-1 && y >= m.ListY && y < m.ListY+m.ListH {
			idx := y - m.ListY + m.Top
			if idx >= 0 && idx < len(m.Visible) {
				m.Selected = idx
				if m.Open != nil {
					m.Open.EditingPath = false
					m.Open.SelectPath = false
				}
				a.modalAccept()
			}
		}
		return
	}
	a.hoverCommand = ""
	a.hoverHint = ""
	for _, b := range a.buttons {
		if b.contains(x, y) {
			a.hoverCommand = b.Command
			a.hoverHint = b.Hint
			if fresh {
				a.drag = ""
				if !b.Disabled && a.buttonEnabled(b.Command) {
					a.pressedCommand = b.Command
					a.pressedUntil = time.Now().Add(150 * time.Millisecond)
					a.execute(b.Command)
				}
				return
			}
			break
		}
	}
	if !press {
		a.drag = ""
	}
	if press && a.drag != "" && !fresh {
		switch a.drag {
		case "sidebar":
			a.sidebarWidth = max(20, x-a.sideRect.X)
		case "panel":
			_, h := a.screen.Size()
			a.panelHeight = max(4, h-y-2)
		case "split":
			w, _ := a.screen.Size()
			start := a.rects[0].X
			a.splitRatio = max(.2, min(.8, float64(x-start)/float64(max(1, w-start))))
		case "diffSplit":
			w, _ := a.screen.Size()
			start := a.rects[0].X
			if a.comparing() {
				a.comparison.Ratio = max(.2, min(.8, float64(x-start)/float64(max(1, w-start))))
			}
		case "gitLogResize":
			a.resizeGitLog(y)
		case "select":
			a.active = a.dragGroup
			v := a.view()
			r := a.rects[a.active]
			if y < r.Y {
				v.Top = max(0, v.Top-1)
			} else if y >= r.Y+r.H {
				v.Top = min(v.Doc.Buffer.LineCount()-1, v.Top+1)
			}
			v.Set(a.offsetAt(a.active, x, y), true)
		}
		return
	}
	if fresh {
		if a.sidebar && x == a.sideRect.X+a.sideRect.W {
			a.drag = "sidebar"
			return
		}
		if a.sidebar && a.sidebarView == "Git" && a.gitError == "" && a.sideRect.contains(x, y) && a.gitLogHeader.contains(x, y) {
			a.drag = "gitLogResize"
			a.hoverHint = "Drag up/down to resize Git log"
			return
		}
		if a.panelVisible && a.panelRect.H > 0 && y == a.panelRect.Y-1 && x >= a.panelRect.X+38 {
			a.drag = "panel"
			return
		}
		if a.comparing() && x == a.comparison.Rects[1].X-1 && y >= 2 {
			a.drag = "diffSplit"
			return
		}
		if !a.comparing() && a.split && x == a.rects[1].X-1 {
			a.drag = "split"
			return
		}
	}
	if a.sidebar && a.sideRect.contains(x, y) {
		a.sidebarMouse(e, fresh, wheel)
		return
	}
	if y == 1 && fresh {
		for _, tab := range a.tabs {
			if tab.contains(x, y) {
				if tab.Doc == nil {
					if x >= tab.X+tab.W-2 {
						a.closeComparison()
					} else {
						a.comparison.Active = true
						a.focus = "diff"
					}
					return
				}
				if x >= tab.X+tab.W-2 {
					a.closeDocument(tab.Doc)
				} else {
					a.activate(tab.Doc)
				}
				return
			}
		}
	}
	if a.panelVisible && a.panelRect.contains(x, y) {
		a.panelMouse(e, fresh, wheel, previous)
		return
	}
	if a.comparisonMouse(x, y, wheel, fresh, e.Modifiers()) {
		return
	}
	for i, r := range a.rects {
		if i == 1 && !a.split {
			continue
		}
		if !r.contains(x, y) {
			continue
		}
		v := a.views[i]
		if wheel != 0 {
			v.Top = max(0, min(max(0, v.Doc.Buffer.LineCount()-1), v.Top+wheel))
			return
		}
		if !fresh {
			return
		}
		a.active = i
		a.focus = "editor"
		a.completion = nil
		p := a.offsetAt(i, x, y)
		if time.Since(a.clickTime) < 400*time.Millisecond && abs(x-a.clickX) <= 1 && y == a.clickY {
			a.clickCount = a.clickCount%3 + 1
		} else {
			a.clickCount = 1
		}
		a.clickTime = time.Now()
		a.clickX = x
		a.clickY = y
		if e.Modifiers()&tcell.ModAlt != 0 {
			v.Selections = append(v.Selections, text.Selection{Anchor: p, Head: p})
			v.Selections = text.NormalizeSelections(v.Selections)
		} else if a.clickCount == 2 {
			lo, hi := text.WordRange(v.Doc.Buffer.String(), p)
			v.Selections = []text.Selection{{Anchor: lo, Head: hi}}
		} else if a.clickCount == 3 {
			row, _ := v.Doc.Buffer.Position(p)
			lo, hi := v.Doc.Buffer.LineStart(row), v.Doc.Buffer.LineEnd(row)
			if hi < v.Doc.Buffer.Len() {
				hi++
			}
			v.Selections = []text.Selection{{Anchor: lo, Head: hi}}
		} else {
			v.Set(p, e.Modifiers()&tcell.ModShift != 0)
			a.drag = "select"
			a.dragGroup = i
		}
		return
	}
}
func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
func (a *App) offsetAt(group, x, y int) int {
	v := a.views[group]
	r := a.rects[group]
	row := max(0, min(v.Top+y-r.Y, v.Doc.Buffer.LineCount()-1))
	gutter := a.gutter(v)
	col := max(0, x-r.X-gutter+v.Left)
	return v.Doc.Buffer.LineStart(row) + text.Offset(v.Doc.Buffer.Line(row), col, a.cfg.TabSize)
}
func (a *App) panelMouse(e *tcell.EventMouse, fresh bool, wheel int, previous tcell.ButtonMask) {
	x, y := e.Position()
	switch a.panel {
	case "Terminal":
		if a.term != nil && !a.termExited && a.term.MouseReporting && e.Modifiers()&tcell.ModShift == 0 {
			a.focus = "terminal"
			a.termOffset = 0
			m := uv.Mouse{X: x - a.panelRect.X, Y: y - a.panelRect.Y}
			if e.Modifiers()&tcell.ModCtrl != 0 {
				m.Mod |= uv.ModCtrl
			}
			if e.Modifiers()&tcell.ModAlt != 0 {
				m.Mod |= uv.ModAlt
			}
			buttons := e.Buttons()
			switch {
			case buttons&tcell.Button1 != 0:
				m.Button = uv.MouseLeft
			case buttons&tcell.Button2 != 0:
				m.Button = uv.MouseMiddle
			case buttons&tcell.Button3 != 0:
				m.Button = uv.MouseRight
			case wheel < 0:
				m.Button = uv.MouseWheelUp
			case wheel > 0:
				m.Button = uv.MouseWheelDown
			}
			switch {
			case wheel != 0:
				a.term.Mouse(uv.MouseWheelEvent(m))
			case buttons == 0 && previous&(tcell.Button1|tcell.Button2|tcell.Button3) != 0:
				if previous&tcell.Button1 != 0 {
					m.Button = uv.MouseLeft
				} else if previous&tcell.Button2 != 0 {
					m.Button = uv.MouseMiddle
				} else {
					m.Button = uv.MouseRight
				}
				a.term.Mouse(uv.MouseReleaseEvent(m))
			case fresh || buttons != previous:
				a.term.Mouse(uv.MouseClickEvent(m))
			default:
				a.term.Mouse(uv.MouseMotionEvent(m))
			}
			return
		}
		if wheel != 0 && a.term != nil {
			a.termOffset = max(0, min(a.term.VT.ScrollbackLen(), a.termOffset-wheel))
		}
		if fresh {
			a.focus = "terminal"
			a.startTerminal()
		}
	case "Problems":
		if wheel != 0 {
			a.problemTop = max(0, a.problemTop+wheel)
		}
		if fresh {
			a.focus = "problems"
			i := y - a.panelRect.Y + a.problemTop
			p := a.problems()
			if i >= 0 && i < len(p) {
				a.problemSelected = i
				p[i].Action()
			}
		}
	}
}
func (a *App) reveal() {
	if a.focus != "editor" || a.view() == nil {
		return
	}
	v := a.view()
	b := v.Doc.Buffer
	r, c := b.Position(v.Head())
	area := a.rects[a.active]
	h := max(1, area.H)
	w := max(4, area.W-a.gutter(v))
	if r < v.Top {
		v.Top = r
	}
	if r >= v.Top+h {
		v.Top = r - h + 1
	}
	column := text.Column(b.Line(r), c, a.cfg.TabSize)
	if column < v.Left {
		v.Left = column
	}
	if column >= v.Left+w {
		v.Left = column - w + 1
	}
	v.Top = max(0, v.Top)
	v.Left = max(0, v.Left)
}
func (a *App) gutter(v *editor.View) int { return len(fmt.Sprint(v.Doc.Buffer.LineCount())) + 2 }
