package directive

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPatchCheckbox_UncheckedToChecked(t *testing.T) {
	line := "- [ ] Feed the dog @id(dog_feed) @once(2026-09-15T08:00:00Z)"
	p, err := PatchCheckbox(line)
	require.NoError(t, err)
	result := p.Apply(line)

	assert.Equal(t, "- [x] Feed the dog @id(dog_feed) @once(2026-09-15T08:00:00Z)", result)
	assertUntouchedOutsideRange(t, line, result, p.Start, p.End)
	assert.Equal(t, p.End-p.Start, 1, "patch touches exactly one byte")
}

func TestPatchCheckbox_AlreadyChecked(t *testing.T) {
	line := "- [x] Feed the dog @id(dog_feed)"
	_, err := PatchCheckbox(line)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoUncheckedCheckbox))
}

func TestPatchCheckbox_NoCheckbox(t *testing.T) {
	line := "Feed the dog @id(dog_feed)"
	_, err := PatchCheckbox(line)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoUncheckedCheckbox))
}

func TestPatchCheckbox_CRLFUntouched(t *testing.T) {
	line := "- [ ] Feed the dog @id(dog_feed)\r\n"
	p, err := PatchCheckbox(line)
	require.NoError(t, err)
	result := p.Apply(line)

	assert.Equal(t, "- [x] Feed the dog @id(dog_feed)\r\n", result)
	assertUntouchedOutsideRange(t, line, result, p.Start, p.End)
}
