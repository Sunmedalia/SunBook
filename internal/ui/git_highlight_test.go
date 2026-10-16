package ui

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v3"
	gitservice "sunbook/internal/git"
	"sunbook/internal/text"
)

func comparisonToken(t *testing.T, a *App, pane int, line, token string) (string, tcell.Style) {
	t.Helper()
	c := a.comparison
	for i, row := range c.Rows {
		source := row.Old
		if pane == 1 {
			source = row.New
		}
		if !strings.HasPrefix(source, line) {
			continue
		}
		position := strings.Index(source, token)
		if position < 0 {
			t.Fatal("token missing from fixture", token, source)
		}
		r := c.Rects[pane]
		gutter := len(strconv.Itoa(max(c.OldLines, c.NewLines))) + 2
		x := r.X + gutter + text.Width(source[:position], a.cfg.TabSize) - c.Left[pane]
		y := r.Y + i - c.Top
		if x < r.X+gutter || x >= r.X+r.W || y < r.Y || y >= r.Y+r.H {
			t.Fatal("token outside viewport", token, x, y)
		}
		value, style, _ := a.screen.Get(x, y)
		return value, style
	}
	t.Fatal("line not in comparison", line)
	return "", tcell.StyleDefault
}

func TestGitComparisonsHighlightBothSnapshotsWithChangeBackgrounds(t *testing.T) {
	a := gitTestWorkspace(t)
	ctx := context.Background()
	path := filepath.Join(a.root, "main.go")
	base := "package sample\n\n/*\nvar hidden = \"old\"\n*/\nvar message = \"中 old\"\n\nfunc main() {\n\tprintln(message)\n}\n"
	index := strings.ReplaceAll(base, "old", "index")
	index = strings.Replace(index, "var message", "var inserted = 123\nvar message", 1)
	worktree := strings.ReplaceAll(index, "index", "worktree")
	worktree = strings.Replace(worktree, "var message", "var another = 456\nvar message", 1)
	writeOpenFixture(t, path, base)
	if err := gitservice.Stage(ctx, a.root, "main.go"); err != nil {
		t.Fatal(err)
	}
	if err := gitservice.Commit(ctx, a.root, "base source"); err != nil {
		t.Fatal(err)
	}
	writeOpenFixture(t, path, index)
	if err := gitservice.Stage(ctx, a.root, "main.go"); err != nil {
		t.Fatal(err)
	}
	writeOpenFixture(t, path, worktree)
	a.typeInput("keep unsaved editor")
	original := a.view().Doc
	a.showSidebar("Git")
	waitUpdates(t, a, func() bool { return !a.gitReading })
	for _, staged := range []bool{true, false} {
		name := "worktree"
		if staged {
			name = "staged"
		}
		t.Run(name, func(t *testing.T) {
			previous := a.comparison
			a.gitDiff(staged)
			waitUpdates(t, a, func() bool { return a.comparison != nil && a.comparison != previous })
			c := a.comparison
			if staged && (c.Pair.Old != base || c.Pair.New != index) || !staged && (c.Pair.Old != index || c.Pair.New != worktree) {
				t.Fatal("highlighted the wrong Git snapshots")
			}
			c.Top = 0
			a.draw()
			for pane, color := range [2]tcell.Color{a.colors.danger, a.colors.success} {
				value, keyword := comparisonToken(t, a, pane, "var message", "var")
				if value != "v" {
					t.Fatal("rendered keyword displaced", pane, value)
				}
				_, literal := comparisonToken(t, a, pane, "var message", "\"")
				_, comment := comparisonToken(t, a, pane, "var hidden", "var")
				_, unchanged := comparisonToken(t, a, pane, "package sample", "package")
				if keyword.GetForeground() == literal.GetForeground() || keyword.GetForeground() == comment.GetForeground() || keyword.GetForeground() == a.base.GetForeground() {
					t.Fatal("keywords, strings, and multiline comments lack distinct syntax colors", pane)
				}
				background := mix(a.base.GetBackground(), color, 12)
				for _, style := range []tcell.Style{keyword, literal, comment} {
					if style.GetBackground() != background {
						t.Fatal("syntax overwrote change background", pane)
					}
				}
				if unchanged.GetBackground() != a.base.GetBackground() || unchanged.GetForeground() != keyword.GetForeground() {
					t.Fatal("unchanged code lost syntax or gained change background", pane)
				}
			}
			if len(a.docs) != 1 || a.view().Doc != original || original.Buffer.String() != "keep unsaved editor" {
				t.Fatal("highlighting changed the editor buffer")
			}
		})
	}
}

func TestComparisonSyntaxPreservesTabsUnicodeAndHorizontalScrolling(t *testing.T) {
	a, _ := testApp(t)
	line := "\tvar message = \"中👩‍💻é\" // note"
	a.comparison = newGitComparison(gitservice.Comparison{Path: "main.go", Old: line, New: line})
	c := a.comparison
	c.highlight(a.base)
	c.Left = [2]int{2, 2} // Start in the middle of the leading tab.
	a.draw()
	for pane := range 2 {
		value, keyword := comparisonToken(t, a, pane, "\tvar message", "var")
		if value != "v" || keyword.GetForeground() == a.base.GetForeground() {
			t.Fatal("tab expansion displaced syntax", pane, value)
		}
		// Pan directly to the Unicode string, preserving byte-based token offsets
		// while terminal coordinates follow grapheme display widths.
		c.Left[pane] = text.Width(line[:strings.Index(line, "中")], a.cfg.TabSize)
		a.draw()
		for _, token := range []string{"中", "👩‍💻", "é"} {
			value, style := comparisonToken(t, a, pane, "\tvar message", token)
			if value != token || style.GetForeground() == a.base.GetForeground() || style.GetForeground() == keyword.GetForeground() {
				t.Fatal("horizontal scroll lost Unicode string highlighting", pane, token, value)
			}
		}
	}
}

func TestComparisonSyntaxFallbacks(t *testing.T) {
	a, _ := testApp(t)
	for _, pair := range []gitservice.Comparison{
		{Path: "unknown.extension", Old: "plain text\n", New: "plain text\n"},
		{Path: "empty.go"},
		{Path: "binary.go", Binary: true, Old: "\x00", New: "\x00\x00"},
		{Path: "large.go", Old: strings.Repeat("x", 2*1024*1024+1), New: "package main\n"},
	} {
		t.Run(pair.Path, func(t *testing.T) {
			c := newGitComparison(pair)
			c.highlight(a.base)
			a.comparison = c
			a.draw()
			if pair.Binary && (len(c.Highlights[0]) != 0 || len(c.Highlights[1]) != 0) {
				t.Fatal("binary content was tokenized")
			}
			if len(pair.Old) > 2*1024*1024 && (len(c.Highlights[0]) != 0 || len(c.Highlights[1]) == 0) {
				t.Fatal("large snapshot did not fall back independently")
			}
		})
	}
}
