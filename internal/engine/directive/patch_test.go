package directive

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertUntouchedOutsideRange checks that every byte outside [start:end) of
// original is identical in result — the update exit bar, not just "the new
// value shows up somewhere."
func assertUntouchedOutsideRange(t *testing.T, original, result string, start, end int) {
	t.Helper()
	require.GreaterOrEqual(t, len(result), start)
	assert.Equal(t, original[:start], result[:start], "bytes before patch changed")
	suffixLen := len(original) - end
	require.GreaterOrEqual(t, len(result), suffixLen)
	assert.Equal(t, original[end:], result[len(result)-suffixLen:], "bytes after patch changed")
}

func TestPatchDirective_UpdateExistingSameLength(t *testing.T) {
	line := "- [ ] feed_dog @skip_count(3)"
	p, err := PatchDirective(line, "skip_count", Value{Type: TypeInteger, Int: 5})
	require.NoError(t, err)
	result := p.Apply(line)

	assert.Equal(t, "- [ ] feed_dog @skip_count(5)", result)
	assertUntouchedOutsideRange(t, line, result, p.Start, p.End)
}

func TestPatchDirective_UpdateVariableLength(t *testing.T) {
	line := "- [ ] feed_dog @skip_count(3) @every(0 8 * * *)"
	p, err := PatchDirective(line, "skip_count", Value{Type: TypeInteger, Int: 12})
	require.NoError(t, err)
	result := p.Apply(line)

	assert.Equal(t, "- [ ] feed_dog @skip_count(12) @every(0 8 * * *)", result)
	assertUntouchedOutsideRange(t, line, result, p.Start, p.End)
	// The untouched suffix, including the sibling directive, must survive intact.
	assert.Contains(t, result, "@every(0 8 * * *)")
}

func TestPatchDirective_UpdateFirstOfDuplicates(t *testing.T) {
	line := "- [ ] task @id(a) @id(b)"
	p, err := PatchDirective(line, "id", Value{Type: TypeString, Str: "c"})
	require.NoError(t, err)
	result := p.Apply(line)

	assert.Equal(t, "- [ ] task @id(c) @id(b)", result)
	assertUntouchedOutsideRange(t, line, result, p.Start, p.End)
}

func TestPatchDirective_InsertOnLineWithNoDirectives(t *testing.T) {
	line := "- [ ] feed the dog"
	p, err := PatchDirective(line, "last_triggered", Value{Type: TypeDatetime, Time: mustParseTime(t, "2026-01-02T08:30:00Z")})
	require.NoError(t, err)
	result := p.Apply(line)

	require.Equal(t, p.Start, p.End, "insertion must be zero-width")
	assert.Equal(t, line, result[:len(line)], "original line must be an unmodified prefix")
	assert.Equal(t, "- [ ] feed the dog @last_triggered(2026-01-02T08:30:00Z)", result)
}

func TestPatchDirective_InsertOnLineWithOtherDirectives(t *testing.T) {
	line := "- [ ] feed_dog @every(0 8 * * *)"
	p, err := PatchDirective(line, "last_triggered", Value{Type: TypeDatetime, Time: mustParseTime(t, "2026-01-02T08:30:00Z")})
	require.NoError(t, err)
	result := p.Apply(line)

	require.Equal(t, p.Start, p.End)
	assert.Equal(t, line, result[:len(line)], "original line, including existing directive, must be an unmodified prefix")
	assert.Equal(t, line+" @last_triggered(2026-01-02T08:30:00Z)", result)

	// The pre-existing span must lex to exactly what it did before.
	before := Lex(line)
	after := Lex(result)
	require.Len(t, before, 1)
	require.GreaterOrEqual(t, len(after), 1)
	assert.Equal(t, before[0].Name, after[0].Name)
	assert.Equal(t, before[0].Value, after[0].Value)
}

func TestPatchDirective_InsertOnLineWithTrailingWhitespace(t *testing.T) {
	line := "- [ ] feed the dog   "
	p, err := PatchDirective(line, "skip_count", Value{Type: TypeInteger, Int: 1})
	require.NoError(t, err)
	result := p.Apply(line)

	assert.Equal(t, "- [ ] feed the dog @skip_count(1)   ", result)
	// No leading space added since content already ends without one needed at insertion point.
	assert.Equal(t, "- [ ] feed the dog", line[:p.Start])
}

func TestPatchDirective_InsertOnLineEndingInCRLF(t *testing.T) {
	line := "- [ ] feed the dog\r\n"
	p, err := PatchDirective(line, "skip_count", Value{Type: TypeInteger, Int: 1})
	require.NoError(t, err)
	result := p.Apply(line)

	assert.Equal(t, "- [ ] feed the dog @skip_count(1)\r\n", result)
	assert.Equal(t, len("- [ ] feed the dog"), p.Start, "insertion point must land before \\r\\n, not inside it")
}

func TestPatchDirective_InsertOnEmptyLine(t *testing.T) {
	line := ""
	p, err := PatchDirective(line, "id", Value{Type: TypeString, Str: "x"})
	require.NoError(t, err)
	result := p.Apply(line)

	assert.Equal(t, "@id(x)", result, "empty line gets no leading space")
	assert.Equal(t, 0, p.Start)
	assert.Equal(t, 0, p.End)
}

func TestPatchDirective_InsertOnWhitespaceOnlyLine(t *testing.T) {
	line := "   "
	p, err := PatchDirective(line, "id", Value{Type: TypeString, Str: "x"})
	require.NoError(t, err)
	result := p.Apply(line)

	assert.Equal(t, "@id(x)   ", result, "no leading space when line already ends in whitespace")
	assert.Equal(t, 0, p.Start)
}

func mustParseTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := ParseValue(TypeDatetime, s, time.UTC)
	require.NoError(t, err)
	return v.Time
}
