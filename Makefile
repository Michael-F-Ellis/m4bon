# Build targets for m4bon
#
# Key insight: go test ./... does NOT catch build errors in the TUI
# (cmd/m4bon/tui) because it has a //go:build darwin && cgo constraint.
# Always run "make" (or "make all") after making changes to ensure
# the full binary compiles.
#
# Usage:
#   make        Build the world + run tests (default)
#   make all    Same as make
#   make build  Build the m4bon binary only
#   make test   Run all tests (without building)
#   make check  Build + test + vet
#   make clean  Remove build artifacts (binaries, WASM, object files)
#   make golden Update golden test files

BINARY := m4bon
GO := go

.PHONY: all build test check clean golden notify wasm gen-examples gh-pages serve

all: build wasm test

build:
	$(GO) build -o $(BINARY) ./cmd/m4bon/

test:
	$(GO) test ./...

check: build wasm test
	$(GO) vet ./...

clean:
	rm -f $(BINARY) web/m4bon.wasm
	$(GO) clean ./...

golden:
	$(GO) test ./musicxml/ -update-golden -count=1
	$(GO) test ./cmd/m4bon/ -update-golden -count=1

# Notify after a long task completes. Usage: make notify MSG="done"
notify:
	@if [ -f ./notify.sh ]; then ./notify.sh "$(MSG)"; fi

# Build the WebAssembly binary for the web TUI.
# Regenerates examples.js from examples/*.dsl first.
# Uses Go's built-in js/wasm target — no external toolchain needed.
wasm: gen-examples
	GOOS=js GOARCH=wasm $(GO) build -o web/m4bon.wasm ./wasm/

# Regenerate web/examples.js from examples/*.dsl files.
# Automatically run as part of 'make wasm'; also callable standalone.
gen-examples:
	$(GO) run ./scripts/gen-examples/main.go

# Deploy via GitHub Actions (recommended):
#   Push to main → .github/workflows/deploy.yml handles build + deploy.
#   Then set repo Settings → Pages → Source to "GitHub Actions".
#
# Legacy deploy — to the gh-pages branch (kept for fallback):
#   make gh-pages  &&  git push origin gh-pages
#   Then set repo Settings → Pages → Source to "Deploy from a branch".
gh-pages: wasm
	@./scripts/deploy-gh-pages.sh

# Serve the web app locally on port 8087 for development.
# Kills any existing listener on that port first.
serve:
	@lsof -ti:8087 | xargs kill -9 2>/dev/null; true
	cd web && python3 -m http.server 8087
