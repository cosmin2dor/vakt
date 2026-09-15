package schedule

import (
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

func TestDetectMissed_DowntimeWindow(t *testing.T) {
	// Daily 09:00 schedule, last triggered three days ago: several fires
	// elapsed while the daemon was down, but only one flag is raised.
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	lastTriggered := now.AddDate(0, 0, -3)

	tasks := map[string]index.Task{
		"feed_dog": {
			ID: "feed_dog",
			Values: map[string]directive.Value{
				"schedule":       {Type: directive.TypeCron, Cron: []string{"0", "9", "*", "*", "*"}},
				"last_triggered": {Type: directive.TypeDatetime, Time: lastTriggered},
			},
		},
	}

	missed, err := DetectMissed(tasks, now)
	require.NoError(t, err)
	require.Len(t, missed, 1)
	assert.Equal(t, "feed_dog", missed[0].Task.ID)
	assert.Contains(t, missed[0].Reason, "schedule due")
	assert.Contains(t, missed[0].Reason, "daemon was not running")
}

func TestDetectMissed_OnceInPastNoLastTriggered(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	tasks := map[string]index.Task{
		"one_off": {
			ID: "one_off",
			Values: map[string]directive.Value{
				"once": {Type: directive.TypeDatetime, Time: now.Add(-time.Hour)},
			},
		},
	}

	missed, err := DetectMissed(tasks, now)
	require.NoError(t, err)
	require.Len(t, missed, 1)
	assert.Equal(t, "one_off", missed[0].Task.ID)
	assert.Contains(t, missed[0].Reason, "@once due")
}

func TestDetectMissed_OnceAlreadyFired_NotFlagged(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	tasks := map[string]index.Task{
		"one_off": {
			ID: "one_off",
			Values: map[string]directive.Value{
				"once":           {Type: directive.TypeDatetime, Time: now.Add(-time.Hour)},
				"last_triggered": {Type: directive.TypeDatetime, Time: now.Add(-time.Hour)},
			},
		},
	}

	missed, err := DetectMissed(tasks, now)
	require.NoError(t, err)
	assert.Empty(t, missed)
}

func TestDetectMissed_ScheduleDueRightNow_NotFlagged(t *testing.T) {
	// The fire point lands exactly on now: normal, about to be picked up by
	// the live scheduler momentarily, not a miss.
	now := time.Date(2024, 6, 15, 9, 0, 0, 0, time.UTC)
	tasks := map[string]index.Task{
		"feed_dog": {
			ID: "feed_dog",
			Values: map[string]directive.Value{
				"schedule": {Type: directive.TypeCron, Cron: []string{"0", "9", "*", "*", "*"}},
			},
		},
	}

	missed, err := DetectMissed(tasks, now)
	require.NoError(t, err)
	assert.Empty(t, missed)
}

func TestDetectMissed_HealthyCurrentTask_NotFlagged(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	tasks := map[string]index.Task{
		"feed_dog": {
			ID: "feed_dog",
			Values: map[string]directive.Value{
				"schedule":       {Type: directive.TypeCron, Cron: []string{"0", "9", "*", "*", "*"}},
				"last_triggered": {Type: directive.TypeDatetime, Time: time.Date(2024, 6, 15, 9, 0, 0, 0, time.UTC)},
			},
		},
	}

	missed, err := DetectMissed(tasks, now)
	require.NoError(t, err)
	assert.Empty(t, missed)
}

func TestRecordMissedTriggers_StampsReasonOnly(t *testing.T) {
	content := "- [ ] Feed the dog @id(feed_dog) @state(active) @schedule(0 9 * * *)\n"
	root := t.TempDir()
	absPath := filepath.Join(root, "household.md")
	require.NoError(t, os.WriteFile(absPath, []byte(content), 0o644))

	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	lastTriggered := now.AddDate(0, 0, -3)
	task := index.Task{
		ID:   "feed_dog",
		Path: "household.md",
		Line: 1,
		Raw:  content,
		Values: map[string]directive.Value{
			"schedule":       {Type: directive.TypeCron, Cron: []string{"0", "9", "*", "*", "*"}},
			"last_triggered": {Type: directive.TypeDatetime, Time: lastTriggered},
		},
	}

	o := NewOrchestrator(vault.NewWriter(nil), root, time.UTC)
	missed, err := o.RecordMissedTriggers(map[string]index.Task{"feed_dog": task}, now)
	require.NoError(t, err)
	require.Len(t, missed, 1)

	got, err := os.ReadFile(absPath)
	require.NoError(t, err)
	assert.Contains(t, string(got), "@id(feed_dog)")
	assert.Contains(t, string(got), "@state(active)")
	assert.Contains(t, string(got), "@schedule(0 9 * * *)")
	assert.Contains(t, string(got), "@reason(missed trigger: schedule due")
}

func TestRecordMissedTriggers_OverwritesHumanReason(t *testing.T) {
	content := "- [ ] Feed the dog @id(feed_dog) @state(active) @schedule(0 9 * * *) @reason(vet said skip today)\n"
	root := t.TempDir()
	absPath := filepath.Join(root, "household.md")
	require.NoError(t, os.WriteFile(absPath, []byte(content), 0o644))

	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	lastTriggered := now.AddDate(0, 0, -3)
	task := index.Task{
		ID:   "feed_dog",
		Path: "household.md",
		Line: 1,
		Raw:  content,
		Values: map[string]directive.Value{
			"schedule":       {Type: directive.TypeCron, Cron: []string{"0", "9", "*", "*", "*"}},
			"last_triggered": {Type: directive.TypeDatetime, Time: lastTriggered},
			"reason":         {Type: directive.TypeString, Str: "vet said skip today"},
		},
	}

	o := NewOrchestrator(vault.NewWriter(nil), root, time.UTC)
	_, err := o.RecordMissedTriggers(map[string]index.Task{"feed_dog": task}, now)
	require.NoError(t, err)

	got, err := os.ReadFile(absPath)
	require.NoError(t, err)
	assert.NotContains(t, string(got), "vet said skip today")
	assert.Contains(t, string(got), "@reason(missed trigger: schedule due")
}
