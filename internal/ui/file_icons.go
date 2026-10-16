package ui

import (
	"path/filepath"
	"strings"

	"sunbook/internal/text"

	"github.com/gdamore/tcell/v3"
)

const fileIconWidth = 4 // Maximum: three display cells for the mark and one gap.

type fileIconSpec struct {
	mark  string
	color tcell.Color
}

// Extensions for the same language share a mark; different languages keep
// distinct marks and colors. Plain characters work with ordinary terminal fonts.
var fileLanguageIcons = func() map[string]fileIconSpec {
	icons := map[string]fileIconSpec{}
	for _, r := range []struct {
		extensions, mark string
		color            int32
	}{
		{".go", "Go", 0x79d5e2},
		{".py .pyi .pyw", "Py", 0xe6c678},
		{".js .mjs .cjs", "JS", 0xe3cd71},
		{".jsx", "JSX", 0x86cfe3},
		{".ts .mts .cts", "TS", 0x89b4ed},
		{".tsx", "TSX", 0x8dbeee},
		{".rs", "Rs", 0xd9ac90},
		{".c .h", "C", 0x9caed6},
		{".cpp .cc .cxx .hpp .hh .hxx .h++ .c++", "C++", 0x88b5e1},
		{".cs .csx", "C#", 0xb8a0de},
		{".java", "Jav", 0xe8ad82},
		{".kt .kts", "Kt", 0xc0a0e0},
		{".swift", "Sw", 0xe8a18a},
		{".rb .rake .gemspec", "Rb", 0xe297a0},
		{".php .phtml", "PHP", 0xb5ace1},
		{".lua", "Lua", 0xa0b0e3},
		{".dart", "Da", 0x86cbd7},
		{".scala .sc", "Sc", 0xdb9da5},
		{".r .rmd", "R", 0x99b6de},
		{".jl", "Jl", 0xb3be95},
		{".ex .exs", "Ex", 0xbfa4d9},
		{".erl .hrl", "Er", 0xdca1b8},
		{".hs .lhs", "Hs", 0xb9a6dd},
		{".fs .fsi .fsx", "F#", 0x8fbbdb},
		{".clj .cljs .cljc .edn", "Clj", 0xa1c49b},
		{".ml .mli", "ML", 0xdcb27f},
		{".pl .pm", "Pl", 0x9fbde2},
		{".zig", "Zig", 0xe1bd85},
		{".sol", "Sol", 0xb3b7d8},
		{".html .htm", "HTM", 0xe5a38e},
		{".xml .xsd .xsl", "XML", 0xdab586},
		{".vue", "Vue", 0x92c6a4},
		{".svelte", "Sve", 0xe5a18d},
		{".astro", "Ast", 0xc1a4df},
		{".css", "CSS", 0x98b7e8},
		{".scss .sass", "Sas", 0xdca6c5},
		{".less", "Les", 0x9aafdb},
		{".json .jsonc .jsonl", "{}", 0xe1c17e},
		{".yaml .yml", "Yml", 0xd8a2b2},
		{".toml", "Tom", 0xd4ae9a},
		{".ini .conf .config .properties", "INI", 0xb7becb},
		{".md .markdown", "Md", 0x9fbcd8},
		{".mdx", "MDX", 0xd5c18a},
		{".rst", "Rst", 0xb3bed0},
		{".txt .text", "Txt", 0xb7becb},
		{".sh .bash", "Sh", 0xa4c899},
		{".zsh", "Zsh", 0xa7c5aa},
		{".fish", "Fsh", 0x99c5bd},
		{".ps1 .psm1 .psd1", "PS", 0x95b6e5},
		{".bat .cmd", "Bat", 0xb4c495},
		{".sql", "SQL", 0xe0c182},
		{".graphql .gql", "GQL", 0xdba8ca},
		{".proto", "PB", 0xb2c8a1},
		{".tf .tfvars .hcl", "Tf", 0xb7a4df},
		{".nix", "Nix", 0x9dbbe2},
	} {
		icon := fileIconSpec{r.mark, tcell.NewHexColor(r.color)}
		for _, ext := range strings.Fields(r.extensions) {
			icons[ext] = icon
		}
	}
	return icons
}()

func (a *App) fileIcon(path string, folder, expanded bool) (string, tcell.Color) {
	if folder {
		if expanded {
			return "▱", a.colors.folder
		}
		return "▰", a.colors.folder
	}
	name := strings.ToLower(filepath.Base(path))
	switch name {
	case "dockerfile", "containerfile":
		return "Dkr", a.colors.folder
	case "makefile", "gnumakefile":
		return "Mk", a.accent.GetForeground()
	case "justfile":
		return "Jst", a.accent.GetForeground()
	case "taskfile.yml", "taskfile.yaml":
		return "Tsk", a.accent.GetForeground()
	case "go.mod", "go.sum":
		return "Go", fileLanguageIcons[".go"].color
	case ".gitignore", ".gitattributes", ".gitmodules":
		return "◇", a.colors.danger
	case "license", "licence", "copying":
		return "§", a.colors.muted
	}
	if name == ".env" || strings.HasPrefix(name, ".env.") {
		return "Env", a.colors.success
	}
	if strings.HasSuffix(name, ".d.ts") || strings.HasSuffix(name, ".d.mts") || strings.HasSuffix(name, ".d.cts") {
		return "dTS", fileLanguageIcons[".ts"].color
	}
	if icon, ok := fileLanguageIcons[filepath.Ext(name)]; ok {
		return icon.mark, icon.color
	}
	switch filepath.Ext(name) {
	case ".svg":
		return "SVG", a.colors.success
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico", ".bmp":
		return "▧", a.colors.success
	case ".mp3", ".wav", ".flac", ".ogg", ".m4a":
		return "♫", a.colors.folder
	case ".mp4", ".mov", ".mkv", ".webm":
		return "▷", a.colors.folder
	case ".zip", ".tar", ".gz", ".xz", ".bz2", ".7z":
		return "▣", a.colors.danger
	case ".db", ".sqlite", ".sqlite3":
		return "DB", a.accent.GetForeground()
	case ".pdf":
		return "PDF", a.colors.danger
	}
	return "▤", a.colors.muted
}
func (a *App) fileIconSpan(path string, folder, expanded bool) int {
	glyph, _ := a.fileIcon(path, folder, expanded)
	return min(fileIconWidth, text.Width(glyph, 4)+1)
}
func (a *App) drawFileIcon(x, y, width int, path string, folder, expanded bool, style tcell.Style) int {
	if width <= 0 {
		return 0
	}
	glyph, color := a.fileIcon(path, folder, expanded)
	a.put(x, y, min(fileIconWidth-1, width), glyph, style.Foreground(color))
	return min(fileIconWidth, text.Width(glyph, 4)+1, width)
}
