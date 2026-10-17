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
type snapshot struct {
	Pieces     []piece
	Selections []Selection
}
type transaction struct {
	Before, After     snapshot
	Forward, Backward []Edit
}

type Buffer struct {
	length            int
	historyGeneration uint64
	maintenanceBytes  int
	original          string
	added             []byte
	pieces            []piece
	cache             string
	valid             bool
	lines             []int
	history           []transaction
	index             int
	Version           int
	LastEdits         []Edit
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
func (b *Buffer) Line(row int) string { return b.String()[b.LineStart(row):b.LineEnd(row)] }
func (b *Buffer) Position(offset int) (row, col int) {
	offset = max(0, min(offset, b.length))
	row = sort.Search(len(b.lines), func(i int) bool { return b.lines[i] > offset }) - 1
	return row, offset - b.lines[row]
}

// Update newline offsets without scanning or materializing unchanged text.
// Multiple edits are applied from right to left in the original coordinate space.
func (b *Buffer) updateLines(e Edit) {
	left := sort.Search(len(b.lines), func(i int) bool { return b.lines[i] > e.Start })
	right := sort.Search(len(b.lines), func(i int) bool { return b.lines[i] > e.End })
	n := strings.Count(e.Text, "\n")
	oldLen := len(b.lines)
	nextLen := oldLen - (right - left) + n
	if nextLen > oldLen {
		b.lines = append(b.lines, make([]int, nextLen-oldLen)...)
	}
	copy(b.lines[left+n:], b.lines[right:oldLen])
	b.lines = b.lines[:nextLen]
	delta := len(e.Text) - (e.End - e.Start)
	for i := left + n; i < len(b.lines); i++ {
		b.lines[i] += delta
	}
	at := left
	for i := 0; i < len(e.Text); i++ {
		if e.Text[i] == '\n' {
			b.lines[at] = e.Start + i + 1
			at++
		}
	}
	b.length += delta
}
func (b *Buffer) snap(sels []Selection) snapshot {
	return snapshot{append([]piece(nil), b.pieces...), append([]Selection(nil), sels...)}
}
func (b *Buffer) restore(s snapshot) []Selection {
	for i := len(b.LastEdits) - 1; i >= 0; i-- {
		b.updateLines(b.LastEdits[i])
	}
	b.pieces = append([]piece(nil), s.Pieces...)
	b.valid = false
	b.Version++
	return append([]Selection(nil), s.Selections...)
}
func (b *Buffer) replace(start, end int, s string) {
	b.updateLines(Edit{Start: start, End: end, Text: s})
	b.maintenanceBytes += len(s) + (end - start)
	var ps []piece
	pos := 0
	insert := func() {
		if s != "" {
			at := len(b.added)
			b.added = append(b.added, s...)

			ps = append(ps, piece{true, at, len(s)})
		}
	}
	inserted := false
	for _, p := range b.pieces {
		pe := pos + p.Len
		if pe <= start {
			ps = append(ps, p)
		} else if pos >= end {
			if !inserted {
				insert()
				inserted = true
			}
			ps = append(ps, p)
		} else {
			if start > pos {
				ps = append(ps, piece{p.Added, p.Start, start - pos})
			}
			if !inserted {
				insert()
				inserted = true
			}
			if end < pe {
				ps = append(ps, piece{p.Added, p.Start + end - pos, pe - end})
			}
		}
		pos = pe
	}
	if !inserted {
		insert()
	}
	b.pieces = nil
	for _, p := range ps {
		if p.Len == 0 {
			continue
		}
		n := len(b.pieces)
		if n > 0 && b.pieces[n-1].Added == p.Added && b.pieces[n-1].Start+b.pieces[n-1].Len == p.Start {
			b.pieces[n-1].Len += p.Len
		} else {
			b.pieces = append(b.pieces, p)
		}
	}
	b.valid = false
}

// Apply validates non-overlapping edits against the original document and commits
// all of them as a single undo transaction. Callers pass the resulting selections.
func (b *Buffer) Apply(edits []Edit, before, after []Selection) bool {
	if len(edits) == 0 {
		return false
	}
	edits = append([]Edit(nil), edits...)
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].Start < edits[j].Start })
	s := b.String()
	for i, e := range edits {
		if e.Start < 0 || e.End < e.Start || e.End > len(s) || !utf8.ValidString(e.Text) || !boundary(s, e.Start) || !boundary(s, e.End) {
			return false
		}
		if i > 0 && (edits[i-1].End > e.Start || edits[i-1].Start == e.Start) {
			return false
		}
	}
	for i := range edits {
		edits[i].Text = strings.Clone(edits[i].Text)
	}
	t := transaction{Before: b.snap(before), Forward: edits}
	delta := 0
	for _, e := range edits {
		t.Backward = append(t.Backward, Edit{Start: e.Start + delta, End: e.Start + delta + len(e.Text), Text: strings.Clone(s[e.Start:e.End])})
		delta += len(e.Text) - (e.End - e.Start)
	}
	for i := len(edits) - 1; i >= 0; i-- {
		e := edits[i]
		b.replace(e.Start, e.End, e.Text)
	}
	b.Version++
	b.LastEdits = edits
	t.After = b.snap(after)
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
func boundary(s string, p int) bool { return p == len(s) || utf8.RuneStart(s[p]) }
func (b *Buffer) CanUndo() bool     { return b.index > 0 }
func (b *Buffer) CanRedo() bool     { return b.index < len(b.history) }
func (b *Buffer) Undo() ([]Selection, bool) {
	if b.index == 0 {
		return nil, false
	}
	b.index--
	b.LastEdits = b.history[b.index].Backward
	return b.restore(b.history[b.index].Before), true
}
func (b *Buffer) Redo() ([]Selection, bool) {
	if b.index >= len(b.history) {
		return nil, false
	}
	b.LastEdits = b.history[b.index].Forward
	s := b.restore(b.history[b.index].After)
	b.index++
	return s, true
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
