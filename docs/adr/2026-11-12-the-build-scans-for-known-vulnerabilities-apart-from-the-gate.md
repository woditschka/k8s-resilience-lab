# The build scans for known vulnerabilities apart from the gate

**Status:** Accepted

## Context

REQ-SIM-007 held that a dependency with a known vulnerability fails the lab. No check did so: the gate command ran no scanner, and the security brief listed vulnerability scanning as a standing gap.

`govulncheck` reports the known vulnerabilities in code the lab calls, in its dependencies and in the standard library of the Go toolchain running it. It reads the advisories from vuln.go.dev on every run. Its answer therefore changes with the date and needs the network, while the lab's own code stays the same.

A first attempt, since reverted, pinned the tool in the lab's own `go.mod`. It raised the lab's `golang.org/x` requirements and added the tool's own modules as indirect requirements. It also made `go mod tidy -diff` fail with no network.

## Options Considered

Where the scan runs:

1. **In `make ci` and in the GitHub workflow** — one command answers for both the change and the advisories.
2. **In the GitHub workflow alone, on each push to main, each pull request and once a week** — the gate answers for the change; the build answers for the advisories.
3. **Nowhere, with the requirement narrowed** — the gap stays listed.

Where the tool's version is held:

4. **`go install …@latest` in the workflow** — no pin.
5. **A `tool` line in the lab's `go.mod`** — one module file.
6. **A separate module file, `tools.mod` with `tools.sum`**, run as `go tool -modfile=tools.mod govulncheck ./...`.

## Decision

Option 2, with option 6. A separate workflow job runs `make vuln-check` on each push to main, each pull request and a weekly schedule. `make ci` does not run it.

Option 1 is rejected because the gate would then fail on a day an advisory appears, for a change that touched nothing, and fail on every machine with no network.

Option 3 is rejected because advisories arrive between commits, and an unchecked requirement is no requirement.

Option 4 is rejected because an unpinned tool can change what the build reports with no reviewed change.

Option 5 is rejected because the tool's own requirements would raise the lab's dependencies and tie the tidy check to the network.

## Consequences

The tool's dependencies never enter the lab's module graph, so `make deps-check` and `make tidy-check` read `go.mod` alone. `go tool` verifies the tool's modules against `tools.sum` on every run.

A clean gate no longer means the lab is free of known vulnerabilities. The weekly run reports a new advisory on main without a commit.

The workflow job reads the Go version from `go.mod`, so the standard library it scans is the one the lab builds with.

An agent whose sandbox blocks vuln.go.dev cannot run the scan. Its tests judge the pin, the gate's composition and the job's triggers offline.

## Implementation

**Requirements:** REQ-SIM-007

## References

- [system-design.md § Contracts](../system-design.md)
- [ADR: The gate runs the tests under the race detector](2026-10-04-race-detector-in-the-gate.md)
