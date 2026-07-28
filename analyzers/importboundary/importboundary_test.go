package importboundary_test

// analysistest coverage for the importboundary analyzer: own-subpackage and
// shared-package imports pass, published contracts pass, any other
// cross-module path is rejected, and allowlisted files are exempt.

import (
	"os"
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

// Codex-review regression: an allowlist line with an empty file suffix
// would HasSuffix-match every file — a typo becoming a repo-wide bypass.
func TestAllowlistRejectsEmptyFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "allowlist.txt")
	if err := os.WriteFile(path, []byte(" -> billing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := importboundary.LoadImportAllowlistForTest(path); err == nil {
		t.Fatal("an allowlist entry with an empty file suffix must be rejected")
	}
}
