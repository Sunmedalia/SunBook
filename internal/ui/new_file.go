package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sunbook/internal/editor"
)

func (a *App) newFile() {
	dir := a.newFileDirectory()
	const hint = "Enter creates · blank = Untitled · Esc cancels"
	m := &modal{Title: "New File", InputEnabled: true, Compact: true,
		Placeholder: "Filename in " + a.directoryLabel(dir) + " (optional)", Hint: hint}
	m.Changed = func(string) { m.Hint = hint }
	m.Accept = func(bool) {
		path := m.Input
		if strings.TrimSpace(path) == "" {
			a.modal = nil
			a.newUntitled(dir)
			return
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		path = filepath.Clean(path)
		if parent, err := filepath.EvalSymlinks(filepath.Dir(path)); err == nil {
			path = filepath.Join(parent, filepath.Base(path))
		}
		for _, d := range a.docs {
			if d.Path == path {
				m.Hint = "That file is already open; choose another name"
				return
			}
		}
		// Exclusive creation keeps existing files and symlinks intact.
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err == nil {
			err = f.Close()
		}
		if err != nil {
			if os.IsExist(err) {
				m.Hint = "File already exists; choose another name or use Open"
			} else {
				m.Hint = "Could not create file; check the path and permissions"
				a.showError(fmt.Errorf("create file: %w", err))
			}
			return
		}
		if err = a.open(path); err != nil {
			a.showError(err)
			return
		}
		a.modal = nil
		for parent := filepath.Dir(path); parent != a.root && parent != filepath.Dir(parent); parent = filepath.Dir(parent) {
			if rel, err := filepath.Rel(a.root, parent); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				break
			}
			a.expanded[parent] = true
		}
		a.scanTree()
		a.refreshGit()
		a.notify("Created " + filepath.Base(path))
	}
	a.setModal(m)
}

func (a *App) newUntitled(dir string) {
	d := editor.New()
	if a.newFileDirs == nil {
		a.newFileDirs = map[*editor.Document]string{}
	}
	a.newFileDirs[d] = dir
	a.addDocument(d)
}

func (a *App) newFileDirectory() string {
	if a.newFileDir != "" {
		if info, err := os.Stat(a.newFileDir); err == nil && info.IsDir() {
			return a.newFileDir
		}
	}
	return a.root
}

func (a *App) directoryLabel(dir string) string {
	if dir == a.root {
		return "workspace"
	}
	if rel, err := filepath.Rel(a.root, dir); err == nil {
		return filepath.ToSlash(rel)
	}
	return dir
}
