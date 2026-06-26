# Build targets for m4bon
#
# Key insight: go test ./... does NOT catch build errors in the TUI
# (cmd/m4bon/tui) because it has a //go:build darwin && cgo constraint.
# Always run "make" (or "make all") after making changes to ensure
# the full binary compiles.
#
# Usage:
#   make        Build binary + run tests (default)
#   make build  Build the m4bon binary only
#   make test   Run all tests (without building binary)
#   make check  Build + test + vet
#   make clean  Remove build artifacts
#   make golden Update golden test files

BINARY := m4bon
GO := go

.PHONY: all build test check clean golden notify wasm gh-pages

all: build test

build:
	$(GO) build -o $(BINARY) ./cmd/m4bon/

test:
	$(GO) test ./...

check: build test
	$(GO) vet ./...

clean:
	rm -f $(BINARY)
	$(GO) clean ./...

golden:
	$(GO) test ./musicxml/ -update-golden -count=1
	$(GO) test ./cmd/m4bon/ -update-golden -count=1

# Notify after a long task completes. Usage: make notify MSG="done"
notify:
	@if [ -f ./notify.sh ]; then ./notify.sh "$(MSG)"; fi

# Build the WebAssembly binary for the web TUI.
# Requires tinygo (https://tinygo.org).
wasm:
	tinygo build -o web/m4bon.wasm -target wasm -no-debug ./wasm/

# Deploy the web TUI to the gh-pages branch for GitHub Pages.
# After running, push with: git push origin gh-pages
# Then enable GitHub Pages in repo Settings → Pages → Source: "Deploy from branch" → gh-pages.
gh-pages: wasm
	@./scripts/deploy-gh-pages.sh
