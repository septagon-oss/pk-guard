package importboundary_test

// analysistest coverage for the importboundary analyzer: own-subpackage and
// shared-package imports pass, published contracts pass, any other
// cross-module path is rejected, and allowlisted files are exempt.

import (
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/septagon-oss/pk-guard/analyzers/importboundary"
)

func TestAnalyzer(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	testdata := filepath.Join(filepath.Dir(thisFile), "testdata")

	for flag, value := range map[string]string{
		"catalog":   filepath.Join(testdata, "catalog.yaml"),
		"allowlist": filepath.Join(testdata, "allowlist.txt"),
	} {
		if err := importboundary.Analyzer.Flags.Set(flag, value); err != nil {
			t.Fatalf("set %s flag: %v", flag, err)
		}
	}
	t.Cleanup(func() {
		_ = importboundary.Analyzer.Flags.Set("catalog", "")
		_ = importboundary.Analyzer.Flags.Set("allowlist", "")
	})

	analysistest.Run(
		t, testdata, importboundary.Analyzer,
		"github.com/acme/modules/user_management/features/users",
		"github.com/acme/modules/billing/features/invoices",
	)
}
