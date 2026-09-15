package watcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cosmin2dor/vakt/internal/engine/vault"
)

// testConfig keeps the exit bar's "within a second" headroom while
// staying fast: debounce+stability well under a second, still non-zero
// so coalescing has something to coalesce.
func testConfig() Config {
	return Config{
		Debounce:        50 * time.Millisecond,
		StabilityWait:   30 * time.Millisecond,
		StabilityChecks: 2,
		RescanInterval:  time.Hour, // disabled for tests that assert on live events only
	}
}

func startWatcher(t *testing.T, root string, reg *vault.HashRegistry, cfg Config) (*Watcher, func()) {
	t.Helper()
	w, err := New(root, reg, cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = w.Run(ctx)
		close(done)
	}()

	return w, func() {
		cancel()
		<-done
	}
}

func awaitChange(t *testing.T, ch <-chan string, timeout time.Duration) string {
	t.Helper()
	select {
	case rel := <-ch:
		return rel
	case <-time.After(timeout):
		t.Fatalf("timed out after %s waiting for a change", timeout)
		return ""
	}
}

func assertNoChange(t *testing.T, ch <-chan string, wait time.Duration) {
	t.Helper()
	select {
	case rel := <-ch:
		t.Fatalf("expected no change, got %q", rel)
	case <-time.After(wait):
	}
}

func TestExternalEditReachesCallerWithinASecond(t *testing.T) {
	root := t.TempDir()
	reg := vault.NewHashRegistry()

	w, stop := startWatcher(t, root, reg, testConfig())
	defer stop()

	path := filepath.Join(root, "chores.md")
	require.NoError(t, os.WriteFile(path, []byte("- [ ] wash dishes @id(wash)\n"), 0o644))

	rel := awaitChange(t, w.Changes(), time.Second)
	require.Equal(t, "chores.md", rel)
}

func TestDaemonWrittenFileProducesNoEvent(t *testing.T) {
	root := t.TempDir()
	reg := vault.NewHashRegistry()
	writer := vault.NewWriter(reg)

	w, stop := startWatcher(t, root, reg, testConfig())
	defer stop()

	path := filepath.Join(root, "chores.md")
	_, err := writer.Write(path, []byte("- [ ] wash dishes @id(wash)\n"))
	require.NoError(t, err)

	assertNoChange(t, w.Changes(), 500*time.Millisecond)
}

func TestBurstOfWritesCoalescesToOneChange(t *testing.T) {
	root := t.TempDir()
	reg := vault.NewHashRegistry()

	w, stop := startWatcher(t, root, reg, testConfig())
	defer stop()

	path := filepath.Join(root, "chores.md")
	for i := 0; i < 5; i++ {
		require.NoError(t, os.WriteFile(path, []byte("revision "+string(rune('0'+i))), 0o644))
		time.Sleep(5 * time.Millisecond)
	}

	rel := awaitChange(t, w.Changes(), time.Second)
	require.Equal(t, "chores.md", rel)
	assertNoChange(t, w.Changes(), 300*time.Millisecond)
}

func TestExternalEditAfterDaemonWriteIsStillDetected(t *testing.T) {
	root := t.TempDir()
	reg := vault.NewHashRegistry()
	writer := vault.NewWriter(reg)

	w, stop := startWatcher(t, root, reg, testConfig())
	defer stop()

	path := filepath.Join(root, "chores.md")
	_, err := writer.Write(path, []byte("- [ ] wash dishes @id(wash)\n"))
	require.NoError(t, err)
	assertNoChange(t, w.Changes(), 300*time.Millisecond)

	require.NoError(t, os.WriteFile(path, []byte("- [ ] wash dishes @id(wash) @done\n"), 0o644))
	rel := awaitChange(t, w.Changes(), time.Second)
	require.Equal(t, "chores.md", rel)
}

func TestDiffHashesReportsAddedAndChangedOnly(t *testing.T) {
	hashA := fakeHash("a")
	hashB := fakeHash("b")

	prev := map[string][32]byte{
		"unchanged.md": hashA,
		"changed.md":   hashA,
	}
	current := map[string][32]byte{
		"unchanged.md": hashA,
		"changed.md":   hashB,
		"new.md":       hashB,
	}

	changed := diffHashes(prev, current)
	require.ElementsMatch(t, []string{"changed.md", "new.md"}, changed)
}

func TestRescanCatchesAMissedEvent(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "chores.md")
	require.NoError(t, os.WriteFile(path, []byte("original\n"), 0o644))

	reg := vault.NewHashRegistry()
	w, err := New(root, reg, testConfig())
	require.NoError(t, err)
	defer func() { _ = w.Close() }()

	// Simulate fsnotify dropping the event entirely: mutate the file on
	// disk without going through the live watch loop, then invoke the
	// rescan pass directly.
	require.NoError(t, os.WriteFile(path, []byte("changed while events were dropped\n"), 0o644))
	w.rescan()

	rel := awaitChange(t, w.Changes(), time.Second)
	require.Equal(t, "chores.md", rel)
}

func TestNewSubdirectoryCreatedAfterStartIsWatched(t *testing.T) {
	root := t.TempDir()
	reg := vault.NewHashRegistry()

	w, stop := startWatcher(t, root, reg, testConfig())
	defer stop()

	sub := filepath.Join(root, "kitchen")
	require.NoError(t, os.Mkdir(sub, 0o755))

	// Give the watcher's dynamic-add handling a moment to register the
	// new directory before writing into it.
	time.Sleep(100 * time.Millisecond)

	path := filepath.Join(sub, "chores.md")
	require.NoError(t, os.WriteFile(path, []byte("- [ ] sweep @id(sweep)\n"), 0o644))

	rel := awaitChange(t, w.Changes(), time.Second)
	require.Equal(t, "kitchen/chores.md", rel)
}

// fakeHash builds a distinct [32]byte per input without hashing real
// content — diffHashes only cares that hashes differ, not their origin.
func fakeHash(s string) [32]byte {
	var h [32]byte
	copy(h[:], s)
	return h
}
