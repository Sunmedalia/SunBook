package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/vt"
	"sunbook/internal/config"
	gitservice "sunbook/internal/git"
	"sunbook/internal/plugin"
	"sunbook/internal/text"
)

// Optional snapshots of the actual rendered cell buffer for visual review.
func TestExtensionPreviewArtifact(t *testing.T) {
	dir := os.Getenv("SUNBOOK_PREVIEW_DIR")
	if dir == "" {
		t.Skip("set SUNBOOK_PREVIEW_DIR for cell snapshots")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	a, mt := testApp(t)
	a.root = "/workspace/SunBook"
	a.cfg.ActivityIcons = "nerd"
	a.cfg.FileIcons = "classic"
	a.view().Doc.Path = filepath.Join(a.root, "main.go")
	source := "package main\n\nimport \"fmt\"\n\n// Extensions run independently from the editor.\nfunc main() {\n    fmt.Println(\"Hello, SunBook\")\n}\n" + strings.Repeat("\n", 20)
	a.view().Doc.Buffer = text.New(source)
	a.git = gitservice.Status{Root: a.root, Branch: "feature/plugin-host", HasCommits: true}
	a.pluginStatuses = []plugin.Status{
		{Entry: plugin.Entry{Manifest: plugin.Manifest{ID: "text-tools", Name: "Text Tools", Version: "1.0.0"}}, State: plugin.Ready},
		{Entry: plugin.Entry{Manifest: plugin.Manifest{ID: "formatter", Name: "Project Formatter", Version: "1.2.0"}}, State: plugin.Restarting},
		{Entry: plugin.Entry{Manifest: plugin.Manifest{ID: "legacy", Name: "Legacy Checker", Version: "0.8.0"}}, State: plugin.Quarantined},
	}
	type cell struct {
		X, Y         int
		Text, FG, BG string
		Bold         bool
	}
	save := func(name string) {
		t.Helper()
		a.draw()
		var cells []cell
		w, h := a.screen.Size()
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				s, style, _ := a.screen.Get(x, y)
				fg, bg := style.GetForeground(), style.GetBackground()
				if !fg.Valid() {
					fg = a.base.GetForeground()
				}
				if !bg.Valid() {
					bg = a.base.GetBackground()
				}
				fr, fgreen, fb := fg.RGB()
				br, bgreen, bb := bg.RGB()
				cells = append(cells, cell{x, y, s, fmt.Sprintf("#%02x%02x%02x", fr, fgreen, fb), fmt.Sprintf("#%02x%02x%02x", br, bgreen, bb), style.GetAttributes()&tcell.AttrBold != 0})
			}
		}
		raw, err := json.Marshal(cells)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, name+".json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	a.sidebarView = "Plugins"
	a.focus = "plugins"
	a.pluginSelected = 1
	a.highlight[a.view().Doc] = highlights{Version: a.view().Doc.Buffer.Version, Spans: syntaxSpans(a.view().Doc.Path, source, a.base, a.themeSyntax)}
	save("plugins-dark")
	light, _ := config.FindTheme("github-light")
	a.applyTheme(light.Theme)
	save("plugins-light")
	a.branches = []gitservice.Branch{{Name: "feature/plugin-host", Ref: "refs/heads/feature/plugin-host", Hash: "37f9bc1", Current: true}, {Name: "main", Ref: "refs/heads/main", Hash: "124ab99"}, {Name: "origin/main", Ref: "refs/remotes/origin/main", Hash: "124ab99", Remote: true}}
	a.sidebarView = "Git"
	a.focus = "git"
	a.showBranchList()
	save("branches")
	status := plugin.Status{Entry: plugin.Entry{Manifest: plugin.Manifest{ID: "demo", Name: "Network Tools", Description: "Analyze the active document using a remote service.", Permissions: []string{plugin.ReadDocument, plugin.Network}}}}
	a.reviewPlugin(status)
	save("plugin-review-light")
	dark, _ := config.FindTheme("sunbook")
	a.applyTheme(dark.Theme)
	a.reviewPlugin(status)
	save("plugin-review-dark")
	mt.SetSize(vt.Coord{X: 45, Y: 12})
	a.screen.Sync()
	a.reviewPlugin(status)
	save("plugin-review-narrow")
}
