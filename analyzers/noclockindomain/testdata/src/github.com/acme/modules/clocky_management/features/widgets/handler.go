// handler.go — non-service feature file; out of scope for noclockindomain
// even though it calls time.Now() directly.
//
// Implements: REQ-004 (test fixture).
// Per: ADR-0009.
// Discipline: C-14.
package widgets

import "time"

// HandlerTimestamp calls time.Now() in a handler file — never flagged
// (only service.go / service_*.go under features/ are domain code).
func HandlerTimestamp() time.Time {
	return time.Now()
}
