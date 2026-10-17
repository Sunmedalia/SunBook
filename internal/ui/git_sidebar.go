package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gitservice "sunbook/internal/git"
	"sunbook/internal/text"
)

func gitGroup(f gitservice.File, staged bool) string {
	switch {
	case f.Conflicted():
		return "CONFLICTS"
	case staged:
		return "STAGED"
	case f.Untracked():
		return "UNTRACKED"
	default:
		return "CHANGES"
	}
}
func (a *App) allGitEntries() []gitEntry {
	var entries []gitEntry
	for _, group := range []string{"CONFLICTS", "STAGED", "CHANGES", "UNTRACKED"} {
		for i, f := range a.git.Files {
			staged := group == "STAGED"
			if (staged && f.Staged() || !staged && f.Changed()) && gitGroup(f, staged) == group {
				entries = append(entries, gitEntry{i, staged})
			}
		}
	}
	return entries
}
func (a *App) gitEntries() []gitEntry {
	entries := a.allGitEntries()
	if a.gitFilter == "" {
		return entries
	}
	filtered := make([]gitEntry, 0, len(entries))
	for _, e := range entries {
		f := a.git.Files[e.File]
		if strings.Contains(strings.ToLower(f.Path+" "+f.OldPath), strings.ToLower(a.gitFilter)) {
			filtered = append(filtered, e)
		}
	}
	return filtered
}
func (a *App) selectedGitEntry() (gitEntry, bool) {
	entries := a.gitEntries()
	if a.gitGroupFocus != "" {
		return gitEntry{}, false
	}
	if a.gitSelected < 0 || a.gitSelected >= len(entries) {
		return gitEntry{}, false
	}
	e := entries[a.gitSelected]
	if a.gitGroupCollapsed(gitGroup(a.git.Files[e.File], e.Staged)) {
		return gitEntry{}, false
	}
	return e, true
}
func (a *App) gitCounts() (staged, changed, conflicts int) {
	for _, f := range a.git.Files {
		if f.Staged() {
			staged++
		}
		if f.Changed() {
			changed++
		}
		if f.Conflicted() {
			conflicts++
		}
	}
	return
}
func (a *App) gitRows() []sidebarRow {
	entries := a.gitEntries()
	counts := map[string]int{}
	totals := map[string]int{}
	for _, e := range a.allGitEntries() {
		totals[gitGroup(a.git.Files[e.File], e.Staged)]++
	}
	for _, e := range entries {
		counts[gitGroup(a.git.Files[e.File], e.Staged)]++
	}
	var rows []sidebarRow
	last := ""
	for i, e := range entries {
		f := a.git.Files[e.File]
		group := gitGroup(f, e.Staged)
		if group != last {
			label := fmt.Sprintf("%s %d", group, counts[group])
			if a.gitFilter != "" {
				label = fmt.Sprintf("%s %d/%d", group, counts[group], totals[group])
			}
			rows = append(rows, sidebarRow{Label: label, Heading: true, Group: group})
			last = group
		}
		if !a.gitGroupCollapsed(group) {
			rows = append(rows, sidebarRow{Label: f.Path, Index: i, Group: group})
		}
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
		if _, e := os.Stat(path); e != nil {
			a.showError(e)
			return
		}
		a.showError(a.open(path))
	}
}
func (a *App) gitBodyStart() int {
	if a.sideRect.H >= 22 {
		return a.sideRect.Y + 9
	}
	if a.sideRect.H >= 18 {
		return a.sideRect.Y + 7
	}
	return a.sideRect.Y + 5
}
func (a *App) drawGitSidebar() {
	r := a.sideRect
	if r.H < 18 && a.focus == "gitFilter" {
		a.focus = "git"
	}
	a.drawSidebarHeader("SOURCE CONTROL", 4)
	a.drawButton(r.X+r.W-4, r.Y, a.actionLabel("git.refresh", "Refresh", true), "git.refresh", "Refresh Git status (F5)", false)
	branch := a.git.Branch
	if branch == "" {
		branch = "Repository"
	}
	if a.git.Detached {
		branch = "detached · " + branch
	}
	suffix := ""
	if a.git.Upstream != "" {
		suffix = fmt.Sprintf(" ↑%d ↓%d", a.git.Ahead, a.git.Behind)
	}
	suffix = fitLabel(suffix, r.W-2)
	branchStyle := a.sidebarBand(r.Y + 1)
	if a.git.Root != "" {
		if a.hoverCommand == "git.branches" {
			branchStyle = branchStyle.Background(a.colors.hover)
		}
		a.buttons = append(a.buttons, button{rect: rect{r.X, r.Y + 1, r.W, 1}, Command: "git.branches", Hint: "Manage branches: switch, create, rename, merge and delete (b)", Disabled: a.gitBusy})
	}
	a.put(r.X+1, r.Y+1, 2, a.actionIcon("git.branches", "Branches"), branchStyle.Foreground(a.colors.folder))
	a.put(r.X+3, r.Y+1, r.W-4, fitLabel(branch, max(0, r.W-6-text.Width(suffix, 4))), branchStyle.Bold(true))
	if a.git.Root != "" {
		a.put(r.X+r.W-3-text.Width(suffix, 4), r.Y+1, 1, "▾", branchStyle.Foreground(a.colors.muted))
	}
	a.put(r.X+r.W-1-text.Width(suffix, 4), r.Y+1, text.Width(suffix, 4), suffix, branchStyle.Foreground(a.colors.folder))
	a.gitListY = a.gitBodyStart()
	a.layoutGitSections()
	if a.gitError != "" {
		if a.focus == "gitMessage" || a.focus == "gitFilter" || a.focus == "gitLog" {
			a.focus = "git"
		}
		a.put(r.X+1, r.Y+3, r.W-2, "No Git repository", a.chrome.Bold(true))
		a.put(r.X+1, r.Y+5, r.W-2, "Open a repository folder", a.chrome.Foreground(a.colors.muted))
		a.put(r.X+1, r.Y+6, r.W-2, "or use git init in F4.", a.chrome.Foreground(a.colors.muted))
		if strings.Contains(a.gitError, "not a git repository") {
			return
		}
		a.put(r.X+1, r.Y+8, r.W-2, fitLabel(a.gitError, r.W-2), a.chrome.Foreground(a.colors.danger))
		return
	}
	messageHeight := 1
	if r.H >= 22 {
		messageHeight = 3
	}
	a.gitMessageRect = rect{r.X + 1, r.Y + 2, r.W - 2, messageHeight}
	a.drawGitMessage()
	buttonY := r.Y + 2 + messageHeight
	staged, changed, conflicts := a.gitCounts()
	moreLabel := a.actionLabel("git.more", "More", true)
	moreWidth := text.Width(moreLabel, 4) + 2
	commitWidth := r.W - 3 - moreWidth
	label := "Commit"
	if staged > 0 {
		label = fmt.Sprintf("Commit · %d", staged)
	}
	if a.gitBusy {
		label = "Working…"
	}
	label = fitLabel(label, max(0, commitWidth-2))
	padding := max(0, commitWidth-2-text.Width(label, 4))
	label = strings.Repeat(" ", padding/2) + label + strings.Repeat(" ", padding-padding/2)
	a.drawButton(r.X+1, buttonY, label, "git.commit", "Commit staged files · Enter in message · failed commits keep your draft", false)
	a.drawButton(r.X+r.W-1-moreWidth, buttonY, moreLabel, "git.more", "Stage all, unstage all, history and Git errors", false)
	actionY := buttonY + 1
	if r.H >= 18 {
		a.gitFilterRect = rect{r.X + 1, buttonY + 1, r.W - 2, 1}
		a.gitFilterOffset = a.drawSidebarInput(a.gitFilterRect, a.gitFilter, a.gitFilterCursor, "Filter changes… (/)", "gitFilter")
		actionY++
	}
	x := r.X + 1
	stageID, stageLabel := "git.stage", "Stage"
	if e, ok := a.selectedGitEntry(); ok && e.Staged {
		stageID, stageLabel = "git.unstage", "Unstage"
	}
	for _, b := range []struct{ label, id, hint string }{
		{stageLabel, stageID, stageLabel + " selected file (s / u)"},
		{"Diff", "git.selectedDiff", "View selected version (Enter)"},
		{"Open", "git.open", "Open selected file (o)"},
		{"Log", "git.history", "Focus Git log (h)"},
	} {
		label := a.actionLabel(b.id, b.label, b.id != "git.stage" && b.id != "git.unstage")
		if x+text.Width(label, 4)+2 > r.X+r.W {
			continue
		}
		x += a.drawButton(x, actionY, label, b.id, b.hint, false) + 1
	}
	if r.H >= 18 {
		summary := fmt.Sprintf("%d staged · %d changes", staged, changed)
		style := a.chrome.Foreground(a.colors.muted)
		if staged == 0 && changed > 0 {
			summary = "Stage files to commit"
		}
		if conflicts > 0 {
			summary = fmt.Sprintf("%d conflicts to resolve", conflicts)
			style = style.Foreground(a.colors.danger)
		}
		if a.gitActionError != "" {
			summary = "Failed: " + a.gitActionError
			style = style.Foreground(a.colors.danger)
		}
		if a.gitBusy {
			summary = a.gitOperation + "…"
			style = style.Foreground(a.accent.GetForeground())
		}
		summaryY := actionY + 1
		style = style.Background(a.sidebarBand(summaryY).GetBackground())
		a.put(r.X+1, summaryY, r.W-2, fitLabel(summary, r.W-2), style)
	}

	a.drawGitLog()
	rows := a.gitRows()
	if len(rows) == 0 {
		label, detail := "Working tree clean", "Saved changes appear here."
		if a.gitFilter != "" {
			label, detail = "No matching changes", "Esc in filter clears it."
		}
		if a.gitChangesRect.H > 1 {
			a.put(r.X+1, a.gitListY+1, r.W-2, label, a.chrome.Foreground(a.colors.success))
		}
		if a.gitChangesRect.H > 3 {
			a.put(r.X+1, a.gitListY+3, r.W-2, detail, a.chrome.Foreground(a.colors.muted))
		}
		return
	}
	a.gitTop = max(0, min(a.gitTop, max(0, len(rows)-1)))
	entries := a.gitEntries()
	for y, i := a.gitListY, a.gitTop; y < a.gitChangesRect.Y+a.gitChangesRect.H && i < len(rows); y, i = y+1, i+1 {
		row := rows[i]
		if row.Heading {
			fields := strings.Fields(row.Label)
			count := fields[len(fields)-1]
			arrow := "▾"
			if a.gitGroupCollapsed(row.Group) {
				arrow = "▸"
			}
			style := a.sidebarBand(y).Foreground(a.colors.muted).Bold(true)
			if row.Group == "CONFLICTS" {
				style = style.Foreground(a.colors.danger)
			}
			if a.hoverCommand == "git.toggle"+row.Group {
				style = style.Background(a.colors.hover)
				a.fill(rect{r.X, y, r.W - 4, 1}, style)
			}
			if a.gitGroupFocus == row.Group {
				style = a.drawFileSelection(rect{r.X, y, r.W - 4, 1}, a.focus == "git").Bold(true)
			}
			countX := a.sidebarCount(r.X+r.W-5, y, count, style)
			a.put(r.X+1, y, max(0, countX-r.X-2), fitLabel(arrow+" "+row.Group, max(0, countX-r.X-2)), style)
			a.buttons = append(a.buttons, button{rect: rect{r.X, y, r.W - 4, 1}, Command: "git.toggle" + row.Group, Hint: "Expand / collapse " + row.Group + " · Enter or ←/→"})
			id, hint, label := "git.stage"+row.Group, "Stage entire group, including filtered files", "+"
			if row.Group == "STAGED" {
				id, hint, label = "git.unstageAll", "Unstage all files", "−"
			}
			a.drawButton(r.X+r.W-4, y, a.actionLabel(id, label, true), id, hint, false)
			continue
		}
		e := entries[row.Index]
		f := a.git.Files[e.File]
		style := a.chrome
		if a.gitGroupFocus == "" && row.Index == a.gitSelected {
			style = a.drawFileSelection(rect{r.X, y, r.W, 1}, a.focus == "git")
		}
		status := f.Worktree
		if e.Staged {
			status = f.Index
		}
		color := a.accent.GetForeground()
		switch {
		case f.Conflicted():
			status = "!"
			color = a.colors.danger
		case f.Untracked():
			status = "?"
			color = a.colors.success
		case status == "A":
			color = a.colors.success
		case status == "D":
			color = a.colors.danger
		case status == "R":
			color = a.colors.folder
		}
		a.put(r.X+1, y, 1, status, style.Foreground(color).Bold(true))
		name := filepath.Base(f.Path)
		n := a.drawFileIcon(r.X+3, y, r.W-4, f.Path, false, false, style)
		nameX, nameWidth := r.X+3+n, r.W-4-n
		a.put(nameX, y, nameWidth, fitLabel(name, nameWidth), style)
		nameW := text.Width(name, 4)
		if dir := filepath.Dir(f.Path); dir != "." && nameW+3 < nameWidth {
			a.put(nameX+nameW+1, y, nameWidth-nameW-1, tailLabel(dir, nameWidth-nameW-1), style.Foreground(a.colors.muted))
		}
		a.sideHits = append(a.sideHits, sideHit{rect{r.X, y, r.W, 1}, row.Index})
	}
	a.sidebarScroll(a.gitChangesRect, a.gitTop, len(rows))
}

