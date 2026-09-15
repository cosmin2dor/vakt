package index

import (
	"fmt"
	"strings"
)

// SyntheticVaultShape sizes a generated vault for a performance sanity
// check (issues/milestone4.md: run-performance-sanity-pass) — not a fixture
// for correctness tests, which use testdata/corpus instead.
type SyntheticVaultShape struct {
	Files        int // markdown files
	TasksPerFile int // task lines per file (approximate total = Files*TasksPerFile)
	DuplicateIDs int // deliberate cross-file @id collisions to inject (SDD.md G4)
	InvalidIDs   int // deliberate @id-pattern violations to inject (SDD.md G3)
}

// HouseholdPlusShape is "a few hundred tasks across a few dozen files" —
// the scale this task's own description names.
func HouseholdPlusShape() SyntheticVaultShape {
	return SyntheticVaultShape{Files: 30, TasksPerFile: 14, DuplicateIDs: 6, InvalidIDs: 4}
}

// GenerateSyntheticVault returns vault-relative filename -> markdown
// content for shape, mixing @schedule/@once, varied @state, @skip_count/
// @skip_until/@reason, and a handful of deliberately duplicate/invalid ids
// so the validation path is exercised too, not just the happy path.
func GenerateSyntheticVault(shape SyntheticVaultShape) map[string]string {
	files := make(map[string]string, shape.Files)
	var firstID string
	dupsLeft := shape.DuplicateIDs
	invalidLeft := shape.InvalidIDs

	for f := 0; f < shape.Files; f++ {
		var b strings.Builder
		fmt.Fprintf(&b, "# Household file %d\n\n", f)
		for t := 0; t < shape.TasksPerFile; t++ {
			id := fmt.Sprintf("task_%d_%d", f, t)
			if f == 0 && t == 0 {
				firstID = id
			}
			if dupsLeft > 0 && f > 0 && t == 0 {
				id = firstID // cross-file collision with file 0's first task
				dupsLeft--
			}

			if invalidLeft > 0 && t == shape.TasksPerFile-1 {
				b.WriteString(invalidTaskLine(f, t))
				invalidLeft--
			} else {
				b.WriteString(taskLine(id, f, t))
			}
			b.WriteByte('\n')
		}
		files[fmt.Sprintf("household-%02d.md", f)] = b.String()
	}
	return files
}

// taskLine renders one task line, rotating through directive shapes so the
// vault mixes @schedule and @once, every @state, and each optional
// suppression directive rather than repeating one shape N times.
func taskLine(id string, f, t int) string {
	hour := t % 24
	state := "active"
	if (f+t)%5 == 0 {
		state = "paused"
	} else if (f+t)%7 == 0 {
		state = "failed"
	}

	switch (f + t) % 6 {
	case 0:
		return fmt.Sprintf("- [ ] Task %s @id(%s) @schedule(0 %d * * *) @state(%s)", id, id, hour, state)
	case 1:
		return fmt.Sprintf("- [ ] Task %s @id(%s) @once(2099-%02d-%02dT%02d:00:00Z) @state(%s)", id, id, (t%12)+1, (t%27)+1, hour, state)
	case 2:
		return fmt.Sprintf("- [ ] Task %s @id(%s) @schedule(0 %d * * *) @state(%s) @skip_count(2)", id, id, hour, state)
	case 3:
		return fmt.Sprintf("- [ ] Task %s @id(%s) @schedule(0 %d * * *) @state(%s) @skip_until(2099-01-01)", id, id, hour, state)
	case 4:
		return fmt.Sprintf("- [ ] Task %s @id(%s) @schedule(0 %d * * *) @state(%s) @reason(waiting on supplies)", id, id, hour, state)
	default:
		return fmt.Sprintf("- [ ] Task %s @id(%s) @schedule(0 %d * * *) @state(%s) @target(ios_notifications)", id, id, hour, state)
	}
}

// invalidTaskLine violates @id's registry pattern (SDD.md G3) on purpose.
func invalidTaskLine(f, t int) string {
	return fmt.Sprintf("- [ ] Invalid task @id(Bad Id %d %d) @state(active)", f, t)
}
