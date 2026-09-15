package schedule

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmin2dor/vakt/internal/engine/directive"
	"github.com/cosmin2dor/vakt/internal/engine/index"
	"github.com/cosmin2dor/vakt/internal/engine/vault"
)

// taskFromLine builds a Task's Values the way the real index would, by
// lexing and typing one line — a small stand-in so these tests don't need
// a full Index just to exercise Fulfill.
func taskFromLine(t *testing.T, id, path string, lineNum int, line string) index.Task {
	t.Helper()
	values := make(map[string]directive.Value)
	for _, span := range directive.Lex(line) {
		v, err := directive.ParseSpan(span, time.UTC)
		require.NoError(t, err)
		values[span.Name] = v
	}
	return index.Task{ID: id, Path: path, Line: lineNum, Raw: line, Values: values}
}

func TestFulfill_ScheduleBeforeFire(t *testing.T) {
	line := "- [ ] Water plants @id(water_plants) @state(active) @schedule(0 8 * * *)\n"
	root, absPath := writeTestVault(t, "household.md", line)

	o := NewOrchestrator(vault.NewWriter(nil), root, time.UTC)
	task := taskFromLine(t, "water_plants", "household.md", 1, line)

	now := time.Date(2026, 9, 15, 6, 0, 0, 0, time.UTC) // well before the 08:00 fire
	require.NoError(t, o.Fulfill(task, now))

	got, err := os.ReadFile(absPath)
	require.NoError(t, err)
	want := "- [ ] Water plants @id(water_plants) @state(active) @schedule(0 8 * * *) @last_completed(2026-09-15T06:00:00Z)\n"
	assert.Equal(t, want, string(got))
}

func TestFulfill_ScheduleAfterFire_TriggeredReturnsActive(t *testing.T) {
	line := "- [ ] Water plants @id(water_plants) @state(triggered) @schedule(0 8 * * *)\n"
	root, absPath := writeTestVault(t, "household.md", line)

	o := NewOrchestrator(vault.NewWriter(nil), root, time.UTC)
	task := taskFromLine(t, "water_plants", "household.md", 1, line)

	now := time.Date(2026, 9, 15, 8, 5, 0, 0, time.UTC) // just after the 08:00 fire
	require.NoError(t, o.Fulfill(task, now))

	got, err := os.ReadFile(absPath)
	require.NoError(t, err)
	want := "- [ ] Water plants @id(water_plants) @state(active) @schedule(0 8 * * *) @last_completed(2026-09-15T08:05:00Z)\n"
	assert.Equal(t, want, string(got))
}

func TestFulfill_ScheduleOutOfBand(t *testing.T) {
	// Fulfilled days away from any specific fire instant; already active.
	line := "- [ ] Water plants @id(water_plants) @state(active) @schedule(0 8 * * *)\n"
	root, absPath := writeTestVault(t, "household.md", line)

	o := NewOrchestrator(vault.NewWriter(nil), root, time.UTC)
	task := taskFromLine(t, "water_plants", "household.md", 1, line)

	now := time.Date(2026, 9, 20, 14, 30, 0, 0, time.UTC)
	require.NoError(t, o.Fulfill(task, now))

	got, err := os.ReadFile(absPath)
	require.NoError(t, err)
	want := "- [ ] Water plants @id(water_plants) @state(active) @schedule(0 8 * * *) @last_completed(2026-09-20T14:30:00Z)\n"
	assert.Equal(t, want, string(got))
}

func TestFulfill_Once_ChecksBoxAndCompletes(t *testing.T) {
	content := "- [ ] Buy birthday gift @id(gift) @state(active) @once(2026-09-20T10:00:00Z)\n" +
		"- [ ] Water plants @id(water_plants) @state(active) @schedule(0 8 * * *)\n"
	root, absPath := writeTestVault(t, "household.md", content)

	o := NewOrchestrator(vault.NewWriter(nil), root, time.UTC)
	firstLine := "- [ ] Buy birthday gift @id(gift) @state(active) @once(2026-09-20T10:00:00Z)\n"
	task := taskFromLine(t, "gift", "household.md", 1, firstLine)

	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	require.NoError(t, o.Fulfill(task, now))

	got, err := os.ReadFile(absPath)
	require.NoError(t, err)
	want := "- [x] Buy birthday gift @id(gift) @state(completed) @once(2026-09-20T10:00:00Z)\n" +
		"- [ ] Water plants @id(water_plants) @state(active) @schedule(0 8 * * *)\n"
	assert.Equal(t, want, string(got))
}

func TestFulfill_Once_AlreadyCompleted_NoOp(t *testing.T) {
	line := "- [x] Buy birthday gift @id(gift) @state(completed) @once(2026-09-20T10:00:00Z)\n"
	root, absPath := writeTestVault(t, "household.md", line)

	o := NewOrchestrator(vault.NewWriter(nil), root, time.UTC)
	task := taskFromLine(t, "gift", "household.md", 1, line)

	now := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	require.NoError(t, o.Fulfill(task, now))

	got, err := os.ReadFile(absPath)
	require.NoError(t, err)
	assert.Equal(t, line, string(got), "already-completed @once fulfillment is a byte-identical no-op")
}

func TestFulfill_NeitherOnceNorSchedule_Errors(t *testing.T) {
	line := "- [ ] Untracked task @id(untracked) @state(active)\n"
	root, _ := writeTestVault(t, "household.md", line)

	o := NewOrchestrator(vault.NewWriter(nil), root, time.UTC)
	task := taskFromLine(t, "untracked", "household.md", 1, line)

	err := o.Fulfill(task, time.Now())
	require.Error(t, err)
}
