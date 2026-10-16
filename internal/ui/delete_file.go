package ui

import (
	"os"
	"path/filepath"

	"sunbook/internal/editor"
)

func (a *App) deleteExplorerFile() {
	if a.treeSelected < 0 || a.treeSelected >= len(a.tree) {
		a.notify("Select a file in Explorer to delete")
		return
	}
	a.confirmDeleteFile(a.tree[a.treeSelected].Path)
}

func (a *App) confirmDeleteFile(path string) {
	if path == "" {
		a.notify("Untitled files have no file on disk to delete")
		return
	}
	info, err := os.Lstat(path)
	if err != nil {
		a.showError(err)
		return
	}
	if info.IsDir() {
		a.notify("Select a file to delete")
		return
	}
	hint := "Enter deletes · Esc cancels"
	for _, d := range a.docs {
		if d.Path == path && d.Dirty() {
			hint = "Unsaved edits stay open · Enter deletes · Esc cancels"
		}
	}
	a.setModal(&modal{Title: "Delete " + a.directoryLabel(path) + "?", Compact: true, Hint: hint,
		Items: []item{{Label: "Delete from disk", Action: func() {
			current, err := os.Lstat(path)
			if err != nil {
				a.showError(err)
				return
			}
			if !os.SameFile(info, current) {
				a.notify("File was replaced; select it again before deleting")
				return
			}
			if err := os.Remove(path); err != nil {
				a.showError(err)
				return
			}
			kept := false
			for _, d := range append([]*editor.Document(nil), a.docs...) {
				if d.Path != path {
					continue
				}
				if d.Dirty() {
					d.Exists = false
					a.keepDocument(d)
					kept = true
				} else {
					a.closeDocument(d)
				}
			}
			for i := len(a.recentFiles) - 1; i >= 0; i-- {
				if a.recentFiles[i] == path {
					a.recentFiles = append(a.recentFiles[:i], a.recentFiles[i+1:]...)
				}
			}
			a.treePath = ""
			a.fileClick = fileClick{}
			a.scanTree()
			a.refreshGit()
			if a.searchQuery != "" {
				a.startWorkspaceSearch()
			}
			if a.sidebar && a.sidebarView == "Explorer" {
				a.focus = "tree"
			}
			message := "Deleted " + filepath.Base(path)
			if kept {
				message += " · unsaved edits kept open; Save restores the file"
			}
			a.notify(message)
		}}, {Label: "Cancel"}}})
}
