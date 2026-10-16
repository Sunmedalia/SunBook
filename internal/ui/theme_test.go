package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2/styles"
	"github.com/gdamore/tcell/v3"
	"sunbook/internal/config"
	gitservice "sunbook/internal/git"
)

func TestThemePickerFiltersAppliesAndPersists(t *testing.T) {
	a, _ := testApp(t)
	a.cfg.FilePath = filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(a.cfg.FilePath, []byte(`{"tabSize":2}`), 0600)
	a.typeInput("unsaved draft 👩‍💻")
	doc, head := a.view().Doc, a.view().Head()
	a.key(tcell.NewEventKey(tcell.KeyCtrlK, "", 0))
	a.key(tcell.NewEventKey(tcell.KeyCtrlT, "", 0))
	if a.modal == nil || a.modal.Title != "Color Theme" || len(a.modal.Visible) != len(config.ThemePresets()) {
		t.Fatal("theme shortcut did not open picker")
	}
	a.key(tcell.NewEventKey(tcell.KeyEsc, "", 0))
	if a.cfg.Theme != config.Default().Theme {
		t.Fatal("cancelling changed theme")
	}
	a.palette()
	a.paste("Color Theme")
	a.key(tcell.NewEventKey(tcell.KeyEnter, "", 0))
	if a.modal == nil || a.modal.Title != "Color Theme" {
		t.Fatal("theme command missing from palette")
	}
	a.paste("GitHub Light")
	if len(a.modal.Visible) != 1 || a.modal.Visible[0].Swatch[0] != "#ffffff" {
		t.Fatal("theme filter or palette swatch is wrong")
	}
	a.draw()
	click(a, a.modalRect.X+20, a.modal.ListY)
	saved, err := config.Load(a.cfg.FilePath)
	if a.modal != nil || a.cfg.Theme.Name != "github-light" || err != nil || saved.Theme != a.cfg.Theme || saved.TabSize != 2 {
		t.Fatal("theme click did not apply and persist", err)
	}
	if a.view().Doc != doc || doc.Buffer.String() != "unsaved draft 👩‍💻" || a.view().Head() != head || !doc.Dirty() {
		t.Fatal("theme selection changed the editor")
	}
}

func TestAutoThemeFollowsTerminalLightAndDarkColors(t *testing.T) {
	t.Setenv("COLORFGBG", "0;15")
	a, _ := testApp(t)
	auto, _ := config.FindTheme("auto")
	a.applyTheme(auto.Theme)
	if a.base.GetBackground() != tcell.ColorWhite.TrueColor() || a.base.GetForeground() != tcell.ColorBlack.TrueColor() || a.themeSyntax != "github" {
		t.Fatal("Auto did not adopt light terminal colors")
	}
	doc := a.view().Doc
	doc.Path = filepath.Join(a.root, "auto.go")
	a.typeInput("package main\n")
	a.runHighlight(doc)
	oldHighlight := nextThemeUpdate(t, a)
	t.Setenv("COLORFGBG", "15;0")
	a.refreshTerminalTheme()
	oldHighlight()
	if a.base.GetBackground() != tcell.ColorBlack.TrueColor() || a.base.GetForeground() != tcell.ColorWhite.TrueColor() || a.themeSyntax != "github-dark" || len(a.highlight) != 0 {
		t.Fatal("Auto did not follow a changed terminal palette or kept stale highlights")
	}
	t.Setenv("COLORFGBG", "")
	a.refreshTerminalTheme()
	if a.base.GetBackground() != tcell.ColorDefault || a.base.GetForeground() != tcell.ColorDefault {
		t.Fatal("unsupported detection must fall back to terminal defaults")
	}
}

func TestTerminalDefaultUsesNoPaintedBackgroundsAndKeepsShellColors(t *testing.T) {
	a, _ := testApp(t)
	preset, _ := config.FindTheme("terminal-default")
	a.applyTheme(preset.Theme)
	insertSource(a, "package main\nvar name = \"hello\"\n")
	a.view().Doc.Path = filepath.Join(a.root, "main.go")
	a.highlight[a.view().Doc] = highlights{Version: a.view().Doc.Buffer.Version,
		Spans: syntaxSpans("main.go", a.view().Doc.Buffer.String(), a.base, a.themeSyntax)}
	for _, panel := range []string{"Explorer", "Search", "Git"} {
		a.sidebarView = panel
		a.draw()
		assertTerminalBackgrounds(t, a)
	}
	a.help()
	a.draw()
	assertTerminalBackgrounds(t, a)
	a.modal = nil
	selected := a.workbenchStyle(a.selected)
	if selected.GetAttributes()&tcell.AttrReverse == 0 {
		t.Fatal("terminal-default selection needs a visible terminal reverse-video marker")
	}
	a.cfg.Shell = "/bin/sh"
	a.showTerminal()
	a.draw() // Resize before inserting a known test cell.
	a.term.Feed([]byte("\x1b[2J\x1b[H\x1b[31;44mX\x1b[0m"))
	a.draw()
	cell, style, _ := a.screen.Get(a.panelRect.X, a.panelRect.Y)
	if cell != "X" || style.GetBackground() == tcell.ColorDefault || style.GetForeground() == tcell.ColorDefault {
		t.Fatal("default theme discarded a child terminal's explicit ANSI colors")
	}
}

