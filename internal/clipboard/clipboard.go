package clipboard

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

type Clipboard struct {
	mu   sync.Mutex
	Text string
}

func commands() (read, write []string) {
	if runtime.GOOS == "darwin" {
		return []string{"pbpaste"}, []string{"pbcopy"}
	}
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		if _, e := exec.LookPath("wl-copy"); e == nil {
			return []string{"wl-paste", "--no-newline"}, []string{"wl-copy"}
		}
	}
	if os.Getenv("DISPLAY") != "" {
		if _, e := exec.LookPath("xclip"); e == nil {
			return []string{"xclip", "-selection", "clipboard", "-o"}, []string{"xclip", "-selection", "clipboard"}
		}
	}
	return nil, nil
}
func (c *Clipboard) Write(s string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Text = s
	_, args := commands()
	if len(args) == 0 {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdin = strings.NewReader(s)
	return cmd.Run() == nil
}
func (c *Clipboard) Read() (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	args, _ := commands()
	if len(args) == 0 {
		return c.Text, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	data, e := exec.CommandContext(ctx, args[0], args[1:]...).Output()
	if e != nil {
		return c.Text, false
	}
	c.Text = string(data)
	return c.Text, true
}
