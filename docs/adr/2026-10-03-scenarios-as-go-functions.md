# Scenarios are Go functions over immutable descriptions

**Status:** Accepted

## Context

A Scenario sets up a Topology and a Workload, injects a Failure, and checks the outcome. The owner asked whether "the k8s tests could be just the go program and not test". The question is where that behavior lives: a `go test` suite, or the binary that also drives the live view (G-2).

The description has to read well at two levels: a high-level shortcut such as a Zone count, and detailed control over Zone, Node, and Workload settings. The owner declined an assertion library — "i do not want to use a libraty that introduces a risk" — and chose the project's own `Expect(t, got).To(matcher)`.

The first Scenario written this way reached 130 lines, most of them error plumbing and expectation helpers. The owner asked for "an internal dsl - setup a bit given when then style", small to read and to write. The owner also asked for the machinery out of sight: "the 'framework' should be more hidden e.g. within internal, the scenarios of the lab should be prominent".

A Scenario also names the Workload it deploys. The owner found the first name unconvincing — "i find \"web\" a bit strange what would you see as workload name in a distributed system ?" — and chose naming by business capability over a technical role. Any domain is possible: "the lab is mainly scaffolding plus a few examples" (tidied). The Workloads the lab ships are one example domain's, not a vocabulary the lab fixes. A Scenario carries two names besides. Its function the owner asked to read "like the tests, without 'Test' at the start" (tidied). Its run name, the short one a run prints and a Simulated Cluster carries, was a constant in the command beside the line handing the function to the runner. The owner also asked (tidied): "Tests are not shipped with the binary — do we need tests at all for the Scenario?"

A Scenario that injects a Node Failure also says which Node fails. The first design passed the Node's name, which the owner rejected: "There is no way to deduce the Node name from the setup." The owner chose two forms instead, a count as the usual one and a position where a Scenario needs the Failure in a particular place.

## Options Considered

1. **A `Scenario` description value a runner interprets** — every new step grows a second language beside Go.
2. **A Scenario as a Go function over a provisioned cluster** — the descriptions stay immutable values; the sequence is Go.
3. **Scenarios as `go test` functions** — the live view then has no path to the same behavior (G-2), and the Scenarios would be unreachable from the binary.

The shape of that function, with option 2 in force:

4. **Statements** — each line one Given, When or Then Step on a value carrying the context, the cluster and the reporting.
5. **Declarative blocks** — a Scenario hands the runner nested literals it interprets, which is option 1 again.
6. **Plain helpers** — package functions over the cluster, each returning an error, which leaves the error plumbing in every Scenario.

How a Step's mandatory values are passed, with option 4 in force:

7. **Plain arguments** — `Topology(3, 3)`, where each number's meaning depends on knowing the argument order.
8. **Named arguments** — `Topology(lab.Zones(3), lab.NodesPerZone(3))`, where each line says what its numbers mean.
9. **A chain accumulating them** — `Workload(Checkout).Pods(9)`, which leaves an incomplete value between the calls.

How a Then after a When judges, with option 4 in force:

13. **One observation per Scenario** — every Then judges the Placement the Scenario first settled at, so a Then after a When judges the Placement from before the Failure.
14. **A When discards the observation it injected into** — the next Step needing a Placement awaits a fresh Settled one, and no Scenario line states a wait.
15. **A settle Step a Scenario writes** — the Scenario carries a waiting time of its own, which the owner declined: "Scenarios stay fast and need no wait setting."

How a Scenario says which Node fails, with option 4 in force:

16. **The Node's name** — `NodeFails(lab.NodeName("zone-1-node-1"))`, a name the Scenario's own Givens never state.
17. **A count the lab resolves** — `NodesFail(lab.Nodes(1))`, resolved against the declared Topology and reported by the Step.
18. **A position in the declared Topology** — `NodeFails(lab.InZone(1), lab.Node(1))`, which says where the Failure lands.

How a Scenario's run name is derived, with option 4 in force:

10. **A constant in the command** — the name sits beside the line handing the function to the runner, so adding a Scenario edits the command.
11. **The Scenario's file name** — the Go runtime's record of where the function is defined yields the name; renaming the file renames the Scenario.
12. **The Scenario's function name** — a second convention shortens the long name, and has to hold for every shape a function name may take.

## Decision

Options 2, 4 and 8. A Scenario is an exported Go function taking one `*lab.Scenario` and returning nothing, one Step per line. The Scenario value holds the first failure, so a Step that cannot be carried out ends the Scenario and names itself, and no line names an error. A Then the Placement does not meet records a Finding, and the later Steps still run.