func (a *App) gitGroupCollapsed(group string) bool {
	return a.gitCollapsed[group]
}
func (a *App) toggleGitGroup(group string) {
	if a.gitCollapsed == nil {
		a.gitCollapsed = map[string]bool{}
	}
	a.gitCollapsed[group] = !a.gitCollapsed[group]
	a.gitGroupFocus = group
	a.focus = "git"
	a.moveGitSelection(0)
}
func (a *App) selectGitRow(row sidebarRow) {
	if row.Heading {
		a.gitGroupFocus = row.Group
	} else {
		a.gitGroupFocus = ""
		a.gitSelected = row.Index
	}
}
func (a *App) selectedGitRow(rows []sidebarRow) int {
	for i, row := range rows {
		if row.Heading && row.Group == a.gitGroupFocus || !row.Heading && a.gitGroupFocus == "" && row.Index == a.gitSelected {
			return i
		}
	}
	// A collapsed group must never leave an invisible file selected.
	entries := a.gitEntries()
	if a.gitSelected >= 0 && a.gitSelected < len(entries) {
		e := entries[a.gitSelected]
		group := gitGroup(a.git.Files[e.File], e.Staged)
		for i, row := range rows {
			if row.Heading && row.Group == group {
				return i
			}
		}
	}
	return 0
}
func (a *App) moveGitSelection(delta int) {
	rows := a.gitRows()
	if len(rows) == 0 {
		a.gitGroupFocus = ""
		a.gitSelected = 0
		return
	}
	index := max(0, min(len(rows)-1, a.selectedGitRow(rows)+delta))
	a.selectGitRow(rows[index])
	height := max(1, a.gitChangesRect.H)
	if index < a.gitTop {
		a.gitTop = index
	}
	if index >= a.gitTop+height {
		a.gitTop = index - height + 1
	}
}
func (a *App) gitFoldSelection(expand bool) {
	rows := a.gitRows()
	if len(rows) == 0 {
		return
	}
	row := rows[a.selectedGitRow(rows)]
	group := row.Group
	if expand {
		if a.gitGroupCollapsed(group) {
			a.toggleGitGroup(group)
		} else if row.Heading {
			a.moveGitSelection(1)
		}
	} else {
		if !row.Heading {
			a.gitGroupFocus = group
			a.moveGitSelection(0)
		} else if !a.gitGroupCollapsed(group) {
			a.toggleGitGroup(group)
		}
	}
}
