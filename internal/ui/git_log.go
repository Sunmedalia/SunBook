package ui

import (
	"context"
	"fmt"
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
		a.post(func() {
			if seq != a.gitLogSeq || workspace != a.root {
				return
			}
			a.gitLogReading = false
			a.gitLogRefreshed = time.Now()
			a.gitLogLoaded = true
			if err != nil {
				a.gitLogError = err.Error()
				return
			}
			selected := ""
			topHash := ""
			if a.gitLogTop > 0 && a.gitLogTop < len(a.gitLog) {
				topHash = a.gitLog[a.gitLogTop].Hash
			}
			if a.gitLogSelected >= 0 && a.gitLogSelected < len(a.gitLog) {
				selected = a.gitLog[a.gitLogSelected].Hash
			}
			a.gitLog = revisions
			a.gitLogError = ""
			a.gitLogSelected = max(0, min(a.gitLogSelected, len(revisions)-1))
			for i, r := range revisions {
				if topHash != "" && r.Hash == topHash {
					a.gitLogTop = i
				}
				if r.Hash == selected {
					a.gitLogSelected = i
				}
			}
			a.gitLogTop = max(0, min(a.gitLogTop, max(0, len(revisions)-1)))
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
	for i, y := a.gitLogTop, r.Y; i < len(a.gitLog) && y < r.Y+r.H; i, y = i+1, y+2 {
		revision := a.gitLog[i]
		style := a.chrome
		if a.focus == "gitLog" && i == a.gitLogSelected {
			style = a.selected
			a.fill(rect{r.X, y, r.W, min(2, r.Y+r.H-y)}, style)
		}
		hashWidth := min(7, len(revision.Hash))
		a.put(r.X+1, y, hashWidth, revision.Hash[:hashWidth], style.Foreground(a.accent.GetForeground()))
		a.put(r.X+hashWidth+2, y, r.W-hashWidth-3, fitLabel(revision.Subject, r.W-hashWidth-3), style)
		if y+1 < r.Y+r.H {
			a.put(r.X+2, y+1, r.W-3, fitLabel(revision.Author+" · "+revision.Age, r.W-3), style.Foreground(a.colors.muted))
		}
	}
	a.sidebarScroll(a.gitLogRect, a.gitLogTop*2, len(a.gitLog)*2)
}
func (a *App) moveGitLog(delta int) {
	a.gitLogSelected = max(0, min(a.gitLogSelected+delta, len(a.gitLog)-1))
	visible := max(1, (a.gitLogRect.H+1)/2)
	if a.gitLogSelected < a.gitLogTop {
		a.gitLogTop = a.gitLogSelected
	}
	if a.gitLogSelected >= a.gitLogTop+visible {
		a.gitLogTop = a.gitLogSelected - visible + 1
	}
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
		a.moveGitLog(-max(1, a.gitLogRect.H/2))
	case "pagedown":
		a.moveGitLog(max(1, a.gitLogRect.H/2))
	case "home":
		a.gitLogSelected = 0
		a.moveGitLog(0)
	case "end":
		a.gitLogSelected = len(a.gitLog) - 1
		a.moveGitLog(0)
	case "enter":
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
	if wheel != 0 {
		a.gitLogTop = max(0, min(max(0, len(a.gitLog)-1), a.gitLogTop+wheel))
		return true
	}
	index := a.gitLogTop + (y-r.Y)/2
	if index >= 0 && index < len(a.gitLog) {
		revision := a.gitLog[index]
		a.hoverHint = revision.Hash + " · " + revision.Subject + " · " + revision.Author + " · " + revision.Age
		if fresh {
			a.focus = "gitLog"
			a.gitLogSelected = index
			a.openGitRevision()
		}
	}
	return true
}
