package schedule

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmin2dor/vakt/internal/engine/directive"
	"github.com/cosmin2dor/vakt/internal/engine/dispatch"
	"github.com/cosmin2dor/vakt/internal/engine/index"
	"github.com/cosmin2dor/vakt/internal/engine/vault"
)

func activeTask(path, content string) index.Task {
	return index.Task{
		ID:     "dog_feed",
		Path:   path,
		Line:   1,
		Raw:    content,
		Values: map[string]directive.Value{"state": {Type: directive.TypeEnum, Str: "active"}},
	}
}

func failedTask(path, content string) index.Task {
	return index.Task{
		ID:     "dog_feed",
		Path:   path,
		Line:   1,
		Raw:    content,
		Values: map[string]directive.Value{"state": {Type: directive.TypeEnum, Str: "failed"}},
	}
}

// TestApplyDeliveryOutcome_FailureStampsStateAndReason covers G13: a
// failed outcome moves an active task to failed and records @reason,
// touching only those two directive spans.
func TestApplyDeliveryOutcome_FailureStampsStateAndReason(t *testing.T) {
	content := "- [ ] Feed the dog @id(dog_feed) @state(active)\n"
	root, _ := writeTestVault(t, "household.md", content)

	o := NewOrchestrator(vault.NewWriter(nil), root, time.UTC)
	task := activeTask("household.md", content)

	outcome := dispatch.Outcome{Accepted: false, Detail: "0/1 accepted; host.example.com: returned 500"}
	require.NoError(t, o.ApplyDeliveryOutcome(task, outcome))

	got, err := os.ReadFile(filepath.Join(root, "household.md"))
	require.NoError(t, err)
	want := "- [ ] Feed the dog @id(dog_feed) @state(failed) @reason(delivery failed: 0/1 accepted; host.example.com: returned 500)\n"
	assert.Equal(t, want, string(got))
}

// TestApplyDeliveryOutcome_AcceptedIsNoWriteback covers the accepted
// branch: nothing is stamped, so the file must be byte-identical.
func TestApplyDeliveryOutcome_AcceptedIsNoWriteback(t *testing.T) {
	content := "- [ ] Feed the dog @id(dog_feed) @state(triggered)\n"
	root, _ := writeTestVault(t, "household.md", content)

	o := NewOrchestrator(vault.NewWriter(nil), root, time.UTC)
	task := index.Task{
		ID:     "dog_feed",
		Path:   "household.md",
		Line:   1,
		Raw:    content,
		Values: map[string]directive.Value{"state": {Type: directive.TypeEnum, Str: "triggered"}},
	}

	require.NoError(t, o.ApplyDeliveryOutcome(task, dispatch.Outcome{Accepted: true, Detail: "1/1 accepted"}))

	got, err := os.ReadFile(filepath.Join(root, "household.md"))
	require.NoError(t, err)
	assert.Equal(t, content, string(got), "an accepted outcome must never write back")
}

// TestClearFailure_MovesFailedBackToActive covers G13's manual-clear path.
func TestClearFailure_MovesFailedBackToActive(t *testing.T) {
	content := "- [ ] Feed the dog @id(dog_feed) @state(failed) @reason(delivery failed: 0/1 accepted)\n"
	root, _ := writeTestVault(t, "household.md", content)

	o := NewOrchestrator(vault.NewWriter(nil), root, time.UTC)
	task := failedTask("household.md", content)

	require.NoError(t, o.ClearFailure(task))

	got, err := os.ReadFile(filepath.Join(root, "household.md"))
	require.NoError(t, err)
	want := "- [ ] Feed the dog @id(dog_feed) @state(active) @reason(delivery failed: 0/1 accepted)\n"
	assert.Equal(t, want, string(got))
}

// TestClearFailure_NonFailedIsNoOp covers Advance's ok=false path: an
// already-active task is left untouched by a stray clear call.
func TestClearFailure_NonFailedIsNoOp(t *testing.T) {
	content := "- [ ] Feed the dog @id(dog_feed) @state(active)\n"
	root, _ := writeTestVault(t, "household.md", content)

	o := NewOrchestrator(vault.NewWriter(nil), root, time.UTC)
	task := activeTask("household.md", content)

	require.NoError(t, o.ClearFailure(task))

	got, err := os.ReadFile(filepath.Join(root, "household.md"))
	require.NoError(t, err)
	assert.Equal(t, content, string(got))
}
