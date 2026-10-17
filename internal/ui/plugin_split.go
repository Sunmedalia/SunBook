package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v3"
	"sunbook/internal/text"
)

// A preview belongs to the original document, independently of editor groups
// and Git comparisons. The source always uses its real editor.View.
type pluginSplitPreview struct {
	Outcome                    *pluginOutcome
	Reading                    bool
	Top                        int
	Left                       int
	Rect                       rect
	RenderWidth, RenderVersion int
	RenderBuffer               *text.Buffer
	Rendered                   []markdownLine
	Rendering                  bool
	Due                        time.Time
	ApplyWhenReady             bool
}

func pluginEditedText(source string, edits []text.Edit) string {
	var b strings.Builder
	start := 0
	for _, edit := range edits {
		b.WriteString(source[start:edit.Start])
		b.WriteString(edit.Text)
		start = edit.End
	}
	b.WriteString(source[start:])
	return b.String()
}

func (a *App) openPluginSplit(out *pluginOutcome) {
	p := a.markdownPreview
	if p == nil || p.Outcome.doc != out.doc {
		p = &pluginSplitPreview{Reading: true, RenderWidth: -1}
		a.markdownPreview = p
	}
	p.Outcome = out
	a.keepDocument(out.doc)
	a.focus = "editor"
	a.completion, a.completionDue = nil, zeroTime
	a.notify("Live Markdown preview · edit on the left · Format applies one undo step")
}

func (a *App) pluginPreview() *pluginSplitPreview {
	p := a.markdownPreview
	if p != nil && !a.comparing() && p.Outcome.doc == a.view().Doc {
		return p
	}
	return nil
}

func (a *App) closePluginSplit() {
	a.markdownPreview = nil
	if a.focus == "markdown" {
		a.focus = "editor"
	}
}

func (a *App) applyPluginSplit() {
	p := a.pluginPreview()
	if p == nil || p.ApplyWhenReady {
		return
	}
	out := p.Outcome
	if out.doc.Buffer != out.buffer || out.doc.Buffer.Version != out.version {
		// Formatting is explicit; obtain a fresh proposal after edits. The renderer
		// does not invoke plugin processes for every keystroke.
		if out.id != "" && out.command != "" {
			p.ApplyWhenReady = true
			a.invokePluginCommand(out.id, out.command, p)
		} else {
			a.notify("Document changed; run the formatting command again")
		}
		return
	}
	if len(out.edits) == 0 {
		if out.result.Message != "" {
			a.notify(out.result.Message)
		} else {
			a.notify("Markdown is already formatted")
		}
		return
	}
	if a.apply(out.edits) {
		a.notify("Markdown formatted · Ctrl+Z Undo · Ctrl+S Save")
	}
}

func (a *App) togglePluginSplitMode() {
	if p := a.pluginPreview(); p != nil {
		p.Reading = !p.Reading
		p.Top, p.Left = 0, 0
	}
}

func (a *App) prepareMarkdownLayout(p *pluginSplitPreview, width int) {
	b := p.Outcome.doc.Buffer
	if p.Rendering || (p.RenderWidth == width && p.RenderBuffer == b && p.RenderVersion == b.Version) {
		return
	}
	if !p.Due.IsZero() && time.Now().Before(p.Due) {
		return
	}
	p.Due = zeroTime
	p.RenderWidth, p.RenderBuffer, p.RenderVersion = width, b, b.Version
	if b.Len() > 1<<20 {
		p.Rendered = []markdownLine{{{Text: "Preview limit: 1 MiB", Role: mdMuted}}}
		return
	}
	version, source := b.Version, b.String()
	p.Rendering = true
	go func() {
		lines := markdownLines(source, width)
		a.post(func() {
			if a.markdownPreview != p {
				return
			}
			p.Rendering = false
			if p.Outcome.doc.Buffer == b && b.Version == version {
				p.Rendered = lines
			} else {
				p.RenderWidth = -1
			}
		})
	}()
}

