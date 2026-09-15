package directive

import (
	"errors"
	"strings"
)

// ErrNoUncheckedCheckbox is returned when a line has no "- [ ]" to check —
// already checked, or no checkbox at all. Callers treat "already checked"
// as a no-op, not a failure; this function just reports what it found.
var ErrNoUncheckedCheckbox = errors.New("directive: no unchecked checkbox on line")

// PatchCheckbox finds the first unchecked Markdown checkbox ("- [ ]") on
// line and returns a Patch flipping its single space byte to "x". Zero-width
// outside that one byte — everything else on the line is untouched.
func PatchCheckbox(line string) (Patch, error) {
	const marker = "- [ ]"
	idx := strings.Index(line, marker)
	if idx < 0 {
		return Patch{}, ErrNoUncheckedCheckbox
	}
	spacePos := idx + len("- [")
	return Patch{Start: spacePos, End: spacePos + 1, Replacement: "x"}, nil
}
