# Scenario code stays idiomatic Go, read by subject

**Status:** Accepted

## Context

A Scenario is read and tinkered with, so its lines must say what each value means. The owner found one `lab` prefix on every name hard to read: "i feel the lab.* makes this hard to read lab.Toleration(lab.Key(lab.NodeNotReady), …)". The owner said "i want to stay ideomatic go", proposed "a node tolderaten could bel node. a pod one pod", and added "we need to stay as close as possibe to the k8s naming".

Go treats a package name as part of every identifier it qualifies. Effective Go and the Google Go Style Guide discourage a dot import outside tests and a name repeating its package's. A keyed literal compiles with a field left out, which REQ-SIM-016 edge case 3 refuses where it is stated.

Asked "is there q way we can write "lab.Pods(9)" as Pods(9) ?", the owner chose "the version go developers would like" and signed the rule below with "lgtm" (REQ-SIM-026).

## Options Considered

How a Step's values are passed:

1. **Plain positional arguments, wrapped in functions labelling each position** — `lab.Key(lab.NodeNotReady)`, names of no value.
2. **Mandatory values as arguments, optional settings as further calls** — each call a complete value.
3. **A chain accumulating mandatory values** — a builder, incomplete between calls.

Where a Scenario's names live:

4. **One `lab` package** — every name repeats its subject.
5. **A dot import of `lab`** — not idiomatic Go.
6. **Packages named by subject**, `pod` and `node` under `internal/lab`.
7. **A package per concept** — few names over many imports.

How a setting of several values is written, and refused:

8. **A keyed struct literal under Kubernetes' Go type and field names**.
9. **A constructor wrapping each literal** — one more name on every line.
10. **The domain constructor** — changes `internal/scenario`, which REQ-SIM-016 rules out.
11. **The Scenario-place rule test** — holds the lab's own Scenarios alone.
12. **The sealed setting, applied to a specification**, recording its first refusal.

What a setting and an amount are called:

13. **The lab's own names**, `TopologySpread` — below the floor of Kubernetes naming.
14. **Kubernetes' Go type names**, `TopologySpreadConstraint` and `Toleration`, and `Request` from the vocabulary.
15. **Unit functions**, `pod.CPU(3)` — stutters.
16. **Kubernetes' resource names as fields**, `CPU: 3`, `Memory: 12 * pod.Gi`.

How a count is written, and how a line names the lab's values:

17. **A defined type per subject**, `lab.Pods(9)` — repeats what the Step's name states.
18. **A plain `int` under the Step naming its subject**, `Deployment(Checkout, 9, …)`, `NodesFail(1)`.
19. **Qualified by the subject's package**, an inline Customization included.
20. **A local vocabulary of aliases, forwarders or package variables** — a smell outside a domain-specific language, and mutable state.

## Decision

Options 2, 6, 8, 12, 14, 16, 18 and 19.

- A Step is one complete call. A mandatory value is an argument, an optional setting a further call, and no function exists only to label a position.
- No dot import. Each exported name sits in its subject's package, a Pod, a Node or the Scenario, without repeating it: `node.NotReady`.
- `pod` holds the specification, `pod.Predefined`, Customizations, `pod.Settings`, full access, the Startup forms, the three settings, `pod.Gi` and their values.
- `node` holds `node.Pool`, `node.PerZone`, `node.Capacity`, `node.Named`, `node.Gi`, the Zone and hostname labels, the Node-condition taints and `node.UnresponsiveWait`.
- `lab` keeps the Scenario, its Steps and `lab.DeploymentName`. `lab` imports `pod` and `node`; neither imports `lab` or the other.
- A setting is a keyed literal under Kubernetes' names, amounts in Kubernetes' units. Kubernetes naming is the floor, and a Named setting stands above it, reported expanded.
- Every field of a setting is required, its zero value the field left out. A toleration requires exactly the fields its operator and effect take. A refusal names the setting and field, ending the Scenario at its Given.
- A count is a bare `int` under the Step naming its subject; `SurvivesLosing(lab.Zones(1))` is the one exception. A Node's room is the `Pods` field of `node.Capacity`.
- A Scenario line qualifies what the lab offers and writes bare what the Scenarios define. The Scenarios hold no alias of a `lab`, `pod` or `node` name.
- The Scenarios define `EvenSpreadOver(key)`, the full spread literal at a `MaxSkew` of one, `DoNotSchedule` and `Honor`.

## Consequences

A Scenario line shows where each name comes from, and each setting reads as its manifest does. The rule tests read the qualified form and report any alias. No Step takes two counts, so a swap cannot compile silently. A field left out is caught at run time, at its Given. A toleration evicting at once, or a request of none, is written through full access. `pod.Gi` and `node.Gi` spell one unit twice, so neither package imports the other.

## Implementation

**Requirements:** REQ-SIM-006, REQ-SIM-016, REQ-SIM-019, REQ-SIM-025, REQ-SIM-026

## References

- [system-design.md § Package Structure](../system-design.md#package-structure)
- [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md)
- [ADR: A Deployment declares one of the lab's predefined Pod specifications](2026-10-05-workload-declares-a-predefined-pod-specification.md)
- [Google Go Style Guide](https://google.github.io/styleguide/go/)
