// lognoreturn.go — the log-without-return sub-check: flags an
// `if err != nil { ... }` whose body acknowledges the error only by
// logging it and then lets the failure path continue as success:
//
//	if err != nil {
//	    logger.Warn("rotate failed", "error", err) // flagged: no return,
//	}                                              // no propagation
//
// The body is considered safe (not flagged) when it contains any of:
//   - a return, panic, os.Exit/Fatal*/Goexit, break/continue/goto;
//   - a reassignment of err (retry loops, error joins);
//   - any use of err outside a logging call (passed to a non-logging
//     function, stored in a struct/variable/channel, compared via
//     errors.Is inside a nested condition, ...).
//
// Logging calls are recognized by method name (the workspace Logger
// interface in core/platformkit-backend-kit/observability — Debug/Info/
// Warn/Error plus the f/w/ln/Context variants used by slog, zap, logrus
// and zerolog), on any receiver that is not itself an error and not the
// fmt/errors packages (so fmt.Errorf and errors.Wrap count as
// propagation, not logging).
//
// The exact `// justified: <reason>` comment is recognized on the `if`
// line or the line above; a bare marker with no reason does not count.
//
// Precision limits (deliberate, documented):
//   - Only plain `err != nil` conditions on error-typed identifiers are
//     analyzed; compound conditions (err != nil && retryable) are skipped.
//   - `if err != nil {}` with an EMPTY body (the tink RotateKey incident:
//     a comment claiming "log and continue" with no log call) is NOT
//     flagged — there is no logging use to anchor on. Empty guards are a
//     separate smell.
//   - err passed to a non-logging call nested inside a logging call's
//     arguments (logger.Warn(save(err))) counts as a logging use; the
//     escape is // justified:.
//   - Function literals inside the body are not analyzed (different
//     control flow).
package safeerror

import (
	"go/ast"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// logMethodNames are method names treated as logging when invoked via a
// selector on a non-error, non-fmt/errors receiver. Derived from the
// workspace Logger interface (Debug/Info/Warn/Error + structured
// variants) and the common stdlib/zap/logrus/zerolog spellings.
var logMethodNames = map[string]bool{
	"Trace": true, "Tracef": true,
	"Debug": true, "Debugf": true, "Debugw": true, "Debugln": true, "DebugContext": true,
	"Info": true, "Infof": true, "Infow": true, "Infoln": true, "InfoContext": true,
	"Warn": true, "Warnf": true, "Warnw": true, "Warnln": true, "WarnContext": true,
	"Warning": true, "Warningf": true, "Warningln": true,
	"Error": true, "Errorf": true, "Errorw": true, "Errorln": true, "ErrorContext": true,
	"Log": true, "Logf": true, "Msg": true, "Msgf": true,
}

// logPrintNames are logging only when called on the stdlib log package
// (log.Print family); on anything else Print* is too ambiguous.
var logPrintNames = map[string]bool{
	"Print": true, "Printf": true, "Println": true,
}

// terminatorFuncNames are call names that end the failure path just as
// hard as a return does.
var terminatorFuncNames = map[string]bool{
	"Fatal": true, "Fatalf": true, "Fatalln": true, "Fatalw": true,
	"Panic": true, "Panicf": true, "Panicln": true,
	"Exit": true, "Goexit": true, "FailNow": true,
}

// runLogNoReturnCheck walks one file and reports err != nil guards whose
// bodies log the error and continue as success.
func runLogNoReturnCheck(pass *analysis.Pass, file *ast.File) {
	ast.Inspect(file, func(n ast.Node) bool {
		ifs, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		errIdent := errNilCondIdent(pass, ifs.Cond)
		if errIdent == nil {
			return true
		}
		if bodyLogsAndContinues(pass, ifs, errIdent) {
			reportSubCheck(pass, file, checkNameLogNoReturn, ifs.Pos(),
				"%q is only logged here and the failure path continues as success; return or propagate the error, or annotate with // justified: <reason>",
				errIdent.Name)
		}
		return true
	})
}

// bodyLogsAndContinues classifies every statement in the guard body.
// It returns true when the body's only error-consuming statements are
// logging calls and nothing terminates or propagates the failure.
func bodyLogsAndContinues(pass *analysis.Pass, ifs *ast.IfStmt, errIdent *ast.Ident) bool {
	parents := buildParents(ifs.Body)

	terminated := false
	reassigned := false
	logUses := 0
	otherUses := 0

	ast.Inspect(ifs.Body, func(n ast.Node) bool {
		if n == nil || terminated || reassigned {
			return false
		}
		switch v := n.(type) {
		case *ast.FuncLit:
			// Deferred/spawned closures follow different control flow.
			return false
		case *ast.ReturnStmt, *ast.BranchStmt:
			terminated = true
			return false
		case *ast.CallExpr:
			if isTerminatorCall(v) {
				terminated = true
				return false
			}
			return true
		case *ast.Ident:
			// Name-based matching: shadow-tolerant (a shadowed err in
			// the body is the same failure being handled onward).
			if v.Name != errIdent.Name {
				return true
			}
			switch classifyErrUse(pass, parents, ifs.Body, v) {
			case errUseReassigned:
				reassigned = true
			case errUseLogged:
				logUses++
			default:
				otherUses++
			}
		}
		return true
	})

	return !terminated && !reassigned && logUses >= 1 && otherUses == 0
}

type errUseKind int

const (
	errUseOther errUseKind = iota
	errUseLogged
	errUseReassigned
)

// classifyErrUse walks from an err identifier up to the guard body and
// decides whether the use is a reassignment, a logging use, or anything
// else (which counts as propagation and disarms the check).
func classifyErrUse(pass *analysis.Pass, parents map[ast.Node]ast.Node, root ast.Node, ident *ast.Ident) errUseKind {
	// Direct LHS of an assignment → reassignment.
	if as, ok := parents[ident].(*ast.AssignStmt); ok {
		if slices.Contains(as.Lhs, ast.Expr(ident)) {
			return errUseReassigned
		}
	}
	// Ascend: the use is a logging use iff some enclosing call within
	// the body is a recognized logging call.
	for cur := ast.Node(ident); cur != nil && cur != root; cur = parents[cur] {
		if call, ok := cur.(*ast.CallExpr); ok && isLoggingCall(pass, call) {
			return errUseLogged
		}
	}
	return errUseOther
}

// isLoggingCall reports whether a call looks like a structured-logging
// invocation: a selector whose method name is a known logging name, on a
// receiver that is not an error value and not the fmt/errors packages.
func isLoggingCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	name := sel.Sel.Name
	if pkgPath := resolvedPkgPath(pass, sel.X); pkgPath != "" {
		// fmt.Errorf / errors.* construct errors — propagation, not logging.
		if pkgPath == "fmt" || pkgPath == "errors" ||
			strings.HasSuffix(pkgPath, "/errors") || strings.HasSuffix(pkgPath, "xerrors") {
			return false
		}
		if pkgPath == "log" && logPrintNames[name] {
			return true
		}
	}
	if !logMethodNames[name] {
		return false
	}
	// err.Error() and friends: a method on an error value is never a
	// logging call.
	if recvType := pass.TypesInfo.TypeOf(sel.X); isErrorType(recvType) {
		return false
	}
	return true
}

// isTerminatorCall reports whether the call ends the failure path
// (panic, os.Exit, log.Fatal*, runtime.Goexit, testing FailNow).
func isTerminatorCall(call *ast.CallExpr) bool {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name == "panic"
	case *ast.SelectorExpr:
		return terminatorFuncNames[fn.Sel.Name]
	}
	return false
}
