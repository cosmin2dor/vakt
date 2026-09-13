// Package directive lexes @name(value) spans out of a line of Markdown,
// and — from M2 onward — assigns typed values, validates them against the
// registry, and computes the surgical byte-range patch for a single span.
// See SDD.md §2.6 and the resolved gaps in §3 (G1-G5).
package directive
