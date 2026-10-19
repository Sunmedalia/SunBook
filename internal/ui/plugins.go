package ui

import (
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/gdamore/tcell/v3"
	"github.com/rivo/uniseg"
	"sunbook/internal/config"
	"sunbook/internal/editor"
	"sunbook/internal/plugin"
	"sunbook/internal/text"
)

func (a *App) pluginDirectory() string {
	if a.cfg.FilePath != "" {
		return filepath.Join(filepath.Dir(a.cfg.FilePath), "plugins")
	}
	return filepath.Join(a.root, ".sunbook-plugins")
}
func (a *App) pluginPreferences() map[string]plugin.Preference {
	copy := make(map[string]plugin.Preference)
	for id, p := range a.cfg.Plugins {
		copy[id] = p
	}
	return copy
}
func (a *App) refreshPlugins() {
	if a.pluginRefreshing {
		a.pluginRefreshPending = true
		return
	}
	a.pluginRefreshing = true
	prefs := a.pluginPreferences()
	go func() {
		err := a.plugins.Refresh(prefs)
		a.postQuiet(func() {
			a.pluginRefreshing = false
			if err != nil {
				a.showError(err)
			}
			a.updatePluginCommands()
			if a.pluginRefreshPending {
				a.pluginRefreshPending = false
				a.refreshPlugins()
			}
		})
	}()
}
func (a *App) updatePluginCommands() {
	snapshot := a.plugins.Snapshot()
	if reflect.DeepEqual(snapshot, a.pluginStatuses) {
		if a.pluginSelectID != "" {
			for i, status := range snapshot {
				if status.Entry.Manifest.ID == a.pluginSelectID {
					if a.pluginSelected != i {
						a.pluginSelected = i
						a.invalidate()
					}
					a.pluginSelectID = ""
					break
				}
			}
		}
		return
	}
	selectedID := a.pluginSelectID
	if selectedID == "" && a.pluginSelected >= 0 && a.pluginSelected < len(a.pluginStatuses) {
		selectedID = a.pluginStatuses[a.pluginSelected].Entry.Manifest.ID
	}
	a.pluginStatuses = snapshot
	for i, s := range snapshot {
		if s.Entry.Manifest.ID == selectedID {
			a.pluginSelected = i
			a.pluginSelectID = ""
			break
		}
	}
	commands := a.commands[:0]
	for _, c := range a.commands {
		if !strings.HasPrefix(c.ID, "plugin.") {
			commands = append(commands, c)
		}
	}
	clear(a.commands[len(commands):])
	a.commands = commands
	for _, status := range a.pluginStatuses {
		if status.State == plugin.Disabled || status.State == plugin.Review || status.State == plugin.Invalid {
			continue
		}
		for _, c := range status.Entry.Manifest.Commands {
			id, contribution := status.Entry.Manifest.ID, c
			label := status.Entry.Manifest.Name + ": " + contribution.Title
			if status.State != plugin.Ready && status.State != plugin.Busy {
				label += " [" + string(status.State) + "]"
			}
			a.commands = append(a.commands, command{ID: pluginCommandID(id, contribution.ID), Label: label, Run: func() { a.runPluginCommand(id, contribution.ID) }})
		}
	}
	a.pluginSelected = max(0, min(a.pluginSelected, len(a.pluginStatuses)-1))
	a.invalidate()
}
func pluginCommandID(id, command string) string { return "plugin." + id + "/" + command }
func (a *App) selectedPlugin() (plugin.Status, bool) {
	if a.pluginSelected >= 0 && a.pluginSelected < len(a.pluginStatuses) {
		return a.pluginStatuses[a.pluginSelected], true
	}
	return plugin.Status{}, false
}
func (a *App) pluginDetails() {
	status, ok := a.selectedPlugin()
	if !ok {
		return
	}
	id := status.Entry.Manifest.ID
	var items []item
	if status.State == plugin.Invalid && a.cfg.Plugins[id].Enabled {
		items = append(items, item{Label: "Disable configured plugin", Action: func() { a.setPluginEnabled(status, false) }})
	}
	if status.State == plugin.Disabled || status.State == plugin.Review {
		items = append(items, item{Label: "Enable plugin…", Detail: "review local code and permissions", Action: func() { a.reviewPlugin(status) }})
	} else if status.State != plugin.Invalid {
		items = append(items, item{Label: "Disable plugin", Action: func() { a.setPluginEnabled(status, false) }}, item{Label: "Restart plugin", Detail: "reset failure circuit", Action: func() { go func() { err := a.plugins.Restart(id); a.post(func() { a.showError(err) }) }() }})
	}
	if status.State == plugin.Ready || status.State == plugin.Busy {
		for _, command := range status.Entry.Manifest.Commands {
			c := command
			items = append(items, item{Label: c.Title, Action: func() { a.runPluginCommand(id, c.ID) }})
		}
	}
	items = append(items, item{Label: "Health and logs", Action: func() {
		a.showText(status.Entry.Manifest.Name+" · "+string(status.State), fmt.Sprintf("Version: %s\nState: %s\nIsolation: %s\nRecent failures: %d\nNext restart: %v\nDirectory: %s\n\n%s\n\nRecent stderr (up to 16 KiB):\n%s", status.Entry.Manifest.Version, status.State, plugin.IsolationDescription(status.Entry.Manifest), status.Failures, status.RetryAt, status.Entry.Directory, status.Error, status.Log))
	}})
	if status.State == plugin.Disabled || status.State == plugin.Invalid || status.State == plugin.Review {
		items = append(items, item{Label: "Remove plugin…", Action: func() {
			a.choose("Remove "+status.Entry.Manifest.Name+"?", []item{{Label: "Remove local plugin files", Detail: "only the installed copy", Action: func() {
				go func() { err := a.plugins.Remove(id); a.post(func() { a.showError(err); a.updatePluginCommands() }) }()
			}}, {Label: "Cancel"}})
		}})
	}
	if a.lastPluginResult != nil && a.lastPluginResult.id == id {
		items = append(items, item{Label: "Last command result", Action: a.showLastPluginResult})
	}
	a.choose(status.Entry.Manifest.Name, items)
	a.modal.AutoHeight = true
	a.modal.Subtitle = string(status.State) + " · " + status.Entry.Manifest.Version
}
func (a *App) reviewPlugin(status plugin.Status) {
	permissions := strings.Join(status.Entry.Manifest.Permissions, ", ")
	if permissions == "" {
		permissions = "No document access"
	}
	a.choose("Enable "+status.Entry.Manifest.Name+"?", []item{{Label: "Enable trusted local plugin", Detail: "starts a separate process", Action: func() { a.setPluginEnabled(status, true) }}, {Label: "Cancel"}})
	a.modal.AutoHeight = true
	a.modal.Body = []string{plugin.IsolationDescription(status.Entry.Manifest), "Permissions: " + permissions, plugin.FileAccessDescription(), status.Entry.Manifest.Description, "Package changes require review. Enable trusted code only."}
}
func (a *App) setPluginEnabled(status plugin.Status, enabled bool) {
	prefs := a.pluginPreferences()
	prefs[status.Entry.Manifest.ID] = plugin.Preference{Enabled: enabled, Fingerprint: status.Entry.Fingerprint}
	if a.cfg.FilePath != "" {
		if err := config.SavePlugins(a.cfg.FilePath, prefs); err != nil {
			a.showError(err)
			return
		}
	}
	a.cfg.Plugins = prefs
	a.refreshPlugins()
}
func (a *App) installPlugin() {
	a.compactPrompt("Install Plugin Folder", "", "Path to a folder containing plugin.json", func(source string) {
		source = openPath(a.root, strings.TrimSpace(source))
		a.notify("Installing plugin…")
		go func() {
			id, err := a.plugins.Install(source)
			a.post(func() {
				if err != nil {
					a.showError(err)
					return
				}
				a.pluginSelectID = id
				a.notify("Installed " + id + "; review and enable it in Plugins")
				a.showSidebar("Plugins")
				a.refreshPlugins()
			})
		}()
	})
}
func (a *App) runPluginCommand(id, command string) { a.invokePluginCommand(id, command, nil) }

