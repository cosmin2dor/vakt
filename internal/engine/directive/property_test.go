package directive

import (
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmin2dor/vakt/internal/model"
)

// corpusRoot is the golden corpus built for internal/engine/index; it's the
// most realistic source of real vault bytes (unicode, CRLF, odd formatting)
// available to this package.
const corpusRoot = "../index/testdata/corpus"

// splicePatch is the reassembly step a real writeback performs: given a
// whole file's bytes, the absolute byte offset where a line starts, and a
// Patch computed against that line's local offsets, produce the new file.
// This is the thing under test — patch.go's own byte-preservation is
// already unit-tested at the single-line level in patch_test.go; what
// isn't proven yet is that splicing a line-local patch back into a
// multi-line file doesn't disturb any other byte.
func splicePatch(data []byte, lineStart int, p Patch) []byte {
	absStart := lineStart + p.Start
	absEnd := lineStart + p.End

	out := make([]byte, 0, len(data)+len(p.Replacement))
	out = append(out, data[:absStart]...)
	out = append(out, p.Replacement...)
	out = append(out, data[absEnd:]...)
	return out
}

// lineRange is one line's [start,end) byte offsets in a file, terminator
// included when present (mirrors what patch_test.go's own CRLF case feeds
// PatchDirective: the line string carries its own line ending).
type lineRange struct{ start, end int }

// splitLinesByOffset finds line boundaries by scanning for '\n' directly in
// the byte slice and recording absolute offsets — never split-and-rejoin,
// which would silently renormalize a CRLF file's line endings.
func splitLinesByOffset(data []byte) []lineRange {
	var lines []lineRange
	start := 0
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			lines = append(lines, lineRange{start, i + 1})
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, lineRange{start, len(data)})
	}
	return lines
}

// corpusFile is one real vault file plus its raw bytes.
type corpusFile struct {
	path string
	data []byte
}

func loadCorpusFiles(t *testing.T) []corpusFile {
	t.Helper()

	var files []corpusFile
	entries, err := os.ReadDir(corpusRoot)
	require.NoError(t, err)

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		vaultDir := filepath.Join(corpusRoot, e.Name(), "vault")
		err := filepath.WalkDir(vaultDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			files = append(files, corpusFile{path: path, data: data})
			return nil
		})
		require.NoError(t, err)
	}
	return files
}

const randStringAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

func randString(rng *rand.Rand, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = randStringAlphabet[rng.Intn(len(randStringAlphabet))]
	}
	return string(b)
}

// synthesizeValue builds a Value for desc, distinct in kind from whatever
// might already be on the line — content doesn't need to differ from an
// existing span (the property under test is about byte placement, not
// value semantics), only the type has to match so Format() behaves as it
// would for a real write.
func synthesizeValue(rng *rand.Rand, desc model.DirectiveDescriptor) Value {
	switch desc.ValueType {
	case TypeEnum:
		if len(desc.Values) == 0 {
			return Value{Type: TypeEnum, Str: randString(rng, 6)}
		}
		return Value{Type: TypeEnum, Str: desc.Values[rng.Intn(len(desc.Values))]}
	case TypeInteger:
		return Value{Type: TypeInteger, Int: rng.Int63n(10_000)}
	case TypeDatetime:
		t := time.Date(2020+rng.Intn(10), time.Month(1+rng.Intn(12)), 1+rng.Intn(28),
			rng.Intn(24), rng.Intn(60), rng.Intn(60), 0, time.UTC)
		return Value{Type: TypeDatetime, Time: t}
	case TypeCron:
		return Value{Type: TypeCron, Cron: []string{
			fmt.Sprint(rng.Intn(60)), fmt.Sprint(rng.Intn(24)), "*", "*", "*",
		}}
	default: // TypeString and anything unregistered
		return Value{Type: TypeString, Str: randString(rng, 8)}
	}
}

