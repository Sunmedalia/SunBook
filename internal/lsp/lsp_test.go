package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/sourcegraph/jsonrpc2"
	"sunbook/internal/config"
	"sunbook/internal/text"
)

type stdio struct {
	io.Reader
	io.Writer
}

func (stdio) Close() error { return nil }

type fakeServer struct {
	contents string
	ranged   int
}

func (s *fakeServer) Handle(ctx context.Context, c *jsonrpc2.Conn, r *jsonrpc2.Request) {
	var result any
	switch r.Method {
	case "initialize":
		var sync any = 1
		if os.Getenv("SUNBOOK_LSP_SYNC") == "2" {
			sync = map[string]any{"openClose": true, "change": 2}
		}
		result = map[string]any{"capabilities": map[string]any{"positionEncoding": "utf-16", "completionProvider": map[string]any{}, "definitionProvider": true, "documentFormattingProvider": true, "textDocumentSync": sync}}
	case "textDocument/didOpen":
		var p struct {
			TextDocument struct {
				URI     string `json:"uri"`
				Version int    `json:"version"`
				Text    string `json:"text"`
			} `json:"textDocument"`
		}
		json.Unmarshal(*r.Params, &p)
		s.contents = p.TextDocument.Text
		c.Notify(ctx, "textDocument/publishDiagnostics", Diagnostics{URI: p.TextDocument.URI, Version: &p.TextDocument.Version, Diagnostics: []Diagnostic{{Message: "test diagnostic", Range: Range{Start: Position{0, 1}, End: Position{0, 2}}, Severity: 1}}})
	case "textDocument/didChange":
		var p struct {
			ContentChanges []ContentChange `json:"contentChanges"`
		}
		json.Unmarshal(*r.Params, &p)
		for _, change := range p.ContentChanges {
			if change.Range == nil {
				s.contents = change.Text
				continue
			}
			s.ranged++
			s.contents, _ = applyChange(s.contents, change)
		}
	case "test/contents":
		result = s.contents
	case "test/ranged":
		result = s.ranged
	case "test/notify":
		c.Notify(ctx, "window/logMessage", map[string]string{"message": "stall"})
	case "textDocument/completion":
		result = []Completion{{Label: "Hello"}}
	case "textDocument/definition":
		result = Location{URI: "file:///tmp/test.go", Range: Range{Start: Position{0, 0}, End: Position{0, 1}}}
	case "test/stall":
		c.Notify(ctx, "window/logMessage", map[string]string{"message": "stall"})
		return
	case "shutdown":
	case "exit":
		os.Exit(0)
	}
	if !r.Notif {
		c.Reply(ctx, r.ID, result)
	}
}
func TestServerHelper(t *testing.T) {
	if os.Getenv("SUNBOOK_LSP_HELPER") != "1" {
		return
	}
	ctx := context.Background()
	c := jsonrpc2.NewConn(ctx, jsonrpc2.NewBufferedStream(stdio{os.Stdin, os.Stdout}, jsonrpc2.VSCodeObjectCodec{}), &fakeServer{})
	<-c.DisconnectNotify()
	os.Exit(0)
}
func TestStdioLifecycleDiagnosticsAndCancellation(t *testing.T) {
	t.Setenv("SUNBOOK_LSP_HELPER", "1")
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	diags := make(chan Diagnostics, 2)
	c, e := Start(ctx, t.TempDir(), config.Server{Command: exe, Args: []string{"-test.run=TestServerHelper"}}, func(d Diagnostics) { diags <- d }, func(string) {})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	if !c.Supports("completionProvider") || c.Supports("renameProvider") {
		t.Fatal("capabilities")
	}
	c.Open("/tmp/test.go", "go", "package test", 3)
	select {
	case d := <-diags:
		if d.Version == nil || *d.Version != 3 || len(d.Diagnostics) != 1 {
			t.Fatal(d)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no diagnostics")
	}
	done := make(chan error, 1)
	c.Request(ctx, "textDocument/completion", nil, func(raw json.RawMessage, e error) {
		items, err := ParseCompletion(raw)
		if e != nil {
			done <- e
		} else if err != nil {
			done <- err
		} else if len(items) != 1 || items[0].Label != "Hello" {
			done <- fmt.Errorf("bad completion %s", raw)
		} else {
			done <- nil
		}
	})
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("request timeout")
	}
	c.Request(ctx, "test/stall", nil, func(_ json.RawMessage, e error) { done <- e })
	cancel()
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("cancel didn't stop request")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation blocked")
	}
}
func TestPositionAndResponseShapes(t *testing.T) {
	b := text.New("中😀ab\n尾")
	pos := At(b, len("中😀a"))
	if pos.Line != 0 || pos.Character != 4 || Offset(b, pos) != len("中😀a") {
		t.Fatal(pos)
	}
	path := "/tmp/space 中文#%.go"
	if Path(URI(path)) != path {
		t.Fatal("URI round trip", URI(path))
	}
	for _, raw := range []string{`[{"label":"x"}]`, `{"items":[{"label":"x"}]}`} {
		items, e := ParseCompletion(json.RawMessage(raw))
		if e != nil || len(items) != 1 {
			t.Fatal(items, e)
		}
	}
	locs, e := ParseLocations(json.RawMessage(`[{"targetUri":"file:///tmp/x.go","targetSelectionRange":{"start":{"line":2,"character":3},"end":{"line":2,"character":4}}}]`))
	if e != nil || len(locs) != 1 || locs[0].URI != "file:///tmp/x.go" || locs[0].Range.Start.Line != 2 {
		t.Fatal(locs, e)
	}
	edits, e := Edits(b, []TextEdit{{Range: Range{Start: Position{0, 1}, End: Position{0, 3}}, NewText: "🙂"}})
	if e != nil || !b.Apply(edits, nil, nil) || b.String() != "中🙂ab\n尾" {
		t.Fatal("Unicode edit", b.String(), e)
	}
}

