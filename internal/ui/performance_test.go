package ui

import (
	"fmt"
	"strings"
	"testing"

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
