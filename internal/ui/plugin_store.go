package ui

import (
	"context"
	"sunbook/internal/config"
	"sunbook/internal/plugin"
)

func (a *App) pluginStoreClient() plugin.Store {
	return plugin.Store{UseGHProxy: a.cfg.PluginUseGHProxy}
}
func (a *App) openPluginStore() {
	if a.pluginStoreCancel != nil {
		a.pluginStoreCancel()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.pluginStoreCancel = cancel
	a.pluginStoreGeneration++
	generation := a.pluginStoreGeneration
	a.pluginStoreLoading = true
	source := a.cfg.PluginCatalogURL
	if source == "" {
		source = plugin.DefaultCatalogURL
	}
	store := a.pluginStoreClient()
	a.showPluginStore()
	a.notify("Loading plugins from GitHub…")
	go func() {
		defer cancel()
		catalog, err := store.Fetch(ctx, source)
		a.post(func() {
			if generation != a.pluginStoreGeneration {
				return
			}
			a.pluginStoreLoading = false
			a.pluginStoreCancel = nil
			ownModal := a.modal != nil && a.modal.Title == "Plugin Store"
			if err != nil {
				if ownModal {
					a.modal.Subtitle = "Download failed · toggle gh-proxy or refresh to retry"
				}
				a.notify("Plugin store: " + err.Error())
				return
			}
			a.pluginCatalog = append([]plugin.StorePlugin{}, catalog.Plugins...)
			if !ownModal {
				a.notify("Store loaded; F1 → Plugins: Show Store")
				return
			}
			input := ""
			if ownModal {
				input = a.modal.Input
			}
			a.showPluginStore()
			a.modal.Input = input
			a.modal.Cursor = len(input)
			a.filterModal()
		})
	}()
}
func (a *App) pluginProxyItem(after func()) item {
	check := "[ ]"
	if a.cfg.PluginUseGHProxy {
		check = "[x]"
	}
	return item{Label: check + " Use gh-proxy for downloads", Detail: "gh-proxy.com · catalog and packages", Action: func() {
		enabled := !a.cfg.PluginUseGHProxy
		if a.cfg.FilePath != "" {
			if err := config.SavePluginProxy(a.cfg.FilePath, enabled); err != nil {
				a.showError(err)
				return
			}
		}
		a.cfg.PluginUseGHProxy = enabled
		after()
	}}
}
func (a *App) showPluginStore() {
	if a.pluginCatalog == nil && !a.pluginStoreLoading {
		a.openPluginStore()
		return
	}
	items := []item{a.pluginProxyItem(a.openPluginStore), {Label: "Refresh GitHub catalog", Action: a.openPluginStore}}
	installedPlugins := a.plugins.Snapshot()
	for _, p := range a.pluginCatalog {
		detail := p.Version + " · " + p.Author + " · " + p.Description
		for _, s := range installedPlugins {
			if s.Entry.Manifest.ID == p.ID {
				detail = "Installed " + s.Entry.Manifest.Version + " · Store " + p.Version + " · " + p.Description
				break
			}
		}
		items = append(items, item{Label: p.Name, Detail: detail, Action: func() { a.storePluginDetails(p) }})
	}
	a.setModal(&modal{Title: "Plugin Store", Items: items, InputEnabled: true, Filter: true, Placeholder: "Search plugins…", Empty: "No matching plugins", Hint: "↑/↓ choose · Enter for details · Esc closes"})
	a.modal.Subtitle = "GitHub · type to search · Enter for details"
	if a.pluginStoreLoading {
		a.modal.Subtitle = "Loading catalog… · gh-proxy can be toggled below"
	}
}
func (a *App) storePluginDetails(p plugin.StorePlugin) {
	items := []item{}
	installed := false
	for _, s := range a.plugins.Snapshot() {
		if s.Entry.Manifest.ID == p.ID {
			installed = true
			break
		}
	}
	if !installed {
		items = append(items, item{Label: "Install from GitHub", Detail: "download and verify; remains disabled", Action: func() { a.installStorePlugin(p) }})
	} else {
		items = append(items, item{Label: "Manage installed plugin", Detail: "disable and remove before installing another version", Action: func() { a.pluginSelectID = p.ID; a.showSidebar("Plugins"); a.updatePluginCommands(); a.pluginDetails() }})
	}
	items = append(items, a.pluginProxyItem(func() { a.storePluginDetails(p) }), item{Label: "Back to store", Action: a.showPluginStore})
	a.choose(p.Name+" · "+p.Version, items)
	a.modal.AutoHeight = true
	a.modal.Body = []string{p.Description, "Author: " + p.Author, "Requires: " + p.Runtime, "Source: " + p.Repository, "SHA-256: " + p.SHA256, "Review installed code and permissions before enabling."}
}
func (a *App) installStorePlugin(p plugin.StorePlugin) {
	if a.pluginStoreInstalling {
		a.notify("A store installation is already in progress")
		return
	}
	for _, status := range a.plugins.Snapshot() {
		if status.Entry.Manifest.ID == p.ID {
			a.notify("Plugin already installed; disable and remove it before reinstalling")
			return
		}
	}
	// Reinstalling the same bytes must still require fresh approval, including
	// after an editor restart. Persist disabled state before downloading.
	prefs := a.pluginPreferences()
	prefs[p.ID] = plugin.Preference{}
	if a.cfg.FilePath != "" {
		if err := config.SavePlugins(a.cfg.FilePath, prefs); err != nil {
			a.showError(err)
			return
		}
	}
	a.cfg.Plugins = prefs
	a.pluginStoreInstalling = true
	a.notify("Downloading and verifying " + p.Name + "…")
	store := a.pluginStoreClient()
	go func() {
		id, err := store.Install(a.ctx, a.plugins, p)
		a.post(func() {
			a.pluginStoreInstalling = false
			if err != nil {
				a.showError(err)
				return
			}
			a.pluginSelectID = id
			a.showSidebar("Plugins")
			a.refreshPlugins()
			a.notify("Installed " + p.Name + "; review permissions to enable it")
		})
	}()
}
