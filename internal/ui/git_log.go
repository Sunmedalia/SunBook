package ui

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/gdamore/tcell/v3"
	gitservice "sunbook/internal/git"
)

func (a *App) refreshGitLog() {
	root, workspace, path := a.git.Root, a.root, a.gitLogPath
	if root == "" {
		return
	}
	a.gitLogSeq++
	seq := a.gitLogSeq
	a.gitLogReading = true
	hasCommits := a.git.HasCommits
	go func() {
		ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
		defer cancel()
		var revisions []gitservice.Revision
		var err error
		if hasCommits {
			revisions, err = gitservice.Log(ctx, root, path)
		}
		a.postQuiet(func() {
			if seq != a.gitLogSeq || workspace != a.root {
				return
			}
			a.gitLogReading = false
			nextError := ""
			if err != nil {
				nextError = err.Error()
			}
			unchanged := a.gitLogLoaded && a.gitLogError == nextError && (err != nil || reflect.DeepEqual(a.gitLog, revisions))
			a.gitLogRefreshed = time.Now()
			a.gitLogLoaded = true
			if unchanged {
				return
			}
			a.invalidate()
			if err != nil {
				a.gitLogError = err.Error()
				return
			}
			selected, topKey := "", ""
			if a.gitLogSelected >= 0 && a.gitLogSelected < len(a.gitLog) {
				selected = a.gitLog[a.gitLogSelected].Hash
			}
			oldRows := a.gitLogRows()
			// Follow new commits when already at the top. Preserve the anchor only
			// when the user has deliberately scrolled into older history.
			if a.gitLogTop > 0 && a.gitLogTop < len(oldRows) {
				topKey = a.gitLogRowKey(oldRows[a.gitLogTop])
			}
			firstLoad := len(a.gitLog) == 0
			a.gitLog = revisions
			a.gitLogError = ""
			a.gitLogSelected = 0
			for i, revision := range revisions {
				if revision.Hash == selected {
					a.gitLogSelected = i
				}
			}
			if a.gitLogOpen == nil {
				a.gitLogOpen = map[string]bool{}
			}
			if firstLoad && len(revisions) > 0 {
				if _, known := a.gitLogOpen[revisions[0].Hash]; !known {
					a.gitLogOpen[revisions[0].Hash] = true
				}
			}
			rows := a.gitLogRows()
			for i, row := range rows {
				if a.gitLogRowKey(row) == topKey {
					a.gitLogTop = i
					break
				}
			}
			if _, index := a.selectedGitLogRow(); index < 0 {
				a.gitLogNode = ""
			}
			a.gitLogTop = max(0, min(a.gitLogTop, max(0, len(rows)-1)))
		})
	}()
}
func (a *App) gitHistory(file bool) {
	a.gitViewSeq++
	path := ""
	if file {
		e, ok := a.selectedGitEntry()
		if !ok {
			return
		}
		f := a.git.Files[e.File]
		path = f.Path
		if f.OldPath != "" {
			path = f.OldPath
		}
	}
	if path != a.gitLogPath {
		a.gitLog = nil
		a.gitLogSelected = 0
		a.gitLogTop = 0
		a.gitLogNode = ""
	}
	a.gitLogPath = path
	a.sidebar = true
	a.sidebarView = "Git"
	a.focus = "gitLog"
	a.refreshGitLog()
}
func (a *App) layoutGitSections() {
	r := a.sideRect
	available := max(0, r.Y+r.H-a.gitListY)
	logHeight := min(available, 1)
	if available >= 8 {
		logHeight = min(9, max(4, available/3))
	}
	if a.gitLogHeight > 0 {
		logHeight = min(available, max(min(3, available), a.gitLogHeight))
	}
	if a.gitLogExpanded || a.focus == "gitLog" && available < 8 && a.gitLogHeight == 0 {
		logHeight = available
	}
	a.gitChangesRect = rect{r.X, a.gitListY, r.W, max(0, available-logHeight)}
	a.gitLogHeader = rect{r.X, a.gitListY + a.gitChangesRect.H, r.W, min(1, logHeight)}
	a.gitLogRect = rect{r.X, a.gitLogHeader.Y + 1, r.W, max(0, logHeight-1)}
}

func (a *App) resizeGitLog(y int) {
	available := max(0, a.sideRect.Y+a.sideRect.H-a.gitBodyStart())
	minimum, maximum := min(3, available), available
	if available >= 5 {
		maximum = available - 2
	}
	a.gitLogHeight = max(minimum, min(maximum, a.sideRect.Y+a.sideRect.H-y))
	a.gitLogExpanded = false
	a.hoverHint = "Drag up/down to resize Git log"
}

