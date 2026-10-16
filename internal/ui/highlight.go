package ui

import (
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/gdamore/tcell/v3"
)

// Tokenize a complete snapshot so multiline comments and strings retain their
// context. Editor and comparison workers share the same language and palette.
func syntaxSpans(path, source string, base tcell.Style) []tokenSpan {
	if len(source) == 0 || len(source) > 2*1024*1024 {
		return nil
	}
	lex := lexers.Match(path)
	if lex == nil {
		lex = lexers.Fallback
	}
	iterator, err := chroma.Coalesce(lex).Tokenise(nil, source)
	if err != nil {
		return nil
	}
	theme := styles.Get("github-dark")
	if theme == nil {
		theme = styles.Fallback
	}
	var spans []tokenSpan
	offset := 0
	for token := iterator(); token != chroma.EOF; token = iterator() {
		style := base
		entry := theme.Get(token.Type)
		if entry.Colour.IsSet() {
			style = style.Foreground(tcell.NewRGBColor(int32(entry.Colour.Red()), int32(entry.Colour.Green()), int32(entry.Colour.Blue())))
		}
		if entry.Bold == chroma.Yes {
			style = style.Bold(true)
		}
		if entry.Italic == chroma.Yes {
			style = style.Italic(true)
		}
		spans = append(spans, tokenSpan{offset, offset + len(token.Value), style})
		offset += len(token.Value)
	}
	return spans
}
