package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadNamedThemesAndColorOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	for _, preset := range ThemePresets() {
		t.Run(preset.Theme.Name, func(t *testing.T) {
			data, _ := json.Marshal(map[string]any{"theme": map[string]string{"name": preset.Theme.Name}})
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if err != nil || cfg.Theme != preset.Theme || cfg.FilePath != path {
				t.Fatal("named theme did not replace defaults", cfg.Theme, err)
			}
		})
	}
	for _, input := range []string{
		`{"theme":{"accent":"#123456"}}`,
		`{"theme":{"name":"dracula","accent":"#123456"}}`,
	} {
		os.WriteFile(path, []byte(input), 0600)
		cfg, err := Load(path)
		if err != nil || cfg.Theme.Accent != "#123456" || cfg.Theme.Background == "" || cfg.Theme.Syntax == "" {
			t.Fatal("partial custom theme lost defaults", cfg.Theme, err)
		}
		if cfg.Theme.Name == "dracula" && cfg.Theme.Background != "#282a36" {
			t.Fatal("custom color erased preset background")
		}
	}
	os.WriteFile(path, []byte(`{"theme":{"name":"missing"}}`), 0600)
	if _, err := Load(path); err == nil {
		t.Fatal("unknown theme accepted")
	}
}

func TestThemeSelectionPersistsWithoutChangingOtherSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	input := `{"tabSize":2,"shell":"/bin/sh","keybindings":{"f8":"language.format"},"servers":{"go":{"command":"custom-gopls"}},"future":{"enabled":true},"theme":{"accent":"#123456"}}`
	if err := os.WriteFile(path, []byte(input), 0640); err != nil {
		t.Fatal(err)
	}
	if err := SaveTheme(path, "github-light"); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil || cfg.Theme.Name != "github-light" || cfg.Theme.Background != "#ffffff" || cfg.TabSize != 2 || cfg.Servers["go"].Command != "custom-gopls" || cfg.Keybindings["f8"] != "language.format" {
		t.Fatal("theme save changed unrelated settings or retained overrides", cfg, err)
	}
	data, _ := os.ReadFile(path)
	var values map[string]json.RawMessage
	json.Unmarshal(data, &values)
	if string(values["future"]) == "" {
		t.Fatal("save dropped unknown config keys")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0640 {
		t.Fatal("save changed file permissions", info.Mode())
	}
	missing := filepath.Join(t.TempDir(), "SunBook", "config.json")
	if err := SaveTheme(missing, "nord"); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(missing)
	if err != nil || cfg.Theme.Name != "nord" || cfg.TabSize != Default().TabSize {
		t.Fatal("first theme selection did not create a usable config", err)
	}
}

func TestThemeSaveDoesNotOverwriteInvalidConfigAndFollowsSymlink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	bad := []byte(`{"unfinished":`)
	os.WriteFile(path, bad, 0600)
	if err := SaveTheme(path, "nord"); err == nil {
		t.Fatal("invalid config was overwritten")
	}
	data, _ := os.ReadFile(path)
	if string(data) != string(bad) {
		t.Fatal("failed save changed invalid config")
	}
	os.WriteFile(path, []byte(`{}`), 0600)
	link := filepath.Join(dir, "linked.json")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if err := SaveTheme(link, "dracula"); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Lstat(link)
	cfg, err := Load(path)
	if info.Mode()&os.ModeSymlink == 0 || err != nil || cfg.Theme.Name != "dracula" {
		t.Fatal("saving through a config symlink broke its target", err)
	}
}
