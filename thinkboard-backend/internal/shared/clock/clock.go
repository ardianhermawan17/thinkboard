// Package clock is the sole allowed source of time.Now() in this module (03-backend-folder-architecture.md §4).
package clock

import "time"

// Clock returns the current time. Swap for a fake in tests that need deterministic time.
type Clock interface {
	Now() time.Time
}

type real struct{}

func (real) Now() time.Time { return time.Now() }

// Real is the production Clock.
var Real Clock = real{}
