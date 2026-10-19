package ui

import (
	"os"
	"strings"
	"sunbook/internal/config"
	"sunbook/internal/plugin"
	"testing"
)

func TestPluginStoreSearchAndInstalledDetails(t *testing.T) {
	a := examplePluginApp(t)
	a.pluginCatalog = []plugin.StorePlugin{
		{ID: "text-tools", Name: "Text Tools", Version: "1.0.0", Description: "Count words", Author: "Sunmedalia", Runtime: "Python 3.9+"},
		{ID: "markdown-tools", Name: "Markdown Tools", Version: "1.2.0", Description: "Format Markdown", Author: "Sunmedalia", Runtime: "Python 3.9+"},
	}
	a.showPluginStore()
	if !a.modal.Filter || !a.modal.InputEnabled {
		t.Fatal("store cannot search")
	}
	a.modal.Input = "Markdown"
	a.filterModal()
	if len(a.modal.Visible) != 1 || a.modal.Visible[0].Label != "Markdown Tools" {
		t.Fatal(a.modal.Visible)
	}
	a.storePluginDetails(a.pluginCatalog[0])
	if a.modal.Items[0].Label != "Manage installed plugin" {
		t.Fatal(a.modal.Items)
	}
	a.storePluginDetails(a.pluginCatalog[1])
	if a.modal.Items[0].Label != "Install from GitHub" || !strings.Contains(strings.Join(a.modal.Body, " "), "Python 3.9+") {
		t.Fatal(a.modal)
	}
}

func TestPluginProxyCheckboxPersists(t *testing.T) {
	a := examplePluginApp(t)
	p := plugin.StorePlugin{ID: "markdown-tools", Name: "Markdown Tools", Version: "1.2.0"}
	for _, enabled := range []bool{true, false} {
		a.storePluginDetails(p)
		checkbox := a.modal.Items[1]
		if !strings.Contains(checkbox.Label, "Use gh-proxy") {
			t.Fatal(checkbox)
		}
		checkbox.Action()
		cfg, err := config.Load(a.cfg.FilePath)
		if err != nil || cfg.PluginUseGHProxy != enabled || a.pluginStoreClient().UseGHProxy != enabled {
			t.Fatal(cfg, err)
		}
		expected := "[ ]"
		if enabled {
			expected = "[x]"
		}
		if !strings.HasPrefix(a.modal.Items[1].Label, expected) {
			t.Fatal(a.modal.Items)
		}
	}
}
func TestPluginProxySaveFailureKeepsSelection(t *testing.T) {
	a := examplePluginApp(t)
	if err := os.WriteFile(a.cfg.FilePath, []byte(`{"broken":`), 0600); err != nil {
		t.Fatal(err)
	}
	a.pluginProxyItem(func() { t.Fatal("applied failed setting") }).Action()
	if a.cfg.PluginUseGHProxy {
		t.Fatal("failed save changed proxy")
	}
}
