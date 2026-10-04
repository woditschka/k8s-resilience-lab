# Scenarios are Go functions over immutable descriptions

**Status:** Accepted

## Context

A Scenario sets up a Topology and a Workload, injects a Failure, and checks the outcome. The owner asked whether "the k8s tests could be just the go program and not test". The question is where that behavior lives: a `go test` suite, or the binary that also drives the live view (G-2).

The description has to read well at two levels: a high-level shortcut such as a Zone count, and detailed control over Zone, Node, and Workload settings. The owner declined an assertion library — "i do not want to use a libraty that introduces a risk" — and chose the project's own `Expect(t, got).To(matcher)`.

## Options Considered

1. **A `Scenario` description value a runner interprets** — every new step grows a second language beside Go.
2. **A Scenario as a Go function over a provisioned cluster** — the descriptions stay immutable values; the sequence is Go.
3. **Scenarios as `go test` functions** — the live view then has no path to the same behavior (G-2), and the Scenarios would be unreachable from the binary.

## Decision

Option 2. A Scenario is an exported Go function taking a context, the provisioned cluster, and an `expect.T`. A `lab` command runs them and exits non-zero when a check fails, so the unattended run still verifies. The descriptions a Scenario builds are immutable values with no builder and no terminal build step, so a chain stopped anywhere is a complete value. Construction is `scenario.NewTopology(...)` and its siblings: the package qualifier is the namespace, so the constructor name carries the type.

Every shortcut is a pure function expanding to the detailed typed settings. No setting is reachable through a shortcut alone, and a shortcut on a Scenario delegates to the corresponding `Topology` or `Workload` call without adding a setting of its own. Node capacity is a setting on the description, never a literal in the mapper, because the PRD's first edge case needs a Workload exceeding its Topology.

Assertions go through the project's own `internal/expect` rather than a library, over the minimal interface that `*testing.T` and the Scenario runner both satisfy. Unit behavior stays test-driven under `go test`: Placement counting and Skew, the capacity rules, the cluster adapter against KWOK, and the `Expect` helper itself.

## Consequences

One body of behavior serves the unattended run and the live view, so neither needs a second code path. A Scenario written with shortcuts and one written in full are the same value. A new setting is one struct field, one `With` method, and one mapper line.

A shortcut can only ever name a combination of existing settings. A convenience that cannot be expressed that way is the signal that a setting is missing from the description.

The assertion vocabulary is the project's own, so a matcher the lab needs is written rather than looked up, and no assertion library enters the dependency tree.

Per-Zone asymmetry, Failure injection, a disruption budget, and a Workload read from a manifest file are later slices, each a further `With` call on the same values.

## Implementation

**Requirements:** REQ-SIM-001

## References

- [system-design.md § Contracts](../system-design.md)
- [ADR: The lab is a Go program on a KWOK Simulated Cluster](2026-10-03-go-program-on-a-kwok-simulated-cluster.md)
