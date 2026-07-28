// Package safeerror defines a go/analysis analyzer that detects raw
// err.Error() strings passed to HTTP error constructors. These leak
// internal implementation details (DB column names, file paths, stack
// traces) to API clients.
//
// Flagged patterns:
//
//	huma.NewError(status, err.Error())
//	huma.Error400BadRequest(err.Error())
//	apperrors.Error404NotFound(err.Error())
//	... any function matching Error{NNN}{Name}(err.Error(), ...)
//
// Suggested fix: use apperrors.SafeError(http.StatusXxx, err) instead.
//
// # Silent-failure checks
//
// Beyond the raw err.Error() leak check, the analyzer always enforces three
// named checks for one discipline: no silent failures in production paths.
//
//   - discarded-error (see discard.go)
//   - log-without-return (see lognoreturn.go)
//   - wrap-after-reassign (see wrapreassign.go)
//
// There are no opt-out flags or file baselines. The three silent-failure
// checks recognize the exact `// justified: <reason>` review contract
// on the flagged line or the line immediately above it. Raw client-error
// leaks have no suppression. Test files, test-support files importing
// `testing`, and generated files (ast.IsGenerated) are exempt.
package safeerror

import (
	"errors"
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

var Analyzer = &analysis.Analyzer{
	Name:     "safeerror",
	Doc:      "detects silent error failures: raw err.Error() leaks, discarded error results, log-without-return, and wrap-after-reassign",
	Run:      run,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
}

// humaNewError matches huma.NewError calls.
const humaNewError = "NewError"

// errorFuncPrefixes are function name prefixes that construct HTTP errors.
// Matches: Error400BadRequest, Error404NotFound, Error500InternalServerError, etc.
const errorFuncPrefix = "Error"

// qualifiedPackageSuffixes are the import path suffixes we care about.
// We match any package whose path ends with one of these.
var qualifiedPackageSuffixes = []string{
	"danielgtaylor/huma/v2",
	"platformkit-backend-kit/app/errors",
}

func run(pass *analysis.Pass) (any, error) {
	insp, ok := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	if !ok || insp == nil {
		return nil, errors.New("safeerror: inspect analyzer result unavailable")
	}

	for _, file := range pass.Files {
		if skipFileForSubChecks(pass, file) {
			continue
		}
		runDiscardCheck(pass, file)
		runLogNoReturnCheck(pass, file)
		runWrapReassignCheck(pass, file)
	}

	nodeFilter := []ast.Node{
		(*ast.CallExpr)(nil),
	}

	insp.Preorder(nodeFilter, func(n ast.Node) {
		call, ok := n.(*ast.CallExpr)
		if !ok || call == nil {
			return
		}

		funcName, pkgPath := resolveCallTarget(pass, call)
		if funcName == "" || pkgPath == "" {
			return
		}

		if !isTargetPackage(pkgPath) {
			return
		}

		if !isErrorConstructor(funcName) {
			return
		}

		if len(call.Args) == 0 {
			return
		}

		msgArgIndex := 0
		if funcName == humaNewError {
			msgArgIndex = 1
		}
		if len(call.Args) <= msgArgIndex {
			return
		}

		if isErrorMethodCall(pass, call.Args[msgArgIndex]) {
			pass.Reportf(
				call.Pos(),
				"raw err.Error() passed to %s leaks internal details to API clients; use apperrors.SafeError(status, err) instead",
				funcName,
			)
			return
		}
	})

	return nil, nil
}

// resolveCallTarget extracts the function name and package path from a call expression.
func resolveCallTarget(pass *analysis.Pass, call *ast.CallExpr) (funcName string, pkgPath string) {
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		funcName = fn.Sel.Name

		// Check if it's a package-level function call (pkg.Func).
		if ident, ok := fn.X.(*ast.Ident); ok {
			obj := pass.TypesInfo.Uses[ident]
			if obj == nil {
				return "", ""
			}
			if pkgName, ok := obj.(*types.PkgName); ok {
				return funcName, pkgName.Imported().Path()
			}
		}

		// Also handle the case where type info resolves the function directly.
		if sel, ok := pass.TypesInfo.Selections[fn]; ok {
			if sel.Obj() != nil && sel.Obj().Pkg() != nil {
				return funcName, sel.Obj().Pkg().Path()
			}
		}

		// Try ObjectOf for the selector.
		obj := pass.TypesInfo.ObjectOf(fn.Sel)
		if obj != nil && obj.Pkg() != nil {
			return funcName, obj.Pkg().Path()
		}

	case *ast.Ident:
		// Direct function call (imported via dot import or same package).
		funcName = fn.Name
		obj := pass.TypesInfo.Uses[fn]
		if obj != nil && obj.Pkg() != nil {
			return funcName, obj.Pkg().Path()
		}
	}

	return "", ""
}

// isTargetPackage checks if the package path is one we should analyze.
func isTargetPackage(pkgPath string) bool {
	for _, suffix := range qualifiedPackageSuffixes {
		if strings.HasSuffix(pkgPath, suffix) {
			return true
		}
	}
	return false
}

// isErrorConstructor checks if the function name matches an HTTP error constructor pattern.
func isErrorConstructor(name string) bool {
	if name == humaNewError {
		return true
	}
	// Match Error400BadRequest, Error404NotFound, Error500InternalServerError, etc.
	if !strings.HasPrefix(name, errorFuncPrefix) {
		return false
	}
	rest := name[len(errorFuncPrefix):]
	if len(rest) < 4 {
		return false
	}
	// Check that the next 3 characters are digits (HTTP status code).
	for _, c := range rest[:3] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// isErrorMethodCall checks if an expression is a call to .Error() on an error value.
func isErrorMethodCall(pass *analysis.Pass, expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return false
	}

	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Error" {
		return false
	}

	// Verify the receiver implements the error interface using type information.
	receiverType := pass.TypesInfo.TypeOf(sel.X)
	if receiverType == nil {
		return false
	}

	errorObj := types.Universe.Lookup("error")
	if errorObj == nil {
		return false
	}
	errorIface, ok := errorObj.Type().Underlying().(*types.Interface)
	if !ok || errorIface == nil {
		return false
	}
	return types.Implements(receiverType, errorIface) ||
		types.Implements(types.NewPointer(receiverType), errorIface)
}