The lab's Scenarios are the top-level `scenarios` package with the README naming them; the machinery stays under `internal/`, and `internal/lab` is the one package a Scenario source imports. The list of them is one exported function beside them, one line per Scenario, so adding a Scenario touches that place alone. It is a function rather than a variable, because no package of the lab holds mutable state. A Scenario's function is named `<Condition>_Should<Outcome>`, the shape `docs/testing-principles.md` § Test Naming gives a test without its `Test` prefix. Option 11: the run name is the stem of the Scenario's file, with hyphens for underscores. A stem that is not a Scenario file name stops the run, naming the function and the file, so a name is never guessed or empty. The Scenario-place test asserts the function-name shape and that the list names every Scenario of the place exactly once, so a Scenario cannot exist unrun. A Scenario has no test of its own: it is itself the check, and a test beside it could only repeat its own Then Steps. The place holds production files alone. The `lab` command's own suite runs every Scenario the list holds, and fails when one reports a Finding or ends at a Step.

Option 14. A When injects one Failure, then discards the observation it injected into, so what the next Then judges is the Placement the control plane reached after the Failure. A Then judging what a When left reads that Failure from the Scenario value; one with no When before it ends the Scenario at its own line.

The implicit wait of option 14 is superseded by [ADR: A Scenario writes its own Wait, with its Given or as a Step of its own](2026-10-07-a-scenario-writes-its-own-wait.md). A Scenario now writes each Wait as a Step of a fourth kind beside Given, When and Then, named in Kubernetes' own words. A Given declares, and the Wait after it applies what the Givens before it declared. A Failure meets the Placement the last Wait reached. A waiting time a Scenario names itself, option 15's, stays declined and is a later slice.

Options 17 and 18, the count first. A When states how many Nodes stop responding, as a named argument, and the lab stops the first Nodes the declared Topology derives. A Topology derives its own names in a fixed order, so that is the same Node on every run, and the When Step names the Nodes it stopped. The `scenario` constructor refuses a count above the Nodes the Topology holds. Both forms resolve to Node names in that one constructor, so the by-position form is a second constructor beside it, leaving the Step surface, the Failure value and every Then unchanged. The by-position form is a later slice, for a Scenario that controls where its Failure lands.

A Step is one complete call. Its mandatory values are named arguments: `Zones`, `NodesPerZone` and `Pods` are defined integer types in `internal/lab`, so a swapped argument does not compile. An optional setting is a further call on the complete value, taking the same named arguments, `Topology(lab.Zones(3), lab.NodesPerZone(3)).NodeCapacity(lab.Pods(2))`. A chain accumulating mandatory values, `Workload(Checkout).Pods(9)`, is a builder in substance and is rejected. The Construction and update row of `docs/architecture-principles.md` § Pattern Catalog admits one entry point per type with `with` copies, so a chain stopped anywhere stays a complete value. A Workload's name is a `WorkloadName`, which a Scenario may pass as a literal. The lab's own Scenarios take theirs from the constants in one file beside them, so the Workloads they deploy have one home. The constants name business capabilities, of whatever domain the Scenarios beside them model; neither the Step surface nor the rule knows that domain. A count type holds no invariant, because the invariant belongs to the description: `scenario.NewTopology` and `scenario.NewWorkload` refuse an invalid count, and the Given that named it ends the Scenario at its own line. A Step mutates the Scenario alone; every description it builds is an immutable `scenario` value through that type's own constructor. A Then judges through the project's own `internal/expect` rather than a library, over the minimal interface that `*testing.T` and a Then Step both satisfy. A shortcut over these settings is a pure function expanding to them, adding no setting of its own. Node capacity is a setting on the description, never a literal in the mapper, because the PRD's first edge case needs a Workload exceeding its Topology.

## Consequences

One body of behavior serves the unattended run and the live view, so neither needs a second code path. A Scenario reads top to bottom and steps through in a debugger. Every Step it carried out is recorded in order, which is what a run reports and what the live view reads later. Renaming a Scenario's file renames the Scenario, which is what a run prints and what its Simulated Cluster carries. A convenience that cannot be expressed as a combination of existing settings is the signal that a setting is missing. A Scenario studying a Failure reads as the Scenario before it plus one When line, and states no waiting time. Every Node it fails is readable from its own Givens, and the run's Step line names the Node that failed. A count says nothing about where the Failure lands, which is what the by-position form is for. A Scenario pays one settle wait per When, so a long sequence of Whens is a slow Scenario. Per-Zone asymmetry, a disruption budget, and a Workload read from a manifest file are later slices, each a further Step over the same values.

## Implementation

**Requirements:** REQ-SIM-001, REQ-SIM-006, REQ-SIM-008

## References

- [system-design.md § Contracts](../system-design.md)
- [ADR: The lab is a Go program on a KWOK Simulated Cluster](2026-10-03-go-program-on-a-kwok-simulated-cluster.md)
- [ADR: A Node Failure withholds the Node's status and shortens the eviction wait](2026-10-05-node-failure-withholds-the-nodes-status.md)
- [ADR: A Scenario writes its own Wait, with its Given or as a Step of its own](2026-10-07-a-scenario-writes-its-own-wait.md)