// Optional integration suite. It uses user-installed tools and never downloads
// anything. Set SUNBOOK_LSP_INTEGRATION=1 to run all three language servers.
func TestRealServers(t *testing.T) {
	if os.Getenv("SUNBOOK_LSP_INTEGRATION") != "1" {
		t.Skip("set SUNBOOK_LSP_INTEGRATION=1 with language servers installed")
	}
	cases := []struct {
		key, name, source string
		position          Position
	}{{"go", "main.go", "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Pr }\n", Position{4, 20}}, {"python", "main.py", "import os\nos.pa\n", Position{1, 5}}, {"typescript", "main.ts", "const message = 'hello';\nmessage.toU\n", Position{1, 11}}}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			root := t.TempDir()
			if tc.key == "go" {
				os.WriteFile(root+"/go.mod", []byte("module example.invalid/test\n\ngo 1.26\n"), 0644)
			}
			path := root + "/" + tc.name
			os.WriteFile(path, []byte(tc.source), 0644)
			server := config.Default().Servers[tc.key]
			diagnostics := make(chan Diagnostics, 64)
			c, e := Start(context.Background(), root, server, func(d Diagnostics) {
				select {
				case diagnostics <- d:
				default:
				}
			}, func(string) {})
			if e != nil {
				t.Fatal(e)
			}
			defer c.Close()
			_, lang := config.Language(path)
			c.Open(path, lang, tc.source, 1)
			done := make(chan error, 1)
			c.Request(context.Background(), "textDocument/completion", map[string]any{"textDocument": map[string]string{"uri": URI(path)}, "position": tc.position}, func(raw json.RawMessage, e error) {
				if e != nil {
					done <- e
					return
				}
				items, e := ParseCompletion(raw)
				if e != nil {
					done <- e
					return
				}
				if len(items) == 0 {
					done <- fmt.Errorf("no completions at %+v: %s", tc.position, raw)
					return
				}
				var labels []string
				for _, it := range items {
					labels = append(labels, it.Label)
				}
				t.Log("completions:", strings.Join(labels[:min(5, len(labels))], ", "))
				done <- nil
			})
			select {
			case e := <-done:
				if e != nil {
					t.Fatal(e)
				}
			case <-time.After(25 * time.Second):
				t.Fatal("completion timeout")
			}
			verifyRealFeatures(t, c, tc.key, path, diagnostics)
		})
	}
}

