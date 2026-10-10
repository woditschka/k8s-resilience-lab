# Every NodePool is named where it is declared

**Status:** Accepted, amended 2026-10-09: a NodePool's capacity is its third mandatory argument, per [ADR: Every NodePool states its Node capacity or names one its Scenarios define](2026-10-09-every-nodepool-states-or-names-its-node-capacity.md)

## Context

[ADR: A Topology declares the NodePools every Zone holds](2026-10-08-a-topology-declares-the-nodepools-every-zone-holds.md) kept `Topology(lab.Zones(3), lab.NodesPerZone(3))` as a short form: one unnamed NodePool whose Nodes carry no label. A name was an optional `.Named` call.

The owner found the short form "less clear" and dropped it. REQ-SIM-013 now states that every NodePool names itself and every Node carries that name. The lab's four Scenarios name their one NodePool general. The PRD refuses a NodePool stating no name and a Topology stating no NodePool.

The earlier ADR's label key and name rule stand. Its Topology-line shape, its unnamed NodePool and its Topology-level capacity do not.

## Options Considered

Where a Scenario states a NodePool's name:

1. **A further call, as before** — `lab.NodePool(lab.NodesPerZone(3)).Named("general")`. An unnamed NodePool compiles, so the refusal is a run-time end at the Topology line.
2. **The first mandatory argument** — `lab.NodePool("general", lab.NodesPerZone(3))`. An unnamed NodePool does not compile; only an empty name is left to refuse at run time.

What a Topology line takes after its Zone count:

3. **The closed interface over a Node count and NodePools** — two kinds, of which only one remains.
4. **NodePools alone** — `Topology(lab.Zones(3), pools ...lab.DeclaredNodePool)`, with no capacity call on the Topology line itself.

## Decision

Option 2, as the owner approved. `lab.NodePool` takes a `lab.NodePoolName` and then a `lab.NodesPerZone`, both defined types, so a swap does not compile. `.Named` is gone, and `.NodeCapacity` stays the NodePool's one further call. The domain's `NewNodePool` refuses an empty name, and an empty name ends the Scenario at the Topology line.

Option 4. The Topology line takes NodePools only, and the `HeldInEveryZone` interface is gone. A line of no NodePool ends the Scenario there, since a variadic list admits none. `DeclaredTopology.NodeCapacity` is gone. The domain keeps one Topology constructor over NodePools, and no Topology holds an unnamed NodePool.

Every Node carries `k8s-resilience-lab/node-pool` with its NodePool's name. The name rule, its pin to the client library, and declaration-order numbering stand as the earlier ADR records them.

## Consequences

Every Topology line reads the same way, and every Node is selectable by its NodePool. Each of the four Scenarios gains a NodePool name, and so does every test building a Topology.

The empty name and the empty Topology line stay run-time refusals at the Given, not compile errors.

## Implementation

**Requirements:** REQ-SIM-013

## References

- [system-design.md § Contracts](../system-design.md)
- [ADR: A Topology declares the NodePools every Zone holds](2026-10-08-a-topology-declares-the-nodepools-every-zone-holds.md)
- [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md)
