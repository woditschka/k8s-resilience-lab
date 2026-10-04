# The module-rule tests share one test-support package

**Status:** Accepted

## Context

Two packages assert the lab's module rules from the module's own package graph, independently of the linter: `internal/cluster` and `internal/scenario`. Each carried the same graph walk, byte for byte, and the call-rule scan sits beside it.

A helper shared as production source has no production caller, which is what the duplication was avoiding.

## Options Considered

1. **The duplication stays** — a comment in each copy naming the reason.
2. **One package's test asserts both rules** — one copy, and a module's rule asserted from outside that module.
3. **A test-support package under `internal/`** — one copy, imported by test files alone.

## Decision

Option 3. `internal/modulerules` holds the package-graph walk and the call-rule scan. The module-rule tests of `internal/cluster` and `internal/scenario` import it, no production package does, and the import-graph test asserts its absence from every production package's dependencies. It imports the standard library alone, so `internal/scenario`'s own rule is unaffected by reaching it.

Option 1 is rejected: a rule changed in one copy and not the other is a rule asserted for one module only.

Option 2 is rejected: a module's rule asserted outside it survives that module being moved or split by accident.

## Consequences

The modulith gains a fifth module whose callers are test files alone, beside `internal/expect`, whose callers are tests and Scenarios.

Its helpers are pure functions over a package graph and a file set, so the package carries its own test and needs no container engine to run it.

A module rule changes in one place, and both modules assert the changed rule.

## Implementation

**Requirements:** REQ-SIM-004, REQ-SIM-001

## References

- [system-design.md § Package Structure](../system-design.md)
- [ADR: The lab is a Go program on a KWOK Simulated Cluster](2026-10-03-go-program-on-a-kwok-simulated-cluster.md)
- [ADR: The lab provisions the Simulated Cluster it drives](2026-10-03-lab-provisioned-simulated-cluster.md)
