package ui

// Keep action symbols in one place so toolbar, sidebar, panel, and palette use
// the same visual vocabulary. Unicode mode also applies to these controls.
func (a *App) actionIcon(id, label string) string {
	nerd, plain := "", ""
	switch id {
	case "file.new", "file.newUntitled":
		nerd, plain = "\uf016", "+"
	case "file.open", "file.quickOpen", "git.open":
		nerd, plain = "\uf07c", "/"
	case "file.save", "file.saveAs", "file.saveAll":
		nerd, plain = "\uf0c7", "↓"
	case "search.find", "search.workspace":
		nerd, plain = "\uf002", "/"
	case "edit.undo":
		nerd, plain = "\uf0e2", "<"
	case "edit.redo":
		nerd, plain = "\uf01e", ">"
	case "view.split":
		nerd, plain = "\uf0db", "||"
	case "view.terminal", "view.showTerminal":
		nerd, plain = "\uf120", ">_"
	case "app.commands", "git.more":
		nerd, plain = "\uf141", "…"
	case "app.help":
		nerd, plain = "\uf059", "?"
	case "app.settings", "view.fileIcons", "view.actionIcons":
		nerd, plain = "\uf013", "⚙"
	case "file.rename", "language.rename":
		nerd, plain = "\uf040", "✎"
	case "view.theme":
		nerd, plain = "\uf1fc", "◐"
	case "view.problems", "search.error":
		nerd, plain = "\uf071", "!"
	case "view.hidePanel", "explorer.clearFilter", "search.clear":
		nerd, plain = "\uf00d", "×"
	case "terminal.kill":
		nerd, plain = "\uf1f8", "DEL"
	case "file.delete", "explorer.delete":
		nerd, plain = "\uf1f8", "⌫"
	case "terminal.new":
		nerd, plain = "+", "+"
	case "view.panelDock":
		nerd, plain = "\uf061", "→"
		if label == "Bottom" {
			nerd, plain = "\uf063", "↓"
		}
	case "plugins.manage", "plugins.install", "activity.plugins":
		nerd, plain = "\uf12e", "◇"
	case "git.branches":
		nerd, plain = "\uf126", "⎇"
	case "plugins.refresh", "git.refresh", "explorer.refresh", "file.reload":
		nerd, plain = "\uf021", "R"
	case "git.commit":
		nerd, plain = "\uf00c", "✓"
	case "git.history", "git.fileHistory":
		nerd, plain = "\uf1da", "<"
	case "git.stage", "git.stageAll", "git.stageCHANGES", "git.stageUNTRACKED", "git.stageCONFLICTS":
		nerd, plain = "\uf067", "+"
	case "git.unstage", "git.unstageAll":
		nerd, plain = "\uf068", "−"
	case "git.selectedDiff", "git.diff", "git.stagedDiff":
		nerd, plain = "\uf0ec", "="
	case "git.toggleLog":
		nerd, plain = "\uf077", "⌃"
		if label == "v" {
			nerd, plain = "\uf078", "⌄"
		}
	case "search.run":
		nerd, plain = "\uf04b", "▶"
	case "search.stop":
		nerd, plain = "\uf04d", "■"
	}
	if a.cfg.ActivityIcons == "unicode" {
		return plain
	}
	return nerd
}
func (a *App) actionLabel(id, label string, compact bool) string {
	if a.cfg.ActivityIcons == "unicode" && !compact {
		return label
	}
	icon := a.actionIcon(id, label)
	if icon == "" {
		return label
	}
	if compact {
		return icon
	}
	return icon + " " + label
}
