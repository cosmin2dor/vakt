package vault

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// writeLockedHook, when set by a test, runs once per Write call while its
// per-path lock is held.
var writeLockedHook func()

// Writer publishes byte content to vault paths atomically and serializes
// concurrent writes to the same path (SDD.md §2.2, §2.7). A Writer is safe
// for concurrent use and holds one *sync.Mutex per path, created on demand.
type Writer struct {
	locks sync.Map // path -> *sync.Mutex

	registry *HashRegistry
}

// NewWriter returns a Writer backed by reg for echo-suppression lookups.
// reg may be nil if the caller does not need the registry (e.g. tests that
// only exercise write mechanics).
func NewWriter(reg *HashRegistry) *Writer {
	return &Writer{registry: reg}
}

// Write publishes content to path: a temp file in path's own directory is
// written, fsynced, then renamed over path (same filesystem, so the rename
// is atomic and a reader never observes a partial file). Writes to the
// same path serialize; writes to different paths run concurrently. On
// success the content's SHA-256 is recorded in the writer's HashRegistry,
// if one was configured, so a later watcher can recognize its own write.
func (w *Writer) Write(path string, content []byte) ([32]byte, error) {
	lockIface, _ := w.locks.LoadOrStore(path, &sync.Mutex{})
	lock := lockIface.(*sync.Mutex)

	lock.Lock()
	defer lock.Unlock()

	// White-box test seam: lets tests observe that no two goroutines are
	// ever inside this critical section at once. Nil in production.
	if writeLockedHook != nil {
		writeLockedHook()
	}

	hash := sha256.Sum256(content)

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".vakt-write-*.tmp")
	if err != nil {
		return hash, fmt.Errorf("vault: creating temp file in %q: %w", dir, err)
	}
	tmpPath := tmp.Name()
	succeeded := false
	defer func() {
		if !succeeded {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return hash, fmt.Errorf("vault: writing temp file %q: %w", tmpPath, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return hash, fmt.Errorf("vault: syncing temp file %q: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		return hash, fmt.Errorf("vault: closing temp file %q: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return hash, fmt.Errorf("vault: publishing %q: %w", path, err)
	}
	succeeded = true

	if w.registry != nil {
		w.registry.Record(path, hash)
	}

	return hash, nil
}

// HashRegistry records the content hash of the daemon's own writes, keyed
// by path, so a filesystem watcher can ask "did I just write this?" after
// an fsnotify event and drop the resulting echo (SDD.md §2.2).
//
// Check consumes the record: a matching hash is removed on the first
// successful check. This is deliberately not keep-forever. The registry
// exists to answer one question — is this fsnotify event the daemon's own
// echo — and once that question has been asked and answered for a given
// write, the record has done its job. Keeping it around risks a false
// positive: a later, genuinely external write with coincidentally
// identical bytes would be misattributed as an echo and silently dropped.
// Prune-on-check accepts more bookkeeping in exchange for that not
// happening. A watcher that never gets around to checking will simply
// leak one entry per unconsumed write, which is bounded by write volume
// and not a concern at vault scale.
type HashRegistry struct {
	mu      sync.Mutex
	entries map[string][32]byte
}

// NewHashRegistry returns an empty HashRegistry.
func NewHashRegistry() *HashRegistry {
	return &HashRegistry{entries: make(map[string][32]byte)}
}

// Record stores hash as the most recently written content for path,
// overwriting any prior unconsumed record for that path.
func (r *HashRegistry) Record(path string, hash [32]byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[path] = hash
}

// Check reports whether hash is the recorded hash for path. A match is
// consumed: the record is removed so a subsequent external write with the
// same bytes is not mistaken for a second echo of the same write.
func (r *HashRegistry) Check(path string, hash [32]byte) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	got, ok := r.entries[path]
	if !ok || got != hash {
		return false
	}
	delete(r.entries, path)
	return true
}
