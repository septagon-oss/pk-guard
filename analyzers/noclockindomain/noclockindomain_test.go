package noclockindomain_test

// analysistest coverage for the noclockindomain analyzer: flagged
// wall-clock/randomness calls, injected-clock allowance, non-service-file
// scope, and module allowlisting.

import (
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/septagon-oss/pk-guard/analyzers/noclockindomain"
)

func TestAnalyzer(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	testdata := filepath.Join(filepath.Dir(thisFile), "testdata")

	// The allowlist tolerates legacy_clock_management (ratcheted debt) so
	// its fixture produces no diagnostics.
	if err := noclockindomain.Analyzer.Flags.Set("allowlist", filepath.Join(testdata, "allowlist.txt")); err != nil {
		t.Fatalf("set allowlist flag: %v", err)
	}
	if err := noclockindomain.Analyzer.Flags.Set("catalog", filepath.Join(testdata, "catalog.yaml")); err != nil {
		t.Fatalf("set catalog flag: %v", err)
	}
	t.Cleanup(func() {
		_ = noclockindomain.Analyzer.Flags.Set("allowlist", "")
		_ = noclockindomain.Analyzer.Flags.Set("catalog", "")
	})

	analysistest.Run(
		t, testdata, noclockindomain.Analyzer,
		"github.com/acme/modules/clocky_management/features/widgets",
		"github.com/acme/modules/clocky_management/rootsvc",
		"github.com/acme/modules/legacy_clock_management/features/ledger",
	)
}
