package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sunbook/internal/plugin"
)

func SaveIcons(path, files, activity string) error {
	if files != "classic" && files != "expressive" {
		return fmt.Errorf("fileIcons must be classic or expressive")
	}
	if activity != "nerd" && activity != "unicode" {
		return fmt.Errorf("activityIcons must be nerd or unicode")
	}
	return saveSettings(path, map[string]any{"fileIcons": files, "activityIcons": activity})
}

// Merge only the requested fields, preserving unknown keys, permissions and
// config symlinks. Atomic replacement also protects the first settings save.
func saveSettings(path string, changes map[string]any) error {
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
	for key, value := range changes {
		settings[key], err = json.Marshal(value)
		if err != nil {
			return err
		}
	}
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

func SavePlugins(path string, preferences map[string]plugin.Preference) error {
	return saveSettings(path, map[string]any{"plugins": preferences})
}

func SavePluginProxy(path string, enabled bool) error {
	return saveSettings(path, map[string]any{"pluginUseGHProxy": enabled})
}
