package ui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/vt"
	"sunbook/internal/config"
	gitservice "sunbook/internal/git"
	"sunbook/internal/plugin"
	"sunbook/internal/text"
)

func waitPluginUI(t *testing.T, a *App, ready func() bool) {
	t.Helper()
	deadline := time.After(8 * time.Second)
	for !ready() {
		select {
		case f := <-a.updates:
			f()
		case <-a.plugins.Changes:
			a.updatePluginCommands()
		case <-deadline:
			t.Fatal("plugin UI did not settle", a.pluginStatuses, a.message)
		}
	}
}
func examplePluginApp(t *testing.T, ids ...string) *App {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("Python example requires python3")
	}
	a, _ := testApp(t)
	a.plugins.Close()
	a.cfg.FilePath = filepath.Join(t.TempDir(), "config.json")
	a.plugins = plugin.NewManager(a.ctx, a.pluginDirectory())
	id := "text-tools"
	if len(ids) > 0 {
		id = ids[0]
	}
	source, err := filepath.Abs(filepath.Join("../../examples", id))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.plugins.Install(source); err != nil {
		t.Fatal(err)
	}
	a.refreshPlugins()
	waitPluginUI(t, a, func() bool { return !a.pluginRefreshing && len(a.pluginStatuses) == 1 })
	return a
}

func TestMarkdownPluginInstallCommandsPreviewAndUndo(t *testing.T) {
	a := examplePluginApp(t, "markdown-tools")
	status := a.pluginStatuses[0]
	if status.State != plugin.Disabled || status.Entry.Manifest.Allows(plugin.Network) || len(status.Entry.Manifest.Commands) != 4 {
		t.Fatal("unexpected Markdown plugin permissions or initial state", status)
	}
	a.showSidebar("Plugins")
	a.pluginDetails()
	a.modalAccept() // Open the permission review.
	if a.modal == nil || !strings.Contains(strings.Join(a.modal.Body, " "), "document.edit") {
		t.Fatal("Markdown permissions are not reviewed")
	}
	a.modalAccept()
	waitPluginUI(t, a, func() bool { return !a.pluginRefreshing && a.pluginStatuses[0].State == plugin.Ready })
	source := "# 计划 👩‍💻\n\n## 准备\n\n- [ ] 编写\n- [x] 测试\n\n### 发布\n"
	a.view().Doc.Path = filepath.Join(a.root, "notes.md")
	a.view().Doc.Buffer = text.New(source)
	a.view().Selections = []text.Selection{{}}
	a.execute("plugin.markdown-tools/update-toc")
	waitPluginUI(t, a, func() bool { return a.modal != nil && strings.HasPrefix(a.modal.Title, "Apply Markdown Tools") })
	if a.view().Doc.Buffer.String() != source {
		t.Fatal("contents changed before accepting preview")
	}
	a.modalAccept()
	updated := a.view().Doc.Buffer.String()
	if !strings.Contains(updated, "- [准备](#准备)") || !strings.Contains(updated, "  - [发布](#发布)") {
		t.Fatal("Unicode table of contents was not inserted", updated)
	}
	a.execute("edit.undo")
	if a.view().Doc.Buffer.String() != source {
		t.Fatal("contents did not undo in one step")
	}
	a.execute("plugin.markdown-tools/task-summary")
	waitPluginUI(t, a, func() bool { return a.modal != nil && a.modal.Title == "Markdown task progress" })
	if !strings.Contains(strings.Join(a.modal.Body, "\n"), "Completed: 1/2 (50%)") || a.view().Doc.Buffer.String() != source {
		t.Fatal("task report changed document or returned incorrect counts")
	}
	a.modal = nil
	a.view().Selections = []text.Selection{{Anchor: len(source), Head: 0}}
	a.execute("plugin.markdown-tools/toggle-tasks")
	waitPluginUI(t, a, func() bool { return a.modal != nil && strings.HasPrefix(a.modal.Title, "Apply Markdown Tools") })
	if a.view().Doc.Buffer.String() != source {
		t.Fatal("tasks changed before preview acceptance")
	}
	a.modalAccept()
	if got := a.view().Doc.Buffer.String(); !strings.Contains(got, "[x] 编写") || !strings.Contains(got, "[ ] 测试") {
		t.Fatal("Unicode task offsets were not applied", got)
	}
	a.execute("edit.undo")
	if a.view().Doc.Buffer.String() != source {
		t.Fatal("task edits did not undo in one step")
	}
	if err := a.plugins.Restart("markdown-tools"); err != nil {
		t.Fatal(err)
	}
	a.updatePluginCommands()
	waitPluginUI(t, a, func() bool { return a.pluginStatuses[0].State == plugin.Ready })
	a.setPluginEnabled(a.pluginStatuses[0], false)
	waitPluginUI(t, a, func() bool { return !a.pluginRefreshing && a.pluginStatuses[0].State == plugin.Disabled })
}
func TestPluginInstallReviewCommandPreviewUndoAndDisable(t *testing.T) {
	a := examplePluginApp(t)
	a.showSidebar("Plugins")
	a.draw()
	a.pluginDetails()
	if a.modal == nil || a.modal.Items[0].Label != "Enable plugin…" {
		t.Fatal("installed plugin wasn't disabled")
	}
	a.modalAccept()
	if a.modal == nil || !strings.Contains(strings.Join(a.modal.Body, " "), "document.edit") {
		t.Fatal("permissions missing")
	}
	a.modalAccept()
	waitPluginUI(t, a, func() bool {
		return len(a.pluginStatuses) == 1 && a.pluginStatuses[0].State == plugin.Ready && !a.pluginRefreshing
	})
	source := "中 hello 👩‍💻"
	a.view().Doc.Buffer = text.New(source)
	a.view().Selections = []text.Selection{{Anchor: len("中 "), Head: len("中 hello")}}
	a.execute("plugin.text-tools/uppercase-selection")
	waitPluginUI(t, a, func() bool { return a.modal != nil && strings.HasPrefix(a.modal.Title, "Apply Text Tools") })
	if a.view().Doc.Buffer.String() != source {
		t.Fatal("plugin edited before preview acceptance")
	}
	a.modalAccept()
	if a.view().Doc.Buffer.String() != "中 HELLO 👩‍💻" {
		t.Fatal("plugin edit not applied")
	}
	a.execute("edit.undo")
	if a.view().Doc.Buffer.String() != source {
		t.Fatal("plugin undo changed granularity")
	}
	a.setPluginEnabled(a.pluginStatuses[0], false)
	waitPluginUI(t, a, func() bool { return !a.pluginRefreshing && a.pluginStatuses[0].State == plugin.Disabled })
	for _, c := range a.commands {
		if strings.HasPrefix(c.ID, "plugin.text-tools/") {
			t.Fatal("disabled command still contributed")
		}
	}
	data, err := os.ReadFile(a.cfg.FilePath)
	if err != nil || !strings.Contains(string(data), `"enabled": false`) {
		t.Fatal("disable wasn't persisted", err)
	}
}

