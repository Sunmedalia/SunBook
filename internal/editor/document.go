package editor

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"sunbook/internal/text"
)

var ErrConflict = errors.New("file changed on disk; reload or explicitly overwrite")

type Document struct {
	Buffer         *text.Buffer
	Path           string
	CRLF, BOM      bool
	saved          string
	diskHash       [32]byte
	diskState      DiskState
	saveGeneration uint64
	Exists         bool
}

func New() *Document { return &Document{Buffer: text.New("")} }
func Open(path string) (*Document, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	// Resolve existing symlinks so an atomic save never replaces the link itself.
	if resolved, e := filepath.EvalSymlinks(path); e == nil {
		path = resolved
	}
	var before os.FileInfo
	if st, e := os.Stat(path); e == nil && !st.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", filepath.Base(path))
	} else if e != nil && !os.IsNotExist(e) {
		return nil, e
	} else {
		before = st
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		d := New()
		d.Path = path
		return d, nil
	}
	if err != nil {
		return nil, err
	}
	if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		return nil, fmt.Errorf("%s is binary or not UTF-8", filepath.Base(path))
	}
	d := &Document{Path: path, Exists: true, diskHash: sha256.Sum256(data)}
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !(DiskState{info: before}).SameMetadata(DiskState{info: st}) {
		return nil, ErrConflict
	}
	d.diskState = DiskState{info: st, Hash: d.diskHash}
	d.BOM = bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf})
	if d.BOM {
		data = data[3:]
	}
	d.CRLF = bytes.Contains(data, []byte("\r\n"))
	s := strings.ReplaceAll(string(data), "\r\n", "\n")
	d.Buffer = text.New(s)
	d.saved = s
	return d, nil
}
func (d *Document) Dirty() bool { return d.Buffer.String() != d.saved }
func (d *Document) Name() string {
	if d.Path == "" {
		return "Untitled"
	}
	return filepath.Base(d.Path)
}
func (d *Document) DiskChanged() bool {
	if d.Path == "" {
		return false
	}
	state, err := InspectDisk(d.Path, DiskState{}, true)
	if err != nil {
		return d.Exists
	}
	return !d.Exists || state.Hash != d.diskHash
}
func (d *Document) Save(force bool) error {
	if d.Path == "" {
		return errors.New("choose a path with Save As")
	}
	if st, e := os.Lstat(d.Path); e == nil && !st.Mode().IsRegular() {
		return errors.New("save target is no longer a regular file; reopen the target")
	}
	if !force && d.DiskChanged() {
		return ErrConflict
	}
	s := d.Buffer.String()
	out := s
	if d.CRLF {
		out = strings.ReplaceAll(s, "\n", "\r\n")
	}
	if d.BOM {
		out = "\ufeff" + out
	}
	mode := os.FileMode(0644)
	if st, err := os.Stat(d.Path); err == nil {
		mode = st.Mode().Perm()
		if mode&0222 == 0 {
			return errors.New("file is read-only")
		}
	}
	f, err := os.CreateTemp(filepath.Dir(d.Path), ".sunbook-save-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(mode); err == nil {
		_, err = f.WriteString(out)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if !force && d.DiskChanged() {
		return ErrConflict
	}
	if err = os.Rename(name, d.Path); err != nil {
		return err
	}
	if dir, e := os.Open(filepath.Dir(d.Path)); e == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	d.saved = s
	d.diskHash = sha256.Sum256([]byte(out))
	d.Exists = true
	d.saveGeneration++
	if st, e := os.Stat(d.Path); e == nil {
		d.diskState = DiskState{info: st, Hash: d.diskHash}
	}
	return nil
}

type View struct {
	Doc        *Document
	Selections []text.Selection
	Top, Left  int
	Preferred  int
}

func NewView(d *Document) *View {
	return &View{Doc: d, Selections: []text.Selection{{}}, Preferred: -1}
}
func (v *View) Head() int {
	if len(v.Selections) == 0 {
		v.Selections = []text.Selection{{}}
	}
	return v.Selections[len(v.Selections)-1].Head
}
func (v *View) Set(p int, selecting bool) {
	p = text.Snap(v.Doc.Buffer.String(), p)
	if !selecting {
		v.Selections = []text.Selection{{Anchor: p, Head: p}}
	} else {
		v.Selections[len(v.Selections)-1].Head = p
	}
	v.Preferred = -1
}
func (v *View) Insert(s string) bool {
	return v.ReplaceRanges(v.Selections, s)
}

// ReplaceRanges preserves the original selections for undo, even when a cut
// or backspace has expanded the affected ranges beyond the original carets.
func (v *View) ReplaceRanges(ranges []text.Selection, s string) bool {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	ranges = text.NormalizeSelections(ranges)
	var edits []text.Edit
	var after []text.Selection
	delta := 0
	for _, sel := range ranges {
		lo, hi := sel.Range()
		if lo != hi || s != "" {
			edits = append(edits, text.Edit{Start: lo, End: hi, Text: s})
		}
		p := lo + delta + len(s)
		after = append(after, text.Selection{Anchor: p, Head: p})
		delta += len(s) - (hi - lo)
	}
	after = text.NormalizeSelections(after)
	if v.Doc.Buffer.Apply(edits, v.Selections, after) {
		v.Selections = after
		v.Preferred = -1
		return true
	}
	return false
}
func (v *View) Delete(back, word bool) bool {
	s := v.Doc.Buffer.String()
	ranges := append([]text.Selection(nil), v.Selections...)
	for i, sel := range ranges {
		if !sel.Empty() {
			continue
		}
		p := sel.Head
		q := p
		if word {
			dir := 1
			if back {
				dir = -1
			}
			q = text.WordMove(s, p, dir)
		} else if back {
			q = text.Prev(s, p)
			if q+1 == p && p < len(s) {
				pairs := map[byte]byte{'(': ')', '[': ']', '{': '}', 34: 34, 39: 39}
				if close, ok := pairs[s[q]]; ok && s[p] == close {
					ranges[i] = text.Selection{Anchor: q, Head: p + 1}
					continue
				}
			}
		} else {
			q = text.Next(s, p)
		}
		ranges[i] = text.Selection{Anchor: p, Head: q}
	}
	return v.ReplaceRanges(ranges, "")
}
func (v *View) Move(dx, dy int, selecting, word bool, tab int) {
	b := v.Doc.Buffer
	s := b.String()
	for i, sel := range v.Selections {
		p := sel.Head
		if !selecting && !sel.Empty() && dy == 0 && !word {
			lo, hi := sel.Range()
			if dx < 0 {
				p = lo
			} else {
				p = hi
			}
		} else if dy != 0 {
			r, c := b.Position(p)
			goal := v.Preferred
			if goal < 0 {
				goal = text.Column(b.Line(r), c, tab)
			}
			r = max(0, min(r+dy, b.LineCount()-1))
			p = b.LineStart(r) + text.Offset(b.Line(r), goal, tab)
			if len(v.Selections) == 1 {
				v.Preferred = goal
			}
		} else if word {
			p = text.WordMove(s, p, dx)
		} else if dx < 0 {
			p = text.Prev(s, p)
		} else {
			p = text.Next(s, p)
		}
		if selecting {
			v.Selections[i].Head = p
		} else {
			v.Selections[i] = text.Selection{Anchor: p, Head: p}
		}
	}
	if dy == 0 {
		v.Preferred = -1
	}
}
func (v *View) Undo(redo bool) bool {
	var s []text.Selection
	var ok bool
	if redo {
		s, ok = v.Doc.Buffer.Redo()
	} else {
		s, ok = v.Doc.Buffer.Undo()
	}
	if ok {
		v.Selections = s
		v.Preferred = -1
	}
	return ok
}
func (v *View) SelectNext() bool {
	s := v.Doc.Buffer.String()
	sel := v.Selections[len(v.Selections)-1]
	lo, hi := sel.Range()
	if lo == hi {
		lo, hi = text.WordRange(s, lo)
		v.Selections[len(v.Selections)-1] = text.Selection{Anchor: lo, Head: hi}
		return hi > lo
	}
	needle := s[lo:hi]
	at := strings.Index(s[hi:], needle)
	if at >= 0 {
		at += hi
	} else {
		at = strings.Index(s[:lo], needle)
	}
	if at < 0 {
		return false
	}
	for _, v := range v.Selections {
		a, z := v.Range()
		if at < z && at+len(needle) > a {
			return false
		}
	}
	v.Selections = append(v.Selections, text.Selection{Anchor: at, Head: at + len(needle)})
	return true
}
func (v *View) SelectedText() string {
	var parts []string
	s := v.Doc.Buffer.String()
	for _, sel := range v.Selections {
		a, z := sel.Range()
		if a == z {
			row, _ := v.Doc.Buffer.Position(a)
			a = v.Doc.Buffer.LineStart(row)
			z = v.Doc.Buffer.LineEnd(row)
			if z < len(s) {
				z++
			}
		}
		parts = append(parts, s[a:z])
	}
	return strings.Join(parts, "\n")
}

// Remap updates a second view sharing the document after an edit.
func (v *View) Clamp() {
	source := v.Doc.Buffer.String()
	for i, s := range v.Selections {
		v.Selections[i] = text.Selection{Anchor: text.Snap(source, s.Anchor), Head: text.Snap(source, s.Head)}
	}
	v.Top = max(0, min(v.Top, v.Doc.Buffer.LineCount()-1))
}
