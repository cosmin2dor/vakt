// Package pushrecorder is a test fixture that plays the role of a Web Push
// service and user agent. It stands up an httptest server backed by its own
// ECDH P-256 keypair and auth secret, hands out a PushSubscription-shaped
// value pointing at itself, and — on every POST it receives — parses the
// VAPID Authorization header (RFC 8292) and decrypts the aes128gcm envelope
// (RFC 8188 / RFC 8291), exposing headers, VAPID claims, and plaintext for
// a caller's own assertions.
//
// It depends only on RFC 8291/8292 and Go's standard library (plus HMAC for
// HKDF, which stdlib does not provide directly), not on any Vakt production
// package, so it can verify internal/integration/webpush end to end with no
// network dependency and no test-only branch in production code.
package pushrecorder
