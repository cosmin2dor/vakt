// Package vault walks the vault directory, reads Markdown files, and owns
// the atomic, per-path serialized writer that patches directive spans back
// to disk. See SDD.md §2.6 and the non-negotiable rule in §4 (M2): Vakt
// never re-serializes a file from its parsed model.
package vault
