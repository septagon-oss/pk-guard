package lognoreturncheck

// Implements: REQ-004 (test fixture).
// Per: ADR-0005.
// Discipline: C-14.

import (
	"errors"
	"fmt"
	"log"
)

type fakeLogger struct{}

func (fakeLogger) Warn(msg string, kv ...any)       {}
func (fakeLogger) Error(msg string, kv ...any)      {}
func (fakeLogger) Infof(format string, args ...any) {}
func (fakeLogger) Fatal(msg string, kv ...any)      {}
func (fakeLogger) With(kv ...any) fakeLogger        { return fakeLogger{} }

var lg fakeLogger

func work() error    { return nil }
func record(e error) {}

// --- Should be flagged ---

func badWarnOnly() {
	err := work()
	if err != nil { // want `is only logged here`
		lg.Warn("work failed", "error", err)
	}
}

func badErrorStringOnly() {
	err := work()
	if err != nil { // want `is only logged here`
		lg.Error("work failed", "error", err.Error())
	}
}

func badStdlibLogPrintf() {
	err := work()
	if err != nil { // want `is only logged here`
		log.Printf("work failed: %v", err)
	}
}

func badChainedLogger() {
	err := work()
	if err != nil { // want `is only logged here`
		lg.With("op", "rotate").Warn("rotate failed", "error", err)
	}
}

// --- Should NOT be flagged ---

func goodLogAndReturn() error {
	err := work()
	if err != nil {
		lg.Error("work failed", "error", err)
		return err
	}
	return nil
}

func goodLogAndPropagateVar() error {
	var failure error
	err := work()
	if err != nil {
		lg.Warn("work failed", "error", err)
		failure = err
	}
	return failure
}

func goodWrapIsPropagation() error {
	var wrapped error
	err := work()
	if err != nil {
		wrapped = fmt.Errorf("work: %w", err)
	}
	return wrapped
}

func goodReassignRetry() error {
	err := work()
	if err != nil {
		lg.Warn("retrying", "error", err)
		err = work()
	}
	return err
}

func goodPassedToNonLoggingCall() {
	err := work()
	if err != nil {
		lg.Warn("work failed", "error", err)
		record(err)
	}
}

func goodFatalTerminates() {
	err := work()
	if err != nil {
		lg.Fatal("work failed", "error", err)
	}
}

func goodPanicTerminates() {
	err := work()
	if err != nil {
		lg.Error("work failed", "error", err)
		panic(err)
	}
}

func goodContinueAcknowledges() {
	for i := 0; i < 3; i++ {
		err := work()
		if err != nil {
			lg.Warn("skipping row", "error", err)
			continue
		}
	}
}

func goodErrorsIsNested(sentinel error) {
	err := work()
	if err != nil {
		if errors.Is(err, sentinel) {
			lg.Warn("known condition", "error", err)
		}
	}
}

func goodNoLoggingAtAll() {
	err := work()
	if err != nil {
		record(err)
	}
}

func goodEmptyBodyOutOfScope() {
	err := work()
	if err != nil {
	}
}

func goodJustifiedAbove() {
	err := work()
	// justified: best-effort cache invalidation, staleness self-heals
	if err != nil {
		lg.Warn("cache invalidation failed", "error", err)
	}
}

func badNolintDoesNotSuppress() {
	err := work()
	//nolint:safeerror -- retired bypass
	if err != nil { // want `is only logged here`
		lg.Warn("metrics emit failed", "error", err)
	}
}
