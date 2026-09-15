package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cosmin2dor/vakt/internal/engine"
	"github.com/cosmin2dor/vakt/internal/engine/directive"
	"github.com/cosmin2dor/vakt/internal/engine/index"
	"github.com/cosmin2dor/vakt/internal/engine/schedule"
	"github.com/cosmin2dor/vakt/internal/engine/vault"
	"github.com/cosmin2dor/vakt/internal/model"
)

// slugCollapseRe turns any run of characters outside @id's charset into a
// single separator.
var slugCollapseRe = regexp.MustCompile(`[^a-z0-9_-]+`)

// CreateTaskHandler appends a new task line to file_path, generating its id
// from the title slug (SDD.md G5). It does not wait for the watcher to
// reconcile the write; the response is built from the line just written.
func CreateTaskHandler(eng *engine.Engine, writer *vault.Writer, vaultRoot string, loc *time.Location) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body model.TaskCreate
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}

		title := strings.TrimSpace(body.Title)
		if title == "" || strings.TrimSpace(body.FilePath) == "" {
			writeError(w, http.StatusBadRequest, "invalid_task", "title and file_path are required")
			return
		}
		if (body.Schedule == nil) == (body.Once == nil) {
			writeError(w, http.StatusBadRequest, "invalid_task", "exactly one of schedule or once is required")
			return
		}

		var scheduleValue directive.Value
		if body.Schedule != nil {
			fields := strings.Fields(*body.Schedule)
			if len(fields) != 5 {
				writeError(w, http.StatusBadRequest, "invalid_schedule", "schedule must be a 5-field cron expression")
				return
			}
			if _, err := schedule.NextFire(*body.Schedule, time.Now().In(loc)); err != nil {
				writeError(w, http.StatusBadRequest, "invalid_schedule", "schedule is not a valid cron expression: "+err.Error())
				return
			}
			scheduleValue = directive.Value{Type: directive.TypeCron, Cron: fields}
		} else {
			scheduleValue = directive.Value{Type: directive.TypeDatetime, Time: body.Once.In(loc)}
		}

		absPath, err := resolveVaultPath(vaultRoot, body.FilePath)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_path", err.Error())
			return
		}

		existing, err := os.ReadFile(absPath)
		if err != nil {
			if !os.IsNotExist(err) {
				writeError(w, http.StatusInternalServerError, "internal_error", "reading target file: "+err.Error())
				return
			}
			if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", "creating target directory: "+err.Error())
				return
			}
			existing = nil
		}

		// The index reconciles asynchronously (watcher), so a same-file
		// collision from a request made moments ago may not be visible
		// there yet — check the file's own current bytes too.
		id := generateTaskID(eng, existing, title)

		var target string
		if body.Target != nil {
			target = strings.TrimSpace(*body.Target)
		}

		line := buildTaskLine(title, id, scheduleValue, target)

		// A file missing its trailing newline would otherwise merge into
		// the new line's start; insert one before appending.
		if len(existing) > 0 && existing[len(existing)-1] != '\n' {
			existing = append(existing, '\n')
		}
		content := append(existing, []byte(line)...)
		if _, err := writer.Write(absPath, content); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "writing task: "+err.Error())
			return
		}

		values := map[string]directive.Value{
			"id":    {Type: directive.TypeString, Str: id},
			"state": {Type: directive.TypeEnum, Str: "active"},
		}
		if body.Schedule != nil {
			values["schedule"] = scheduleValue
		} else {
			values["once"] = scheduleValue
		}
		if target != "" {
			values["target"] = directive.Value{Type: directive.TypeString, Str: target}
		}

		t := index.Task{
			ID:     id,
			Path:   body.FilePath,
			Raw:    strings.TrimSuffix(line, "\n"),
			Values: values,
		}

		writeJSON(w, http.StatusCreated, buildTaskDTO(t, time.Now().In(loc)))
	}
}

// resolveVaultPath resolves rel against vaultRoot and rejects anything that
// would escape it.
func resolveVaultPath(vaultRoot, rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", errFileOutsideVault
	}
	joined := filepath.Join(vaultRoot, rel)
	cleanRoot := filepath.Clean(vaultRoot)
	if joined != cleanRoot && !strings.HasPrefix(joined, cleanRoot+string(filepath.Separator)) {
		return "", errFileOutsideVault
	}
	return joined, nil
}

var errFileOutsideVault = errors.New("file_path escapes the vault root")

// generateTaskID slugifies title into an @id and appends a numeric suffix
// on collision with an id already in the index or already present in the
// target file's own current bytes (SDD.md G5).
func generateTaskID(eng *engine.Engine, fileContent []byte, title string) string {
	taken := idsInContent(fileContent)
	idExists := func(id string) bool {
		if taken[id] {
			return true
		}
		_, ok := eng.Index().Task(id)
		return ok
	}

	base := slugify(title)
	if !idExists(base) {
		return base
	}
	for n := 2; ; n++ {
		candidate := suffixed(base, n)
		if !idExists(candidate) {
			return candidate
		}
	}
}

// idsInContent collects every @id value already present in raw file bytes.
func idsInContent(content []byte) map[string]bool {
	ids := make(map[string]bool)
	for _, line := range strings.Split(string(content), "\n") {
		for _, span := range directive.Lex(line) {
			if span.Name == "id" {
				ids[span.Value] = true
			}
		}
	}
	return ids
}

// suffixed appends "-N" to base, trimming base so the result still fits
// @id's 64-char limit.
func suffixed(base string, n int) string {
	suffix := "-" + strconv.Itoa(n)
	if len(base)+len(suffix) > 64 {
		base = base[:64-len(suffix)]
		base = strings.Trim(base, "-_")
	}
	return base + suffix
}

// slugify lowercases title and collapses everything outside @id's charset
// ([a-z0-9_-]) into a single "-", trimming to 64 chars and to a non-empty
// result.
func slugify(title string) string {
	s := strings.ToLower(title)
	s = slugCollapseRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-_")
	if len(s) > 64 {
		s = strings.Trim(s[:64], "-_")
	}
	if s == "" {
		s = "task"
	}
	return s
}

// buildTaskLine renders a brand-new task line: not a directive patch (no
// existing span), just a plain string append (CLAUDE.md: writes are patches
// to an existing span; a new line has none to patch).
func buildTaskLine(title, id string, sv directive.Value, target string) string {
	var b strings.Builder
	b.WriteString("- [ ] ")
	b.WriteString(title)
	b.WriteString(" @id(")
	b.WriteString(id)
	b.WriteString(")")
	if sv.Type == directive.TypeCron {
		b.WriteString(" @schedule(")
	} else {
		b.WriteString(" @once(")
	}
	b.WriteString(sv.Format())
	b.WriteString(")")
	b.WriteString(" @state(active)")
	if target != "" {
		b.WriteString(" @target(")
		b.WriteString(target)
		b.WriteString(")")
	}
	b.WriteString("\n")
	return b.String()
}
