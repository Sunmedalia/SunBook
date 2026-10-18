package ui

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v3"
	"github.com/rivo/uniseg"
	gitservice "sunbook/internal/git"
	"sunbook/internal/text"
)

type comparisonRow struct {
	Old, New             string
	OldNumber, NewNumber int
	Removed, Added       bool
}
type gitComparison struct {
	Pair               gitservice.Comparison
	Rows               []comparisonRow
	Changes            []int
	Top, Pane          int
	Left               [2]int
	Rects              [2]rect
	Ratio              float64
	Active             bool
	Added, Removed     int
	OldLines, NewLines int
	LineOffsets        [2][]int
	Highlights         [2][]tokenSpan
}

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

func comparisonLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}
func newGitComparison(pair gitservice.Comparison) *gitComparison {
	c := &gitComparison{Pair: pair, Ratio: .5, Active: true}
	if pair.Binary {
		c.Rows = []comparisonRow{{Old: fmt.Sprintf("Binary file · %d bytes", len(pair.Old)), New: fmt.Sprintf("Binary file · %d bytes", len(pair.New))}}
		return c
	}
	old, new := comparisonLines(pair.Old), comparisonLines(pair.New)
	c.OldLines, c.NewLines = len(old), len(new)
	for pane, lines := range [2][]string{old, new} {
		offset := 0
		for _, line := range lines {
			c.LineOffsets[pane] = append(c.LineOffsets[pane], offset)
			offset += len(line) + 1
		}
	}
	oi, ni := 0, 0
	appendRow := func(removed, added bool) {
		r := comparisonRow{Removed: removed, Added: added}
		if oi < len(old) {
			r.Old = old[oi]
			r.OldNumber = oi + 1
			oi++
		}
		if ni < len(new) {
			r.New = new[ni]
			r.NewNumber = ni + 1
			ni++
		}
		c.Rows = append(c.Rows, r)
	}
	appendEqualUntil := func(oldEnd, newEnd int) {
		for oi < oldEnd || ni < newEnd {
			r := comparisonRow{}
			if oi < oldEnd && oi < len(old) {
				r.Old = old[oi]
				r.OldNumber = oi + 1
				oi++
			}
			if ni < newEnd && ni < len(new) {
				r.New = new[ni]
				r.NewNumber = ni + 1
				ni++
			}
			c.Rows = append(c.Rows, r)
		}
	}
	var removed, added []int
	flush := func() {
		if len(removed)+len(added) == 0 {
			return
		}
		c.Changes = append(c.Changes, len(c.Rows))
		for i := 0; i < max(len(removed), len(added)); i++ {
			r := comparisonRow{}
			if i < len(removed) {
				p := removed[i]
				r.OldNumber = p + 1
				r.Old = old[p]
				r.Removed = true
				c.Removed++
			}
			if i < len(added) {
				p := added[i]
				r.NewNumber = p + 1
				r.New = new[p]
				r.Added = true
				c.Added++
			}
			c.Rows = append(c.Rows, r)
		}
		removed, added = nil, nil
	}
	inHunk := false
	for _, line := range strings.Split(pair.Patch, "\n") {
		if m := hunkHeader.FindStringSubmatch(line); m != nil {
			flush()
			os, _ := strconv.Atoi(m[1])
			ns, _ := strconv.Atoi(m[3])
			if m[2] != "0" {
				os--
			}
			if m[4] != "0" {
				ns--
			}
			appendEqualUntil(min(len(old), max(oi, os)), min(len(new), max(ni, ns)))
			inHunk = true
			continue
		}
		if !inHunk || line == "" {
			continue
		}
		switch line[0] {
		case ' ':
			flush()
			appendRow(false, false)
		case '-':
			if oi < len(old) {
				removed = append(removed, oi)
				oi++
			}
		case '+':
			if ni < len(new) {
				added = append(added, ni)
				ni++
			}
		case '\\': // Git's end-of-file newline marker has no display row.
		default:
			flush()
			inHunk = false
		}
	}
	flush()
	appendEqualUntil(len(old), len(new))
	if len(c.Changes) > 0 {
		c.Top = max(0, c.Changes[0]-3)
	}
	return c
}
func (c *gitComparison) highlight(base tcell.Style, syntax string) {
	if !c.Pair.Binary {
		c.Highlights[0] = syntaxSpans(c.Pair.Path, c.Pair.Old, base, syntax)
		c.Highlights[1] = syntaxSpans(c.Pair.Path, c.Pair.New, base, syntax)
	}
}
func (a *App) comparing() bool { return a.comparison != nil && a.comparison.Active }
func (a *App) drawComparison() {
	c := a.comparison
	r := a.rects[0]
	width := a.editorArea.W
	left := max(1, min(width-2, int(float64(width)*c.Ratio)))
	c.Rects = [2]rect{{r.X, 3, left, r.H}, {r.X + left + 1, 3, max(0, width-left-1), r.H}}
	c.Top = max(0, min(c.Top, max(0, len(c.Rows)-max(1, r.H))))
	for pane, pr := range c.Rects {
		contentWidth := pr.W
		if len(c.Rows) > pr.H && pr.W > 1 {
			contentWidth--
		}
		label := c.Pair.OldLabel
		if pane == 1 {
			label = c.Pair.NewLabel
		}
		header := a.chrome.Foreground(a.colors.muted).Bold(true)
		if pane == c.Pane {
			header = header.Foreground(a.accent.GetForeground())
		}
		a.fill(rect{pr.X, 2, pr.W, 1}, a.chrome)
		a.put(pr.X+1, 2, pr.W-2, fitLabel(label+" · "+c.Pair.Path, pr.W-2), header)
		gutter := len(strconv.Itoa(max(c.OldLines, c.NewLines))) + 2
		gutter = min(gutter, max(1, pr.W/3))
		for y, index := pr.Y, c.Top; y < pr.Y+pr.H && index < len(c.Rows); y, index = y+1, index+1 {
			row := c.Rows[index]
			line, number, changed := row.Old, row.OldNumber, row.Removed
			color := a.colors.danger
			if pane == 1 {
				line, number, changed = row.New, row.NewNumber, row.Added
				color = a.colors.success
			}
			style := a.base
			if changed {
				style = style.Background(mix(a.base.GetBackground(), color, 12))
			}
			if number == 0 && !c.Pair.Binary {
				style = style.Background(a.colors.inset)
			}
			a.fill(rect{pr.X, y, pr.W, 1}, style)
			if number > 0 {
				a.put(pr.X, y, gutter-1, lineNumber(number, gutter-1), style.Foreground(a.colors.muted))
			}
			if changed {
				marker := "−"
				if pane == 1 {
					marker = "+"
				}
				a.put(pr.X+gutter-1, y, 1, marker, style.Foreground(color))
			}
			offset := 0
			if number > 0 {
				offset = c.LineOffsets[pane][number-1]
			}
			a.drawComparisonLine(pr.X+gutter, y, contentWidth-gutter, line, offset, c.Left[pane], c.Highlights[pane], style)
		}
		a.drawContentScrollbar(pr, &c.Top, len(c.Rows), "diff", pane)
	}
	for y := 2; y < r.Y+r.H; y++ {
		a.put(c.Rects[1].X-1, y, 1, "│", a.base.Foreground(a.colors.border))
	}
}
func (a *App) drawComparisonLine(x, y, width int, line string, offset, left int, spans []tokenSpan, base tcell.Style) {
	if width <= 0 {
		return
	}
	if len(spans) == 0 {
		line = expandTabs(line, a.cfg.TabSize)
		a.put(x, y, width, line[text.Offset(line, left, a.cfg.TabSize):], base)
		return
	}
	span := sort.Search(len(spans), func(i int) bool { return spans[i].End > offset })
	column := 0
	g := uniseg.NewGraphemes(line)
	for g.Next() {
		lo, _ := g.Positions()
		position := offset + lo
		for span < len(spans) && spans[span].End <= position {
			span++
		}
		style := base
		if span < len(spans) && spans[span].Start <= position {
			// Syntax controls text color and attributes; the comparison owns the
			// red/green row background, including spaces and tabs.
			style = spans[span].Style.Background(base.GetBackground())
		}
		w := max(1, g.Width())
		start := column - left
		if g.Str() == "\t" {
			w = a.cfg.TabSize - column%a.cfg.TabSize
			for dx := max(0, start); dx < min(width, start+w); dx++ {
				a.put(x+dx, y, 1, " ", style)
			}
		} else if start >= 0 && start+w <= width {
			a.put(x+start, y, w, g.Str(), style)
		}
		column += w
		if start >= width {
			break
		}
	}
}
func (a *App) closeComparison() { a.gitViewSeq++; a.comparison = nil; a.focus = "editor" }
func (a *App) comparisonKey(e *tcell.EventKey, name string) {
	c := a.comparison
	height := max(1, c.Rects[0].H)
	switch name {
	case "escape", "ctrl+w":
		a.closeComparison()
	case "ctrl+q":
		a.confirmExit()
	case "ctrl+tab":
		a.nextTab(1)
	case "ctrl+shift+tab":
		a.nextTab(-1)
	case "up":
		c.Top = max(0, c.Top-1)
	case "down":
		c.Top = min(max(0, len(c.Rows)-height), c.Top+1)
	case "pageup":
		c.Top = max(0, c.Top-height+1)
	case "pagedown":
		c.Top = min(max(0, len(c.Rows)-height), c.Top+height-1)
	case "home":
		c.Top = 0
		c.Left = [2]int{}
	case "end":
		c.Top = max(0, len(c.Rows)-height)
	case "left":
		c.Left[c.Pane] = max(0, c.Left[c.Pane]-8)
	case "right":
		a.panComparison(c.Pane, 8)
	case "tab", "shift+tab":
		c.Pane = 1 - c.Pane
	case "n", "alt+down":
		a.nextComparisonChange(1)
	case "p", "alt+up":
		a.nextComparisonChange(-1)
	case "o":
		a.showError(a.open(filepath.Join(c.Pair.Root, c.Pair.Path)))
	case "f5":
		a.refreshGit()
	case "ctrl+s", "ctrl+z", "ctrl+y":
		a.notify("Read-only comparison · press o to open the working file")
	default:
		if id := a.bindings[name]; id != "" {
			a.execute(id)
		}
	}
}
func (a *App) panComparison(pane, delta int) {
	c := a.comparison
	limit := 0
	for _, row := range c.Rows {
		s := row.Old
		if pane == 1 {
			s = row.New
		}
		limit = max(limit, text.Width(s, a.cfg.TabSize))
	}
	c.Left[pane] = max(0, min(max(0, limit-max(1, c.Rects[pane].W-8)), c.Left[pane]+delta))
}
func (a *App) nextComparisonChange(direction int) {
	c := a.comparison
	if direction > 0 {
		for _, row := range c.Changes {
			if row > c.Top {
				c.Top = row
				return
			}
		}
	} else {
		for i := len(c.Changes) - 1; i >= 0; i-- {
			if c.Changes[i] < c.Top {
				c.Top = c.Changes[i]
				return
			}
		}
	}
}
func (a *App) comparisonMouse(x, y, wheel int, fresh bool, mod tcell.ModMask) bool {
	if !a.comparing() {
		return false
	}
	c := a.comparison
	for pane, r := range c.Rects {
		if !r.contains(x, y) && !(y == 2 && x >= r.X && x < r.X+r.W) {
			continue
		}
		if fresh {
			c.Pane = pane
			a.focus = "diff"
		}
		if wheel != 0 {
			if mod&tcell.ModShift != 0 {
				a.panComparison(pane, wheel*4)
			} else {
				c.Top = max(0, min(max(0, len(c.Rows)-max(1, r.H)), c.Top+wheel))
			}
		}
		return true
	}
	return false
}
