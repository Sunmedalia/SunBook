package ui

import (
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/gdamore/tcell/v3"
)

// Tokenize a complete snapshot so multiline comments and strings retain their
// context. Editor and comparison workers share the same language and palette.
func syntaxSpans(path, source string, base tcell.Style, syntax string) []tokenSpan {
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
	var spans []tokenSpan
	offset := 0
	for token := iterator(); token != chroma.EOF; token = iterator() {
		style := base
		entry := theme.Get(token.Type)
		if entry.Colour.IsSet() {
			style = style.Foreground(tcell.NewRGBColor(int32(entry.Colour.Red()), int32(entry.Colour.Green()), int32(entry.Colour.Blue())))
		}
		if terminalPalette {
			style = base.Foreground(terminalTokenColor(token.Type))
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
