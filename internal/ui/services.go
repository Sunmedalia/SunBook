package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/gdamore/tcell/v3"
	"sunbook/internal/config"
	"sunbook/internal/editor"
	"sunbook/internal/lsp"
	"sunbook/internal/text"
)

func (a *App) scanTree() {
	a.invalidateTreeIndex()
	a.treeSeq++
	seq := a.treeSeq
	root := a.root
	expanded := map[string]bool{}
	for p, v := range a.expanded {
		expanded[p] = v
	}
	go func() {
		var entries []treeEntry
		var walk func(string, int)
		walk = func(dir string, depth int) {
			if len(entries) > 10000 || depth > 30 {
				return
			}
			files, e := os.ReadDir(dir)
			if e != nil {
				return
			}
			sort.SliceStable(files, func(i, j int) bool {
				if files[i].IsDir() != files[j].IsDir() {
					return files[i].IsDir()
				}
				return strings.ToLower(files[i].Name()) < strings.ToLower(files[j].Name())
			})
			for _, f := range files {
				if f.Name() == ".git" {
					continue
				}
				path := filepath.Join(dir, f.Name())
				entries = append(entries, treeEntry{path, f.Name(), depth, f.IsDir()})
				if f.IsDir() && expanded[path] {
					walk(path, depth+1)
				}
			}
		}
		walk(root, 0)
		a.post(func() {
			if seq == a.treeSeq {
				a.treeAll = entries
				a.applyTreeFilter()
			}
		})
	}()
}
func (a *App) toggleTree() {
	if a.treeSelected < 0 || a.treeSelected >= len(a.tree) {
		return
	}
	e := a.tree[a.treeSelected]
	if e.Dir {
		a.expanded[e.Path] = !a.expanded[e.Path]
		a.scanTree()
	} else {
		a.showError(a.open(e.Path))
	}
}
func (a *App) jump(path string, line, col int) {
	if e := a.open(path); e != nil {
		a.showError(e)
		return
	}
	b := a.view().Doc.Buffer
	line = max(0, min(line, b.LineCount()-1))
	a.view().Set(b.LineStart(line)+min(col, len(b.Line(line))), false)
	a.reveal()
}

