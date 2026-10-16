package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

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
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	settings := map[string]json.RawMessage{}
	mode := os.FileMode(0600)
	data, err := os.ReadFile(path)
	if err == nil {
		if err = json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("read config: %w", err)
		}
		if info, err := os.Stat(path); err == nil {
			mode = info.Mode().Perm()
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if settings == nil {
		settings = map[string]json.RawMessage{}
	}
	settings["theme"], _ = json.Marshal(struct {
		Name string `json:"name"`
	}{name})
	data, err = json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".sunbook-config-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = f.Chmod(mode); err != nil {
		return err
	}
	if _, err = f.Write(append(data, '\n')); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
