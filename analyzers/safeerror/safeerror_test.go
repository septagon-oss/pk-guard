package safeerror_test

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/septagon-oss/pk-guard/analyzers/safeerror"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func testdataDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	return filepath.Join(filepath.Dir(thisFile), "testdata")
}

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, testdataDir(t), safeerror.Analyzer, "example")
}

func TestDiscardedError(t *testing.T) {
	analysistest.Run(t, testdataDir(t), safeerror.Analyzer, "discardcheck")
}

func TestLogWithoutReturn(t *testing.T) {
	analysistest.Run(t, testdataDir(t), safeerror.Analyzer, "lognoreturncheck")
}

func TestWrapAfterReassign(t *testing.T) {
	analysistest.Run(t, testdataDir(t), safeerror.Analyzer, "wrapreassigncheck")
}

func TestAnalyzerFailsClosedWithoutInspectorResult(t *testing.T) {
	_, err := safeerror.Analyzer.Run(&analysis.Pass{ResultOf: map[*analysis.Analyzer]any{}})
	if err == nil || !strings.Contains(err.Error(), "inspect analyzer result unavailable") {
		t.Fatalf("Analyzer.Run() error = %v, want missing-inspector failure", err)
	}
}

// The per-check enable/disable flags and the baseline flag were retired in
// the analyzer's original home: selectively disabling checks let debt hide
// behind build configuration. The analyzer runs whole or not at all;
// exceptions go through the allowlist, where they are visible and reasoned.
func TestAnalyzerHasNoCompatibilityFlagsOrBaseline(t *testing.T) {
	for _, flagName := range []string{"discard", "lognoreturn", "wrapreassign", "baseline"} {
		if safeerror.Analyzer.Flags.Lookup(flagName) != nil {
			t.Fatalf("retired compatibility flag %q returned", flagName)
		}
	}
}
