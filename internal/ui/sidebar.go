package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v3"
	"sunbook/internal/text"
)

type sideHit struct {
	rect
	Index int
}
type sidebarRow struct {
	Label   string
	Index   int
	Heading bool
}
type gitEntry struct {
	File   int
	Staged bool
}

func sidebarFocus(f string) bool {
	return f == "tree" || f == "filter" || f == "searchInput" || f == "search" || f == "git"
}
func (a *App) focusSidebar() {
	switch a.sidebarView {
	case "Search":
		a.focus = "searchInput"
		a.searchCursor = len(a.searchQuery)
	case "Git":
		a.focus = "git"
	default:
		a.focus = "tree"
	}
}
func (a *App) showSidebar(view string) {
	a.sidebar = true
	a.sidebarView = view
	a.completion = nil
	a.focusSidebar()
	if view == "Git" {
		a.refreshGit()
	}
}
func (a *App) toggleSidebar() {
	a.sidebar = !a.sidebar
	if !a.sidebar && sidebarFocus(a.focus) {
		a.focus = "editor"
	}
}
func (a *App) toggleSidebarView(view string) {
	if a.sidebar && a.sidebarView == view {
		a.toggleSidebar()
	} else {
		a.showSidebar(view)
	}
}
func (a *App) drawActivityBar() {
	r := a.activityRect
	if r.W == 0 {
		return
	}
	a.fill(r, a.base.Background(a.colors.inset))
	for i, v := range []struct{ label, view, id, hint string }{{"E", "Explorer", "activity.explorer", "Explorer (Ctrl+Shift+E)"}, {"S", "Search", "activity.search", "Search workspace (Ctrl+Shift+F)"}, {"G", "Git", "activity.git", "Source Control (Ctrl+Shift+G)"}} {
		y := r.Y + 1 + i*3
		if y >= r.Y+r.H {
			break
		}
		active := a.sidebar && a.sidebarView == v.view
		a.drawButton(r.X+1, y, v.label, v.id, v.hint, active)
		if active {
			a.put(r.X, y, 1, "▎", a.accent.Background(a.colors.inset))
		}
	}
}
func (a *App) drawSidebar() {
	a.fill(a.sideRect, a.chrome)
	switch a.sidebarView {
	case "Search":
		a.drawSearchSidebar()
	case "Git":
		a.drawGitSidebar()
	default:
		a.drawTree()
	}
}
func (a *App) drawSidebarInput(r rect, value string, cursor int, placeholder, focus string) int {
	style := a.inputStyle(a.focus == focus)
	a.fill(r, style)
	offset := 0
	if col := text.Width(value[:cursor], a.cfg.TabSize); col >= r.W {
		offset = text.Offset(value, col-r.W+1, a.cfg.TabSize)
	}
	if value == "" && a.focus != focus {
		a.put(r.X, r.Y, r.W, placeholder, style.Foreground(a.colors.muted))
	} else {
		a.put(r.X, r.Y, r.W, value[offset:], style)
	}
	if a.focus == focus && a.modal == nil {
		a.screen.ShowCursor(r.X+text.Width(value[offset:cursor], a.cfg.TabSize), r.Y)
	}
	return offset
}
func tailLabel(s string, width int) string {
	if width <= 1 || text.Width(s, 4) <= width {
		return s
	}
	return "…" + s[text.Offset(s, text.Width(s, 4)-width+1, 4):]
}
func revealList(top *int, rows []sidebarRow, selected, height int) {
	for i, r := range rows {
		if !r.Heading && r.Index == selected {
			if i < *top {
				*top = i
			}
			if i >= *top+height {
				*top = i - height + 1
			}
			return
		}
	}
}
func (a *App) drawSidebarRows(rows []sidebarRow, start, top, selected int) {
	r := a.sideRect
	for y, i := start, max(0, top); y < r.Y+r.H && i < len(rows); y, i = y+1, i+1 {
		row := rows[i]
		style := a.chrome
		if row.Heading {
			style = a.chrome.Foreground(a.colors.muted).Bold(true)
		} else {
			if row.Index == selected {
				style = a.selected
				a.fill(rect{r.X, y, r.W, 1}, style)
				a.put(r.X, y, 1, "▎", style.Foreground(a.accent.GetForeground()))
			}
			a.sideHits = append(a.sideHits, sideHit{rect{r.X, y, r.W, 1}, row.Index})
		}
		label := row.Label
		if row.Heading || a.sidebarView == "Git" {
			label = tailLabel(label, r.W-2)
		}
		a.put(r.X+1, y, r.W-2, label, style)
	}
}
func (a *App) gitEntries() []gitEntry {
	var entries []gitEntry
	for i, f := range a.git.Files {
		if f.Index != " " && f.Index != "?" && f.Index != "" {
			entries = append(entries, gitEntry{i, true})
		}
	}
	for i, f := range a.git.Files {
		if f.Worktree != " " && f.Worktree != "" || f.Index == "?" {
			entries = append(entries, gitEntry{i, false})
		}
	}
	return entries
}
func (a *App) selectedGitEntry() (gitEntry, bool) {
	entries := a.gitEntries()
	if a.gitSelected < 0 || a.gitSelected >= len(entries) {
		return gitEntry{}, false
	}
	return entries[a.gitSelected], true
}
func (a *App) gitRows() []sidebarRow {
	entries := a.gitEntries()
	var rows []sidebarRow
	last := ""
	for i, e := range entries {
		heading := "CHANGES"
		if e.Staged {
			heading = "STAGED CHANGES"
		}
		if last != heading {
			rows = append(rows, sidebarRow{Label: heading, Heading: true})
			last = heading
		}
		f := a.git.Files[e.File]
		status := f.Worktree
		if e.Staged {
			status = f.Index
		}
		rows = append(rows, sidebarRow{Label: status + " " + f.Path, Index: i})
	}
	return rows
}
func (a *App) selectedGitDiff() {
	if entry, ok := a.selectedGitEntry(); ok {
		a.gitDiff(entry.Staged)
	}
}
func (a *App) openGitFile() {
	if entry, ok := a.selectedGitEntry(); ok {
		path := filepath.Join(a.git.Root, a.git.Files[entry.File].Path)
		if _, err := os.Stat(path); err != nil {
			a.showError(err)
			return
		}
		a.showError(a.open(path))
	}
}
func (a *App) drawGitSidebar() {
	r := a.sideRect
	a.put(r.X+1, r.Y, r.W-6, "SOURCE CONTROL", a.chrome.Foreground(a.colors.muted).Bold(true))
	a.drawButton(r.X+r.W-4, r.Y, "R", "git.refresh", "Refresh Git status", false)
	a.put(r.X+1, r.Y+1, r.W-2, "on "+a.git.Branch, a.chrome.Foreground(a.colors.muted))
	x := r.X + 1
	for _, b := range []struct{ label, id string }{{"Stage", "git.stage"}, {"Unstage", "git.unstage"}, {"Diff", "git.selectedDiff"}} {
		w := len(b.label) + 2
		if x+w > r.X+r.W {
			break
		}
		x += a.drawButton(x, r.Y+2, b.label, b.id, b.label+" selected file", false) + 1
	}
	x = r.X + 1
	for _, b := range []struct{ label, id string }{{"Commit", "git.commit"}, {"Open", "git.open"}} {
		if x+len(b.label)+2 > r.X+r.W {
			break
		}
		x += a.drawButton(x, r.Y+3, b.label, b.id, b.label+" selected changes", false) + 1
	}
	if a.gitError != "" {
		a.put(r.X+1, r.Y+5, r.W-2, "Not a Git worktree", a.muted)
		a.put(r.X+1, r.Y+6, r.W-2, a.gitError, a.muted)
		return
	}
	rows := a.gitRows()
	if len(rows) == 0 {
		label := "Working tree clean"
		if a.gitBusy || a.gitReading {
			label = "Refreshing…"
		}
		a.put(r.X+1, r.Y+5, r.W-2, label, a.muted)
		return
	}
	a.drawSidebarRows(rows, r.Y+5, a.gitTop, a.gitSelected)
}
func (a *App) moveGitSelection(delta int) {
	a.gitSelected = max(0, min(a.gitSelected+delta, len(a.gitEntries())-1))
	revealList(&a.gitTop, a.gitRows(), a.gitSelected, max(1, a.sideRect.H-5))
}
func (a *App) searchRows() []sidebarRow {
	var rows []sidebarRow
	last := ""
	for i, hit := range a.searchResults {
		if last != hit.Path {
			rel, _ := filepath.Rel(a.root, hit.Path)
			rows = append(rows, sidebarRow{Label: filepath.ToSlash(rel), Heading: true})
			last = hit.Path
		}
		rows = append(rows, sidebarRow{Label: fmt.Sprintf("%d:%d %s", hit.Line+1, hit.Column+1, strings.TrimSpace(hit.Text)), Index: i})
	}
	return rows
}
func (a *App) drawSearchSidebar() {
	r := a.sideRect
	a.put(r.X+1, r.Y, r.W-2, "SEARCH", a.chrome.Foreground(a.colors.muted).Bold(true))
	a.searchInputRect = rect{r.X + 1, r.Y + 1, r.W - 2, 1}
	a.searchOffset = a.drawSidebarInput(a.searchInputRect, a.searchQuery, a.searchCursor, "Search workspace…", "searchInput")
	x := r.X + 1
	for _, b := range []struct{ label, id string }{{"Go", "search.run"}, {"Clear", "search.clear"}, {"Stop", "search.stop"}} {
		if x+len(b.label)+2 > r.X+r.W {
			break
		}
		x += a.drawButton(x, r.Y+2, b.label, b.id, b.label+" workspace search", false) + 1
	}
	a.put(r.X+1, r.Y+3, r.W-2, "Regex · smart case", a.muted)
	status := fmt.Sprintf("%d results (limit 2000)", len(a.searchResults))
	if a.searchQuery == "" {
		status = "Type to search"
	} else if a.searchRunning || !a.searchDue.IsZero() {
		status = "Searching…"
	} else if a.searchError != "" {
		status = a.searchError
	}
	a.put(r.X+1, r.Y+4, r.W-2, status, a.muted)
	a.drawSidebarRows(a.searchRows(), r.Y+5, a.searchTop, a.searchSelected)
}
func (a *App) moveSearchSelection(delta int) {
	a.searchSelected = max(0, min(a.searchSelected+delta, len(a.searchResults)-1))
	revealList(&a.searchTop, a.searchRows(), a.searchSelected, max(1, a.sideRect.H-5))
}
func (a *App) openSearchResult() {
	if a.searchSelected >= 0 && a.searchSelected < len(a.searchResults) {
		hit := a.searchResults[a.searchSelected]
		a.jump(hit.Path, hit.Line, hit.Column)
	}
}
func (a *App) sidebarMouse(e *tcell.EventMouse, fresh bool, wheel int) {
	x, y := e.Position()
	switch a.sidebarView {
	case "Search":
		if fresh && a.searchInputRect.contains(x, y) {
			a.focus = "searchInput"
			a.searchCursor = a.searchOffset + text.Offset(a.searchQuery[a.searchOffset:], x-a.searchInputRect.X, a.cfg.TabSize)
			return
		}
		if wheel != 0 {
			a.searchTop = max(0, min(a.searchTop+wheel, max(0, len(a.searchRows())-1)))
		}
		if fresh {
			a.focus = "search"
			for _, hit := range a.sideHits {
				if hit.contains(x, y) {
					a.searchSelected = hit.Index
					a.openSearchResult()
					return
				}
			}
		}
	case "Git":
		if wheel != 0 {
			a.gitTop = max(0, min(a.gitTop+wheel, max(0, len(a.gitRows())-1)))
		}
		if fresh {
			a.focus = "git"
			for _, hit := range a.sideHits {
				if hit.contains(x, y) {
					a.gitSelected = hit.Index
					a.selectedGitDiff()
					return
				}
			}
		}
	default:
		if fresh && a.treeFilterRect.contains(x, y) {
			a.focusTreeFilter()
			a.treeQueryCursor = a.treeQueryOffset + text.Offset(a.treeQuery[a.treeQueryOffset:], max(0, x-a.treeFilterRect.X), a.cfg.TabSize)
			return
		}
		if wheel != 0 {
			a.treeTop = max(0, min(max(0, len(a.tree)-1), a.treeTop+wheel))
		}
		if fresh {
			a.focus = "tree"
			idx := a.treeTop + y - a.sideRect.Y - 3
			if idx >= 0 && idx < len(a.tree) {
				a.treeSelected = idx
				a.toggleTree()
			}
		}
	}
}
