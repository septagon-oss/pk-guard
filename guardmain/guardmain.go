// Package guardmain composes go/analysis analyzers into a single vettool
// binary, so a repository — or a client overlay extending one — declares its
// guard set in a five-line main:
//
//	package main
//
//	import (
//		"github.com/septagon-oss/pk-guard/guardmain"
//		"github.com/acme/platform/guards"          // estate-specific analyzers
//	)
//
//	func main() {
//		guardmain.Run(append(guardmain.Std(), guards.All()...)...)
//	}
//
// The resulting binary is a standard vettool: run it directly on packages
// (`./guard ./...`) or through `go vet -vettool=./guard ./...`. Each
// analyzer keeps its own flags (allowlists, catalog paths), so composition
// never hides configuration.
//
// # Extension contract
//
// Producers extend by shipping analyzers; consumers extend by composing
// mains. Consumers may add analyzers freely — tightening is always local —
// but loosening a producer's guard happens only through that guard's own
// allowlist or justification comment, where the exception is written down
// and visible. There is no flag to disable an analyzer wholesale: a guard
// you can switch off in build configuration is a guard that silently
// stopped running.
package guardmain

import (
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/multichecker"

	"github.com/septagon-oss/pk-guard/analyzers/buildtags"
	"github.com/septagon-oss/pk-guard/analyzers/importboundary"
	"github.com/septagon-oss/pk-guard/analyzers/noclockindomain"
	"github.com/septagon-oss/pk-guard/analyzers/safeerror"
)

// Std returns the standard pk-guard analyzer set. The slice is a fresh copy;
// callers may append their own analyzers without affecting other callers.
//
// The set is deliberately small and generic. Module-topology analyzers
// (importboundary, noclockindomain) are inert until the repository provides
// a module catalog — on repositories without one they report nothing, so
// including them is always safe.
func Std() []*analysis.Analyzer {
	return []*analysis.Analyzer{
		buildtags.Analyzer,
		importboundary.Analyzer,
		noclockindomain.Analyzer,
		safeerror.Analyzer,
	}
}

// Run starts the composed vettool. It follows multichecker semantics:
// process flags, analyze the named packages, exit non-zero on findings.
func Run(analyzers ...*analysis.Analyzer) {
	multichecker.Main(analyzers...)
}
