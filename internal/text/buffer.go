// Package text implements the UTF-8 piece table and transactional edit history.
// All positions are byte offsets at grapheme boundaries, except LSP positions.
package text

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

type piece struct {
	Added      bool
	Start, Len int
}
type Selection struct{ Anchor, Head int }

func (s Selection) Range() (int, int) { return min(s.Anchor, s.Head), max(s.Anchor, s.Head) }
func (s Selection) Empty() bool       { return s.Anchor == s.Head }

type Edit struct {
	Start, End int
	Text       string
}

// A transaction stores its edits in both directions with the text each one
// removed, so undo and redo replay edits instead of keeping piece snapshots.
// Forward uses the coordinates before the transaction, Backward those after.
type transaction struct {
	Before, After     []Selection
	Forward, Backward []Edit
}

type Buffer struct {
	length            int
	historyGeneration uint64
	maintenanceBytes  int
	original          string
	added             []byte
	pieces            []piece
	spare             []piece // the previous piece table, reused by the next edit
	pieceEnds         []int   // lazy document end offset of each piece
	cache             string
	valid             bool
	lines             []int
	spareLines        []int
	history           []transaction
	index             int
	Version           int
	LastEdits         []Edit
	// LastRemoved[i] is the text LastEdits[i] replaced, so listeners such as
	// incremental LSP sync can describe a change without the previous snapshot.
	LastRemoved []string
}

func New(s string) *Buffer {
	b := &Buffer{original: s, cache: s, valid: true, length: len(s), Version: 1, lines: []int{0}}
	if len(s) > 0 {
		b.pieces = []piece{{Len: len(s)}}
	}
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			b.lines = append(b.lines, i+1)
		}
	}
	return b
}
func (b *Buffer) String() string {
	if !b.valid {
		if len(b.pieces) == 1 && !b.pieces[0].Added {
			p := b.pieces[0]
			b.cache = b.original[p.Start : p.Start+p.Len]
		} else {
			var out strings.Builder
			out.Grow(b.length)
			for _, p := range b.pieces {
				if p.Added {
					out.Write(b.added[p.Start : p.Start+p.Len])
				} else {
					out.WriteString(b.original[p.Start : p.Start+p.Len])
				}
			}
			b.cache = out.String()
		}
		b.valid = true
	}
	return b.cache
}
func (b *Buffer) Len() int              { return b.length }
func (b *Buffer) LineCount() int        { return len(b.lines) }
func (b *Buffer) LineStart(row int) int { return b.lines[max(0, min(row, len(b.lines)-1))] }
func (b *Buffer) LineEnd(row int) int {
	if row+1 < len(b.lines) {
		return b.lines[row+1] - 1
	}
	return b.length
}
func (b *Buffer) Line(row int) string { return b.Slice(b.LineStart(row), b.LineEnd(row)) }

// locate returns the index of the piece containing offset p (< Len) and the
// document offset at which that piece starts.
func (b *Buffer) locate(p int) (int, int) {
	if len(b.pieceEnds) != len(b.pieces) {
		b.pieceEnds = b.pieceEnds[:0]
		end := 0
		for _, pc := range b.pieces {
			end += pc.Len
			b.pieceEnds = append(b.pieceEnds, end)
		}
	}
	i := sort.Search(len(b.pieceEnds), func(i int) bool { return b.pieceEnds[i] > p })
	return i, b.pieceEnds[i] - b.pieces[i].Len
}

// Slice returns the text in [lo, hi) without materializing the whole document.
func (b *Buffer) Slice(lo, hi int) string {
	lo, hi = max(0, min(lo, b.length)), max(0, min(hi, b.length))
	if lo >= hi {
		return ""
	}
	if b.valid {
		return b.cache[lo:hi]
	}
	i, pos := b.locate(lo)
	if p := b.pieces[i]; hi <= pos+p.Len && !p.Added {
		return b.original[p.Start+lo-pos : p.Start+hi-pos]
	}
	out := make([]byte, 0, hi-lo)
	for at := lo; at < hi; i++ {
		p := b.pieces[i]
		from, to := at-pos, min(hi-pos, p.Len)
		if p.Added {
			out = append(out, b.added[p.Start+from:p.Start+to]...)
		} else {
			out = append(out, b.original[p.Start+from:p.Start+to]...)
		}
		at = pos + to
		pos += p.Len
	}
	return string(out)
}

