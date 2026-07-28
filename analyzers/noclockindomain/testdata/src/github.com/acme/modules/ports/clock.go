// Package ports stubs the shared Clock port for the noclockindomain
// analyzer tests.
//
// Implements: REQ-004 (test fixture).
// Per: ADR-0009.
// Discipline: C-14.
package ports

import "time"

// Clock supplies wall-clock time to domain services.
type Clock interface {
	Now() time.Time
}

// SystemClock is the production Clock implementation.
type SystemClock struct{}

// Now returns the current wall-clock time.
func (SystemClock) Now() time.Time { return time.Now() }
