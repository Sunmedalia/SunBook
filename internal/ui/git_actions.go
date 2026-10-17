package ui

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	gitservice "sunbook/internal/git"
)

func (a *App) refreshGit() {
	if a.gitBusy || a.gitReading {
		return
	}
	a.gitReading = true
	root := a.root
	a.gitSeq++
	seq := a.gitSeq
	go func() {
		ctx, cancel := context.WithTimeout(a.ctx, 8*time.Second)
		defer cancel()
		status, err := gitservice.ReadStatus(ctx, root)
		a.postQuiet(func() {
			if seq != a.gitSeq || a.root != root {
				return
			}
			a.gitReading = false
			nextError := ""
			if err != nil {
				nextError = err.Error()
			}
			if (reflect.DeepEqual(a.git, status) || err != nil) && a.gitError == nextError {
				if !a.gitLogReading && time.Since(a.gitLogRefreshed) >= time.Minute {
					a.refreshGitLog()
				}
				return
			}
			a.invalidate()
			if err != nil {
				a.gitError = err.Error()
				a.git = gitservice.Status{}
				return
			}
			oldPath := ""
			oldStaged := false
			if e, ok := a.selectedGitEntry(); ok {
				oldPath = a.git.Files[e.File].Path
				oldStaged = e.Staged
			}
			headChanged := a.git.Head != status.Head
			a.git = status
			a.gitError = ""
			a.gitSelected = max(0, min(a.gitSelected, len(a.gitEntries())-1))
			fallback := -1
			for i, e := range a.gitEntries() {
				if a.git.Files[e.File].Path == oldPath {
					fallback = i
					if e.Staged == oldStaged {
						break
					}
				}
			}
			if fallback >= 0 {
				a.gitSelected = fallback
			}
			rows := a.gitRows()
			if len(rows) > 0 {
				a.selectGitRow(rows[a.selectedGitRow(rows)])
			} else {
				a.gitGroupFocus = ""
			}
			a.gitTop = max(0, min(a.gitTop, max(0, len(a.gitRows())-1)))
			if headChanged || !a.gitLogReading && (!a.gitLogLoaded || time.Since(a.gitLogRefreshed) >= time.Minute) {
				a.refreshGitLog()
			}
		})
	}()
}
func (a *App) mutateGit(label string, run func(context.Context, string) error, done func()) {
	if a.gitBusy {
		a.notify("Git operation in progress")
		return
	}
	if a.git.Root == "" {
		a.notify("Open a Git repository first")
		return
	}
	root, workspace := a.git.Root, a.root
	a.gitSeq++
	seq := a.gitSeq
	a.gitViewSeq++
	a.gitReading = false
	a.gitBusy = true
	a.gitOperation = label
	a.gitActionError = ""
	a.notify(label + "…")
	go func() {
		ctx, cancel := context.WithTimeout(a.ctx, 60*time.Second)
		defer cancel()
		err := run(ctx, root)
		a.post(func() {
			if a.root != workspace || seq != a.gitSeq {
				return
			}
			a.gitBusy = false
			a.gitOperation = ""
			if err != nil {
				a.gitActionError = err.Error()
				a.checkFiles()
				a.scanTree()
				a.showError(err)
			} else {
				if done != nil {
					done()
				}
				a.notify(label + " complete")
			}
			a.refreshGit()
		})
	}()
}
func (a *App) gitAction(stage bool) {
	e, ok := a.selectedGitEntry()
	if !ok || a.gitBusy {
		return
	}
	f := a.git.Files[e.File]
	if stage && e.Staged || !stage && !e.Staged {
		return
	}
	label := "Staging " + f.Path
	if !stage {
		label = "Unstaging " + f.Path
	}
	a.mutateGit(label, func(ctx context.Context, root string) error {
		if stage {
			return gitservice.StageFile(ctx, root, f)
		}
		return gitservice.UnstageFile(ctx, root, f)
	}, nil)
}
func (a *App) gitAllAction(stage bool) {
	staged, changed, _ := a.gitCounts()
	if stage && changed == 0 || !stage && staged == 0 {
		a.notify("No matching changes")
		return
	}
	if stage {
		a.mutateGit("Staging all repository changes", gitservice.StageAll, nil)
	} else {
		a.mutateGit("Unstaging all files", gitservice.UnstageAll, nil)
	}
}
func (a *App) gitStageGroup(group string) {
	var files []gitservice.File
	for _, f := range a.git.Files {
		if f.Changed() && gitGroup(f, false) == group {
			files = append(files, f)
		}
	}
	if len(files) == 0 {
		return
	}
	a.mutateGit(fmt.Sprintf("Staging %d files", len(files)), func(ctx context.Context, root string) error { return gitservice.StageFiles(ctx, root, files) }, nil)
}
func (a *App) gitDiff(staged bool) {
	e, ok := a.selectedGitEntry()
	if !ok || a.git.Root == "" {
		return
	}
	f := a.git.Files[e.File]
	a.readGitComparison(a.git.Root, func(ctx context.Context, root string) (gitservice.Comparison, error) {
		return gitservice.Compare(ctx, root, f, staged)
	})
}
func (a *App) readGitComparison(root string, read func(context.Context, string) (gitservice.Comparison, error)) {
	workspace := a.root
	a.gitViewSeq++
	seq := a.gitViewSeq
	base := a.base
	syntax, themeSeq := a.themeSyntax, a.themeSeq
	go func() {
		ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
		defer cancel()
		pair, err := read(ctx, root)
		var view *gitComparison
		if err == nil {
			view = newGitComparison(pair)
			view.highlight(base, syntax)
		}
		a.post(func() {
			if seq != a.gitViewSeq || workspace != a.root {
				return
			}
			if err != nil {
				a.showError(err)
				return
			}
			if c := a.comparison; c != nil && c.Pair.Path == view.Pair.Path && c.Pair.OldLabel == view.Pair.OldLabel && c.Pair.NewLabel == view.Pair.NewLabel {
				view.Top, view.Left, view.Pane, view.Ratio = c.Top, c.Left, c.Pane, c.Ratio
			}
			a.comparison = view
			if themeSeq != a.themeSeq {
				view.Highlights = [2][]tokenSpan{}
				a.rehighlightComparison(view)
			}
			a.completion = nil
			a.completionDue = zeroTime
		})
	}()
}
func (a *App) readGitText(title string, read func(context.Context, string) (string, error)) {
	if a.git.Root == "" {
		return
	}
	root, workspace := a.git.Root, a.root
	a.gitViewSeq++
	seq := a.gitViewSeq
	a.notify("Loading " + title + "…")
	go func() {
		ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
		defer cancel()
		body, err := read(ctx, root)
		a.post(func() {
			if a.root != workspace || seq != a.gitViewSeq {
				return
			}
			if err != nil {
				a.showError(err)
				return
			}
			a.showText(title, body)
			a.modal.Hint = "↑/↓ scroll · ←/→ pan · PgUp/PgDn · Home resets · Esc closes"
			a.message = ""
		})
	}()
}
func (a *App) focusGitMessage() {
	a.sidebar = true
	a.sidebarView = "Git"
	a.focus = "gitMessage"
	a.gitMessageCursor = len(a.gitMessage)
}
func (a *App) commit() {
	staged, _, conflicts := a.gitCounts()
	if a.gitBusy {
		a.notify("Git operation in progress")
		return
	}
	if conflicts > 0 {
		a.notify("Resolve and stage conflicts before committing")
		return
	}
	if staged == 0 {
		a.notify("Stage changes before committing")
		return
	}
	if strings.TrimSpace(a.gitMessage) == "" {
		a.focusGitMessage()
		a.notify("Write a commit message, then press Enter to commit staged files")
		return
	}
	msg := a.gitMessage
	a.mutateGit("Committing staged changes", func(ctx context.Context, root string) error { return gitservice.Commit(ctx, root, msg) }, func() {
		if a.gitMessage == msg {
			a.gitMessage = ""
			a.gitMessageCursor = 0
			a.gitMessageTop = 0
		}
		// The status refresh after this callback loads the new HEAD. Reveal it
		// in repository history and supersede any older/file-specific request.
		a.gitLogSeq++
		a.gitLogReading = false
		a.gitLogPath = ""
		a.gitLogSelected = -1
		a.gitLogNode = ""
		a.gitLogTop = 0
	})
}
func (a *App) gitMore() {
	staged, changed, _ := a.gitCounts()
	var items []item
	items = append(items, item{Label: "Branches…", Detail: a.git.Branch, Action: a.branchManager})
	if a.gitActionError != "" {
		errText := a.gitActionError
		items = append(items, item{Label: "Show last Git error", Action: func() { a.showText("Git Error", errText) }})
	}
	if _, ok := a.selectedGitEntry(); ok {
		items = append(items, item{Label: "View selected diff", Detail: "Enter", Action: a.selectedGitDiff}, item{Label: "Open selected file", Detail: "o", Action: a.openGitFile})
	}
	if changed > 0 {
		items = append(items, item{Label: fmt.Sprintf("Stage all changes (%d)", changed), Detail: "entire repository", Action: func() { a.gitAllAction(true) }})
	}
	if staged > 0 {
		items = append(items, item{Label: fmt.Sprintf("Unstage all files (%d)", staged), Action: func() { a.gitAllAction(false) }})
	}
	if a.git.HasCommits {
		items = append(items, item{Label: "Recent commits", Detail: "latest 40", Action: func() { a.gitHistory(false) }})
		if _, ok := a.selectedGitEntry(); ok {
			items = append(items, item{Label: "Selected file history", Detail: "follows renames", Action: func() { a.gitHistory(true) }})
		}
	}
	items = append(items, item{Label: "Repository details", Action: a.gitDetails}, item{Label: "Refresh status", Detail: "F5", Action: a.refreshGit})
	a.choose("Source Control", items)
}
func (a *App) gitDetails() {
	upstream := a.git.Upstream
	if upstream == "" {
		upstream = "No upstream configured"
	}
	a.showText("Repository", fmt.Sprintf("Root: %s\nBranch: %s\nUpstream: %s\nAhead: %d   Behind: %d\n\nCounts use locally available refs.\nStage and commit use saved files on disk.", a.git.Root, a.git.Branch, upstream, a.git.Ahead, a.git.Behind))
}
