# Scenarios are Go functions over immutable descriptions

**Status:** Accepted

## Context

A Scenario sets up a Topology and a Deployment, injects a Failure, and checks the outcome. The owner asked whether "the k8s tests could be just the go program and not test", so the behavior must also reach the Live view (G-2). The owner asked for "an internal dsl - setup a bit given when then style", with the machinery "more hidden e.g. within internal" and the Scenarios "prominent". The owner declined an assertion library: "i do not want to use a libraty that introduces a risk".

The owner chose Deployment names by business capability over a technical role, "the lab is mainly scaffolding plus a few examples". A Scenario that fails a Node must say which. The owner rejected a Node name: "There is no way to deduce the Node name from the setup."

A Scenario carries a short Scenario name, which a run prints and a Simulated Cluster and Timeline file carry. The owner wanted every Scenario in one file: "let's go for the single file with all scenarios then", the name "(b) From the function name".

## Options Considered

What a Scenario is:

1. **A description value a runner interprets** — every new Step grows a second language beside Go.
2. **A Go function over a provisioned cluster** — the descriptions stay immutable values; the sequence is Go.
3. **`go test` functions** — unreachable from the binary and the Live view.

The shape of that function:

4. **Statements**, each line one Step on a value carrying the context, the cluster and the reporting.
5. **Declarative blocks** the runner interprets, which is option 1 again.
6. **Plain helpers** returning errors, which leaves the error plumbing in every Scenario.

How a Scenario says which Node fails:

7. **The Node's name**, which the Scenario's own Givens never state.
8. **A count the lab resolves** against the declared Topology.
9. **A position in the declared Topology**.

How a Scenario name is derived:

10. **A constant in the command** — adding a Scenario edits the command.
11. **The Scenario's file name** — needs one file per Scenario.
12. **The identifier before `_Should`, lowercased and hyphenated**.
13. **The whole identifier** — carries the outcome too and outgrows a container name.
14. **A name stated beside each function** — a second name to keep in step.

Where the Scenarios and their list stand:

15. **One file per Scenario** — a directory the owner found crowded.
16. **`All` in `scenarios.go`, the Scenarios in a second file** — the list apart from what it lists.
17. **`All` at the top of `scenarios.go`, every Scenario after it in `All`'s order**.

## Decision

Options 2, 4, 8 with 9 to follow, 12 and 17.

- A Scenario is an exported function taking one `*lab.Scenario` and returning nothing, one Step per line. The Scenario value holds the first failure, so no line names an error. A Then the Placement does not meet records a Finding, and later Steps still run.
- The Scenarios are the top-level `scenarios` package. `scenarios/scenarios.go` holds `All` first, a function rather than a variable, then every Scenario in its order; no other source holds one. The Scenario-place test asserts the layout, and that `All` names every Scenario exactly once.
- A function is named `<Condition>_Should<Outcome>`. Its Scenario name is the identifier before the first `_Should`, read from the Go runtime's record of the function, its words lowercased and hyphenated. `GPUNodes` reads `gpu-nodes`.
- A function with no condition, a closure, a method value, or a condition outside ASCII letters and digits is refused before any cluster, naming it. Two functions deriving one name are refused, naming both.
- A Scenario has no test of its own: it is itself the check. The `lab` command's suite runs every Scenario in `All` and fails on a Finding or a Step left undone.
- A When states how many Nodes or Zones stop responding, and the lab stops the first the Topology derives, naming them in its Step. A count above the Topology's is refused. The by-position form is a later slice, a second constructor beside the count.
- A Step mutates the Scenario alone; every description is an immutable `scenario` value built through its own constructor. A Then judges through the project's own `internal/expect`.
- A Deployment's name is a `lab.DeploymentName`. The lab's Scenarios take theirs from constants in one file, named for a business capability of the example domain rather than a technical role.
- A shortcut over the settings is a pure function expanding to them. How a Step's values are written is decided in [ADR: Scenario code stays idiomatic Go, read by subject](2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md).

Options 1 and 5 grow a second language. Option 3 cuts off the binary. Option 6 leaves error plumbing in every Scenario. Option 7 names what no Given states. Options 10, 11, 13 and 14 cost an edit elsewhere, a file per Scenario, an outgrown name, or a second name. Options 15 and 16 split what one file reads as an index.

## Consequences

One body of behavior serves a run and the Live view. A Scenario reads top to bottom and steps through in a debugger, and every Step it carried out is recorded in order. Renaming a function renames its Scenario, cluster and Timeline file. A Scenario name holds lowercase ASCII letters, digits and hyphens alone, so it is safe as a container name and a path element. The one file grows with every Scenario added.

## Implementation

**Requirements:** REQ-SIM-001, REQ-SIM-006, REQ-SIM-008, REQ-SIM-020

## References

- [system-design.md § Package Structure](../system-design.md#package-structure)
- [ADR: The lab is a Go program on a KWOK Simulated Cluster](2026-10-03-go-program-on-a-kwok-simulated-cluster.md)
- [ADR: A Scenario writes its own Wait, with its Given or as a Step of its own](2026-10-07-a-scenario-writes-its-own-wait.md)
- [ADR: Scenario code stays idiomatic Go, read by subject](2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md)
