package plugin

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
)

type Position struct {
	Anchor int `json:"anchor"`
	Head   int `json:"head"`
}
type Document struct {
	Path       string     `json:"path"`
	Version    int        `json:"version"`
	Text       string     `json:"text"`
	Selections []Position `json:"selections"`
}
type Invocation struct {
	Command  string    `json:"command"`
	Document *Document `json:"document,omitempty"`
}
type Edit struct {
	Start int    `json:"start"`
	End   int    `json:"end"`
	Text  string `json:"text"`
}
type Result struct {
	Title   string `json:"title,omitempty"`
	Message string `json:"message,omitempty"`
	Body    string `json:"body,omitempty"`
	Edits   []Edit `json:"edits,omitempty"`
}
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string { return e.Message }

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      uint64          `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *RPCError       `json:"error"`
}
type boundedLog struct {
	sync.Mutex
	data     []byte
	window   time.Time
	bytes    int
	overflow func(error)
}

func (l *boundedLog) Write(p []byte) (int, error) {
	l.Lock()
	n := len(p)
	now := time.Now()
	if now.Sub(l.window) >= time.Second {
		l.window = now
		l.bytes = 0
	}
	l.bytes += n
	over := l.bytes > 1024*1024
	if len(p) > 16384 {
		p = p[len(p)-16384:]
	}
	l.data = append(l.data, p...)
	if len(l.data) > 16384 {
		l.data = append([]byte(nil), l.data[len(l.data)-16384:]...)
	}
	callback := l.overflow
	l.Unlock()
	if over {
		err := errors.New("plugin stderr exceeds 1 MiB per second")
		if callback != nil {
			callback(err)
		}
		return n, err
	}
	return n, nil
}
func (l *boundedLog) String() string { l.Lock(); defer l.Unlock(); return string(l.data) }

type process struct {
	cmd       *exec.Cmd
	in        io.WriteCloser
	out       io.ReadCloser
	responses chan response
	readErr   chan error
	done      chan struct{}
	log       boundedLog
	seq       uint64
	once      sync.Once
	scratch   string
}

func startProcess(entry Entry) (*process, error) {
	current, err := ReadManifest(entry.Directory)
	if err != nil {
		return nil, fmt.Errorf("plugin package verification failed: %w", err)
	}
	if current.Fingerprint != entry.Fingerprint {
		return nil, errors.New("plugin package changed; refresh and review it before running")
	}
	argv := entry.Manifest.Command
	executable := argv[0]
	if filepath.IsAbs(executable) || strings.ContainsRune(executable, filepath.Separator) {
		if !filepath.IsAbs(executable) {
			executable = filepath.Join(entry.Directory, executable)
		}
	} else {
		var err error
		executable, err = exec.LookPath(executable)
		if err != nil {
			return nil, err
		}
	}
	// Keep the logical executable path: CPython uses it to detect a virtualenv.
	// The sandbox separately allows the resolved binary and runtime libraries.
	executable, err = filepath.Abs(executable)
	if err != nil {
		return nil, err
	}
	scratch, err := os.MkdirTemp("", "sunbook-plugin-"+entry.Manifest.ID+"-*")
	if err != nil {
		return nil, err
	}
	canonicalScratch, err := filepath.EvalSymlinks(scratch)
	if err != nil {
		os.RemoveAll(scratch)
		return nil, err
	}
	scratch = canonicalScratch
	started := false
	defer func() {
		if !started {
			os.RemoveAll(scratch)
		}
	}()
	cmd, err := isolatedCommand(entry, executable, scratch)
	if err != nil {
		return nil, err
	}
	cmd.Dir = entry.Directory
	cmd.Env = pluginEnvironment(scratch, executable)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 500 * time.Millisecond
	p := &process{cmd: cmd, scratch: scratch, responses: make(chan response, 1), readErr: make(chan error, 1), done: make(chan struct{})}
	p.in, err = cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	p.out, err = cmd.StdoutPipe()
	if err != nil {
		p.in.Close()
		return nil, err
	}
	p.log.overflow = func(err error) {
		select {
		case p.readErr <- err:
		default:
		}
	}
	cmd.Stderr = &p.log
	if err = cmd.Start(); err != nil {
		p.in.Close()
		p.out.Close()
		return nil, err
	}
	started = true
	go func() {
		defer func() {
			if r := recover(); r != nil {
				select {
				case p.readErr <- fmt.Errorf("plugin response panic: %v", r):
				default:
				}
			}
		}()
		scanner := bufio.NewScanner(p.out)
		scanner.Buffer(make([]byte, 4096), MaxMessage)
		for scanner.Scan() {
			var r response
			if !utf8.Valid(scanner.Bytes()) {
				select {
				case p.readErr <- errors.New("plugin output is not UTF-8"):
				default:
				}
				return
			}
			if err := json.Unmarshal(scanner.Bytes(), &r); err != nil {
				select {
				case p.readErr <- fmt.Errorf("invalid plugin JSON: %w", err):
				default:
				}
				return
			}
			select {
			case p.responses <- r:
			case <-p.done:
				return
			}
		}
		err := scanner.Err()
		if err == nil {
			err = io.EOF
		}
		select {
		case p.readErr <- err:
		default:
		}
	}()
	go func() { _ = cmd.Wait(); close(p.done) }()
	return p, nil
}
func (p *process) stop() {
	p.once.Do(func() {
		if p.cmd.Process != nil {
			_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
		}
		p.in.Close()
		p.out.Close()
		os.RemoveAll(p.scratch)
	})
}
func (p *process) call(ctx context.Context, method string, params any, result any) error {
	p.seq++
	raw, err := json.Marshal(struct {
		JSONRPC string `json:"jsonrpc"`
		ID      uint64 `json:"id"`
		Method  string `json:"method"`
		Params  any    `json:"params"`
	}{"2.0", p.seq, method, params})
	if err != nil {
		return err
	}
	if len(raw)+1 > MaxMessage {
		return &RPCError{Code: -32602, Message: "plugin request exceeds 4 MiB; use a smaller document"}
	}
	written := make(chan error, 1)
	go func() { _, err := io.Copy(p.in, bytes.NewReader(append(raw, '\n'))); written <- err }()
	select {
	case err = <-written:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		p.stop()
		return ctx.Err()
	case <-p.done:
		return errors.New("plugin process exited")
	}
	select {
	case r := <-p.responses:
		select {
		case err := <-p.readErr:
			return err
		default:
		}
		if r.JSONRPC != "2.0" || r.ID != p.seq || (r.Error == nil) == (len(r.Result) == 0) {
			return errors.New("plugin response has an invalid envelope or request id")
		}
		if r.Error != nil {
			if len(r.Error.Message) == 0 || len(r.Error.Message) > 4096 {
				return errors.New("plugin returned an invalid error message")
			}
			return r.Error
		}
		if result != nil {
			if err := json.Unmarshal(r.Result, result); err != nil {
				return fmt.Errorf("invalid plugin result: %w", err)
			}
		}
		return nil
	case err := <-p.readErr:
		return err
	case <-p.done:
		select {
		case r := <-p.responses:
			if r.ID == p.seq && r.Error != nil {
				return r.Error
			}
		default:
		}
		return errors.New("plugin process exited")
	case <-ctx.Done():
		p.stop()
		return ctx.Err()
	}
}

func (p *process) ping(ctx context.Context) error {
	var pong map[string]json.RawMessage
	if err := p.call(ctx, "ping", struct{}{}, &pong); err != nil {
		return err
	}
	if pong == nil {
		return errors.New("plugin health result must be an object")
	}
	return nil
}
func (r *Result) UnmarshalJSON(raw []byte) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return errors.New("plugin command result must be an object")
	}
	type alias Result
	return json.Unmarshal(raw, (*alias)(r))
}
