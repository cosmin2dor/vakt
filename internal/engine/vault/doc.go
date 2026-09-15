// Package vault walks the vault directory, reads Markdown files, and owns
// the atomic, per-path serialized writer that patches directive spans back
// to disk. See SDD.md §2.6.
//
// Walk discovers tasks by @id (SDD G1, G3-G5). Writer publishes content
// atomically and per-path serialized (SDD §2.2, §2.7) and records each
// write's hash in a HashRegistry for the (separate, later) file watcher's
// echo suppression. File watching itself is separate, later work.
package vault
