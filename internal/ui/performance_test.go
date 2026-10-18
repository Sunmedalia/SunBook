package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v3"
	"sunbook/internal/lsp"
	"sunbook/internal/text"
)

func BenchmarkDiagnosticRendering(b *testing.B) {
	a, _ := testApp(b)
	d := a.view().Doc
	d.Path = "benchmark.go"
	d.Buffer = text.New(strings.Repeat("var 中文 = 123 // 👩‍💻\n", 1000))
	diagnostics := make([]lsp.Diagnostic, 1000)
	for i := range diagnostics {
		diagnostics[i] = lsp.Diagnostic{Range: lsp.Range{Start: lsp.Position{Line: i, Character: 4}, End: lsp.Position{Line: i, Character: 10}}, Message: fmt.Sprint(i)}
	}
	a.diagnostics[lsp.URI(d.Path)] = lsp.Diagnostics{Diagnostics: diagnostics}
	a.draw()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		a.draw()
	}
}

func BenchmarkTerminalOutput(b *testing.B) {
	a, _ := testApp(b)
	a.cfg.Shell = "/bin/sh"
	a.showTerminal()
	a.draw()
	data := []byte(strings.Repeat("build output 中文 1234567890\r\n", 32))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		a.term.Feed(data)
		a.draw()
	}
}

func BenchmarkSyntaxHighlight(b *testing.B) {
	unit := "// 中文 😀 comment\nfunc example(x int) string {\n\tif x > 10 {\n\t\treturn fmt.Sprintf(\"%d items\", x)\n\t}\n\treturn \"none\"\n}\n\n"
	for _, size := range []struct {
		name   string
		copies int
	}{{"4KB", 33}, {"120KB", 1000}} {
		source := strings.Repeat(unit, size.copies)
		b.Run(size.name, func(b *testing.B) {
			b.SetBytes(int64(len(source)))
			b.ReportAllocs()
			for range b.N {
				syntaxSpans("benchmark.go", source, tcell.StyleDefault, "github-dark")
			}
		})
	}
}

func BenchmarkMultiCursorRendering(b *testing.B) {
	a, _ := testApp(b)
	v := a.view()
	v.Doc.Buffer = text.New(strings.Repeat("value := compute(中文, 😀) // trailing comment\n", 5000))
	// One caret per line, as after "select all occurrences" in a large file.
	v.Selections = nil
	for row := range v.Doc.Buffer.LineCount() {
		p := v.Doc.Buffer.LineStart(row)
		v.Selections = append(v.Selections, text.Selection{Anchor: p, Head: p + 5})
	}
	v.Top = 2500
	a.draw()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		a.draw()
	}
}
