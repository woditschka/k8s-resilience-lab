# Scenario code stays idiomatic Go, read by subject

**Status:** Accepted

## Context

A Scenario source imports `internal/lab` alone and writes every name under the `lab` prefix, per [ADR: A Workload declares one of the lab's predefined Pod specifications](2026-10-05-workload-declares-a-predefined-pod-specification.md). The package offers some 56 exported names, and a multi-value setting is a positional call whose arguments pass through identity functions that label each position.

The owner read Resilient placement and said: "i feel the lab.* makes this hard to read lab.Toleration(lab.Key(lab.NodeNotReady), lab.Operator(lab.Exists), lab.Effect(lab.NoExecute), lab.TolerationSeconds(lab.UnresponsiveNodeWait))". Root answered that a dot import is not idiomatic Go, and the owner said: "i want to stay ideomatic go". The owner then proposed: "a node tolderaten could bel node. a pod one pod." The owner accepted root's sketch of that split with "lgtm". Answering a later question, the owner added: "we need to stay as close as possibe to the k8s naming".

Go treats a package name as part of every identifier it qualifies. Effective Go and the Google Go Style Guide both discourage a dot import outside tests and a name repeating its package's.

## Options Considered

Where a Scenario's names live:

1. **One `lab` package, as before**: one prefix for every subject, so each name repeats its subject, `lab.NodeNotReady`.
2. **A dot import of `lab`**: drops the prefix, and the reader can no longer tell where a name comes from.
3. **Packages named by subject**: `pod` and `node` under `internal/lab`, with `lab` keeping the Scenario, its Steps, Zones and the Topology.
4. **A package per concept**: zone, workload and failure besides, each holding a few names and adding an import to every Scenario.

How a setting of several values is written:

5. **Positional arguments, each wrapped in a function labelling its position**: `lab.Key(lab.NodeNotReady)`.
6. **A keyed struct literal whose field names are Kubernetes' names for those fields**: Go's own form of named arguments.

## Decision

Options 3 and 6. Scenario code, and the surface `internal/lab` offers it, follows these rules:

- No dot import.
- Each exported name sits in the package of its subject, a Pod, a Node or the Scenario, and does not repeat that subject: `node.NotReady`, not `lab.NodeNotReady`.
- A setting of several values is a struct with exported fields, written as a keyed composite literal. Its field names are Kubernetes' names for those fields, Go-cased: `pod.Toleration{Key: node.NotReady, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: node.UnresponsiveWait}`.
- No function exists only to label an argument's position.
- Kubernetes naming is the floor. A name the lab gives a Kubernetes field or value is Kubernetes' name for it. A name above that floor is a Named setting, composed of settings in those names and reported expanded, per [ADR: The Scenarios name leaving an unresponsive Node as one setting](2026-10-18-the-scenarios-name-leaving-an-unresponsive-node-as-one-setting.md).
- Scenarios import `lab`, `pod` and `node`. `lab` imports `pod` and `node`, and neither imports `lab`. `internal/scenario` stays the domain, unchanged.
- The split stops at `pod` and `node`.

Option 1 keeps the stutter the owner found hard to read. Option 2 is not idiomatic Go, which the owner ruled out. Option 4 spreads few names over many imports. Option 5's wrappers carry no value and a reader learns them as names of their own.

## Consequences

A Scenario's qualifiers read as the nouns its names belong to, and each setting reads as its Kubernetes manifest does. The rule tests reading Scenario sources match on the three qualifiers.

A keyed literal compiles with a field left out, where a positional argument does not. A setting's constructor in the domain, or the Scenario-place rule, must refuse a literal missing a field the setting needs.

The single-import rule of the 2026-10-05 ADR widens to three subject packages, still curated to what a Scenario may name.

Left to the slice moving the names: their exact spelling, where a Pod count lives, and each setting type's name against Kubernetes' own.

## Implementation

**Requirements:** REQ-SIM-016

## References

- [system-design.md § Package Structure](../system-design.md#package-structure)
- [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md)
- [ADR: A Workload declares one of the lab's predefined Pod specifications](2026-10-05-workload-declares-a-predefined-pod-specification.md)
- [ADR: The Scenarios name leaving an unresponsive Node as one setting](2026-10-18-the-scenarios-name-leaving-an-unresponsive-node-as-one-setting.md)
- [Google Go Style Guide](https://google.github.io/styleguide/go/)
