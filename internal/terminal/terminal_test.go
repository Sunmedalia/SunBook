package terminal

import (
	"strings"
	"testing"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
)

func TestPTYInteractiveResizeAndInterrupt(t *testing.T) {
	output := make(chan []byte, 256)
	exited := make(chan error, 1)
	s, e := Start("/bin/sh", t.TempDir(), 80, 12, func(p []byte) { output <- p }, func(e error) { exited <- e })
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	wait := func(needle string) {
		t.Helper()
		deadline := time.NewTimer(5 * time.Second)
		defer deadline.Stop()
		var raw strings.Builder
		for {
			select {
			case p := <-output:
				s.Feed(p)
				raw.Write(p)
				if strings.Contains(strings.Join(strings.Fields(raw.String()), " "), needle) {
					return
				}
			case e := <-exited:
				t.Fatal("early exit", e)
			case <-deadline.C:
				t.Fatalf("no %q in %q", needle, raw.String())
			}
		}
	}
	s.Paste("printf '\\033[31mHELLO_%s\\033[0m\\n' ANSI\n")
	wait("HELLO_ANSI")
	if !strings.Contains(s.VT.String(), "HELLO_ANSI") {
		t.Fatal("VT did not render output")
	}
	if e = s.Resize(91, 17); e != nil {
		t.Fatal(e)
	}
	s.Paste("stty size\n")
	wait("17 91")
	s.Paste("sleep 20\n")
	time.Sleep(100 * time.Millisecond)
	s.Key(uv.Key{Code: 'c', Mod: uv.ModCtrl})
	s.Paste("printf 'INTERRUPTED_%s\\n' OK\n")
	wait("INTERRUPTED_OK")
	s.Paste("exit\n")
	select {
	case <-exited:
	case <-time.After(3 * time.Second):
		t.Fatal("shell did not exit")
	}
}

func TestPTYMouseReporting(t *testing.T) {
	out := make(chan []byte, 128)
	s, e := Start("/bin/sh", t.TempDir(), 80, 12, func(p []byte) { out <- p }, func(error) {})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	wait := func(needle string) {
		t.Helper()
		var raw strings.Builder
		deadline := time.After(4 * time.Second)
		for {
			select {
			case p := <-out:
				s.Feed(p)
				raw.Write(p)
				if strings.Contains(strings.Join(strings.Fields(raw.String()), " "), needle) {
					return
				}
			case <-deadline:
				t.Fatalf("missing %q in %q", needle, raw.String())
			}
		}
	}
	s.Paste("stty raw -echo; printf 'READY_%s\\n' MOUSE; dd bs=1 count=9 2>/dev/null | od -An -tx1; stty sane\n")
	wait("READY_MOUSE")
	s.Feed([]byte("\x1b[?1000h\x1b[?1006h"))
	if !s.MouseReporting {
		t.Fatal("mouse reporting not enabled")
	}
	s.Mouse(uv.MouseClickEvent{X: 2, Y: 1, Button: uv.MouseLeft})
	wait("1b 5b 3c 30 3b 33 3b 32 4d")
	s.Feed([]byte("\x1b[?1000l"))
	if s.MouseReporting {
		t.Fatal("mouse reporting not disabled")
	}
}