func verifyRealFeatures(t *testing.T, c *Client, key, path string, diagnostics <-chan Diagnostics) {
	t.Helper()
	source := ""
	line := 0
	switch key {
	case "go":
		source = "package main\nfunc greet() string { return \"hi\" }\nfunc main() { println(greet()) }\n"
		line = 2
	case "python":
		source = "def greet():\n    return \"hi\"\nmessage = greet()\n"
		line = 2
	case "typescript":
		source = "function greet() { return 'hi'; }\nconst message = greet();\n"
		line = 1
	}
	c.Change(path, source, 2)
	col := strings.Index(strings.Split(source, "\n")[line], "greet") + 1
	params := map[string]any{"textDocument": map[string]string{"uri": URI(path)}, "position": Position{line, col}}
	call := func(method string, p any) json.RawMessage {
		t.Helper()
		type response struct {
			raw json.RawMessage
			err error
		}
		ch := make(chan response, 1)
		c.Request(context.Background(), method, p, func(raw json.RawMessage, e error) { ch <- response{raw, e} })
		select {
		case r := <-ch:
			if r.err != nil {
				t.Fatalf("%s: %v", method, r.err)
			}
			return r.raw
		case <-time.After(15 * time.Second):
			t.Fatalf("%s timed out", method)
			return nil
		}
	}
	locs, e := ParseLocations(call("textDocument/definition", params))
	if e != nil || len(locs) == 0 {
		t.Fatal("definition", locs, e)
	}
	if raw := call("textDocument/hover", params); string(raw) == "null" {
		t.Fatal("missing hover")
	}
	refs := map[string]any{"textDocument": params["textDocument"], "position": params["position"], "context": map[string]bool{"includeDeclaration": true}}
	locs, e = ParseLocations(call("textDocument/references", refs))
	if e != nil || len(locs) < 1 {
		t.Fatal("references", locs, e)
	}
	rename := map[string]any{"textDocument": params["textDocument"], "position": params["position"], "newName": "greeting"}
	raw := call("textDocument/rename", rename)
	var ws WorkspaceEdit
	if e = json.Unmarshal(raw, &ws); e != nil || len(ws.Changes)+len(ws.DocumentChanges) == 0 {
		t.Fatal("rename", string(raw), e)
	}
	if key == "python" {
		cmd := exec.Command("ruff", "format", "--stdin-filename", path, "-")
		cmd.Stdin = strings.NewReader(source)
		if out, e := cmd.Output(); e != nil || !strings.Contains(string(out), "greet") {
			t.Fatal("ruff formatting", string(out), e)
		}
	} else {
		var edits []TextEdit
		raw = call("textDocument/formatting", map[string]any{"textDocument": params["textDocument"], "options": map[string]any{"tabSize": 4, "insertSpaces": true}})
		if e = json.Unmarshal(raw, &edits); e != nil {
			t.Fatal("format", string(raw), e)
		}
		buf := text.New(source)
		changes, e := Edits(buf, edits)
		if e != nil {
			t.Fatal("format ranges", e)
		}
		if len(changes) > 0 && !buf.Apply(changes, nil, nil) {
			t.Fatal("format edits invalid")
		}
	}
	c.Change(path, source+"\nmissing_symbol\n", 3)
	deadline := time.After(15 * time.Second)
	for {
		select {
		case d := <-diagnostics:
			for _, diag := range d.Diagnostics {
				if strings.Contains(diag.Message, "missing_symbol") {
					t.Log("definition, hover, references, rename, format, diagnostics passed")
					return
				}
			}
		case <-deadline:
			t.Fatal("no diagnostics for missing_symbol")
		}
	}
}

// applyChange applies one ranged change the way a server would, rejecting
// positions that do not name an exact UTF-16 boundary.
func applyChange(contents string, change ContentChange) (string, error) {
	b := text.New(contents)
	a, z := Offset(b, change.Range.Start), Offset(b, change.Range.End)
	if At(b, a) != change.Range.Start || At(b, z) != change.Range.End || z < a {
		return contents, fmt.Errorf("inexact range %+v in %q", change.Range, contents)
	}
	return contents[:a] + change.Text + contents[z:], nil
}

func randomEdits(rng *rand.Rand, s string) []text.Edit {
	pieces := []string{"", "x", "\n", "中文", "😀", "é", "a\nb", "🎉\n🎉", "\n\n"}
	var offsets []int
	for i := range s {
		offsets = append(offsets, i)
	}
	offsets = append(offsets, len(s))
	var edits []text.Edit
	at := 0
	for range 1 + rng.IntN(4) {
		// Non-overlapping ranges with distinct starts, as Buffer.Apply requires.
		i := at + rng.IntN(max(1, len(offsets)-at))
		if i >= len(offsets) {
			break
		}
		j := min(len(offsets)-1, i+rng.IntN(4))
		edits = append(edits, text.Edit{Start: offsets[i], End: offsets[j], Text: pieces[rng.IntN(len(pieces))]})
		at = j + 1
	}
	return edits
}

