package text

import (
	"strings"
	"testing"
)

func TestCompactionPreservesHistoryAndRejectsStaleResults(t *testing.T) {
	b := New(strings.Repeat("x", 20*1024*1024))
	b.Apply([]Edit{{Start: 0, End: b.Len(), Text: ""}}, nil, nil)
	for i := range 1001 {
		b.Apply([]Edit{{Start: b.Len(), End: b.Len(), Text: "中"}}, []Selection{{Head: i * 3}}, []Selection{{Head: (i + 1) * 3}})
	}
	if len(b.history) != 1000 {
		t.Fatal("history limit changed")
	}
	version := b.Version
	task := b.CompactionTask()
	if task == nil {
		t.Fatal("maintenance not scheduled")
	}
	result := task()
	if !b.InstallCompaction(result) || result.skip {
		t.Fatal("no compaction")
	}
	if len(b.original)+len(b.added) > 4096 || b.Version != version {
		t.Fatal("storage not reclaimed or logical version changed")
	}
	for i := 1000; i >= 1; i-- {
		sels, ok := b.Undo()
		if !ok || b.String() != strings.Repeat("中", i) || len(sels) != 1 || sels[0].Head != i*3 {
			t.Fatal("compacted undo", i)
		}
	}
	if b.CanUndo() {
		t.Fatal("extra undo history")
	}
	for i := 2; i <= 1001; i++ {
		sels, ok := b.Redo()
		if !ok || b.String() != strings.Repeat("中", i) || sels[0].Head != i*3 {
			t.Fatal("compacted redo", i)
		}
	}
	b.RetryCompaction()
	task = b.CompactionTask()
	b.Apply([]Edit{{Start: 0, End: 0, Text: "x"}}, nil, nil)
	if b.InstallCompaction(task()) {
		t.Fatal("stale compaction installed")
	}
}

func TestLineIndexAcrossMultiEditUndoBranches(t *testing.T) {
	b := New("中\n😀\ntail\n")
	before := b.String()
	edits := []Edit{{Start: 0, End: 0, Text: "a\nb\n"}, {Start: len("中\n"), End: len("中\n😀\n"), Text: "z"}}
	if !b.Apply(edits, nil, nil) {
		t.Fatal("apply")
	}
	after := b.String()
	check := func(want string) {
		t.Helper()
		if b.Len() != len(want) || b.String() != want {
			t.Fatal("text/length")
		}
		lines := strings.Split(want, "\n")
		if b.LineCount() != len(lines) {
			t.Fatal("line count")
		}
		at := 0
		for i, line := range lines {
			if b.LineStart(i) != at || b.Line(i) != line || b.LineEnd(i) != at+len(line) {
				t.Fatal("line", i)
			}
			at += len(line) + 1
		}
	}
	check(after)
	b.Undo()
	check(before)
	b.Redo()
	check(after)
	b.Undo()
	b.Apply([]Edit{{Start: b.Len(), End: b.Len(), Text: "branch\n"}}, nil, nil)
	check(before + "branch\n")
	if b.CanRedo() {
		t.Fatal("redo branch retained")
	}
}

func TestCompactionRemapsBothStoresAndRetainsRedo(t *testing.T) {
	dead := strings.Repeat("q", 9*1024*1024)
	b := New(dead + "base\n")
	b.Apply([]Edit{{Start: 0, End: len(dead), Text: ""}}, nil, nil)
	b.Apply([]Edit{{Start: b.Len(), End: b.Len(), Text: dead}}, nil, nil)
	b.Apply([]Edit{{Start: len("base\n"), End: b.Len(), Text: ""}}, nil, nil)
	for range 1001 {
		b.Apply([]Edit{{Start: b.Len(), End: b.Len(), Text: "中\n"}}, nil, nil)
	}
	b.Undo()
	before := b.String()
	version := b.Version
	if !b.InstallCompaction(b.CompactionTask()()) || b.Version != version || b.String() != before {
		t.Fatal("mixed-store compaction changed content")
	}
	if len(b.original)+len(b.added) > 8192 {
		t.Fatal("unreachable data retained")
	}
	if _, ok := b.Redo(); !ok || b.String() != "base\n"+strings.Repeat("中\n", 1001) {
		t.Fatal("redo lost during compaction")
	}
	for range 1000 {
		b.Undo()
	}
	if b.String() != "base\n中\n" || b.LineCount() != 3 {
		t.Fatal("original suffix or line history was corrupted")
	}
}

func TestCompactionWorkerSnapshotDoesNotRaceWithEditing(t *testing.T) {
	b := New(strings.Repeat("x", 9*1024*1024))
	b.Apply([]Edit{{Start: 0, End: b.Len(), Text: ""}}, nil, nil)
	for range 1001 {
		b.Apply([]Edit{{Start: 0, End: 0, Text: "x"}}, nil, nil)
	}
	task := b.CompactionTask()
	done := make(chan *Compaction, 1)
	go func() { done <- task() }()
	for range 100 {
		b.Apply([]Edit{{Start: 0, End: 0, Text: "y"}}, nil, nil)
	}
	if b.InstallCompaction(<-done) {
		t.Fatal("concurrent edit accepted stale storage")
	}
}
