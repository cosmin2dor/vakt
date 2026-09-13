// Package config persists system state on the /config volume — state that
// belongs to the running daemon, not to the user's vault (SDD.md §2.3).
// A VAPID keypair and the list of enrolled push subscriptions must survive
// a container restart, and putting them in the vault would surface machine
// secrets in the user's file browser and git history. This package starts
// with the push subscription store; VAPID keypair storage is a separate
// piece of state left for a later task.
package config
