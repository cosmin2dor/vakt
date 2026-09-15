package watcher

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/cosmin2dor/vakt/internal/engine/vault"
)

// Config tunes the debounce/stability/rescan windows. None of these are
// specified by the SDD, so they are fields rather than constants — a
// caller (or a test) can tighten or loosen them.
type Config struct {
	// Debounce coalesces a burst of fsnotify events for the same path
	// into one reaction, timed from the last event seen.
	Debounce time.Duration
	// StabilityWait is the gap between successive size/mtime checks used
	// to confirm a file has stopped changing before it is read.
	StabilityWait time.Duration
	// StabilityChecks bounds how many stable-check rounds are attempted
	// before giving up waiting and reading the file as-is.
	StabilityChecks int
	// RescanInterval is how often the full tree is re-hashed to catch
	// events fsnotify silently dropped (SDD.md §6 risks).
	RescanInterval time.Duration
}

// DefaultConfig returns windows sized so "reaches the caller within a
// second" (the task's own exit bar) holds with headroom.
func DefaultConfig() Config {
	return Config{
		Debounce:        150 * time.Millisecond,
		StabilityWait:   75 * time.Millisecond,
		StabilityChecks: 3,
		RescanInterval:  30 * time.Second,
	}
}

// Watcher reports vault-relative paths whose content changed for a real,
// non-echo reason. It watches directories (not files, SDD.md §2.7),
// debounces bursts, waits for write-stability, and suppresses the
// daemon's own writes via a vault.HashRegistry. It knows nothing about
// parsing, indexing, or the event bus — that is the caller's job.
type Watcher struct {
	root     string
	registry *vault.HashRegistry
	cfg      Config

	fsw *fsnotify.Watcher
	out chan string

	mu     sync.Mutex
	timers map[string]*time.Timer // absolute path -> pending debounce timer
	hashes map[string][32]byte    // vault-relative path -> last known content hash
	closed bool
}

// New creates a Watcher rooted at root, registers a watch on every
// directory in the tree, and seeds a hash baseline for the periodic
// rescan. registry may be nil if the caller has no writer echoes to
// suppress (e.g. a read-only vault).
func New(root string, registry *vault.HashRegistry, cfg Config) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("watcher: creating fsnotify watcher: %w", err)
	}

	w := &Watcher{
		root:     root,
		registry: registry,
		cfg:      cfg,
		fsw:      fsw,
		out:      make(chan string, 64),
		timers:   make(map[string]*time.Timer),
		hashes:   make(map[string][32]byte),
	}

	if err := w.addDirsRecursive(root); err != nil {
		_ = fsw.Close()
		return nil, fmt.Errorf("watcher: watching %q: %w", root, err)
	}

	baseline, err := snapshotHashes(root)
	if err != nil {
		_ = fsw.Close()
		return nil, fmt.Errorf("watcher: seeding hash baseline: %w", err)
	}
	w.hashes = baseline

	return w, nil
}

// Changes returns vault-relative paths for real external edits. The
// caller (wire-reactive-cycle) re-parses and reconciles; this package
// does not know what an index is.
func (w *Watcher) Changes() <-chan string {
	return w.out
}

// Run drives the event loop and the periodic rescan until ctx is
// cancelled or the underlying fsnotify watcher errors out.
func (w *Watcher) Run(ctx context.Context) error {
	defer func() { _ = w.Close() }()

	ticker := time.NewTicker(w.cfg.RescanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-w.fsw.Events:
			if !ok {
				return nil
			}
			w.handleEvent(ev)
		case _, ok := <-w.fsw.Errors:
			if !ok {
				return nil
			}
			// fsnotify surfaces internal errors (e.g. a dropped watch);
			// the periodic rescan is exactly the backstop for that, so
			// we keep running rather than tearing down the loop.
		case <-ticker.C:
			w.rescan()
		}
	}
}

// Close releases the underlying fsnotify watcher. Safe to call more than
// once; Run also calls it on exit.
func (w *Watcher) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	w.mu.Unlock()
	return w.fsw.Close()
}

// handleEvent dispatches one raw fsnotify event: a new directory gets its
// own watch added (best-effort support for subdirectories created after
// start), a markdown file write/create gets debounced for processing.
func (w *Watcher) handleEvent(ev fsnotify.Event) {
	info, err := os.Lstat(ev.Name)
	if err == nil && info.IsDir() {
		if ev.Op&fsnotify.Create != 0 {
			_ = w.addDirsRecursive(ev.Name) // best-effort; a failed add here is caught by the next rescan
		}
		return
	}

	if !isMarkdown(ev.Name) {
		return
	}
	if ev.Op&(fsnotify.Write|fsnotify.Create) == 0 {
		return
	}

	w.scheduleDebounced(ev.Name)
}

