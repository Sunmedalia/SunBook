package ui

import (
	"strings"

	"github.com/rivo/uniseg"
	"sunbook/internal/text"
)

type gitMessageLine struct{ Start, End int }

// Keep byte positions for editing and mouse placement while wrapping on terminal
// cells. The draft itself retains newlines and complete Unicode graphemes.
func gitMessageLines(value string, width int) []gitMessageLine {
	width = max(1, width)
	var lines []gitMessageLine
	start, col := 0, 0
	g := uniseg.NewGraphemes(value)
	for g.Next() {
		lo, hi := g.Positions()
		if g.Str() == "\n" {
			lines = append(lines, gitMessageLine{start, lo})
			start, col = hi, 0
			continue
		}
		w := max(1, g.Width())
		if col > 0 && col+w > width {
			lines = append(lines, gitMessageLine{start, lo})
			start, col = lo, 0
		}
		col += w
	}
	lines = append(lines, gitMessageLine{start, len(value)})
	if col >= width {
		lines = append(lines, gitMessageLine{len(value), len(value)})
	}
	return lines
}
func gitMessageRow(lines []gitMessageLine, cursor int) int {
	index := 0
	for i, line := range lines {
		if line.Start > cursor {
			break
		}
		index = i
		if cursor < line.End {
			break
		}
	}
	return index
}
func (a *App) drawGitMessage() {
	r := a.gitMessageRect
	style := a.base.Background(a.colors.inset)
	a.fill(r, style)
	a.gitMessageCursor = text.Snap(a.gitMessage, a.gitMessageCursor)
	lines := gitMessageLines(a.gitMessage, r.W)
	cursorRow := gitMessageRow(lines, a.gitMessageCursor)
	a.gitMessageTop = max(0, min(a.gitMessageTop, max(0, len(lines)-r.H)))
	if cursorRow < a.gitMessageTop {
		a.gitMessageTop = cursorRow
	}
	if cursorRow >= a.gitMessageTop+r.H {
		a.gitMessageTop = cursorRow - r.H + 1
	}
	for y, i := r.Y, a.gitMessageTop; y < r.Y+r.H && i < len(lines); y, i = y+1, i+1 {
		line := lines[i]
		a.put(r.X, y, r.W, a.gitMessage[line.Start:line.End], style)
	}
	if a.focus == "gitMessage" {
		for y := r.Y; y < r.Y+r.H; y++ {
			a.put(r.X-1, y, 1, "▎", style.Foreground(a.accent.GetForeground()))
		}
	}
	if a.gitMessage == "" {
		a.put(r.X, r.Y, r.W, "Commit message… (c)", style.Foreground(a.colors.muted))
		if r.H >= 3 {
			a.put(r.X, r.Y+2, r.W, "Ctrl+J adds a new line", style.Foreground(a.colors.muted))
		}
	}
	if a.focus == "gitMessage" && a.modal == nil && r.W > 0 && r.H > 0 {
		line := lines[cursorRow]
		col := text.Width(a.gitMessage[line.Start:a.gitMessageCursor], a.cfg.TabSize)
		a.screen.ShowCursor(r.X+min(r.W-1, col), r.Y+cursorRow-a.gitMessageTop)
	}
}
func (a *App) gitMessageMouse(x, y int, fresh bool) bool {
	r := a.gitMessageRect
	if !r.contains(x, y) {
		return false
	}
	a.hoverHint = "Commit message · Enter Commit · Ctrl+J New line · Tab Filter"
	if fresh {
		a.focus = "gitMessage"
		lines := gitMessageLines(a.gitMessage, r.W)
		line := lines[min(len(lines)-1, a.gitMessageTop+y-r.Y)]
		a.gitMessageCursor = line.Start + text.Offset(a.gitMessage[line.Start:line.End], x-r.X, a.cfg.TabSize)
	}
	return true
}
func (a *App) moveGitMessageCursor(delta int) {
	lines := gitMessageLines(a.gitMessage, a.gitMessageRect.W)
	row := gitMessageRow(lines, a.gitMessageCursor)
	if delta > 0 && row == len(lines)-1 {
		a.focus = "git"
		a.moveGitSelection(0)
		return
	}
	target := max(0, min(len(lines)-1, row+delta))
	col := text.Width(a.gitMessage[lines[row].Start:a.gitMessageCursor], a.cfg.TabSize)
	line := lines[target]
	a.gitMessageCursor = line.Start + text.Offset(a.gitMessage[line.Start:line.End], col, a.cfg.TabSize)
}
func normalizeGitMessage(value string) string {
	return strings.NewReplacer("\r\n", "\n", "\r", "\n", "\t", " ").Replace(value)
}
