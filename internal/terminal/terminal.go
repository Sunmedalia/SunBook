package terminal

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"
)

// Session owns the PTY and emulator. The UI applies output through Feed, so
// rendering and emulator mutations are serialized on the UI event loop.
type Session struct {
	VT             *vt.Emulator
	file           *os.File
	cmd            *exec.Cmd
	done           chan struct{}
	inputDone      chan struct{}
	once           sync.Once
	Width, Height  int
	MouseReporting bool
	mouseModes     map[ansi.Mode]bool
}

func Start(shell, dir string, w, h int, output func([]byte), exited func(error)) (*Session, error) {
	cmd := exec.Command(shell)
	cmd.Dir = dir
	for _, v := range os.Environ() {
		if len(v) >= 5 && v[:5] == "TERM=" {
			continue
		}
		cmd.Env = append(cmd.Env, v)
	}
	cmd.Env = append(cmd.Env, "TERM=xterm-256color", "COLORTERM=truecolor")
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(w), Rows: uint16(h)})
	if err != nil {
		return nil, err
	}
	s := &Session{VT: vt.NewEmulator(w, h), file: f, cmd: cmd, done: make(chan struct{}), inputDone: make(chan struct{}), Width: w, Height: h}
	s.VT.SetScrollbackSize(10000)
	s.mouseModes = map[ansi.Mode]bool{}
	setMouse := func(mode ansi.Mode, enabled bool) {
		switch mode {
		case ansi.ModeMouseX10, ansi.ModeMouseNormal, ansi.ModeMouseHighlight, ansi.ModeMouseButtonEvent, ansi.ModeMouseAnyEvent:
			s.mouseModes[mode] = enabled
			s.MouseReporting = false
			for _, on := range s.mouseModes {
				s.MouseReporting = s.MouseReporting || on
			}
		}
	}
	s.VT.SetCallbacks(vt.Callbacks{EnableMode: func(mode ansi.Mode) { setMouse(mode, true) }, DisableMode: func(mode ansi.Mode) { setMouse(mode, false) }})
	// Replies, key encodings and bracketed paste are emitted by the VT through Read.
	go func() { defer close(s.inputDone); _, _ = io.Copy(f, s.VT) }()
	go func() {
		buf := make([]byte, 32768)
		for {
			n, e := f.Read(buf)
			if n > 0 {
				output(append([]byte(nil), buf[:n]...))
			}
			if e != nil {
				break
			}
		}
		err := cmd.Wait()
		close(s.done)
		exited(err)
	}()
	return s, nil
}
func (s *Session) Feed(p []byte)         { _, _ = s.VT.Write(p) }
func (s *Session) Key(k uv.Key)          { s.VT.SendKey(uv.KeyPressEvent(k)) }
func (s *Session) Paste(text string)     { s.VT.Paste(text) }
func (s *Session) Mouse(m uv.MouseEvent) { s.VT.SendMouse(m) }
func (s *Session) Resize(w, h int) error {
	w = max(1, w)
	h = max(1, h)
	if w == s.Width && h == s.Height {
		return nil
	}
	if w > 65535 || h > 65535 {
		return fmt.Errorf("terminal dimensions too large")
	}
	s.Width = w
	s.Height = h
	s.VT.Resize(w, h)
	return pty.Setsize(s.file, &pty.Winsize{Cols: uint16(w), Rows: uint16(h)})
}
func (s *Session) Close() {
	s.once.Do(func() {
		_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGHUP)
		_ = s.file.Close()
		// Close the pipe before the emulator: vt.Read checks its closed flag
		// without synchronization, so no reader may remain during vt.Close.
		if pipe, ok := s.VT.InputPipe().(io.Closer); ok {
			_ = pipe.Close()
		}
		<-s.inputDone
		_ = s.VT.Close()
		select {
		case <-s.done:
		case <-time.After(500 * time.Millisecond):
			_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
			select {
			case <-s.done:
			case <-time.After(time.Second):
			}
		}
	})
}
