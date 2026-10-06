# A Scenario's Workload declares the spread of its own Pods

**Status:** Accepted

## Context

The simplest Scenario, Resilient placement, exists to show resilience to Node and Zone loss. Its nine Pods therefore have to sit three in every Zone and one on every Node.

An even spread is one Scenario's goal rather than the lab's law. A later Scenario declares a different specification, or one with no spread, and the lab reports whatever Placement it produces.

A spread also has to survive the loss it is built for. One the surviving Nodes cannot satisfy leaves Pods Pending, which is the less resilient outcome of the two the lab can show.

## Options Considered

1. **Even spread in the mapper** — every Workload spread the same way, and no Scenario able to declare otherwise.
2. **Required Pod anti-affinity over the hostname label** — at most one Pod per Node, absolutely.
3. **Preferred spread** — the scheduler aims for evenness and places the Pod anyway when it misses.
4. **Topology spread constraints declared per Workload** — a maximum Skew per domain, carried onto the Pod template.

## Decision

Option 4, enforced. A Workload's spread comes from the Pod specification it declares, which holds zero or more topology spread settings, per [ADR: A Workload declares one of the lab's predefined Pod specifications](2026-10-05-workload-declares-a-predefined-pod-specification.md). Each setting names its own topology key, maximum Skew, `whenUnsatisfiable` and `nodeTaintsPolicy`, and the mapper transcribes it onto the Deployment's Pod template with the Workload's own label as the selector. Resilient placement sets the Zone and the hostname label each at a maximum Skew of one, `DoNotSchedule`, and taints honoured in the count. The spread setting is the one a Scenario may also name as a Customization, per the same ADR. Every field used is generally available in the pinned Kubernetes version, per [ADR: Kubernetes version pin](2026-10-03-kubernetes-version-pin.md).

`DoNotSchedule` makes an unmet rule visible as a Pending Pod, which is what a Scenario asserts. Option 3 hides the same failure behind a Running Pod in the wrong place.

Option 2 is rejected on the lab's own goal. An absolute one-Pod-per-Node rule leaves the ninth Pod Pending as soon as a Node is lost. A maximum Skew of one lets that Pod double up on a surviving Node, keeping nine Pods Running — the outcome the Resilient placement Scenario is built to demonstrate.

Taints are honoured so a tainted Node leaves the count. A Node a Failure cordons therefore shrinks the domain set instead of making every rule unsatisfiable.

No constraint declares a minimum domain count, for the same reason option 2 is rejected, per [ADR: Provisioning ends when the scheduler has placed a Pod on every Node](2026-10-04-scheduler-observable-provisioning.md). That ADR carries the guarantee this rule needs: the scheduler has seen every Node of the Topology before a Workload is deployed.

## Consequences

The Resilient placement Scenario's assertion is a consequence of its own declaration, so the same reasoning reads from the Scenario rather than from the lab's internals.

Spread is evaluated when a Pod is scheduled and never after. A Node that returns from a Failure stays empty until Pods are recreated, so no Scenario asserts that a Placement re-balances on its own.

A specification that holds no spread is legal, and its Pods land wherever the scheduler puts them.

A Customization widening a spread's Skew, or naming another topology key, is the lab's way of showing a weaker spread without a second predefined specification.

A Topology too small for its Workload reports Pending Pods rather than an uneven Placement, which is the PRD's edge case 1.

## Implementation

**Requirements:** REQ-SIM-001, REQ-SIM-009

## References

- [system-design.md § Contracts](../system-design.md)
- [ADR: The lab is a Go program on a KWOK Simulated Cluster](2026-10-03-go-program-on-a-kwok-simulated-cluster.md)
- [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md)
