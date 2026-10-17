package ui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/gdamore/tcell/v3"
	"github.com/rivo/uniseg"
	"sunbook/internal/lsp"
	"sunbook/internal/text"
)

func (a *App) put(x, y, width int, s string, style tcell.Style) {
	a.putCell(x, y, width, s, a.workbenchStyle(style))
}

func (a *App) putCell(x, y, width int, s string, style tcell.Style) {
	if width <= 0 {
		return
	}
	g := uniseg.NewGraphemes(s)
	pos := 0
	for g.Next() {
		str := g.Str()
		if str == "\t" {
			str = " "
		}
		for _, r := range str {
			if r < 32 || r == 127 {
				str = " "
				break
			}
		}
		w := max(1, g.Width())
		if pos+w > width {
			break
		}
		a.screen.Put(x+pos, y, str, style)
		pos += w
	}
}
func (a *App) fill(r rect, style tcell.Style) {
	a.screen.FillArea(r.X, r.Y, r.W, r.H, ' ', a.workbenchStyle(style))
}
func (a *App) layout() {
	w, h := a.screen.Size()
	rail := 0
	if w >= 40 {
		rail = 5
	}
	a.activityRect = rect{0, 1, rail, max(0, h-3)}
	side := 0
	if a.sidebar && w >= 60 {
		side = min(a.sidebarWidth, w/3)
	}
	a.sideRect = rect{rail, 1, side, max(0, h-3)}
	start := rail
	if side > 0 {
		start += side + 1
	}
	width := max(0, w-start)
	bottom := max(3, h-2)
	a.panelRect = rect{}
	a.panelDivider = rect{}
	if a.panelVisible && h >= 14 {
		if a.panelDock == "right" && width >= 64 {
			pw := max(28, min(a.panelWidth, width-31))
			width -= pw + 1
			a.panelRect = rect{start + width + 1, 2, pw, h - 4}
			a.panelDivider = rect{start + width, 1, 1, h - 3}
		} else {
			ph := max(3, min(a.panelHeight, h-9))
			a.panelRect = rect{start, h - 2 - ph, width, ph}
			bottom = a.panelRect.Y - 1
			a.panelDivider = rect{start, bottom, width, 1}
		}
	}
	a.editorArea = rect{start, 1, width, max(0, bottom-1)}
	height := max(0, bottom-3)
	a.rects[0] = rect{start, 3, width, height}
	a.rects[1] = rect{}
	if a.split && width >= 30 {
		left := max(12, min(width-13, int(float64(width)*a.splitRatio)))
		a.rects[0].W = left
		a.rects[1] = rect{start + left + 1, 3, width - left - 1, height}
	}
	if a.rects[1].W == 0 {
		a.active = 0
	}
	if sidebarFocus(a.focus) && side == 0 {
		a.focus = "editor"
	}
	if a.panelRect.H == 0 && (a.focus == "terminal" || a.focus == "problems") {
		a.focus = "editor"
	}
}
func (a *App) draw() {
	a.layout()
	w, h := a.screen.Size()
	a.screen.Fill(' ', a.base)
	a.screen.HideCursor()
	a.buttons = nil
	a.scrollbars = nil
	a.treeFilterRect = rect{}
	a.searchInputRect = rect{}
	a.searchIncludeRect = rect{}
	a.searchExcludeRect = rect{}
	a.searchGroupHits = nil
	a.gitMessageRect = rect{}
	a.gitFilterRect = rect{}
	a.sideHits = nil
	a.modalRect = rect{}
	if m := a.modal; m != nil {
		m.InputRect = rect{}
		m.ListY = 0
		m.ListH = 0
		if s := m.Open; s != nil {
			s.UpRect = rect{}
			s.GoRect = rect{}
			s.PathRect = rect{}
		}
	}
	if w < 24 || h < 7 {
		a.terminalMenu = nil
		a.put(0, 0, w, "SunBook — enlarge terminal", a.accent)
		a.screen.Show()
		return
	}
	a.fill(rect{0, 0, w, 1}, a.chrome)
	a.drawToolbar()
	a.drawActivityBar()
	if a.sideRect.W > 0 {
		a.drawSidebar()
		for y := 1; y < h-2; y++ {
			a.put(a.sideRect.X+a.sideRect.W, y, 1, "│", a.base.Foreground(a.colors.border))
		}
	}
	start := a.rects[0].X
	a.fill(rect{start, 1, a.editorArea.W, 1}, a.chrome)
	a.drawTabs(start, a.editorArea.W)
	if a.comparing() {
		a.drawComparison()
	} else {
		path := a.view().Doc.Path
		if path == "" {
			path = "Untitled — Ctrl+S to save"
		} else if rel, e := filepath.Rel(a.root, path); e == nil {
			path = rel
		}
		a.put(start+1, 2, a.editorArea.W-2, path, a.muted)
		a.drawEditor(0)
		if a.split && a.rects[1].W > 0 {
			for y := 3; y < a.rects[1].Y+a.rects[1].H; y++ {
				a.put(a.rects[1].X-1, y, 1, "│", a.base.Foreground(a.colors.border))
			}
			a.drawEditor(1)
		}
	}
	if a.panelRect.H > 0 {
		if a.panelDivider.W == 1 {
			st := a.base.Foreground(a.colors.border)
			if a.drag == "panelSide" {
				st = st.Foreground(a.accent.GetForeground())
			}
			for y := a.panelDivider.Y; y < a.panelDivider.Y+a.panelDivider.H; y++ {
				a.put(a.panelDivider.X, y, 1, "│", st)
			}
		}
		a.drawPanel()
	}
	a.drawTabDrop()
	hint := "F1 Commands   Ctrl+P Files   Ctrl+S Save   Ctrl+Q Quit"
	switch a.focus {
	case "plugins":
		hint = "Enter Actions · i Install plugin · r Refresh · Esc Editor"
	case "git":
		hint = "Enter Diff · s Stage · u Unstage · b Branches · c Message · / Filter · m More · h History"
	case "gitMessage":
		hint = "Enter Commit staged · Tab Filter · Ctrl+U Clear · Esc File list"
	case "gitFilter":
		hint = "Type to filter changes · Enter File list · Esc Clear filter"
	}
	if a.comparing() && (a.focus == "diff" || a.focus == "editor") {
		hint = "Read-only diff · ↑/↓ Scroll · Tab Other side · n/p Change · o Open file · Esc Close"
	}
	if a.focus == "gitLog" {
		hint = "Git log · ↑/↓ Select · Enter Commit · Tab Changes · Esc Editor"
	}
	if a.hoverHint != "" {
		hint = a.hoverHint
	}
	style := a.muted
	if a.message != "" && time.Now().Before(a.messageUntil) {
		hint = a.message
		style = a.accent
	}
	a.fill(rect{0, h - 2, w, 1}, a.base)
	a.put(1, h-2, w-2, hint, style)
	a.drawStatus(w, h-1)
	if a.completion != nil {
		a.drawCompletion()
	}
	if a.terminalMenu != nil {
		a.drawTerminalMenu()
	}
	if a.modal != nil {
		a.drawModal()
	}
	a.screen.Show()
}
func (a *App) drawTree() {
	r := a.sideRect
	a.fill(r, a.chrome)
	a.drawSidebarHeader("EXPLORER", 12)
	if r.W >= 20 {
		a.drawButton(r.X+r.W-12, r.Y, a.actionLabel("file.new", "New", true), "file.new", "New file in selected folder (Ctrl+N)", false)
		a.drawButton(r.X+r.W-8, r.Y, a.actionLabel("explorer.delete", "Delete", true), "explorer.delete", "Delete selected file (Delete in Explorer)", false)
		a.drawButton(r.X+r.W-4, r.Y, a.actionLabel("explorer.refresh", "Refresh", true), "explorer.refresh", "Refresh Explorer", false)
	}
	rootStyle := a.sidebarBand(r.Y + 1)
	if a.treePath == a.root {
		rootStyle = a.drawFileSelection(rect{r.X, r.Y + 1, r.W, 1}, a.focus == "tree")
	}
	countX := a.sidebarCount(r.X+r.W-1, r.Y+1, fmt.Sprint(len(a.tree)), rootStyle)
	a.put(r.X+1, r.Y+1, countX-r.X-2, fitLabel(filepath.Base(a.root), countX-r.X-2), rootStyle.Bold(true))
	a.treeFilterRect = rect{r.X + 1, r.Y + 2, max(1, r.W-6), 1}
	field := a.treeFilterRect
	a.treeQueryOffset = a.drawSidebarInput(field, a.treeQuery, a.treeQueryCursor, "Filter files (/)", "filter")
	if a.treeQuery != "" {
		a.drawButton(r.X+r.W-4, field.Y, a.actionLabel("explorer.clearFilter", "Clear", true), "explorer.clearFilter", "Clear Explorer filter (Esc)", false)
	}
	available := max(1, r.H-3)
	if a.focus == "tree" {
		if a.treeSelected < a.treeTop {
			a.treeTop = a.treeSelected
		}
		if a.treeSelected >= a.treeTop+available {
			a.treeTop = a.treeSelected - available + 1
		}
	}
	a.treeTop = max(0, min(a.treeTop, max(0, len(a.tree)-1)))
	if len(a.tree) == 0 && a.treeQuery != "" {
		label := "No matching files"
		if a.treeIndexLoading {
			label = "Searching files…"
		}
		a.put(r.X+1, r.Y+3, r.W-2, label, a.muted)
	}
	for i := 0; i < available && i+a.treeTop < len(a.tree); i++ {
		idx := i + a.treeTop
		e := a.tree[idx]
		prefix := "  "
		if e.Dir {
			prefix = "▸ "
			if a.expanded[e.Path] {
				prefix = "▾ "
			}
		}
		style := a.chrome
		if e.Dir {
			style = style.Foreground(a.colors.folder).Bold(true)
		}
		if idx == a.treeSelected {
			style = a.drawFileSelection(rect{r.X, r.Y + 3 + i, r.W, 1}, a.focus == "tree" || a.focus == "filter")
		}
		for depth := 0; depth < e.Depth; depth++ {
			gx := r.X + 2 + depth*2
			if gx < r.X+r.W-1 {
				a.put(gx, r.Y+3+i, 1, "│", style.Foreground(a.colors.border))
			}
		}
		x := r.X + 1 + e.Depth*2
		end := r.X + r.W - 1
		a.put(x, r.Y+3+i, max(0, min(2, end-x)), prefix, style.Foreground(a.colors.muted))
		x += 2
		x += a.drawFileIcon(x, r.Y+3+i, end-x, e.Path, e.Dir, a.expanded[e.Path], style)
		a.put(x, r.Y+3+i, end-x, fitLabel(e.Name, end-x), style)
	}
	a.sidebarScroll(rect{r.X, r.Y + 3, r.W, max(0, r.H-3)}, a.treeTop, len(a.tree))
}
func (a *App) drawEditor(group int) {
	v := a.views[group]
	r := a.rects[group]
	if v == nil || r.W <= 0 || r.H <= 0 {
		return
	}
	b := v.Doc.Buffer
	textRight := r.X + r.W - a.editorScrollbarWidth(group)
	gutter := a.gutter(v)
	current, _ := b.Position(v.Head())
	hl := a.highlight[v.Doc]
	if hl.Version != b.Version {
		hl.Spans = nil
	}
	diags := a.cachedDiagnostics(v.Doc)
	matchA, matchB := matchingBrackets(b.String(), v.Head())
	for dy := 0; dy < r.H; dy++ {
		row := v.Top + dy
		y := r.Y + dy
		if row >= b.LineCount() {
			a.put(r.X, y, gutter, "  ·", a.base.Foreground(a.colors.border))
			continue
		}
		lineStyle := a.base
		if group == a.active && row == current {
			lineStyle = lineStyle.Background(a.colors.line)
			a.fill(rect{r.X, y, r.W, 1}, lineStyle)
		}
		numberStyle := a.muted
		if row == current && group == a.active {
			numberStyle = a.accent
		}
		a.put(r.X, y, gutter-1, fmt.Sprintf("%*d", gutter-2, row+1), numberStyle)
		rowDiagnostics := diags.row(row)
		if rowDiagnostics.marker {
			a.put(r.X+gutter-1, y, 1, "!", a.base.Foreground(a.colors.danger))
		}
		line := b.Line(row)
		offset := b.LineStart(row)
		col := 0
		g := uniseg.NewGraphemes(line)
		spanIdx := sort.Search(len(hl.Spans), func(i int) bool { return hl.Spans[i].End > offset })
		for g.Next() {
			lo, hi := g.Positions()
			p := offset + lo
			str := g.Str()
			gw := max(1, g.Width())
			tab := str == "\t"
			if tab {
				gw = a.cfg.TabSize - col%a.cfg.TabSize
			}
			screenX := r.X + gutter + col - v.Left
			style := lineStyle
			for spanIdx < len(hl.Spans) && hl.Spans[spanIdx].End <= p {
				spanIdx++
			}
			if spanIdx < len(hl.Spans) && hl.Spans[spanIdx].Start <= p {
				style = hl.Spans[spanIdx].Style
				if row == current && group == a.active {
					style = style.Background(a.colors.line)
				}
			}
			for _, diag := range rowDiagnostics.spans {
				if p >= diag.start && p < diag.end {
					style = style.Underline(true)
					break
				}
			}
			if p == matchA || p == matchB {
				style = style.Background(a.colors.selection).Bold(true)
			}
			for _, sel := range v.Selections {
				loSel, z := sel.Range()
				if p < z && offset+hi > loSel {
					style = style.Background(a.colors.selection)
				}
				if sel.Head == p && (group != a.active || a.focus != "editor" || sel.Head != v.Head()) {
					style = style.Reverse(true)
				}
			}
			if tab {
				for dx := 0; dx < gw; dx++ {
					xx := screenX + dx
					if xx >= r.X+gutter && xx < textRight {
						a.put(xx, y, 1, " ", style)
					}
				}
			} else if screenX >= r.X+gutter && screenX+gw <= textRight {
				a.put(screenX, y, gw, str, style)
			}
			col += gw
			if screenX >= textRight {
				break
			}
		}
		end := b.LineEnd(row)
		for _, sel := range v.Selections {
			loSel, z := sel.Range()
			if loSel <= end && z > end {
				xx := r.X + gutter + col - v.Left
				if xx >= r.X+gutter && xx < textRight {
					a.put(xx, y, 1, " ", a.selected)
				}
			}
			if sel.Head == end && (len(v.Selections) > 1 || group != a.active) {
				xx := r.X + gutter + col - v.Left
				if xx >= r.X+gutter && xx < textRight {
					a.put(xx, y, 1, " ", a.base.Reverse(true))
				}
			}
		}
	}
	if a.editorScrollbarWidth(group) > 0 {
		a.drawContentScrollbar(r, &v.Top, b.LineCount(), "editor", group)
	}
	if group == a.active && a.focus == "editor" && a.modal == nil {
		row, c := b.Position(v.Head())
		x := r.X + gutter + text.Column(b.Line(row), c, a.cfg.TabSize) - v.Left
		y := r.Y + row - v.Top
		if x >= r.X+gutter && x < textRight && y >= r.Y && y < r.Y+r.H {
			a.screen.ShowCursor(x, y)
		}
	}
}
func matchingBrackets(s string, head int) (int, int) {
	if len(s) == 0 {
		return -1, -1
	}
	p := head
	if p >= len(s) || !strings.ContainsRune("()[]{}", rune(s[p])) {
		p = head - 1
	}
	if p < 0 || p >= len(s) {
		return -1, -1
	}
	pairs := map[byte]byte{'(': ')', '[': ']', '{': '}', ')': '(', ']': '[', '}': '{'}
	target, ok := pairs[s[p]]
	if !ok {
		return -1, -1
	}
	dir := 1
	if strings.ContainsRune(")]}", rune(s[p])) {
		dir = -1
	}
	depth := 0
	for q, steps := p, 0; q >= 0 && q < len(s) && steps < 10000; q, steps = q+dir, steps+1 {
		if s[q] == s[p] {
			depth++
		}
		if s[q] == target {
			depth--
			if depth == 0 {
				return p, q
			}
		}
	}
	return -1, -1
}
func (a *App) drawPanel() {
	r := a.panelRect
	a.fill(rect{r.X, r.Y - 1, r.W, 1}, a.chrome)
	right := r.X + r.W - 1
	action := func(label, id, hint string, compact bool) bool {
		label = a.actionLabel(id, label, compact)
		width := text.Width(label, 4) + 2
		if right-width < r.X+2 {
			return false
		}
		right -= width
		a.drawButton(right, r.Y-1, label, id, hint, false)
		right--
		return true
	}
	action("Hide", "view.hidePanel", "Hide panel; keep shell sessions running", true)
	if a.panel == "Terminal" && a.term != nil {
		action("Kill", "terminal.kill", "Close the current shell session", true)
	}
	label, hint := "Right", "Right: move panel to the right; keep the shell session"
	if a.panelDock == "right" {
		label, hint = "Bottom", "Bottom: move panel below the editor; keep the shell session"
	}
	action(label, "view.panelDock", hint, r.W < 45)
	x := r.X + 1
	choices := []struct{ name, id string }{{"Problems", "view.problems"}, {"Terminal", "view.showTerminal"}}
	if r.W < 60 {
		if a.panel == "Terminal" {
			choices = choices[1:]
		} else {
			choices = choices[:1]
		}
	}
	for _, v := range choices {
		label := a.actionLabel(v.id, v.name, false)
		width := text.Width(label, 4) + 2
		limit := right
		if a.panel == "Terminal" {
			limit -= 4 // Keep the new-session button accessible in narrow docks.
			if len(a.terminals) > 1 {
				limit -= 4
			}
		}
		if x+width > limit && v.name == "Terminal" {
			label = a.actionLabel(v.id, v.name, true)
			width = text.Width(label, 4) + 2
		}
		if x+width > limit {
			continue
		}
		x += a.drawButton(x, r.Y-1, label, v.id, "Show "+v.name, a.panel == v.name) + 1
	}
	if a.panel == "Terminal" {
		if x+3 <= right {
			x += a.drawButton(x, r.Y-1, "+", "terminal.new", "New terminal session", false) + 1
		}
		if tab := a.terminalActive; tab != nil && right-x >= 3 {
			label := fmt.Sprintf("%d · %s ▾", tab.id, filepath.Base(a.cfg.Shell))
			if tab.exited {
				label = fmt.Sprintf("%d · exited ▾", tab.id)
			}
			if text.Width(label, 4)+2 > right-x {
				label = fmt.Sprintf("%d▾", tab.id)
				if text.Width(label, 4)+2 > right-x {
					label = fitLabel(fmt.Sprint(tab.id), right-x-2)
				}
			}
			a.drawButton(x, r.Y-1, label, "terminal.switch", "Switch terminal session", a.terminalMenu != nil)
		}
	}
	if x == r.X+1 {
		a.put(x, r.Y-1, max(0, right-x), fitLabel(a.panel, right-x), a.accent.Bold(true))
	}
	switch a.panel {
	case "Terminal":
		a.drawTerminal()
	case "Problems":
		a.drawProblems()
	}
}
func (a *App) drawTerminal() {
	r := a.panelRect
	if a.term == nil {
		a.put(r.X+1, r.Y+1, r.W-2, "Start a shell with Terminal +", a.muted)
		return
	}
	_ = a.term.Resize(max(1, r.W), max(1, r.H))
	vt := a.term.VT
	a.termOffset = max(0, min(a.termOffset, vt.ScrollbackLen()))
	for y := 0; y < r.H; y++ {
		for x := 0; x < r.W; x++ {
			var cellContent string
			style := a.base
			row := y - a.termOffset
			cell := vt.CellAt(x, row)
			if row < 0 {
				cell = vt.ScrollbackCellAt(x, vt.ScrollbackLen()+row)
			}
			if cell == nil {
				continue
			}
			if cell.Width == 0 {
				continue
			}
			cellContent = cell.Content
			if cellContent == "" {
				cellContent = " "
			}
			if cell.Style.Fg != nil {
				style = style.Foreground(tcell.FromImageColor(cell.Style.Fg))
			}
			if cell.Style.Bg != nil {
				style = style.Background(tcell.FromImageColor(cell.Style.Bg))
			}
			style = style.Bold(cell.Style.Attrs&uv.AttrBold != 0).Italic(cell.Style.Attrs&uv.AttrItalic != 0).Reverse(cell.Style.Attrs&uv.AttrReverse != 0)
			if cell.Style.Underline != 0 {
				style = style.Underline(true)
			}
			a.putCell(r.X+x, r.Y+y, cell.Width, cellContent, style)
			x += max(0, cell.Width-1)
		}
	}
	if a.focus == "terminal" && a.modal == nil && a.terminalMenu == nil && !a.termExited && a.termOffset == 0 && !vt.CursorHidden() {
		p := vt.CursorPosition()
		a.screen.ShowCursor(r.X+p.X, r.Y+p.Y)
	}
}

