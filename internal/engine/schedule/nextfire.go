package schedule

import (
	"time"

	"github.com/robfig/cron/v3"
)

// parser accepts the standard 5-field cron form (minute hour dom month dow), per PRD §3.2.
var parser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// NextFire returns the next time expr fires strictly after t, in t's location (SDD G9).
func NextFire(expr string, after time.Time) (time.Time, error) {
	sched, err := parser.Parse(expr)
	if err != nil {
		return time.Time{}, err
	}
	return sched.Next(after), nil
}