// ByteAt returns the byte at offset p, which must be in [0, Len).
func (b *Buffer) ByteAt(p int) byte {
	if b.valid {
		return b.cache[p]
	}
	i, pos := b.locate(p)
	pc := b.pieces[i]
	if pc.Added {
		return b.added[pc.Start+p-pos]
	}
	return b.original[pc.Start+p-pos]
}
func (b *Buffer) boundary(p int) bool { return p == b.length || utf8.RuneStart(b.ByteAt(p)) }

// Equal reports whether the document is s, comparing piece by piece.
func (b *Buffer) Equal(s string) bool {
	if len(s) != b.length {
		return false
	}
	if b.valid {
		return b.cache == s
	}
	at := 0
	for _, p := range b.pieces {
		want := s[at : at+p.Len]
		if p.Added && string(b.added[p.Start:p.Start+p.Len]) != want || !p.Added && b.original[p.Start:p.Start+p.Len] != want {
			return false
		}
		at += p.Len
	}
	return true
}
func (b *Buffer) Position(offset int) (row, col int) {
	offset = max(0, min(offset, b.length))
	row = sort.Search(len(b.lines), func(i int) bool { return b.lines[i] > offset }) - 1
	return row, offset - b.lines[row]
}

// applyBatch replaces sorted, non-overlapping edits, all given in the current
// document's coordinates, with one pass over the piece and line tables.
// Inserted text is appended to the add store from left to right.
func (b *Buffer) applyBatch(edits []Edit) {
	old := b.pieces
	out := b.spare[:0]
	push := func(p piece) {
		if p.Len == 0 {
			return
		}
		if n := len(out); n > 0 && out[n-1].Added == p.Added && out[n-1].Start+out[n-1].Len == p.Start {
			out[n-1].Len += p.Len
			return
		}
		out = append(out, p)
	}
	pi, pos := 0, 0 // pos is the document offset of old[pi]
	keep := func(from, to int) {
		for from < to {
			for pos+old[pi].Len <= from {
				pos += old[pi].Len
				pi++
			}
			p := old[pi]
			lo, hi := from-pos, min(to, pos+p.Len)-pos
			push(piece{p.Added, p.Start + lo, hi - lo})
			from = pos + hi
		}
	}
	at := 0
	for _, e := range edits {
		keep(at, e.Start)
		if e.Text != "" {
			start := len(b.added)
			b.added = append(b.added, e.Text...)
			push(piece{true, start, len(e.Text)})
		}
		at = e.End
		b.maintenanceBytes += len(e.Text) + (e.End - e.Start)
	}
	keep(at, b.length)
	b.pieces, b.spare = out, old[:0]
	b.pieceEnds = b.pieceEnds[:0]
	b.updateLines(edits)
	b.valid = false
}

// updateLines rebuilds newline offsets in one merge pass without scanning or
// materializing unchanged text. A line start s follows the newline at s-1, so
// it survives an edit unless that newline lies inside the replaced range.
func (b *Buffer) updateLines(edits []Edit) {
	out := b.spareLines[:0]
	li, delta := 0, 0
	for _, e := range edits {
		for li < len(b.lines) && b.lines[li] <= e.Start {
			out = append(out, b.lines[li]+delta)
			li++
		}
		for li < len(b.lines) && b.lines[li] <= e.End {
			li++
		}
		for i := 0; i < len(e.Text); i++ {
			if e.Text[i] == '\n' {
				out = append(out, e.Start+delta+i+1)
			}
		}
		delta += len(e.Text) - (e.End - e.Start)
	}
	for _, s := range b.lines[li:] {
		out = append(out, s+delta)
	}
	b.lines, b.spareLines = out, b.lines[:0]
	b.length += delta
}

// Apply validates non-overlapping edits against the original document and commits
// all of them as a single undo transaction. Callers pass the resulting selections.
func (b *Buffer) Apply(edits []Edit, before, after []Selection) bool {
	if len(edits) == 0 {
		return false
	}
	edits = append([]Edit(nil), edits...)
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].Start < edits[j].Start })
	for i, e := range edits {
		if e.Start < 0 || e.End < e.Start || e.End > b.length || !utf8.ValidString(e.Text) || !b.boundary(e.Start) || !b.boundary(e.End) {
			return false
		}
		if i > 0 && (edits[i-1].End > e.Start || edits[i-1].Start == e.Start) {
			return false
		}
	}
	t := transaction{
		Before:   append([]Selection(nil), before...),
		After:    append([]Selection(nil), after...),
		Forward:  edits,
		Backward: make([]Edit, len(edits)),
	}
	delta := 0
	for i, e := range edits {
		edits[i].Text = strings.Clone(e.Text)
		t.Backward[i] = Edit{Start: e.Start + delta, End: e.Start + delta + len(e.Text), Text: b.Slice(e.Start, e.End)}
		delta += len(e.Text) - (e.End - e.Start)
	}
	b.applyBatch(edits)
	b.Version++
	b.LastEdits = edits
	b.LastRemoved = removedTexts(t.Backward)
	clear(b.history[b.index:])
	b.history = append(b.history[:b.index], t)
	b.historyGeneration++
	if len(b.history) > 1000 {
		b.history[0] = transaction{}
		b.history = b.history[1:]
	}
	b.index = len(b.history)
	return true
}

