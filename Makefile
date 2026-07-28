.PHONY: help build test vet guard staticcheck race verify

TMPDIRS := .tmp-go-tmp
export GOTMPDIR := $(CURDIR)/.tmp-go-tmp
export TMPDIR := $(CURDIR)/.tmp-go-tmp
# Standalone module: never resolve through an enclosing Go workspace. Inside
# the PlatformKit estate a go.work exists two directories up and does not
# list this repo; without this pin every make target fails there.
export GOWORK := off

$(TMPDIRS):
	@mkdir -p $(TMPDIRS)

help: ## List targets
	@grep -E '^[a-z-]+:.*##' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-12s %s\n", $$1, $$2}'

build: | $(TMPDIRS) ## Build all packages and the pk-guard binary
	go build ./...
	go build -o .bin/pk-guard ./cmd/pk-guard

test: | $(TMPDIRS) ## Run all tests, cache disabled
	go test -count=1 ./...

vet: | $(TMPDIRS) ## go vet
	go vet ./...

guard: build ## pk-guard guards itself
	./.bin/pk-guard ./...

staticcheck: | $(TMPDIRS) ## staticcheck if installed
	@command -v staticcheck >/dev/null 2>&1 && staticcheck ./... || echo "staticcheck not installed; skipping"

race: | $(TMPDIRS) ## Tests under the race detector
	go test -race -count=1 ./...

verify: test vet guard staticcheck race ## Full local gate
