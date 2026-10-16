package editor

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"sunbook/internal/text"
)

func TestSaveEncodingPermissionsAndConflict(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "space 中文.go")
	raw := []byte("\ufeff你好\r\nworld\r\n")
	if e := os.WriteFile(path, raw, 0750); e != nil {
		t.Fatal(e)
	}
	d, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	if !d.CRLF || !d.BOM || d.Dirty() {
		t.Fatal("metadata")
	}
	v := NewView(d)
	v.Insert("x")
	if !d.Dirty() {
		t.Fatal("not dirty")
	}
	if e = d.Save(false); e != nil {
		t.Fatal(e)
	}
	out, _ := os.ReadFile(path)
	if !bytes.Equal(out, append([]byte("\xef\xbb\xbfx"), raw[3:]...)) {
		t.Fatalf("roundtrip %q", out)
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0750 {
		t.Fatal("permissions lost")
	}
	os.WriteFile(path, []byte("external"), 0750)
	v.Insert("y")
	if e = d.Save(false); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	out, _ = os.ReadFile(path)
	if string(out) != "external" {
		t.Fatal("overwrote external changes")
	}
	if e = d.Save(true); e != nil || d.Dirty() {
		t.Fatal("force", e)
	}
}
func TestFailedSaveKeepsBuffer(t *testing.T) {
	d := New()
	d.Path = filepath.Join(t.TempDir(), "missing", "a")
	v := NewView(d)
	v.Insert("keep me")
	if d.Save(false) == nil || d.Buffer.String() != "keep me" || !d.Dirty() {
		t.Fatal("failed save lost data")
	}
}
func TestRejectBinaryAndSaveThroughSymlink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "binary")
	os.WriteFile(path, []byte{0, 1}, 0644)
	if _, e := Open(path); e == nil {
		t.Fatal("accepted binary")
	}
	os.WriteFile(path, []byte{0xff, 0xfe}, 0644)
	if _, e := Open(path); e == nil {
		t.Fatal("accepted bad UTF8")
	}
	os.WriteFile(path, []byte("hello"), 0644)
	link := filepath.Join(dir, "link")
	if e := os.Symlink(path, link); e != nil {
		t.Fatal(e)
	}
	d, e := Open(link)
	if e != nil {
		t.Fatal(e)
	}
	NewView(d).Insert("x")
	if e = d.Save(false); e != nil {
		t.Fatal(e)
	}
	st, _ := os.Lstat(link)
	if st.Mode()&os.ModeSymlink == 0 {
		t.Fatal("replaced symlink")
	}
	out, _ := os.ReadFile(path)
	if string(out) != "xhello" {
		t.Fatal(string(out))
	}
}
func TestMultiCursorUnicodeUndo(t *testing.T) {
	d := New()
	d.Buffer = text.New("你好 👩‍💻\n你好 é")
	v := NewView(d)
	v.Set(0, false)
	v.SelectNext()
	v.SelectNext()
	if len(v.Selections) != 2 {
		t.Fatal(v.Selections)
	}
	v.Insert("Hi")
	if d.Buffer.String() != "Hi 👩‍💻\nHi é" {
		t.Fatal(d.Buffer.String())
	}
	if !v.Undo(false) || d.Buffer.String() != "你好 👩‍💻\n你好 é" {
		t.Fatal("undo")
	}
	v.Selections = []text.Selection{{Anchor: d.Buffer.Len(), Head: d.Buffer.Len()}}
	v.Delete(true, false)
	if d.Buffer.String() != "你好 👩‍💻\n你好 " {
		t.Fatal("split combining sequence", d.Buffer.String())
	}
}
func TestPreferredScreenColumn(t *testing.T) {
	d := New()
	d.Buffer = text.New("中ab\nx\n中ab")
	v := NewView(d)
	v.Set(len("中a"), false)
	v.Move(0, 1, false, false, 4)
	if v.Head() != len("中ab\nx") {
		t.Fatal(v.Head())
	}
	v.Move(0, 1, false, false, 4)
	if v.Head() != len("中ab\nx\n中a") {
		t.Fatal("preferred screen column", v.Head())
	}
}

func TestDeleteRestoresCaretAndPairs(t *testing.T) {
	d := New()
	d.Buffer = text.New("abc()")
	v := NewView(d)
	v.Set(2, false)
	v.Delete(true, false)
	v.Undo(false)
	if len(v.Selections) != 1 || v.Selections[0].Anchor != 2 || v.Selections[0].Head != 2 {
		t.Fatal("undo restored expanded delete range", v.Selections)
	}
	v.Set(4, false)
	v.Delete(true, false)
	if d.Buffer.String() != "abc" {
		t.Fatal("pair deletion", d.Buffer.String())
	}
	v.Undo(false)
	if d.Buffer.String() != "abc()" || v.Head() != 4 {
		t.Fatal("pair undo")
	}
}
