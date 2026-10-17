package ui

import (
	"sunbook/internal/editor"
	"sunbook/internal/lsp"
)

type diagnosticSpan struct{ start, end, first, last int }
type diagnosticRow struct {
	spans  []diagnosticSpan
	marker bool
}
type diagnosticCache struct {
	version int
	source  []lsp.Diagnostic
	spans   []diagnosticSpan
	rows    map[int]diagnosticRow
}

func (a *App) cachedDiagnostics(d *editor.Document) *diagnosticCache {
	if a.diagnosticCache == nil {
		a.diagnosticCache = make(map[*editor.Document]*diagnosticCache)
	}
	source := a.diagnostics[lsp.URI(d.Path)].Diagnostics
	c := a.diagnosticCache[d]
	same := c != nil && c.version == d.Buffer.Version && len(c.source) == len(source)
	if same && len(source) > 0 {
		same = &c.source[0] == &source[0]
	}
	if same {
		return c
	}
	c = &diagnosticCache{version: d.Buffer.Version, source: source, rows: make(map[int]diagnosticRow)}
	for _, diag := range source {
		c.spans = append(c.spans, diagnosticSpan{start: lsp.Offset(d.Buffer, diag.Range.Start), end: lsp.Offset(d.Buffer, diag.Range.End), first: diag.Range.Start.Line, last: diag.Range.End.Line})
	}
	a.diagnosticCache[d] = c
	return c
}

func (c *diagnosticCache) row(row int) diagnosticRow {
	if r, ok := c.rows[row]; ok {
		return r
	}
	// Bound the lazy row index, including documents with very long diagnostics.
	if len(c.rows) >= 256 {
		clear(c.rows)
	}
	var r diagnosticRow
	for _, span := range c.spans {
		if span.first == row {
			r.marker = true
		}
		if span.first <= row && row <= span.last {
			r.spans = append(r.spans, span)
		}
	}
	c.rows[row] = r
	return r
}
