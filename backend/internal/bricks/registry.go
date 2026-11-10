package bricks

import "github.com/STGime/laamo/backend/internal/engine"

// All returns the executors by catalog brick type. Bricks land here one
// issue at a time (#24 Every… and Tap Go, #25 Search, …); a stack using a
// brick without an executor fails at the start of its run with a clear
// Problem card.
func All() map[string]engine.Brick {
	return map[string]engine.Brick{}
}
