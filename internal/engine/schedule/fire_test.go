package schedule

import (
	"testing"
	"time"

	"github.com/cosmin2dor/vakt/internal/engine/directive"
	"github.com/stretchr/testify/require"
)

func newYork(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	return loc
}

func TestPrevFire_SpringForward(t *testing.T) {
	loc := newYork(t)
	// 2024-03-10 02:00 EST -> 03:00 EDT: 02:30 never happens that day.
	after := time.Date(2024, 3, 11, 12, 0, 0, 0, loc)
	got, ok, err := PrevFire("30 2 * * *", after)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, got.Equal(time.Date(2024, 3, 11, 2, 30, 0, 0, loc)), "got %v", got)
}

func TestNextFire_SpringForward_SkipsNonexistentHour(t *testing.T) {
	loc := newYork(t)
	before := time.Date(2024, 3, 9, 3, 0, 0, 0, loc)
	got, err := NextFire("30 2 * * *", before)
	require.NoError(t, err)
	// The 02:30 job does not fire on the skipped day; the next real
	// occurrence is the following day, not a normalized 03:30 on 03-10.
	require.True(t, got.Equal(time.Date(2024, 3, 11, 2, 30, 0, 0, loc)), "got %v", got)
}

func TestNextFire_FallBack_FiresOnce(t *testing.T) {
	loc := newYork(t)
	// 2024-11-03 02:00 EDT -> 01:00 EST: the hour repeats, but 02:30 (after
	// the fold) occurs exactly once that day.
	before := time.Date(2024, 11, 2, 3, 0, 0, 0, loc)
	first, err := NextFire("30 2 * * *", before)
	require.NoError(t, err)
	require.True(t, first.Equal(time.Date(2024, 11, 3, 2, 30, 0, 0, loc)), "got %v", first)

	second, err := NextFire("30 2 * * *", first)
	require.NoError(t, err)
	require.False(t, second.Equal(first), "next fire after itself must not repeat the same instant")
	require.True(t, second.Equal(time.Date(2024, 11, 4, 2, 30, 0, 0, loc)), "got %v", second)
}

func TestPrevFire_FallBack(t *testing.T) {
	loc := newYork(t)
	after := time.Date(2024, 11, 3, 12, 0, 0, 0, loc)
	got, ok, err := PrevFire("30 2 * * *", after)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, got.Equal(time.Date(2024, 11, 3, 2, 30, 0, 0, loc)), "got %v", got)
}

func TestPrevFire_NoPriorOccurrence(t *testing.T) {
	// April has 30 days, so day 31 never occurs: this schedule never fires.
	after := utc(2024, 1, 1, 0, 0)
	_, ok, err := PrevFire("0 0 31 4 *", after)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestPrevFire_YearlySchedule(t *testing.T) {
	after := utc(2024, 12, 31, 0, 0)
	got, ok, err := PrevFire("0 0 1 6 *", after)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, got.Equal(utc(2024, 6, 1, 0, 0)), "got %v", got)
}

func TestNextFireValue_Once(t *testing.T) {
	future := utc(2024, 6, 1, 12, 0)
	v := directive.Value{Type: directive.TypeDatetime, Time: future}

	got, ok, err := NextFireValue(v, utc(2024, 1, 1, 0, 0))
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, got.Equal(future))

	// Once after's already past the timestamp, @once never fires again.
	_, ok, err = NextFireValue(v, future)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestPrevFireValue_Once(t *testing.T) {
	past := utc(2024, 1, 1, 12, 0)
	v := directive.Value{Type: directive.TypeDatetime, Time: past}

	// Not yet due: no previous fire.
	_, ok, err := PrevFireValue(v, utc(2024, 1, 1, 0, 0))
	require.NoError(t, err)
	require.False(t, ok)

	// Inclusive at the instant itself, and after.
	got, ok, err := PrevFireValue(v, past)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, got.Equal(past))

	got, ok, err = PrevFireValue(v, utc(2024, 6, 1, 0, 0))
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, got.Equal(past))
}

func TestNextFireValue_Cron(t *testing.T) {
	v := directive.Value{Type: directive.TypeCron, Cron: []string{"30", "2", "*", "*", "*"}}
	got, ok, err := NextFireValue(v, utc(2024, 1, 1, 0, 0))
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, got.Equal(utc(2024, 1, 1, 2, 30)))
}

func TestPrevFireValue_Cron_DSTFallBack(t *testing.T) {
	loc := newYork(t)
	v := directive.Value{Type: directive.TypeCron, Cron: []string{"30", "2", "*", "*", "*"}}
	got, ok, err := PrevFireValue(v, time.Date(2024, 11, 3, 12, 0, 0, 0, loc))
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, got.Equal(time.Date(2024, 11, 3, 2, 30, 0, 0, loc)))
}
