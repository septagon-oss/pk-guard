// wrapreassign.go — the wrap-after-reassign sub-check: inside a block
// guarded by `if err != nil`, flags a wrap or bare return of err that
// happens AFTER err was reassigned in that block, unless every path to
// the wrap re-establishes that the NEW err is non-nil. The production
// incident this encodes (auth_management guest_roles.go):
//
//	role, err := b.roles.GetByName(...)
//	if err != nil { ... }
//	role, err = b.perms.CreateRole(...)
//	if err != nil {                                   // guard
//	    role, err = b.roles.GetByName(...)            // REASSIGNED
//	    if err != nil || role == nil {                // OR-recheck!
//	        return fmt.Errorf("create guest role: %w", err) // flagged:
//	    }                                             // reachable with
//	}                                                 // err == nil
//
// The original failure is destroyed and the log shows %!w(<nil>).
//
// What counts as a wrap/return use of err:
//   - a bare `return err` (err among the results);
//   - fmt.Errorf("...%w...", ..., err) (non-literal formats count too);
//   - errors.Wrap/Wrapf/WithMessage/WithMessagef/Join(..., err) from any
//     errors-flavored package (errors, github.com/pkg/errors, xerrors).
//
// A use is considered safe only when an enclosing branch condition
// REQUIRES the reassigned err to be non-nil:
//   - then-branch of `err != nil` or of `err != nil && X`;
//   - else-branch of `err == nil` (and De Morgan equivalents).
//
// Per the incident, `err != nil || X` does NOT guarantee — the use is
// reachable through X with a nil err — and is flagged.
//
// The exact `// justified: <reason>` comment is recognized on the
// flagged line or line above.
//
// Precision limits (deliberate, documented):
//   - Ordering is positional (token position), not a real dominator
//     analysis: a reassignment in one branch and a use in a later
//     sibling branch is flagged even when the paths are exclusive.
//     Use the exact justification comment when the flow is
//     provably safe.
//   - Only plain `=` reassignments are tracked; `:=` shadowing inside
//     nested blocks is out of scope (go vet's shadow checks cover it,
//     and tracking it positionally would misattribute sibling scopes).
//   - err is matched by name within the guard body (shadow-tolerant on
//     the wrap side by design: wrapping the shadowing err is the bug).
//   - Function literals inside the guard body are not analyzed.
//   - Loops are handled positionally; a use textually before a
//     reassignment inside the same loop body is not flagged.
//
package safeerror

import (
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// wrapFuncNames are errors-package helpers that wrap an error argument.
var wrapFuncNames = map[string]bool{
	"Wrap": true, "Wrapf": true,
	"WithMessage": true, "WithMessagef": true,
	"Join": true,
}

// runWrapReassignCheck walks one file, finds err != nil guard blocks,
// and reports wraps/returns of err that follow a reassignment of err
// inside the same guard without a dominating non-nil recheck.
func runWrapReassignCheck(pass *analysis.Pass, file *ast.File) {
	// Nested plain err != nil guards can both observe the same
	// reassign+wrap pair; report each use position once.
	seen := map[token.Pos]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		ifs, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		errIdent := errNilCondIdent(pass, ifs.Cond)
		if errIdent == nil {
			return true
		}
		checkGuardBlock(pass, file, ifs, errIdent.Name, seen)
		return true
	})
}

// checkGuardBlock analyzes one `if err != nil { ... }` body.
func checkGuardBlock(pass *analysis.Pass, file *ast.File, guard *ast.IfStmt, errName string, seen map[token.Pos]bool) { //nolint:gocognit,gocyclo // branch dominance is intentionally explicit for this analyzer
	body := guard.Body
	parents := buildParents(body)

	// 1. Earliest plain `=` reassignment of errName in the body
	//    (function literals excluded).
	firstReassign := token.NoPos
	ast.Inspect(body, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		as, ok := n.(*ast.AssignStmt)
		if !ok || as.Tok != token.ASSIGN {
			return true
		}
		for _, lhs := range as.Lhs {
			if ident, ok := lhs.(*ast.Ident); ok && ident.Name == errName {
				if firstReassign == token.NoPos || as.Pos() < firstReassign {
					firstReassign = as.Pos()
				}
			}
		}
		return true
	})
	if firstReassign == token.NoPos {
		return
	}

	// 2. Wrap/return uses of errName positioned after the reassignment.
	ast.Inspect(body, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		switch v := n.(type) {
		case *ast.ReturnStmt:
			for _, res := range v.Results {
				if ident, ok := res.(*ast.Ident); ok && ident.Name == errName && ident.Pos() > firstReassign {
					flagIfUnguarded(pass, file, parents, body, ident, "bare return of", seen)
				}
			}
		case *ast.CallExpr:
			if !isWrapCall(pass, v) {
				return true
			}
			for _, arg := range v.Args {
				ast.Inspect(arg, func(a ast.Node) bool {
					if _, ok := a.(*ast.FuncLit); ok {
						return false
					}
					if ident, ok := a.(*ast.Ident); ok && ident.Name == errName && ident.Pos() > firstReassign {
						flagIfUnguarded(pass, file, parents, body, ident, "wrap of", seen)
					}
					return true
				})
			}
		}
		return true
	})
}

