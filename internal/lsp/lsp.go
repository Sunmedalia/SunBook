// Package lsp is a stdio LSP client. The UI never blocks on a server request;
// each client serializes notifications and requests on its own work queue.
package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sourcegraph/jsonrpc2"
	"sunbook/internal/config"
	"sunbook/internal/text"
)

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}
type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity"`
	Message  string `json:"message"`
}
type Diagnostics struct {
	URI         string       `json:"uri"`
	Version     *int         `json:"version"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}
type Location struct {
	URI                  string `json:"uri"`
	Range                Range  `json:"range"`
	TargetURI            string `json:"targetUri"`
	TargetSelectionRange Range  `json:"targetSelectionRange"`
}
type Completion struct {
	Label            string `json:"label"`
	Detail           string `json:"detail"`
	InsertText       string `json:"insertText"`
	InsertTextFormat int    `json:"insertTextFormat"`
	TextEdit         *struct {
		Range   *Range `json:"range"`
		Insert  *Range `json:"insert"`
		Replace *Range `json:"replace"`
		NewText string `json:"newText"`
	} `json:"textEdit"`
	AdditionalTextEdits []TextEdit `json:"additionalTextEdits"`
}
type WorkspaceEdit struct {
	Changes         map[string][]TextEdit `json:"changes"`
	DocumentChanges []struct {
		TextDocument struct {
			URI     string `json:"uri"`
			Version *int   `json:"version"`
		} `json:"textDocument"`
		Edits []TextEdit `json:"edits"`
		Kind  string     `json:"kind"`
	} `json:"documentChanges"`
}

func URI(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}
func Path(uri string) string {
	u, e := url.Parse(uri)
	if e != nil || u.Scheme != "file" || u.Host != "" && u.Host != "localhost" {
		return ""
	}
	return filepath.FromSlash(u.Path)
}
func At(b *text.Buffer, p int) Position     { r, c := b.LSPPosition(p); return Position{r, c} }
func Offset(b *text.Buffer, p Position) int { return b.LSPOffset(p.Line, p.Character) }
func Edits(b *text.Buffer, edits []TextEdit) ([]text.Edit, error) {
	out := make([]text.Edit, 0, len(edits))
	for _, e := range edits {
		if e.Range.Start.Line < 0 || e.Range.Start.Line >= b.LineCount() || e.Range.End.Line < 0 || e.Range.End.Line >= b.LineCount() || e.Range.Start.Character < 0 || e.Range.End.Character < 0 {
			return nil, errors.New("server returned an invalid edit range")
		}
		a, z := Offset(b, e.Range.Start), Offset(b, e.Range.End)
		if At(b, a) != e.Range.Start || At(b, z) != e.Range.End {
			return nil, errors.New("server returned an edit inside a surrogate or outside a line")
		}
		if z < a {
			return nil, errors.New("server returned a reversed edit range")
		}
		out = append(out, text.Edit{Start: a, End: z, Text: e.NewText})
	}
	return out, nil
}

type transport struct {
	io.ReadCloser
	writer io.WriteCloser
	once   sync.Once
}

func (t *transport) Write(p []byte) (int, error) { return t.writer.Write(p) }
func (t *transport) Close() error {
	t.once.Do(func() { _ = t.writer.Close(); _ = t.ReadCloser.Close() })
	return nil
}

type job struct {
	method   string
	params   any
	callback func(json.RawMessage, error)
}
type Client struct {
	conn          *jsonrpc2.Conn
	cmd           *exec.Cmd
	ctx           context.Context
	cancel        context.CancelFunc
	jobs          chan job
	requestID     uint64
	done          chan struct{}
	root          string
	settings      map[string]any
	Capabilities  map[string]json.RawMessage
	onDiagnostics func(Diagnostics)
	onMessage     func(string)
	once          sync.Once
}

func Start(parent context.Context, root string, s config.Server, onDiagnostics func(Diagnostics), onMessage func(string)) (*Client, error) {
	ctx, cancel := context.WithCancel(parent)
	c := &Client{ctx: ctx, cancel: cancel, root: root, settings: s.Settings, jobs: make(chan job, 1024), done: make(chan struct{}), onDiagnostics: onDiagnostics, onMessage: onMessage}
	cmd := exec.CommandContext(ctx, s.Command, s.Args...)
	cmd.Dir = root
	c.cmd = cmd
	in, e := cmd.StdinPipe()
	if e != nil {
		cancel()
		return nil, e
	}
	out, e := cmd.StdoutPipe()
	if e != nil {
		_ = in.Close()
		cancel()
		return nil, e
	}
	cmd.Stderr = io.Discard
	if e = cmd.Start(); e != nil {
		_ = in.Close()
		_ = out.Close()
		cancel()
		return nil, e
	}
	c.conn = jsonrpc2.NewConn(ctx, jsonrpc2.NewBufferedStream(&transport{ReadCloser: out, writer: in}, jsonrpc2.VSCodeObjectCodec{}), jsonrpc2.AsyncHandler(c))
	go func() {
		err := cmd.Wait()
		close(c.done)
		if ctx.Err() == nil {
			onMessage(fmt.Sprintf("%s exited: %v", s.Command, err))
		}
		_ = c.conn.Close()
	}()
	initCtx, initCancel := context.WithTimeout(ctx, 15*time.Second)
	defer initCancel()
	var result struct {
		Capabilities map[string]json.RawMessage `json:"capabilities"`
	}
	params := map[string]any{"processId": os.Getpid(), "rootUri": URI(root), "clientInfo": map[string]any{"name": "SunBook", "version": "0.1.0"}, "workspaceFolders": []map[string]any{{"uri": URI(root), "name": filepath.Base(root)}}, "capabilities": map[string]any{
		"general":      map[string]any{"positionEncodings": []string{"utf-16"}},
		"workspace":    map[string]any{"configuration": true, "workspaceFolders": true, "workspaceEdit": map[string]any{"documentChanges": true, "resourceOperations": []string{}}},
		"textDocument": map[string]any{"synchronization": map[string]any{"didSave": true}, "completion": map[string]any{"completionItem": map[string]any{"snippetSupport": false, "insertReplaceSupport": true}}, "publishDiagnostics": map[string]any{"versionSupport": true}},
	}}
	if s.InitializationOptions != nil {
		params["initializationOptions"] = s.InitializationOptions
	}
	if e = c.conn.Call(initCtx, "initialize", params, &result); e != nil {
		c.Close()
		return nil, fmt.Errorf("initialize %s: %w", s.Command, e)
	}
	c.Capabilities = result.Capabilities
	if raw := c.Capabilities["positionEncoding"]; len(raw) > 0 && string(raw) != "\"utf-16\"" {
		c.Close()
		return nil, errors.New("language server did not select UTF-16 positions")
	}
	if e = c.conn.Notify(ctx, "initialized", map[string]any{}); e != nil {
		c.Close()
		return nil, e
	}
	if len(s.Settings) > 0 {
		_ = c.conn.Notify(ctx, "workspace/didChangeConfiguration", map[string]any{"settings": s.Settings})
	}
	go c.loop()
	return c, nil
}
func (c *Client) Supports(key string) bool {
	raw := c.Capabilities[key]
	return len(raw) > 0 && string(raw) != "false" && string(raw) != "null"
}
func (c *Client) loop() {
	for {
		select {
		case <-c.ctx.Done():
			return
		case j := <-c.jobs:
			ctx, cancel := context.WithTimeout(c.ctx, 8*time.Second)
			if j.callback == nil {
				if e := c.conn.Notify(ctx, j.method, j.params); e != nil && c.ctx.Err() == nil {
					c.onMessage(e.Error())
				}
			} else {
				var result json.RawMessage
				c.requestID++
				id := fmt.Sprintf("sunbook-%d", c.requestID)
				err := c.conn.Call(ctx, j.method, j.params, &result, jsonrpc2.PickID(jsonrpc2.ID{Str: id, IsString: true}))
				if ctx.Err() != nil && c.ctx.Err() == nil {
					cancelCtx, stop := context.WithTimeout(c.ctx, time.Second)
					_ = c.conn.Notify(cancelCtx, "$/cancelRequest", map[string]string{"id": id})
					stop()
				}
				j.callback(result, err)
			}
			cancel()
		}
	}
}
func (c *Client) Done() <-chan struct{} { return c.done }
func (c *Client) Notify(method string, p any) {
	select {
	case c.jobs <- job{method: method, params: p}:
	case <-c.ctx.Done():
	}
}
func (c *Client) Request(method string, p any, cb func(json.RawMessage, error)) {
	// A request cancelled while still queued must complete its callback too.
	var once sync.Once
	done := make(chan struct{})
	wrapped := func(raw json.RawMessage, e error) { once.Do(func() { close(done); cb(raw, e) }) }
	go func() {
		select {
		case <-c.ctx.Done():
			wrapped(nil, c.ctx.Err())
		case <-done:
		}
	}()
	select {
	case c.jobs <- job{method, p, wrapped}:
	case <-c.ctx.Done():
		wrapped(nil, c.ctx.Err())
	}
}
func (c *Client) Open(path, lang, contents string, version int) {
	c.Notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{"uri": URI(path), "languageId": lang, "version": version, "text": contents}})
}
func (c *Client) Change(path, contents string, version int) {
	c.Notify("textDocument/didChange", map[string]any{"textDocument": map[string]any{"uri": URI(path), "version": version}, "contentChanges": []map[string]string{{"text": contents}}})
}
func (c *Client) Close() {
	c.once.Do(func() {
		if c.conn != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
			_ = c.conn.Call(ctx, "shutdown", nil, nil)
			_ = c.conn.Notify(ctx, "exit", nil)
			cancel()
		}
		c.cancel()
		if c.conn != nil {
			_ = c.conn.Close()
		}
		if c.cmd != nil && c.cmd.Process != nil {
			select {
			case <-c.done:
			case <-time.After(300 * time.Millisecond):
				_ = c.cmd.Process.Kill()
			}
		}
	})
}

func (c *Client) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	var result any
	var rpcErr *jsonrpc2.Error
	switch req.Method {
	case "textDocument/publishDiagnostics":
		var d Diagnostics
		if req.Params != nil && json.Unmarshal(*req.Params, &d) == nil {
			c.onDiagnostics(d)
		}
	case "window/logMessage", "window/showMessage":
		var p struct {
			Message string `json:"message"`
		}
		if req.Params != nil && json.Unmarshal(*req.Params, &p) == nil {
			c.onMessage(p.Message)
		}
	case "workspace/configuration":
		var p struct {
			Items []struct {
				Section string `json:"section"`
			} `json:"items"`
		}
		if req.Params != nil {
			_ = json.Unmarshal(*req.Params, &p)
		}
		items := make([]any, len(p.Items))
		for i, item := range p.Items {
			var v any = c.settings
			for _, part := range strings.Split(item.Section, ".") {
				if part == "" {
					continue
				}
				m, ok := v.(map[string]any)
				if !ok {
					v = nil
					break
				}
				v = m[part]
			}
			items[i] = v
		}
		result = items
	case "workspace/workspaceFolders":
		result = []map[string]string{{"uri": URI(c.root), "name": filepath.Base(c.root)}}
	case "window/workDoneProgress/create":
	case "workspace/applyEdit":
		result = map[string]any{"applied": false, "failureReason": "server initiated edits are not supported; use Rename or Format"}
	default:
		if !req.Notif {
			rpcErr = &jsonrpc2.Error{Code: jsonrpc2.CodeMethodNotFound, Message: "unsupported method: " + req.Method}
		}
	}
	if !req.Notif {
		if rpcErr != nil {
			_ = conn.ReplyWithError(ctx, req.ID, rpcErr)
		} else {
			_ = conn.Reply(ctx, req.ID, result)
		}
	}
}

func ParseCompletion(raw json.RawMessage) ([]Completion, error) {
	var items []Completion
	if len(raw) == 0 || string(raw) == "null" {
		return items, nil
	}
	if raw[0] == '[' {
		err := json.Unmarshal(raw, &items)
		return items, err
	}
	var list struct {
		Items []Completion `json:"items"`
	}
	e := json.Unmarshal(raw, &list)
	return list.Items, e
}
func ParseLocations(raw json.RawMessage) ([]Location, error) {
	if string(raw) == "null" || len(raw) == 0 {
		return nil, nil
	}
	var list []Location
	if raw[0] == '[' {
		if e := json.Unmarshal(raw, &list); e != nil {
			return nil, e
		}
	} else {
		var loc Location
		if e := json.Unmarshal(raw, &loc); e != nil {
			return nil, e
		}
		list = []Location{loc}
	}
	for i, l := range list {
		if l.URI == "" {
			list[i].URI = l.TargetURI
			list[i].Range = l.TargetSelectionRange
		}
	}
	return list, nil
}
