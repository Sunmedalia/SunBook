// Package lsp is a stdio LSP client. The UI never blocks on a server request;
// each client sends ordered messages and waits for request responses concurrently.
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

// ContentChange is one didChange entry; a nil Range replaces the whole document.
type ContentChange struct {
	Range *Range `json:"range,omitempty"`
	Text  string `json:"text"`
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

// Changes describes the buffer's last transaction as sequential LSP changes.
// Edits are sent left to right: the text before each edit already matches the
// new buffer, and the removed text gives the end of the replaced range.
func Changes(b *text.Buffer) ([]ContentChange, bool) {
	if len(b.LastEdits) == 0 || len(b.LastRemoved) != len(b.LastEdits) {
		return nil, false
	}
	changes := make([]ContentChange, len(b.LastEdits))
	delta := 0
	for i, e := range b.LastEdits {
		start := At(b, e.Start+delta)
		end := start
		removed := b.LastRemoved[i]
		if n := strings.LastIndexByte(removed, '\n'); n >= 0 {
			end = Position{start.Line + strings.Count(removed, "\n"), utf16Len(removed[n+1:])}
		} else {
			end.Character += utf16Len(removed)
		}
		changes[i] = ContentChange{Range: &Range{start, end}, Text: e.Text}
		delta += len(e.Text) - (e.End - e.Start)
	}
	return changes, true
}
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r > 0xffff {
			n++
		}
	}
	return n
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

var ErrBusy = errors.New("language server queue is busy")

type job struct {
	ctx        context.Context
	changePath string
	method     string
	params     any
	callback   func(json.RawMessage, error)
}
type versionedDocument struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}
type didChangeParams struct {
	TextDocument   versionedDocument `json:"textDocument"`
	ContentChanges []ContentChange   `json:"contentChanges"`
}

// syncedBuffer is the buffer state the server last received for a path.
type syncedBuffer struct {
	buffer  *text.Buffer
	version int
}
type Client struct {
	conn          *jsonrpc2.Conn
	cmd           *exec.Cmd
	ctx           context.Context
	cancel        context.CancelFunc
	queueMu       sync.Mutex
	jobs          []job
	wake          chan struct{}
	active        chan struct{}
	requestID     uint64
	done          chan struct{}
	root          string
	settings      map[string]any
	Capabilities  map[string]json.RawMessage
	onDiagnostics func(Diagnostics)
	onMessage     func(string)
	incremental   bool
	synced        map[string]syncedBuffer
	once          sync.Once
	failure       sync.Once
	writeTimeout  time.Duration
}

