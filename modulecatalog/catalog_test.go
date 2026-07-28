package modulecatalog_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/septagon-oss/pk-guard/modulecatalog"
)

func writeCatalog(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "module_contracts.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write catalog: %v", err)
	}
	return path
}

func TestLoadAndQuery(t *testing.T) {
	path := writeCatalog(t, `
modulePrefix: github.com/acme/platform/modules
modules:
  - id: alpha_management
  - id: web3_payments
  - id: auth_push_approval
`)
	cat, err := modulecatalog.Load(path, "")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	cases := map[string]bool{
		"alpha_management":   true,
		"web3_payments":      true,
		"auth_push_approval": true,
		"unknown_management": false,
		"":                   false,
	}
	for name, want := range cases {
		if got := cat.IsModule(name); got != want {
			t.Errorf("IsModule(%q) = %v, want %v", name, got, want)
		}
	}

	pkgCases := map[string]string{
		"github.com/acme/platform/modules/alpha_management":             "alpha_management",
		"github.com/acme/platform/modules/alpha_management/features/x":  "alpha_management",
		"github.com/acme/platform/modules/auth_push_approval/contracts": "auth_push_approval",
		"github.com/acme/platform/modules/ports":                        "",
		"github.com/acme/platform/modules":                              "",
		"github.com/example/elsewhere":                                  "",
	}
	for pkg, want := range pkgCases {
		if got := cat.ModuleForPackage(pkg); got != want {
			t.Errorf("ModuleForPackage(%q) = %q, want %q", pkg, got, want)
		}
	}
}

// The prefix may come from the analyzer flag instead of the file, but the
// file wins when both are present — it travels with the code it describes.
func TestPrefixResolution(t *testing.T) {
	declared := writeCatalog(t, `
modulePrefix: github.com/acme/declared
modules:
  - id: alpha
`)
	cat, err := modulecatalog.Load(declared, "github.com/acme/fallback")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := cat.Prefix(); got != "github.com/acme/declared" {
		t.Errorf("declared prefix should win over fallback, got %q", got)
	}

	undeclared := writeCatalog(t, `
modules:
  - id: alpha
`)
	cat, err = modulecatalog.Load(undeclared, "github.com/acme/fallback")
	if err != nil {
		t.Fatalf("load with fallback: %v", err)
	}
	if got := cat.Prefix(); got != "github.com/acme/fallback" {
		t.Errorf("fallback prefix should apply when file declares none, got %q", got)
	}

	if _, err := modulecatalog.Load(undeclared, ""); err == nil {
		t.Fatal("a catalog with no prefix from either source must be rejected, or every boundary silently passes")
	}
}

func TestSharedAndContractsConventions(t *testing.T) {
	path := writeCatalog(t, `
modulePrefix: github.com/acme/m
contractsSegment: api/public
sharedPackages: [kit, testfixtures]
modules:
  - id: alpha
`)
	cat, err := modulecatalog.Load(path, "")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := cat.ContractsSegment(); got != "api/public" {
		t.Errorf("ContractsSegment = %q, want api/public", got)
	}
	for rel, want := range map[string]bool{
		"kit":              true,
		"kit/sub":          true,
		"testfixtures":     true,
		"ports":            false, // defaults are replaced, not merged, when declared
		"alpha/features/x": false,
	} {
		if got := cat.IsShared(rel); got != want {
			t.Errorf("IsShared(%q) = %v, want %v", rel, got, want)
		}
	}

	defaults := writeCatalog(t, `
modulePrefix: github.com/acme/m
modules:
  - id: alpha
`)
	cat, err = modulecatalog.Load(defaults, "")
	if err != nil {
		t.Fatalf("load defaults: %v", err)
	}
	if got := cat.ContractsSegment(); got != modulecatalog.DefaultContractsSegment {
		t.Errorf("default ContractsSegment = %q, want %q", got, modulecatalog.DefaultContractsSegment)
	}
	if !cat.IsShared("ports") || !cat.IsShared("internal") || !cat.IsShared("testutil") {
		t.Error("default shared packages should apply when the catalog declares none")
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := modulecatalog.Load("/nonexistent/path/module_contracts.yaml", "")
	if !errors.Is(err, modulecatalog.ErrCatalogNotFound) {
		t.Fatalf("expected ErrCatalogNotFound, got %v", err)
	}
}

func TestLoadEmptyCatalog(t *testing.T) {
	path := writeCatalog(t, `modules: []`)
	if _, err := modulecatalog.Load(path, "x"); err == nil {
		t.Fatalf("expected error for empty catalog, got nil")
	}
}

func TestLoadInvalidEntry(t *testing.T) {
	path := writeCatalog(t, `
modulePrefix: github.com/acme/m
modules:
  - id: ""
`)
	if _, err := modulecatalog.Load(path, ""); err == nil {
		t.Fatalf("expected error for empty module id, got nil")
	}
}

func TestResolverCachesCatalog(t *testing.T) {
	path := writeCatalog(t, `
modulePrefix: github.com/acme/m
modules:
  - id: alpha_management
`)
	r := modulecatalog.NewResolver(path, "")
	first, err := r.Get()
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	second, err := r.Get()
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if first != second {
		t.Errorf("Resolver should return identical Catalog instance on repeated Get")
	}
}

// Codex-review regression: a nested module ID would load fine and then
// never match ModuleForPackage, silently blinding every topology guard.
func TestNestedModuleIDsAreRejected(t *testing.T) {
	path := writeCatalog(t, `
modulePrefix: github.com/acme/m
modules:
  - id: commerce/billing
`)
	if _, err := modulecatalog.Load(path, ""); err == nil {
		t.Fatal("a module id containing '/' must be rejected at load, not silently ignored at match time")
	}
}

// Codex-review regression: an explicitly empty sharedPackages list means
// "nothing is shared" and must not be conflated with an omitted field.
func TestExplicitlyEmptySharedPackagesGrantsNothing(t *testing.T) {
	path := writeCatalog(t, `
modulePrefix: github.com/acme/m
sharedPackages: []
modules:
  - id: alpha
`)
	cat, err := modulecatalog.Load(path, "")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	for _, rel := range []string{"ports", "internal", "testutil"} {
		if cat.IsShared(rel) {
			t.Errorf("explicitly empty sharedPackages must not grant default %q", rel)
		}
	}
}
