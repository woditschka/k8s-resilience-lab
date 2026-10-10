# Every NodePool states its Node capacity or names one its Scenarios define

**Status:** Accepted

## Context

REQ-SIM-015 holds that every value deciding a Placement stands on a Scenario's own lines, stated there or named there, and that the lab gives no such value silently. Until now a NodePool stating no capacity took the lab's own, and a stated capacity omitting its Pod count took the lab's own count. The owner accepted the product expert's recommendations: a NodePool states cpu, memory and Pod count, all three, or names a Named Node capacity.

A first design kept Standard's values in `internal/scenario`, offered it as `lab.StandardNodes()`, and reported it on a run-start settings line. Reviewing it, the owner asked whether the size of a Standard Node, living deep in the framework, should not live in the Scenarios, and redirected the slice to them. REQ-SIM-015 now holds that the Scenarios define each Named Node capacity once, beside the Scenarios using it, and the lab defines none. The Topology's Given line reports what each name stands for.

Two recorded decisions stand in the way. [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md) made the Pod count a trailing variadic argument of the capacity call. [ADR: Every NodePool is named where it is declared](2026-10-09-every-nodepool-is-named-where-it-is-declared.md) kept `.NodeCapacity` as the NodePool's one optional further call.

## Options Considered

Where a NodePool states its capacity:

1. **The further call, made obligatory**: `lab.NodePool("general", lab.NodesPerZone(3)).NodeCapacity(...)`, with a NodePool lacking the call ending the Scenario at the Topology line. A missing capacity still compiles.
2. **The third mandatory argument**: `lab.NodePool("general", lab.NodesPerZone(3), StandardNodes())`, or `lab.NodeCapacity(lab.CPU(4), lab.MemoryGi(16), lab.Pods(10))` in its place. A missing capacity does not compile.

How a stated capacity takes its Pod count:

3. **A trailing variadic, refused at none or more than one**: run-time refusals at the Given.
4. **A mandatory named argument, last**: a capacity of cpu and memory alone does not compile.

Where a Named Node capacity is defined:

5. **In `internal/scenario`, offered by `internal/lab`**: the domain holds each name with its values in one list, and the Step surface offers one function per name. This was the first design.
6. **In the Scenarios' place, through a Step-surface mechanism**: `internal/lab` offers `lab.NamedNodeCapacity(name, cpu, memory, pods)`, and a file beside the Scenarios defines each name once with it.

How the Scenarios' place holds a definition:

7. **A package-level variable**: `var StandardNodes = lab.NamedNodeCapacity(...)`.
8. **An exported function taking nothing**: `func StandardNodes() lab.DeclaredNodeCapacity`, returning that one call.

Where a run reports what a name stands for:

9. **A settings line when the run starts**: one line per name the lab defines.
10. **The Topology's Given line**: the name, then the cpu, memory and Pod count its definition gives.

## Decision

Option 2. The Scenario ADR's rule makes a mandatory value a named argument and an optional setting a further call. Capacity is now mandatory, so it moves into the argument list, as the NodePool's name did. `DeclaredNodePool.NodeCapacity` is gone.

Option 4, for the same rule. The capacity reads cpu, memory, then Pod count, the order every report names.

Option 6. Option 5 put a value deciding a Placement inside the framework, which REQ-SIM-015 now refuses. The lab keeps the mechanism and the Scenarios own the values. The domain's `NewNamedNodeCapacity` builds a capacity under a name, refusing an empty name, and holds no name of its own. `lab.NamedNodeCapacity` states all three values, as `lab.NodeCapacity` does, under the name. `scenarios/nodes.go` defines each name, as `scenarios/workloads.go` holds each Workload name.

Option 8. No package of the lab holds mutable process-wide state, and a Go variable stays assignable. A function taking nothing returns the same value on every call, as `scenarios.All` does. Each function is named for the Nodes it describes: `StandardNodes()` reads on a Topology line as the Nodes the NodePool holds.

Option 10. The lab defines no name, so it has no list to report when a run starts. REQ-SIM-014 keeps each Scenario's lines together, so the values stand where the Scenario names them. A named NodePool reads as "a NodePool general of 3 Standard Nodes holding 16 cpu, 64Gi memory and 10 Pods each", the stated form with the name before "Nodes".

Both capacity forms return one value carrying the domain capacity, or the domain's refusal, beside the three values written and the name where named. A NodePool carrying a refusal ends the Scenario at the Topology line, as it does today. The domain's `NewNodePool` refuses the zero capacity no constructor builds, so a NodePool stating no capacity is refused in the domain too.

## Consequences

A Scenario's Topology line shows every value its Nodes hold, under the name where it names one. The two omissions REQ-SIM-011 and REQ-SIM-013 refuse are compile errors on the Step surface, with the zero capacity refused at run time. The lab's own capacity, the Standard entry in `internal/scenario`, the list of names and the run-start "Standard Node:" line are retired. Standard keeps the values the retired default held, so every Scenario keeps its asserted outcome.

The Scenarios' place admits a file of Named Node capacities beside its file of Workload names. A source rule holds that file to definitions alone and each name to one definition. No source of the lab outside that place calls `lab.NamedNodeCapacity`.

Two definitions under one name, each in its own Scenario, are not refused within a run. Each Given line reports the values its own definition gives. The place rule keeps the lab's own Scenarios to one definition per name.

A further named capacity, such as Large or GPU, is one function in `scenarios/nodes.go`, such as `LargeNodes()`, arriving with the first Scenario that uses it. The lab's code does not change.

## Implementation

**Requirements:** REQ-SIM-015, REQ-SIM-011, REQ-SIM-013

## References

- [system-design.md § Contracts](../system-design.md)
- [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md)
- [ADR: Every NodePool is named where it is declared](2026-10-09-every-nodepool-is-named-where-it-is-declared.md)
