# A Topology declares the NodePools every Zone holds

**Status:** Accepted, amended 2026-10-09: every NodePool is named as its first argument and a Topology takes NodePools alone, options 4 and 8; amended 2026-10-09: a NodePool states its capacity or names one its Scenarios define, as its third argument, options 12, 14, 16, 18 and 20; amended 2026-10-18: `lab.NodePool`, `lab.NodePoolName`, `lab.NodesPerZone` and `lab.NamedNodeCapacity` are now `node.Pool`, `node.PoolName`, `node.PerZone` and `node.Named`, and a capacity is the keyed literal `node.Capacity`, per [ADR: Scenario code stays idiomatic Go, read by subject](2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md); amended 2026-10-18: a NodePool states its own Node labels, options 22, 25 and 27

## Context

On 2026-10-08, REQ-SIM-013 let one Zone hold Nodes of different kinds. The owner found the one-capacity Topology "to rigid" and settled on the term NodePool, the cloud providers' node pool. A NodePool is declared once, with a Node count per Zone, and spans every Zone. Every cloud provider labels a Node with its pool's name as a value under a key of its own: GKE, AKS, EKS node groups and Karpenter each do so.

On 2026-10-09 the owner found the short form `Topology(lab.Zones(3), lab.NodesPerZone(3))` "less clear" and dropped it. REQ-SIM-013 now states that every NodePool names itself and every Node carries that name. The PRD refuses a NodePool stating no name and a Topology stating no NodePool.

The same day, REQ-SIM-015 held that every value deciding a Placement stands on a Scenario's own lines, stated there or named there. The owner accepted the product expert's recommendations: a NodePool states cpu, memory and Pod count, all three, or names a Named Node capacity. A first design kept Standard's values in `internal/scenario`. Reviewing it, the owner asked whether the size of a Standard Node, living deep in the framework, should not live in the Scenarios, and redirected the slice to them.

On 2026-10-18, REQ-SIM-017 let a NodePool state labels its Nodes carry, beside the lab's own Zone, hostname and NodePool labels. A Then counts a Workload's Pods per value of such a label. The owner answered the scoping questions with "yes": no rack concept, a Scenario spreading over on-demand and spot, and a Failure by label in the next slice.

## Options Considered

Which label carries a Node's NodePool:

1. **`node-role.kubernetes.io/<name>`**: the name sits in a key reserved to Kubernetes that says role rather than pool.
2. **A lab-own key with the name as its value**: the providers' form under the lab's prefix, so one selector names one NodePool.

Where a Scenario states a NodePool's name:

3. **A further call, `.Named`**: an unnamed NodePool compiles. Chosen on 2026-10-08.
4. **The first mandatory argument**: an unnamed NodePool does not compile.

What a Topology line takes after its Zone count:

5. **A second Given**, `Given().NodePools(…)`: two Steps naming one value.
6. **A wrapper argument**, `lab.EachZoneHolding(…)`: a word stating nothing the line does not say.
7. **A closed interface over a Node count per Zone and NodePools**: the short form compiles unchanged. Chosen on 2026-10-08.
8. **NodePools alone**: `Topology(zones, pools ...node.DeclaredPool)`.

Where a NodePool name is judged as a label value:

9. **The client library's validation, called from `internal/cluster`**: the validation home splits across two modules.
10. **The domain's own rule, pinned to the client library's by a test**.

Where a NodePool states its capacity:

11. **A further call, made obligatory**: a missing capacity still compiles.
12. **The third mandatory argument**: a missing capacity does not compile.

How a stated capacity takes its Pod count:

13. **A trailing variadic**: refused at none or more than one at run time.
14. **A mandatory named argument, last**: a capacity without one does not compile. Replaced on 2026-10-18 by the keyed literal's `Pods` field.

Where a Named Node capacity is defined:

15. **In `internal/scenario`, offered by `internal/lab`**: a value deciding a Placement inside the framework.
16. **In the Scenarios' place, through a Step-surface mechanism**.

How the Scenarios' place holds a definition:

17. **A package-level variable**: a Go variable stays assignable.
18. **An exported function taking nothing**.

Where a run reports what a name stands for:

19. **A settings line when the run starts**.
20. **The Topology's Given line**.

How a NodePool states its labels:

21. **A further argument of `node.Pool`**: every NodePool line writes an empty map.
22. **A further call, `.Labels(node.Labels{…})`**, taking Kubernetes' label map.
23. **A slice of keyed pairs**: Kubernetes has no such type.

Which keys stay the lab's:

24. **The three the PRD names**: a stated label could take the simulator's selection label, so a Failure could no longer stop that Node.
25. **Every key the lab writes on a Node, and every key under the lab's prefix**.

Where a Then reads a Node's labels:

26. **The declared Topology**: judges what the Scenario asked for.
27. **The observed Node**: judges what the control plane holds.

## Decision

Options 2, 4, 8, 10, 12, 14, 16, 18, 20, 22, 25 and 27.

- `Given().Topology` takes the Zone count, then one or more `node.Pool`. A line of no NodePool ends the Scenario there.
- `node.Pool` takes a `node.PoolName`, a `node.PerZone` and a `node.Capacity`, in that order. The defined types make a swap a compile error.
- A capacity is a `node.Capacity` literal of `CPU`, `Memory` and `Pods`, or one the Scenarios name through `node.Named`. A dimension left out is refused at the Topology Given.
- `scenarios/nodes.go` defines each Named Node capacity once, as a function taking nothing, such as `StandardNodes()`. The lab defines none.
- `.Labels(node.Labels{…})` adds a NodePool's own labels and may be written more than once. The lab's own keys are those it writes on every Node and every key under its prefix.
- Every Node carries `k8s-resilience-lab/node-pool` with its NodePool's name, written by the mapper after the stated labels.
- `internal/scenario` holds the label key and value syntax and the lab's own keys without a Kubernetes import. A test in `internal/cluster` pins them to the client library and to what the mapper writes.
- The simulator's selection label stays a literal of `internal/cluster`, pinned by the same test to the domain's lab-own keys.
- A NodePool the domain refuses ends the Scenario at the Topology Given, naming the refusal. The refusals are an empty name, a refused capacity, a lab-own key, a key or value outside the syntax, and one key stated twice.
- The Given line reports each NodePool's name, Node count and capacity, the named one with its values, then its labels as `key=value` in key order.
- A Topology holds its NodePools in declaration order, and a Zone's Nodes are numbered through them, so Node names stay positional.
- An observation reads every label a Node carries. A Then counting Pods per value judges every value the NodePools state, and a key no NodePool states ends the Scenario at that Then.

The Scenario ADR's rule makes a mandatory value an argument and an optional setting a further call. Name and capacity became mandatory, so each moved into the argument list. Labels stay optional, so they stay a call.

The dated course: on 2026-10-08 a Topology took a Node count per Zone or NodePools, a name was the optional `.Named`, and capacity the optional `.NodeCapacity`. On 2026-10-09 the name became the first argument and the short form went. Later that day capacity became the third argument, with Named capacities defined beside the Scenarios. On 2026-10-18 the names moved to package `node` and the labels call joined.

## Consequences

A Scenario's Topology line shows every value its Nodes hold, readable top to bottom in the order the Nodes are numbered. Every Node is selectable by its NodePool, and a spread can count NodePools or stated label values as domains.

A NodePool reads as a manifest: `node.Pool("spot", node.PerZone(1), StandardNodes()).Labels(node.Labels{CapacityType: "spot"})`. A Scenario of one NodePool names it general. A further Named capacity is one function in `scenarios/nodes.go`, and the lab's code does not change.

The empty name, a capacity dimension left out and every label refusal are run-time ends at the Given rather than compile errors. Two definitions under one name in two Scenarios are not refused within a run; the place rule keeps the lab's own Scenarios to one.

A hand-written label syntax lives in the domain, held to the library's by a test rather than by the library. A Node in the domain carries its labels as an immutable value, so two Nodes compare through an equality method rather than `==`.

## Implementation

**Requirements:** REQ-SIM-011, REQ-SIM-013, REQ-SIM-015, REQ-SIM-017

## References

- [system-design.md § Contracts](../system-design.md#contracts)
- [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md)
- [ADR: Provisioning ends when the scheduler has placed a Pod on every Node](2026-10-04-scheduler-observable-provisioning.md)
- [ADR: A Workload declares one of the lab's predefined Pod specifications](2026-10-05-workload-declares-a-predefined-pod-specification.md)
- [ADR: Scenario code stays idiomatic Go, read by subject](2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md)