// A transaction's inverse edits hold exactly the text its edits removed.
func removedTexts(inverse []Edit) []string {
	out := make([]string, len(inverse))
	for i, e := range inverse {
		out[i] = e.Text
	}
	return out
}
func (b *Buffer) CanUndo() bool { return b.index > 0 }
func (b *Buffer) CanRedo() bool { return b.index < len(b.history) }
func (b *Buffer) Undo() ([]Selection, bool) {
	if b.index == 0 {
		return nil, false
	}
	b.index--
	t := b.history[b.index]
	b.LastEdits = t.Backward
	b.LastRemoved = removedTexts(t.Forward)
	b.applyBatch(t.Backward)
	b.Version++
	return append([]Selection(nil), t.Before...), true
}
func (b *Buffer) Redo() ([]Selection, bool) {
	if b.index >= len(b.history) {
		return nil, false
	}
	t := b.history[b.index]
	b.LastEdits = t.Forward
	b.LastRemoved = removedTexts(t.Backward)
	b.applyBatch(t.Forward)
	b.Version++
	b.index++
	return append([]Selection(nil), t.After...), true
}

func Prev(s string, p int) int {
	p = max(0, min(p, len(s)))
	if p == 0 {
		return 0
	}
	start := strings.LastIndexByte(s[:p-1], '\n') + 1
	g := uniseg.NewGraphemes(s[start:p])
	at := 0
	for g.Next() {
		at, _ = g.Positions()
	}
	return start + at
}

// Snap returns the closest grapheme boundary at or before p.
func Snap(s string, p int) int {
	p = max(0, min(p, len(s)))
	if p == 0 || p == len(s) {
		return p
	}
	start := strings.LastIndexByte(s[:p], '\n') + 1
	g := uniseg.NewGraphemes(s[start:])
	for g.Next() {
		lo, hi := g.Positions()
		if start+hi > p {
			return start + lo
		}
		if start+hi == p {
			return p
		}
	}
	return p
}
func Next(s string, p int) int {
	p = max(0, min(p, len(s)))
	g := uniseg.NewGraphemes(s[p:])
	if g.Next() {
		_, end := g.Positions()
		return p + end
	}
	return len(s)
}
func word(c rune) bool { return unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_' }
func WordRange(s string, p int) (int, int) {
	p = max(0, min(p, len(s)))
	if p == len(s) && p > 0 {
		p = Prev(s, p)
	}
	lo, hi := p, p
	r, _ := utf8.DecodeRuneInString(s[p:])
	w := word(r)
	for lo > 0 {
		q := Prev(s, lo)
		r, _ := utf8.DecodeRuneInString(s[q:])
		if word(r) != w || unicode.IsSpace(r) {
			break
		}
		lo = q
	}
	for hi < len(s) {
		r, _ := utf8.DecodeRuneInString(s[hi:])
		if word(r) != w || unicode.IsSpace(r) {
			break
		}
		hi = Next(s, hi)
	}
	if lo == hi {
		hi = Next(s, p)
	}
	return lo, hi
}
func WordMove(s string, p, dir int) int {
	if dir < 0 {
		for p > 0 {
			q := Prev(s, p)
			r, _ := utf8.DecodeRuneInString(s[q:])
			p = q
			if word(r) {
				break
			}
		}
		for p > 0 {
			q := Prev(s, p)
			r, _ := utf8.DecodeRuneInString(s[q:])
			if !word(r) {
				break
			}
			p = q
		}
	} else {
		for p < len(s) {
			r, _ := utf8.DecodeRuneInString(s[p:])
			if !word(r) {
				break
			}
			p = Next(s, p)
		}
		for p < len(s) {
			r, _ := utf8.DecodeRuneInString(s[p:])
			if word(r) {
				break
			}
			p = Next(s, p)
		}
	}
	return p
}
func Width(s string, tab int) int { return Column(s, len(s), tab) }
func Column(s string, offset, tab int) int {
	tab = max(1, tab)
	col := 0
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		a, _ := g.Positions()
		if a >= offset {
			break
		}
		if g.Str() == "\t" {
			col += tab - col%tab
		} else {
			col += max(1, g.Width())
		}
	}
	return col
}
func Offset(s string, col, tab int) int {
	x := 0
	tab = max(1, tab)
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		a, z := g.Positions()
		w := max(1, g.Width())
		if g.Str() == "\t" {
			w = tab - x%tab
		}
		if col < x+w {
			if col-x >= (w+1)/2 {
				return z
			}
			return a
		}
		x += w
	}
	return len(s)
}

