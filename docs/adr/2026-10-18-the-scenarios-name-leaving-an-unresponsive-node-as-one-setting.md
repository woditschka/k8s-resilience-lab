# The Scenarios name leaving an unresponsive Node as one setting, and a run reports every setting a Workload's Pods carry

**Status:** Accepted, amended 2026-10-18: `lab.Settings` and `lab.Setting` are now `pod.Settings` and `pod.Setting`, the settings `lab.TopologySpread`, `lab.Request` and `lab.Toleration` are now the keyed literals `pod.TopologySpreadConstraint`, `pod.Request` and `pod.Toleration`, and `lab.UnresponsiveNodeWait` is now `node.UnresponsiveWait`, per [ADR: Pod and Node settings are keyed literals under Kubernetes' names, refused at their Given](2026-10-18-pod-and-node-settings-are-keyed-literals-refused-at-their-given.md)

## Context

[ADR: Every predefined Pod specification is defined by the Scenarios, with its own Startup time](2026-10-18-every-pod-specification-is-defined-by-the-scenarios.md) has each definition write the lab's not-ready and unreachable tolerations itself. The owner read Resilient placement in `scenarios/pods.go` and found it hard to read. Two of its four settings repeat in every definition and decide nothing there: the domain holds both at the lab's waiting time whatever is written (REQ-SIM-008).

Asked whether REQ-SIM-009 changes, the owner answered: "we need to stay as close as possibe to the k8s naming , this does not mean we cannot create a method on a higher level that names the lower level config". Kubernetes naming is the floor, and a name above it stands for settings written under their Kubernetes names. The run reports every setting under its Kubernetes name.

A first reading placed that name in `internal/lab`. The owner challenged it: "ResilientPlacement had the settings directly, LeaveUnresponsiveNodes just aggregates the direct settings on a higher level, so i think it would alos belong to pods.go". REQ-SIM-009 and REQ-SIM-016 record the Scenarios defining it.

[ADR: A Workload declares one of the lab's predefined Pod specifications](2026-10-05-workload-declares-a-predefined-pod-specification.md) keeps the report free of configuration: a Workload's Step names its specification and not its settings.

## Options Considered

What a definition writes for the lab's two tolerations:

1. **Both tolerations, as before**: every definition repeats two settings it cannot change.
2. **Nothing, the mechanism adding them**: option 5 of the Scenario-defined specification ADR, where the reader no longer sees them.
3. **One Named setting the lab defines**: `lab.LeaveUnresponsiveNodes()`, composed in `internal/lab`. A reader of `pods.go` follows the name into the machinery to see what it stands for.
4. **One Named setting the Scenarios define, through a grouping means the lab offers**: `LeaveUnresponsiveNodes()` in `scenarios/pods.go`, its body the two `lab.Toleration` settings passed to `lab.Settings`.

Where a run reports the settings a Workload's Pods carry:

5. **In the Given Step's name**: one line of several hundred characters, quoted again by every Finding and every ended-at message naming that Step.
6. **On lines of their own under the Given**: the Step's name stays the intent, and one line per setting follows it.

## Decision

Option 4. `lab.Settings` takes zero or more `lab.Setting` values and returns the Customization applying each in the order written. `lab.Setting` is the sealed type that `lab.TopologySpread`, `lab.Request` and `lab.Toleration` return, so a group holds settings alone: a Startup time, a full access or another group does not compile there. The lab names no group of its own and holds its waiting time whatever a group states.

`scenarios/pods.go` defines `LeaveUnresponsiveNodes()`, an exported function taking nothing, beside the specifications that name it. Its body is one `lab.Settings` call of exactly the not-ready and unreachable `lab.Toleration` settings, written under their Kubernetes field names at `lab.UnresponsiveNodeWait`. It is a function rather than a variable, because no package of the lab holds mutable state. It is the shortcut [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md) admits: a pure function expanding to existing settings, adding none of its own. Every predefined Pod specification names it and writes neither toleration itself. The Scenario-place rule asserts its one definition and its use, and that no production source under `internal/` or `cmd/` calls `lab.Settings`.

Option 6. A Given declaring a Workload records every setting the domain holds for its Pods, in the order held, beside its Step name. The run prints each on a line of its own under that Given, as a manifest writes it. Each line names the Pod-spec field the setting fills, then its fields under Kubernetes' API names with their values:

```text
    tolerations: {key: node.kubernetes.io/not-ready, operator: Exists, effect: NoExecute, tolerationSeconds: 2}
```

A Named setting reports as the settings it stands for. A full access reports as one line saying it applies, since its change is a function the lab cannot read back. A refused specification reports no setting.

Option 1 is what the owner found hard to read. Option 2 hides a setting deciding a Placement, which REQ-SIM-009 refuses. Option 3 puts a Scenario-level name in the machinery, which the owner refused: the specification aggregating settings sits in `pods.go`, so the name aggregating them sits there too. Option 5 makes a Step's name carry configuration wherever it is quoted.

## Consequences

A definition reads as its own spreads and Request plus one named line, and what that line stands for is one function beside it in `pods.go`. Every run prints it expanded.

The lab's own tests cannot import `scenarios`. They write the two tolerations where a fixture needs them, and test `lab.Settings` on groups of their own.

When settings become keyed literals in a `pod` package, the grouping means moves with them and `LeaveUnresponsiveNodes` is rewritten inside its own body alone, per [ADR: Scenario code stays idiomatic Go, read by subject](2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md).

Two Workloads under one intent still share a Step name, and their setting lines differ where one was customized. The configuration-free report of the 2026-10-05 ADR now covers the Step name alone.

A Customization writing another time for either toleration still changes nothing: the domain holds the lab's waiting time, and the setting line reports it.

A further Named setting is one more function in the Scenarios' place, arriving with the Scenario needing it.

## Implementation

**Requirements:** REQ-SIM-016, REQ-SIM-009

## References

- [system-design.md § Contracts](../system-design.md#contracts)
- [ADR: Every predefined Pod specification is defined by the Scenarios, with its own Startup time](2026-10-18-every-pod-specification-is-defined-by-the-scenarios.md)
- [ADR: A Workload declares one of the lab's predefined Pod specifications](2026-10-05-workload-declares-a-predefined-pod-specification.md)
- [ADR: Scenario code stays idiomatic Go, read by subject](2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md)
