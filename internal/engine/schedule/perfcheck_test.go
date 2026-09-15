package schedule

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cosmin2dor/vakt/internal/engine/index"
	"github.com/cosmin2dor/vakt/internal/events"
)

// TestPerfCheck_ApplyRearm is milestone4's run-performance-sanity-pass: the
// min-heap design is supposed to make a single task change O(log n), not a
// full-heap rebuild, as the vault grows to "a few hundred tasks across a
// few dozen files" — see deploy/RUNBOOK.md for the recorded number, and
// internal/engine/index/perfcheck_test.go / internal/api/perfcheck_test.go
// for the index and HTTP legs of the same check.
func TestPerfCheck_ApplyRearm(t *testing.T) {
	shape := index.HouseholdPlusShape()
	files := index.GenerateSyntheticVault(shape)

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

	idx := index.New(root, time.UTC)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	taskCount := len(idx.Tasks())

	bus := events.New[index.Change]()
	sub := bus.Subscribe(1)
	sched := New(idx.Tasks(), sub, nil) // full vault already scheduled

	// One task change, folded into the already-populated heap.
	editedPath := "household-00.md"
	edited := files[editedPath] + "- [ ] One more chore @id(perfcheck_extra) @schedule(0 10 * * *) @state(active)\n"
	if err := os.WriteFile(filepath.Join(root, editedPath), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	change, err := idx.ReconcileFile(editedPath)
	if err != nil {
		t.Fatalf("ReconcileFile: %v", err)
	}

	applyStart := time.Now()
	sched.apply(change)
	rearmed := sched.rearm()
	applyElapsed := time.Since(applyStart)
	_ = rearmed

	t.Logf("perfcheck: Scheduler apply()+rearm() for one change, %d tasks scheduled: %s", taskCount, applyElapsed)
}
