package editor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func BenchmarkDiskChecks(b *testing.B) {
	root := b.TempDir()
	docs := make([]*Document, 16)
	for i := range docs {
		path := filepath.Join(root, fmt.Sprint(i))
		if err := os.WriteFile(path, []byte(strings.Repeat("x", 256*1024)), 0600); err != nil {
			b.Fatal(err)
		}
		var err error
		docs[i], err = Open(path)
		if err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		for _, d := range docs {
			if d.DiskChanged() {
				b.Fatal("unchanged file")
			}
		}
	}
}

func BenchmarkMetadataDiskChecks(b *testing.B) {
	root := b.TempDir()
	paths := make([]string, 16)
	states := make([]DiskState, 16)
	for i := range paths {
		paths[i] = filepath.Join(root, fmt.Sprint(i))
		if err := os.WriteFile(paths[i], []byte(strings.Repeat("x", 256*1024)), 0600); err != nil {
			b.Fatal(err)
		}
		var err error
		states[i], err = InspectDisk(paths[i], DiskState{}, true)
		if err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		for i, path := range paths {
			if _, err := InspectDisk(path, states[i], false); err != nil {
				b.Fatal(err)
			}
		}
	}
}
