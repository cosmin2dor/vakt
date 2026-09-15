// Package engine wires the reactive cycle (SDD.md §2.2): watcher changes
// are re-parsed into the index, and reconciliations that changed a task or
// a diagnostic are published on the event bus. It adds no new logic of
// its own — every step below calls an existing package's public API.
package engine

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/cosmin2dor/vakt/internal/engine/index"
	"github.com/cosmin2dor/vakt/internal/engine/vault"
	"github.com/cosmin2dor/vakt/internal/engine/watcher"
	"github.com/cosmin2dor/vakt/internal/events"
)

// Engine closes the loop: watcher -> re-parse -> index reconciliation ->
// event bus. It holds no HTTP or provider knowledge (SDD.md §2.6).
type Engine struct {
	idx     *index.Index
	watcher *watcher.Watcher
	bus     *events.Bus[index.Change]
}

// New builds the index from root, constructs a watcher rooted at the same
// path sharing reg for echo suppression, and creates the bus. reg should
// be the same *vault.HashRegistry passed to any vault.Writer the caller
// uses, so the daemon's own writes are recognized and not re-indexed.
func New(root string, loc *time.Location, reg *vault.HashRegistry) (*Engine, error) {
	idx := index.New(root, loc)
	if err := idx.Build(); err != nil {
		return nil, fmt.Errorf("engine: building initial index: %w", err)
	}

	w, err := watcher.New(root, reg, watcher.DefaultConfig())
	if err != nil {
		return nil, fmt.Errorf("engine: constructing watcher: %w", err)
	}

	return &Engine{
		idx:     idx,
		watcher: w,
		bus:     events.New[index.Change](),
	}, nil
}

// Run starts the watcher and reconciles the index on every reported
// change, publishing to the bus whenever a task or a diagnostic actually
// changed. It blocks until ctx is cancelled, then stops the watcher and
// returns.
func (e *Engine) Run(ctx context.Context) error {
	watchErr := make(chan error, 1)
	go func() { watchErr <- e.watcher.Run(ctx) }()

	changes := e.watcher.Changes()
	for {
		select {
		case <-ctx.Done():
			return <-watchErr // wait for the watcher to actually stop before returning
		case rel, ok := <-changes:
			if !ok {
				return <-watchErr
			}
			diagsBefore := e.idx.Diagnostics()
			change, err := e.idx.ReconcileFile(rel)
			if err != nil {
				continue // a read/parse error here isn't fatal to the cycle; the file stays as last known-good (SDD.md G15)
			}
			// A diagnostic can appear or clear (e.g. a duplicate @id)
			// without any task being added/updated/removed — still worth
			// waking subscribers for, since nothing else observes it.
			if isEmpty(change) && reflect.DeepEqual(diagsBefore, e.idx.Diagnostics()) {
				continue // nothing a subscriber could observe changed
			}
			e.bus.Publish(change)
		}
	}
}

// Subscribe registers a new subscriber to the index's change events.
func (e *Engine) Subscribe(bufLen int) *events.Subscription[index.Change] {
	return e.bus.Subscribe(bufLen)
}

// Index returns read access to the live index for callers.
func (e *Engine) Index() *index.Index {
	return e.idx
}

func isEmpty(c index.Change) bool {
	return len(c.Added) == 0 && len(c.Updated) == 0 && len(c.Removed) == 0
}
