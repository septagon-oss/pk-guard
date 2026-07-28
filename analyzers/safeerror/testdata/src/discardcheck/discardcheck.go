package discardcheck

// Implements: REQ-004 (test fixture).
// Per: ADR-0005.
// Discipline: C-14.

func del() error                    { return nil }
func del2() (int, error)            { return 0, nil }
func errFirst() (error, int)        { return nil, 0 }
func threeVals() (int, error, bool) { return 0, nil, false }

var m = map[string]int{}

// --- Should be flagged ---

func badSingle() {
	_ = del() // want `error result silently discarded`
}

func badMultiBothBlank() {
	_, _ = del2() // want `error result silently discarded`
}

func badErrorNotLast() {
	_, n := errFirst() // want `error result silently discarded`
	_ = n
}

func badMiddlePosition() {
	a, _, b := threeVals() // want `error result silently discarded`
	_, _ = a, b
}

func badPairwise() {
	var a int
	a, _ = 1, del() // want `error result silently discarded`
	_ = a
}

func badPlainErrVar() {
	err := del()
	_ = err // want `error result silently discarded`
}

// --- Should NOT be flagged ---

func okHandled() error {
	if err := del(); err != nil {
		return err
	}
	return nil
}

func okCommaOkNotError() {
	_, ok := m["k"]
	_ = ok
}

func badNolint() {
	//nolint:safeerror -- retired bypass
	_ = del() // want `error result silently discarded`
}

func badEmptyJustification() {
	// justified:
	_ = del() // want `error result silently discarded`
}

func badIncidentalJustificationText() {
	// This is justified: according to an unrelated prose sentence.
	_ = del() // want `error result silently discarded`
}

func okJustifiedSameLine() {
	_ = del() // justified: fire-and-forget notification per ADR-0005 carve-out
}

func okJustifiedLineAbove() {
	// justified: defer-time close on read-only stream
	_, _ = del2()
}
