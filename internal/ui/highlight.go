package ui

import (
	"path/filepath"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/gdamore/tcell/v3"
	"sunbook/internal/text"
)

// Tokenize a complete snapshot so multiline comments and strings retain their
// context. Editor and comparison workers share the same language and palette.
func syntaxSpans(path, source string, base tcell.Style, syntax string) []tokenSpan {
	if len(source) == 0 || len(source) > 2*1024*1024 {
		return nil
	}
	iterator, err := chroma.Coalesce(matchLexer(path)).Tokenise(nil, source)
	if err != nil {
		return nil
	}
	if syntax == "" {
		syntax = "github-dark"
	}
	terminalPalette := syntax == "terminal-dark" || syntax == "terminal-light"
	if terminalPalette {
		light := syntax == "terminal-light"
		syntax = "github-dark"
		if light {
			syntax = "github"
		}
	}
	theme := styles.Get(syntax)
	if theme == nil {
		theme = styles.Fallback
	}
	// A file has few token types, so resolve each one's style once.
	resolved := map[chroma.TokenType]tcell.Style{}
	tokenStyle := func(token chroma.TokenType) tcell.Style {
		if style, ok := resolved[token]; ok {
			return style
		}
		style := base
		entry := theme.Get(token)
		if entry.Colour.IsSet() {
			style = style.Foreground(tcell.NewRGBColor(int32(entry.Colour.Red()), int32(entry.Colour.Green()), int32(entry.Colour.Blue())))
		}
		if terminalPalette {
			style = base.Foreground(terminalTokenColor(token))
		}
		if entry.Bold == chroma.Yes {
			style = style.Bold(true)
		}
		if entry.Italic == chroma.Yes {
			style = style.Italic(true)
		}
		resolved[token] = style
		return style
	}
	var spans []tokenSpan
	offset := 0
	for token := iterator(); token != chroma.EOF; token = iterator() {
		spans = append(spans, tokenSpan{offset, offset + len(token.Value), tokenStyle(token.Type)})
		offset += len(token.Value)
	}
	return spans
}

// lexers.Match tests every registered filename glob, which costs milliseconds;
// its result depends only on the base name.
var lexerCache struct {
	sync.Mutex
	byName map[string]chroma.Lexer
}

func matchLexer(path string) chroma.Lexer {
	name := filepath.Base(path)
	lexerCache.Lock()
	lex, ok := lexerCache.byName[name]
	lexerCache.Unlock()
	if ok {
		return lex
	}
	lex = lexers.Match(name)
	if lex == nil {
		lex = lexers.Fallback
	}
	lexerCache.Lock()
	if lexerCache.byName == nil || len(lexerCache.byName) >= 512 {
		lexerCache.byName = map[string]chroma.Lexer{}
	}
	lexerCache.byName[name] = lex
	lexerCache.Unlock()
	return lex
}

// shiftSpans moves highlight spans through one transaction's edits so the
// previous colours stay visible until the new snapshot is tokenized. Offsets
// map like mapOffset; spans removed entirely by an edit are dropped.
func shiftSpans(spans []tokenSpan, edits []text.Edit) []tokenSpan {
	m := offsetMapper{edits: edits}
	out := spans[:0]
	for _, span := range spans {
		span.Start, span.End = m.at(span.Start), m.at(span.End)
		if span.End > span.Start {
			out = append(out, span)
		}
	}
	return out
}

// offsetMapper maps non-decreasing offsets through sorted edits in one pass.
type offsetMapper struct {
	edits    []text.Edit
	i, delta int
}

func (m *offsetMapper) at(p int) int {
	for m.i < len(m.edits) && m.edits[m.i].End < p {
		e := m.edits[m.i]
		m.delta += len(e.Text) - (e.End - e.Start)
		m.i++
	}
	if m.i < len(m.edits) && p >= m.edits[m.i].Start {
		e := m.edits[m.i]
		return e.Start + m.delta + len(e.Text)
	}
	return p + m.delta
}

func terminalTokenColor(token chroma.TokenType) tcell.Color {
	switch {
	case token.InCategory(chroma.Comment):
		return tcell.ColorGray
	case token.InCategory(chroma.Keyword):
		return tcell.ColorPurple
	case token.InSubCategory(chroma.LiteralString):
		return tcell.ColorGreen
	case token.InSubCategory(chroma.LiteralNumber):
		return tcell.ColorTeal
	case token == chroma.NameFunction || token == chroma.NameBuiltin:
		return tcell.ColorBlue
	case token == chroma.NameClass || token == chroma.NameNamespace:
		return tcell.ColorOlive
	case token.InCategory(chroma.Operator):
		return tcell.ColorTeal
	case token.InCategory(chroma.Error):
		return tcell.ColorRed
	default:
		return tcell.ColorDefault
	}
}
