package config

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type Server struct {
	Command               string         `json:"command"`
	Args                  []string       `json:"args"`
	Settings              map[string]any `json:"settings,omitempty"`
	InitializationOptions map[string]any `json:"initializationOptions,omitempty"`
}
type Theme struct {
	Background string `json:"background"`
	Foreground string `json:"foreground"`
	Accent     string `json:"accent"`
}
type Config struct {
	TabSize      int               `json:"tabSize"`
	InsertSpaces bool              `json:"insertSpaces"`
	Shell        string            `json:"shell"`
	PythonPath   string            `json:"pythonPath"`
	Theme        Theme             `json:"theme"`
	Keybindings  map[string]string `json:"keybindings"`
	Servers      map[string]Server `json:"servers"`
}

func Default() Config {
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/sh"
	}
	return Config{TabSize: 4, InsertSpaces: true, Shell: sh, Theme: Theme{"#161e2b", "#d4deea", "#e2b66f"}, Keybindings: map[string]string{}, Servers: map[string]Server{
		"go": {Command: "gopls"}, "python": {Command: "pyright-langserver", Args: []string{"--stdio"}}, "typescript": {Command: "typescript-language-server", Args: []string{"--stdio"}},
	}}
}
func Path() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "SunBook", "config.json")
}
func Load(path string) (Config, error) {
	c := Default()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err = json.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("config: %w", err)
	}
	// JSON replaces map values, so merge server overrides with their defaults.
	defaults := Default().Servers
	var partial struct {
		Servers map[string]json.RawMessage `json:"servers"`
	}
	if err = json.Unmarshal(data, &partial); err != nil {
		return c, err
	}
	if c.Servers == nil {
		c.Servers = map[string]Server{}
	}
	for key, raw := range partial.Servers {
		server := defaults[key]
		if err = json.Unmarshal(raw, &server); err != nil {
			return c, err
		}
		c.Servers[key] = server
	}
	if c.TabSize < 1 || c.TabSize > 16 {
		return c, fmt.Errorf("tabSize must be between 1 and 16")
	}
	if c.Shell == "" {
		c.Shell = "/bin/sh"
	}
	return c, nil
}
func Language(path string) (string, string) {
	switch filepath.Ext(path) {
	case ".go":
		return "go", "go"
	case ".py", ".pyi":
		return "python", "python"
	case ".js", ".mjs", ".cjs":
		return "typescript", "javascript"
	case ".jsx":
		return "typescript", "javascriptreact"
	case ".ts", ".mts", ".cts":
		return "typescript", "typescript"
	case ".tsx":
		return "typescript", "typescriptreact"
	}
	return "", ""
}
func Doctor(c Config) string {
	out := "SunBook dependency check\n\n"
	for _, key := range []string{"go", "python", "typescript"} {
		s := c.Servers[key]
		p, e := exec.LookPath(s.Command)
		if e != nil {
			p = "MISSING (editing remains available)"
		}
		out += fmt.Sprintf("%-27s %s\n", s.Command, p)
	}
	for _, name := range []string{"git", "rg", "ruff", c.Shell} {
		p, e := exec.LookPath(name)
		if e != nil {
			p = "MISSING"
		}
		out += fmt.Sprintf("%-27s %s\n", name, p)
	}
	out += "\nLanguage servers are installed by the user. See README.md.\n"
	return out
}
