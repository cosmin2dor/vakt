package index

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cosmin2dor/vakt/internal/engine/directive"
	"github.com/cosmin2dor/vakt/internal/model"
)

// idPatternRe compiles @id's registry pattern (SDD.md G3) once, from the
// generated descriptor — the same source directive/validate.go's own copy
// uses, so this isn't a third hardcoded regex.
var idPatternRe = func() *regexp.Regexp {
	desc, ok := directive.Descriptor("id")
	if !ok || desc.Pattern == nil {
		panic("directive registry has no @id descriptor/pattern")
	}
	return regexp.MustCompile(*desc.Pattern)
}()

// Task is one indexed task line: its @id, location, raw text, and every
// directive on the line as a typed value. Replaces vault.Walk's ad-hoc
// map — that task's own doc comment called it "a map, not a component".
type Task struct {
	ID   string // SDD.md G1, G3
	Path string // vault-relative, forward-slash
	Line int    // 1-based
	Raw  string

	// Values holds every directive on the line, keyed by name, including
	// @id itself. Only directives that parsed successfully appear here —
	// an unrecognized or malformed one is a diagnostic instead (see
	// Diagnostics), not an entry here.
	Values map[string]directive.Value
}

// Change is a reconciliation result: what changed in the index as a result
// of re-parsing one file, computed as a diff against the file's previous
// contribution — not a full rebuild (SDD.md §2.2's reconciliation edge).
type Change struct {
	Added   []Task
	Updated []Task
	Removed []string // task ids no longer present
}

// Index is the in-memory, thread-safe task index: every task addressable
// by @id, plus every diagnostic raised building it.
type Index struct {
	mu   sync.RWMutex
	root string
	loc  *time.Location

	tasks       map[string]Task // id -> task, global across the vault
	idsByPath   map[string][]string
	diagsByPath map[string][]model.Diagnostic
}

// New returns an empty index rooted at root, evaluating datetimes in loc
// (SDD.md G9's vault-wide timezone). A nil loc defaults to time.Local.
func New(root string, loc *time.Location) *Index {
	if loc == nil {
		loc = time.Local
	}
	return &Index{
		root:        root,
		loc:         loc,
		tasks:       make(map[string]Task),
		idsByPath:   make(map[string][]string),
		diagsByPath: make(map[string][]model.Diagnostic),
	}
}

// Build performs a full walk of root and populates the index from
// scratch. Files are visited in lexical path order, so cross-file @id
// duplicate resolution (SDD.md G4 — first by path, then by line) is well
// defined.
func (idx *Index) Build() error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.tasks = make(map[string]Task)
	idx.idsByPath = make(map[string][]string)
	idx.diagsByPath = make(map[string][]model.Diagnostic)

	var relPaths []string
	err := filepath.WalkDir(idx.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walking vault at %q: %w", path, err)
		}
		if d.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(d.Name())) != ".md" {
			return nil
		}
		rel, err := filepath.Rel(idx.root, path)
		if err != nil {
			return fmt.Errorf("computing vault-relative path for %q: %w", path, err)
		}
		relPaths = append(relPaths, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(relPaths) // explicit path order; don't rely on WalkDir's own guarantee

	for _, rel := range relPaths {
		if err := idx.indexFileLocked(rel); err != nil {
			return err
		}
	}
	return nil
}

// ReconcileFile re-reads and re-parses one file, replacing whatever the
// index previously held for it, and reports exactly what changed rather
// than rebuilding the whole index. relPath is vault-relative. A file that
// no longer exists on disk reports its previous tasks as Removed.
//
// Known limitation: if this file's @id previously lost a cross-file
// duplicate (SDD.md G4) to a different file, and that other file is later
// removed or changed, this call does not re-arbitrate and promote this
// file's occurrence — that needs a vault-wide re-walk, not a single-file
// reconciliation. Out of scope here; flagged for whoever builds the
// watcher/reactive-cycle integration on top of this.
func (idx *Index) ReconcileFile(relPath string) (Change, error) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	before := make(map[string]Task, len(idx.idsByPath[relPath]))
	for _, id := range idx.idsByPath[relPath] {
		before[id] = idx.tasks[id]
	}

	for _, id := range idx.idsByPath[relPath] {
		delete(idx.tasks, id)
	}
	delete(idx.idsByPath, relPath)
	delete(idx.diagsByPath, relPath)

	if err := idx.indexFileLocked(relPath); err != nil {
		return Change{}, err
	}

	var change Change
	after := idx.idsByPath[relPath]
	afterSet := make(map[string]bool, len(after))
	for _, id := range after {
		afterSet[id] = true
		newTask := idx.tasks[id]
		if oldTask, existed := before[id]; !existed {
			change.Added = append(change.Added, newTask)
		} else if !reflect.DeepEqual(oldTask, newTask) {
			change.Updated = append(change.Updated, newTask)
		}
	}
	for id := range before {
		if !afterSet[id] {
			change.Removed = append(change.Removed, id)
		}
	}

	return change, nil
}

