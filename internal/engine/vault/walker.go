package vault

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cosmin2dor/vakt/internal/engine/directive"
)

// idPattern is SDD.md G3.
var idPattern = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)

// Task is the walker's minimal internal notion of a task line — not the
// API's Task DTO.
type Task struct {
	ID   string // SDD.md G1, matches idPattern (G3)
	Path string // vault-relative, forward-slash
	Line int    // 1-based
	Raw  string
}

// ProblemKind classifies a Problem raised while walking the vault.
type ProblemKind int

const (
	// InvalidID: @id present but fails idPattern (SDD.md G3).
	InvalidID ProblemKind = iota
	// DuplicateID: valid @id already claimed by an earlier occurrence
	// (SDD.md G4 — first by path, then by line, wins).
	DuplicateID
)

func (k ProblemKind) String() string {
	switch k {
	case InvalidID:
		return "invalid_id"
	case DuplicateID:
		return "duplicate_id"
	default:
		return "unknown"
	}
}

// Problem is the walker's lightweight, local notion of "this @id line had
// an issue" — not a general diagnostics model.
type Problem struct {
	Kind ProblemKind
	Path string
	Line int
	ID   string
	Raw  string
}

// Walk recursively discovers .md files under root and lexes each line for
// an @id. A line without one isn't a task line (SDD.md G5). Files are
// visited in lexical path order, lines top to bottom, so "first" for a
// duplicate @id (G4) is well defined. The map is keyed by @id, globally
// (G1). error is for I/O failures only; per-line issues are Problems.
func Walk(root string) (map[string]Task, []Problem, error) {
	tasks := make(map[string]Task)
	var problems []Problem

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walking vault at %q: %w", path, err)
		}
		if d.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(d.Name())) != ".md" {
			return nil
		}

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("computing vault-relative path for %q: %w", path, err)
		}
		relPath = filepath.ToSlash(relPath)

		fileProblems, err := walkFile(path, relPath, tasks)
		if err != nil {
			return err
		}
		problems = append(problems, fileProblems...)

		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	return tasks, problems, nil
}

// walkFile lexes one file line by line, claiming ids into tasks directly
// (so later files see earlier claims).
func walkFile(absPath, relPath string, tasks map[string]Task) ([]Problem, error) {
	f, err := os.Open(absPath)
	if err != nil {
		return nil, fmt.Errorf("opening vault file %q: %w", relPath, err)
	}
	defer func() { _ = f.Close() }()

	var problems []Problem

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		id, ok := firstID(line)
		if !ok {
			continue // no @id: not a task line (G5)
		}

		if !idPattern.MatchString(id) {
			problems = append(problems, Problem{
				Kind: InvalidID,
				Path: relPath,
				Line: lineNum,
				ID:   id,
				Raw:  line,
			})
			continue
		}

		if _, exists := tasks[id]; exists {
			problems = append(problems, Problem{
				Kind: DuplicateID,
				Path: relPath,
				Line: lineNum,
				ID:   id,
				Raw:  line,
			})
			continue
		}

		tasks[id] = Task{
			ID:   id,
			Path: relPath,
			Line: lineNum,
			Raw:  line,
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading vault file %q: %w", relPath, err)
	}

	return problems, nil
}

// firstID returns the value of the first @id(...) span on line, if any.
func firstID(line string) (string, bool) {
	for _, span := range directive.Lex(line) {
		if span.Name == "id" {
			return span.Value, true
		}
	}
	return "", false
}
