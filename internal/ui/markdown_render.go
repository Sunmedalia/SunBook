package ui

import (
	"fmt"
	"html"
	"sort"
	"strings"
	"unicode"

	"github.com/rivo/uniseg"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	mdtext "github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"sunbook/internal/text"
)

type markdownRole uint8

const (
	mdBold markdownRole = 1 << iota
	mdItalic
	mdCode
	mdHeading
	mdLink
	mdStrike
	mdMuted
)

type markdownRun struct {
	Text string
	Role markdownRole
}
type markdownLine []markdownRun

// Parse and lay out entirely in a background worker. No HTML execution, image
// loading, URL fetching, shell commands or terminal escape interpretation.
func markdownLines(source string, width int) []markdownLine {
	data := []byte(source)
	// YAML metadata belongs to source view, not the reading surface.
	if strings.HasPrefix(source, "---\n") {
		if end := strings.Index(source[4:], "\n---\n"); end >= 0 {
			data = []byte(source[4+end+5:])
		} else if end := strings.Index(source[4:], "\n...\n"); end >= 0 {
			data = []byte(source[4+end+5:])
		}
	}
	parser := goldmark.New(goldmark.WithExtensions(extension.GFM))
	doc := parser.Parser().Parse(mdtext.NewReader(data))
	var logical []markdownLine
	blank := func() {
		if len(logical) > 0 && len(logical[len(logical)-1]) > 0 {
			logical = append(logical, nil)
		}
	}
	var inline func(ast.Node, markdownRole, int) markdownLine
	inline = func(n ast.Node, role markdownRole, depth int) markdownLine {
		if depth > 64 {
			return markdownLine{{"… nesting limit", mdMuted}}
		}
		var runs markdownLine
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			switch node := child.(type) {
			case *ast.Text:
				value := string(node.Segment.Value(data))
				if !node.IsRaw() {
					value = html.UnescapeString(string(util.UnescapePunctuations([]byte(value))))
				}
				runs = append(runs, markdownRun{value, role})
				if node.HardLineBreak() {
					runs = append(runs, markdownRun{"\n", role})
				} else if node.SoftLineBreak() {
					runs = append(runs, markdownRun{" ", role})
				}
			case *ast.String:
				runs = append(runs, markdownRun{string(node.Value), role})
			case *ast.Emphasis:
				flag := mdItalic
				if node.Level == 2 {
					flag = mdBold
				}
				runs = append(runs, inline(child, role|flag, depth+1)...)
			case *ast.CodeSpan:
				runs = append(runs, inline(child, role|mdCode, depth+1)...)
			case *ast.Link:
				runs = append(runs, inline(child, role|mdLink, depth+1)...)
			case *ast.Image:
				runs = append(runs, markdownRun{"[Image: ", role | mdMuted})
				runs = append(runs, inline(child, role|mdMuted, depth+1)...)
				runs = append(runs, markdownRun{"]", role | mdMuted})
			case *ast.AutoLink:
				runs = append(runs, markdownRun{string(node.Label(data)), role | mdLink})
			case *extast.Strikethrough:
				runs = append(runs, inline(child, role|mdStrike, depth+1)...)
			case *extast.TaskCheckBox:
				label := "☐ "
				if node.IsChecked {
					label = "☑ "
				}
				runs = append(runs, markdownRun{label, role | mdLink})
			case *ast.RawHTML:
				// Render text around tags; never execute or display raw HTML.
			default:
				runs = append(runs, inline(child, role, depth+1)...)
			}
		}
		return runs
	}
	var block func(ast.Node, string, int)
	block = func(n ast.Node, prefix string, depth int) {
		if depth > 64 {
			logical = append(logical, markdownLine{{prefix + "… nesting limit", mdMuted}})
			return
		}
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			lead := markdownLine{}
			if prefix != "" {
				lead = append(lead, markdownRun{prefix, mdMuted})
			}
			switch node := child.(type) {
			case *ast.Heading:
				blank()
				logical = append(logical, append(lead, inline(child, mdBold|mdHeading, 0)...))
				if node.Level <= 2 {
					logical = append(logical, markdownLine{{prefix + strings.Repeat("─", max(1, min(width-len(prefix), 32))), mdMuted}})
				}
				blank()
			case *ast.Paragraph, *ast.TextBlock:
				logical = append(logical, append(lead, inline(child, 0, 0)...))
				if _, ok := node.(*ast.Paragraph); ok {
					blank()
				}
			case *ast.List:
				index := node.Start
				for item := child.FirstChild(); item != nil; item = item.NextSibling() {
					start := len(logical)
					marker := "• "
					if node.IsOrdered() {
						marker = fmt.Sprintf("%d. ", index)
						index++
					}
					block(item, prefix+strings.Repeat(" ", text.Width(marker, 4)), depth+1)
					if len(logical) > start && len(logical[start]) > 0 {
						logical[start][0].Text = prefix + marker
					}
				}
				blank()
			case *ast.Blockquote:
				block(child, prefix+"│ ", depth+1)
				blank()
			case *ast.FencedCodeBlock, *ast.CodeBlock:
				blank()
				if code, ok := node.(*ast.FencedCodeBlock); ok && len(code.Language(data)) > 0 {
					logical = append(logical, markdownLine{{prefix + "╭ " + string(code.Language(data)), mdMuted}})
				}
				for i := 0; i < child.Lines().Len(); i++ {
					segment := child.Lines().At(i)
					line := strings.TrimSuffix(string(segment.Value(data)), "\n")
					logical = append(logical, markdownLine{{prefix + expandTabs(line, 4), mdCode}})
				}
				blank()
			case *ast.ThematicBreak:
				logical = append(logical, markdownLine{{prefix + strings.Repeat("─", max(1, min(width-len(prefix), 32))), mdMuted}})
				blank()
			case *ast.HTMLBlock:
				// HTML and comments are intentionally omitted from terminal preview.
			case *extast.Table:
				blank()
				var rows []markdownTableRow
				for row := child.FirstChild(); row != nil; row = row.NextSibling() {
					_, header := row.(*extast.TableHeader)
					entry := markdownTableRow{Header: header}
					role := markdownRole(0)
					if header {
						role = mdBold
					}
					for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
						entry.Cells = append(entry.Cells, inline(cell, role, 0))
					}
					rows = append(rows, entry)
				}
				logical = append(logical, layoutMarkdownTable(rows, node.Alignments, prefix, width)...)
				blank()
			default:
				block(child, prefix, depth+1)
			}
		}
	}
	block(doc, "", 0)
	return wrapMarkdown(logical, max(1, width))
}

