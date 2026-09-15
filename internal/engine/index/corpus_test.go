package index

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmin2dor/vakt/internal/model"
)

// corpusExpectation is one testdata/corpus/<case>/expected.json: the exact
// tasks and diagnostics Build() must produce against that case's vault/.
type corpusExpectation struct {
	Tasks       []corpusTask       `json:"tasks"`
	Diagnostics []corpusDiagnostic `json:"diagnostics"`
}

type corpusValue struct {
	Type      string `json:"type"`
	Formatted string `json:"formatted"` // Value.Format() output, not the raw span text
}

type corpusTask struct {
	ID     string                 `json:"id"`
	Path   string                 `json:"path"`
	Line   int                    `json:"line"`
	Raw    string                 `json:"raw"`
	Values map[string]corpusValue `json:"values"`
}

type corpusDiagnostic struct {
	Code     string `json:"code"`
	FilePath string `json:"file_path"`
	Line     int    `json:"line"`
	Severity string `json:"severity"`
	TaskID   string `json:"task_id"`
}

// TestGoldenCorpus walks testdata/corpus and, for every subdirectory,
// builds an Index against its vault/ and asserts Tasks()/Diagnostics()
// match its expected.json exactly. Adding a case is adding a directory —
// this function needs no per-case changes.
//
// Comparison is order-independent (assert.ElementsMatch): task iteration
// order comes from a map, and diagnostic order is an implementation
// detail of Diagnostics()'s sort, not part of the contract under test.
func TestGoldenCorpus(t *testing.T) {
	const corpusRoot = "testdata/corpus"

	entries, err := os.ReadDir(corpusRoot)
	require.NoError(t, err)

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()

		t.Run(name, func(t *testing.T) {
			caseDir := filepath.Join(corpusRoot, name)
			vaultDir := filepath.Join(caseDir, "vault")
			if info, err := os.Stat(vaultDir); err != nil || !info.IsDir() {
				t.Fatalf("corpus case %q has no vault/ directory", name)
			}

			raw, err := os.ReadFile(filepath.Join(caseDir, "expected.json"))
			if err != nil {
				t.Fatalf("corpus case %q: missing expected.json: %v", name, err)
			}
			var want corpusExpectation
			if err := json.Unmarshal(raw, &want); err != nil {
				t.Fatalf("corpus case %q: malformed expected.json: %v", name, err)
			}

			idx := New(vaultDir, time.UTC)
			require.NoError(t, idx.Build(), "corpus case %q: Build failed", name)

			assert.ElementsMatch(t, want.Tasks, actualCorpusTasks(idx.Tasks()), "case %q: tasks", name)
			assert.ElementsMatch(t, want.Diagnostics, actualCorpusDiagnostics(idx.Diagnostics()), "case %q: diagnostics", name)
		})
	}
}

func actualCorpusTasks(tasks map[string]Task) []corpusTask {
	out := make([]corpusTask, 0, len(tasks))
	for _, tk := range tasks {
		values := make(map[string]corpusValue, len(tk.Values))
		for name, v := range tk.Values {
			values[name] = corpusValue{Type: v.Type, Formatted: v.Format()}
		}
		out = append(out, corpusTask{
			ID:     tk.ID,
			Path:   tk.Path,
			Line:   tk.Line,
			Raw:    tk.Raw,
			Values: values,
		})
	}
	return out
}

func actualCorpusDiagnostics(diags []model.Diagnostic) []corpusDiagnostic {
	out := make([]corpusDiagnostic, 0, len(diags))
	for _, d := range diags {
		code := ""
		if d.Code != nil {
			code = *d.Code
		}
		out = append(out, corpusDiagnostic{
			Code:     code,
			FilePath: d.FilePath,
			Line:     d.Line,
			Severity: string(d.Severity),
			TaskID:   d.TaskId,
		})
	}
	return out
}
