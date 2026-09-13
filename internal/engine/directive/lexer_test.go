package directive

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLex(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []Span
	}{
		{
			name: "multiple directives on one line",
			line: "- [ ] Feed the dog @id(dog_feed) @schedule(0 8 * * *) @target(ios_notifications) @state(active)",
			want: []Span{
				{Start: 19, End: 32, Name: "id", NameStart: 20, NameEnd: 22, Value: "dog_feed", ValueStart: 23, ValueEnd: 31},
				{Start: 33, End: 53, Name: "schedule", NameStart: 34, NameEnd: 42, Value: "0 8 * * *", ValueStart: 43, ValueEnd: 52},
				{Start: 54, End: 80, Name: "target", NameStart: 55, NameEnd: 61, Value: "ios_notifications", ValueStart: 62, ValueEnd: 79},
				{Start: 81, End: 95, Name: "state", NameStart: 82, NameEnd: 87, Value: "active", ValueStart: 88, ValueEnd: 94},
			},
		},
		{
			name: "no directives at all",
			line: "- [ ] Feed the dog",
			want: nil,
		},
		{
			name: "at-name with no parens is inert text",
			line: "- [ ] Feed the dog @id",
			want: nil,
		},
		{
			name: "unterminated at-name-paren with no closing paren",
			line: "- [ ] Feed the dog @id(dog_feed",
			want: nil,
		},
		{
			name: "at signs in prose, a store address, and an email are not directives",
			line: "buy stuff @ the store, email user@example.com",
			want: nil,
		},
		{
			name: "value containing an unescaped close-paren lexes short, up to the first one",
			line: `@payload({"a": "b)"})`,
			want: []Span{
				{Start: 0, End: 18, Name: "payload", NameStart: 1, NameEnd: 8, Value: `{"a": "b`, ValueStart: 9, ValueEnd: 17},
			},
		},
		{
			name: "two directives with no separating whitespace",
			line: "@id(a)@state(active)",
			want: []Span{
				{Start: 0, End: 6, Name: "id", NameStart: 1, NameEnd: 3, Value: "a", ValueStart: 4, ValueEnd: 5},
				{Start: 6, End: 20, Name: "state", NameStart: 7, NameEnd: 12, Value: "active", ValueStart: 13, ValueEnd: 19},
			},
		},
		{
			name: "uppercase names do not match the lowercase-only name grammar",
			line: "@ID(x) @Id(y)",
			want: nil,
		},
		{
			name: "names may not start with a digit or underscore",
			line: "@2fa(on) @_id(x)",
			want: nil,
		},
		{
			name: "empty value between the parens is still a valid, empty-value span",
			line: "@id()",
			want: []Span{
				{Start: 0, End: 5, Name: "id", NameStart: 1, NameEnd: 3, Value: "", ValueStart: 4, ValueEnd: 4},
			},
		},
		{
			name: "empty line",
			line: "",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Lex(tt.line)
			require.Equal(t, tt.want, got, "Lex(%q)", tt.line)

			// Offsets must reproduce the original text exactly.
			for i, sp := range got {
				assert.Equal(t, "@"+sp.Name+"("+sp.Value+")", tt.line[sp.Start:sp.End], "span[%d]: line[Start:End]", i)
				assert.Equal(t, sp.Name, tt.line[sp.NameStart:sp.NameEnd], "span[%d]: line[NameStart:NameEnd]", i)
				assert.Equal(t, sp.Value, tt.line[sp.ValueStart:sp.ValueEnd], "span[%d]: line[ValueStart:ValueEnd]", i)
			}
		})
	}
}
