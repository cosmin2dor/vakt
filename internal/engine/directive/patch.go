package directive

import "strings"

// Patch is a byte-range replacement for one directive span on one line.
// Start/End are offsets into the ORIGINAL line; applying never re-derives
// the line from a parsed model (SDD.md §4 M2) — it only splices bytes.
type Patch struct {
	Start, End  int
	Replacement string
}

// Apply splices the patch into line. Start==End is a pure insertion.
func (p Patch) Apply(line string) string {
	return line[:p.Start] + p.Replacement + line[p.End:]
}

// SplicePatch reassembles a whole file's bytes with p applied at lineStart,
// the absolute offset where the patched line begins. p's Start/End are
// line-local; only that span changes, byte-for-byte elsewhere.
func SplicePatch(data []byte, lineStart int, p Patch) []byte {
	absStart := lineStart + p.Start
	absEnd := lineStart + p.End

	out := make([]byte, 0, len(data)+len(p.Replacement))
	out = append(out, data[:absStart]...)
	out = append(out, p.Replacement...)
	out = append(out, data[absEnd:]...)
	return out
}

// PatchDirective computes a Patch that writes value under name on line.
//
// If name already has a span (first occurrence wins, mirroring G4's
// "first wins" spirit for malformed duplicates), the patch replaces just
// [ValueStart:ValueEnd]. Otherwise it inserts "@name(value)" at the end
// of the line's content, before any trailing whitespace or line ending.
// Insertion-only, appended past every existing byte, can never corrupt an
// existing span — it doesn't need to understand directive ordering, and
// it keeps writeback consistent with "never re-serialize from the parsed
// model" (only this one span's bytes ever change).
func PatchDirective(line string, name string, value Value) (Patch, error) {
	formatted := value.Format()

	for _, span := range Lex(line) {
		if span.Name == name {
			return Patch{Start: span.ValueStart, End: span.ValueEnd, Replacement: formatted}, nil
		}
	}

	pos := insertionPoint(line)
	replacement := "@" + name + "(" + formatted + ")"
	if pos > 0 && !isTrailingWhitespace(line[pos-1]) {
		replacement = " " + replacement
	}
	return Patch{Start: pos, End: pos, Replacement: replacement}, nil
}

// insertionPoint finds where a line's real content ends, before trailing
// whitespace or a line ending (including CRLF, so an inserted directive
// never lands inside "\r\n").
func insertionPoint(line string) int {
	end := len(line)
	if strings.HasSuffix(line, "\r\n") {
		end -= 2
	} else if strings.HasSuffix(line, "\n") {
		end -= 1
	}
	for end > 0 && isTrailingWhitespace(line[end-1]) {
		end--
	}
	return end
}

func isTrailingWhitespace(b byte) bool {
	return b == ' ' || b == '\t'
}
