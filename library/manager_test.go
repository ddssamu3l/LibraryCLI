package library

import (
	"os"
	"path/filepath"
	"testing"
)

func newManager(t *testing.T) *LibraryManager {
	dir := t.TempDir()
	mgr, err := NewLibraryManager(filepath.Join(dir, "lib.db"))
	if err != nil {
		t.Fatalf("mgr: %v", err)
	}
	t.Cleanup(func() { mgr.Close() })
	return mgr
}

func TestAddBookFromFile(t *testing.T) {
	mgr := newManager(t)
	tmp := filepath.Join(t.TempDir(), "bk.txt")
	if err := os.WriteFile(tmp, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	id, err := mgr.AddBookFromFile("Hello", "Anon", tmp)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	b, err := mgr.GetBook(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if b.Content == "" {
		t.Fatalf("content empty")
	}
}
