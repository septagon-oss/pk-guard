// Package noclockindomain defines a go/analysis analyzer that flags direct
// wall-clock and randomness calls in module DOMAIN code — files matching
// features/*/service.go (and service_*.go) inside a cataloged module.
//
// Flagged patterns:
//
//	time.Now() / time.Since(...) / time.Until(...)
//	rand.<Fn>(...) from math/rand or math/rand/v2
//
// Rationale: domain math computed on raw time.Now() is untestable. The
// blessed pattern injects a Clock seam instead; an injected clock call such
// as s.clock.Now() is never flagged because its receiver is not the time
// package.
//
// Scope is deliberately narrow: only files named service.go or service_*.go
// under a features/<name>/ directory of a cataloged module are domain code
// for this rule. Handlers, feature wiring, repositories, module-root files,
// and _test.go files are not checked. Non-call references (e.g. the
// `now: time.Now` field-default injection idiom) are also not flagged — they
// ARE the injection pattern.
//
// Module topology comes from the modulecatalog file (-catalog / -modprefix).
//
// Allowlist: -noclockindomain.allowlist=<file> using the shared allowlist
// grammar (rule:subject|owner=...|until=...|reason=...). The rule is
// "clock_in_domain" and the subject is the module name, so existing
// offenders ratchet: allowlisted modules stay green, new modules are blocked.
package noclockindomain

import (
	"errors"
	"fmt"
	"go/ast"
	"go/types"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/tools/go/analysis"

	"github.com/septagon-oss/pk-guard/analyzers/allowlist"
	"github.com/septagon-oss/pk-guard/modulecatalog"
)

// Analyzer flags direct time.Now/Since/Until and rand.* calls in
// business-module domain service files.
var Analyzer = &analysis.Analyzer{
	Name: "noclockindomain",
	Doc: "flags direct time.Now/Since/Until and math/rand calls in " +
		"business-module feature service files; inject ports.Clock instead",
	Run: run,
}

var (
	allowlistPath string
	catalogPath   string
	modPrefix     string
)

func init() {
	Analyzer.Flags.StringVar(&allowlistPath, "allowlist", "",
		"path to the noclockindomain allowlist file")
	Analyzer.Flags.StringVar(&catalogPath, "catalog", "",
		"path to the module catalog YAML (defaults to "+modulecatalog.DefaultCatalogPath+")")
	Analyzer.Flags.StringVar(&modPrefix, "modprefix", "",
		"module import-path prefix when the catalog does not declare modulePrefix")
}

// resolver caches the loaded catalog across packages in one analyzer run.
// Bound from flags exactly once: the driver runs packages concurrently, and
// flags cannot change mid-process in a vettool.
var (
	resolverOnce sync.Once
	resolver     *modulecatalog.Resolver
)

func loadCatalog() (*modulecatalog.Catalog, error) {
	resolverOnce.Do(func() {
		resolver = modulecatalog.NewResolver(catalogPath, modPrefix)
	})
	return resolver.Get()
}

// ruleClockInDomain is the allowlist rule key. Subjects are business-module
// names (e.g. "billing_management").
const ruleClockInDomain = "clock_in_domain"

// timeFuncs are the time-package functions that read the wall clock.
var timeFuncs = map[string]bool{
	"Now":   true,
	"Since": true,
	"Until": true,
}

// randPackages are the randomness packages banned from domain code.
var randPackages = map[string]bool{
	"math/rand":    true,
	"math/rand/v2": true,
}

func run(pass *analysis.Pass) (any, error) {
	catalog, catErr := loadCatalog()
	switch {
	case errors.Is(catErr, modulecatalog.ErrCatalogNotFound):
		return nil, nil
	case catErr != nil:
		return nil, fmt.Errorf("noclockindomain: %w", catErr)
	}
	module := catalog.ModuleForPackage(pass.Pkg.Path())
	if module == "" {
		return nil, nil
	}

	entries, err := allowlist.Load(allowlistPath)
	if err != nil {
		return nil, fmt.Errorf("noclockindomain: %w", err)
	}
	if entries.IsAllowed(ruleClockInDomain, module) {
		return nil, nil
	}

	for _, file := range pass.Files {
		filename := pass.Fset.Position(file.Pos()).Filename
		if !isDomainServiceFile(filename) {
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			pkgName, ok := pass.TypesInfo.Uses[ident].(*types.PkgName)
			if !ok {
				// Not a package selector — e.g. s.clock.Now() on an
				// injected ports.Clock. Never flagged.
				return true
			}
			importPath := pkgName.Imported().Path()
			switch {
			case importPath == "time" && timeFuncs[sel.Sel.Name]:
				pass.Reportf(call.Pos(),
					"direct time.%s() in domain service code is untestable; "+
						"inject the shared ports.Clock (modules/platformkit-business-modules/ports/clock.go) "+
						"and call the injected clock instead (see billing_management/features/subscriptions WithClock)",
					sel.Sel.Name)
			case randPackages[importPath]:
				pass.Reportf(call.Pos(),
					"direct %s.%s() (%s) in domain service code is nondeterministic; "+
						"inject randomness behind a port (mirror the ports.Clock pattern) instead",
					ident.Name, sel.Sel.Name, importPath)
			}
			return true
		})
	}
	return nil, nil
}

// isDomainServiceFile reports whether filename is domain service code for
// this rule: a file named service.go or service_*.go directly under a
// features/<name>/ directory. _test.go files are never domain code.
func isDomainServiceFile(filename string) bool {
	base := filepath.Base(filename)
	if strings.HasSuffix(base, "_test.go") {
		return false
	}
	if base != "service.go" && !strings.HasPrefix(base, "service_") {
		return false
	}
	if !strings.HasSuffix(base, ".go") {
		return false
	}
	dir := filepath.ToSlash(filepath.Dir(filename))
	parts := strings.Split(dir, "/")
	// The file must sit directly inside features/<feature>/ — i.e. the
	// grandparent directory is "features".
	return len(parts) >= 2 && parts[len(parts)-2] == "features"
}
