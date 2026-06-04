package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWrite_GeneratesReadableFile(t *testing.T) {
	// Generated source files should be world-readable (0644), not the 0600
	// that os.CreateTemp produces, so teammates can read them like ordinary
	// checked-in source.
	dest := filepath.Join(t.TempDir(), "blueprints.go")

	if err := atomicWrite(dest, func(w io.Writer) error {
		_, _ = io.WriteString(w, "package blueprints\n")
		return nil
	}); err != nil {
		t.Fatalf("atomicWrite: %v", err)
	}

	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("generated file mode = %v, want %v", got, os.FileMode(0o644))
	}
}
