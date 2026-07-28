// Package rootsvc holds a service.go that is NOT under a features/<name>/
// directory — out of scope for noclockindomain.
//
// Implements: REQ-004 (test fixture).
// Per: ADR-0009.
// Discipline: C-14.
package rootsvc

import "time"

// InfraNow calls time.Now() outside the features/*/service*.go scope —
// never flagged.
func InfraNow() time.Time {
	return time.Now()
}