func TestPluginReviewShowsIsolationAndScrollsAllPermissions(t *testing.T) {
	a, mt := testApp(t)
	status := plugin.Status{Entry: plugin.Entry{Manifest: plugin.Manifest{ID: "demo", Name: "Network Tools", Version: "1.0.0", Permissions: []string{plugin.ReadDocument, plugin.EditDocument, plugin.Network}, Description: "Review this extension before enabling it."}}}
	screenText := func() string {
		var result strings.Builder
		w, h := a.screen.Size()
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				c, _, _ := a.screen.Get(x, y)
				result.WriteString(c)
			}
			result.WriteByte('\n')
		}
		return result.String()
	}
	for _, theme := range []string{"sunbook", "github-light"} {
		if preset, ok := config.FindTheme(theme); ok {
			a.applyTheme(preset.Theme)
		}
		for _, size := range []vt.Coord{{X: 110, Y: 32}, {X: 60, Y: 20}, {X: 45, Y: 12}} {
			mt.SetSize(size)
			a.screen.Sync()
			a.reviewPlugin(status)
			a.draw()
			if !strings.Contains(screenText(), plugin.IsolationDescription(status.Entry.Manifest)) {
				t.Fatal("review hides platform/network protection", theme, size, screenText())
			}
			if a.modal.BodyH < len(a.modal.Body) {
				for range len(a.modal.Body) {
					a.modalKey(tcell.NewEventKey(tcell.KeyDown, "", tcell.ModCtrl), "ctrl+down")
					a.draw()
				}
				if a.modal.BodyTop+a.modal.BodyH != len(a.modal.Body) {
					t.Fatal("short review cannot scroll to its final disclosure", size, a.modal)
				}
			}
			if size.X == 45 {
				// Long permission lines must pan just like text-only modals.
				a.modal.BodyTop = 1
				a.modalKey(tcell.NewEventKey(tcell.KeyRight, "", 0), "right")
				a.modalKey(tcell.NewEventKey(tcell.KeyRight, "", 0), "right")
				a.draw()
				if !strings.Contains(screenText(), "document.edit, network") {
					t.Fatal("permission line cannot be panned", screenText())
				}
			}
		}
	}
}
func TestPluginStaleResultAndGraphemeValidation(t *testing.T) {
	a := examplePluginApp(t)
	a.setPluginEnabled(a.pluginStatuses[0], true)
	waitPluginUI(t, a, func() bool { return a.pluginStatuses[0].State == plugin.Ready && !a.pluginRefreshing })
	a.view().Doc.Buffer = text.New("hello")
	a.view().Selections = []text.Selection{{Anchor: 0, Head: 5}}
	a.runPluginCommand("text-tools", "uppercase-selection")
	a.typeInput("new")
	waitPluginUI(t, a, func() bool { return strings.Contains(a.message, "ignored") })
	if a.modal != nil || a.view().Doc.Buffer.String() != "new" {
		t.Fatal("stale plugin edited document")
	}
	for _, edits := range [][]plugin.Edit{{{Start: 0, End: 1, Text: "x"}}, {{Start: len("中"), End: len("中👩"), Text: "x"}}, {{Start: 0, End: 3, Text: "x"}, {Start: 1, End: 3, Text: "y"}}} {
		if _, err := validatePluginEdits("中👩‍💻", edits); err == nil {
			t.Fatal("invalid plugin edit accepted", edits)
		}
	}
}
func TestBranchesUIAndUnsavedBufferGuard(t *testing.T) {
	a := gitTestWorkspace(t)
	ctx := context.Background()
	path := filepath.Join(a.root, "file.txt")
	os.WriteFile(path, []byte("base"), 0600)
	gitservice.StageAll(ctx, a.root)
	if err := gitservice.Commit(ctx, a.root, "base"); err != nil {
		t.Fatal(err)
	}
	if err := a.open(path); err != nil {
		t.Fatal(err)
	}
	a.branchManager()
	waitUpdates(t, a, func() bool { return a.modal != nil && a.modal.Filter })
	a.modalAccept()
	a.paste("feature/ui")
	a.modalAccept()
	waitUpdates(t, a, func() bool { return !a.gitBusy && !a.gitReading })
	status, err := gitservice.ReadStatus(ctx, a.root)
	if err != nil || status.Branch != "feature/ui" {
		t.Fatal(status, err)
	}
	a.branchManager()
	waitUpdates(t, a, func() bool { return a.modal != nil && a.modal.Filter })
	a.modal = nil
	a.typeInput("draft")
	a.changeBranches("Switching to main", func(ctx context.Context, root string) error { return gitservice.SwitchBranch(ctx, root, "main") })
	if a.modal == nil || !strings.Contains(a.modal.Title, "Save buffers") {
		t.Fatal("dirty buffers weren't guarded")
	}
	a.modal.Selected = 1
	a.modalAccept()
	status, _ = gitservice.ReadStatus(ctx, a.root)
	if status.Branch != "feature/ui" || !a.view().Doc.Dirty() {
		t.Fatal("cancel changed branch or discarded edits")
	}
	a.changeBranches("Switching to main", func(ctx context.Context, root string) error { return gitservice.SwitchBranch(ctx, root, "main") })
	a.modalAccept()
	waitUpdates(t, a, func() bool { return !a.gitBusy && !a.gitReading })
	status, _ = gitservice.ReadStatus(ctx, a.root)
	if status.Branch != "main" || a.view().Doc.Dirty() {
		t.Fatal("save and switch failed", status)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "draft") {
		t.Fatal("switch discarded saved changes")
	}
}
func TestExtensionControlsFitNarrowAndLightLayouts(t *testing.T) {
	a, mt := testApp(t)
	a.pluginStatuses = []plugin.Status{{Entry: plugin.Entry{Manifest: plugin.Manifest{ID: "demo", Name: "Example formatter", Version: "1.0.0"}}, State: plugin.Ready}, {Entry: plugin.Entry{Manifest: plugin.Manifest{ID: "broken", Name: "Broken plugin", Version: "1.0.0"}}, State: plugin.Quarantined, Error: "failed"}}
	for _, size := range []vt.Coord{{X: 110, Y: 32}, {X: 60, Y: 20}, {X: 45, Y: 12}, {X: 24, Y: 7}} {
		mt.SetSize(size)
		a.screen.Sync()
		a.sidebarView = "Plugins"
		a.focus = "plugins"
		a.draw()
		for i, b := range a.buttons {
			if b.X < 0 || b.Y < 0 || b.X+b.W > int(size.X) || b.Y >= int(size.Y) {
				t.Fatal("extension action outside screen", size, b)
			}
			for _, other := range a.buttons[:i] {
				if b.Y == other.Y && b.X < other.X+other.W && b.X+b.W > other.X {
					t.Fatal("overlapping actions", size, b, other)
				}
			}
		}
	}
	mt.SetSize(vt.Coord{X: 110, Y: 32})
	a.screen.Sync()
	a.sidebarView = "Plugins"
	a.focus = "plugins"
	a.draw()
	a.key(tcell.NewEventKey(tcell.KeyDown, "", 0))
	a.key(tcell.NewEventKey(tcell.KeyEnter, "", 0))
	if a.modal == nil || a.modal.Title != "Broken plugin" {
		t.Fatal("plugin keyboard navigation unavailable")
	}
}