func (a *App) problems() []item {
	var items []item
	uris := make([]string, 0, len(a.diagnostics))
	for uri := range a.diagnostics {
		uris = append(uris, uri)
	}
	sort.Strings(uris)
	for _, uri := range uris {
		for _, d := range a.diagnostics[uri].Diagnostics {
			path := lsp.Path(uri)
			diag := d
			rel, _ := filepath.Rel(a.root, path)
			severity := "error"
			if d.Severity == 2 {
				severity = "warning"
			} else if d.Severity >= 3 {
				severity = "info"
			}
			items = append(items, item{Label: fmt.Sprintf("%s  %s:%d:%d  %s", severity, rel, d.Range.Start.Line+1, d.Range.Start.Character+1, d.Message), Action: func() {
				if a.open(path) == nil {
					a.view().Set(lsp.Offset(a.view().Doc.Buffer, diag.Range.Start), false)
					a.reveal()
				}
			}})
		}
	}
	return items
}
func (a *App) drawProblems() {
	r := a.panelRect
	items := a.problems()
	if len(items) == 0 {
		a.put(r.X+1, r.Y+1, r.W-2, "No problems reported.", a.muted)
		return
	}
	if a.focus == "problems" {
		if a.problemSelected < a.problemTop {
			a.problemTop = a.problemSelected
		}
		if a.problemSelected >= a.problemTop+r.H {
			a.problemTop = a.problemSelected - r.H + 1
		}
	}
	for i := 0; i < r.H && i+a.problemTop < len(items); i++ {
		idx := i + a.problemTop
		style := a.base
		if idx == a.problemSelected {
			style = a.selected
			a.fill(rect{r.X, r.Y + i, r.W, 1}, style)
		}
		a.put(r.X+1, r.Y+i, r.W-2, items[idx].Label, style)
	}
}
func (a *App) drawCompletion() {
	c := a.completion
	if c == nil || len(c.Items) == 0 {
		return
	}
	v := a.view()
	r := a.rects[a.active]
	row, col := v.Doc.Buffer.Position(v.Head())
	width := max(1, min(60, r.W-2))
	height := min(8, len(c.Items))
	x := min(r.X+a.gutter(v)+text.Column(v.Doc.Buffer.Line(row), col, a.cfg.TabSize)-v.Left, r.X+r.W-width)
	x = max(r.X, x)
	y := r.Y + row - v.Top + 1
	if y+height > r.Y+r.H {
		y = max(1, y-height-1)
	}
	top := max(0, c.Selected-height+1)
	for i := 0; i < height; i++ {
		it := c.Items[top+i]
		style := a.chrome
		if top+i == c.Selected {
			style = a.selected
		}
		a.fill(rect{x, y + i, width, 1}, style)
		a.put(x+1, y+i, width-2, it.Label+"  "+it.Detail, style)
	}
}
func (a *App) drawModal() {
	m := a.modal
	w, h := a.screen.Size()
	width := max(1, min(96, w-4))
	height := min(h-2, 20)
	if m.AutoHeight && !m.Compact {
		height = 5 + min(10, len(m.Items))
		if m.InputEnabled {
			height += 2
		}
		if len(m.Body) > 0 && len(m.Items) > 0 {
			height += min(5, len(m.Body)) + 1
			if len(m.Body) > 5 {
				height++
			}
		}
		height = min(h-2, max(6, height))
	}
	if m.Compact {
		width = max(1, min(64, w-4))
		height = min(h-2, 3+min(6, len(m.Visible)))
		if m.InputEnabled {
			height = min(h-2, 4)
		}
	}
	if height < 3 {
		height = h
	}
	x := (w - width) / 2
	y := max(1, (h-height)/3)
	a.modalRect = rect{x, y, width, height}
	a.fill(rect{x + 1, y + 1, width, height}, a.base.Background(a.colors.shadow))
	a.fill(a.modalRect, a.chrome)
	border := a.chrome.Foreground(a.colors.border)
	a.put(x, y, width, "╭"+strings.Repeat("─", max(0, width-2))+"╮", border)
	a.put(x, y+height-1, width, "╰"+strings.Repeat("─", max(0, width-2))+"╯", border)
	for row := y + 1; row < y+height-1; row++ {
		a.put(x, row, 1, "│", border)
		a.put(x+width-1, row, 1, "│", border)
	}
	a.put(x+2, y, width-4, " "+fitLabel(m.Title, width-6)+" ", a.accent.Background(a.chrome.GetBackground()).Bold(true))
	subtitle := m.Subtitle
	if m.Open != nil && m.Open.Quick && m.Selected >= 0 && m.Selected < len(m.Visible) {
		subtitle = m.Visible[m.Selected].Path
	}
	if subtitle != "" && (m.Open == nil || m.Open.Quick) {
		a.put(x+2, y+1, width-4, tailLabel(subtitle, width-4), a.chrome.Foreground(a.colors.muted))
	}
	contentY := y + 2
	if m.Compact {
		contentY = y + 1
	}
	if m.InputEnabled {
		fieldStyle := a.inputStyle(m.Open == nil || !m.Open.EditingPath)
		a.fill(rect{x + 2, contentY, width - 4, 1}, fieldStyle)
		inputWidth := max(1, width-4)
		offset := 0
		if text.Width(m.Input[:m.Cursor], a.cfg.TabSize) >= inputWidth {
			offset = text.Offset(m.Input, text.Width(m.Input[:m.Cursor], a.cfg.TabSize)-inputWidth+1, a.cfg.TabSize)
		}
		if m.Input == "" && m.Placeholder != "" {
			a.put(x+2, contentY, inputWidth, m.Placeholder, fieldStyle.Foreground(a.colors.muted))
		} else {
			a.put(x+2, contentY, inputWidth, m.Input[offset:], fieldStyle)
		}
		m.InputRect = rect{x + 2, contentY, inputWidth, 1}
		m.InputOffset = offset
		if m.Open == nil || !m.Open.EditingPath {
			a.screen.ShowCursor(x+2+text.Width(m.Input[offset:m.Cursor], a.cfg.TabSize), contentY)
		}
		contentY += 2
		if m.Compact {
			contentY--
		}
	} else {
		a.screen.HideCursor()
	}
	if m.Open != nil && !m.Open.Quick {
		a.drawOpenNavigation(m, rect{x + 2, y + 1, width - 4, 1})
	}
	if len(m.Body) > 0 && len(m.Items) > 0 {
		bodySpace := max(0, y+height-5-contentY)
		m.BodyH = min(5, len(m.Body), bodySpace)
		if len(m.Body) > m.BodyH && bodySpace == m.BodyH && m.BodyH > 1 {
			m.BodyH-- // Keep a scroll hint visible above the action list.
		}
		m.BodyTop = max(0, min(m.BodyTop, len(m.Body)-max(1, m.BodyH)))
		for i := 0; i < m.BodyH; i++ {
			line := m.Body[i+m.BodyTop]
			offset := text.Offset(line, m.BodyLeft, a.cfg.TabSize)
			a.put(x+2, contentY, width-4, line[offset:], a.muted)
			contentY++
		}
		if len(m.Body) > m.BodyH && bodySpace > m.BodyH {
			a.put(x+2, contentY, width-4, "Ctrl+↑/↓ scroll · ←/→ pan", a.muted)
			contentY++
		}
		contentY++
	}
	available := max(0, y+height-3-contentY)
	if m.Compact {
		available = max(0, y+height-2-contentY)
	}
	m.ListY = contentY
	m.ListH = available
	if len(m.Visible) > 0 {
		if m.Selected < m.Top {
			m.Top = m.Selected
		}
		if m.Selected >= m.Top+available && available > 0 {
			m.Top = m.Selected - available + 1
		}
		m.Top = max(0, min(m.Top, len(m.Visible)-1))
		for i := 0; i < available && i+m.Top < len(m.Visible); i++ {
			idx := m.Top + i
			it := m.Visible[idx]
			style := a.chrome
			if idx == m.Selected {
				if m.Open != nil {
					style = a.drawFileSelection(rect{x + 1, contentY + i, width - 2, 1}, !m.Open.EditingPath)
				} else {
					style = a.selected
					a.fill(rect{x + 1, contentY + i, width - 2, 1}, style)
				}
			}
			labelWidth := width - 4
			if it.Detail != "" {
				detailWidth := min(width/3, text.Width(it.Detail, 4))
				labelWidth -= detailWidth + 2
				a.put(x+width-detailWidth-2, contentY+i, detailWidth, fitLabel(it.Detail, detailWidth), style.Foreground(a.colors.muted))
			}
			labelX := x + 2
			if it.Swatch[0] != "" && labelWidth >= 14 {
				for _, color := range it.Swatch {
					a.put(labelX, contentY+i, 2, "██", style.Foreground(tcell.GetColor(color)))
					labelX += 3
					labelWidth -= 3
				}
			}
			if m.Open != nil {
				n := a.drawFileIcon(labelX, contentY+i, labelWidth, it.Path, it.Dir, false, style)
				labelX += n
				labelWidth -= n
			}
			a.put(labelX, contentY+i, labelWidth, fitLabel(it.Label, labelWidth), style)
		}
	} else if len(m.Body) > 0 {
		bodyWidth := width - 4
		if len(m.Body) > available {
			bodyWidth--
		}
		for i := 0; i < available && i+m.Top < len(m.Body); i++ {
			line := m.Body[i+m.Top]
			style := a.chrome
			if strings.HasPrefix(m.Title, "Diff:") {
				switch {
				case strings.HasPrefix(line, "@@"):
					style = style.Foreground(a.accent.GetForeground())
				case strings.HasPrefix(line, "+"):
					style = style.Foreground(a.colors.success)
				case strings.HasPrefix(line, "-"):
					style = style.Foreground(a.colors.danger)
				}
			}
			line = expandTabs(line, a.cfg.TabSize)
			offset := text.Offset(line, m.BodyLeft, a.cfg.TabSize)
			a.put(x+2, contentY+i, bodyWidth, line[offset:], style)
		}
		a.drawContentScrollbar(rect{x + 2, contentY, width - 4, available}, &m.Top, len(m.Body), "modal", 0)
	} else if m.Open != nil {
		a.put(x+2, contentY, width-4, fitLabel(m.Empty, width-4), a.muted)
	} else if m.Filter {
		a.put(x+2, contentY, width-4, "No matches", a.muted)
	}
	a.put(x+2, y+height-2, width-4, fitLabel(m.Hint, width-4), a.chrome.Foreground(a.colors.muted))
}

func expandTabs(line string, tabSize int) string {
	if !strings.ContainsRune(line, '\t') {
		return line
	}
	var out strings.Builder
	col := 0
	g := uniseg.NewGraphemes(line)
	for g.Next() {
		if g.Str() == "\t" {
			width := tabSize - col%tabSize
			out.WriteString(strings.Repeat(" ", width))
			col += width
		} else {
			out.WriteString(g.Str())
			col += max(1, g.Width())
		}
	}
	return out.String()
}