// flagIfUnguarded reports the use unless some enclosing branch between
// the use and the guard body requires errName != nil.
func flagIfUnguarded(pass *analysis.Pass, file *ast.File, parents map[ast.Node]ast.Node, root ast.Node, use *ast.Ident, useKind string, seen map[token.Pos]bool) {
	if seen[use.Pos()] {
		return
	}
	child := ast.Node(use)
	for cur := parents[use]; cur != nil; child, cur = cur, parents[cur] {
		ifs, ok := cur.(*ast.IfStmt)
		if !ok {
			if cur == root {
				break
			}
			continue
		}
		switch child {
		case ast.Node(ifs.Body):
			if condRequiresNonNil(ifs.Cond, use.Name) {
				return // dominated by a real non-nil recheck
			}
		case ifs.Else:
			if negRequiresNonNil(ifs.Cond, use.Name) {
				return // else of `err == nil` (or equivalent)
			}
		}
	}
	seen[use.Pos()] = true
	reportSubCheck(pass, file, checkNameWrapReassign, use.Pos(),
		"%s %q after it was reassigned inside its err != nil guard; the original failure is lost and the reassigned %q can be nil here (%%!w(<nil>)) — re-check the new error or wrap the original",
		useKind, use.Name, use.Name)
}

// condRequiresNonNil reports whether the condition being TRUE guarantees
// name != nil. Conjunctions require either side; disjunctions require
// BOTH sides (err != nil || X is reachable with err == nil).
func condRequiresNonNil(cond ast.Expr, name string) bool {
	switch e := cond.(type) {
	case *ast.ParenExpr:
		return condRequiresNonNil(e.X, name)
	case *ast.UnaryExpr:
		if e.Op == token.NOT {
			return negRequiresNonNil(e.X, name)
		}
	case *ast.BinaryExpr:
		switch e.Op {
		case token.NEQ:
			return isNamedIdent(e.X, name) && isNilIdent(e.Y) ||
				isNamedIdent(e.Y, name) && isNilIdent(e.X)
		case token.LAND:
			return condRequiresNonNil(e.X, name) || condRequiresNonNil(e.Y, name)
		case token.LOR:
			return condRequiresNonNil(e.X, name) && condRequiresNonNil(e.Y, name)
		}
	}
	return false
}

// negRequiresNonNil reports whether the condition being FALSE guarantees
// name != nil (i.e. the else-branch is err != nil).
func negRequiresNonNil(cond ast.Expr, name string) bool {
	switch e := cond.(type) {
	case *ast.ParenExpr:
		return negRequiresNonNil(e.X, name)
	case *ast.UnaryExpr:
		if e.Op == token.NOT {
			return condRequiresNonNil(e.X, name)
		}
	case *ast.BinaryExpr:
		switch e.Op {
		case token.EQL:
			return isNamedIdent(e.X, name) && isNilIdent(e.Y) ||
				isNamedIdent(e.Y, name) && isNilIdent(e.X)
		case token.LOR: // ¬(A ∨ B) = ¬A ∧ ¬B — either side suffices
			return negRequiresNonNil(e.X, name) || negRequiresNonNil(e.Y, name)
		case token.LAND: // ¬(A ∧ B) = ¬A ∨ ¬B — both sides must guarantee
			return negRequiresNonNil(e.X, name) && negRequiresNonNil(e.Y, name)
		}
	}
	return false
}

// isWrapCall reports whether the call wraps an error: fmt.Errorf with a
// %w verb (or a non-literal format), or errors.Wrap*/WithMessage*/Join
// from an errors-flavored package.
func isWrapCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkgPath := resolvedPkgPath(pass, sel.X)
	if pkgPath == "" {
		return false
	}
	if pkgPath == "fmt" && sel.Sel.Name == "Errorf" {
		if len(call.Args) == 0 {
			return false
		}
		if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
			return strings.Contains(lit.Value, "%w")
		}
		return true // non-literal format: assume it may wrap
	}
	if pkgPath == "errors" || strings.HasSuffix(pkgPath, "/errors") || strings.HasSuffix(pkgPath, "xerrors") {
		return wrapFuncNames[sel.Sel.Name]
	}
	return false
}

// isNamedIdent reports whether the expression is an identifier with the
// given name.
func isNamedIdent(e ast.Expr, name string) bool {
	ident, ok := e.(*ast.Ident)
	return ok && ident.Name == name
}
