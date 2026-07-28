// Package widgets is a fixture for the noclockindomain analyzer: a domain
// feature service that reads the wall clock and randomness directly.
//
// Implements: REQ-004 (test fixture).
// Per: ADR-0009.
// Discipline: C-14.
package widgets

import (
	"math/rand"
	randv2 "math/rand/v2"
	"time"

	"github.com/acme/modules/ports"
)

// Service is a domain service with an injected clock.
type Service struct {
	clock ports.Clock
}

// BadBillingPeriod computes period math on the raw wall clock — flagged.
func (s *Service) BadBillingPeriod() time.Time {
	start := time.Now() // want `direct time\.Now\(\) in domain service code is untestable`
	return start.AddDate(0, 1, 0)
}

// BadElapsed uses time.Since / time.Until — flagged.
func (s *Service) BadElapsed(t time.Time) (time.Duration, time.Duration) {
	elapsed := time.Since(t)   // want `direct time\.Since\(\) in domain service code is untestable`
	remaining := time.Until(t) // want `direct time\.Until\(\) in domain service code is untestable`
	return elapsed, remaining
}

// BadRandom draws randomness from math/rand and math/rand/v2 — flagged.
func (s *Service) BadRandom() (int, int) {
	a := rand.Intn(10)   // want `direct rand\.Intn\(\) \(math/rand\) in domain service code is nondeterministic`
	b := randv2.IntN(10) // want `direct randv2\.IntN\(\) \(math/rand/v2\) in domain service code is nondeterministic`
	return a, b
}

// GoodInjectedClock uses the injected ports.Clock — never flagged.
func (s *Service) GoodInjectedClock() time.Time {
	return s.clock.Now()
}

// GoodTimeValueReference references time.Now as a value (the field-default
// injection idiom), not a call — never flagged.
func (s *Service) GoodTimeValueReference() func() time.Time {
	return time.Now
}

// GoodNonClockTimeAPI uses time-package APIs that do not read the wall
// clock — never flagged.
func (s *Service) GoodNonClockTimeAPI(v string) (time.Time, error) {
	return time.Parse(time.RFC3339, v)
}
