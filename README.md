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

The extension contract: **consumers may tighten freely, but loosening a
guard happens only through that guard's own allowlist or justification
comment** — written down, with a reason, visible in review. There is no flag
to turn an analyzer off wholesale; a guard you can switch off in build
configuration is a guard that silently stopped running.

## Allowlists ratchet

Analyzers that support exceptions take an allowlist file
(`-<analyzer>.allowlist=<path>`) using a shared grammar:

```
rule:subject|owner=team-payments|until=2026-12-31|reason=legacy clock injection pending
```

Existing offenders stay green; new offenders are blocked. Shrinking the file
is the only way it changes in a healthy repository.

## Status

Extracted from the analyzer suite that guards PlatformKit's own estate,
where these checks run on every push. Early: expect additions (ratchet
runner, allow-inventory report, tier budgets) before the API is called
stable.

## License

Apache-2.0
