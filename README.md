# pk-guard

> Part of [PlatformKit](https://github.com/septagon-oss/platformkit) — the open-source Go backend for multi-tenant SaaS.

Composable architecture guardrails for Go repositories, built on
[`go/analysis`](https://pkg.go.dev/golang.org/x/tools/go/analysis). pk-guard
exists for one problem: code is now written faster than humans can review it,
so the review has to be encoded — as analyzers that run in hooks and CI, with
every exception written down and auditable.

**Depends on.** `golang.org/x/tools` and `gopkg.in/yaml.v3`. Nothing else, by
policy: a guard tool that can break because of an unrelated dependency is a
guard tool that silently stops guarding.

## The analyzers

| Analyzer | What it enforces |
|---|---|
| `safeerror` | No silent failures: an error that is logged-but-not-returned, discarded, or overwritten before use is a finding. Escape hatch: an exact `// justified: <reason>` comment on the site. |
| `importboundary` | Module isolation in a modular monolith: a module imports its own subpackages, declared shared packages, or another module's published-contracts sub-tree — nothing else. |
| `noclockindomain` | Domain code (`features/*/service*.go`) never reads the wall clock or `math/rand` directly; inject a clock seam instead. |
| `buildtags` | E2E/BDD test files carry `//go:build e2e`, so browser tooling never leaks into regular builds. |

`importboundary` and `noclockindomain` read the repository's **module
catalog** (`catalog/module_contracts.yaml` by default):

```yaml
modulePrefix: github.com/acme/platform/modules
contractsSegment: contracts/provides   # optional; this is the default
sharedPackages: [ports, internal, testutil]
modules:
  - id: billing
  - id: user_management
```

No catalog file → those analyzers report nothing. They are safe to include
everywhere.

## Run it

```sh
go run github.com/septagon-oss/pk-guard/cmd/pk-guard@latest ./...
```

Or as a vettool: `go vet -vettool=$(which pk-guard) ./...`.

## Extend it

A repository (or a client overlay extending a platform) composes its own
guard binary in five lines:

```go
package main

import (
    "github.com/septagon-oss/pk-guard/guardmain"
    acmeguards "github.com/acme/platform/guards"
)

func main() {
    guardmain.Run(append(guardmain.Std(), acmeguards.All()...)...)
}
```

The extension contract: **consumers may tighten freely; loosening is
designed to go through each guard's own allowlist or justification
comment** — written down, with a reason, visible in review.

## Exceptions are written down

Every escape hatch is explicit, local, and reviewable — but the grammar is
per-analyzer:

- `safeerror`: the exact `// justified: <reason>` comment on the flagged
  line (or the line above). No file-level suppression exists.
- `noclockindomain`: `-noclockindomain.allowlist=<path>` with expiring
  entries — `clock_in_domain:billing|owner=team-payments|until=2026-12-31|reason=...`.
- `importboundary`: `-importboundary.allowlist=<path>` with
  `file-suffix -> target_module` lines; both sides are required, and an
  empty suffix is rejected rather than becoming a repo-wide match.

Existing offenders stay green; new offenders are blocked. Shrinking an
allowlist is the only way it changes in a healthy repository.

## Two properties to know before relying on it

- **The catalog is the scope.** A module directory absent from the catalog
  is invisible to the topology analyzers — they cannot guard what is not
  declared. Generate the catalog from your module registry, or pair it with
  a completeness check, so a new module cannot be forgotten.
- **Skipping is visible, not impossible.** The multichecker driver exposes
  `-NAME=false` per analyzer. pk-guard adds no quieter off-switch: disabling
  a guard is always legible in the invocation your hooks and CI run, which
  is where review should look.

## Status

Extracted from the analyzer suite that guards PlatformKit's own estate,
where these checks run on every push. Early: expect additions (ratchet
runner, allow-inventory report, tier budgets) before the API is called
stable.

## License

Apache-2.0
