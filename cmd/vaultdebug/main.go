// Command vaultdebug builds an in-memory index of a vault and prints its
// tasks and diagnostics to stdout. It exists purely as observability for
// M2's demo (SDD.md §4) — not a user-facing API surface; that's M4's job.
package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/cosmin2dor/vakt/internal/engine/directive"
	"github.com/cosmin2dor/vakt/internal/engine/index"
	"github.com/cosmin2dor/vakt/internal/model"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 || args[0] == "-h" || args[0] == "--help" {
		printfln(stderr, "usage: vaultdebug <vault-path>")
		printfln(stderr, "env: VAKT_TZ sets the IANA timezone tasks are evaluated in (default: local)")
		return 2
	}
	root := args[0]

	loc, err := resolveLoc(os.Getenv("VAKT_TZ"))
	if err != nil {
		printfln(stderr, "vaultdebug: %v", err)
		return 1
	}

	info, err := os.Stat(root)
	if err != nil {
		printfln(stderr, "vaultdebug: vault path %q: %v", root, err)
		return 1
	}
	if !info.IsDir() {
		printfln(stderr, "vaultdebug: vault path %q is not a directory", root)
		return 1
	}

	idx := index.New(root, loc)
	if err := idx.Build(); err != nil {
		printfln(stderr, "vaultdebug: building index: %v", err)
		return 1
	}

	printTasks(stdout, idx.Tasks())
	printDiagnostics(stdout, idx.Diagnostics())
	return 0
}

// resolveLoc loads an IANA zone name, defaulting to time.Local when unset.
func resolveLoc(tz string) (*time.Location, error) {
	if tz == "" {
		return time.Local, nil
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, fmt.Errorf("loading VAKT_TZ %q: %w", tz, err)
	}
	return loc, nil
}

// printfln is fmt.Fprintf plus a trailing newline, with the write error
// deliberately discarded — a debug tool has nowhere useful to report it.
func printfln(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format+"\n", args...)
}

func printTasks(w io.Writer, tasks map[string]index.Task) {
	ids := make([]string, 0, len(tasks))
	for id := range tasks {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	printfln(w, "== tasks (%d) ==", len(ids))
	for _, id := range ids {
		t := tasks[id]
		printfln(w, "- %s", t.ID)
		printfln(w, "    path: %s:%d", t.Path, t.Line)

		names := make([]string, 0, len(t.Values))
		for name := range t.Values {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			printfln(w, "    @%s: %s", name, formatValue(t.Values[name]))
		}
	}
	printfln(w, "")
}

// formatValue renders a directive.Value's typed field per its Type, falling
// back to Raw for anything unexpected rather than printing a zero value.
func formatValue(v directive.Value) string {
	switch v.Type {
	case directive.TypeString, directive.TypeEnum:
		return v.Str
	case directive.TypeInteger:
		return fmt.Sprintf("%d", v.Int)
	case directive.TypeDatetime:
		return v.Time.Format(time.RFC3339)
	case directive.TypeCron:
		return fmt.Sprintf("%v", v.Cron)
	default:
		return v.Raw
	}
}

func printDiagnostics(w io.Writer, diags []model.Diagnostic) {
	printfln(w, "== diagnostics (%d) ==", len(diags))
	for _, d := range diags {
		code := "-"
		if d.Code != nil {
			code = *d.Code
		}
		printfln(w, "- [%s] %s %s:%d task=%s %s",
			d.Severity, code, d.FilePath, d.Line, d.TaskId, d.Message)
	}
}
