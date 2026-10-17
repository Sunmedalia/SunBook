package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/jsonrpc2"
	"sunbook/internal/config"
)

func helperClient(t *testing.T, message func(string)) *Client {
	t.Helper()
	t.Setenv("SUNBOOK_LSP_HELPER", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c, err := Start(context.Background(), t.TempDir(), config.Server{Command: exe, Args: []string{"-test.run=TestServerHelper"}}, func(Diagnostics) {}, message)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

func TestSlowRequestDoesNotBlockChangesOrFollowingRequests(t *testing.T) {
	c := helperClient(t, func(string) {})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	slow := make(chan error, 2)
	var calls atomic.Int32
	c.Request(ctx, "test/stall", nil, func(_ json.RawMessage, err error) { calls.Add(1); slow <- err })
	c.Open("/tmp/ordered.go", "go", "package old", 1)
	c.Change("/tmp/ordered.go", "package fresh", 2)
	done := make(chan json.RawMessage, 1)
	c.Request(context.Background(), "test/contents", nil, func(raw json.RawMessage, err error) {
		if err != nil {
			done <- nil
		} else {
			done <- raw
		}
	})
	select {
	case raw := <-done:
		if string(raw) != `"package fresh"` {
			t.Fatalf("request overtook change, got %s", raw)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("slow request blocked subsequent work")
	}
	cancel()
	select {
	case err := <-slow:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("request cancellation blocked")
	}
	c.Close()
	if calls.Load() != 1 {
		t.Fatal("callback called multiple times")
	}
}

func TestRequestQueueBoundAndChangeBarriers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &Client{ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1)}
	c.Change("a", "one", 1)
	c.Change("a", "two", 2)
	c.Notify("textDocument/didSave", nil)
	c.Change("a", "three", 3)
	if len(c.jobs) != 3 || c.jobs[0].params.(map[string]any)["contentChanges"].([]map[string]string)[0]["text"] != "two" {
		t.Fatal("change coalescing crossed barrier")
	}
	c.jobs = make([]job, 1024)
	done := make(chan error, 1)
	c.Request(ctx, "test/overflow", nil, func(_ json.RawMessage, err error) { done <- err })
	select {
	case err := <-done:
		if !errors.Is(err, ErrBusy) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("full queue blocked caller")
	}
}

func TestInflightLimitStillAllowsNotifications(t *testing.T) {
	stalls := make(chan string, 16)
	c := helperClient(t, func(s string) {
		if s == "stall" {
			stalls <- s
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for range 8 {
		c.Request(ctx, "test/stall", nil, func(json.RawMessage, error) {})
	}
	for range 8 {
		select {
		case <-stalls:
		case <-time.After(2 * time.Second):
			t.Fatal("request wasn't dispatched")
		}
	}
	done := make(chan error, 1)
	c.Request(ctx, "textDocument/completion", nil, func(_ json.RawMessage, err error) { done <- err })
	select {
	case err := <-done:
		if !errors.Is(err, ErrBusy) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("inflight limit blocked")
	}
	c.Notify("test/notify", nil)
	select {
	case s := <-stalls:
		if s != "stall" {
			t.Fatal(s)
		}
	case <-time.After(time.Second):
		t.Fatal("notifications blocked by inflight limit")
	}
}

func TestBlockedTransportDoesNotBlockUIAndCancels(t *testing.T) {
	left, right := net.Pipe()
	defer right.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &Client{ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1), active: make(chan struct{}, 8), writeTimeout: 50 * time.Millisecond, onMessage: func(string) {}}
	c.conn = jsonrpc2.NewConn(ctx, jsonrpc2.NewBufferedStream(left, jsonrpc2.VSCodeObjectCodec{}), &fakeServer{})
	defer c.conn.Close()
	go c.loop()
	requestCtx, stop := context.WithTimeout(ctx, time.Second)
	defer stop()
	done := make(chan error, 1)
	c.Request(requestCtx, "test/stopped-reader", strings.Repeat("x", 1024*1024), func(_ json.RawMessage, err error) { done <- err })
	// Notifications can be enqueued while the worker is blocked in Write.
	c.Change("a", "still editable", 1)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("stopped reader blocked cancellation")
	}
	select {
	case <-c.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("blocked transport wasn't terminated")
	}
}

func TestUserCancellationDoesNotCloseWritingTransport(t *testing.T) {
	left, right := net.Pipe()
	defer right.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &Client{ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1), active: make(chan struct{}, 8), onMessage: func(string) {}}
	c.conn = jsonrpc2.NewConn(ctx, jsonrpc2.NewBufferedStream(left, jsonrpc2.VSCodeObjectCodec{}), &fakeServer{})
	defer c.conn.Close()
	go c.loop()
	requestCtx, stop := context.WithTimeout(ctx, 50*time.Millisecond)
	defer stop()
	done := make(chan error, 1)
	c.Request(requestCtx, "test/temporarily-busy-reader", nil, func(_ json.RawMessage, err error) { done <- err })
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation blocked")
	}
	if c.ctx.Err() != nil {
		t.Fatal("user cancellation killed the language server")
	}
}

func TestNotificationOverflowTerminatesDesynchronizedClient(t *testing.T) {
	left, right := net.Pipe()
	defer right.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	messages := make(chan string, 1)
	c := &Client{ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1), jobs: make([]job, 1024), onMessage: func(s string) { messages <- s }}
	c.conn = jsonrpc2.NewConn(ctx, jsonrpc2.NewBufferedStream(left, jsonrpc2.VSCodeObjectCodec{}), &fakeServer{})
	defer c.conn.Close()
	c.Notify("textDocument/didSave", nil)
	if c.ctx.Err() == nil {
		t.Fatal("dropped notification left client running")
	}
	select {
	case msg := <-messages:
		if !strings.Contains(msg, "Restart Language Servers") {
			t.Fatal(msg)
		}
	case <-time.After(time.Second):
		t.Fatal("overflow wasn't reported")
	}
}
