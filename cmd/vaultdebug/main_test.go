package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedVault writes a small vault covering valid tasks, a missing @id, a
// duplicate @id (cross-file), and an invalid @id.
func seedVault(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	a := "# Chores\n\n" +
		"- Take out trash @id(trash) @state(active) @schedule(0 8 * * *)\n" +
		"- Water plants @id(plants) @state(active) @once(2026-01-01T08:00)\n" +
		"- Missing id line @state(active)\n" +
		"- Bad id @id(BAD ID!) @state(active)\n"
	b := "- Duplicate @id(trash) @state(paused)\n"
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte(a), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.md"), []byte(b), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRun_SeededVault_EmitsTasksAndDiagnostics(t *testing.T) {
	dir := seedVault(t)

	var stdout, stderr bytes.Buffer
	code := run([]string{dir}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()

	for _, want := range []string{
		"== tasks (2) ==",
		"- plants",
		"- trash",
		"@schedule: [0 8 * * *]",
		"== diagnostics (3) ==",
		"[warning] missing_id",
		"[error] invalid_id",
		"[error] duplicate_id",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q; got:\n%s", want, out)
		}
	}
}

func TestRun_NonexistentPath_ReturnsErrorNotPanic(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"/definitely/does/not/exist"}, &stdout, &stderr)

	if code == 0 {
		t.Fatalf("expected non-zero exit code, got 0")
	}
	if stderr.Len() == 0 {
		t.Fatal("expected an error message on stderr")
	}
}

func TestRun_PathIsAFile_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "notadir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{file}, &stdout, &stderr)

	if code == 0 {
		t.Fatal("expected non-zero exit code for a non-directory path")
	}
}

func TestRun_NoArgs_PrintsUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage:") {
		t.Errorf("expected usage message, got %q", stderr.String())
	}
}
