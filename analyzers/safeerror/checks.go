package safeerror

// checks.go — shared strict safeerror analysis helpers.

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
)

const (
	checkNameDiscard      = "discarded-error"
	checkNameLogNoReturn  = "log-without-return"
	checkNameWrapReassign = "wrap-after-reassign"
)

// skipFileForSubChecks exempts generated files and test support. Some reusable
// contract suites intentionally live in ordinary .go files so implementations
// in other packages can invoke them; importing testing identifies that shape.
func skipFileForSubChecks(pass *analysis.Pass, file *ast.File) bool {
	filename := pass.Fset.Position(file.Pos()).Filename
	if strings.HasSuffix(filename, "_test.go") || ast.IsGenerated(file) {
		return true
	}
	for _, imported := range file.Imports {
		path, err := strconv.Unquote(imported.Path.Value)
		if err == nil && path == "testing" {
			return true
		}
	}
	return false
}

func subCheckSuppressed(pass *analysis.Pass, file *ast.File, pos token.Pos) bool {
	p := pass.Fset.Position(pos)
	for _, commentGroup := range file.Comments {
		for _, comment := range commentGroup.List {
			line := pass.Fset.Position(comment.Pos()).Line
			if line != p.Line && line != p.Line-1 {
				continue
			}
			text := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))
			reason, justified := strings.CutPrefix(text, "justified:")
			if justified && strings.TrimSpace(reason) != "" {
				return true
			}
		}
	}
	return false
}

func reportSubCheck(pass *analysis.Pass, file *ast.File, check string, pos token.Pos, format string, args ...any) {
	if subCheckSuppressed(pass, file, pos) {
		return
	}
	pass.Reportf(pos, "%s (safeerror/%s)", fmt.Sprintf(format, args...), check)
}

func isErrorType(valueType types.Type) bool {
	if valueType == nil {
		return false
	}
	errorObject := types.Universe.Lookup("error")
	return errorObject != nil && types.Identical(valueType, errorObject.Type())
}

func errNilCondIdent(pass *analysis.Pass, condition ast.Expr) *ast.Ident {
	binary, ok := condition.(*ast.BinaryExpr)
	if !ok || binary.Op != token.NEQ {
		return nil
	}
	left, right := binary.X, binary.Y
	if isNilIdent(left) {
		left, right = right, left
	}
	if !isNilIdent(right) {
		return nil
	}
	identifier, ok := left.(*ast.Ident)
	if !ok || identifier.Name == "_" || !isErrorType(pass.TypesInfo.TypeOf(identifier)) {
		return nil
	}
	return identifier
}

func isNilIdent(expression ast.Expr) bool {
	identifier, ok := expression.(*ast.Ident)
	return ok && identifier.Name == "nil"
}

func buildParents(root ast.Node) map[ast.Node]ast.Node {
	parents := map[ast.Node]ast.Node{}
	var stack []ast.Node
	ast.Inspect(root, func(node ast.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if len(stack) > 0 {
			parents[node] = stack[len(stack)-1]
		}
		stack = append(stack, node)
		return true
	})
	return parents
}

func resolvedPkgPath(pass *analysis.Pass, expression ast.Expr) string {
	identifier, ok := expression.(*ast.Ident)
	if !ok {
		return ""
	}
	if packageName, ok := pass.TypesInfo.Uses[identifier].(*types.PkgName); ok {
		return packageName.Imported().Path()
	}
	return ""
}
