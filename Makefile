# The lab's gate command and the targets it sequences.
#
# `make ci` is the one command that decides whether the lab is sound (REQ-SIM-004).
# It is declared as the gate command in scripts/layout.toml [gate]; the target names
# are the ones CLAUDE.md § Build Commands documents.
#
# golangci-lint is invoked from GOPATH/bin rather than through `go run`: upstream
# discourages `go run` because it builds the linter against this module's Go version
# instead of the tested release (CLAUDE.md § Toolchain).

GOLANGCI_LINT := $(shell go env GOPATH)/bin/golangci-lint

# Module path prefixes of the dependency classes docs/system-design.md
# § Dependency Policy § Prohibited rejects. That table is the source of truth; one
# group here per class, so a class the table gains but this list lacks surfaces as a
# name `make deps-check` never reports.
PROHIBITED_IMPORTS := \
	github.com/onsi/gomega \
	github.com/stretchr/testify \
	github.com/golang/mock \
	go.uber.org/mock \
	github.com/vektra/mockery \
	github.com/testcontainers/testcontainers-go \
	github.com/docker/docker \
	github.com/docker/cli \
	sigs.k8s.io/controller-runtime \
	k8s.io/kubectl

.PHONY: ci tidy-check fmt fmt-check vet lint lint-fix deps-check test test-race build

ci: tidy-check fmt-check vet lint deps-check test build

tidy-check:
	go mod tidy -diff

fmt:
	go fmt ./...

fmt-check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "fmt-check: these files are not gofmt-clean:"; \
		printf '%s\n' "$$unformatted" | sed 's/^/  /'; \
		exit 1; \
	fi

vet:
	go vet ./...

lint:
	$(GOLANGCI_LINT) run

lint-fix:
	$(GOLANGCI_LINT) run --fix

deps-check:
	@compiled=$$(go list -deps -test ./...) || exit 1; \
	status=0; \
	for prefix in $(PROHIBITED_IMPORTS); do \
		offenders=$$(printf '%s\n' "$$compiled" | grep -e "^$$prefix$$" -e "^$$prefix/" || true); \
		if [ -n "$$offenders" ]; then \
			echo "deps-check: prohibited dependency (docs/system-design.md § Dependency Policy § Prohibited):"; \
			printf '%s\n' "$$offenders" | sed 's/^/  /'; \
			status=1; \
		fi; \
	done; \
	exit $$status

# The gate's test check runs the tests once, under the race detector, per
# docs/adr/2026-10-04-race-detector-in-the-gate.md. The lab's subject is the order
# in which a control plane reacts, and the detector's slower timing is the regime a
# wrong order shows in.
#
# The check asserts no toolchain prerequisite of its own: no CGO_ENABLED test, no
# search for a C compiler and no operating-system branch, because each of those is
# a second rule about where the detector needs a compiler and macOS needs none.
# The Go toolchain refuses where the detector needs cgo and a compiler it cannot
# find; that refusal is this failure, and the install names below are the ones
# CLAUDE.md § Toolchain documents.
test:
	@go test -race ./... || { \
		refused=$$?; \
		echo "test: the tests must pass under the race detector (docs/adr/2026-10-04-race-detector-in-the-gate.md)"; \
		echo "  if the Go toolchain refused the detector above, it needs cgo and a C compiler:"; \
		echo "    Debian, Ubuntu: sudo apt-get install gcc"; \
		echo "    Fedora, RHEL:   sudo dnf install gcc"; \
		echo "  macOS needs neither"; \
		exit $$refused; \
	}

# test-race is the name CLAUDE.md § Build Commands documents for the detector run.
# The gate's own test check is that run, so the two names are one target.
test-race: test

build:
	go build ./...
