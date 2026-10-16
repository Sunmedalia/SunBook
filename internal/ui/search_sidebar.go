package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/gdamore/tcell/v3"
	"github.com/rivo/uniseg"
	"sunbook/internal/text"
)

type searchGroupHit struct {
	rect
	Path string
}

func (a *App) searchBodyStart() int {
	y := a.sideRect.Y + 5
	if a.searchPathsVisible && a.sideRect.H >= 12 {
		y += 2
	}
	return y
}
func (a *App) searchRows() []sidebarRow {
	counts := map[string]int{}
	for _, h := range a.searchResults {
		counts[h.Path]++
	}
	var rows []sidebarRow
	last := ""
	for i, h := range a.searchResults {
		if h.Path != last {
			rel, _ := filepath.Rel(a.root, h.Path)
			rows = append(rows, sidebarRow{Label: filepath.ToSlash(rel), Group: h.Path, Heading: true, Index: counts[h.Path]})
			last = h.Path
		}
		if !a.searchCollapsed[h.Path] {
			rows = append(rows, sidebarRow{Index: i})
		}
	}
	return rows
}
func (a *App) drawSearchSidebar() {
	r := a.sideRect
	a.drawSidebarHeader("SEARCH", 4)
	if a.searchError != "" {
		a.drawButton(r.X+r.W-4, r.Y, a.actionLabel("search.error", "Error", true), "search.error", "Show search error", false)
	}
	a.searchInputRect = rect{r.X + 1, r.Y + 1, r.W - 2, 1}
	a.searchOffset = a.drawSidebarInput(a.searchInputRect, a.searchQuery, a.searchCursor, "Search workspace…", "searchInput")
	x := r.X + 1
	for _, b := range []struct {
		label, id, hint string
		active          bool
	}{{"Aa", "search.case", "Match case; off uses smart case", a.searchOptions.Case}, {"W", "search.word", "Match whole words", a.searchOptions.Word}, {".*", "search.regex", "Use regular expressions", a.searchOptions.Regex}, {"...", "search.paths", "Include / exclude file globs", a.searchPathsVisible}} {
		if x+len(b.label)+2 > r.X+r.W {
			continue
		}
		x += a.drawButton(x, r.Y+2, b.label, b.id, b.hint, b.active) + 1
	}
	x = r.X + 1
	for _, b := range []struct{ label, id string }{{"Go", "search.run"}, {"Clear", "search.clear"}, {"Stop", "search.stop"}} {
		label := a.actionLabel(b.id, b.label, b.id != "search.run")
		if x+text.Width(label, 4)+2 > r.X+r.W {
			continue
		}
		x += a.drawButton(x, r.Y+3, label, b.id, b.label+" workspace search", false) + 1
	}
	statusY := r.Y + 4
	if a.searchPathsVisible && r.H >= 12 {
		a.searchIncludeRect = rect{r.X + 2, statusY, r.W - 3, 1}
		a.searchExcludeRect = rect{r.X + 2, statusY + 1, r.W - 3, 1}
		a.put(r.X, statusY, 2, "+ ", a.chrome.Foreground(a.colors.folder))
		a.put(r.X, statusY+1, 2, "− ", a.chrome.Foreground(a.colors.muted))
		a.searchIncludeOffset = a.drawSidebarInput(a.searchIncludeRect, a.searchOptions.Include, a.searchIncludeCursor, "Include: src/**, *.go", "searchInclude")
		a.searchExcludeOffset = a.drawSidebarInput(a.searchExcludeRect, a.searchOptions.Exclude, a.searchExcludeCursor, "Exclude: vendor/**", "searchExclude")
		statusY += 2
	}
	files := map[string]bool{}
	for _, h := range a.searchResults {
		files[h.Path] = true
	}
	label := fmt.Sprintf("%d results · %d files", len(a.searchResults), len(files))
	if a.searchTruncated {
		label = fmt.Sprintf("%d results · limit reached", len(a.searchResults))
	}
	if a.searchQuery == "" {
		label = "Type to search"
	} else if a.searchError != "" {
		label = "Search failed · click !"
	} else if a.searchRunning || !a.searchDue.IsZero() {
		if len(a.searchResults) == 0 {
			label = "Searching…"
		} else {
			label += " …"
		}
	} else if len(a.searchResults) == 0 {
		label = "No matches"
	}
	summaryStyle := a.sidebarBand(statusY).Foreground(a.colors.muted)
	a.put(r.X+1, statusY, r.W-2, fitLabel(label, r.W-2), summaryStyle)
	if len(a.searchResults) == 0 && a.searchQuery == "" && r.Y+r.H-a.searchBodyStart() >= 3 {
		a.put(r.X+2, a.searchBodyStart()+1, r.W-4, "Search your workspace", a.chrome.Foreground(a.colors.muted))
		a.put(r.X+2, a.searchBodyStart()+2, r.W-4, "Use ... for file filters", a.chrome.Foreground(a.colors.disabled))
	}
	rows := a.searchRows()
	if len(rows) > 0 {
		a.searchTop = max(0, min(a.searchTop, len(rows)-1))
	}
	for y, i := a.searchBodyStart(), a.searchTop; y < r.Y+r.H && i < len(rows); y, i = y+1, i+1 {
		row := rows[i]
		if row.Heading {
			prefix := "▾ "
			if a.searchCollapsed[row.Group] {
				prefix = "▸ "
			}
			name := filepath.Base(row.Group)
			style := a.sidebarBand(y).Bold(true)
			countX := a.sidebarCount(r.X+r.W-1, y, fmt.Sprint(row.Index), style)
			a.put(r.X+1, y, 2, prefix, style)
			n := a.drawFileIcon(r.X+3, y, r.W-4, row.Group, false, false, style)
			a.put(r.X+3+n, y, countX-r.X-4-n, fitLabel(name, countX-r.X-4-n), style)
			a.searchGroupHits = append(a.searchGroupHits, searchGroupHit{rect{r.X, y, r.W, 1}, row.Group})
			continue
		}
		hit := a.searchResults[row.Index]
		style := a.chrome
		if row.Index == a.searchSelected {
			style = a.drawFileSelection(rect{r.X, y, r.W, 1}, a.focus == "search")
		}
		line := strings.TrimRight(hit.Text, "\r\n")
		prefix := fmt.Sprintf("%d:%d ", hit.Line+1, text.Column(line, min(hit.Column, len(line)), a.cfg.TabSize)+1)
		pw := min(text.Width(prefix, 4), max(1, (r.W-2)/2))
		a.put(r.X+1, y, pw, prefix, style.Foreground(a.colors.muted))
		a.drawSearchSnippet(r.X+1+pw, y, r.W-2-pw, hit, style)
		a.sideHits = append(a.sideHits, sideHit{rect{r.X, y, r.W, 1}, row.Index})
	}
	a.sidebarScroll(rect{r.X, a.searchBodyStart(), r.W, max(0, r.Y+r.H-a.searchBodyStart())}, a.searchTop, len(rows))
}
func (a *App) drawSearchSnippet(x, y, width int, h searchHit, style tcell.Style) {
	if width <= 0 {
		return
	}
	line := strings.TrimRight(h.Text, "\r\n")
	start := min(len(line), h.Column)
	col := text.Column(line, start, a.cfg.TabSize)
	offset := text.Offset(line, max(0, col-width/3), a.cfg.TabSize)
	pos := 0
	if offset > 0 {
		a.put(x, y, 1, "…", style.Foreground(a.colors.muted))
		pos = 1
	}
	column := text.Column(line, offset, a.cfg.TabSize)
	g := uniseg.NewGraphemes(line[offset:])
	for g.Next() {
		lo, hi := g.Positions()
		s := g.Str()
		w := max(1, g.Width())
		if s == "\t" {
			w = a.cfg.TabSize - column%a.cfg.TabSize
			s = strings.Repeat(" ", w)
		}
		if pos+w > width {
			break
		}
		st := style
		if offset+lo < h.End && offset+hi > h.Column {
			st = st.Foreground(a.accent.GetForeground()).Bold(true).Underline(true)
		}
		a.put(x+pos, y, w, s, st)
		pos += w
		column += w
	}
}
func (a *App) moveSearchSelection(delta int) {
	a.searchSelected = max(0, min(a.searchSelected+delta, len(a.searchResults)-1))
	if len(a.searchResults) > 0 {
		delete(a.searchCollapsed, a.searchResults[a.searchSelected].Path)
	}
	revealList(&a.searchTop, a.searchRows(), a.searchSelected, max(1, a.sideRect.Y+a.sideRect.H-a.searchBodyStart()))
}
func (a *App) openSearchResult() {
	if a.searchSelected < 0 || a.searchSelected >= len(a.searchResults) {
		return
	}
	h := a.searchResults[a.searchSelected]
	if err := a.open(h.Path); err != nil {
		a.showError(err)
		return
	}
	b := a.view().Doc.Buffer
	row := max(0, min(h.Line, b.LineCount()-1))
	start := b.LineStart(row) + min(len(b.Line(row)), h.Column)
	end := b.LineStart(row) + min(len(b.Line(row)), max(h.Column, h.End))
	source := b.String()
	selectionEnd := text.Snap(source, end)
	if selectionEnd < end {
		selectionEnd = text.Next(source, selectionEnd)
	}
	a.view().Selections = []text.Selection{{Anchor: selectionEnd, Head: text.Snap(source, start)}}
	a.reveal()
}
func (a *App) toggleSearchGroup(path string) {
	if a.searchCollapsed == nil {
		a.searchCollapsed = map[string]bool{}
	}
	a.searchCollapsed[path] = !a.searchCollapsed[path]
	a.focus = "search"
}
func (a *App) rerunSearchOptions() {
	a.stopWorkspaceSearch()
	a.searchResults = nil
	a.searchSelected = 0
	a.searchTop = 0
	a.searchError = ""
	a.searchTruncated = false
	if a.searchQuery != "" {
		a.searchDue = time.Now().Add(250 * time.Millisecond)
	}
}
func (a *App) toggleSearchPaths() {
	if a.sideRect.H < 12 {
		a.prompt("Search Include Globs", a.searchOptions.Include, func(s string) {
			a.searchOptions.Include = s
			a.searchIncludeCursor = len(s)
			a.prompt("Search Exclude Globs", a.searchOptions.Exclude, func(s string) { a.searchOptions.Exclude = s; a.searchExcludeCursor = len(s); a.rerunSearchOptions() })
		})
		return
	}
	a.searchPathsVisible = !a.searchPathsVisible
	if !a.searchPathsVisible && (a.focus == "searchInclude" || a.focus == "searchExclude") {
		a.focus = "searchInput"
	}
}
func (a *App) searchOptionsMouse(x, y int, fresh bool) bool {
	if !fresh {
		return false
	}
	for _, f := range []struct {
		r           rect
		name, value string
		offset      int
		cursor      *int
	}{{a.searchIncludeRect, "searchInclude", a.searchOptions.Include, a.searchIncludeOffset, &a.searchIncludeCursor}, {a.searchExcludeRect, "searchExclude", a.searchOptions.Exclude, a.searchExcludeOffset, &a.searchExcludeCursor}} {
		if f.r.contains(x, y) {
			a.focus = f.name
			*f.cursor = f.offset + text.Offset(f.value[f.offset:], x-f.r.X, a.cfg.TabSize)
			return true
		}
	}
	return false
}
func (a *App) insertSearchPath(s string) {
	value, cursor := &a.searchOptions.Include, &a.searchIncludeCursor
	if a.focus == "searchExclude" {
		value, cursor = &a.searchOptions.Exclude, &a.searchExcludeCursor
	}
	s = strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(s)
	*value = (*value)[:*cursor] + s + (*value)[*cursor:]
	*cursor += len(s)
	a.rerunSearchOptions()
}
func (a *App) searchPathKey(e *tcell.EventKey, name string) {
	value, cursor := &a.searchOptions.Include, &a.searchIncludeCursor
	if a.focus == "searchExclude" {
		value, cursor = &a.searchOptions.Exclude, &a.searchExcludeCursor
	}
	changed := false
	switch name {
	case "escape":
		a.focus = "searchInput"
	case "enter":
		a.startWorkspaceSearch()
		a.focus = "search"
	case "tab":
		if a.focus == "searchInclude" {
			a.focus = "searchExclude"
		} else {
			a.focus = "searchInput"
		}
	case "left":
		*cursor = text.Prev(*value, *cursor)
	case "right":
		*cursor = text.Next(*value, *cursor)
	case "home", "ctrl+a":
		*cursor = 0
	case "end", "ctrl+e":
		*cursor = len(*value)
	case "ctrl+u":
		*value = ""
		*cursor = 0
		changed = true
	case "ctrl+v":
		a.readClipboard()
	case "backspace", "backspace2":
		p := text.Prev(*value, *cursor)
		*value = (*value)[:p] + (*value)[*cursor:]
		*cursor = p
		changed = true
	case "delete":
		p := text.Next(*value, *cursor)
		*value = (*value)[:*cursor] + (*value)[p:]
		changed = true
	default:
		if e.Str() != "" && e.Modifiers()&(tcell.ModCtrl|tcell.ModAlt|tcell.ModMeta) == 0 {
			a.insertSearchPath(e.Str())
		}
	}
	if changed {
		a.rerunSearchOptions()
	}
}
