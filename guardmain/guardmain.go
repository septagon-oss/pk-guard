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
// but loosening a producer's guard is designed to go through that guard's
// own allowlist or justification comment, where the exception is written
// down and visible.
//
// Honesty note: the underlying multichecker driver exposes a -NAME=false
// flag per analyzer, so an invocation CAN skip one — the protection is that
// doing so is visible in the invocation (build scripts, hooks, CI config),
// not buried in source. Review your guard invocation like you review code;
// pk-guard deliberately adds no quieter way to turn an analyzer off.
package guardmain

import (
	"fmt"
	"os"
	"strings"

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
//
// Duplicate analyzer names are rejected up front with a composition error
// naming both sides — the driver would otherwise panic deep in global flag
// registration, which tells the extender nothing about which analyzer to
// rename.
func Run(analyzers ...*analysis.Analyzer) {
	seen := make(map[string]*analysis.Analyzer, len(analyzers))
	for _, a := range analyzers {
		if a == nil {
			continue
		}
		if prev, dup := seen[a.Name]; dup && prev != a {
			fmt.Fprintf(os.Stderr,
				"pk-guard: composition error: two different analyzers are both named %q (%s / %s); rename one before composing\n",
				a.Name, oneLine(prev.Doc), oneLine(a.Doc))
			os.Exit(2)
		}
		seen[a.Name] = a
	}
	multichecker.Main(analyzers...)
}

// oneLine returns the first line of an analyzer doc for error messages.
func oneLine(doc string) string {
	if i := strings.IndexByte(doc, '\n'); i >= 0 {
		return doc[:i]
	}
	return doc
}
