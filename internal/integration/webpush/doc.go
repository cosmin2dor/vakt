// Package webpush is the ios_notifications integration module: VAPID
// signing (RFC 8292), aes128gcm payload encryption (RFC 8291), and HTTP
// delivery to every enrolled subscription. It receives task context and a
// fixed payload from the dispatch registry and cannot write to the vault
// itself. See SDD.md §2.6 and PRD.md §4.
package webpush
