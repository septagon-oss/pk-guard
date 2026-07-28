// discard.go — the discarded-error sub-check: flags any assignment where
// a blank identifier `_` receives a value whose static type is `error`,
// in ANY position of the assignment (not just the last):
//
//	_ = svc.Flush(ctx)              // flagged
//	_, _ = s.orders.Delete(ctx, id) // flagged (second _ is an error)
//	n, _ := w.Write(b)              // flagged
//
// The exact `// justified: <reason>` comment is recognized on the
// same line or the line above.
//
// Precision limits (deliberate, documented):
//   - Only values statically typed as the built-in `error` interface are
//     matched; concrete named error types are not.
//   - Package-level `var _ = f()` declarations are out of scope (they are
//     side-effect imports by convention, not runtime error paths).
//   - Test support and generated files are exempt (see checks.go).
//
package safeerror

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// runDiscardCheck walks one file and reports blank-identifier discards
// of error-typed values.
func runDiscardCheck(pass *analysis.Pass, file *ast.File) { //nolint:gocognit // tuple and pairwise assignment forms are intentionally checked fail-closed
	ast.Inspect(file, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}

		// Multi-value form: x, _ := f() — one RHS call returning a tuple.
		if len(as.Rhs) == 1 && len(as.Lhs) > 1 {
			tup, ok := pass.TypesInfo.TypeOf(as.Rhs[0]).(*types.Tuple)
			if !ok || tup == nil {
				return true
			}
			for i, lhs := range as.Lhs {
				if i >= tup.Len() {
					break
				}
				if isBlankIdent(lhs) && isErrorType(tup.At(i).Type()) {
					reportDiscard(pass, file, lhs)
				}
			}
			return true
		}

		// Pairwise form: _ = err, or a, _ = x, f().
		if len(as.Lhs) == len(as.Rhs) {
			for i, lhs := range as.Lhs {
				if isBlankIdent(lhs) && isErrorType(pass.TypesInfo.TypeOf(as.Rhs[i])) {
					reportDiscard(pass, file, lhs)
				}
			}
		}
		return true
	})
}

func reportDiscard(pass *analysis.Pass, file *ast.File, at ast.Expr) {
	reportSubCheck(pass, file, checkNameDiscard, at.Pos(),
		"error result silently discarded via blank identifier; handle or propagate it, or annotate the line with // justified: <reason>")
}

// isBlankIdent reports whether the expression is the blank identifier.
func isBlankIdent(e ast.Expr) bool {
	ident, ok := e.(*ast.Ident)
	return ok && ident.Name == "_"
}