func TestPluginEditsNormalizeNewlinesAndRejectBinary(t *testing.T) {
	edits, err := validatePluginEdits("old", []plugin.Edit{{Start: 0, End: 3, Text: "a\r\nb\rc"}})
	if err != nil || edits[0].Text != "a\nb\nc" {
		t.Fatal(edits, err)
	}
	if _, err := validatePluginEdits("old", []plugin.Edit{{Start: 0, End: 3, Text: "a\x00b"}}); err == nil {
		t.Fatal("binary edit accepted")
	}
}

func TestPluginResultsDoNotReplaceAnOpenDialog(t *testing.T) {
	a := examplePluginApp(t)
	a.setPluginEnabled(a.pluginStatuses[0], true)
	waitPluginUI(t, a, func() bool { return a.pluginStatuses[0].State == plugin.Ready && !a.pluginRefreshing })
	a.runPluginCommand("text-tools", "word-count")
	a.settings()
	settings := a.modal
	waitPluginUI(t, a, func() bool { return a.lastPluginResult != nil })
	if a.modal != settings {
		t.Fatal("plugin stole an active dialog")
	}
	a.modal = nil
	a.showLastPluginResult()
	if a.modal == nil || a.modal.Title != "Document summary" {
		t.Fatal("deferred result wasn't retained")
	}
}
func TestPluginCommandsRequireEditableDocument(t *testing.T) {
	a, _ := testApp(t)
	a.comparison = &gitComparison{Active: true}
	a.runPluginCommand("anything", "run")
	if !strings.Contains(a.message, "editable document") {
		t.Fatal("plugin read a hidden document behind a comparison")
	}
}
func TestPluginRefreshRetainsSelectionAndDoesNotPaintUnchangedState(t *testing.T) {
	a := examplePluginApp(t)
	a.pluginSelected = 0
	a.pluginSelectID = "text-tools"
	a.repaint = false
	a.updatePluginCommands()
	if a.pluginSelectID != "" || a.pluginSelected != 0 {
		t.Fatal("pending selection was not applied")
	}
	if a.repaint {
		t.Fatal("unchanged health poll requested a frame")
	}
}