func TestChangesReplayEditsUndoAndRedo(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for seed := range 200 {
		b := text.New("a😀b\n中文 é\n\nlast")
		for step := range 40 {
			before := b.String()
			switch r := rng.IntN(10); {
			case r < 6:
				if !b.Apply(randomEdits(rng, before), nil, nil) {
					continue
				}
			case r < 8:
				if _, ok := b.Undo(); !ok {
					continue
				}
			default:
				if _, ok := b.Redo(); !ok {
					continue
				}
			}
			changes, ok := Changes(b)
			if !ok {
				t.Fatal("no changes for an edit")
			}
			got := before
			for _, change := range changes {
				var err error
				if got, err = applyChange(got, change); err != nil {
					t.Fatalf("seed %d step %d: %v", seed, step, err)
				}
			}
			if got != b.String() {
				t.Fatalf("seed %d step %d: replay %q, buffer %q, changes %+v", seed, step, got, b.String(), changes)
			}
		}
	}
}

func TestRangeChangesMergeAndFullTextReplaces(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &Client{ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1)}
	ranged := func(version int, s string) {
		c.change("a", version, []ContentChange{{Range: &Range{}, Text: s}})
	}
	ranged(2, "x")
	ranged(3, "y")
	pending := c.jobs[0].params.(*didChangeParams)
	if len(c.jobs) != 1 || pending.TextDocument.Version != 3 || len(pending.ContentChanges) != 2 || pending.ContentChanges[1].Text != "y" {
		t.Fatal("range changes were not appended", pending)
	}
	c.Change("a", "full", 4)
	ranged(5, "z")
	pending = c.jobs[0].params.(*didChangeParams)
	if len(c.jobs) != 1 || pending.TextDocument.Version != 5 || len(pending.ContentChanges) != 2 || pending.ContentChanges[0].Range != nil || pending.ContentChanges[0].Text != "full" {
		t.Fatal("full text did not replace pending ranges", pending)
	}
	ranged(6, "other")
	c.jobs[0].changePath = "b"
	ranged(7, "split")
	if len(c.jobs) != 2 {
		t.Fatal("changes merged across documents")
	}
}

