package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cosmin2dor/vakt/internal/engine/index"
	"github.com/cosmin2dor/vakt/internal/engine/vault"
)

func startEngine(t *testing.T, e *Engine) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = e.Run(ctx)
		close(done)
	}()
	return func() {
		cancel()
		<-done
	}
}

func awaitChange(t *testing.T, ch <-chan index.Change, timeout time.Duration) index.Change {
	t.Helper()
	select {
	case c := <-ch:
		return c
	case <-time.After(timeout):
		t.Fatalf("timed out after %s waiting for a change", timeout)
		return index.Change{}
	}
}

func assertNoChange(t *testing.T, ch <-chan index.Change, wait time.Duration) {
	t.Helper()
	select {
	case c := <-ch:
		t.Fatalf("expected no change, got %+v", c)
	case <-time.After(wait):
	}
}

// External edits (simulated with plain os.WriteFile, no vault.Writer)
// must reach the index and the bus.
func TestExternalEditUpdatesIndexAndPublishes(t *testing.T) {
	root := t.TempDir()
	reg := vault.NewHashRegistry()

	e, err := New(root, nil, reg)
	require.NoError(t, err)
	stop := startEngine(t, e)
	defer stop()

	sub := e.Subscribe(4)
	defer sub.Unsubscribe()

	path := filepath.Join(root, "chores.md")
	require.NoError(t, os.WriteFile(path, []byte("- [ ] wash dishes @id(wash)\n"), 0o644))

	change := awaitChange(t, sub.C(), 2*time.Second)
	require.Len(t, change.Added, 1)
	require.Equal(t, "wash", change.Added[0].ID)

	task, ok := e.Index().Task("wash")
	require.True(t, ok)
	require.Equal(t, "wash", task.ID)
}

// The daemon's own writeback, through a vault.Writer sharing the same
// HashRegistry, must not produce a bus event. This is the one place a
// path-form mismatch (absolute vs. vault-relative) between the watcher
// and the writer would surface as a silent failure, so the assertion
// below is only honest evidence that nothing arrived within the window —
// not proof no event could ever arrive.
func TestDaemonWritebackProducesNoEvent(t *testing.T) {
	root := t.TempDir()
	reg := vault.NewHashRegistry()
	writer := vault.NewWriter(reg)

	e, err := New(root, nil, reg)
	require.NoError(t, err)
	stop := startEngine(t, e)
	defer stop()

	sub := e.Subscribe(4)
	defer sub.Unsubscribe()

	path := filepath.Join(root, "chores.md")
	_, err = writer.Write(path, []byte("- [ ] wash dishes @id(wash)\n"))
	require.NoError(t, err)

	assertNoChange(t, sub.C(), 1500*time.Millisecond)
}