func Start(parent context.Context, root string, s config.Server, onDiagnostics func(Diagnostics), onMessage func(string)) (*Client, error) {
	ctx, cancel := context.WithCancel(parent)
	c := &Client{ctx: ctx, cancel: cancel, root: root, settings: s.Settings, wake: make(chan struct{}, 1), active: make(chan struct{}, 8), done: make(chan struct{}), onDiagnostics: onDiagnostics, onMessage: onMessage}
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
		unexpected := ctx.Err() == nil
		cancel()
		close(c.done)
		_ = c.conn.Close()
		if unexpected {
			onMessage(fmt.Sprintf("%s exited: %v", s.Command, err))
		}
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
	c.incremental = syncKind(c.Capabilities["textDocumentSync"]) == 2
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

// textDocumentSync is either a TextDocumentSyncKind or an options object.
func syncKind(raw json.RawMessage) int {
	var kind int
	if json.Unmarshal(raw, &kind) == nil {
		return kind
	}
	var options struct {
		Change int `json:"change"`
	}
	_ = json.Unmarshal(raw, &options)
	return options.Change
}
func (c *Client) Supports(key string) bool {
	raw := c.Capabilities[key]
	return len(raw) > 0 && string(raw) != "false" && string(raw) != "null"
}

// enqueue never waits on server IO. Only adjacent changes to one document are
// merged: a full-document change replaces the pending one and range changes
// are appended. Requests, saves and closes are ordering barriers.
func (c *Client) enqueue(j job) bool {
	c.queueMu.Lock()
	defer c.queueMu.Unlock()
	if c.ctx.Err() != nil {
		return false
	}
	n := len(c.jobs)
	merged := j.changePath != "" && n > 0 && c.jobs[n-1].changePath == j.changePath && mergeChanges(&c.jobs[n-1], j)
	if !merged {
		if n >= 1024 {
			return false
		}
		c.jobs = append(c.jobs, j)
	}
	select {
	case c.wake <- struct{}{}:
	default:
	}
	return true
}
func mergeChanges(pending *job, next job) bool {
	before, ok := pending.params.(*didChangeParams)
	after, ok2 := next.params.(*didChangeParams)
	if !ok || !ok2 {
		return false
	}
	if after.ContentChanges[0].Range == nil {
		*pending = next
		return true
	}
	if len(before.ContentChanges)+len(after.ContentChanges) > 1024 {
		return false
	}
	before.TextDocument.Version = after.TextDocument.Version
	before.ContentChanges = append(before.ContentChanges, after.ContentChanges...)
	return true
}
func (c *Client) nextJob() (job, bool) {
	c.queueMu.Lock()
	defer c.queueMu.Unlock()
	if len(c.jobs) == 0 {
		return job{}, false
	}
	j := c.jobs[0]
	c.jobs[0] = job{}
	c.jobs = c.jobs[1:]
	return j, true
}
func (c *Client) fail(message string) {
	c.failure.Do(func() {
		c.cancel()
		if c.conn != nil {
			_ = c.conn.Close()
		}
		if c.onMessage != nil {
			go c.onMessage(message + "; use Restart Language Servers")
		}
	})
}

// Context cancellation alone cannot interrupt a blocked stdio Write. Closing
// the transport prevents a non-reading server from retaining the dispatcher.
func (c *Client) guardWrite(ctx context.Context) func() bool {
	return context.AfterFunc(ctx, func() {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) && c.ctx.Err() == nil {
			c.fail("Language server transport stalled")
		}
	})
}
func (c *Client) writeContext() (context.Context, context.CancelFunc) {
	timeout := c.writeTimeout
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	return context.WithTimeout(c.ctx, timeout)
}
func (c *Client) loop() {
	defer func() {
		c.queueMu.Lock()
		clear(c.jobs)
		c.jobs = nil
		c.queueMu.Unlock()
	}()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-c.wake:
		}
		for {
			if c.ctx.Err() != nil {
				return
			}
			j, ok := c.nextJob()
			if !ok {
				break
			}
			if j.callback == nil {
				ctx, cancel := c.writeContext()
				stop := c.guardWrite(ctx)
				err := c.conn.Notify(ctx, j.method, j.params)
				stop()
				cancel()
				if err != nil && c.ctx.Err() == nil {
					c.fail(err.Error())
				}
				continue
			}
			if j.ctx.Err() != nil {
				j.callback(nil, j.ctx.Err())
				continue
			}
			select {
			case c.active <- struct{}{}:
			default:
				j.callback(nil, ErrBusy)
				continue
			}
			c.requestID++
			id := fmt.Sprintf("sunbook-%d", c.requestID)
			// User cancellation completes the callback immediately, but must not
			// close a healthy transport while a message is being written.
			writeCtx, finishWrite := c.writeContext()
			stop := c.guardWrite(writeCtx)
			waiter, err := c.conn.DispatchCall(writeCtx, j.method, j.params, jsonrpc2.PickID(jsonrpc2.ID{Str: id, IsString: true}))
			stop()
			finishWrite()
			if err != nil {
				<-c.active
				j.callback(nil, err)
				continue
			}
			go func() {
				defer func() { <-c.active }()
				var raw json.RawMessage
				err := waiter.Wait(j.ctx, &raw)
				if j.ctx.Err() != nil && c.ctx.Err() == nil {
					c.Notify("$/cancelRequest", map[string]string{"id": id})
				}
				j.callback(raw, err)
			}()
		}
	}
}
func (c *Client) Done() <-chan struct{} { return c.done }
func (c *Client) Notify(method string, p any) {
	if !c.enqueue(job{method: method, params: p}) && c.ctx.Err() == nil {
		c.fail("Language server notification queue overflow")
	}
}
func (c *Client) Request(parent context.Context, method string, p any, cb func(json.RawMessage, error)) {
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	stopClient := context.AfterFunc(c.ctx, cancel)
	var once sync.Once
	done := make(chan struct{})
	wrapped := func(raw json.RawMessage, err error) {
		once.Do(func() { close(done); stopClient(); cancel(); cb(raw, err) })
	}
	go func() {
		select {
		case <-ctx.Done():
			wrapped(nil, ctx.Err())
		case <-done:
		}
	}()
	if !c.enqueue(job{ctx: ctx, method: method, params: p, callback: wrapped}) {
		err := error(ErrBusy)
		if c.ctx.Err() != nil {
			err = c.ctx.Err()
		}
		go wrapped(nil, err)
	}
}
func (c *Client) Open(path, lang, contents string, version int) {
	c.Notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{"uri": URI(path), "languageId": lang, "version": version, "text": contents}})
}
func (c *Client) Change(path, contents string, version int) {
	c.change(path, version, []ContentChange{{Text: contents}})
}
func (c *Client) change(path string, version int, changes []ContentChange) {
	params := &didChangeParams{TextDocument: versionedDocument{URI: URI(path), Version: version}, ContentChanges: changes}
	if !c.enqueue(job{method: "textDocument/didChange", changePath: path, params: params}) && c.ctx.Err() == nil {
		c.fail("Language server change queue overflow")
	}
}

// OpenBuffer, SyncBuffer and CloseDocument track what the server has seen, so
// a single edit to that same buffer can be sent as ranges. Anything else, such
// as a reloaded buffer or a skipped version, falls back to the full text.
func (c *Client) OpenBuffer(path, lang string, b *text.Buffer) {
	c.track(path, b)
	c.Open(path, lang, b.String(), b.Version)
}
func (c *Client) SyncBuffer(path string, b *text.Buffer) {
	previous := c.track(path, b)
	if previous.buffer == b && b.Version == previous.version {
		// Unchanged; servers reject a repeated version.
		return
	}
	if c.incremental && previous.buffer == b && b.Version == previous.version+1 {
		if changes, ok := Changes(b); ok {
			c.change(path, b.Version, changes)
			return
		}
	}
	c.Change(path, b.String(), b.Version)
}
func (c *Client) CloseDocument(path string) {
	c.queueMu.Lock()
	delete(c.synced, path)
	c.queueMu.Unlock()
	c.Notify("textDocument/didClose", map[string]any{"textDocument": map[string]string{"uri": URI(path)}})
}
func (c *Client) track(path string, b *text.Buffer) syncedBuffer {
	c.queueMu.Lock()
	defer c.queueMu.Unlock()
	if c.synced == nil {
		c.synced = map[string]syncedBuffer{}
	}
	previous := c.synced[path]
	c.synced[path] = syncedBuffer{b, b.Version}
	return previous
}
func (c *Client) Close() {
	c.once.Do(func() {
		if c.conn != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
			stop := context.AfterFunc(ctx, func() { _ = c.conn.Close() })
			_ = c.conn.Call(ctx, "shutdown", nil, nil)
			_ = c.conn.Notify(ctx, "exit", nil)
			stop()
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
