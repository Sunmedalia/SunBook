package text

import (
	"math/rand/v2"
	"runtime"
	"sort"
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

// Unlike the edit/undo benchmark, this materializes the edited snapshot on every
// iteration, as rendering and full-document language synchronization would.
func BenchmarkEditMaterializedSnapshot(b *testing.B) {
	source := strings.Repeat("package demo // 中文👩‍💻\n", 100000)
	buf := New(source)
	p := len(source) / 2
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		buf.Apply([]Edit{{Start: p, End: p, Text: "x"}}, nil, nil)
		if len(buf.String()) != len(source)+1 {
			b.Fatal("snapshot")
		}
		buf.Undo()
	}
}

func FuzzBufferHistory(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	f.Add([]byte{2, 2, 2, 1, 0, 1, 4, 7, 8, 9, 255, 254})
	f.Fuzz(func(t *testing.T, actions []byte) {
		b := New("中👩‍💻\né tail\n")
		states := []string{b.String()}
		at := 0
		for i, action := range actions {
			if i >= 256 {
				break
			}
			switch action % 6 {
			case 0:
				_, ok := b.Undo()
				if ok != (at > 0) {
					t.Fatal("undo availability")
				}
				if ok {
					at--
				}
			case 1:
				_, ok := b.Redo()
				if ok != (at+1 < len(states)) {
					t.Fatal("redo availability")
				}
				if ok {
					at++
				}
			default:
				s := states[at]
				bounds := []int{0}
				g := uniseg.NewGraphemes(s)
				for g.Next() {
					_, end := g.Positions()
					bounds = append(bounds, end)
				}
				// One to four non-overlapping edits form a single transaction,
				// as multiple carets produce.
				count := 1 + int(action/6)%4
				var cuts []int
				for k := range 2 * count {
					cuts = append(cuts, bounds[(int(action)*(k+1)+i*7+k)%len(bounds)])
				}
				sort.Ints(cuts)
				var edits []Edit
				next, prev := "", 0
				for k := 0; k < len(cuts); k += 2 {
					lo, hi := cuts[k], cuts[k+1]
					if len(edits) > 0 && edits[len(edits)-1].Start == lo {
						continue
					}
					insert := []string{"x", "\n", "中", "👩‍💻", "é", ""}[(int(action)+k)%6]
					edits = append(edits, Edit{Start: lo, End: hi, Text: insert})
					next += s[prev:lo] + insert
					prev = hi
				}
				next += s[prev:]
				if !b.Apply(edits, nil, nil) {
					t.Fatal("valid edit rejected")
				}
				states = append(states[:at+1], next)
				at++
			}
			want := states[at]
			if b.String() != want || b.Len() != len(want) {
				t.Fatal("content mismatch")
			}
			lines := strings.Split(want, "\n")
			if b.LineCount() != len(lines) {
				t.Fatal("line count")
			}
			for row, line := range lines {
				if b.Line(row) != line {
					t.Fatal("line content")
				}
			}
			// Read through the pieces, as if the cache had not been rebuilt.
			b.valid = false
			for p := range len(want) {
				if b.ByteAt(p) != want[p] {
					t.Fatal("byte", p)
				}
				q := min(len(want), p+1+int(action)%9)
				if b.Slice(p, q) != want[p:q] {
					t.Fatal("slice", p, q)
				}
			}
			if !b.Equal(want) || b.Equal(want+"x") {
				t.Fatal("equal")
			}
		}
	})
}

// One caret per line, each typing the same character: one transaction with
// `carets` edits per keystroke, as after "select all occurrences".
func benchmarkMultiCursorTyping(b *testing.B, carets, keys int) {
	source := strings.Repeat("hello world line\n", carets)
	b.ReportAllocs()
	for range b.N {
		buf := New(source)
		sels := make([]Selection, carets)
		for i := range sels {
			sels[i] = Selection{i * 17, i * 17}
		}
		for range keys {
			edits := make([]Edit, carets)
			after := make([]Selection, carets)
			for i, s := range sels {
				edits[i] = Edit{s.Head, s.Head, "x"}
				after[i] = Selection{s.Head + i + 1, s.Head + i + 1}
			}
			if !buf.Apply(edits, sels, after) {
				b.Fatal("apply")
			}
			sels = after
		}
		if buf.Len() != len(source)+carets*keys {
			b.Fatal("length")
		}
	}
	b.ReportMetric(float64(b.Elapsed().Microseconds())/1000/float64(b.N*keys), "ms/key")
}
func BenchmarkMultiCursorTyping(b *testing.B) {
	b.Run("carets=1000", func(b *testing.B) { benchmarkMultiCursorTyping(b, 1000, 20) })
	b.Run("carets=5000", func(b *testing.B) { benchmarkMultiCursorTyping(b, 5000, 20) })
}

// A thousand single-caret edits scattered over a 3.4 MB document; the metric is
// the heap retained by the buffer and its undo history afterwards.
func BenchmarkScatteredEditHistory(b *testing.B) {
	source := strings.Repeat("hello world line\n", 200000)
	var retained float64
	for range b.N {
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		buf := New(source)
		for k := range 1000 {
			p := (k * 104729) % buf.Len()
			if !buf.Apply([]Edit{{p, p, "y"}}, nil, nil) {
				b.Fatal("apply")
			}
		}
		runtime.GC()
		runtime.ReadMemStats(&after)
		retained = (float64(after.HeapAlloc) - float64(before.HeapAlloc)) / (1 << 20)
		runtime.KeepAlive(buf)
	}
	b.ReportMetric(retained, "retained-MB")
}
