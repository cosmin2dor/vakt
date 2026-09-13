// Package schedule evaluates cron and @once expressions, keeps a min-heap
// of next fire points behind a single timer, and applies the suppression
// ladder from SDD.md §3 (G7). See also §2.7: one timer, not one per task.
package schedule