func (a *App) drawPluginSplit() {
	p := a.pluginPreview()
	r := p.Rect
	a.drawEditor(a.active)
	source := a.rects[a.active]
	a.put(source.X+1, 2, source.W-2, a.view().Doc.Name(), a.muted)
	for y := 2; y < r.Y+r.H; y++ {
		a.put(r.X-1, y, 1, "│", a.base.Foreground(a.colors.border))
	}
	a.fill(rect{r.X, 2, r.W, 2}, a.chrome)
	label := "Markdown source"
	if p.Reading {
		label = "Live preview"
	}
	a.put(r.X+1, 2, r.W-2, label, a.accent.Bold(true))
	x := r.X
	for _, action := range []struct{ label, id, hint string }{
		{"Format", "plugins.previewApply", "Format current document · one undo step"},
		{"Source/Read", "plugins.previewToggle", "Toggle Markdown source and live reading preview"},
		{"Close", "plugins.previewClose", "Close right preview"},
	} {
		width := text.Width(action.label, 4) + 2
		reserve := 0
		if action.id != "plugins.previewClose" {
			reserve = text.Width("Close", 4) + 3
		}
		if x+width+reserve <= r.X+r.W {
			x += a.drawButton(x, 3, action.label, action.id, action.hint, false) + 1
		}
	}
	if p.Reading {
		a.prepareMarkdownLayout(p, max(1, r.W-3))
	}
	total := a.pluginSplitTotal()
	p.Top = max(0, min(p.Top, max(0, total-r.H)))
	for y, index := r.Y, p.Top; y < r.Y+r.H && index < total; y, index = y+1, index+1 {
		if !p.Reading {
			b := p.Outcome.doc.Buffer
			gutter := min(6, max(2, r.W/4))
			a.put(r.X, y, gutter-1, fmt.Sprintf("%*d", gutter-1, index+1), a.muted)
			a.drawComparisonLine(r.X+gutter, y, r.W-gutter-1, b.Line(index), b.LineStart(index), p.Left, nil, a.base)
			continue
		}
		x := r.X + 1
		for _, run := range p.Rendered[index] {
			style := a.base
			if run.Role&mdMuted != 0 {
				style = a.muted
			}
			if run.Role&mdHeading != 0 {
				style = style.Foreground(a.accent.GetForeground())
			}
			if run.Role&mdLink != 0 {
				style = style.Foreground(a.accent.GetForeground()).Underline(tcell.UnderlineStyleSolid, a.accent.GetForeground())
			}
			if run.Role&mdCode != 0 {
				style = style.Background(a.colors.inset)
			}
			if run.Role&mdBold != 0 {
				style = style.Bold(true)
			}
			if run.Role&mdItalic != 0 {
				style = style.Italic(true)
			}
			if run.Role&mdStrike != 0 {
				style = style.StrikeThrough(true)
			}
			a.put(x, y, max(0, r.X+r.W-1-x), run.Text, style)
			x += text.Width(run.Text, 4)
		}
	}
	a.drawContentScrollbar(r, &p.Top, total, "pluginPreview", 1)
}

func (a *App) pluginSplitTotal() int {
	p := a.pluginPreview()
	if p == nil {
		return 0
	}
	if p.Reading {
		return len(p.Rendered)
	}
	return p.Outcome.doc.Buffer.LineCount()
}

func (a *App) pluginSplitKey(e *tcell.EventKey, name string) {
	p := a.pluginPreview()
	if p == nil {
		a.focus = "editor"
		return
	}
	height := max(1, p.Rect.H)
	switch name {
	case "a", "ctrl+enter":
		a.applyPluginSplit()
	case "v":
		a.togglePluginSplitMode()
	case "escape", "ctrl+w":
		a.closePluginSplit()
	case "up":
		p.Top = max(0, p.Top-1)
	case "down":
		p.Top = min(max(0, a.pluginSplitTotal()-height), p.Top+1)
	case "pageup":
		p.Top = max(0, p.Top-height+1)
	case "pagedown":
		p.Top = min(max(0, a.pluginSplitTotal()-height), p.Top+height-1)
	case "home":
		p.Top, p.Left = 0, 0
	case "end":
		p.Top = max(0, a.pluginSplitTotal()-height)
	case "tab", "shift+tab":
		a.focus = "editor"
	case "left":
		p.Left = max(0, p.Left-8)
	case "right":
		if !p.Reading {
			p.Left += 8
		}
	default:
		if id := a.bindings[name]; id != "" {
			a.execute(id)
		}
	}
}

func (a *App) pluginSplitMouse(x, y, wheel int, fresh bool, mod tcell.ModMask) bool {
	p := a.pluginPreview()
	if p == nil {
		return false
	}
	r := p.Rect
	if !r.contains(x, y) && !(y >= 2 && y <= 3 && x >= r.X && x < r.X+r.W) {
		return false
	}
	if fresh {
		a.focus = "markdown"
	}
	if wheel != 0 {
		if mod&tcell.ModShift != 0 && !p.Reading {
			p.Left = max(0, p.Left+wheel*4)
		} else {
			p.Top = max(0, min(max(0, a.pluginSplitTotal()-max(1, r.H)), p.Top+wheel))
		}
	}
	return true
}
