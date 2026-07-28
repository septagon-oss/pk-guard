// service_pricing.go — service_*.go files under features/ are domain code
// and are checked exactly like service.go.
//
// Implements: REQ-004 (test fixture).
// Per: ADR-0009.
// Discipline: C-14.
package widgets

import "time"

// BadPricingCutoff reads the wall clock in a service_*.go file — flagged.
func BadPricingCutoff() time.Time {
	return time.Now() // want `direct time\.Now\(\) in domain service code is untestable`
}
