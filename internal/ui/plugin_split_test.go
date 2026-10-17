package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/vt"
	"sunbook/internal/config"
	"sunbook/internal/editor"
	"sunbook/internal/plugin"
	"sunbook/internal/text"
)

func TestPluginSplitRejectsTextThatDiffersFromProposedEdits(t *testing.T) {
	a := examplePluginApp(t, "markdown-tools")
	directory := a.pluginStatuses[0].Entry.Directory
	source := "from sunbook_plugin import Plugin\nPlugin({'format-preview': lambda doc: {'preview': {'kind':'markdown','text':'shown'},'edits':[{'start':0,'end':len(doc['text'].encode('utf-8')),'text':'different'}]}}).run()\n"
	if err := os.WriteFile(filepath.Join(directory, "main.py"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	entry, err := plugin.ReadManifest(directory)
	if err != nil {
		t.Fatal(err)
	}
	a.setPluginEnabled(plugin.Status{Entry: entry}, true)
	waitPluginUI(t, a, func() bool { return !a.pluginRefreshing && a.pluginStatuses[0].State == plugin.Ready })
	a.view().Doc.Buffer = text.New("Original 中文")
	a.execute("plugin.markdown-tools/format-preview")
	waitPluginUI(t, a, func() bool { return strings.Contains(a.message, "does not match") })
	if a.comparing() || a.modal != nil || a.view().Doc.Buffer.String() != "Original 中文" {
		t.Fatal("misleading preview was displayed or applied")
	}
}

func renderLivePreview(t *testing.T, a *App) {
	t.Helper()
	p := a.pluginPreview()
	p.Due = zeroTime
	a.draw()
	waitPluginUI(t, a, func() bool {
		a.draw()
		return !p.Rendering && p.RenderBuffer == p.Outcome.doc.Buffer && p.RenderVersion == p.Outcome.doc.Buffer.Version
	})
	a.draw()
}

func renderedMarkdown(p *pluginSplitPreview) string {
	var b strings.Builder
	for _, line := range p.Rendered {
		for _, run := range line {
			b.WriteString(run.Text)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func TestMarkdownFormattingLiveEditorAndUndo(t *testing.T) {
	a := examplePluginApp(t, "markdown-tools")
	a.setPluginEnabled(a.pluginStatuses[0], true)
	waitPluginUI(t, a, func() bool { return !a.pluginRefreshing && a.pluginStatuses[0].State == plugin.Ready })
	source := "#   中文 👩‍💻   ###\n正文\n\n##   Tasks\n+ [X] 测试\n"
	d := a.view().Doc
	d.Path = filepath.Join(a.root, "notes.md")
	d.Buffer = text.New(source)
	a.splitEditor() // Opening from the second group must preserve both real views.
	views, split, active, docs := a.views, a.split, a.active, len(a.docs)
	a.execute("plugin.markdown-tools/format-preview")
	waitPluginUI(t, a, func() bool { return a.pluginPreview() != nil })
	renderLivePreview(t, a)
	p := a.pluginPreview()
	if a.comparison != nil || len(a.docs) != docs || a.views != views || a.split != split || a.active != active || a.focus != "editor" || d.Buffer.String() != source {
		t.Fatal("preview replaced the editor, created a tab or modified source")
	}
	for _, tab := range a.tabs {
		if tab.Doc == nil {
			t.Fatal("preview created a comparison tab")
		}
	}
	if !strings.Contains(renderedMarkdown(p), "☑") || strings.Contains(renderedMarkdown(p), "## ") {
		t.Fatal("Markdown was not rendered", renderedMarkdown(p))
	}
	// Source input remains ordinary editable input, including former preview hotkeys.
	a.view().Set(d.Buffer.Len(), false)
	a.key(tcell.NewEventKey(tcell.KeyRune, "a", 0))
	a.key(tcell.NewEventKey(tcell.KeyRune, "v", 0))
	a.paste("\n## 实时标题\n未保存内容\n```go\nunfinished")
	if !strings.Contains(d.Buffer.String(), "av\n## 实时标题") {
		t.Fatal("source typing was intercepted")
	}
	renderLivePreview(t, a)
	if !strings.Contains(renderedMarkdown(p), "实时标题") || !strings.Contains(renderedMarkdown(p), "未保存内容") || !strings.Contains(renderedMarkdown(p), "unfinished") {
		t.Fatal("unsaved edits did not render", renderedMarkdown(p))
	}
	a.execute("edit.undo")
	a.execute("edit.undo")
	a.execute("edit.undo")
	if d.Buffer.String() != source {
		t.Fatal("source undo stopped working", d.Buffer.String())
	}
	// Format stays in this editor and is a single undo step.
	a.execute("plugins.previewApply")
	waitPluginUI(t, a, func() bool { return !p.ApplyWhenReady })
	formatted := d.Buffer.String()
	if !strings.Contains(formatted, "# 中文 👩‍💻\n\n正文") || !strings.Contains(formatted, "- [x] 测试") || a.pluginPreview() != p {
		t.Fatal("format did not apply in place", formatted)
	}
	a.execute("edit.undo")
	if d.Buffer.String() != source || a.pluginPreview() != p {
		t.Fatal("format undo lost source or preview")
	}
	// After editing, explicit Format obtains a fresh plugin result.
	a.view().Set(d.Buffer.Len(), false)
	a.paste("\n##   Fresh\n")
	a.execute("plugins.previewApply")
	waitPluginUI(t, a, func() bool { return !p.ApplyWhenReady })
	if !strings.Contains(d.Buffer.String(), "## Fresh\n") {
		t.Fatal("format used an old snapshot", d.Buffer.String())
	}
	// A response arriving after another edit cannot overwrite that edit.
	a.view().Set(d.Buffer.Len(), false)
	a.paste("\n##   Race\n")
	a.execute("plugins.previewApply")
	a.paste("typing while plugin runs")
	during := d.Buffer.String()
	waitPluginUI(t, a, func() bool { return !p.ApplyWhenReady })
	if d.Buffer.String() != during || !strings.Contains(a.message, "formatting cancelled") {
		t.Fatal("late formatting overwrote typing", d.Buffer.String(), a.message)
	}
	a.execute("view.other")
	if a.focus != "markdown" || a.active != active {
		t.Fatal("preview focus changed editor group")
	}
	a.key(tcell.NewEventKey(tcell.KeyRune, "v", 0))
	if p.Reading {
		t.Fatal("right source mode did not toggle")
	}
	a.key(tcell.NewEventKey(tcell.KeyEsc, "", 0))
	if a.markdownPreview != nil || a.views != views || a.split != split || a.active != active {
		t.Fatal("closing preview changed existing split")
	}

	// Incomplete Markdown still opens, even if input changes while it is opening.
	d.Buffer = text.New("# Incomplete\n```go\ncode")
	a.view().Set(d.Buffer.Len(), false)
	a.execute("plugin.markdown-tools/format-preview")
	a.paste("more")
	waitPluginUI(t, a, func() bool { return a.pluginPreview() != nil })
	renderLivePreview(t, a)
	p = a.pluginPreview()
	if !strings.Contains(renderedMarkdown(p), "codemore") {
		t.Fatal("opening lost edits made during request")
	}
	incomplete := d.Buffer.String()
	a.applyPluginSplit()
	waitPluginUI(t, a, func() bool { return !p.ApplyWhenReady })
	if d.Buffer.String() != incomplete || !strings.Contains(a.message, "Close the fenced") {
		t.Fatal("incomplete code was formatted", a.message)
	}
	// Closing while a formatting request runs must not reopen a preview.
	a.paste("\n```\n")
	a.applyPluginSplit()
	a.closePluginSplit()
	waitPluginUI(t, a, func() bool { return !p.ApplyWhenReady })
	if a.markdownPreview != nil {
		t.Fatal("late result reopened a closed preview")
	}
}

func TestMarkdownLivePreviewLayoutAndStaleResults(t *testing.T) {
	a, mt := testApp(t)
	source := "#   Source\n" + strings.Repeat("line\n", 80)
	d := a.view().Doc
	d.Buffer = text.New(source)
	out := &pluginOutcome{name: "Markdown Tools", path: "notes.md", doc: d, buffer: d.Buffer, version: d.Buffer.Version, result: plugin.Result{Preview: &plugin.Preview{Kind: "markdown", Text: source}}, edits: []text.Edit{{Start: 0, End: len(source), Text: "# Source\n"}}}
	a.openPluginSplit(out)
	for _, theme := range []string{"sunbook", "github-light"} {
		preset, _ := config.FindTheme(theme)
		a.applyTheme(preset.Theme)
		for _, size := range []vt.Coord{{X: 110, Y: 32}, {X: 60, Y: 20}, {X: 45, Y: 12}, {X: 24, Y: 7}} {
			mt.SetSize(size)
			a.screen.Sync()
			a.draw()
			for _, b := range a.buttons {
				if b.Y == 3 && (b.X < 0 || b.X+b.W > int(size.X)) {
					t.Fatal("button outside terminal", size, b)
				}
			}
			left, right := a.rects[a.active], a.pluginPreview().Rect
			closeVisible := false
			for _, button := range a.buttons {
				if button.Command == "plugins.previewClose" {
					closeVisible = true
				}
			}
			if right.W >= 7 && !closeVisible {
				t.Fatal("narrow preview hides Close", size, right)
			}
			if left.X+left.W >= right.X || right.X+right.W > int(size.X) {
				t.Fatal("panes overlap", size, left, right)
			}
		}
	}
	mt.SetSize(vt.Coord{X: 110, Y: 32})
	a.screen.Sync()
	renderLivePreview(t, a)
	p := a.pluginPreview()
	a.pluginSplitMouse(p.Rect.X+2, 5, 0, true, 0)
	if a.focus != "markdown" {
		t.Fatal("cannot focus preview")
	}
	a.togglePluginSplitMode()
	a.draw()
	a.pluginSplitKey(tcell.NewEventKey(tcell.KeyEnd, "", 0), "end")
	if p.Top == 0 || a.view().Top != 0 {
		t.Fatal("preview scroll changed source")
	}
	d.Buffer.Apply([]text.Edit{{Start: d.Buffer.Len(), End: d.Buffer.Len(), Text: "new text"}}, nil, nil)
	changed := d.Buffer.String()
	a.applyPluginSplit()
	if d.Buffer.String() != changed || !strings.Contains(a.message, "Document changed") {
		t.Fatal("stale candidate applied")
	}
	a.focus = "editor"
	a.changed(d)
	due := p.Due
	p.Reading = true
	p.Rendering = false
	p.RenderWidth = -1
	a.draw()
	if p.Rendering || due.IsZero() {
		t.Fatal("typing was not debounced")
	}
	a.repaint = false
	a.runScheduled(due)
	if !p.Due.IsZero() || !a.repaint {
		t.Fatal("preview deadline did not request a paint")
	}
	// A worker result for a replaced buffer must never overwrite a newer preview.
	p.Reading = true
	p.Due = zeroTime
	p.RenderWidth = -1
	a.draw()
	d.Buffer = text.New("# Replacement\n")
	waitPluginUI(t, a, func() bool { return !p.Rendering })
	renderLivePreview(t, a)
	if !strings.Contains(renderedMarkdown(p), "Replacement") || strings.Contains(renderedMarkdown(p), "Source") {
		t.Fatal("stale render displayed", renderedMarkdown(p))
	}
	other := editor.New()
	a.addDocument(other)
	a.draw()
	if a.pluginPreview() != nil {
		t.Fatal("preview leaked onto another document")
	}
	a.activate(d)
	renderLivePreview(t, a)
	if a.pluginPreview() != p {
		t.Fatal("returning to source lost preview")
	}
}

func TestMarkdownRendererStylesWrappingAndUntrustedHTML(t *testing.T) {
	source := "---\ntitle: hidden metadata\n---\n\n# Heading\n\nA **bold** and *italic* [link](https://example.invalid).\n\n- [x] done\n- [ ] pending\n\n> quoted\n\n```go\nfmt.Println(\"code\")\n```\n\n| Name | Value |\n| --- | --- |\n| 中文 | 1 |\n\n<script>alert('never run')</script>\n\n![alt text](https://example.invalid/pixel.png)\n\n👩‍💻e\u0301" + strings.Repeat(" word", 20)
	for _, width := range []int{1, 12, 35, 70} {
		lines := markdownLines(source, width)
		roles := markdownRole(0)
		var rendered strings.Builder
		for _, line := range lines {
			var plain strings.Builder
			for _, run := range line {
				plain.WriteString(run.Text)
				roles |= run.Role
			}
			if text.Width(plain.String(), 4) > width {
				t.Fatal("rendered line exceeds width", width, plain.String())
			}
			rendered.WriteString(plain.String())
			rendered.WriteByte('\n')
		}
		if width >= 35 {
			for _, expected := range []string{"Heading", "bold", "italic", "link", "☑ done", "☐ pending", "quoted", "fmt.Println", "中文", "[Image: alt text]", "👩‍💻e\u0301"} {
				if !strings.Contains(rendered.String(), expected) {
					t.Fatal("missing rendered content", expected, rendered.String())
				}
			}
		}
		if strings.Contains(rendered.String(), "hidden metadata") || strings.Contains(rendered.String(), "never run") || strings.Contains(rendered.String(), "pixel.png") {
			t.Fatal("HTML/remote asset was included in reading output")
		}
		if roles&(mdBold|mdItalic|mdHeading|mdCode|mdLink) != (mdBold | mdItalic | mdHeading | mdCode | mdLink) {
			t.Fatal("semantic styles lost", roles)
		}
	}
}

func TestMarkdownTableWrapsFullContentWithinColumns(t *testing.T) {
	description := "Explorer 支持目录展开、文件名/路径过滤和语言图标；完整保留中文 👩‍💻 e\u0301，以及粗体、链接和 Ctrl+P 这些代码。"
	source := "| 功能 | 使用方式 |\n| :--- | ---: |\n| 文件浏览 | Explorer 支持目录展开、文件名/路径过滤和语言图标；完整保留中文 👩‍💻 e\u0301，以及**粗体**、[链接](https://example.invalid)和 `Ctrl+P` 这些代码。 |\n"
	compact := func(s string) string {
		return strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) {
				return -1
			}
			return r
		}, s)
	}
	for _, width := range []int{12, 24, 44, 80, 120} {
		lines := markdownLines(source, width)
		var label, body strings.Builder
		afterHeader := false
		roles := markdownRole(0)
		for _, line := range lines {
			var plain strings.Builder
			for _, run := range line {
				plain.WriteString(run.Text)
				roles |= run.Role
			}
			value := plain.String()
			if text.Width(value, 4) > width {
				t.Fatal("table exceeds pane width", width, value)
			}
			if strings.Contains(value, "…") {
				t.Fatal("table content was truncated", width, value)
			}
			if strings.HasPrefix(value, "─") {
				afterHeader = true
				continue
			}
			if !afterHeader || strings.TrimSpace(value) == "" {
				continue
			}
			columns := strings.SplitN(value, " │ ", 2)
			if len(columns) != 2 {
				t.Fatal("wrapped table lost column boundaries", width, value)
			}
			label.WriteString(columns[0])
			body.WriteString(columns[1])
		}
		if compact(label.String()) != "文件浏览" || compact(body.String()) != compact(description) {
			t.Fatal("wrapped table lost content", width, label.String(), body.String())
		}
		if roles&(mdBold|mdCode|mdLink) != (mdBold | mdCode | mdLink) {
			t.Fatal("table lost inline formatting", roles)
		}
	}
	// A short name column leaves the rest of the pane for the long description.
	lines := markdownLines("| Key | Description |\n| --- | --- |\n| A | "+strings.Repeat("word ", 30)+"|\n", 80)
	for _, line := range lines {
		var value strings.Builder
		for _, run := range line {
			value.WriteString(run.Text)
		}
		if index := strings.Index(value.String(), "│"); index >= 0 && text.Width(value.String()[:index], 4) != 4 {
			t.Fatal("short column wasted description space", value.String())
		}
	}
}

func TestMarkdownTableNarrowFlowPreservesAllColumns(t *testing.T) {
	var headers, separator, values []string
	for i := 0; i < 40; i++ {
		headers = append(headers, fmt.Sprintf("H%d", i))
		separator = append(separator, "---")
		values = append(values, fmt.Sprintf("value%d", i))
	}
	source := "|" + strings.Join(headers, "|") + "|\n|" + strings.Join(separator, "|") + "|\n|" + strings.Join(values, "|") + "|\n"
	for _, width := range []int{12, 45, 120} {
		var body strings.Builder
		for _, line := range markdownLines(source, width) {
			var plain strings.Builder
			for _, run := range line {
				plain.WriteString(run.Text)
			}
			if text.Width(plain.String(), 4) > width {
				t.Fatal("flowed row exceeds width", width, plain.String())
			}
			body.WriteString(plain.String())
			body.WriteByte('\n')
		}
		for _, value := range values {
			if !strings.Contains(body.String(), value) {
				t.Fatal("table dropped a column", width, value)
			}
		}
	}
}
