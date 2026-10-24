package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
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

func TestWriteOutput_FailedGenerationKeepsExistingFile(t *testing.T) {
	// Arrange: an earlier run left generated code at the output path, and the
	// next generation fails after it has already written part of its output.
	dir := t.TempDir()
	dest := filepath.Join(dir, "blueprints.go")
	const previous = "package blueprints\n\n// previous output\n"
	if err := os.WriteFile(dest, []byte(previous), 0o600); err != nil {
		t.Fatal(err)
	}
	errGenerate := errors.New("generation failed")
	var stdout, stderr bytes.Buffer

	// Act
	exitCode := writeOutput(&stdout, &stderr, dest, func(w io.Writer) error {
		_, _ = io.WriteString(w, "package broken\n")
		return errGenerate
	})

	// Assert
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}
	if !strings.Contains(stderr.String(), errGenerate.Error()) {
		t.Fatalf("stderr = %q, want it to report %q", stderr.String(), errGenerate)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want nothing when writing to a file", stdout.String())
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != previous {
		t.Fatalf("output file = %q, want the previous content %q", got, previous)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "blueprints.go" {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("output directory = %v, want only blueprints.go (temp file must be removed)", names)
	}
}