type markdownTableRow struct {
	Cells  []markdownLine
	Header bool
}

func markdownLineWidth(line markdownLine) int {
	width := 0
	for _, run := range line {
		width += text.Width(run.Text, 4)
	}
	return width
}

// Fit columns to the pane, then wrap each cell independently. Short label
// columns keep their natural width and leave the remaining space for prose.
func layoutMarkdownTable(rows []markdownTableRow, alignments []extast.Alignment, prefix string, width int) []markdownLine {
	count := len(alignments)
	if count == 0 {
		return nil
	}
	widths := make([]int, count)
	for col := range widths {
		widths[col] = 1
	}
	for _, row := range rows {
		for col, cell := range row.Cells {
			if col < count {
				widths[col] = max(widths[col], markdownLineWidth(cell), 1)
			}
		}
	}
	available := width - text.Width(prefix, 4) - 3*(count-1)
	// Very narrow panes (or many columns) cannot retain the column grid. Flow
	// the complete row instead; never discard columns or replace text with “…”.
	if available < count*2 {
		var lines []markdownLine
		for _, row := range rows {
			line := markdownLine{{prefix, mdMuted}}
			for col, cell := range row.Cells {
				if col > 0 {
					line = append(line, markdownRun{" │ ", mdMuted})
				}
				line = append(line, cell...)
			}
			lines = append(lines, line)
			if row.Header {
				lines = append(lines, markdownLine{{prefix + strings.Repeat("─", max(1, min(width-text.Width(prefix, 4), 32))), mdMuted}})
			}
		}
		return lines
	}
	order := make([]int, count)
	for col := range order {
		order[col] = col
	}
	sort.SliceStable(order, func(i, j int) bool { return widths[order[i]] < widths[order[j]] })
	for position, col := range order {
		widths[col] = min(widths[col], available/(count-position))
		available -= widths[col]
	}
	var lines []markdownLine
	for _, row := range rows {
		wrapped := make([][]markdownLine, count)
		height := 1
		for col := range widths {
			var cell markdownLine
			if col < len(row.Cells) {
				cell = row.Cells[col]
			}
			wrapped[col] = wrapMarkdown([]markdownLine{cell}, widths[col])
			height = max(height, len(wrapped[col]))
		}
		for y := 0; y < height; y++ {
			line := markdownLine{{prefix, mdMuted}}
			for col, w := range widths {
				if col > 0 {
					line = append(line, markdownRun{" │ ", mdMuted})
				}
				var cell markdownLine
				if y < len(wrapped[col]) {
					cell = wrapped[col][y]
				}
				padding := max(0, w-markdownLineWidth(cell))
				left := 0
				switch alignments[col] {
				case extast.AlignRight:
					left = padding
				case extast.AlignCenter:
					left = padding / 2
				}
				if left > 0 {
					line = append(line, markdownRun{strings.Repeat(" ", left), 0})
				}
				line = append(line, cell...)
				if padding > left {
					line = append(line, markdownRun{strings.Repeat(" ", padding-left), 0})
				}
			}
			lines = append(lines, line)
		}
		if row.Header {
			lines = append(lines, markdownLine{{prefix + strings.Repeat("─", max(1, min(width-text.Width(prefix, 4), 32))), mdMuted}})
		}
	}
	return lines
}

