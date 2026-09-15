package schedule

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmin2dor/vakt/internal/engine/directive"
	"github.com/cosmin2dor/vakt/internal/engine/index"
	"github.com/cosmin2dor/vakt/internal/engine/vault"
)

func writeTestVault(t *testing.T, name, content string) (root, absPath string) {
	t.Helper()
	root = t.TempDir()
	absPath = filepath.Join(root, name)
	require.NoError(t, os.WriteFile(absPath, []byte(content), 0o644))
	return root, absPath
}

func TestStampDirective_ByteDiffTouchesOnlyStampedSpan(t *testing.T) {
	content := "- [ ] Feed the dog @id(dog_feed) @state(active)\n" +
		"- [ ] Water plants @id(water_plants) @state(paused)\n"
	root, absPath := writeTestVault(t, "household.md", content)

	o := NewOrchestrator(vault.NewWriter(nil), root, time.UTC)
	task := index.Task{ID: "dog_feed", Path: "household.md", Line: 1, Raw: content}

	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	require.NoError(t, o.StampLastTriggered(task, now))

	got, err := os.ReadFile(absPath)
	require.NoError(t, err)

	want := "- [ ] Feed the dog @id(dog_feed) @state(active) @last_triggered(2026-09-15T12:00:00Z)\n" +
		"- [ ] Water plants @id(water_plants) @state(paused)\n"
	assert.Equal(t, want, string(got))
}

func TestStampDirective_MalformedFileNeverWritten(t *testing.T) {
	// duplicate @id on one line: G4 error-severity diagnostic.
	content := "- [ ] A @id(dup) @state(active)\n- [ ] B @id(dup) @state(active)\n"
	root, absPath := writeTestVault(t, "bad.md", content)

	o := NewOrchestrator(vault.NewWriter(nil), root, time.UTC)
	task := index.Task{ID: "dup", Path: "bad.md", Line: 1, Raw: content}

	err := o.StampDirective(task, "state", directive.Value{Type: directive.TypeEnum, Str: "triggered"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrFileInvalid))

	got, err := os.ReadFile(absPath)
	require.NoError(t, err)
	assert.Equal(t, content, string(got), "malformed file must be byte-identical after a rejected write")
}

func TestStampDirective_MultipleStampsDontStomp(t *testing.T) {
	content := "- [ ] Feed the dog @id(dog_feed) @state(active)\n"
	root, _ := writeTestVault(t, "household.md", content)

	o := NewOrchestrator(vault.NewWriter(nil), root, time.UTC)
	task := index.Task{ID: "dog_feed", Path: "household.md", Line: 1, Raw: content}

	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	require.NoError(t, o.StampLastTriggered(task, now))
	require.NoError(t, o.StampState(task, StateTriggered))

	got, err := os.ReadFile(filepath.Join(root, "household.md"))
	require.NoError(t, err)

	want := "- [ ] Feed the dog @id(dog_feed) @state(triggered) @last_triggered(2026-09-15T12:00:00Z)\n"
	assert.Equal(t, want, string(got))
}

func TestStampDirective_ApplyWriteBack(t *testing.T) {
	content := "- [ ] Feed the dog @id(dog_feed) @state(active) @skip_count(2)\n"
	root, _ := writeTestVault(t, "household.md", content)

	o := NewOrchestrator(vault.NewWriter(nil), root, time.UTC)
	task := index.Task{ID: "dog_feed", Path: "household.md", Line: 1, Raw: content}

	req := WriteBackRequest{Task: task, Directive: "skip_count", Value: directive.Value{Type: directive.TypeInteger, Int: 1}}
	require.NoError(t, o.ApplyWriteBack(req))

	got, err := os.ReadFile(filepath.Join(root, "household.md"))
	require.NoError(t, err)
	assert.Equal(t, "- [ ] Feed the dog @id(dog_feed) @state(active) @skip_count(1)\n", string(got))
}

func TestStampDirective_CRLFPreserved(t *testing.T) {
	content := "- [ ] Feed the dog @id(dog_feed) @state(active)\r\n" +
		"- [ ] Water plants @id(water_plants) @state(paused)\r\n"
	root, _ := writeTestVault(t, "household.md", content)

	o := NewOrchestrator(vault.NewWriter(nil), root, time.UTC)
	task := index.Task{ID: "water_plants", Path: "household.md", Line: 2, Raw: content}

	require.NoError(t, o.StampState(task, StateActive))

	got, err := os.ReadFile(filepath.Join(root, "household.md"))
	require.NoError(t, err)

	want := "- [ ] Feed the dog @id(dog_feed) @state(active)\r\n" +
		"- [ ] Water plants @id(water_plants) @state(active)\r\n"
	assert.Equal(t, want, string(got))
}
