package ui

import (
	"github.com/gdamore/tcell/v3"
	"sunbook/internal/text"
)

type sideHit struct {
	rect
	Index int
}
type sidebarRow struct {
	Label   string
	Group   string
	Index   int
	Heading bool
}
type gitEntry struct {
	File   int
	Staged bool
}

func sidebarFocus(f string) bool {
	return f == "plugins" || f == "tree" || f == "filter" || f == "searchInput" || f == "search" || f == "searchInclude" || f == "searchExclude" || f == "git" || f == "gitMessage" || f == "gitFilter" || f == "gitLog"
}
func (a *App) focusSidebar() {
	switch a.sidebarView {
	case "Plugins":
		a.focus = "plugins"
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
	if view == "Plugins" {
		a.refreshPlugins()
	}
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
func (a *App) drawSidebar() {
	a.fill(a.sideRect, a.chrome)
	switch a.sidebarView {
	case "Plugins":
		a.drawPlugins()
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
	if value == "" {
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
func (a *App) sidebarMouse(e *tcell.EventMouse, fresh bool, wheel int) {
	x, y := e.Position()
	switch a.sidebarView {
	case "Plugins":
		if wheel != 0 {
			a.pluginTop = max(0, min(a.pluginTop+wheel, max(0, len(a.pluginStatuses)-1)))
			a.pluginSelected = max(a.pluginTop, min(a.pluginSelected, a.pluginTop+max(1, (a.sideRect.H-4)/3)-1))
		}
		if fresh {
			a.focus = "plugins"
		}
		for _, hit := range a.sideHits {
			if hit.contains(x, y) {
				if hit.Index < len(a.pluginStatuses) {
					status := a.pluginStatuses[hit.Index]
					a.hoverHint = status.Entry.Manifest.Name + " · " + string(status.State)
					if status.Error != "" {
						a.hoverHint += " · " + status.Error
					}
				}
				if !fresh {
					return
				}
				a.pluginSelected = hit.Index
				a.pluginDetails()
				return
			}
		}
	case "Search":
		if a.searchOptionsMouse(x, y, fresh) {
			return
		}
		if fresh && a.searchInputRect.contains(x, y) {
			a.focus = "searchInput"
			a.searchCursor = a.searchOffset + text.Offset(a.searchQuery[a.searchOffset:], x-a.searchInputRect.X, a.cfg.TabSize)
			return
		}
		if wheel != 0 {
			a.searchTop = max(0, min(a.searchTop+wheel, max(0, len(a.searchRows())-max(1, a.sideRect.Y+a.sideRect.H-a.searchBodyStart()))))
		}
		for _, h := range a.searchGroupHits {
			if h.contains(x, y) {
				a.hoverHint = h.Path + " · click to expand / collapse"
				if fresh {
					a.toggleSearchGroup(h.Path)
				}
				return
			}
		}
		if fresh {
			a.focus = "search"
		}
		for _, hit := range a.sideHits {
			if hit.contains(x, y) && hit.Index < len(a.searchResults) {
				a.hoverHint = a.searchResults[hit.Index].Path + " · click to preview · double-click to keep open"
				if fresh {
					a.searchSelected = hit.Index
					keep := a.doubleFileClick("search", a.searchResults[hit.Index].Path, x, y)
					a.openSearchResultMode(!keep)
				}
				return
			}
		}
	case "Git":
		if a.gitLogMouse(x, y, wheel, fresh) {
			return
		}
		if a.gitMessageMouse(x, y, fresh) {
			return
		}
		for _, field := range []struct {
			r      rect
			focus  string
			value  string
			offset int
			cursor *int
		}{
			{a.gitFilterRect, "gitFilter", a.gitFilter, a.gitFilterOffset, &a.gitFilterCursor},
		} {
			if fresh && field.r.contains(x, y) {
				a.focus = field.focus
				*field.cursor = field.offset + text.Offset(field.value[field.offset:], x-field.r.X, a.cfg.TabSize)
				return
			}
		}
		if wheel != 0 {
			a.gitTop = max(0, min(a.gitTop+wheel, max(0, len(a.gitRows())-1)))
		}
		if fresh {
			a.focus = "git"
		}
		for _, hit := range a.sideHits {
			if hit.contains(x, y) {
				entries := a.gitEntries()
				if hit.Index < len(entries) {
					e := entries[hit.Index]
					f := a.git.Files[e.File]
					a.hoverHint = f.Path + " · Enter diff · s stage · u unstage · o open"
					if f.OldPath != "" {
						a.hoverHint = f.OldPath + " → " + a.hoverHint
					}
				}
				if fresh {
					a.gitGroupFocus = ""
					a.gitSelected = hit.Index
					a.selectedGitDiff()
				}
				return
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
			if y == a.sideRect.Y+1 {
				a.selectTree(-1)
				return
			}
			idx := a.treeTop + y - a.sideRect.Y - 3
			if idx >= 0 && idx < len(a.tree) {
				a.selectTree(idx)
				entry := a.tree[idx]
				if entry.Dir {
					a.toggleTree()
				} else {
					keep := a.doubleFileClick("explorer", entry.Path, x, y)
					a.showError(a.openFile(entry.Path, !keep))
				}
			}
		}
	}
}
