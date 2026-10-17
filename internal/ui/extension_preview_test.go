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
	mt.SetSize(vt.Coord{X: 110, Y: 32})
	a.screen.Sync()
	a.sidebar = false
	source = "#   Markdown 工作笔记  ###\n\n##   本周计划\n+ [X] 实现 Markdown 格式化\n+ [ ] 验证分屏预览\n\n### 使用说明\n原文 **保留**，右侧阅读预览支持 `代码`、列表和引用。\n\n> 确认后再应用，Ctrl+Z 一次撤销。\n\n```go\nfmt.Println(\"Hello, SunBook\")\n```\n\n| 功能 | 状态 |\n| --- | --- |\n| 格式化 | 完成 |\n| 分屏 | 完成 |\n"
	formatted := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(source, "#   ", "# "), "  ###", ""), "+ [", "- [")
	d := a.view().Doc
	d.Buffer = text.New(source)
	d.Path = "notes.md"
	out := &pluginOutcome{name: "Markdown Tools", path: d.Path, doc: d, buffer: d.Buffer, version: d.Buffer.Version, result: plugin.Result{Preview: &plugin.Preview{Kind: "markdown", Text: formatted}}, edits: []text.Edit{{Start: 0, End: len(source), Text: formatted}}}
	a.modal = nil
	a.openPluginSplit(out)
	a.draw()
	waitUpdates(t, a, func() bool { return !a.pluginPreview().Rendering })
	save("markdown-split-dark")
	a.applyTheme(light.Theme)
	save("markdown-split-light")
	a.togglePluginSplitMode()
	save("markdown-split-source")
	a.togglePluginSplitMode()
	mt.SetSize(vt.Coord{X: 60, Y: 20})
	a.screen.Sync()
	a.draw()
	waitUpdates(t, a, func() bool { return !a.pluginPreview().Rendering })
	save("markdown-split-narrow")

	// Review the actual README table that previously truncated long descriptions.
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(readme), "| 功能 | 使用方式 |")
	if start < 0 {
		t.Fatal("README feature table missing")
	}
	table := string(readme)[start:]
	if end := strings.Index(table, "\n\n"); end >= 0 {
		table = table[:end]
	}
	d.Buffer = text.New("## 功能\n\n" + table + "\n")
	d.Path = "README.md"
	a.applyTheme(dark.Theme)
	mt.SetSize(vt.Coord{X: 150, Y: 48})
	a.screen.Sync()
	p := a.pluginPreview()
	p.Top = 0
	p.Due = zeroTime
	renderLivePreview(t, a)
	save("markdown-table-wrap")

	a.closePluginSplit()
	a.sidebar = true
	a.sidebarView = "Git"
	a.sidebarWidth = 38
	a.git = gitservice.Status{Root: a.root, Branch: "feature/source-control", HasCommits: true, Files: []gitservice.File{
		{Path: "internal/ui/git_sidebar.go", Index: "M", Worktree: " "},
		{Path: "README.md", Index: " ", Worktree: "M"},
		{Path: "internal/ui/git_input.go", Index: " ", Worktree: "M"},
		{Path: "docs/usage.md", Index: " ", Worktree: "M"},
		{Path: "internal/ui/git_commit_ui.go", Index: "?", Worktree: "?"},
		{Path: "notes/release.md", Index: "?", Worktree: "?"},
	}}
	a.gitError = ""
	a.message = ""
	a.gitActionError = ""
	a.gitLog = []gitservice.Revision{{Hash: "a1b2c3d4", Author: "SunBook", Age: "2 hours ago", Subject: "Improve Markdown live preview"}}
	a.gitLogLoaded = true
	a.gitMessage = "feat: refine source control\n\nAdd collapsible change groups."
	a.gitMessageCursor = 0
	a.focus = "gitMessage"
	mt.SetSize(vt.Coord{X: 130, Y: 36})
	a.screen.Sync()
	save("git-commit-dark")
	a.toggleGitGroup("CHANGES")
	a.toggleGitGroup("UNTRACKED")
	save("git-groups-collapsed")
	a.applyTheme(light.Theme)
	save("git-commit-light")
	mt.SetSize(vt.Coord{X: 60, Y: 14})
	a.screen.Sync()
	save("git-commit-compact")
}
