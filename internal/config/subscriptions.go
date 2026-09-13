package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// subscriptionsFile is the name of the JSON file the Store keeps inside its
// base directory.
const subscriptionsFile = "subscriptions.json"

// Keys holds the two public keys a browser returns alongside a push
// subscription's endpoint, matching the field names of the standard
// PushSubscription.toJSON() shape.
type Keys struct {
	P256dh string `json:"p256dh"`
	Auth   string `json:"auth"`
}

// PushSubscription is a Web Push subscription as a browser's
// PushSubscription.toJSON() produces it. Field names and JSON tags match
// that shape exactly so a later HTTP handler (implement-subscription-
// endpoints) can decode a request body straight into this type.
//
// Endpoint is the subscription's natural identity: browsers hand out no
// other stable identifier, and SDD.md G14 prunes a subscription by acting
// on the endpoint a push service's 404/410 response was for.
type PushSubscription struct {
	Endpoint       string `json:"endpoint"`
	Keys           Keys   `json:"keys"`
	ExpirationTime *int64 `json:"expirationTime,omitempty"`
}

// Store persists Web Push subscriptions as a JSON file on disk, keyed by
// endpoint. On disk the file is a JSON object mapping endpoint -> the
// subscription for that endpoint (rather than a JSON array), so a reader
// can look one up by endpoint without a scan; Go callers get PushSubscription
// values back either way through List, Add, and Remove.
//
// Writes are published atomically: the new content is written to a temp
// file in the same directory as the store's file and then moved into place
// with os.Rename, so a reader never observes a partial write and a crash
// mid-write cannot corrupt the store (SDD.md §2.2, §2.7).
//
// A Store is safe for concurrent use.
type Store struct {
	mu   sync.RWMutex
	dir  string
	path string
	subs map[string]PushSubscription
}

// NewStore opens (or initializes) a subscription store rooted at dir. dir
// is a caller-supplied directory — production wiring points it at the
// /config volume, tests point it at a temp directory. This package never
// hardcodes /config.
//
// If dir has no subscriptions file yet, NewStore returns an empty store
// rather than an error: a fresh /config volume is the normal, expected
// starting state.
func NewStore(dir string) (*Store, error) {
	if dir == "" {
		return nil, fmt.Errorf("config: store directory must not be empty")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("config: creating store directory: %w", err)
	}

	s := &Store{
		dir:  dir,
		path: filepath.Join(dir, subscriptionsFile),
		subs: make(map[string]PushSubscription),
	}

	data, err := os.ReadFile(s.path)
	switch {
	case err == nil:
		var subs map[string]PushSubscription
		if err := json.Unmarshal(data, &subs); err != nil {
			return nil, fmt.Errorf("config: parsing %s: %w", s.path, err)
		}
		s.subs = subs
	case os.IsNotExist(err):
		// No file yet: start empty.
	default:
		return nil, fmt.Errorf("config: reading %s: %w", s.path, err)
	}

	return s, nil
}

// Add inserts sub, or replaces the existing subscription with the same
// endpoint. A browser re-subscribing at an endpoint it already holds is a
// normal occurrence, not a conflict, so Add is an idempotent upsert rather
// than an error on a duplicate endpoint.
func (s *Store) Add(sub PushSubscription) error {
	if sub.Endpoint == "" {
		return fmt.Errorf("config: subscription endpoint must not be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.subs[sub.Endpoint] = sub
	return s.persistLocked()
}

// Remove deletes the subscription for endpoint. Removing an endpoint that
// is not present is a no-op, not an error: it keeps callers idempotent when
// e.g. a push provider's pruning logic (G14) and a browser's own unsubscribe
// both race to remove the same endpoint.
func (s *Store) Remove(endpoint string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.subs[endpoint]; !ok {
		return nil
	}
	delete(s.subs, endpoint)
	return s.persistLocked()
}

// List returns a snapshot of every stored subscription. The order is
// unspecified.
func (s *Store) List() []PushSubscription {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]PushSubscription, 0, len(s.subs))
	for _, sub := range s.subs {
		out = append(out, sub)
	}
	return out
}

// persistLocked writes s.subs to disk atomically. Callers must hold s.mu.
func (s *Store) persistLocked() error {
	data, err := json.MarshalIndent(s.subs, "", "  ")
	if err != nil {
		return fmt.Errorf("config: encoding subscriptions: %w", err)
	}

	tmp, err := os.CreateTemp(s.dir, ".subscriptions-*.tmp")
	if err != nil {
		return fmt.Errorf("config: creating temp file: %w", err)
	}
	tmpPath := tmp.Name()
	// If anything below fails before the rename, remove the temp file
	// rather than leaving it behind.
	succeeded := false
	defer func() {
		if !succeeded {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("config: writing temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("config: syncing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("config: closing temp file: %w", err)
	}

	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("config: publishing %s: %w", s.path, err)
	}
	succeeded = true
	return nil
}
