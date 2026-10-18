package ui

import (
	"fmt"
	"path/filepath"
	"sort"
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

// A 50k-file workspace index, filtered one keystroke at a time.
func benchmarkWorkspaceFiles(root string) []string {
	files := make([]string, 0, 50000)
	for i := range 50000 {
		files = append(files, filepath.Join(root, fmt.Sprintf("pkg%03d", i%500), fmt.Sprintf("module_%d", i%37), fmt.Sprintf("file_%05d.go", i)))
	}
	files = append(files, filepath.Join(root, "internal", "ui", "render.go"))
	sort.Strings(files)
	return files
}

func BenchmarkExplorerFilterTyping(b *testing.B) {
	a, _ := testApp(b)
	a.setTreeIndex(newFileIndex(a.root, benchmarkWorkspaceFiles(a.root)))
	query := "internal/ui/render"
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		for i := 1; i <= len(query); i++ {
			a.setTreeQuery(query[:i])
		}
		a.setTreeQuery("")
	}
}

func BenchmarkQuickOpenTyping(b *testing.B) {
	a, _ := testApp(b)
	a.setTreeIndex(newFileIndex(a.root, benchmarkWorkspaceFiles(a.root)))
	query := "uirender"
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		a.quickOpen()
		m := a.modal
		for i := 1; i <= len(query); i++ {
			m.Input = query[:i]
			a.updateOpenModal(m)
		}
		a.modal = nil
	}
}
