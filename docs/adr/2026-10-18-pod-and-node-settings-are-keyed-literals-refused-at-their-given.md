# Pod and Node settings are keyed literals under Kubernetes' names, refused at their Given

**Status:** Accepted

## Context

[ADR: Scenario code stays idiomatic Go, read by subject](2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md) moves a Scenario's names into `pod` and `node` packages and writes a setting of several values as a keyed literal. It leaves three points to the slice moving the names: their spelling, where a Pod count lives, and each setting type's name against Kubernetes' own.

A keyed literal compiles with a field left out. REQ-SIM-016 edge case 3 refuses such a setting where it is stated, and the domain package stays unchanged. The owner also asked for a function naming one spread: "ok, put it on slice 2".

## Options Considered

Where a left-out field is refused:

1. **A constructor wrapping each literal**, `pod.Tolerate(pod.Toleration{…})`: one more name on every line, existing only to check its argument.
2. **The domain constructor**: changes `internal/scenario`, which REQ-SIM-016 rules out.
3. **The Scenario-place rule test**: holds the lab's own Scenarios alone, and only when the gate runs.
4. **The setting, applied to a specification**: the literal is a sealed `pod.Setting`, and applying it with a field left out records the specification's first refusal.

What a setting type is called:

5. **The lab's own names**: `TopologySpread`, or the sketch's `Spread`.
6. **Kubernetes' Go type names**: `TopologySpreadConstraint` and `Toleration`. Kubernetes has no type for one Pod's requests, so `Request` keeps the vocabulary's term.

How an amount is written:

7. **Unit functions**: `pod.CPU(3)`, `pod.MemoryGi(12)`, the first repeating its field's name.
8. **Kubernetes' resource names as fields, Kubernetes' units in the values**: `CPU: 3` in whole cores and `Memory: 12 * pod.Gi`, as a manifest writes `cpu: 3` and `memory: 12Gi`.

## Decision

Options 4, 6 and 8.

- `pod` holds what a Pod carries: the specification, its definer `pod.Predefined`, the Customizations, `pod.Settings`, full access, the Startup forms, the three settings, `pod.Gi`, and the setting values `DoNotSchedule`, `Honor`, `Exists` and `NoExecute`.
- `node` holds what a Node is: `node.Pool`, its Nodes `node.PerZone`, `node.Capacity`, `node.Named`, `node.Gi`, the labels `node.Zone` and `node.Hostname`, the taints `node.NotReady` and `node.Unreachable`, and `node.UnresponsiveWait`.
- `lab` keeps the Scenario, its Steps, `lab.Zones`, `lab.Nodes`, `lab.Pods` and `lab.WorkloadName`.
- Every field of a setting the lab offers is required, and its zero value is the field left out. The domain holds each as a plain value with no unset state, and REQ-SIM-015 lets no Placement-deciding value fall to a default.
- The rule refuses more than Kubernetes requires of a setting, by intent: `tolerationSeconds` and `nodeTaintsPolicy`, optional to Kubernetes, are required here, and a value written as zero counts as left out. Its ground is REQ-SIM-015, under which no Placement-deciding value falls to a default.
- A setting with a field left out records a refusal naming the setting and that field under its Kubernetes name. The Given declaring its Workload ends the Scenario with it, as a refused Startup time does.
- A Node capacity left out of a dimension is refused by the domain at the Topology Given, as the zero capacity is.
- A Pod count reads by its subject (edge case 4): `lab.Pods` for a Workload's size and Required Capacity, the `Pods` field of `node.Capacity` for a Node's room.
- The Scenarios define the Even spread as `EvenSpreadOver`, taking the topology key and returning the full literal at a `MaxSkew` of one, `DoNotSchedule` and `Honor`.
- A plain number stands beside its field name with no wrapper, `MaxSkew: 1` included.

Option 1 adds a name of no value to the reader. Option 2 widens the slice past its scope. Option 3 lets an engineer's own Scenario run with a field left out. Option 5 departs from the floor of Kubernetes naming. Option 7 stutters.

## Consequences

A Scenario's settings read as their manifest: `pod.Toleration{Key: node.NotReady, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: node.UnresponsiveWait}`.

A field left out is caught at run time, at its Given, where the positional form refused it at compile time. The capacity ADR's "does not compile" becomes a refusal at the Topology Given.

A toleration evicting at once, or a request of none, is written through full access.

`lab` reads a specification and a NodePool through exported read-only methods of `pod` and `node`, which no Scenario writes.

`pod.Gi` and `node.Gi` spell one unit twice, so neither package imports the other. A cpu finer than a core, or a value such as `ScheduleAnyway`, arrives with the Scenario needing it.

## Implementation

**Requirements:** REQ-SIM-016

## References

- [system-design.md § Package Structure](../system-design.md#package-structure)
- [ADR: Scenario code stays idiomatic Go, read by subject](2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md)
- [ADR: The Scenarios name leaving an unresponsive Node as one setting](2026-10-18-the-scenarios-name-leaving-an-unresponsive-node-as-one-setting.md)
- [ADR: Every NodePool states its Node capacity or names one its Scenarios define](2026-10-09-every-nodepool-states-or-names-its-node-capacity.md)
