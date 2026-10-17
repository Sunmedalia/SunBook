package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"sunbook/internal/editor"
	gitservice "sunbook/internal/git"
)

func (a *App) branchManager() {
	if a.gitBusy {
		a.notify("Wait for the current Git operation")
		return
	}
	a.branchSeq++
	seq, workspace := a.branchSeq, a.root
	a.setModal(&modal{Title: "Branches", Body: []string{"Reading local and remote branches…"}, Hint: "Esc closes"})
	m := a.modal
	go func() {
		ctx, cancel := context.WithTimeout(a.ctx, 8*time.Second)
		defer cancel()
		status, err := gitservice.ReadStatus(ctx, workspace)
		var branches []gitservice.Branch
		merging := false
		if err == nil {
			branches, err = gitservice.Branches(ctx, status.Root)
			merging = gitservice.MergeInProgress(ctx, status.Root)
		}
		a.post(func() {
			if a.root != workspace || a.branchSeq != seq || a.modal != m {
				return
			}
			if err != nil {
				a.showText("Branches unavailable", err.Error())
				return
			}
			a.git = status
			a.branchBase = status
			a.branches = branches
			a.mergeInProgress = merging
			a.showBranchList()
		})
	}()
}
func (a *App) showBranchList() {
	items := []item{{Label: "Create and switch…", Detail: "from current HEAD", Action: func() { a.createBranchFrom("", false) }}}
	if a.mergeInProgress {
		items = append(items, item{Label: "Abort current merge…", Detail: "review before aborting", Action: a.abortBranchMerge})
	}
	for _, branch := range a.branches {
		b := branch
		group := "local"
		if b.Remote {
			group = "remote"
		}
		label := b.Name
		detail := group + " · " + b.Hash
		if b.Current {
			label = "● " + label
			detail = "current · " + b.Hash
		} else if b.Worktree != "" {
			detail = "in another worktree"
		}
		items = append(items, item{Label: label, Detail: detail, Action: func() { a.branchActions(b) }})
	}
	a.setModal(&modal{Title: "Branches", Subtitle: a.git.Branch + " · " + filepath.Base(a.git.Root), AutoHeight: true, Placeholder: "Find a local or remote branch…", InputEnabled: true, Filter: true, Items: items, Hint: "Type to filter · Enter actions · Esc closes"})
}
func (a *App) branchActions(b gitservice.Branch) {
	var items []item
	if b.Remote {
		items = append(items, item{Label: "Create tracking branch…", Detail: b.Name, Action: func() { a.createBranchFrom(b.Ref, true) }})
	} else {
		if !b.Current && b.Worktree == "" {
			items = append(items, item{Label: "Switch to branch", Detail: b.Name, Action: func() {
				a.changeBranches("Switching to "+b.Name, func(ctx context.Context, root string) error { return gitservice.SwitchBranch(ctx, root, b.Name) })
			}})
		}
		items = append(items, item{Label: "Rename branch…", Action: func() {
			a.compactPrompt("Rename "+b.Name, b.Name, "New branch name", func(name string) {
				a.mutateGit("Renaming branch", func(ctx context.Context, root string) error { return gitservice.RenameBranch(ctx, root, b.Name, name) }, a.branchMutationDone)
			})
		}})
		items = append(items, item{Label: "Set upstream…", Detail: b.Upstream, Action: func() { a.branchUpstream(b.Name) }})
		if !b.Current && b.Worktree == "" {
			items = append(items, item{Label: "Delete merged branch…", Detail: "keeps unmerged work", Action: func() {
				a.branchChoose("Delete branch "+b.Name+"?", []item{{Label: "Delete merged branch", Action: func() {
					a.mutateGit("Deleting branch", func(ctx context.Context, root string) error { return gitservice.DeleteBranch(ctx, root, b.Name) }, a.branchMutationDone)
				}}, {Label: "Cancel"}})
			}})
		}
	}
	if !b.Current && a.git.HasCommits && !a.git.Detached && !a.mergeInProgress {
		items = append(items, item{Label: "Merge into " + a.git.Branch + "…", Detail: b.Name, Action: func() {
			a.branchChoose("Merge "+b.Name+" into "+a.git.Branch+"?", []item{{Label: "Merge branch", Detail: "conflicts remain available to resolve", Action: func() {
				a.changeBranches("Merging "+b.Name, func(ctx context.Context, root string) error { return gitservice.MergeBranch(ctx, root, b.Ref) })
			}}, {Label: "Cancel"}})
		}})
	}
	items = append(items, item{Label: "Branch details", Action: func() {
		a.showText(b.Name, fmt.Sprintf("Ref: %s\nCommit: %s\nSubject: %s\nUpstream: %s\nWorktree: %s", b.Ref, b.Hash, b.Subject, b.Upstream, b.Worktree))
	}}, item{Label: "Back to branches", Action: a.showBranchList})
	a.branchChoose(b.Name, items)
	a.modal.Subtitle = b.Subject
}
func (a *App) createBranchFrom(ref string, track bool) {
	initial := ""
	if track {
		parts := strings.SplitN(strings.TrimPrefix(ref, "refs/remotes/"), "/", 2)
		if len(parts) == 2 {
			initial = parts[1]
		}
	}
	a.compactPrompt("New branch name", initial, "Branch name, for example feature/search", func(name string) {
		a.changeBranches("Creating "+name, func(ctx context.Context, root string) error {
			return gitservice.CreateBranch(ctx, root, name, ref, true, track)
		})
	})
}
func (a *App) branchUpstream(name string) {
	var items []item
	for _, b := range a.branches {
		if b.Remote {
			ref := b.Ref
			items = append(items, item{Label: b.Name, Action: func() {
				a.mutateGit("Setting upstream", func(ctx context.Context, root string) error { return gitservice.SetUpstream(ctx, root, name, ref) }, a.branchMutationDone)
			}})
		}
	}
	if len(items) == 0 {
		a.showText("No remote branches", "Fetch remote branches with git fetch in the terminal, then reopen Branches.")
		return
	}
	a.setModal(&modal{Title: "Upstream for " + name, InputEnabled: true, Filter: true, Items: items, Hint: "Enter selects upstream · Esc cancels"})
}
func (a *App) changeBranches(label string, run func(context.Context, string) error) {
	expected := a.branchBase
	if expected.Root == "" {
		expected = a.git
	}
	a.saveBeforeBranchChange(func() {
		a.mutateGit(label, func(ctx context.Context, root string) error {
			current, err := gitservice.ReadStatus(ctx, root)
			if err != nil {
				return err
			}
			if current.Branch != expected.Branch || current.Head != expected.Head {
				return fmt.Errorf("repository HEAD changed; reopen Branches and try again")
			}
			return run(ctx, root)
		}, a.branchMutationDone)
	})
}
func (a *App) saveBeforeBranchChange(done func()) {
	var dirty []*editor.Document
	for _, d := range a.docs {
		if d.Dirty() {
			dirty = append(dirty, d)
		}
	}
	if len(dirty) == 0 {
		done()
		return
	}
	a.branchChoose("Save buffers before changing branches?", []item{{Label: "Save all and continue", Detail: fmt.Sprintf("%d modified buffer(s)", len(dirty)), Action: func() {
		var save func(int)
		save = func(i int) {
			if i == len(dirty) {
				done()
				return
			}
			a.save(dirty[i], func() { save(i + 1) })
		}
		save(0)
	}}, {Label: "Cancel", Detail: "keep current branch and unsaved edits"}})
}
func (a *App) branchMutationDone() {
	a.gitViewSeq++
	a.comparison = nil
	a.gitLogLoaded = false
	a.checkFiles()
	a.scanTree()
}
func (a *App) abortBranchMerge() {
	a.branchChoose("Abort merge?", []item{{Label: "Abort merge", Detail: "discard this merge's conflict resolution", Action: func() { a.changeBranches("Aborting merge", gitservice.AbortMerge) }}, {Label: "Cancel"}})
}

func (a *App) branchChoose(title string, items []item) {
	a.choose(title, items)
	a.modal.AutoHeight = true
}
