# A Topology declares the NodePools every Zone holds

**Status:** Superseded by [ADR-2026-10-09](2026-10-09-every-nodepool-is-named-where-it-is-declared.md)

## Context

REQ-SIM-013 lets one Zone hold Nodes of different kinds. The owner found the one-capacity Topology "to rigid" and settled on the term NodePool, the cloud providers' node pool. A NodePool is declared once, with a Node count per Zone, and spans every Zone.

`Topology(lab.Zones(3), lab.NodesPerZone(3))` stays the short form, so every Scenario of the lab reads as before. The PRD refuses a NodePool of no Node, a repeated name, two without a name, a name no label value can hold, and both forms at once.

Each Node carries its NodePool's name as a label. Which key carries it, and the words a Scenario declares NodePools in, were left to the design.

Every cloud provider labels a Node with its pool's name as a value under a key of its own. GKE, AKS, EKS node groups and Karpenter each do so.

## Options Considered

Which label carries a Node's NodePool:

1. **`node-role.kubernetes.io/<name>`** — the name sits in the key with an empty value, which `kubectl get nodes` prints as a role. The prefix is reserved to Kubernetes, the key says role rather than pool, and finding a Node's NodePool means scanning its keys.
2. **A lab-own key with the name as its value** — the providers' form under the prefix the lab's Workload and probe labels already carry, so one selector names one NodePool.

How a Scenario declares the NodePools:

3. **A second Given** — `Given().NodePools(lab.Zones(3), …)` beside `Topology`, so the short form and the long form are two Steps naming one value.
4. **A wrapper argument** — `Topology(lab.Zones(3), lab.EachZoneHolding(…))`, which adds a word that states nothing the line does not already say.
5. **What each Zone holds, as the arguments after the Zone count** — `Topology(lab.Zones(3), lab.NodePool(lab.NodesPerZone(2)), lab.NodePool(lab.NodesPerZone(1)).Named("gpu"))`. A Node count per Zone is one such argument, so the short form compiles unchanged.

Where a NodePool name is judged as a label value:

6. **The client library's validation, called from `internal/cluster`** — the domain constructor then refuses nothing about the name, and the validation home splits across two modules.
7. **The domain's own rule, pinned to the client library's** — `internal/scenario` holds the label-value syntax with the standard library, and a test in `internal/cluster` asserts it agrees with Kubernetes' own.

## Decision

Option 2. A Node of a named NodePool carries the label `k8s-resilience-lab/node-pool` with that name as its value. A Node of the unnamed NodePool carries no such label. The key is the mapper's own, beside the Workload label.

Option 5. `Given().Topology` takes the Zone count, then what each Zone holds: one `lab.NodesPerZone`, or one or more `lab.NodePool`. The two kinds satisfy one closed interface `internal/lab` alone implements. A NodePool's Node count per Zone is its mandatory named argument. Its name, `.Named`, and its capacity, `.NodeCapacity`, are further calls on the complete value, per [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md). `lab.Nodes` stays the When's count of Nodes stopped, so a NodePool counts its Nodes as `lab.NodesPerZone`, the owner's "Node count per Zone".

A NodePool the domain refuses is carried to the Topology line, which ends the Scenario naming the refusal, as a refused Pod specification is carried to its Workload line. Mixing a Node count per Zone with a NodePool is refused there. A Topology of NodePools refuses `.NodeCapacity` on its own line, the short form's capacity being part of the short form.

Option 7. `internal/scenario` judges a NodePool name against Kubernetes' label-value syntax, which the domain may not import. A test in `internal/cluster` pins that rule to the client library's own validation over the same names. The API server refuses a value that escaped both.

A Topology holds its NodePools in declaration order. A Zone's Nodes are numbered from one through them, so Node names stay positional and a When's resolution order is the Topology's.

## Consequences

Every Scenario source of the lab compiles and reports unchanged. A Scenario declares a Zone of mixed Nodes in one line, readable top to bottom in the order the Nodes are numbered.

A later Step can select a NodePool's Nodes by one label selector, and a spread can count NodePools as domains under that key. Taints, labels and growth limits per NodePool are further calls on the same value.

The Topology line accepts argument lists the PRD refuses, so those refusals are run-time ends at the Given rather than compile errors. A hand-written label-value rule exists in the domain, held to the library's by a test rather than by the library itself.

## Implementation

**Requirements:** REQ-SIM-013

## References

- [system-design.md § Contracts](../system-design.md)
- [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md)
- [ADR: A Workload declares one of the lab's predefined Pod specifications](2026-10-05-workload-declares-a-predefined-pod-specification.md)
- [ADR: Provisioning ends when the scheduler has placed a Pod on every Node](2026-10-04-scheduler-observable-provisioning.md)
