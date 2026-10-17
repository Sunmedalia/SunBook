package editor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInspectDiskMetadataFullAndReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := InspectDisk(path, DiskState{}, true)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("other"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	poll, err := InspectDisk(path, first, false)
	if err != nil {
		t.Fatal(err)
	}
	if poll.Hash != first.Hash {
		t.Fatal("metadata poll read unchanged metadata")
	}
	full, err := InspectDisk(path, first, true)
	if err != nil {
		t.Fatal(err)
	}
	if full.Hash == first.Hash {
		t.Fatal("full check missed modified content")
	}
	replacement := path + ".new"
	if err := os.WriteFile(replacement, []byte("third"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(replacement, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	replaced, err := InspectDisk(path, full, false)
	if err != nil {
		t.Fatal(err)
	}
	if replaced.SameMetadata(full) || replaced.Hash == full.Hash {
		t.Fatal("identity replacement was missed")
	}
}
