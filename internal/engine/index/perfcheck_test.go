package index

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestPerfCheck_BuildAndReconcile is milestone4's run-performance-sanity-pass:
// a wall-clock sanity check (not a benchmark suite) that Index.Build (cold
// start) and Index.ReconcileFile (single-file edit, vault already loaded)
// stay comfortably fast at "a few hundred tasks across a few dozen files".
// Runs in well under a second at this scale — see deploy/RUNBOOK.md for the
// recorded numbers, and internal/engine/schedule and internal/api for the
// scheduler and GET /tasks legs of the same check.
func TestPerfCheck_BuildAndReconcile(t *testing.T) {
	shape := HouseholdPlusShape()
	files := GenerateSyntheticVault(shape)

	root := t.TempDir()
	for rel, content := range files {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	buildStart := time.Now()
	idx := New(root, time.UTC)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	buildElapsed := time.Since(buildStart)

	taskCount := len(idx.Tasks())
	t.Logf("perfcheck: Index.Build() over %d files, %d tasks, %d diagnostics: %s",
		shape.Files, taskCount, len(idx.Diagnostics()), buildElapsed)

	// Single-file edit AFTER the full vault is loaded — the "does it get
	// slower as the vault grows" check (indexFileLocked's cross-file
	// duplicate-id check is a map lookup, not a scan of other files).
	editedPath := "household-00.md"
	edited := files[editedPath] + fmt.Sprintf("- [ ] One more chore @id(%s) @schedule(0 10 * * *) @state(active)\n", "perfcheck_extra")
	if err := os.WriteFile(filepath.Join(root, editedPath), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	reconcileStart := time.Now()
	if _, err := idx.ReconcileFile(editedPath); err != nil {
		t.Fatalf("ReconcileFile: %v", err)
	}
	reconcileElapsed := time.Since(reconcileStart)
	t.Logf("perfcheck: Index.ReconcileFile() single file, %d tasks already loaded: %s", taskCount, reconcileElapsed)
}