func (a *App) drawGitLog() {
	r := a.gitLogHeader
	if r.H == 0 {
		return
	}
	style := a.chrome.Foreground(a.colors.muted).Bold(true)
	if a.focus == "gitLog" {
		style = style.Foreground(a.accent.GetForeground())
	}
	border := a.chrome.Foreground(a.colors.border)
	if a.drag == "gitLogResize" {
		border = border.Foreground(a.accent.GetForeground())
	}
	a.put(r.X, r.Y, r.W, strings.Repeat("─", r.W), border)
	label := fmt.Sprintf("GIT LOG %d", len(a.gitLog))
	if a.gitLogPath != "" {
		label = fmt.Sprintf("FILE LOG %d", len(a.gitLog))
	}
	a.put(r.X+1, r.Y, r.W-5, " "+fitLabel(label, r.W-7)+" ", style)
	symbol := "^"
	if a.gitLogExpanded {
		symbol = "v"
	}
	a.drawButton(r.X+r.W-4, r.Y, a.actionLabel("git.toggleLog", symbol, true), "git.toggleLog", "Expand / restore Git log; h focuses log", a.gitLogExpanded)
	r = a.gitLogRect
	if r.H == 0 {
		return
	}
	if a.gitLogError != "" {
		a.put(r.X+1, r.Y, r.W-2, fitLabel(a.gitLogError, r.W-2), a.chrome.Foreground(a.colors.danger))
		return
	}
	if a.gitLogLoaded && len(a.gitLog) == 0 {
		a.put(r.X+1, r.Y, r.W-2, "No commits yet", a.chrome.Foreground(a.colors.muted))
		return
	}
	a.drawGitLogRows()
}
func (a *App) openGitRevision() {
	if a.gitLogSelected < 0 || a.gitLogSelected >= len(a.gitLog) {
		return
	}
	r := a.gitLog[a.gitLogSelected]
	a.readGitText("Diff: Commit "+r.Hash, func(ctx context.Context, root string) (string, error) { return gitservice.Show(ctx, root, r.Hash, "") })
}

func (a *App) gitLogKey(e *tcell.EventKey, name string) {
	switch name {
	case "up":
		a.moveGitLog(-1)
	case "down":
		a.moveGitLog(1)
	case "pageup":
		a.moveGitLog(-max(1, a.gitLogRect.H))
	case "pagedown":
		a.moveGitLog(max(1, a.gitLogRect.H))
	case "home":
		a.gitLogSelected, a.gitLogNode = 0, ""
		a.moveGitLog(0)
	case "end":
		a.moveGitLog(len(a.gitLogRows()))
	case "enter", "space", " ":
		a.activateGitLogRow()
	case "right":
		a.gitLogTreeArrow(true)
	case "left":
		a.gitLogTreeArrow(false)
	case "d":
		a.openGitRevision()
	case "tab", "shift+tab":
		a.focus = "git"
	case "escape":
		a.focus = "editor"
	case "f5":
		a.refreshGit()
		a.refreshGitLog()
	default:
		if id := a.bindings[name]; id != "" {
			a.execute(id)
		}
	}
}
func (a *App) gitLogMouse(x, y, wheel int, fresh bool) bool {
	if a.gitLogHeader.contains(x, y) {
		a.hoverHint = "Drag up/down to resize Git log"
		if fresh {
			a.focus = "gitLog"
		}
		return true
	}
	r := a.gitLogRect
	if !r.contains(x, y) {
		return false
	}
	rows := a.gitLogRows()
	if wheel != 0 {
		a.gitLogTop = max(0, min(max(0, len(rows)-1), a.gitLogTop+wheel))
		return true
	}
	index := a.gitLogTop + y - r.Y
	if index >= 0 && index < len(rows) {
		row := rows[index]
		revision := a.gitLog[row.Commit]
		a.hoverHint = revision.Hash + " · " + revision.Subject + " · " + revision.Author + " · " + revision.Age
		if row.Kind == "file" || row.Kind == "folder" {
			a.hoverHint = row.Path + " · " + a.hoverHint
			if row.File.OldPath != "" {
				a.hoverHint = row.File.OldPath + " → " + a.hoverHint
			}
		} else {
			a.hoverHint += fmt.Sprintf(" · %d files · click to expand/collapse · d opens patch", len(revision.Files))
		}
		if fresh && row.Kind != "empty" {
			a.focus = "gitLog"
			if row.Kind == "meta" {
				row.Kind = "commit"
			}
			a.selectGitLogRow(row, index)
			a.activateGitLogRow()
		}
	}
	return true
}
