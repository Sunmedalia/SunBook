package ui

import (
	"math/rand/v2"
	"testing"

	"github.com/gdamore/tcell/v3"
	"sunbook/internal/text"
)

func TestShiftSpansMatchesMapOffset(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 8))
	pieces := []string{"", "x", "\n", "中文", "😀", "ab\ncd"}
	for seed := range 300 {
		b := text.New("package main\n\nfunc main() { println(\"中文 😀\") }\n// end\n")
		for range 30 {
			// Tokens cover the whole snapshot, as chroma's do.
			var spans []tokenSpan
			for at, s := 0, b.String(); at < len(s); {
				end := min(len(s), at+1+rng.IntN(6))
				for end < len(s) && !utf8Start(s[end]) {
					end++
				}
				spans = append(spans, tokenSpan{at, end, tcell.StyleDefault.Bold(len(spans)%2 == 0)})
				at = end
			}
			s := b.String()
			changed := false
			switch rng.IntN(5) {
			case 0:
				_, changed = b.Undo()
			case 1:
				_, changed = b.Redo()
			default:
				var edits []text.Edit
				for at := 0; at < len(s) && len(edits) < 3; {
					start := at + rng.IntN(len(s)-at+1)
					end := min(len(s), start+rng.IntN(5))
					for start < len(s) && !utf8Start(s[start]) {
						start++
					}
					end = max(end, start)
					for end < len(s) && !utf8Start(s[end]) {
						end++
					}
					edits = append(edits, text.Edit{Start: start, End: end, Text: pieces[rng.IntN(len(pieces))]})
					at = end + 1
				}
				changed = b.Apply(edits, nil, nil)
			}
			if !changed {
				continue
			}
			var want []tokenSpan
			for _, span := range spans {
				span.Start, span.End = mapOffset(span.Start, b.LastEdits), mapOffset(span.End, b.LastEdits)
				if span.End > span.Start {
					want = append(want, span)
				}
			}
			got := shiftSpans(append([]tokenSpan(nil), spans...), b.LastEdits)
			if len(got) != len(want) {
				t.Fatalf("seed %d: %d spans, want %d", seed, len(got), len(want))
			}
			for i := range got {
				if got[i] != want[i] || got[i].End > b.Len() || i > 0 && got[i].Start < got[i-1].End {
					t.Fatalf("seed %d span %d: %+v, want %+v", seed, i, got[i], want[i])
				}
			}
		}
	}
}

func utf8Start(c byte) bool { return c&0xc0 != 0x80 }

func TestEditKeepsShiftedHighlightUntilRetokenized(t *testing.T) {
	a, _ := testApp(t)
	v := a.view()
	d := v.Doc
	d.Path = "main.go"
	insertSource(a, "package main\n\nfunc main() {}\n")
	spans := syntaxSpans(d.Path, d.Buffer.String(), a.base, a.themeSyntax)
	a.highlight[d] = highlights{d.Buffer.Version, spans}
	keyword := spans[0].Style
	v.Selections = []text.Selection{{Anchor: 0, Head: 0}}
	if !d.Buffer.Apply([]text.Edit{{Start: 0, End: 0, Text: "// note\n"}}, v.Selections, v.Selections) {
		t.Fatal("edit")
	}
	a.changed(d)
	hl := a.highlight[d]
	if hl.Version != d.Buffer.Version || len(hl.Spans) == 0 {
		t.Fatal("edit discarded the previous highlighting", hl.Version, d.Buffer.Version)
	}
	// "package" moved down one line but keeps its keyword colour.
	at := len("// note\n")
	found := false
	for _, span := range hl.Spans {
		if span.Start == at && span.End == at+len("package") && span.Style == keyword {
			found = true
		}
	}
	if !found {
		t.Fatal("keyword span was not shifted", hl.Spans)
	}
	// A reload is a new buffer without edits; stale colours must not be reused.
	fresh := text.New("x := 1\n")
	fresh.Version = d.Buffer.Version + 1
	d.Buffer = fresh
	a.changed(d)
	if a.highlight[d].Version == d.Buffer.Version {
		t.Fatal("reloaded buffer reused shifted spans")
	}
}
