package config

import "fmt"

type ThemePreset struct {
	Label, Description string
	Theme              Theme
}

func ThemePresets() []ThemePreset {
	return []ThemePreset{
		{"SunBook", "Dark · slate & gold", Theme{Name: "sunbook", Background: "#161e2b", Foreground: "#d4deea", Accent: "#e2b66f", Syntax: "github-dark"}},
		{"Tokyo Night", "Dark · blue & violet", Theme{Name: "tokyo-night", Background: "#1a1b26", Foreground: "#c0caf5", Accent: "#7aa2f7", Syntax: "tokyonight-night"}},
		{"Dracula", "Dark · purple & pink", Theme{Name: "dracula", Background: "#282a36", Foreground: "#f8f8f2", Accent: "#bd93f9", Syntax: "dracula"}},
		{"Nord", "Dark · arctic blue", Theme{Name: "nord", Background: "#2e3440", Foreground: "#eceff4", Accent: "#88c0d0", Syntax: "nord"}},
		{"Gruvbox Dark", "Dark · warm earth", Theme{Name: "gruvbox-dark", Background: "#282828", Foreground: "#ebdbb2", Accent: "#fabd2f", Syntax: "gruvbox"}},
		{"GitHub Light", "Light · white & blue", Theme{Name: "github-light", Background: "#ffffff", Foreground: "#24292e", Accent: "#0969da", Syntax: "github"}},
		{"Auto", "Follow terminal colors · adapt light / dark", Theme{Name: "auto", Background: "#161e2b", Foreground: "#d4deea", Accent: "#7aa2f7", Syntax: "github-dark"}},
		{"Terminal Default", "No background · terminal foreground and ANSI palette", Theme{Name: "terminal-default", Background: "#161e2b", Foreground: "#d4deea", Accent: "#7aa2f7", Syntax: "github-dark"}},
	}
}

func FindTheme(name string) (ThemePreset, bool) {
	for _, preset := range ThemePresets() {
		if preset.Theme.Name == name {
			return preset, true
		}
	}
	return ThemePreset{}, false
}

// SaveTheme changes only the theme selection, retaining unrelated settings and
// unknown keys. Write atomically so an interrupted save cannot corrupt config.
func SaveTheme(path, name string) error {
	if _, ok := FindTheme(name); !ok {
		return fmt.Errorf("unknown theme %q", name)
	}
	return saveSettings(path, map[string]any{"theme": map[string]string{"name": name}})
}
