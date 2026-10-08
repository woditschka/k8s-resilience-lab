# Scenarios

The lab's experiments. A Scenario is one file here, holding one exported Go function over `*lab.Scenario` whose every line is one Step. Nothing of the machinery that runs them lives here: the Step surface and the runner are `internal/lab`.

Two files sit beside them. `scenarios.go` holds `All`, the list of the Scenarios a run carries out. `workloads.go` holds the names of the Workloads they deploy, one constant each.

Run them all with `go run ./cmd/lab`.

## How one works

A Scenario writes these kinds of Step:

- A **Given** declares the Topology or the Workload, and reaches no cluster.
- A **When** injects the Failure.
- A **Wait** applies what the Givens above it declared, holds until the cluster reports the state it names, and reports the time it took where it was reached and the time it was held to where it was not.
- A **Then** judges the Placement the last Wait reached, and records a Finding while the later Steps still run.

A Then and a Failure both read the Placement the last Wait reached, and a Failure discards it. So a Wait stands before the first Then, and another between a Failure and the Then after it. A Scenario missing either ends at that Step rather than judging a world that is still changing.

A Wait is written as a Step of its own, `s.WaitFor().AllPodsReady()`, or on the Given whose declaration it fits, `s.Given().Topology(…).WaitFor().AllNodesReady()`. Both forms are a Wait Step. The Scenarios here write the standalone one.

## The vocabulary a Scenario writes

Each name below is defined in `internal/lab`, which stays the authority on what it does.

**Givens**

- `Given().Topology(lab.Zones, lab.NodesPerZone)`, with `.NodeCapacity(lab.CPU, lab.MemoryGi)` for the room each Node has, and a trailing `lab.Pods` where how many Pods a Node holds is what the Scenario is about
- `Given().Workload(name, lab.Pods, specification)`, with `.RequiredCapacity(lab.Pods)` for the fewest Pods it keeps Running through a Failure
- the Workload constants of `workloads.go`, business capabilities of the domain the Scenarios model

**Pod specifications**

- `lab.ResilientPlacement()` — its Pods spread as evenly over the Zones and over the Nodes as the control plane can place them
- `lab.DefaultPlacement()` — no spread, so where its Pods land is the scheduler's own choice
- `.Customize(…)` on a specification — that specification with the lab's own settings changed
- `lab.FullAccess(…)` — the Customization carrying a Scenario's own change to the Kubernetes Pod specification

**Failures**

- `When().NodesFail(lab.Nodes)` — that many Nodes stop responding
- `When().ZonesFail(lab.Zones)` — every Node of that many Zones stops responding at once

**Waits**

- `WaitFor().AllNodesReady()` and `WaitFor().AllNodesReadyWithinSeconds(seconds)`
- `WaitFor().AllPodsReady()` and `WaitFor().AllPodsReadyWithinSeconds(seconds)`
- `WaitFor().PlacementSettled()` and `WaitFor().PlacementSettledWithinSeconds(seconds)` — the Placement alone, so it is reached while Pods stay Pending
- the first four on the Given that declares the Topology or the Workload

A Wait stating no time is held to the lab's own waiting time, which every run reports at its start. A stated time is marked as stated on the line, so it is never read for the time the Wait took.

**Thens**

- `Then().AllPodsRunning()`
- `Then().PodsPerZone(pods)`
- `Then().PodsPerNode(pods)`
- `Then().PodsPending(pods)` — how many Pods the Placement leaves Pending, which is what no Node had room for
- `Then().NoPodsOnTheFailedNodes()`
- `Then().NoPodsInTheFailedZones()`
- `Then().RequiredCapacityHeld()` — the declared Required Capacity, judged at the moment of the Failure and once the Placement has settled

## The Scenarios

- **resilient-placement**, `resilient_placement.go` — the lab's simplest Scenario and its starting point. A Workload declares Resilient placement on Nodes with room for more than one of its Pods, so the even Placement is the scheduler honouring that specification.
- **node-stops-responding**, `node_stops_responding.go` — the Resilient placement world, and then one Node stops responding: what the control plane restores, and the capacity held through it.
- **zone-stops-responding**, `zone_stops_responding.go` — the same world, and then every Node of one Zone stops responding at once: the capacity surviving a lost Zone.
- **no-room-after-a-zone-stops-responding**, `no_room_after_a_zone_stops_responding.go` — the lost Zone again, on Nodes with room for one Pod each: six of the nine Pods are Running, the three with nowhere to go stay Pending, and the declared Required Capacity of six is still held.

A Scenario's Steps are read in its source.

## Writing one

The function is named for the condition it runs under and the outcome it asserts, `<Condition>_Should<Outcome>`.

```go
func AWorkloadPlacedForResilience_ShouldPlaceThreePodsPerZoneAndOnePerNode(s *lab.Scenario) {
	s.Given().Topology(lab.Zones(3), lab.NodesPerZone(3))
	s.WaitFor().AllNodesReady()
	s.Given().Workload(Checkout, lab.Pods(9), lab.ResilientPlacement())
	s.WaitFor().AllPodsReady()
	s.Then().PodsPerZone(3)
}
```

Every line is one Step opened on `s`, so a reader scans the Step kinds in one column and a line is inserted, removed or reordered on its own. The Scenarios here are held to that layout by a rule test, per [the layout ADR](../docs/adr/2026-10-07-the-labs-scenarios-write-one-step-per-line.md).

The Scenario runs under the stem of its file, with hyphens for underscores, so renaming the file renames the Scenario. A new Scenario is a file here and one line in `scenarios.go`.

A Scenario's mandatory values are passed as named arguments — `lab.Zones`, `lab.NodesPerZone`, `lab.Pods`, `lab.WorkloadName` and the specification — so a swapped argument does not compile. An optional setting is a further call on the complete declaration, `.NodeCapacity` on the Topology and `.RequiredCapacity` on the Workload. `.NodeCapacity` carries an optional value of its own, the Pod count, which comes last and keeps the lab's own count where the line leaves it out.

A Scenario has no test of its own: it is itself the check, and a test beside it could only repeat its own Then Steps. The lab's Scenarios run in `cmd/lab`'s own suite, which fails when one reports a Finding or ends at a Step.
