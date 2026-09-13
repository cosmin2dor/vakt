package directive

// Span is one @name(value) match in a line of Markdown, with byte offsets
// following Go slice convention: line[Start:End] reproduces the whole
// match, line[NameStart:NameEnd] the name, line[ValueStart:ValueEnd] the
// value. Raw text only — no typed values, no validation (that's M2).
type Span struct {
	Start, End int

	Name               string
	NameStart, NameEnd int

	Value                string
	ValueStart, ValueEnd int
}

// Lex finds @name(value) spans in one line, left to right: '@', a name
// matching [a-z][a-z0-9_]*, '(', then bytes up to the first ')'. No
// nested parens, no Markdown-context-awareness — a bare "@name" or an
// unterminated "@name(" is not a match.
func Lex(line string) []Span {
	var spans []Span

	i := 0
	n := len(line)
	for i < n {
		if line[i] != '@' {
			i++
			continue
		}

		start := i
		nameStart := i + 1
		j := nameStart

		// First byte must be [a-z], so "@2fa(" or "@_id(" in prose
		// isn't lexed as a directive at all.
		if j >= n || !isLowerAlpha(line[j]) {
			i++
			continue
		}
		j++

		for j < n && isNameCont(line[j]) {
			j++
		}
		nameEnd := j

		if j >= n || line[j] != '(' {
			i = nameStart
			continue
		}
		valueStart := j + 1

		k := valueStart
		for k < n && line[k] != ')' {
			k++
		}
		if k >= n {
			// Unterminated: resume just past this '@'.
			i = nameStart
			continue
		}
		valueEnd := k
		end := k + 1

		spans = append(spans, Span{
			Start:      start,
			End:        end,
			Name:       line[nameStart:nameEnd],
			NameStart:  nameStart,
			NameEnd:    nameEnd,
			Value:      line[valueStart:valueEnd],
			ValueStart: valueStart,
			ValueEnd:   valueEnd,
		})

		i = end
	}

	return spans
}

func isLowerAlpha(b byte) bool {
	return b >= 'a' && b <= 'z'
}

func isNameCont(b byte) bool {
	return isLowerAlpha(b) || (b >= '0' && b <= '9') || b == '_'
}