// candidateLine is one directive-bearing line found in a corpus file, with
// its absolute file offset and the spans already on it.
type candidateLine struct {
	lineRange
	text  string
	spans []Span
}

func candidateLines(data []byte) []candidateLine {
	var out []candidateLine
	for _, lr := range splitLinesByOffset(data) {
		text := string(data[lr.start:lr.end])
		spans := Lex(text)
		if len(spans) > 0 {
			out = append(out, candidateLine{lineRange: lr, text: text, spans: spans})
		}
	}
	return out
}

// pickInsertDescriptor returns a registry directive not already present on
// the line, so an "insert" iteration genuinely adds a new directive rather
// than colliding with an existing one. ok is false if every directive in
// the registry is already on the line.
func pickInsertDescriptor(rng *rand.Rand, spans []Span) (model.DirectiveDescriptor, bool) {
	present := make(map[string]bool, len(spans))
	for _, s := range spans {
		present[s.Name] = true
	}

	var candidates []model.DirectiveDescriptor
	for _, d := range model.Directives {
		if !present[d.Name] {
			candidates = append(candidates, d)
		}
	}
	if len(candidates) == 0 {
		return model.DirectiveDescriptor{}, false
	}
	return candidates[rng.Intn(len(candidates))], true
}

// TestBytePreservationProperty proves SDD.md §4's M2 non-negotiable rule in
// the context of a real, multi-line file: patching any directive on any
// corpus line changes only that span's absolute byte range — every other
// byte, including trailing whitespace, line endings (CRLF corpus case
// included), and surrounding prose, is untouched.
//
// Seed is fixed so a failure is reproducible without hunting for the seed
// that triggered it.
func TestBytePreservationProperty(t *testing.T) {
	const seed = 20260915
	const iterationsPerFile = 40

	rng := rand.New(rand.NewSource(seed))
	t.Logf("property test seed: %d", seed)

	files := loadCorpusFiles(t)
	require.NotEmpty(t, files, "expected at least one corpus vault file")

	testedAnyFile := false
	for _, cf := range files {
		lines := candidateLines(cf.data)
		if len(lines) == 0 {
			// Some corpus cases (e.g. missing-id) may have lines with no
			// directive spans at all; nothing to patch there.
			continue
		}
		testedAnyFile = true

		t.Run(cf.path, func(t *testing.T) {
			for i := 0; i < iterationsPerFile; i++ {
				line := lines[rng.Intn(len(lines))]

				var name string
				var value Value
				doInsert := rng.Intn(2) == 0
				if doInsert {
					if desc, ok := pickInsertDescriptor(rng, line.spans); ok {
						name = desc.Name
						value = synthesizeValue(rng, desc)
					} else {
						doInsert = false
					}
				}
				if !doInsert {
					span := line.spans[rng.Intn(len(line.spans))]
					name = span.Name
					if desc, ok := Descriptor(name); ok {
						value = synthesizeValue(rng, desc)
					} else {
						value = Value{Type: TypeString, Str: randString(rng, 8)}
					}
				}

				patch, err := PatchDirective(line.text, name, value)
				require.NoError(t, err)

				result := splicePatch(cf.data, line.start, patch)

				absStart := line.start + patch.Start
				absEnd := line.start + patch.End
				wantPrefix := cf.data[:absStart]
				wantSuffix := cf.data[absEnd:]

				require.Equal(t, len(wantPrefix)+len(patch.Replacement)+len(wantSuffix), len(result),
					"result length must account for exactly the patched span")
				assert.Equal(t, wantPrefix, result[:len(wantPrefix)],
					"bytes before the patched span must be untouched, including every other line")
				assert.Equal(t, wantSuffix, result[len(wantPrefix)+len(patch.Replacement):],
					"bytes after the patched span must be untouched, including trailing line endings")
			}
		})
	}
	require.True(t, testedAnyFile, "expected at least one corpus file with a directive-bearing line")
}