// Root returns the vault-root directory Task.Path is relative to, so a
// caller can resolve a Task to an absolute filesystem path.
func (idx *Index) Root() string {
	return idx.root
}

// Tasks returns a snapshot of every currently indexed task, keyed by id.
func (idx *Index) Tasks() map[string]Task {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	out := make(map[string]Task, len(idx.tasks))
	for k, v := range idx.tasks {
		out[k] = v
	}
	return out
}

// Task returns one task by id.
func (idx *Index) Task(id string) (Task, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	t, ok := idx.tasks[id]
	return t, ok
}

// Diagnostics returns every diagnostic currently held, ordered by file
// path then line, for determinism.
func (idx *Index) Diagnostics() []model.Diagnostic {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	paths := make([]string, 0, len(idx.diagsByPath))
	for p := range idx.diagsByPath {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	var all []model.Diagnostic
	for _, p := range paths {
		diags := append([]model.Diagnostic(nil), idx.diagsByPath[p]...)
		sort.SliceStable(diags, func(i, j int) bool { return diags[i].Line < diags[j].Line })
		all = append(all, diags...)
	}
	return all
}

// indexFileLocked reads and parses one already-vault-relative file, adding
// its valid, not-already-claimed tasks to idx.tasks and recording its
// diagnostics. Caller holds idx.mu.
func (idx *Index) indexFileLocked(relPath string) error {
	absPath := filepath.Join(idx.root, filepath.FromSlash(relPath))
	content, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // file is gone; caller already cleared its claims
		}
		return fmt.Errorf("reading vault file %q: %w", relPath, err)
	}

	text := string(content)
	diags := directive.ValidateFile(relPath, text, idx.loc)

	var ids []string
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lineNum := i + 1
		spans := directive.Lex(line)
		idSpan, hasID := firstIDSpan(spans)
		if !hasID {
			continue // G5: no @id, not a task line
		}
		if !idPatternRe.MatchString(idSpan.Value) {
			continue // G3: invalid; ValidateFile already diagnosed it
		}
		if containsString(ids, idSpan.Value) {
			continue // within-file duplicate; ValidateFile already diagnosed it
		}
		if existing, claimed := idx.tasks[idSpan.Value]; claimed && existing.Path != relPath {
			// Claimed by a different, already-indexed file — a cross-file
			// duplicate (G4). ValidateFile only sees one file at a time
			// and can't catch this; record it here instead.
			diags = append(diags, crossFileDuplicateDiag(relPath, lineNum, idSpan.Value))
			continue
		}

		values := make(map[string]directive.Value, len(spans))
		for _, s := range spans {
			v, perr := directive.ParseSpan(s, idx.loc)
			if perr != nil {
				continue // unrecognized/malformed; already diagnosed by ValidateFile
			}
			values[s.Name] = v
		}

		idx.tasks[idSpan.Value] = Task{
			ID:     idSpan.Value,
			Path:   relPath,
			Line:   lineNum,
			Raw:    line,
			Values: values,
		}
		ids = append(ids, idSpan.Value)
	}

	idx.diagsByPath[relPath] = diags
	idx.idsByPath[relPath] = ids
	return nil
}

func firstIDSpan(spans []directive.Span) (directive.Span, bool) {
	for _, s := range spans {
		if s.Name == "id" {
			return s, true
		}
	}
	return directive.Span{}, false
}

func containsString(vs []string, v string) bool {
	for _, c := range vs {
		if c == v {
			return true
		}
	}
	return false
}

func crossFileDuplicateDiag(path string, line int, id string) model.Diagnostic {
	code := "duplicate_id"
	return model.Diagnostic{
		Code:     &code,
		FilePath: path,
		Line:     line,
		Message:  "@id is already claimed by another file earlier in vault order",
		Severity: model.DiagnosticSeverityError,
		TaskId:   id,
	}
}
