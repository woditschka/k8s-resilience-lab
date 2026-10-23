# Scenario code stays idiomatic Go, read by subject

**Status:** Accepted, amended 2026-10-18: a setting is a sealed literal under Kubernetes' type and field names, refused at its Given with a field left out, options 10, 12 and 14; amended 2026-10-19: a toleration requires the fields its operator and effect take, per [ADR: A NodePool states NoSchedule taints, and a later Deployment runs beside the earlier, judged alone](2026-10-19-a-nodepool-states-noschedule-taints-and-a-later-workload-deploys-beside-the-earlier.md); amended 2026-10-23: every count is a plain `int` named by its Step, and a Scenario line qualifies what the lab offers and writes bare what the Scenarios define, options 16 and 17

## Context

A Scenario source imports `internal/lab` alone and writes every name under the `lab` prefix, per [ADR: A Deployment declares one of the lab's predefined Pod specifications](2026-10-05-workload-declares-a-predefined-pod-specification.md). The package offers some 56 exported names, and a multi-value setting is a positional call whose arguments pass through identity functions that label each position.

On 2026-10-18 the owner read Resilient placement and said: "i feel the lab.* makes this hard to read lab.Toleration(lab.Key(lab.NodeNotReady), lab.Operator(lab.Exists), lab.Effect(lab.NoExecute), lab.TolerationSeconds(lab.UnresponsiveNodeWait))". Root answered that a dot import is not idiomatic Go, and the owner said: "i want to stay ideomatic go". The owner then proposed: "a node tolderaten could bel node. a pod one pod." The owner accepted root's sketch of that split with "lgtm". Answering a later question, the owner added: "we need to stay as close as possibe to the k8s naming".

Go treats a package name as part of every identifier it qualifies. Effective Go and the Google Go Style Guide both discourage a dot import outside tests and a name repeating its package's.

The split first left three points to the slice moving the names: their spelling, where a Pod count lives, and each setting type's name against Kubernetes' own. That slice settled them the same day. A keyed literal compiles with a field left out, where a positional argument does not. REQ-SIM-016 edge case 3 refuses such a setting where it is stated, and the domain package stays unchanged. The owner also asked for a function naming one spread: "ok, put it on slice 2".

On 2026-10-19 the NoSchedule taint slice found that requiring every toleration field would make a NoSchedule toleration state a waiting time Kubernetes refuses. It narrowed the rule for a toleration to the fields its operator and effect take.

On 2026-10-23 the owner read the typed counts, `lab.Zones(3)`, `lab.Pods(9)` and `lab.Nodes(1)`, and asked: "is there q way we can write "lab.Pods(9)" as Pods(9) ?" Each Step's name already states the subject its count counts. Asked for the idiomatic Go way to name the lab's values on a Scenario line, root answered that Go's idiom is the qualifier, and that a re-export vocabulary is tolerated in a domain-specific language but reads as a smell elsewhere. The owner chose "the version go developers would like". Root drafted the rule this record states, and the owner signed it with "lgtm".

## Options Considered

Where a Scenario's names live:

1. **One `lab` package, as before**: one prefix for every subject, so each name repeats its subject, `lab.NodeNotReady`.
2. **A dot import of `lab`**: drops the prefix, and the reader can no longer tell where a name comes from.
3. **Packages named by subject**: `pod` and `node` under `internal/lab`, with `lab` keeping the Scenario, its Steps, Zones and the Topology.
4. **A package per concept**: zone, workload and failure besides, each holding a few names and adding an import to every Scenario.

How a setting of several values is written:

5. **Positional arguments, each wrapped in a function labelling its position**: `lab.Key(lab.NodeNotReady)`.
6. **A keyed struct literal whose field names are Kubernetes' names for those fields**: Go's own form of named arguments.

Where a left-out field is refused:

7. **A constructor wrapping each literal**, `pod.Tolerate(pod.Toleration{…})`: one more name on every line, existing only to check its argument.
8. **The domain constructor**: changes `internal/scenario`, which REQ-SIM-016 rules out.
9. **The Scenario-place rule test**: holds the lab's own Scenarios alone, and only when the gate runs.
10. **The setting, applied to a specification**: the literal is a sealed `pod.Setting`, and applying it with a field left out records the specification's first refusal.

What a setting type is called:

11. **The lab's own names**: `TopologySpread`, or the sketch's `Spread`.
12. **Kubernetes' Go type names**: `TopologySpreadConstraint` and `Toleration`. Kubernetes has no type for one Pod's requests, so `Request` keeps the vocabulary's term.

How an amount is written:

13. **Unit functions**: `pod.CPU(3)`, `pod.MemoryGi(12)`, the first repeating its field's name.
14. **Kubernetes' resource names as fields, Kubernetes' units in the values**: `CPU: 3` in whole cores and `Memory: 12 * pod.Gi`, as a manifest writes `cpu: 3` and `memory: 12Gi`.

How a count is written:

15. **A defined type per subject**: `lab.Pods(9)`, `lab.Zones(3)`, `lab.Nodes(1)`, the 2026-10-18 form.
16. **A plain `int` under the Step naming its subject**: `Zones(3, …)`, `Deployment(Checkout, 9, …)`, `SurvivesLosingPods(3)`.

How a Scenario's lines name the lab's values:

17. **Qualified by the subject's package, an inline Customization included**: `node.Pool("general", node.PerZone(3), StandardNode())`, `pod.StartupSeconds(3)`.
18. **A local vocabulary**: the Scenarios re-export each lab name their lines write as a forwarding function or alias, so a line names every value bare.
19. **A local vocabulary of package variables**, each carrying a function of the lab.

## Decision

Options 3, 6, 10, 12, 14, 16 and 17. Scenario code, and the surface `internal/lab` offers it, follows these rules:

- No dot import.
- Each exported name sits in the package of its subject, a Pod, a Node or the Scenario, and does not repeat that subject: `node.NotReady`, not `lab.NodeNotReady`.
- `pod` holds what a Pod carries: the specification, its definer `pod.Predefined`, the Customizations, `pod.Settings`, full access, the Startup forms, the three settings, `pod.Gi`, and the setting values `DoNotSchedule`, `Honor`, `Exists` and `NoExecute`.
- `node` holds what a Node is: `node.Pool`, its Nodes `node.PerZone`, `node.Capacity`, `node.Named`, `node.Gi`, the labels `node.Zone` and `node.Hostname`, the taints `node.NotReady` and `node.Unreachable`, and `node.UnresponsiveWait`.
- `lab` keeps the Scenario, its Steps and `lab.DeploymentName`.
- Scenarios import `lab`, `pod` and `node`. `lab` imports `pod` and `node`, and neither imports `lab` or the other. `internal/scenario` stays the domain, unchanged. The split stops at `pod` and `node`.
- A setting of several values is a struct with exported fields, written as a keyed composite literal under Kubernetes' Go type and field names: `pod.Toleration{Key: node.NotReady, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: node.UnresponsiveWait}`. An amount is written in Kubernetes' units, and a plain number stands beside its field name with no wrapper, `MaxSkew: 1` included.
- No function exists only to label an argument's position.
- Kubernetes naming is the floor. A name above that floor is a Named setting, composed of settings in those names and reported expanded, per [ADR: A Deployment declares one of the lab's predefined Pod specifications](2026-10-05-workload-declares-a-predefined-pod-specification.md).
- Every field of a setting the lab offers is required, and its zero value is the field left out. The domain holds each as a plain value with no unset state, and REQ-SIM-015 lets no Placement-deciding value fall to a default. `tolerationSeconds` and `nodeTaintsPolicy`, optional to Kubernetes, are therefore required here.
- A toleration requires the fields its operator and effect take: Equal takes a value and Exists none, and NoExecute takes a waiting time and NoSchedule none. A field stated against them is refused as one left out is.
- A setting with a field left out records a refusal naming the setting and that field under its Kubernetes name. The Given declaring its Deployment ends the Scenario with it, as a refused Startup time does.
- A Node capacity left out of a dimension is refused by the domain at the Topology Given, as the zero capacity is.
- A count is a bare `int` under the Step naming its subject: `Zones(3, …)` for the Topology, `ZonesFail(1)`, `NodesFail(1)`, `SurvivesLosingPods(3)` and `PodsPerZone(3)`. A Deployment's replicas are one too, `Deployment(Checkout, 9, …)`, the parameter named `replicas` after Kubernetes' field. A Node's room is the `Pods` field of `node.Capacity`.
- A Scenario line qualifies what the lab offers and writes bare what the Scenarios define: `s.Given().Zones(3, node.Pool("general", node.PerZone(3), StandardNode()))`. An inline Customization is written qualified, `pod.StartupSeconds(3)` and `pod.FullAccess(…)` included.
- The Scenarios hold no alias of a `lab`, `pod` or `node` name: no type alias, no constant or variable standing for one, and no function only forwarding to one.
- The Scenarios define the Even spread as `EvenSpreadOver`, taking the topology key, `EvenSpreadOver(node.Zone)`, and returning the full literal at a `MaxSkew` of one, `DoNotSchedule` and `Honor`.

Option 1 keeps the stutter the owner found hard to read. Option 2 is not idiomatic Go, which the owner ruled out. Option 4 spreads few names over many imports. Option 5's wrappers carry no value, and a reader learns them as names of their own. Option 7 adds a name of no value to the reader. Option 8 widens the slice past its scope. Option 9 lets an engineer's own Scenario run with a field left out. Option 11 departs from the floor of Kubernetes naming. Option 13 stutters. Option 15 repeats the subject the Step's name already states and puts a framework name on every count. Option 18 re-exports the lab's names, which Go reads as a smell outside a domain-specific language, and leaves an inline Customization no bare home. Option 19 is mutable package state besides, which the lab holds none of.

## Consequences

A Scenario line shows where each name the lab offers comes from, and each setting reads as its Kubernetes manifest does. The rule tests read the qualified form, and report any alias of a lab, pod or node name in the Scenarios.

A count's subject is its Step's name, so a swap of two counts compiles where a Step takes two. No Step takes two counts.

The single-import rule of the 2026-10-05 ADR widens to three subject packages, still curated to what a Scenario may name.

A field left out is caught at run time, at its Given, where the positional form refused it at compile time. The NodePool ADR's "does not compile" becomes a refusal at the Topology Given.

A toleration evicting at once, or a request of none, is written through full access.

`lab` reads a specification and a NodePool through exported read-only methods of `pod` and `node`, which no Scenario writes.

`pod.Gi` and `node.Gi` spell one unit twice, so neither package imports the other. A cpu finer than a core, or a value such as `ScheduleAnyway`, arrives with the Scenario needing it.

## Implementation

**Requirements:** REQ-SIM-016, REQ-SIM-025, REQ-SIM-026

## References

- [system-design.md § Package Structure](../system-design.md#package-structure)
- [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md)
- [ADR: A Deployment declares one of the lab's predefined Pod specifications](2026-10-05-workload-declares-a-predefined-pod-specification.md)
- [ADR: A Topology declares the NodePools every Zone holds](2026-10-08-a-topology-declares-the-nodepools-every-zone-holds.md)
- [ADR: A NodePool states NoSchedule taints, and a later Deployment runs beside the earlier, judged alone](2026-10-19-a-nodepool-states-noschedule-taints-and-a-later-workload-deploys-beside-the-earlier.md)
- [Google Go Style Guide](https://google.github.io/styleguide/go/)
