# A Scenario's Workload declares the spread of its own Pods

**Status:** Accepted

## Context

The main Scenario, Even spread, exists to show resilience to Node and Zone loss. Its nine Pods therefore have to sit three in every Zone and one on every Node.

Even spread is one Scenario's goal rather than the lab's law. A later Scenario declares different rules, or none, and the lab has to report whatever Placement those rules produce.

The rules also have to survive the loss they are built for. A rule the surviving Nodes cannot satisfy leaves Pods Pending, which is the less resilient outcome of the two the lab can show.

## Options Considered

1. **Even spread in the mapper** — every Workload spread the same way, and no Scenario able to declare otherwise.
2. **Required Pod anti-affinity over the hostname label** — at most one Pod per Node, absolutely.
3. **Preferred spread** — the scheduler aims for evenness and places the Pod anyway when it misses.
4. **Topology spread constraints declared per Workload** — a maximum Skew per domain, carried onto the Pod template.

## Decision

Option 4, enforced. A Workload carries zero or more spread rules, each naming a domain — the Zones or the Nodes — and a maximum Skew. The mapper turns each rule into one topology spread constraint on the Deployment's Pod template: the Zone or hostname label as the topology key, the declared Skew, `DoNotSchedule`, taints honoured in the count, and the Workload's own label as the selector. Even spread declares one rule per domain at a maximum Skew of one. Every field used is generally available in the pinned Kubernetes version, per [ADR: Kubernetes version pin](2026-10-03-kubernetes-version-pin.md).

`DoNotSchedule` makes an unmet rule visible as a Pending Pod, which is what a Scenario asserts. Option 3 hides the same failure behind a Running Pod in the wrong place.

Option 2 is rejected on the lab's own goal. An absolute one-Pod-per-Node rule leaves the ninth Pod Pending as soon as a Node is lost. A maximum Skew of one lets that Pod double up on a surviving Node, which keeps nine Pods Running — the outcome Even spread is built to demonstrate.

Taints are honoured so a tainted Node leaves the count. A Node a Failure cordons therefore shrinks the domain set instead of making every rule unsatisfiable.

No constraint declares a minimum domain count, for the same reason option 2 is rejected, per [ADR: Provisioning ends when the scheduler has placed a Pod on every Node](2026-10-04-scheduler-observable-provisioning.md). That ADR carries the guarantee this rule needs: the scheduler has seen every Node of the Topology before a Workload is deployed.

## Consequences

Even spread's assertion is a consequence of its own declaration, so the same reasoning reads from the Scenario rather than from the lab's internals.

Spread is evaluated when a Pod is scheduled and never after. A Node that returns from a Failure stays empty until Pods are recreated, so no Scenario asserts that a Placement re-balances on its own.

A Workload that declares no rule is legal, and its Pods land wherever the scheduler puts them.

A Topology too small for its Workload reports Pending Pods rather than an uneven Placement, which is the PRD's edge case 1.

## Implementation

**Requirements:** REQ-SIM-001

## References

- [system-design.md § Contracts](../system-design.md)
- [ADR: The lab is a Go program on a KWOK Simulated Cluster](2026-10-03-go-program-on-a-kwok-simulated-cluster.md)
- [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md)
