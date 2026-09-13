package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// vapidFile is the name of the JSON file the VAPIDStore keeps inside its
// base directory.
const vapidFile = "vapid.json"

// vapidKeypair is the on-disk representation: the P-256 keypair's raw
// bytes, base64url-encoded. Public is the uncompressed EC point (as
// PushManager.subscribe's applicationServerKey expects); Private is the
// raw scalar (SEC 1 §2.3.6).
type vapidKeypair struct {
	Public  string `json:"public_key"`
	Private string `json:"private_key"`
}

// VAPIDStore persists a single VAPID keypair (RFC 8292) on disk, generating
// one on first use. A stable keypair across restarts matters: every
// browser subscription is bound to the public key it was created with, so
// regenerating one on each start would silently invalidate every existing
// subscription (SDD.md §2.3).
//
// Like Store, writes are atomic (temp file + rename) and a VAPIDStore is
// safe for concurrent use.
type VAPIDStore struct {
	mu      sync.RWMutex
	dir     string
	path    string
	keypair vapidKeypair
}

// NewVAPIDStore opens the VAPID keypair rooted at dir, generating and
// persisting a new one if none exists yet ("generate-if-absent"). dir is
// the same /config volume Store uses; this package never hardcodes /config.
func NewVAPIDStore(dir string) (*VAPIDStore, error) {
	if dir == "" {
		return nil, fmt.Errorf("config: vapid store directory must not be empty")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("config: creating store directory: %w", err)
	}

	s := &VAPIDStore{
		dir:  dir,
		path: filepath.Join(dir, vapidFile),
	}

	data, err := os.ReadFile(s.path)
	switch {
	case err == nil:
		var kp vapidKeypair
		if err := json.Unmarshal(data, &kp); err != nil {
			return nil, fmt.Errorf("config: parsing %s: %w", s.path, err)
		}
		s.keypair = kp
	case os.IsNotExist(err):
		kp, err := generateVAPIDKeypair()
		if err != nil {
			return nil, fmt.Errorf("config: generating vapid keypair: %w", err)
		}
		s.keypair = kp
		if err := s.persistLocked(); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("config: reading %s: %w", s.path, err)
	}

	return s, nil
}

// PublicKey returns the base64url-encoded (unpadded) VAPID public key, the
// uncompressed EC point suitable as PushManager.subscribe's
// applicationServerKey.
func (s *VAPIDStore) PublicKey() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.keypair.Public
}

// generateVAPIDKeypair creates a fresh P-256 ECDSA keypair (VAPID, RFC
// 8292, is signed with ECDSA), raw-encoded and base64url-unpadded.
func generateVAPIDKeypair() (vapidKeypair, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return vapidKeypair{}, fmt.Errorf("generating P-256 key: %w", err)
	}

	privBytes, err := priv.Bytes()
	if err != nil {
		return vapidKeypair{}, fmt.Errorf("encoding private key: %w", err)
	}
	pubBytes, err := priv.PublicKey.Bytes()
	if err != nil {
		return vapidKeypair{}, fmt.Errorf("encoding public key: %w", err)
	}

	return vapidKeypair{
		Public:  base64.RawURLEncoding.EncodeToString(pubBytes),
		Private: base64.RawURLEncoding.EncodeToString(privBytes),
	}, nil
}

// persistLocked writes s.keypair to disk atomically. Callers must hold
// s.mu (NewVAPIDStore calls it before any concurrent access is possible).
func (s *VAPIDStore) persistLocked() error {
	data, err := json.MarshalIndent(s.keypair, "", "  ")
	if err != nil {
		return fmt.Errorf("config: encoding vapid keypair: %w", err)
	}

	tmp, err := os.CreateTemp(s.dir, ".vapid-*.tmp")
	if err != nil {
		return fmt.Errorf("config: creating temp file: %w", err)
	}
	tmpPath := tmp.Name()
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