func (a *App) invokePluginCommand(id, command string, formatting *pluginSplitPreview) {
	started := false
	defer func() {
		if formatting != nil && !started {
			formatting.ApplyWhenReady = false
		}
	}()
	if a.comparing() {
		a.notify("Open an editable document before running plugin commands")
		return
	}
	d := a.view().Doc
	version := d.Buffer.Version
	buffer := d.Buffer
	doc := &plugin.Document{Path: d.Path, Version: version}
	var status plugin.Status
	found := false
	for _, s := range a.pluginStatuses {
		if s.Entry.Manifest.ID == id {
			status = s
			found = true
			break
		}
	}
	if !found {
		a.notify("Plugin not installed")
		return
	}
	if status.Entry.Manifest.Allows(plugin.ReadDocument) {
		if d.Buffer.Len() > plugin.MaxMessage-64*1024 {
			a.notify("Document exceeds the plugin snapshot size limit")
			return
		}
		doc.Text = d.Buffer.String()
		for _, sel := range a.view().Selections {
			doc.Selections = append(doc.Selections, plugin.Position{Anchor: sel.Anchor, Head: sel.Head})
		}
	}
	a.notify("Running " + status.Entry.Manifest.Name + "…")
	started = true
	go func() {
		result, err := a.plugins.Invoke(a.ctx, id, plugin.Invocation{Command: command, Document: doc})
		var changes []text.Edit
		if err == nil && len(result.Edits) > 0 {
			changes, err = validatePluginEdits(doc.Text, result.Edits)
		}
		if err == nil && result.Preview != nil && len(changes) > 0 {
			// Display the exact candidate that will be applied, never a different
			// plugin-supplied text beside unrelated edits.
			if pluginEditedText(doc.Text, changes) != result.Preview.Text {
				err = fmt.Errorf("plugin preview does not match its proposed edits")
			}
		}
		a.post(func() {
			if formatting != nil {
				formatting.ApplyWhenReady = false
				if a.pluginPreview() != formatting {
					return
				}
				if d.Buffer != buffer || d.Buffer.Version != version || a.modal != nil {
					a.notify("Document changed; formatting cancelled. Click Format to retry")
					return
				}
			}
			if err != nil {
				a.notify(status.Entry.Manifest.Name + ": " + err.Error())
				return
			}
			a.lastPluginResult = &pluginOutcome{id: id, command: command, name: status.Entry.Manifest.Name, doc: d, buffer: buffer, version: version, result: result, edits: changes, path: doc.Path}
			if formatting != nil {
				if result.Preview == nil {
					a.notify("Plugin did not return a formatting preview")
					return
				}
				formatting.Outcome = a.lastPluginResult
				a.applyPluginSplit()
				return
			}
			if len(changes) == 0 && result.Preview == nil {
				a.lastPluginResult.doc = nil
				a.lastPluginResult.buffer = nil
			}
			if a.modal != nil {
				a.notify("Plugin result ready; close this dialog, then F1 → Plugins: Show Last Result")
				return
			}
			a.showLastPluginResult()
		})
	}()
}

