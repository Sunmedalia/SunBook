package ui

import (
	"github.com/gdamore/tcell/v3"
	"strings"
	"testing"
	"time"

	gitservice "sunbook/internal/git"
)

func TestDisabledButtonDoesNotDispatchMouseCommand(t *testing.T) {
	a, _ := testApp(t)
	called := false
	for i := range a.commands {
		if a.commands[i].ID == "edit.undo" {
			a.commands[i].Run = func() { called = true }
		}
	}
	clickCommand(t, a, "edit.undo")
	if called || a.pressedCommand == "edit.undo" {
		t.Fatal("disabled Undo dispatched a command")
	}
	a.typeInput("change")
	clickCommand(t, a, "edit.undo")
	if !called || a.pressedCommand != "edit.undo" || !time.Now().Before(a.pressedUntil) {
		t.Fatal("enabled Undo did not dispatch with press feedback")
	}
}

func TestGitButtonsFollowSelectedVersionAndBusyState(t *testing.T) {
	a, _ := testApp(t)
	a.git = gitservice.Status{Root: a.root, Files: []gitservice.File{{Path: "file.go", Index: "M", Worktree: "M"}}}
	if a.buttonEnabled("git.stage") || !a.buttonEnabled("git.unstage") || !a.buttonEnabled("git.commit") {
		t.Fatal("staged selection has incorrect actions")
	}
	a.gitSelected = 1
	a.gitReading = true
	if !a.buttonEnabled("git.stage") || a.buttonEnabled("git.unstage") {
		t.Fatal("worktree selection or background refresh disabled Stage")
	}
	a.gitBusy = true
	for _, id := range []string{"git.stage", "git.unstage", "git.commit"} {
		if a.buttonEnabled(id) {
			t.Fatal("mutation enabled while Git is busy", id)
		}
	}
}

func TestLongUnicodeTabPreservesCloseTarget(t *testing.T) {
	a, _ := testApp(t)
	doc := a.view().Doc
	a.view().Doc.Path = "/workspace/很长的文件名称👩‍💻需要保留关闭按钮.go"
	a.draw()
	tab := a.tabs[0]
	s, _, _ := a.screen.Get(tab.X+tab.W-2, tab.Y)
	if s != "×" {
		t.Fatal("long filename overwrote the close target", s)
	}
	click(a, tab.X+tab.W-2, tab.Y)
	if a.view().Doc == doc || a.modal != nil {
		t.Fatal("long tab's visible close button did not close the document")
	}
}

func TestModalFrameAndFooterDoNotActivateHiddenCommands(t *testing.T) {
	a, _ := testApp(t)
	a.palette()
	a.draw()
	m := a.modal
	click(a, a.modalRect.X+3, m.ListY+m.ListH)
	click(a, a.modalRect.X, m.ListY)
	if a.modal != m {
		t.Fatal("dialog frame or footer activated a command")
	}
	click(a, a.modalRect.X+3, m.ListY)
	if a.modal != nil {
		t.Fatal("visible command row did not activate")
	}
}

func TestHelpButtonShowsConfiguredShortcutsAndClosesSafely(t *testing.T) {
	a, _ := testApp(t)
	a.typeInput("keep draft")
	a.cfg.Keybindings = map[string]string{"ctrl+s": "", "ctrl+alt+s": "file.save"}
	a.registerCommands()
	a.workspaceSearch()
	clickCommand(t, a, "app.help")
	m := a.modal
	if m == nil || m.Title != "Keyboard Shortcuts" {
		t.Fatal("Help did not open shortcut reference")
	}
	saveLine := ""
	for _, line := range m.Body {
		if strings.HasSuffix(line, "File: Save") {
			saveLine = line
		}
	}
	if !strings.HasPrefix(saveLine, "ctrl+alt+s ") {
		t.Fatal("Help ignored shortcut overrides", saveLine)
	}
	a.draw()
	a.key(tcell.NewEventKey(tcell.KeyPgDn, "", 0))
	if m.Top == 0 {
		t.Fatal("shortcut reference did not scroll")
	}
	a.key(tcell.NewEventKey(tcell.KeyEsc, "", 0))
	if a.modal != nil || a.focus != "searchInput" || a.view().Doc.Buffer.String() != "keep draft" {
		t.Fatal("Help changed focus or document")
	}
}
