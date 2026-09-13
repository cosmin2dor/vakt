// Package vault walks the vault directory, reads Markdown files, and owns
// the atomic, per-path serialized writer that patches directive spans back
// to disk. See SDD.md §2.6.
//
// Currently read-only: Walk discovers tasks by @id (SDD G1, G3-G5). The
// writer and file watching are separate, later work.
package vault
