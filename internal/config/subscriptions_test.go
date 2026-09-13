package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func exampleSub(endpoint string) PushSubscription {
	return PushSubscription{
		Endpoint: endpoint,
		Keys: Keys{
			P256dh: "p256dh-" + endpoint,
			Auth:   "auth-" + endpoint,
		},
	}
}

func TestNewStore_FreshDirectoryIsEmpty(t *testing.T) {
	dir := t.TempDir()

	s, err := NewStore(dir)
	require.NoError(t, err)
	assert.Empty(t, s.List())

	// No file should be created just by opening the store.
	_, err = os.Stat(filepath.Join(dir, subscriptionsFile))
	assert.True(t, os.IsNotExist(err))
}

func TestAddAndList(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	require.NoError(t, err)

	sub := exampleSub("https://push.example/a")
	require.NoError(t, s.Add(sub))

	got := s.List()
	require.Len(t, got, 1)
	assert.Equal(t, sub, got[0])
}

func TestAdd_DuplicateEndpointUpserts(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	require.NoError(t, err)

	first := exampleSub("https://push.example/a")
	require.NoError(t, s.Add(first))

	updated := first
	updated.Keys.Auth = "new-auth"
	require.NoError(t, s.Add(updated))

	got := s.List()
	require.Len(t, got, 1, "duplicate endpoint should upsert, not append")
	assert.Equal(t, "new-auth", got[0].Keys.Auth)
}

func TestAdd_EmptyEndpointErrors(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	require.NoError(t, err)

	err = s.Add(PushSubscription{})
	assert.Error(t, err)
	assert.Empty(t, s.List())
}

func TestRemove_ExistingEndpoint(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	require.NoError(t, err)

	sub := exampleSub("https://push.example/a")
	require.NoError(t, s.Add(sub))
	require.NoError(t, s.Remove(sub.Endpoint))

	assert.Empty(t, s.List())
}

func TestRemove_NonexistentEndpointIsNoop(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	require.NoError(t, err)

	err = s.Remove("https://push.example/does-not-exist")
	assert.NoError(t, err)
	assert.Empty(t, s.List())
}

// TestSurvivesRestart is the exit criterion from the issue: subscriptions
// written by one Store instance must be read back correctly by a second,
// independent Store instance pointed at the same directory — the natural
// stand-in for "survives a container restart" without a real restart.
func TestSurvivesRestart(t *testing.T) {
	dir := t.TempDir()

	first, err := NewStore(dir)
	require.NoError(t, err)

	subA := exampleSub("https://push.example/a")
	subB := exampleSub("https://push.example/b")
	require.NoError(t, first.Add(subA))
	require.NoError(t, first.Add(subB))
	require.NoError(t, first.Remove(subB.Endpoint))

	second, err := NewStore(dir)
	require.NoError(t, err)

	got := second.List()
	require.Len(t, got, 1)
	assert.Equal(t, subA, got[0])
}

// TestAtomicWrite_NoPartialFileVisible checks that after a successful
// write, only the real subscriptions file is present in the directory —
// no leftover temp file from the write-then-rename sequence.
func TestAtomicWrite_NoPartialFileVisible(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	require.NoError(t, err)

	require.NoError(t, s.Add(exampleSub("https://push.example/a")))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "exactly one file should remain in the store directory")
	assert.Equal(t, subscriptionsFile, entries[0].Name())
}

// TestConcurrentAddRemove exercises Add/Remove from many goroutines at
// once. Run with -race to confirm the mutex actually guards both the
// in-memory map and the write path.
func TestConcurrentAddRemove(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	require.NoError(t, err)

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n * 2)

	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			endpoint := "https://push.example/" + string(rune('a'+i%26))
			_ = s.Add(exampleSub(endpoint))
		}()
		go func() {
			defer wg.Done()
			endpoint := "https://push.example/" + string(rune('a'+i%26))
			_ = s.Remove(endpoint)
		}()
	}
	wg.Wait()

	// No assertion on the final contents (the interleaving is
	// nondeterministic) — this test's job is to prove -race stays clean
	// and the store remains internally consistent enough to reload.
	reloaded, err := NewStore(dir)
	require.NoError(t, err)
	_ = reloaded.List()
}
