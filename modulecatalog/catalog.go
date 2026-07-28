// Package modulecatalog is the single source of truth for a repository's
// module topology, shared by every analyzer that needs to answer "which
// module owns this package?".
//
// # Why this package exists
//
// Boundary analyzers that each invent their own "is this a module?"
// predicate drift apart: one recognizes an exception the other does not,
// and modules whose names miss a suffix heuristic are silently skipped by
// one analyzer but not another. The gap is invisible until an audit trips
// over it. This package centralizes the predicate in one catalog file that
// enumerates every module; analyzers consume the loaded catalog and never
// invent their own list.
//
// # The catalog file
//
// The catalog is YAML. Only the fields below are read; generators may put
// anything else alongside them:
//
//	modulePrefix: github.com/acme/platform/modules
//	contractsSegment: contracts/provides
//	sharedPackages: [ports, internal, testutil]
//	modules:
//	  - id: billing
//	  - id: user_management
//	externalModuleProviders: [payments_remote]
//
// modulePrefix is the import-path prefix under which every module lives.
// It may instead be supplied by the consuming analyzer (most expose a
// -modprefix flag); the catalog file wins when both are present, because
// the file travels with the code it describes.
package modulecatalog

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// DefaultCatalogPath is the relative path to the module catalog YAML, used
// when no explicit path is supplied. Resolved relative to the analyzer's
// working directory.
const DefaultCatalogPath = "catalog/module_contracts.yaml"

// DefaultContractsSegment is the sub-tree of another module's directory
// that may be imported across module boundaries when the catalog does not
// declare its own.
const DefaultContractsSegment = "contracts/provides"

// defaultSharedPackages are the sibling directories every module may import
// when the catalog does not declare its own set. Deliberately minimal: a
// real repository should declare sharedPackages in its catalog.
var defaultSharedPackages = []string{"ports", "internal", "testutil"}

// Catalog is the loaded module topology: the set of module IDs plus the
// import-path prefix and boundary conventions they live under.
//
// The zero value is an empty catalog; use Load to populate one.
type Catalog struct {
	ids              map[string]struct{}
	prefix           string
	shared           map[string]struct{}
	contractsSegment string
}

// Prefix returns the import-path prefix under which modules live, without a
// trailing slash. Empty when neither the catalog file nor the loader
// supplied one.
func (c *Catalog) Prefix() string {
	if c == nil {
		return ""
	}
	return c.prefix
}

// ContractsSegment returns the sub-tree of a module that other modules may
// import (for example "contracts/provides").
func (c *Catalog) ContractsSegment() string {
	if c == nil || c.contractsSegment == "" {
		return DefaultContractsSegment
	}
	return c.contractsSegment
}

// IsModule reports whether name appears as a top-level module ID in the
// catalog.
func (c *Catalog) IsModule(name string) bool {
	if c == nil {
		return false
	}
	_, ok := c.ids[name]
	return ok
}

// IsShared reports whether the first path component of rel (a module-tree
// relative import path) is a declared shared package — importable by every
// module without crossing a boundary.
func (c *Catalog) IsShared(rel string) bool {
	if c == nil {
		return false
	}
	first := rel
	if before, _, ok := strings.Cut(rel, "/"); ok {
		first = before
	}
	_, ok := c.shared[first]
	return ok
}

// All returns the catalog IDs in unspecified order. The slice is a fresh
// copy and may be mutated by callers without affecting the catalog.
func (c *Catalog) All() []string {
	if c == nil {
		return nil
	}
	out := make([]string, 0, len(c.ids))
	for id := range c.ids {
		out = append(out, id)
	}
	return out
}

// ModuleForPackage returns the module that owns pkgPath, or "" if pkgPath
// is not within any cataloged module.
//
// pkgPath is expected to be a Go import path. The match is the first path
// component immediately following the module prefix, when that component is
// a cataloged module ID.
func (c *Catalog) ModuleForPackage(pkgPath string) string {
	rel, ok := c.Rel(pkgPath)
	if !ok || len(c.ids) == 0 {
		return ""
	}
	first := rel
	if before, _, ok0 := strings.Cut(rel, "/"); ok0 {
		first = before
	}
	if _, ok := c.ids[first]; ok {
		return first
	}
	return ""
}

// Rel returns the path relative to the module prefix and whether pkgPath is
// within the module tree at all. A catalog without a prefix owns nothing.
func (c *Catalog) Rel(pkgPath string) (string, bool) {
	if c == nil || c.prefix == "" || pkgPath == c.prefix {
		return "", false
	}
	if !strings.HasPrefix(pkgPath, c.prefix+"/") {
		return "", false
	}
	return strings.TrimPrefix(pkgPath, c.prefix+"/"), true
}

