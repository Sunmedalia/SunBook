package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v3"
	"sunbook/internal/config"
	"sunbook/internal/text"
)

func TestSettingsSwitchIconsPersistsAndPreservesEditorState(t *testing.T) {
	a, _ := testApp(t)
	a.cfg.FilePath = filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(a.cfg.FilePath, []byte(`{"tabSize":2,"theme":{"name":"nord"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	a.typeInput("draft 👩‍💻")
	doc, head := a.view().Doc, a.view().Head()
	clickCommand(t, a, "app.settings")
	if a.modal == nil || a.modal.Title != "Settings" {
		t.Fatal("activity gear did not open Settings")
	}
	a.modalAccept()
	if a.modal == nil || a.modal.Title != "File Icon Theme" {
		t.Fatal("Settings did not open file icons")
	}
	a.key(tcell.NewEventKey(tcell.KeyDown, "", 0))
	a.key(tcell.NewEventKey(tcell.KeyEnter, "", 0))
	if a.cfg.FileIcons != "classic" || a.modal != nil {
		t.Fatal("Classic selection did not apply")
	}
	cfg, err := config.Load(a.cfg.FilePath)
	if err != nil || cfg.FileIcons != "classic" || cfg.ActivityIcons != a.cfg.ActivityIcons || cfg.Theme.Name != "nord" || cfg.TabSize != 2 {
		t.Fatal("icon settings did not persist or overwrote theme/editor settings", cfg, err)
	}
	if mark, _ := a.fileIcon("main.py", false, false); mark != "Py" {
		t.Fatal("Classic no longer contains the original Python icon", mark)
	}
	clickCommand(t, a, "app.settings")
	a.modal.Selected = 1
	a.modalAccept()
	a.key(tcell.NewEventKey(tcell.KeyDown, "", 0))
	a.key(tcell.NewEventKey(tcell.KeyEnter, "", 0))
	cfg, err = config.Load(a.cfg.FilePath)
	if err != nil || cfg.FileIcons != "classic" || cfg.ActivityIcons != "unicode" || a.actionIcon("app.settings", "Settings") != "⚙" {
		t.Fatal("action icons cannot be saved independently", cfg, err)
	}
	a.execute("view.fileIcons")
	a.key(tcell.NewEventKey(tcell.KeyUp, "", 0))
	a.modalAccept()
	if a.cfg.FileIcons != "expressive" || a.cfg.ActivityIcons != "unicode" {
		t.Fatal("changing file icons changed button icons")
	}
	if a.view().Doc != doc || a.view().Head() != head || doc.Buffer.String() != "draft 👩‍💻" || !doc.Dirty() {
		t.Fatal("switching icon settings changed the editor")
	}
	a.execute("view.fileIcons")
	a.key(tcell.NewEventKey(tcell.KeyDown, "", 0))
	a.key(tcell.NewEventKey(tcell.KeyEsc, "", 0))
	if a.cfg.FileIcons != "expressive" {
		t.Fatal("cancelling icon picker applied an unconfirmed selection")
	}
}

func TestFileIconThemesKeepOriginalAndRenderDistinctMeasuredMascots(t *testing.T) {
	a, _ := testApp(t)
	cases := []struct{ path, classic, expressive string }{
		{"main.PY", "Py", "🐍"}, {"stub.pyi", "Py", "🐍"}, {"main.rs", "Rs", "🦀"},
		{"app.go", "Go", "🐹"}, {"App.java", "Jav", "☕"}, {"app.rb", "Rb", "💎"},
		{"app.php", "PHP", "🐘"}, {"App.swift", "Sw", "🐦"}, {"Dockerfile", "Dkr", "🐳"},
		{"Cargo.toml", "Tom", "🦀"}, {"pyproject.toml", "Tom", "🐍"},
		{"README.md", "Md", "📖"}, {"package.json", "{}", "📦"}, {"package-lock.json", "{}", "🔒"},
		{"index.ts", "TS", "🔷"}, {"index.tsx", "TSX", "⚛️T"}, {"types.d.ts", "dTS", "d🔷"},
		{"data.sqlite", "DB", "🗃️"}, {"report.csv", "▤", "📊"}, {"unknown.bin", "▤", "📄"},
	}
	for _, tc := range cases {
		for _, theme := range []string{"classic", "expressive"} {
			a.cfg.FileIcons = theme
			mark, _ := a.fileIcon(tc.path, false, false)
			want := tc.classic
			if theme == "expressive" {
				want = tc.expressive
			}
			if mark != want || a.fileIconSpan(tc.path, false, false) != text.Width(mark, 4)+1 || text.Width(mark, 4) > fileIconWidth-1 {
				t.Fatal("icon theme lost a file mapping or overlaps its filename", tc.path, theme, mark, want)
			}
		}
	}
	for _, mark := range expressiveLanguageMarks {
		if text.Width(mark, 4) > fileIconWidth-1 {
			t.Fatal("language icon overflows file slot", mark)
		}
	}
}

func TestIconSettingsSaveFailureStillAppliesForSession(t *testing.T) {
	a, _ := testApp(t)
	a.cfg.FilePath = filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(a.cfg.FilePath, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	a.chooseFileIcons()
	a.modal.Selected = 1
	a.modalAccept()
	if a.cfg.FileIcons != "classic" || !strings.Contains(a.message, "could not save") {
		t.Fatal("save failure lost the chosen icons or did not report persistence failure")
	}
	data, _ := os.ReadFile(a.cfg.FilePath)
	if string(data) != "invalid" {
		t.Fatal("icon picker overwrote an invalid config")
	}
}