func wrapMarkdown(lines []markdownLine, width int) []markdownLine {
	type glyph struct {
		text  string
		role  markdownRole
		width int
	}
	var output []markdownLine
	for _, line := range lines {
		var row markdownLine
		column := 0
		var word []glyph
		wordWidth := 0
		appendText := func(s string, role markdownRole) {
			if len(row) > 0 && row[len(row)-1].Role == role {
				row[len(row)-1].Text += s
			} else {
				row = append(row, markdownRun{s, role})
			}
		}
		flushRow := func() { output = append(output, row); row = nil; column = 0 }
		appendGlyph := func(g glyph) {
			if column+g.width > width {
				flushRow()
			}
			if g.width > width {
				g.text = "�"
				g.width = 1
			}
			appendText(g.text, g.role)
			column += g.width
		}
		flushWord := func() {
			if column > 0 && wordWidth <= width && column+wordWidth > width {
				flushRow()
			}
			for _, g := range word {
				appendGlyph(g)
			}
			word = word[:0]
			wordWidth = 0
		}
		for _, run := range line {
			g := uniseg.NewGraphemes(run.Text)
			for g.Next() {
				s := g.Str()
				if s == "\n" {
					flushWord()
					flushRow()
					continue
				}
				if run.Role&mdCode != 0 {
					flushWord()
				}
				// Remove terminal controls, including OSC/CSI starts and bidi
				// overrides. Preserve combining and ZWJ graphemes unchanged.
				s = strings.Map(func(r rune) rune {
					if unicode.IsControl(r) || r >= 0x202a && r <= 0x202e || r >= 0x2066 && r <= 0x2069 {
						return ' '
					}
					return r
				}, s)
				item := glyph{s, run.Role, max(1, uniseg.StringWidth(s))}
				if run.Role&mdCode != 0 {
					appendGlyph(item)
				} else if strings.TrimSpace(s) == "" {
					flushWord()
					if column < width {
						appendGlyph(item)
					}
				} else {
					word = append(word, item)
					wordWidth += item.width
				}
			}
		}
		flushWord()
		output = append(output, row)
	}
	return output
}
