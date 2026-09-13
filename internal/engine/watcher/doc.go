// Package watcher wraps fsnotify with debounce, stability checks, and echo
// suppression so the daemon's own writes never re-trigger themselves. It
// watches directories, not files — see SDD.md §2.7.
package watcher