// Load reads the catalog YAML at path. An empty path falls back to
// DefaultCatalogPath. A missing file is treated as an absent catalog and
// returns an empty Catalog with a sentinel error so callers can distinguish
// "not a module tree" from "catalog is corrupt".
//
// fallbackPrefix is used when the catalog file does not declare
// modulePrefix — typically sourced from an analyzer's -modprefix flag. The
// file's own declaration wins because it travels with the code.
func Load(path, fallbackPrefix string) (*Catalog, error) {
	if path == "" {
		path = DefaultCatalogPath
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Catalog{ids: map[string]struct{}{}}, ErrCatalogNotFound
		}
		return nil, fmt.Errorf("read catalog %q: %w", path, err)
	}
	var doc catalogDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse catalog %q: %w", path, err)
	}
	if len(doc.Modules) == 0 {
		return nil, fmt.Errorf("catalog %q declares no modules", path)
	}
	prefix := strings.TrimSuffix(strings.TrimSpace(doc.ModulePrefix), "/")
	if prefix == "" {
		prefix = strings.TrimSuffix(strings.TrimSpace(fallbackPrefix), "/")
	}
	if prefix == "" {
		return nil, fmt.Errorf(
			"catalog %q declares no modulePrefix and no fallback prefix was supplied", path)
	}
	ids := make(map[string]struct{}, len(doc.Modules)+len(doc.ExternalModuleProviders))
	for _, m := range doc.Modules {
		id := strings.TrimSpace(m.ID)
		if id == "" {
			return nil, fmt.Errorf("catalog %q contains a module entry with no id", path)
		}
		ids[id] = struct{}{}
	}
	// Approved external (contract-only / remote) module providers have no
	// in-tree implementation — it is supplied by app or client packages — so
	// they are not catalog Modules. Their published contracts packages are
	// still legitimate cross-module imports, so register their IDs too.
	// Without this, importboundary rejects dependents that import an external
	// provider's contracts.
	for _, id := range doc.ExternalModuleProviders {
		if id = strings.TrimSpace(id); id != "" {
			ids[id] = struct{}{}
		}
	}
	sharedList := doc.SharedPackages
	if len(sharedList) == 0 {
		sharedList = defaultSharedPackages
	}
	shared := make(map[string]struct{}, len(sharedList))
	for _, s := range sharedList {
		if s = strings.TrimSpace(s); s != "" {
			shared[s] = struct{}{}
		}
	}
	return &Catalog{
		ids:              ids,
		prefix:           prefix,
		shared:           shared,
		contractsSegment: strings.Trim(strings.TrimSpace(doc.ContractsSegment), "/"),
	}, nil
}

// ErrCatalogNotFound is returned by Load when the catalog file is missing.
// Analyzers running outside a module tree should treat this as "no modules
// to check" rather than a fatal error.
var ErrCatalogNotFound = errors.New("module catalog not found")

// catalogDoc is the minimal shape of the catalog YAML needed by the
// analyzers. Additional fields are ignored.
type catalogDoc struct {
	ModulePrefix     string   `yaml:"modulePrefix"`
	ContractsSegment string   `yaml:"contractsSegment"`
	SharedPackages   []string `yaml:"sharedPackages"`

	Modules []catalogModule `yaml:"modules"`
	// ExternalModuleProviders are approved contract-only / remote module IDs
	// (no in-tree implementation). Their contracts packages are valid
	// cross-module import targets.
	ExternalModuleProviders []string `yaml:"externalModuleProviders"`
}

type catalogModule struct {
	ID string `yaml:"id"`
}

// Resolver caches a Catalog loaded from a configurable path. It is safe for
// concurrent use; the catalog is loaded at most once per Resolver.
//
// Analyzers should hold a single Resolver and call Get on each pass; Get
// returns the same Catalog instance every time once successfully loaded.
type Resolver struct {
	path           string
	fallbackPrefix string

	once sync.Once
	cat  *Catalog
	err  error
}

// NewResolver returns a Resolver that will load from path on first Get.
// Empty path uses DefaultCatalogPath. fallbackPrefix supplies modulePrefix
// when the catalog file does not declare one.
func NewResolver(path, fallbackPrefix string) *Resolver {
	return &Resolver{path: path, fallbackPrefix: fallbackPrefix}
}

// Get returns the cached catalog or loads it on first call.
func (r *Resolver) Get() (*Catalog, error) {
	r.once.Do(func() {
		r.cat, r.err = Load(r.path, r.fallbackPrefix)
	})
	return r.cat, r.err
}
