package terminal

import (
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/charmbracelet/x/ansi"
	"github.com/gdamore/tcell/v3"
)

type DefaultColors struct {
	Foreground, Background tcell.Color
}

// ColorTty observes OSC 10/11 replies without consuming or delaying input.
// tcell's own parser discards these replies, including fragmented responses.
// All writes share a lock so color requests cannot split screen escape codes.
type ColorTty struct {
	tcell.Tty
	writeMu sync.Mutex
	mu      sync.Mutex
	colors  DefaultColors
	state   int
	osc     []byte
}

func NewColorTty(tty tcell.Tty) *ColorTty { return &ColorTty{Tty: tty} }

func (t *ColorTty) Write(p []byte) (int, error) {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	return t.Tty.Write(p)
}

func (t *ColorTty) RequestColors() {
	_, _ = t.Write([]byte(ansi.RequestForegroundColor + ansi.RequestBackgroundColor))
}

func (t *ColorTty) DefaultColors() DefaultColors {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.colors
}

func (t *ColorTty) Read(p []byte) (int, error) {
	n, err := t.Tty.Read(p)
	t.observe(p[:n])
	return n, err
}

func (t *ColorTty) observe(p []byte) {
	for _, b := range p {
		switch t.state {
		case 0:
			if b == '\x1b' {
				t.state = 1
			}
		case 1:
			t.state = 0
			if b == ']' {
				t.state, t.osc = 2, t.osc[:0]
			} else if b == '\x1b' {
				t.state = 1
			}
		case 2:
			switch b {
			case '\a':
				t.acceptColor()
				t.state = 0
			case '\x1b':
				t.state = 3
			default:
				// Only color replies are short. Never retain clipboard payloads.
				if len(t.osc) < 128 {
					t.osc = append(t.osc, b)
				} else {
					t.state = 4
				}
			}
		case 3:
			if b == '\\' {
				t.acceptColor()
				t.state = 0
			} else {
				t.state = 4
			}
		case 4: // Ignore oversized or malformed OSC strings through their terminator.
			if b == '\a' {
				t.state = 0
			} else if b == '\x1b' {
				t.state = 5
			}
		case 5:
			if b == '\\' || b == '\a' {
				t.state = 0
			} else {
				t.state = 4
			}
		}
	}
}

func (t *ColorTty) acceptColor() {
	kind, value, ok := strings.Cut(string(t.osc), ";")
	if !ok || (kind != "10" && kind != "11") {
		return
	}
	color, valid := parseTerminalColor(value)
	if !valid {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if kind == "10" {
		t.colors.Foreground = color
	} else {
		t.colors.Background = color
	}
}

func parseTerminalColor(s string) (tcell.Color, bool) {
	var parts []string
	if value, ok := strings.CutPrefix(s, "rgb:"); ok {
		parts = strings.Split(value, "/")
	} else if value, ok := strings.CutPrefix(s, "#"); ok && (len(value) == 3 || len(value) == 6 || len(value) == 12) {
		width := len(value) / 3
		parts = []string{value[:width], value[width : 2*width], value[2*width:]}
	}
	if len(parts) != 3 {
		return tcell.ColorDefault, false
	}
	var rgb [3]int32
	for i, part := range parts {
		if len(part) < 1 || len(part) > 4 {
			return tcell.ColorDefault, false
		}
		n, err := strconv.ParseUint(part, 16, 16)
		if err != nil {
			return tcell.ColorDefault, false
		}
		maximum := uint64(1)<<(4*len(part)) - 1
		rgb[i] = int32((n*255 + maximum/2) / maximum)
	}
	return tcell.NewRGBColor(rgb[0], rgb[1], rgb[2]), true
}

// COLORFGBG is a fallback for terminals that do not answer OSC queries.
func ColorsFromEnv() DefaultColors {
	parts := strings.Split(os.Getenv("COLORFGBG"), ";")
	if len(parts) < 2 {
		return DefaultColors{}
	}
	parse := func(s string) tcell.Color {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 || n > 255 {
			return tcell.ColorDefault
		}
		return tcell.PaletteColor(n).TrueColor()
	}
	return DefaultColors{Foreground: parse(parts[0]), Background: parse(parts[len(parts)-1])}
}
