package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestIconSettingsPreserveThemeAndUnrelatedValuesAcrossReloads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	initial := `{"tabSize":2,"shell":"/bin/sh","future":{"enabled":true},"theme":{"name":"nord","accent":"#123456"}}`
	if err := os.WriteFile(path, []byte(initial), 0640); err != nil {
		t.Fatal(err)
	}
	for _, files := range []string{"classic", "expressive"} {
		for _, buttons := range []string{"unicode", "nerd"} {
			if err := SaveIcons(path, files, buttons); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if err != nil || cfg.FileIcons != files || cfg.ActivityIcons != buttons || cfg.TabSize != 2 || cfg.Shell != "/bin/sh" || cfg.Theme.Name != "nord" || cfg.Theme.Accent != "#123456" {
				t.Fatal("icons did not persist independently of other settings", cfg, err)
			}
		}
	}
	if err := SaveTheme(path, "github-light"); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil || cfg.FileIcons != "expressive" || cfg.ActivityIcons != "nerd" || cfg.Theme.Name != "github-light" {
		t.Fatal("theme save replaced icon settings", cfg, err)
	}
	data, _ := os.ReadFile(path)
	var values map[string]json.RawMessage
	if err := json.Unmarshal(data, &values); err != nil || len(values["future"]) == 0 {
		t.Fatal("icon settings dropped unknown config values", err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0640 {
		t.Fatal("icon settings changed config permissions", err)
	}
}

func TestIconSettingsValidateAndKeepInvalidConfigIntact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	bad := []byte(`{"unfinished":`)
	if err := os.WriteFile(path, bad, 0600); err != nil {
		t.Fatal(err)
	}
	if err := SaveIcons(path, "classic", "nerd"); err == nil {
		t.Fatal("invalid config was overwritten")
	}
	data, _ := os.ReadFile(path)
	if string(data) != string(bad) {
		t.Fatal("failed save changed invalid config")
	}
	for _, input := range []string{`{"fileIcons":"unknown"}`, `{"fileIcons":""}`} {
		os.WriteFile(path, []byte(input), 0600)
		if _, err := Load(path); err == nil {
			t.Fatal("invalid file icon theme accepted", input)
		}
	}
	if err := SaveIcons(path, "unknown", "nerd"); err == nil {
		t.Fatal("unknown file icons saved")
	}
	if err := SaveIcons(path, "classic", "unknown"); err == nil {
		t.Fatal("unknown action icons saved")
	}
	missing := filepath.Join(t.TempDir(), "SunBook", "config.json")
	if err := SaveIcons(missing, "classic", "unicode"); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "linked.json")
	if err := os.Symlink(missing, link); err != nil {
		t.Fatal(err)
	}
	if err := SaveIcons(link, "expressive", "nerd"); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(missing)
	if err != nil || cfg.FileIcons != "expressive" || cfg.ActivityIcons != "nerd" {
		t.Fatal("saving icons through a symlink failed", cfg, err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("icon save replaced the config symlink", err)
	}
}
