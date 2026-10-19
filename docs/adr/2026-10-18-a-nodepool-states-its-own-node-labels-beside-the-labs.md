# A NodePool states its own Node labels beside the lab's, and a Then counts Pods per value

**Status:** Accepted

## Context

REQ-SIM-017 lets a NodePool state labels its Nodes carry, beside the lab's own Zone, hostname and NodePool labels. A Then counts a Workload's Pods per value of such a label, and the Even spread spreads over it. The owner answered the scoping questions with "yes": no rack concept in this slice, a Scenario spreading over on-demand and spot, and a Failure by label in the next slice.

Four points constrain later slices. They are how a NodePool states its labels, which keys stay the lab's, where a stated label is refused, and where a Then reads a Node's labels from. The next slice, a Failure by label, reads the same labels.

## Options Considered

How a NodePool states its labels:

1. **A further argument of `node.Pool`**: every existing NodePool line writes an empty map, and a mandatory argument then holds an optional setting.
2. **A further call on the declared NodePool**, `.Labels(node.Labels{…})`, taking Kubernetes' own type for labels, a map from key to value.
3. **A slice of keyed pairs**, `[]node.Label{{Key: …, Value: …}}`: Kubernetes has no such type, and the pair spells each label twice as long.

Which keys stay the lab's:

4. **The three the PRD names**: Zone, hostname and NodePool name. The simulator's own selection label would then be stated over, and lost when a Node fails.
5. **Every key the lab writes on a Node, and every key under the lab's own prefix**.

Where a Then reads a Node's labels:

6. **The declared Topology**: a Pod's Node is looked up in what the Scenario asked for.
7. **The observed Node**: the observation reads the labels the control plane holds, as it reads a Node's Zone.

## Decision

Options 2, 5 and 7.

- `node.Labels` is Kubernetes' label type, a map from key to value. `.Labels(node.Labels{…})` on a declared NodePool adds them, and may be written more than once.
- One key twice in one map literal does not compile. The same key in two calls is refused, the first refusal winning, as a refused capacity does.
- The lab's own keys are those it writes on every Node and every key under the lab's prefix. Stating one is refused.
- The domain holds the lab's keys and the label syntax as its own rules, naming no Kubernetes package. A mapper test pins both to the client library and to what the mapper writes.
- The simulator's selection label stays a literal of the cluster package, since the simulator's command is composed from that package's own literals alone. The same mapper test pins it to the domain's lab-own keys.
- A refused label ends the Scenario at the Topology Given, as a refused capacity does.
- The observation reads every label a Node carries beside the lab's own. A Then counts Pods per value against the values the NodePools state.
- A Then naming a key no NodePool states ends the Scenario at that Then, naming the key.

Option 1 makes every NodePool line state that it states no label, contrary to REQ-SIM-017 edge case 3. Option 3 departs from Kubernetes' names. Option 4 lets a stated label take the simulator's selection label, so a Failure could no longer stop that Node. Option 6 judges what the Scenario asked for, not what the control plane holds.

## Consequences

A Scenario's NodePool reads as a manifest's labels: `node.Pool("spot", node.PerZone(1), StandardNodes()).Labels(node.Labels{CapacityType: "spot"})`.

A label key the Scenarios define is a name in their place, as a Workload name is.

A Scenario may declare several NodePools, naming each after what its Nodes hold. A Scenario of one NodePool still names it general.

A Node in the domain carries its labels as an immutable value. Two Nodes and two Placements compare through an equality method, no longer by `==`.

A future lab label under the lab's prefix needs no change to the refusal. A future lab label outside it joins the lab's keys and its pin test.

The Failure by label reads the same observed labels, and refuses a key no NodePool states in the same way.

## Implementation

**Requirements:** REQ-SIM-017

## References

- [system-design.md § Contracts](../system-design.md#contracts)
- [ADR: Pod and Node settings are keyed literals under Kubernetes' names, refused at their Given](2026-10-18-pod-and-node-settings-are-keyed-literals-refused-at-their-given.md)
- [ADR: Scenario code stays idiomatic Go, read by subject](2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md)
- [ADR: Every NodePool is named where it is declared](2026-10-09-every-nodepool-is-named-where-it-is-declared.md)
- [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md)