type pluginOutcome struct {
	id, command, name, path string
	doc                     *editor.Document
	buffer                  *text.Buffer
	version                 int
	result                  plugin.Result
	edits                   []text.Edit
}

func (a *App) showLastPluginResult() {
	out := a.lastPluginResult
	if out == nil {
		a.notify("Run a plugin command to produce a result")
		return
	}
	d, buffer, version, result, changes := out.doc, out.buffer, out.version, out.result, out.edits
	if result.Preview != nil {
		if d == nil || a.comparing() || a.view().Doc != d {
			a.notify("Plugin preview ignored: the document or active editor changed")
			return
		}
		a.openPluginSplit(out)
		return
	}
	if len(changes) > 0 {
		if d == nil || a.comparing() || a.view().Doc != d || d.Buffer != buffer || d.Buffer.Version != version {
			a.notify("Plugin result ignored: the document or active editor changed")
			return
		}
		a.choose("Apply "+out.name+" edits?", []item{{Label: fmt.Sprintf("Apply %d edit(s)", len(changes)), Detail: "one undo step · remains unsaved", Action: func() {
			if d == nil || a.comparing() || a.view().Doc != d || d.Buffer != buffer || d.Buffer.Version != version {
				a.notify("Document changed; plugin edits cancelled")
				return
			}
			if !a.apply(changes) {
				a.notify("Plugin edits could not be applied")
				return
			}
			a.notify("Plugin edits applied; Ctrl+Z undoes them")
		}}, {Label: "Cancel"}})
		a.modal.Body = pluginEditPreview(d, changes)
	} else if result.Body != "" {
		title := result.Title
		if title == "" {
			title = out.name
		}
		a.showText(title, result.Body)
		name := filepath.Base(out.path)
		if out.path == "" {
			name = "Untitled"
		}
		a.modal.Subtitle = fmt.Sprintf("%s · snapshot v%d", name, version)
	} else {
		message := result.Message
		if message == "" {
			message = "Command finished"
		}
		a.notify(out.name + ": " + message)
	}
}
func validatePluginEdits(source string, edits []plugin.Edit) ([]text.Edit, error) {
	out := make([]text.Edit, len(edits))
	for i, e := range edits {
		normalized := strings.ReplaceAll(strings.ReplaceAll(e.Text, "\r\n", "\n"), "\r", "\n")
		out[i] = text.Edit{Start: e.Start, End: e.End, Text: normalized}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	// Plugin byte offsets must preserve full user-perceived characters, not just
	// UTF-8 code points. Build the boundary set once for all returned edits.
	boundaries := map[int]bool{0: true}
	total := len(source)
	for i, e := range out {
		if e.Start < 0 || e.End < e.Start || e.End > len(source) || !utf8.ValidString(e.Text) || strings.ContainsRune(e.Text, 0) {
			return nil, fmt.Errorf("plugin returned an invalid edit range")
		}
		if i > 0 && (out[i-1].End > e.Start || out[i-1].Start == e.Start) {
			return nil, fmt.Errorf("plugin returned overlapping edits")
		}
		if e.Start != 0 {
			boundaries[e.Start] = false
		}
		if e.End != 0 {
			boundaries[e.End] = false
		}
		total += len(e.Text) - (e.End - e.Start)
	}
	g := uniseg.NewGraphemes(source)
	for g.Next() {
		_, end := g.Positions()
		if _, needed := boundaries[end]; needed {
			boundaries[end] = true
		}
	}
	for _, valid := range boundaries {
		if !valid {
			return nil, fmt.Errorf("plugin edit splits a Unicode grapheme")
		}
	}
	if total > plugin.MaxMessage {
		return nil, fmt.Errorf("plugin result exceeds the document size limit")
	}
	return out, nil
}
func pluginEditPreview(d *editor.Document, edits []text.Edit) []string {
	body := []string{d.Name() + " · document snapshot v" + fmt.Sprint(d.Buffer.Version)}
	source := d.Buffer.String()
	for i, e := range edits {
		if i >= 20 {
			body = append(body, fmt.Sprintf("… and %d more edit(s)", len(edits)-i))
			break
		}
		row, _ := d.Buffer.Position(e.Start)
		before, after := source[e.Start:e.End], e.Text
		if len(before) > 200 {
			before = before[:text.Snap(before, 200)] + "…"
		}
		if len(after) > 200 {
			after = after[:text.Snap(after, 200)] + "…"
		}
		body = append(body, fmt.Sprintf("L%d: %q → %q", row+1, before, after))
	}
	return body
}
func (a *App) pluginKey(e *tcell.EventKey, name string) {
	switch name {
	case "up":
		a.pluginSelected = max(0, a.pluginSelected-1)
	case "down":
		a.pluginSelected = min(len(a.pluginStatuses)-1, a.pluginSelected+1)
	case "enter":
		a.pluginDetails()
	case "pageup":
		a.pluginSelected = max(0, a.pluginSelected-max(1, (a.sideRect.H-4)/3))
	case "pagedown":
		a.pluginSelected = min(len(a.pluginStatuses)-1, a.pluginSelected+max(1, (a.sideRect.H-4)/3))
	case "home":
		a.pluginSelected = 0
	case "end":
		a.pluginSelected = max(0, len(a.pluginStatuses)-1)
	case "r":
		a.refreshPlugins()
	case "s":
		a.openPluginStore()
	case "i":
		a.installPlugin()
	case "escape":
		a.focus = "editor"
	}
}
func (a *App) drawPlugins() {
	r := a.sideRect
	a.drawSidebarHeader("PLUGINS", 4)
	a.drawButton(r.X+r.W-4, r.Y, a.actionLabel("plugins.refresh", "Refresh", true), "plugins.refresh", "Reload installed plugin manifests", false)
	band := a.sidebarBand(r.Y + 1)
	ready := 0
	for _, s := range a.pluginStatuses {
		if s.State == plugin.Ready || s.State == plugin.Busy {
			ready++
		}
	}
	a.put(r.X+1, r.Y+1, r.W-2, fmt.Sprintf("%d ready / %d installed", ready, len(a.pluginStatuses)), band)
	if r.H > 3 {
		a.drawButton(r.X+1, r.Y+2, "Local…", "plugins.install", "Install a local plugin folder (i)", false)
	}
	if r.H > 3 {
		a.drawButton(r.X+11, r.Y+2, "Store…", "plugins.store", "Browse GitHub plugin store (s)", false)
	}
	if len(a.pluginStatuses) == 0 {
		a.put(r.X+1, r.Y+4, r.W-2, "No plugins installed", a.accent)
		if r.H > 7 {
			a.put(r.X+1, r.Y+6, r.W-2, "Browse Store (s) or install", a.muted)
			a.put(r.X+1, r.Y+7, r.W-2, "then review permissions.", a.muted)
		}
		return
	}
	height := max(1, (r.H-4)/3)
	if a.pluginSelected < a.pluginTop {
		a.pluginTop = a.pluginSelected
	}
	if a.pluginSelected >= a.pluginTop+height {
		a.pluginTop = a.pluginSelected - height + 1
	}
	a.pluginTop = max(0, a.pluginTop)
	for i, y := a.pluginTop, r.Y+4; i < len(a.pluginStatuses) && y+1 < r.Y+r.H; i, y = i+1, y+3 {
		s := a.pluginStatuses[i]
		style := a.chrome
		if i == a.pluginSelected {
			style = a.drawFileSelection(rect{r.X, y, r.W, 2}, a.focus == "plugins")
		}
		color := a.colors.muted
		switch s.State {
		case plugin.Ready:
			color = a.colors.success
		case plugin.Busy, plugin.Starting, plugin.Restarting, plugin.Review:
			color = a.accent.GetForeground()
		case plugin.Quarantined, plugin.Invalid:
			color = a.colors.danger
		}
		a.put(r.X+1, y, 1, "●", style.Foreground(color))
		a.put(r.X+3, y, r.W-4, fitLabel(s.Entry.Manifest.Name, r.W-4), style.Bold(true))
		a.put(r.X+3, y+1, r.W-4, fitLabel(string(s.State)+" · "+s.Entry.Manifest.Version, r.W-4), style.Foreground(color))
		a.sideHits = append(a.sideHits, sideHit{rect: rect{r.X, y, r.W, 2}, Index: i})
	}
}
func (a *App) pluginStatusLabel() (string, bool) {
	ready, failed := 0, 0
	for _, s := range a.pluginStatuses {
		if s.State == plugin.Ready || s.State == plugin.Busy {
			ready++
		}
		if s.State == plugin.Invalid || s.State == plugin.Quarantined {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Sprintf("plugins !%d", failed), true
	}
	return fmt.Sprintf("plugins %d/%d", ready, len(a.pluginStatuses)), false
}