func TestIncrementalSyncKeepsServerInStep(t *testing.T) {
	t.Setenv("SUNBOOK_LSP_SYNC", "2")
	c := helperClient(t, func(string) {})
	if !c.incremental {
		t.Fatal("options-form textDocumentSync change kind was not detected")
	}
	request := func(method string, out any) {
		t.Helper()
		done := make(chan error, 1)
		c.Request(context.Background(), method, nil, func(raw json.RawMessage, err error) {
			if err == nil {
				err = json.Unmarshal(raw, out)
			}
			done <- err
		})
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	rng := rand.New(rand.NewPCG(3, 4))
	path := "/tmp/incremental.go"
	b := text.New("package main\n// 中文 😀\n")
	c.OpenBuffer(path, "go", b)
	for range 300 {
		if rng.IntN(5) == 0 {
			b.Undo()
		} else {
			b.Apply(randomEdits(rng, b.String()), nil, nil)
		}
		c.SyncBuffer(path, b)
	}
	var contents string
	var ranged int
	request("test/contents", &contents)
	request("test/ranged", &ranged)
	if contents != b.String() || ranged == 0 {
		t.Fatalf("server drifted (ranged %d): %q != %q", ranged, contents, b.String())
	}
	// A reloaded document is a new buffer; it must resend its full text.
	reloaded := text.New("reloaded\n")
	reloaded.Version = b.Version + 1
	c.SyncBuffer(path, reloaded)
	reloaded.Apply([]text.Edit{{Start: 0, End: 0, Text: "// "}}, nil, nil)
	c.SyncBuffer(path, reloaded)
	request("test/contents", &contents)
	if contents != "// reloaded\n" {
		t.Fatalf("reload was not sent in full: %q", contents)
	}
}

// Optional: after random edits, undo and multi-cursor renames sent as ranges,
// each server must report the same symbols as a fresh full-text open.
func TestRealServersIncrementalSync(t *testing.T) {
	if os.Getenv("SUNBOOK_LSP_INTEGRATION") != "1" {
		t.Skip("set SUNBOOK_LSP_INTEGRATION=1 with language servers installed")
	}
	for _, tc := range []struct{ key, name, source, function string }{
		{"go", "main.go", "package main\n\n// 中文 😀\nfunc main() {}\n", "func f%d() { _ = \"😀 中文 é\" }\n"},
		{"python", "main.py", "# 中文 😀\ndef main():\n    pass\n", "def f%d():  # 😀 中文\n    return 'é'\n"},
		{"typescript", "main.ts", "// 中文 😀\nfunction main() {}\n", "function f%d() { return '😀 中文 é'; }\n"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			server := config.Default().Servers[tc.key]
			if _, err := exec.LookPath(server.Command); err != nil {
				t.Skip(server.Command, " is not installed")
			}
			root := t.TempDir()
			os.WriteFile(root+"/go.mod", []byte("module example.invalid/test\n\ngo 1.26\n"), 0644)
			path := root + "/" + tc.name
			os.WriteFile(path, []byte(tc.source), 0644)
			start := func() *Client {
				c, e := Start(context.Background(), root, server, func(Diagnostics) {}, func(string) {})
				if e != nil {
					t.Fatal(e)
				}
				t.Cleanup(c.Close)
				return c
			}
			symbols := func(c *Client) string {
				t.Helper()
				for range 75 {
					done := make(chan string, 1)
					c.Request(context.Background(), "textDocument/documentSymbol", map[string]any{"textDocument": map[string]string{"uri": URI(path)}}, func(raw json.RawMessage, err error) {
						var v any
						if err != nil || json.Unmarshal(raw, &v) != nil || v == nil || len(raw) <= 2 {
							done <- ""
							return
						}
						out, _ := json.Marshal(v)
						done <- string(out)
					})
					if s := <-done; s != "" {
						return s
					}
					time.Sleep(200 * time.Millisecond)
				}
				t.Fatal("no document symbols")
				return ""
			}
			c := start()
			if !c.incremental {
				t.Skip(server.Command, " does not request incremental sync")
			}
			_, lang := config.Language(path)
			b := text.New(tc.source)
			c.OpenBuffer(path, lang, b)
			rng := rand.New(rand.NewPCG(5, 6))
			for range 300 {
				changed := false
				if rng.IntN(4) == 0 {
					_, changed = b.Undo()
				} else {
					changed = b.Apply(randomEdits(rng, b.String()), nil, nil)
				}
				if changed {
					c.SyncBuffer(path, b)
				}
			}
			for {
				if _, ok := b.Undo(); !ok {
					break
				}
				c.SyncBuffer(path, b)
			}
			for i := range 20 {
				end := b.Len()
				b.Apply([]text.Edit{{Start: end, End: end, Text: fmt.Sprintf(tc.function, i)}}, nil, nil)
				c.SyncBuffer(path, b)
			}
			var renames []text.Edit
			for _, i := range []int{3, 7, 11} {
				name := fmt.Sprintf("f%d(", i)
				at := strings.Index(b.String(), name)
				renames = append(renames, text.Edit{Start: at, End: at + len(name) - 1, Text: "renamed_中文_" + name[:len(name)-1]})
			}
			if !b.Apply(renames, nil, nil) {
				t.Fatal("rename edits")
			}
			c.SyncBuffer(path, b)
			got := symbols(c)
			fresh := start()
			fresh.Open(path, lang, b.String(), 1)
			if want := symbols(fresh); got != want || !strings.Contains(got, "renamed_中文_f7") {
				t.Fatalf("incremental document drifted\ngot  %s\nwant %s", got, want)
			}
		})
	}
}

func TestSyncBufferSkipsUnchangedVersions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &Client{ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1)}
	b := text.New("x")
	c.OpenBuffer("a", "go", b)
	c.SyncBuffer("a", b)
	if len(c.jobs) != 1 {
		t.Fatal("unchanged buffer resent with a repeated version", len(c.jobs))
	}
	b.Apply([]text.Edit{{Start: 1, End: 1, Text: "y"}}, nil, nil)
	c.SyncBuffer("a", b)
	// Without incremental support the server still receives the full text.
	if len(c.jobs) != 2 || c.jobs[1].params.(*didChangeParams).ContentChanges[0].Text != "xy" {
		t.Fatal("full-text fallback", c.jobs)
	}
	c.CloseDocument("a")
	if len(c.synced) != 0 {
		t.Fatal("closed document still retains its buffer")
	}
}