// LSPPosition/LSPOffset use UTF-16 code units, the LSP default encoding.
func (b *Buffer) LSPPosition(offset int) (int, int) {
	r, c := b.Position(offset)
	units := 0
	for _, r := range b.Line(r)[:c] {
		units++
		if r > 0xffff {
			units++
		}
	}
	return r, units
}
func (b *Buffer) LSPOffset(row, col int) int {
	row = max(0, min(row, b.LineCount()-1))
	units := 0
	for i, r := range b.Line(row) {
		n := 1
		if r > 0xffff {
			n = 2
		}
		if units+n > col {
			return b.LineStart(row) + i
		}
		units += n
		if units == col {
			return b.LineStart(row) + i + utf8.RuneLen(r)
		}
	}
	if col <= 0 {
		return b.LineStart(row)
	}
	return b.LineEnd(row)
}

// NormalizeSelections merges overlapping selections, including duplicate carets.
func NormalizeSelections(s []Selection) []Selection {
	s = append([]Selection(nil), s...)
	sort.SliceStable(s, func(i, j int) bool { a, _ := s[i].Range(); b, _ := s[j].Range(); return a < b })
	var out []Selection
	for _, v := range s {
		lo, hi := v.Range()
		if len(out) > 0 {
			a, z := out[len(out)-1].Range()
			if lo < z || lo == a {
				out[len(out)-1] = Selection{a, max(z, hi)}
				continue
			}
		}
		out = append(out, v)
	}
	return out
}

// window returns whole lines from extra rows before p's row through extra rows
// after it, including the newline that ends the last one, and the window's
// document offset. Grapheme clusters never span a newline, so the string
// functions below give the same answer inside a window that starts on a line.
func (b *Buffer) window(p, extra int) (string, int) {
	r, _ := b.Position(p)
	lo := b.LineStart(r - extra)
	hi := min(b.length, b.LineEnd(min(r+extra, len(b.lines)-1))+1)
	return b.Slice(lo, hi), lo
}

// complete reports whether a result computed in the window [lo, lo+n) can be
// trusted: it must not stop at a window edge that is not a document edge.
func (b *Buffer) complete(lo, n int, results ...int) bool {
	for _, q := range results {
		if q == lo && lo > 0 || q == lo+n && lo+n < b.length {
			return false
		}
	}
	return true
}

// Snap, Prev and Next are the Buffer forms of the package functions; they read
// only the lines around p instead of the whole document.
func (b *Buffer) Snap(p int) int {
	p = max(0, min(p, b.length))
	s, lo := b.window(p, 0)
	return lo + Snap(s, p-lo)
}
func (b *Buffer) Prev(p int) int {
	p = max(0, min(p, b.length))
	if p == 0 {
		return 0
	}
	s, lo := b.window(p-1, 0)
	return lo + Prev(s, p-lo)
}
func (b *Buffer) Next(p int) int {
	p = max(0, min(p, b.length))
	s, lo := b.window(p, 0)
	return lo + Next(s, p-lo)
}

// WordMove and WordRange can cross any number of blank lines, so the window
// grows until the result no longer touches its edges.
func (b *Buffer) WordMove(p, dir int) int {
	p = max(0, min(p, b.length))
	for extra := 1; ; extra *= 4 {
		s, lo := b.window(p, extra)
		if q := lo + WordMove(s, p-lo, dir); b.complete(lo, len(s), q) {
			return q
		}
	}
}
func (b *Buffer) WordRange(p int) (int, int) {
	p = max(0, min(p, b.length))
	for extra := 1; ; extra *= 4 {
		s, lo := b.window(p, extra)
		x, y := WordRange(s, p-lo)
		if b.complete(lo, len(s), lo+x, lo+y) {
			return lo + x, lo + y
		}
	}
}