// scheduleDebounced resets a per-path timer so a burst of events for the
// same path collapses into a single processPath call, fired once no new
// event has arrived for cfg.Debounce.
func (w *Watcher) scheduleDebounced(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if t, ok := w.timers[path]; ok {
		t.Reset(w.cfg.Debounce)
		return
	}
	w.timers[path] = time.AfterFunc(w.cfg.Debounce, func() {
		w.mu.Lock()
		delete(w.timers, path)
		w.mu.Unlock()
		w.processPath(path)
	})
}

// processPath waits for the file to stop changing, then hashes it and
// either suppresses it as the daemon's own echo or emits it as a real
// external change.
func (w *Watcher) processPath(path string) {
	if !w.waitStable(path) {
		return // gone, or never settled — next rescan will catch a real edit
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return
	}
	hash := sha256.Sum256(content)

	rel, err := w.relPath(path)
	if err != nil {
		return
	}

	// The writer's HashRegistry is keyed by whatever path the writer was
	// called with — the absolute filesystem path, not our vault-relative
	// form — so the echo check must use the same key space.
	if w.registry != nil && w.registry.Check(path, hash) {
		w.recordHash(rel, hash)
		return // echo of the daemon's own write
	}

	w.recordHash(rel, hash)
	w.emit(rel)
}

// waitStable reports whether path's size and mtime stop changing across
// cfg.StabilityChecks rounds, each separated by cfg.StabilityWait. A
// naive "check twice with a gap" repeated a few times — enough to avoid
// reading a half-written file, not a general locking protocol.
func (w *Watcher) waitStable(path string) bool {
	prev, err := os.Stat(path)
	if err != nil {
		return false
	}

	for i := 0; i < w.cfg.StabilityChecks; i++ {
		time.Sleep(w.cfg.StabilityWait)
		cur, err := os.Stat(path)
		if err != nil {
			return false
		}
		if cur.Size() == prev.Size() && cur.ModTime().Equal(prev.ModTime()) {
			return true
		}
		prev = cur
	}
	return true // gave up waiting; read the last-seen snapshot rather than drop it
}

// rescan re-hashes the whole tree and reports anything that changed
// since the last time we looked, backing up events fsnotify may have
// dropped (SDD.md §6 risks). Deletions are not reported — out of scope.
func (w *Watcher) rescan() {
	current, err := snapshotHashes(w.root)
	if err != nil {
		return
	}

	w.mu.Lock()
	prev := w.hashes
	w.mu.Unlock()

	for _, rel := range diffHashes(prev, current) {
		hash := current[rel]
		absPath := filepath.Join(w.root, filepath.FromSlash(rel))
		if w.registry != nil && w.registry.Check(absPath, hash) {
			w.recordHash(rel, hash)
			continue
		}
		w.recordHash(rel, hash)
		w.emit(rel)
	}
}

func (w *Watcher) recordHash(rel string, hash [32]byte) {
	w.mu.Lock()
	w.hashes[rel] = hash
	w.mu.Unlock()
}

func (w *Watcher) emit(rel string) {
	w.out <- rel
}

func (w *Watcher) relPath(absPath string) (string, error) {
	rel, err := filepath.Rel(w.root, absPath)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

// addDirsRecursive registers a watch on dir and every subdirectory
// beneath it (SDD.md §2.7: watch directories, not files).
func (w *Watcher) addDirsRecursive(dir string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		return w.fsw.Add(path)
	})
}

// snapshotHashes hashes every markdown file under root, keyed by
// vault-relative path. Shared by the initial baseline and each rescan.
func snapshotHashes(root string) (map[string][32]byte, error) {
	hashes := make(map[string][32]byte)

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !isMarkdown(path) {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil // transient read races (mid-write) are left for the next rescan
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		hashes[filepath.ToSlash(rel)] = sha256.Sum256(content)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return hashes, nil
}

// diffHashes returns vault-relative paths present in current whose hash
// differs from (or is absent from) prev — i.e. added or changed files.
// Pure and independent of any live event stream, so the rescan's
// hash-diff logic is testable on its own.
func diffHashes(prev, current map[string][32]byte) []string {
	var changed []string
	for rel, hash := range current {
		if old, ok := prev[rel]; !ok || old != hash {
			changed = append(changed, rel)
		}
	}
	return changed
}

func isMarkdown(path string) bool {
	return strings.ToLower(filepath.Ext(path)) == ".md"
}
