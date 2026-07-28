// Package importboundary defines a go/analysis analyzer that enforces
// module boundary rules in a modular monolith.
//
// A module may import:
//   - its own sub-packages,
//   - the catalog's declared shared packages (ports, internal, ...),
//   - or another module's published-contracts sub-tree only (by default
//     `contracts/provides`; configurable via the catalog's contractsSegment).
//
// Cross-module imports targeting any other path under another module's tree
// are rejected. Publish runtime interfaces through shared packages and
// cross-module contracts through the published-contracts sub-tree.
//
// The module topology (prefix, module IDs, shared packages, contracts
// segment) comes from the modulecatalog file; see that package for the
// format. Exceptions are declared in an allowlist file of
// `file-suffix -> target_module` lines, and every exception is visible in
// one place rather than scattered through the tree.
package importboundary

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"golang.org/x/tools/go/analysis"

	"github.com/septagon-oss/pk-guard/modulecatalog"
)

// Analyzer is the importboundary analyzer.
var Analyzer = &analysis.Analyzer{
	Name: "importboundary",
	Doc:  "enforces cross-module import boundaries in a modular monolith",
	Run:  run,
}

var (
	allowlistPath string
	catalogPath   string
	modPrefix     string
)

func init() {
	Analyzer.Flags.StringVar(&allowlistPath, "allowlist", "", "path to import boundary allowlist file")
	Analyzer.Flags.StringVar(&catalogPath, "catalog", "", "path to the module catalog YAML (defaults to "+modulecatalog.DefaultCatalogPath+")")
	Analyzer.Flags.StringVar(&modPrefix, "modprefix", "", "module import-path prefix when the catalog does not declare modulePrefix")
}

// resolver caches the loaded catalog across all packages in a single
// analyzer run. The go/analysis driver runs packages concurrently, so the
// binding from flags to resolver happens exactly once — a lazy rebind on
// every run() was a data race. Flags cannot change mid-process in a
// vettool, so once-only binding loses nothing.
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

func run(pass *analysis.Pass) (any, error) {
	catalog, catErr := loadCatalog()
	switch {
	case errors.Is(catErr, modulecatalog.ErrCatalogNotFound):
		// Outside a module tree: no boundaries to enforce.
		return nil, nil
	case catErr != nil:
		return nil, fmt.Errorf("importboundary: %w", catErr)
	}

	pkgPath := pass.Pkg.Path()
	sourceModule := catalog.ModuleForPackage(pkgPath)
	if sourceModule == "" {
		// Package is not inside a cataloged module - nothing to check.
		return nil, nil
	}

	allowed, err := loadImportAllowlist(allowlistPath)
	if err != nil {
		return nil, fmt.Errorf("importboundary: %w", err)
	}

	contractsSegment := catalog.ContractsSegment()

	for _, file := range pass.Files {
		filePath := pass.Fset.Position(file.Pos()).Filename

		for _, spec := range file.Imports {
			importPath := strings.Trim(spec.Path.Value, `"`)
			relImport, inTree := catalog.Rel(importPath)
			if !inTree {
				continue
			}

			// The source module may freely import its own sub-packages.
			if relImport == sourceModule || strings.HasPrefix(relImport, sourceModule+"/") {
				continue
			}

			if catalog.IsShared(relImport) {
				continue
			}

			// Only published contracts of other modules may be imported.
			targetModule := catalog.ModuleForPackage(importPath)
			if targetModule != "" && targetModule != sourceModule {
				sub := strings.TrimPrefix(relImport, targetModule+"/")
				if sub == contractsSegment || strings.HasPrefix(sub, contractsSegment+"/") {
					continue
				}
			}

			if isImportAllowed(allowed, filePath, targetModule) {
				continue
			}

			pass.Reportf(
				spec.Pos(),
				"module %q cannot import %q; use a shared package or %s/%s/ instead",
				sourceModule, importPath, targetModule, contractsSegment,
			)
		}
	}

	return nil, nil
}

// importException represents a file -> target_module allowlist entry.
type importException struct {
	fileSuffix   string // e.g., "auth_management/features/auth_provider/feature.go"
	targetModule string // e.g., "tenant_management"
}

// loadImportAllowlist reads the allowlist file. An empty path returns no
// entries. A missing file is reported as an error rather than silently
// ignored — a typo or path change in the build configuration must surface,
// not loosen enforcement.
func loadImportAllowlist(path string) ([]importException, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open allowlist %q: %w", path, err)
	}
	defer func() {
		// justified: read-only file; the close error is non-actionable.
		_ = f.Close()
	}()

	var entries []importException
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "->", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("allowlist %q: invalid line %q (expected `file -> module`)", path, line)
		}
		fileSuffix := strings.TrimSpace(parts[0])
		targetModule := strings.TrimSpace(parts[1])
		if fileSuffix == "" || targetModule == "" {
			// An empty suffix would HasSuffix-match every file in the tree —
			// a one-character typo must not become a repository-wide bypass.
			return nil, fmt.Errorf(
				"allowlist %q: invalid line %q (both file suffix and module are required)", path, line)
		}
		entries = append(entries, importException{
			fileSuffix:   fileSuffix,
			targetModule: targetModule,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read allowlist %q: %w", path, err)
	}
	return entries, nil
}

func isImportAllowed(allowed []importException, filePath, targetModule string) bool {
	for _, entry := range allowed {
		if entry.targetModule == targetModule && strings.HasSuffix(filePath, entry.fileSuffix) {
			return true
		}
	}
	return false
}
