package wrapreassigncheck

// Implements: REQ-004 (test fixture).
// Per: ADR-0005.
// Discipline: C-14.

import (
	"errors"
	"fmt"
)

type role struct{ id string }

func getByName(name string) (*role, error)  { return nil, nil }
func createRole(name string) (*role, error) { return nil, nil }
func work() error                           { return nil }
func cleanup() error                        { return nil }

// --- Should be flagged ---

// Distilled from the auth_management guest_roles.go production incident:
// the recheck exists but ORs the err check with role == nil, so the wrap
// is reachable with the reassigned err being nil (%!w(<nil>)).
func badGuestRoles() (*role, error) {
	r, err := getByName("guest")
	if err != nil {
		return nil, fmt.Errorf("resolve guest role: %w", err)
	}
	if r == nil {
		r, err = createRole("guest")
		if err != nil {
			r, err = getByName("guest")
			if err != nil || r == nil {
				return nil, fmt.Errorf("create guest role: %w", err) // want `after it was reassigned inside its err != nil guard`
			}
		}
	}
	return r, nil
}

func badPlainUnrecheckedWrap() error {
	err := work()
	if err != nil {
		err = cleanup()
		return fmt.Errorf("op failed: %w", err) // want `after it was reassigned inside its err != nil guard`
	}
	return nil
}

func badBareReturn() error {
	err := work()
	if err != nil {
		err = cleanup()
		return err // want `after it was reassigned inside its err != nil guard`
	}
	return nil
}

func badErrorsJoinUnrechecked() error {
	err := work()
	if err != nil {
		err = cleanup()
		return errors.Join(err) // want `after it was reassigned inside its err != nil guard`
	}
	return nil
}

// --- Should NOT be flagged ---

func goodNoReassign() error {
	err := work()
	if err != nil {
		return fmt.Errorf("work failed: %w", err)
	}
	return nil
}

func goodRecheckedWrap() error {
	err := work()
	if err != nil {
		err = cleanup()
		if err != nil {
			return fmt.Errorf("cleanup failed: %w", err)
		}
	}
	return nil
}

func goodRecheckInInit() error {
	err := work()
	if err != nil {
		if err = cleanup(); err != nil {
			return fmt.Errorf("cleanup failed: %w", err)
		}
	}
	return nil
}

func goodRecheckWithConjunction(fatal bool) error {
	err := work()
	if err != nil {
		err = cleanup()
		if err != nil && fatal {
			return fmt.Errorf("cleanup failed: %w", err)
		}
	}
	return nil
}

func goodElseOfNilCheck() error {
	err := work()
	if err != nil {
		err = cleanup()
		if err == nil {
			return nil
		} else {
			return fmt.Errorf("cleanup failed: %w", err)
		}
	}
	return nil
}

func goodUseBeforeReassign() error {
	err := work()
	if err != nil {
		wrapped := fmt.Errorf("work failed: %w", err)
		err = cleanup()
		if err != nil {
			return errors.Join(wrapped, err)
		}
		return wrapped
	}
	return nil
}

func goodJustifiedAbove() error {
	err := work()
	if err != nil {
		err = cleanup()
		// justified: the cleanup error intentionally replaces the original
		return fmt.Errorf("cleanup: %w", err)
	}
	return nil
}

func badNolintDoesNotSuppress() error {
	err := work()
	if err != nil {
		err = cleanup()
		//nolint:safeerror -- retired bypass
		return fmt.Errorf("cleanup: %w", err) // want `after it was reassigned inside its err != nil guard`
	}
	return nil
}
