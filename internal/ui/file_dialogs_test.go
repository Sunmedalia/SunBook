package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v3"
)

func TestUntitledSaveUsesCompactInputAndKeepsDraftOnCancel(t *testing.T) {
	a, _ := testApp(t)
	a.typeInput("draft 👩‍💻\n")
	doc := a.view().Doc
	a.save(doc, nil)
	a.draw()
	if a.modal == nil || !a.modal.Compact || a.modalRect.H != 4 || a.modalRect.W > 64 || len(a.modal.Items) != 0 || a.modal.InputRect.H != 1 {
		t.Fatal("Save As should contain only a compact filename field")
	}
	a.modalAccept()
	if a.modal == nil || a.view().Doc != doc || doc.Path != "" {
		t.Fatal("empty Save input must keep the prompt and untitled draft")
	}
	a.paste("cancelled.txt")
	a.key(tcell.NewEventKey(tcell.KeyEsc, "", 0))
	if a.view().Doc != doc || doc.Path != "" || !doc.Dirty() || doc.Buffer.String() != "draft 👩‍💻\n" {
		t.Fatal("cancelling Save changed the untitled draft")
	}
	if _, err := os.Stat(filepath.Join(a.root, "cancelled.txt")); !os.IsNotExist(err) {
		t.Fatal("cancelling Save created a file", err)
	}
	a.save(doc, nil)
	a.paste("草稿-👩‍💻.txt")
	a.modalAccept()
	data, err := os.ReadFile(filepath.Join(a.root, "草稿-👩‍💻.txt"))
	if err != nil || string(data) != "draft 👩‍💻\n" || doc.Dirty() || a.modal != nil {
		t.Fatal("compact Save did not preserve and save the draft", err)
	}
	// A named document saves directly without prompting again.
	a.typeInput("more")
	a.save(doc, nil)
	if a.modal != nil || doc.Dirty() {
		t.Fatal("saving a named document should not open an input")
	}
}

func TestNewSingleInputCreatesUntitledOrNamedFileWithoutOverwriting(t *testing.T) {
	a, _ := testApp(t)
	initial := a.view().Doc
	clickCommand(t, a, "file.new")
	a.draw()
	if a.modal == nil || !a.modal.Compact || !a.modal.InputEnabled || len(a.modal.Items) != 0 || a.modalRect.H != 4 || a.view().Doc != initial {
		t.Fatal("New should open a single compact optional filename field")
	}
	a.modalAccept()
	untitled := a.view().Doc
	if untitled == initial || untitled.Path != "" || a.modal != nil {
		t.Fatal("empty input did not create an unnamed editor tab")
	}
	a.typeInput("keep this draft")
	clickCommand(t, a, "file.new")
	a.draw()
	if a.modal == nil || a.modal.Title != "New File" || a.modalRect.H != 4 {
		t.Fatal("New did not open the optional filename field directly")
	}
	path := filepath.Join(a.root, "existing.txt")
	if err := os.WriteFile(path, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	a.paste("existing.txt")
	a.modalAccept()
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "original" || a.modal == nil || !strings.Contains(a.modal.Hint, "already exists") {
		t.Fatal("New overwrote an existing file or lost the retry prompt", err)
	}
	a.key(tcell.NewEventKey(tcell.KeyCtrlU, "", 0))
	a.paste("新文件.go")
	a.modalAccept()
	created := a.view().Doc
	if a.modal != nil || created.Path != filepath.Join(a.root, "新文件.go") || !created.Exists || untitled.Buffer.String() != "keep this draft" || !untitled.Dirty() {
		t.Fatal("creating a named file damaged an existing draft or failed to open it")
	}
	if data, err := os.ReadFile(created.Path); err != nil || len(data) != 0 {
		t.Fatal("named file was not created empty on disk", err)
	}
	count := len(a.docs)
	clickCommand(t, a, "file.new")
	a.paste("cancelled.go")
	a.key(tcell.NewEventKey(tcell.KeyEsc, "", 0))
	if a.modal != nil || a.view().Doc != created || len(a.docs) != count {
		t.Fatal("cancelling New changed the active document or created a tab")
	}
	if _, err := os.Stat(filepath.Join(a.root, "cancelled.go")); !os.IsNotExist(err) {
		t.Fatal("cancelling New created a file", err)
	}
	clickCommand(t, a, "file.new")
	a.paste("   ")
	a.modalAccept()
	if a.modal != nil || a.view().Doc.Path != "" || len(a.docs) != count+1 {
		t.Fatal("whitespace-only input should create an unnamed file")
	}
}
