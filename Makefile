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

.PHONY: all build test check clean golden notify

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
