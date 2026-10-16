package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPartialOverrides(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(p, []byte(`{"tabSize":2,"insertSpaces":false,"servers":{"typescript":{"initializationOptions":{"tsserver":{"path":"/custom/tsserver.js"}}},"python":{"settings":{"python":{"analysis":{"typeCheckingMode":"strict"}}}}},"keybindings":{"f8":"language.format"}}`), 0644)
	c, e := Load(p)
	if e != nil {
		t.Fatal(e)
	}
	if c.TabSize != 2 || c.InsertSpaces || c.Servers["typescript"].Command != "typescript-language-server" || len(c.Servers["typescript"].Args) != 1 || c.Servers["python"].Command != "pyright-langserver" || c.Servers["go"].Command != "gopls" || c.Theme.Background == "" {
		t.Fatal("defaults lost", c)
	}
	os.WriteFile(p, []byte(`{"tabSize":0}`), 0644)
	if _, e = Load(p); e == nil {
		t.Fatal("invalid tab size")
	}
}
