package text

import (
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/rivo/uniseg"
)

func TestPieceTableTransactions(t *testing.T) {
	b := New("你好 world\n🙂 end")
	before := []Selection{{0, 0}, {len("你好 world\n"), len("你好 world\n")}}
	after := []Selection{{1, 1}, {13, 13}}
	if !b.Apply([]Edit{{0, len("你好"), "Hi"}, {len("你好 world\n"), len("你好 world\n🙂"), "😀!"}}, before, after) {
		t.Fatal("apply")
	}
	if got := b.String(); got != "Hi world\n😀! end" {
		t.Fatal(got)
	}
	if s, ok := b.Undo(); !ok || len(s) != 2 || b.String() != "你好 world\n🙂 end" {
		t.Fatal("undo", s, b.String())
	}
	if s, ok := b.Redo(); !ok || s[1].Head != 13 || b.String() != "Hi world\n😀! end" {
		t.Fatal("redo", s, b.String())
	}
	b.Undo()
	b.Apply([]Edit{{0, 0, "x"}}, before, before)
	if _, ok := b.Redo(); ok {
		t.Fatal("redo survived branching")
	}
}
func TestPieceTableAgainstString(t *testing.T) {
	rng := rand.New(rand.NewPCG(31, 47))
	b := New("start\n你好🙂")
	want := b.String()
	var history []string
	for range 800 {
		bounds := []int{0}
		g := uniseg.NewGraphemes(want)
		for g.Next() {
			_, z := g.Positions()
			bounds = append(bounds, z)
		}
		i, j := rng.IntN(len(bounds)), rng.IntN(len(bounds))
		if i > j {
			i, j = j, i
		}
		lo, hi := bounds[i], bounds[j]
		insert := []string{"x", "\n", "你好", "👩‍💻", "", "é"}[rng.IntN(6)]
		history = append(history, want)
		want = want[:lo] + insert + want[hi:]
		if !b.Apply([]Edit{{lo, hi, insert}}, nil, nil) {
			t.Fatal("rejected valid edit")
		}
		if b.String() != want {
			t.Fatalf("got %q want %q", b.String(), want)
		}
		lines := strings.Split(want, "\n")
		if b.LineCount() != len(lines) {
			t.Fatal("line count")
		}
		for r, line := range lines {
			if b.Line(r) != line {
				t.Fatal("line index")
			}
		}
	}
	for i := len(history) - 1; i >= 0; i-- {
		if _, ok := b.Undo(); !ok || b.String() != history[i] {
			t.Fatal("history mismatch", i)
		}
	}
}
func TestGraphemesAndLSP(t *testing.T) {
	s := "中👩‍💻é\tZ\n尾"
	b := New(s)
	p := 0
	want := []string{"中", "👩‍💻", "é", "\t", "Z", "\n", "尾"}
	for _, cluster := range want {
		q := Next(s, p)
		if s[p:q] != cluster || Prev(s, q) != p {
			t.Fatalf("cluster %q at %d:%d", cluster, p, q)
		}
		p = q
	}
	if Width("中\t👩‍💻é", 4) != 7 {
		t.Fatal("cell width")
	}
	p = len("中👩‍💻")
	row, col := b.LSPPosition(p)
	if row != 0 || col != 6 || b.LSPOffset(row, col) != p {
		t.Fatal(row, col, b.LSPOffset(row, col), p)
	}
	if b.LSPOffset(1, 0) != b.LineStart(1) {
		t.Fatal("zero column")
	}
	if Snap(s, len("中")+2) != len("中") {
		t.Fatal("snap inside emoji")
	}
}
func TestInvalidEditsAtomic(t *testing.T) {
	b := New("中abc")
	for _, edits := range [][]Edit{{{1, 2, "x"}}, {{0, 4, "x"}, {3, 5, "y"}}, {{0, 0, "x"}, {0, 0, "z"}}, {{-1, 0, "x"}}, {{0, 30, "x"}}, {{0, 0, string([]byte{0xff})}}} {
		if b.Apply(edits, nil, nil) {
			t.Fatal("accepted invalid edit")
		}
		if b.String() != "中abc" || b.Version != 1 {
			t.Fatal("partial mutation")
		}
	}
}
func BenchmarkEditLargeDocument(b *testing.B) {
	source := strings.Repeat("package demo // 中文👩‍💻\n", 100000)
	buf := New(source)
	p := len(source) / 2
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		buf.Apply([]Edit{{p, p, "x"}}, nil, nil)
		buf.Undo()
	}
}
