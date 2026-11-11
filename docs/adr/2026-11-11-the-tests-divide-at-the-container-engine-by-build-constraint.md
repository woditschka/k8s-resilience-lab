# The lab's tests divide at the container engine by build constraint

**Status:** Accepted

## Context

The lab's own tests take long because the cluster-backed ones each start a Simulated Cluster. The owner asked whether they are needed for every change, and drew the line at "needs the container engine". The owner named the two kinds "unit tests (fast) and integrationtests".

REQ-SIM-035 asks for three runs: the tests needing no engine alone, the tests needing one alone, and both under the one command. The testing brief holds that no test skips: a test missing what it needs fails, naming it.

Engine-needing and engine-free tests share packages and files today: `cmd/lab`, `internal/cluster` and `internal/lab` hold both kinds.

## Options Considered

1. **A skip under `go test -short`** in each package's engine helper. It gives the engine-free run, but not a run of the engine tests alone, and it adds skips.
2. **A name marker selected by `-run` and `-skip`** — a marker in every engine test's name, breaking the `<Condition>_Should<Outcome>` shape.
3. **One positive tag on the engine files** — every tool and the IDE then needs the tag to see those files. A forgotten tag lets an import or a lint finding through unseen.
4. **Two negative constraints, one per kind** — an engine test file carries `//go:build !unit`, an engine-free test file `//go:build !integration`. Built with no tag, every file compiles.

## Decision

Option 4. A run tagged `unit` compiles no engine test, a run tagged `integration` no engine-free test, and a run with no tag compiles both. `make test-unit` and `make test-integration` are the two runs, and `make test` runs both under the race detector.

Every test file holding a test function carries exactly one of the two constraints. A file of helpers alone may carry neither. A module-rule test holds this, and that no engine-free file reaches the engine or provisions a cluster.

Option 1 is rejected because it cannot run the engine tests alone, and a skip is what the testing brief rules out.

Option 2 is rejected because test names are specifications, and a marker reads as neither condition nor outcome.

Option 3 is rejected because vet, lint, the dependency check and the IDE would each need the tag to see the engine tests.

## Consequences

No test skips: a run leaves the other kind's tests uncompiled. The race detector ADR still holds: the gate runs every test once, under the detector.

`go vet`, golangci-lint, the dependency check and the IDE need no tag, since the untagged build holds every file.

A file mixing both kinds splits in two. A new test file names its kind on its first line, or the module-rule test fails.

The fast target's names follow the owner, not the testing pyramid: a test reading real files runs under `make test-unit`.

## Implementation

**Requirements:** REQ-SIM-035

## References

- [system-design.md § Contracts](../system-design.md#contracts)
- [ADR: The gate runs the tests under the race detector](2026-10-04-race-detector-in-the-gate.md)
