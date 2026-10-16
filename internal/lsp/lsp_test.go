package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

type fakeServer struct{}

func (fakeServer) Handle(ctx context.Context, c *jsonrpc2.Conn, r *jsonrpc2.Request) {
	var result any
	switch r.Method {
	case "initialize":
		result = map[string]any{"capabilities": map[string]any{"positionEncoding": "utf-16", "completionProvider": map[string]any{}, "definitionProvider": true, "documentFormattingProvider": true, "textDocumentSync": 1}}
	case "textDocument/didOpen":
		var p struct {
			TextDocument struct {
				URI     string `json:"uri"`
				Version int    `json:"version"`
			} `json:"textDocument"`
		}
		json.Unmarshal(*r.Params, &p)
		c.Notify(ctx, "textDocument/publishDiagnostics", Diagnostics{URI: p.TextDocument.URI, Version: &p.TextDocument.Version, Diagnostics: []Diagnostic{{Message: "test diagnostic", Range: Range{Start: Position{0, 1}, End: Position{0, 2}}, Severity: 1}}})
	case "textDocument/completion":
		result = []Completion{{Label: "Hello"}}
	case "textDocument/definition":
		result = Location{URI: "file:///tmp/test.go", Range: Range{Start: Position{0, 0}, End: Position{0, 1}}}
	case "test/stall":
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
	c := jsonrpc2.NewConn(ctx, jsonrpc2.NewBufferedStream(stdio{os.Stdin, os.Stdout}, jsonrpc2.VSCodeObjectCodec{}), fakeServer{})
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
	c.Request("textDocument/completion", nil, func(raw json.RawMessage, e error) {
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
	c.Request("test/stall", nil, func(_ json.RawMessage, e error) { done <- e })
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
			c.Request("textDocument/completion", map[string]any{"textDocument": map[string]string{"uri": URI(path)}, "position": tc.position}, func(raw json.RawMessage, e error) {
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
		c.Request(method, p, func(raw json.RawMessage, e error) { ch <- response{raw, e} })
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
