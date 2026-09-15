package schedule

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func utc(y int, mo time.Month, d, h, mi int) time.Time {
	return time.Date(y, mo, d, h, mi, 0, 0, time.UTC)
}

// Golden conformance table for the standard 5-field cron syntax (PRD §3.2).
// A future library swap must reproduce every row exactly, or fail loudly here.
func TestNextFire_Conformance(t *testing.T) {
	cases := []struct {
		name  string
		expr  string
		after time.Time
		want  time.Time
	}{
		{"every minute", "* * * * *", utc(2024, 1, 1, 0, 0), utc(2024, 1, 1, 0, 1)},
		{"fixed minute, before fire", "30 * * * *", utc(2024, 1, 1, 0, 0), utc(2024, 1, 1, 0, 30)},
		{"fixed minute, at fire rolls to next hour", "30 * * * *", utc(2024, 1, 1, 0, 30), utc(2024, 1, 1, 1, 30)},
		{"fixed hour, before fire", "0 9 * * *", utc(2024, 1, 1, 0, 0), utc(2024, 1, 1, 9, 0)},
		{"fixed hour, at fire rolls to next day", "0 9 * * *", utc(2024, 1, 1, 9, 0), utc(2024, 1, 2, 9, 0)},
		{"day-of-month, before fire", "0 0 15 * *", utc(2024, 1, 1, 0, 0), utc(2024, 1, 15, 0, 0)},
		{"day-of-month, rolls to next month", "0 0 15 * *", utc(2024, 1, 15, 0, 0), utc(2024, 2, 15, 0, 0)},
		{"month field, before fire", "0 0 1 6 *", utc(2024, 1, 1, 0, 0), utc(2024, 6, 1, 0, 0)},
		{"month field, rolls to next year", "0 0 1 6 *", utc(2024, 6, 1, 0, 0), utc(2025, 6, 1, 0, 0)},
		{"day-of-week, same day before fire", "0 9 * * 1", utc(2024, 1, 1, 0, 0), utc(2024, 1, 1, 9, 0)},
		{"day-of-week, rolls to next Monday", "0 9 * * 1", utc(2024, 1, 1, 9, 0), utc(2024, 1, 8, 9, 0)},
		{"step minutes, before fire", "*/15 * * * *", utc(2024, 1, 1, 0, 0), utc(2024, 1, 1, 0, 15)},
		{"step minutes, rolls to next hour", "*/15 * * * *", utc(2024, 1, 1, 0, 50), utc(2024, 1, 1, 1, 0)},
		{"hour range, before fire", "0 9-17 * * *", utc(2024, 1, 1, 8, 0), utc(2024, 1, 1, 9, 0)},
		{"hour range, past last hour rolls to next day", "0 9-17 * * *", utc(2024, 1, 1, 17, 0), utc(2024, 1, 2, 9, 0)},
		{"month list, before fire", "0 0 1 3,6,9,12 *", utc(2024, 1, 1, 0, 0), utc(2024, 3, 1, 0, 0)},
		{"month list, rolls to next entry", "0 0 1 3,6,9,12 *", utc(2024, 3, 1, 0, 0), utc(2024, 6, 1, 0, 0)},
		{"multiple runs per day, first", "0 8,13,18 * * *", utc(2024, 1, 1, 0, 0), utc(2024, 1, 1, 8, 0)},
		{"multiple runs per day, second", "0 8,13,18 * * *", utc(2024, 1, 1, 8, 0), utc(2024, 1, 1, 13, 0)},
		{"multiple runs per day, third", "0 8,13,18 * * *", utc(2024, 1, 1, 13, 0), utc(2024, 1, 1, 18, 0)},
		{"multiple runs per day, wraps to next day", "0 8,13,18 * * *", utc(2024, 1, 1, 18, 0), utc(2024, 1, 2, 8, 0)},
		{"weekday range, skips weekend", "0 9 * * 1-5", utc(2024, 1, 5, 9, 0), utc(2024, 1, 8, 9, 0)},
		{"exact date, all five fields", "30 14 25 12 *", utc(2024, 1, 1, 0, 0), utc(2024, 12, 25, 14, 30)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NextFire(tc.expr, tc.after)
			require.NoError(t, err)
			require.True(t, got.Equal(tc.want), "expr %q after %v: got %v, want %v", tc.expr, tc.after, got, tc.want)
		})
	}
}
