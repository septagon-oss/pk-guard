// Package ledger is a fixture for allowlist coverage: the module
// legacy_clock_management is allowlisted under clock_in_domain, so its
// direct wall-clock reads are tolerated (ratcheted debt), producing no
// diagnostics.
//
// Implements: REQ-004 (test fixture).
// Per: ADR-0009.
// Discipline: C-14.
package ledger

import "time"

// AllowlistedNow would be flagged if legacy_clock_management were not in
// the allowlist.
func AllowlistedNow() time.Time {
	return time.Now()
}
