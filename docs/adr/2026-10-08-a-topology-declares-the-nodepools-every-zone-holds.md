# A Topology declares the NodePools every Zone holds

**Status:** Accepted

## Context

REQ-SIM-013 lets one Zone hold Nodes of different kinds. The owner found a one-capacity Topology "to rigid" and settled on NodePool, the cloud providers' node pool: declared once with a Node count per Zone, spanning every Zone. Every provider labels a Node with its pool's name under a key of its own. Every NodePool names itself, and the owner found the short form without NodePools "less clear".

Every value deciding a Placement stands on a Scenario's own lines (REQ-SIM-015). The owner asked whether the size of a Standard Node, "deep in the framework", should not live in the Scenarios.

A NodePool may state labels a Then counts Pods by (REQ-SIM-017), and NoSchedule taints that keep a general Deployment off it (REQ-SIM-019). The owner scoped taints to NoSchedule alone, a tolerating Deployment reaching the NodePool by its Request.

## Options Considered

How a Node carries its NodePool:

1. **`node-role.kubernetes.io/<name>`** — a key reserved to Kubernetes that says role rather than pool.
2. **A lab-own key with the name as its value**, the providers' form.

What a Topology takes, and how a NodePool states its name and capacity:

3. **A Node count per Zone, or NodePools** — the short form the owner dropped.
4. **A second Given, or a wrapper argument** — two Steps for one value, or a word stating nothing.
5. **NodePools alone**, each with its name, Node count and capacity as mandatory arguments.
6. **Name or capacity as a further call** — a NodePool missing one compiles.

Where a Named Node capacity is defined:

7. **In `internal/scenario`** — a Placement-deciding value in the framework.
8. **In the Scenarios' place, as a function taking nothing**; a package variable stays assignable.

Where label and taint syntax is judged:

9. **The client library, called from `internal/cluster`** — the validation splits across two modules.
10. **The domain's own rules, pinned to the client library by a test**.

How a NodePool states labels and taints:

11. **A further argument** — every NodePool line writes an empty map.
12. **Further calls, `.Labels(node.Labels{…})` and `.Taints(node.Taint{…})`**, under Kubernetes' names.
13. **A taint folded into the labels** — a taint is no label, and its effect has no place there.

Which keys stay the lab's, and where a Then reads them:

14. **The three keys the PRD names** — a stated label could take the simulator's selection label.
15. **Every key the lab writes, and every key under its prefix**.
16. **The declared Topology** — judges what the Scenario asked for.
17. **The observed Node** — judges what the control plane holds.

## Decision

Options 2, 5, 8, 10, 12, 15 and 17.

- `Given().Zones` takes the Zone count as an `int`, then one or more `node.Pool(name, node.PerZone(n), capacity)`. The defined types make a swap a compile error.
- A capacity is a `node.Capacity` literal of `CPU`, `Memory` and `Pods`, or a Named Node capacity such as `StandardNode()` from `scenarios/nodes.go`. The lab defines none.
- `.Labels` and `.Taints` may be written more than once. A taint admits the NoSchedule effect alone.
- Every Node carries `k8s-resilience-lab/node-pool` with its NodePool's name, written after the stated labels. The simulator's selection label is pinned to the domain's lab-own keys.
- The domain holds the label, taint and lab-own key rules without a Kubernetes import; a test pins them to the client library and the mapper.
- A NodePool the domain refuses ends the Scenario at the Topology Given, naming the refusal. Refusals cover an empty name, a refused capacity, a lab-own key or Node-condition taint key, syntax, a repeated key, and a taint effect other than NoSchedule.
- Each probe Pod tolerates its own Node's taints, so provisioning ends on a tainted Node.
- The Given line reports each NodePool's name, Node count, capacity with a named one's values, then its labels and taints in key order.
- NodePools keep declaration order, and a Zone's Nodes are numbered through them. A Then reads the observed Node's labels; a key or NodePool the Topology lacks ends the Scenario at that Then.

## Consequences

A Scenario's Topology line shows every value its Nodes hold, and reads as a manifest: `node.Pool("gpu", node.PerZone(1), GPUNode()).Taints(node.Taint{Key: GPU, Value: GPUPresent, Effect: node.NoSchedule})`. Every Node is selectable by its NodePool, and a spread can count NodePools or label values as domains.

A dimension left out and every label or taint refusal are run-time ends at the Given rather than compile errors. A hand-written syntax lives in the domain, held to the library's by a test. PreferNoSchedule and NoExecute taints stay refused until a slice admits them. Drawing a Deployment onto a NodePool by node selection stays an Open Question.

## Implementation

**Requirements:** REQ-SIM-011, REQ-SIM-013, REQ-SIM-015, REQ-SIM-017, REQ-SIM-019

## References

- [system-design.md § Contracts](../system-design.md#contracts)
- [ADR: Provisioning ends when the scheduler has placed a Pod on every Node](2026-10-04-scheduler-observable-provisioning.md)
- [ADR: Scenario code stays idiomatic Go, read by subject](2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md)
