package ui

import (
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v3"
)

// Keep the original marks intact in classicFileIcon. Both themes share the
// language colors and the same measured icon slot in every file list and tab.
var expressiveLanguageMarks = map[string]string{
	"Go": "🐹", "Py": "🐍", "Rs": "🦀", "Jav": "☕", "Rb": "💎", "PHP": "🐘",
	"Sw": "🐦", "Lua": "🌙", "Pl": "🐪", "JS": "⚡", "TS": "🔷", "dTS": "d🔷",
	"JSX": "⚛️", "TSX": "⚛️T", "Kt": "🟣", "C#": "♯", "Da": "🎯", "Ex": "💧",
	"HTM": "🌐", "XML": "◇", "Vue": "💚", "Sve": "🔥", "Ast": "🚀",
	"CSS": "🎨", "Sas": "🌸", "Les": "◈", "{}": "🧩", "Yml": "⚙Y", "Tom": "⚙T",
	"INI": "⚙", "Md": "📝", "MDX": "📘", "Rst": "¶", "Txt": "📄", "Sh": "🐚",
	"Zsh": "🐚", "Fsh": "🐟", "PS": "⌘", "Bat": "▣", "SQL": "🗃️", "GQL": "◈",
	"PB": "📡", "Tf": "☁️", "Nix": "❄️", "Env": "🔐", "Dkr": "🐳", "Mk": "🛠️",
	"Jst": "🛠️", "Tsk": "🛠️", "Sc": "🔴", "Er": "♦", "Hs": "λ", "F#": "♯",
	"Clj": "◉", "ML": "🐫", "Jl": "🟢", "Zig": "↯", "Sol": "◆",
}

func (a *App) fileIcon(path string, folder, expanded bool) (string, tcell.Color) {
	mark, color := a.classicFileIcon(path, folder, expanded)
	if a.cfg.FileIcons == "classic" {
		return mark, color
	}
	if folder {
		if expanded {
			return "📂", color
		}
		return "📁", color
	}
	name := strings.ToLower(filepath.Base(path))
	switch {
	case name == "cargo.toml":
		return "🦀", fileLanguageIcons[".rs"].color
	case name == "pyproject.toml" || name == "requirements.txt" || name == "pipfile" || name == "setup.cfg":
		return "🐍", fileLanguageIcons[".py"].color
	case name == "tsconfig.json" || strings.HasPrefix(name, "tsconfig."):
		return "🔷", fileLanguageIcons[".ts"].color
	case name == "jsconfig.json":
		return "⚡", fileLanguageIcons[".js"].color
	case name == "package-lock.json" || name == "pnpm-lock.yaml" || name == "npm-shrinkwrap.json" || strings.HasSuffix(name, ".lock"):
		return "🔒", a.accent.GetForeground()
	case name == "package.json":
		return "📦", a.colors.success
	case strings.HasPrefix(name, "dockerfile.") || name == "docker-compose.yml" || name == "docker-compose.yaml" || name == "compose.yml" || name == "compose.yaml":
		return "🐳", a.colors.folder
	case strings.HasPrefix(name, "readme"):
		return "📖", a.colors.folder
	case strings.HasPrefix(name, "changelog"):
		return "📋", a.colors.folder
	case name == "license" || name == "licence" || name == "copying" || strings.HasPrefix(name, "license."):
		return "📜", a.colors.muted
	case name == ".gitignore" || name == ".gitattributes" || name == ".gitmodules":
		return "🌿", a.colors.danger
	}
	if pictogram, ok := expressiveLanguageMarks[mark]; ok {
		return pictogram, color
	}
	switch filepath.Ext(name) {
	case ".ipynb":
		return "📓", fileLanguageIcons[".py"].color
	case ".svg":
		return "🎨", color
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico", ".bmp", ".avif":
		return "🖼️", a.colors.success
	case ".mp3", ".wav", ".flac", ".ogg", ".m4a", ".aac":
		return "🎵", a.colors.folder
	case ".mp4", ".mov", ".mkv", ".webm", ".avi":
		return "🎬", a.colors.folder
	case ".zip", ".tar", ".gz", ".xz", ".bz2", ".7z", ".rar":
		return "📦", a.colors.danger
	case ".db", ".sqlite", ".sqlite3":
		return "🗃️", a.accent.GetForeground()
	case ".pdf":
		return "📕", a.colors.danger
	case ".csv", ".tsv", ".xlsx", ".xls", ".ods":
		return "📊", a.colors.success
	case ".doc", ".docx", ".odt", ".rtf":
		return "📝", a.colors.folder
	case ".log":
		return "🧾", a.colors.muted
	case ".ttf", ".otf", ".woff", ".woff2":
		return "Aa", a.colors.muted
	case ".diff", ".patch":
		return "⇄", a.accent.GetForeground()
	}
	if mark == "▤" {
		return "📄", color
	}
	return mark, color
}