func assertTerminalBackgrounds(t *testing.T, a *App) {
	t.Helper()
	w, h := a.screen.Size()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			_, style, _ := a.screen.Get(x, y)
			if style.GetBackground() != tcell.ColorDefault || style.GetForeground().IsRGB() {
				t.Fatalf("terminal palette was replaced at %d,%d: %+v", x, y, style)
			}
		}
	}
}

func TestAllThemesRenderWithMatchingSyntaxAndReadableSurfaces(t *testing.T) {
	a, _ := testApp(t)
	doc := a.view().Doc
	doc.Path = filepath.Join(a.root, "main.go")
	a.typeInput("package main\nvar message = \"hello\"\n// comment\n")
	for _, preset := range config.ThemePresets() {
		t.Run(preset.Theme.Name, func(t *testing.T) {
			if _, ok := styles.Registry[preset.Theme.Syntax]; !ok {
				t.Fatal("preset has no syntax theme", preset.Theme.Syntax)
			}
			a.applyTheme(preset.Theme)
			a.highlightDue[doc] = time.Time{}
			a.flushHighlights()
			waitUpdates(t, a, func() bool { return a.highlight[doc].Version == doc.Buffer.Version })
			for _, span := range a.highlight[doc].Spans {
				if span.Style.GetBackground() != a.base.GetBackground() {
					t.Fatal("highlight retained the previous background")
				}
			}
			for _, panel := range []string{"Explorer", "Search", "Git"} {
				a.sidebarView = panel
				a.draw()
				_, style, _ := a.screen.Get(0, 2)
				if (style.GetBackground() == tcell.ColorDefault) != a.usesTerminalPalette() {
					t.Fatal("surface lost configured theme", panel)
				}
			}
			if preset.Theme.Name == "github-light" {
				if !lightBackground(a.base.GetBackground()) || a.colors.success == tcell.GetColor("#9BC99A") || a.colors.inset == mix(a.base.GetBackground(), tcell.ColorBlack, 18) {
					t.Fatal("light theme reused low-contrast dark surfaces")
				}
			}
		})
	}
}

func nextThemeUpdate(t *testing.T, a *App) func() {
	t.Helper()
	select {
	case update := <-a.updates:
		return update
	case <-time.After(4 * time.Second):
		t.Fatal("highlight did not finish")
		return nil
	}
}

func TestThemeSwitchRejectsOldEditorAndComparisonHighlights(t *testing.T) {
	a, _ := testApp(t)
	doc := a.view().Doc
	doc.Path = filepath.Join(a.root, "main.go")
	a.typeInput("package main\n")
	a.runHighlight(doc)
	oldEditor := nextThemeUpdate(t, a)
	light, _ := config.FindTheme("github-light")
	a.applyTheme(light.Theme)
	oldEditor()
	if len(a.highlight) != 0 {
		t.Fatal("old editor worker overwrote the new theme")
	}
	a.runHighlight(doc)
	nextThemeUpdate(t, a)()
	if a.highlight[doc].Spans[0].Style.GetBackground() != a.base.GetBackground() {
		t.Fatal("new editor highlight has old theme")
	}
	c := newGitComparison(gitservice.Comparison{Path: "main.go", Old: "package old\n", New: "package new\n"})
	a.comparison = c
	a.rehighlightComparison(c)
	oldComparison := nextThemeUpdate(t, a)
	dark, _ := config.FindTheme("dracula")
	a.applyTheme(dark.Theme)
	oldComparison()
	if len(c.Highlights[0]) != 0 {
		t.Fatal("old comparison worker overwrote new theme")
	}
	waitUpdates(t, a, func() bool { return len(c.Highlights[0]) > 0 && len(c.Highlights[1]) > 0 })
	for _, spans := range c.Highlights {
		if spans[0].Style.GetBackground() != a.base.GetBackground() {
			t.Fatal("comparison kept old syntax colors")
		}
	}
}

func TestThemeSaveFailureKeepsSessionTheme(t *testing.T) {
	a, _ := testApp(t)
	a.cfg.FilePath = filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(a.cfg.FilePath, []byte("invalid"), 0600)
	a.chooseTheme()
	a.paste("Nord")
	a.modalAccept()
	if a.cfg.Theme.Name != "nord" || !strings.Contains(a.message, "could not save") {
		t.Fatal("failed persistence lost applied theme or did not explain failure")
	}
}