func TestBranchOperationRejectsChangedHEAD(t *testing.T) {
	a := gitTestWorkspace(t)
	ctx := context.Background()
	os.WriteFile(filepath.Join(a.root, "base"), []byte("base"), 0600)
	gitservice.StageAll(ctx, a.root)
	if err := gitservice.Commit(ctx, a.root, "base"); err != nil {
		t.Fatal(err)
	}
	a.branchManager()
	waitUpdates(t, a, func() bool { return a.modal != nil && a.modal.Filter })
	a.modal = nil
	if err := gitservice.CreateBranch(ctx, a.root, "external", "", true, false); err != nil {
		t.Fatal(err)
	}
	a.changeBranches("Creating unwanted", func(ctx context.Context, root string) error {
		return gitservice.CreateBranch(ctx, root, "unwanted", "", true, false)
	})
	waitUpdates(t, a, func() bool { return !a.gitBusy && !a.gitReading })
	status, _ := gitservice.ReadStatus(ctx, a.root)
	if status.Branch != "external" || !strings.Contains(a.gitActionError, "HEAD changed") {
		t.Fatal("stale branch operation was not rejected", status, a.gitActionError)
	}
}

func TestPluginCommandNamespacesAreUnambiguous(t *testing.T) {
	if pluginCommandID("foo.bar", "baz") == pluginCommandID("foo", "bar.baz") {
		t.Fatal("plugin command namespaces can collide")
	}
}
