// Package buildtags defines a go/analysis analyzer that ensures E2E and
// BDD test files carry the //go:build e2e constraint. Without this tag,
// test infrastructure (go-rod, chromedp) leaks into regular builds.
package buildtags

import (
	"go/ast"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/analysis"
)

var Analyzer = &analysis.Analyzer{
	Name: "buildtags",
	Doc:  "ensures e2e/bdd test files have //go:build e2e constraints",
	Run:  run,
}

func run(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		pos := pass.Fset.Position(file.Pos())
		if !needsE2ETag(pos.Filename) {
			continue
		}

		if hasE2EBuildTag(file) {
			continue
		}

		pass.Reportf(
			file.Package,
			"file %s must have //go:build e2e constraint",
			filepath.Base(pos.Filename),
		)
	}
	return nil, nil
}

func needsE2ETag(filename string) bool {
	base := filepath.Base(filename)
	dir := filepath.Dir(filename)

	// Files named e2e.go always need the tag.
	if base == "e2e.go" {
		return true
	}

	// Files under tests/e2e/ or tests/bdd/ need the tag.
	parts := strings.Split(filepath.ToSlash(dir), "/")
	for i, part := range parts {
		if part == "tests" && i+1 < len(parts) {
			next := parts[i+1]
			if next == "e2e" || next == "bdd" {
				return true
			}
		}
	}

	return false
}

func hasE2EBuildTag(file *ast.File) bool {
	for _, cg := range file.Comments {
		for _, c := range cg.List {
			text := c.Text
			if strings.HasPrefix(text, "//go:build ") && strings.Contains(text, "e2e") {
				return true
			}
			if strings.HasPrefix(text, "// +build ") && strings.Contains(text, "e2e") {
				return true
			}
		}
	}
	return false
}
