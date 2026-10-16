package terminal

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v3"
)

type colorTestTTY struct {
	tcell.Tty
	input  io.Reader
	output bytes.Buffer
}

func (t *colorTestTTY) Read(p []byte) (int, error)  { return t.input.Read(p) }
func (t *colorTestTTY) Write(p []byte) (int, error) { return t.output.Write(p) }

func TestTerminalColorsObserveFragmentedRepliesWithoutChangingInput(t *testing.T) {
	input := "\x1b[Aa\x1b]10;rgb:aaaa/bbbb/cccc\x1b\\\x1b]11;rgb:f/0/8\a\x03\x1b"
	underlying := &colorTestTTY{input: strings.NewReader(input)}
	tty := NewColorTty(underlying)
	var forwarded bytes.Buffer
	buf := make([]byte, 1) // Split every escape, payload and terminator across reads.
	for {
		n, err := tty.Read(buf)
		forwarded.Write(buf[:n])
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if forwarded.String() != input {
		t.Fatal("color detection consumed or changed keyboard/control input")
	}
	if colors := tty.DefaultColors(); colors.Foreground != tcell.GetColor("#aabbcc") || colors.Background != tcell.GetColor("#ff0088") {
		t.Fatal("incorrect terminal colors", colors)
	}
	tty.RequestColors()
	if underlying.output.String() != "\x1b]10;?\a\x1b]11;?\a" {
		t.Fatal("unexpected terminal color request", underlying.output.String())
	}
}

func TestTerminalColorsIgnoreInvalidUnrelatedAndOversizedReplies(t *testing.T) {
	tty := NewColorTty(nil)
	tty.observe([]byte("\x1b]11;#123456\a"))
	want := tty.DefaultColors()
	for _, input := range []string{
		"\x1b]10;?\a", "\x1b]11;rgb:zz/00/00\a", "\x1b]11;rgb:/0/0\a",
		"\x1b]11;rgb:00000/0/0\a", "\x1b]52;c;data\x1b\\",
		"\x1b]11;" + strings.Repeat("f", 4096) + "\x1b\\",
	} {
		tty.observe([]byte(input))
		if tty.DefaultColors() != want {
			t.Fatal("invalid response changed colors", input[:min(40, len(input))])
		}
	}
	tty.observe([]byte("\x1b]10;rgb:0000/0000/0000\a"))
	if tty.DefaultColors().Foreground != tcell.GetColor("#000000") {
		t.Fatal("parser did not recover after oversized response")
	}
}

func TestTerminalColorEnvironmentFallback(t *testing.T) {
	for _, tc := range []struct {
		value  string
		fg, bg tcell.Color
	}{
		{"0;15", tcell.ColorBlack.TrueColor(), tcell.ColorWhite.TrueColor()},
		{"15;default;0", tcell.ColorWhite.TrueColor(), tcell.ColorBlack.TrueColor()},
		{"", tcell.ColorDefault, tcell.ColorDefault},
		{"999;-1", tcell.ColorDefault, tcell.ColorDefault},
	} {
		t.Setenv("COLORFGBG", tc.value)
		if got := ColorsFromEnv(); got.Foreground != tc.fg || got.Background != tc.bg {
			t.Fatal("incorrect COLORFGBG fallback", tc.value, got)
		}
	}
}