func (a *App) scheduleHighlight(d *editor.Document) {
	version := d.Buffer.Version
	if a.highlightPending[d] == version {
		return
	}
	a.highlightPending[d] = version
	a.highlightDue[d] = time.Now().Add(120 * time.Millisecond)
}
func (a *App) flushHighlights() {
	for d, due := range a.highlightDue {
		if time.Now().Before(due) || a.highlightRunning[d] {
			continue
		}
		delete(a.highlightDue, d)
		a.highlightRunning[d] = true
		a.runHighlight(d)
	}
}
func (a *App) runHighlight(d *editor.Document) {
	version := d.Buffer.Version
	s := d.Buffer.String()
	path := d.Path
	bg := a.base
	if len(s) > 2*1024*1024 {
		delete(a.highlight, d)
		delete(a.highlightRunning, d)
		return
	}
	go func() {
		lex := lexers.Match(path)
		if lex == nil {
			lex = lexers.Fallback
		}
		lex = chroma.Coalesce(lex)
		iterator, e := lex.Tokenise(nil, s)
		var spans []tokenSpan
		if e == nil {
			offset := 0
			theme := styles.Get("github-dark")
			if theme == nil {
				theme = styles.Fallback
			}
			for token := iterator(); token != chroma.EOF; token = iterator() {
				style := bg
				entry := theme.Get(token.Type)
				if entry.Colour.IsSet() {
					style = style.Foreground(tcell.NewRGBColor(int32(entry.Colour.Red()), int32(entry.Colour.Green()), int32(entry.Colour.Blue())))
				}
				if entry.Bold == chroma.Yes {
					style = style.Bold(true)
				}
				if entry.Italic == chroma.Yes {
					style = style.Italic(true)
				}
				spans = append(spans, tokenSpan{offset, offset + len(token.Value), style})
				offset += len(token.Value)
			}
		}
		a.post(func() {
			delete(a.highlightRunning, d)
			for _, open := range a.docs {
				if open == d && d.Buffer.Version == version {
					a.highlight[d] = highlights{version, spans}
					break
				}
			}
		})
	}()
}
func (a *App) startLanguage(d *editor.Document) {
	key, lang := config.Language(d.Path)
	if key == "" || d.Path == "" {
		return
	}
	if c := a.clients[key]; c != nil {
		if a.opened[d] != c {
			c.Open(d.Path, lang, d.Buffer.String(), d.Buffer.Version)
			a.opened[d] = c
		}
		return
	}
	if a.starting[key] || a.serverErrors[key] != "" {
		return
	}
	server := a.cfg.Servers[key]
	if server.Command == "" {
		a.serverErrors[key] = "No language server configured"
		return
	}
	if key == "python" && a.cfg.PythonPath != "" {
		settings := map[string]any{}
		for k, v := range server.Settings {
			settings[k] = v
		}
		python := map[string]any{}
		if old, ok := settings["python"].(map[string]any); ok {
			for k, v := range old {
				python[k] = v
			}
		}
		python["pythonPath"] = a.cfg.PythonPath
		settings["python"] = python
		server.Settings = settings
	}
	a.starting[key] = true
	root := a.root
	seq := a.languageSeq
	go func() {
		client, err := lsp.Start(a.ctx, root, server, func(diag lsp.Diagnostics) {
			a.post(func() {
				if a.root != root || a.languageSeq != seq {
					return
				}
				for _, d := range a.docs {
					if lsp.URI(d.Path) == diag.URI && (diag.Version == nil || *diag.Version == d.Buffer.Version) {
						a.diagnostics[diag.URI] = diag
						break
					}
				}
			})
		}, func(msg string) {
			a.post(func() {
				if a.root == root && a.languageSeq == seq {
					a.notify(msg)
				}
			})
		})
		a.post(func() {
			if a.root != root || a.languageSeq != seq {
				if client != nil {
					go client.Close()
				}
				return
			}
			a.starting[key] = false
			if err != nil {
				a.serverErrors[key] = err.Error()
				a.notify("LSP: " + err.Error() + " · editing remains available")
				return
			}
			a.clients[key] = client
			go func() {
				select {
				case <-client.Done():
					a.post(func() {
						if a.languageSeq != seq || a.clients[key] != client {
							return
						}
						delete(a.clients, key)
						a.serverErrors[key] = "Language server exited; use Restart Language Servers"
						for doc, c := range a.opened {
							if c == client {
								delete(a.opened, doc)
								delete(a.diagnostics, lsp.URI(doc.Path))
							}
						}
						a.notify(a.serverErrors[key])
					})
				case <-a.ctx.Done():
				}
			}()
			delete(a.serverErrors, key)
			for _, doc := range a.docs {
				k, _ := config.Language(doc.Path)
				if k == key {
					a.startLanguage(doc)
				}
			}
		})
	}()
}
func (a *App) request(method, capability string, extra map[string]any, done func(*editor.Document, json.RawMessage)) {
	d := a.view().Doc
	c := a.opened[d]
	if c == nil {
		a.notify("Language server unavailable. Run sunbook --doctor or Restart Language Servers.")
		return
	}
	if !c.Supports(capability) {
		a.notify("Language server does not support " + method)
		return
	}
	params := map[string]any{"textDocument": map[string]any{"uri": lsp.URI(d.Path)}, "position": lsp.At(d.Buffer, a.view().Head())}
	for k, v := range extra {
		params[k] = v
	}
	version := d.Buffer.Version
	c.Request(method, params, func(raw json.RawMessage, err error) {
		a.post(func() {
			if err != nil {
				a.showError(err)
				return
			}
			if a.view().Doc != d || d.Buffer.Version != version {
				a.notify("Document changed; request result ignored")
				return
			}
			done(d, raw)
		})
	})
}
func (a *App) requestCompletion(manual bool) {
	if a.focus != "editor" || a.modal != nil {
		return
	}
	d := a.view().Doc
	c := a.opened[d]
	if c == nil || !c.Supports("completionProvider") {
		if manual {
			a.notify("Completion unavailable; check language server")
		}
		return
	}
	head := a.view().Head()
	if len(a.view().Selections) != 1 || !a.view().Selections[0].Empty() {
		return
	}
	if !manual {
		if head == 0 {
			return
		}
		s := d.Buffer.String()
		prev := s[head-1]
		if prev != '.' && !(prev >= 'a' && prev <= 'z' || prev >= 'A' && prev <= 'Z' || prev == '_') {
			return
		}
	}
	version := d.Buffer.Version
	c.Request("textDocument/completion", map[string]any{"textDocument": map[string]string{"uri": lsp.URI(d.Path)}, "position": lsp.At(d.Buffer, head), "context": map[string]int{"triggerKind": 1}}, func(raw json.RawMessage, err error) {
		a.post(func() {
			if err != nil {
				if manual {
					a.showError(err)
				}
				return
			}
			if a.view().Doc != d || d.Buffer.Version != version || a.view().Head() != head || a.modal != nil || a.focus != "editor" {
				return
			}
			items, e := lsp.ParseCompletion(raw)
			if e != nil {
				a.showError(e)
				return
			}
			if len(items) > 200 {
				items = items[:200]
			}
			if len(items) > 0 {
				a.completion = &completionState{Doc: d, Version: version, Head: head, Items: items}
			} else if manual {
				a.notify("No completions")
			}
		})
	})
}
func (a *App) acceptCompletion() {
	c := a.completion
	if c == nil {
		return
	}
	a.completion = nil
	if c.Doc != a.view().Doc || c.Version != c.Doc.Buffer.Version {
		return
	}
	it := c.Items[c.Selected]
	s := it.InsertText
	if s == "" {
		s = it.Label
	}
	lo, hi := c.Head, c.Head
	source := c.Doc.Buffer.String()
	for lo > 0 {
		p := text.Prev(source, lo)
		r, _ := utf8.DecodeRuneInString(source[p:])
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			break
		}
		lo = p
	}
	edit := text.Edit{Start: lo, End: hi, Text: s}
	if it.TextEdit != nil {
		rng := it.TextEdit.Range
		if rng == nil {
			rng = it.TextEdit.Replace
		}
		if rng == nil {
			rng = it.TextEdit.Insert
		}
		if rng != nil {
			edit.Start = lsp.Offset(c.Doc.Buffer, rng.Start)
			edit.End = lsp.Offset(c.Doc.Buffer, rng.End)
		}
		edit.Text = it.TextEdit.NewText
	}
	if it.InsertTextFormat == 2 {
		a.notify("Server returned a snippet despite snippetSupport=false; completion skipped")
		return
	}
	edits, e := lsp.Edits(c.Doc.Buffer, it.AdditionalTextEdits)
	if e != nil {
		a.showError(e)
		return
	}
	edits = append(edits, edit)
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].Start < edits[j].Start })
	if !a.apply(edits) {
		a.notify("Completion contained conflicting edits")
		return
	}
	pos := edit.Start + len(edit.Text)
	for _, e := range edits {
		if e.Start < edit.Start {
			pos += len(e.Text) - (e.End - e.Start)
		}
	}
	a.view().Set(pos, false)
}
func (a *App) definition(references bool) {
	method, capability := "textDocument/definition", "definitionProvider"
	extra := map[string]any{}
	if references {
		method = "textDocument/references"
		capability = "referencesProvider"
		extra["context"] = map[string]bool{"includeDeclaration": true}
	}
	a.request(method, capability, extra, func(_ *editor.Document, raw json.RawMessage) {
		locs, e := lsp.ParseLocations(raw)
		if e != nil {
			a.showError(e)
			return
		}
		if len(locs) == 0 {
			a.notify("No locations found")
			return
		}
		open := func(l lsp.Location) {
			p := lsp.Path(l.URI)
			if p == "" {
				a.notify("Only local file locations are supported")
				return
			}
			if e := a.open(p); e != nil {
				a.showError(e)
				return
			}
			a.view().Set(lsp.Offset(a.view().Doc.Buffer, l.Range.Start), false)
			a.reveal()
		}
		if len(locs) == 1 {
			open(locs[0])
			return
		}
		var items []item
		for _, loc := range locs {
			loc := loc
			rel, _ := filepath.Rel(a.root, lsp.Path(loc.URI))
			items = append(items, item{Label: fmt.Sprintf("%s:%d:%d", rel, loc.Range.Start.Line+1, loc.Range.Start.Character+1), Action: func() { open(loc) }})
		}
		a.choose("Locations", items)
	})
}
func (a *App) hover() {
	a.request("textDocument/hover", "hoverProvider", nil, func(_ *editor.Document, raw json.RawMessage) {
		var result struct {
			Contents json.RawMessage `json:"contents"`
		}
		if json.Unmarshal(raw, &result) != nil || len(result.Contents) == 0 {
			a.notify("No hover information")
			return
		}
		var parts []string
		var parse func(json.RawMessage)
		parse = func(v json.RawMessage) {
			if len(v) == 0 {
				return
			}
			switch v[0] {
			case '"':
				var s string
				_ = json.Unmarshal(v, &s)
				parts = append(parts, s)
			case '[':
				var list []json.RawMessage
				_ = json.Unmarshal(v, &list)
				for _, v := range list {
					parse(v)
				}
			case '{':
				var p struct {
					Value string `json:"value"`
				}
				_ = json.Unmarshal(v, &p)
				parts = append(parts, p.Value)
			}
		}
		parse(result.Contents)
		a.showText("Hover", strings.Join(parts, "\n\n"))
	})
}
func (a *App) format() {
	d := a.view().Doc
	key, _ := config.Language(d.Path)
	if key == "python" {
		s := d.Buffer.String()
		version := d.Buffer.Version
		path := d.Path
		root := a.root
		a.notify("Formatting…")
		go func() {
			ctx, cancel := context.WithTimeout(a.ctx, 8*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "ruff", "format", "--stdin-filename", path, "-")
			cmd.Dir = root
			cmd.Stdin = strings.NewReader(s)
			var stderr strings.Builder
			cmd.Stderr = &stderr
			out, e := cmd.Output()
			a.post(func() {
				if e != nil {
					a.notify("ruff format: " + stderr.String() + e.Error())
					return
				}
				if a.view().Doc != d || d.Buffer.Version != version {
					a.notify("Document changed; formatting ignored")
					return
				}
				if string(out) != s {
					a.apply([]text.Edit{{Start: 0, End: d.Buffer.Len(), Text: string(out)}})
				}
			})
		}()
		return
	}
	a.request("textDocument/formatting", "documentFormattingProvider", map[string]any{"options": map[string]any{"tabSize": a.cfg.TabSize, "insertSpaces": a.cfg.InsertSpaces}}, func(d *editor.Document, raw json.RawMessage) {
		var edits []lsp.TextEdit
		if e := json.Unmarshal(raw, &edits); e != nil {
			a.showError(e)
			return
		}
		changes, e := lsp.Edits(d.Buffer, edits)
		if e != nil {
			a.showError(e)
			return
		}
		sort.Slice(changes, func(i, j int) bool { return changes[i].Start < changes[j].Start })
		if len(changes) > 0 && !a.apply(changes) {
			a.notify("Formatting returned overlapping edits")
		}
	})
}
func (a *App) renameSymbol() {
	a.prompt("Rename Symbol", "", func(name string) {
		if strings.TrimSpace(name) == "" {
			return
		}
		a.request("textDocument/rename", "renameProvider", map[string]any{"newName": name}, func(_ *editor.Document, raw json.RawMessage) {
			var ws lsp.WorkspaceEdit
			if e := json.Unmarshal(raw, &ws); e != nil {
				a.showError(e)
				return
			}
			a.previewWorkspaceEdit(ws)
		})
	})
}
func (a *App) previewWorkspaceEdit(ws lsp.WorkspaceEdit) {
	changes := map[string][]lsp.TextEdit{}
	for uri, edits := range ws.Changes {
		changes[uri] = edits
	}
	for _, dc := range ws.DocumentChanges {
		if dc.Kind != "" {
			a.notify("Rename requires unsupported file operations")
			return
		}
		if dc.TextDocument.Version != nil {
			for _, d := range a.docs {
				if lsp.URI(d.Path) == dc.TextDocument.URI && d.Buffer.Version != *dc.TextDocument.Version {
					a.notify("Rename result is stale")
					return
				}
			}
		}
		changes[dc.TextDocument.URI] = append(changes[dc.TextDocument.URI], dc.Edits...)
	}
	type pending struct {
		doc     *editor.Document
		version int
		edits   []text.Edit
		opened  bool
	}
	var all []pending
	var body []string
	for uri, edits := range changes {
		path := lsp.Path(uri)
		if path == "" {
			a.notify("Rename contains a non-local path")
			return
		}
		var d *editor.Document
		opened := false
		for _, doc := range a.docs {
			if doc.Path == path {
				d = doc
				opened = true
				break
			}
		}
		if d == nil {
			var e error
			d, e = editor.Open(path)
			if e != nil {
				a.showError(e)
				return
			}
		}
		es, e := lsp.Edits(d.Buffer, edits)
		if e != nil {
			a.showError(e)
			return
		}
		sort.Slice(es, func(i, j int) bool { return es[i].Start < es[j].Start })
		for i, e := range es {
			if i > 0 && (es[i-1].End > e.Start || es[i-1].Start == e.Start) {
				a.notify("Rename contains overlapping edits")
				return
			}
		}
		all = append(all, pending{d, d.Buffer.Version, es, opened})
		rel, _ := filepath.Rel(a.root, path)
		body = append(body, fmt.Sprintf("%s — %d edit(s)", rel, len(es)))
		for _, e := range es {
			r, _ := d.Buffer.Position(e.Start)
			body = append(body, fmt.Sprintf("  L%d: %q → %q", r+1, d.Buffer.String()[e.Start:e.End], e.Text))
		}
	}
	if len(all) == 0 {
		a.notify("No rename edits")
		return
	}
	a.choose("Apply Rename? (buffers remain unsaved)", []item{{Label: "Apply edits", Action: func() {
		for _, p := range all {
			if p.doc.Buffer.Version != p.version || p.doc.DiskChanged() {
				a.notify("A file changed since preview; rename cancelled")
				return
			}
		}
		old := a.active
		oldDoc := a.view().Doc
		for _, p := range all {
			if !p.opened {
				a.addDocument(p.doc)
				a.startLanguage(p.doc)
			}
			a.activate(p.doc)
			a.apply(p.edits)
		}
		a.active = old
		a.activate(oldDoc)
		a.notify("Rename applied. Use Save All to write changes.")
	}}, {Label: "Cancel"}})
	a.modal.Body = body
}
