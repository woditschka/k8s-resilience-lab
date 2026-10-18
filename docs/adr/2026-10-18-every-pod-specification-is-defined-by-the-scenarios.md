# Every predefined Pod specification is defined by the Scenarios, with its own Startup time

**Status:** Accepted, amended 2026-10-18: a definition names the Scenarios' one Named setting in place of its two tolerations, per [ADR: The Scenarios name leaving an unresponsive Node as one setting, and a run reports every setting a Workload's Pods carry](2026-10-18-the-scenarios-name-leaving-an-unresponsive-node-as-one-setting.md); amended again 2026-10-18: `lab.PodSpecification` and `lab.PredefinedPodSpecification` are now `pod.Specification` and `pod.Predefined`, `lab.StartupTime` and `lab.Settings` are now `pod.StartupTime` and `pod.Settings`, `lab.UnresponsiveNodeWait` is now `node.UnresponsiveWait`, and `lab.NamedNodeCapacity` is now `node.Named`, per [ADR: Pod and Node settings are keyed literals under Kubernetes' names, refused at their Given](2026-10-18-pod-and-node-settings-are-keyed-literals-refused-at-their-given.md)

## Context

REQ-SIM-015 holds that every value deciding a Placement stands on a Scenario's own lines, stated there or named there. A name stands for values the Scenarios define once, beside the Scenarios using it, and the lab only carries out the definition. [ADR: Every NodePool states its Node capacity or names one its Scenarios define](2026-10-09-every-nodepool-states-or-names-its-node-capacity.md) did this for Node capacity.

The owner asked the same of the Pod: "the size of astandard node and standard pod lives deep in the framework - should it not live in the scenarios?" Two recorded decisions stand in the way. [ADR: A Workload declares one of the lab's predefined Pod specifications](2026-10-05-workload-declares-a-predefined-pod-specification.md) defines Resilient placement and Default placement in `internal/lab`, over a base adding the lab's tolerations. [ADR: The lab owns the simulator's Stages to give a Pod its Startup time](2026-10-08-the-lab-owns-the-simulators-stages-for-a-startup-time.md) gives a specification stating no Startup time the lab's own.

REQ-SIM-009 still holds that a definition shows every setting it holds under its Kubernetes name, the lab's waiting time before Pods leave an unresponsive Node included. REQ-SIM-008 still holds that a Scenario states no waiting time of its own.

`lab.PodSpecification` already names the type a Workload Step declares, so Go admits no function of that name beside it.

## Options Considered

Where a predefined Pod specification is defined:

1. **In `internal/lab`, as today**: the values deciding a Placement stay in the framework, which REQ-SIM-015 now refuses.
2. **In the Scenarios' place, through a Step-surface mechanism**: `scenarios/pods.go` defines each one with `lab.PredefinedPodSpecification`, as `scenarios/nodes.go` defines each Named Node capacity.

How a definition states its Startup time:

3. **As one more Customization**: a definition omitting it compiles and is refused at run time.
4. **As a mandatory argument after the name**: a definition omitting it does not compile.

How a definition shows the lab's tolerations:

5. **The mechanism adds them, as the base does today**: the definition no longer shows them.
6. **The definition writes them through an exported toleration setting**, its time named by the lab's waiting time. The domain holds the lab's two tolerations at that time whatever is written for those taints.
7. **The mechanism refuses a definition lacking them**: a Customization could still write another time for either taint.

## Decision

Option 2. The lab keeps the mechanism and the Scenarios own the values. `lab.PredefinedPodSpecification(name, startupTime, settings...)` builds the specification, carrying the domain value or its first refusal, as `lab.NamedNodeCapacity` does. Each definition is an exported function taking nothing, returning that one call, named for its intent: `ResilientPlacement()` and `DefaultPlacement()`. A Workload line declares one of them, customized or not.

Option 4, by the rule of [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md): a mandatory value is a named argument. The four Startup forms return one `lab.StartupTime`, which is also a Customization, so a Workload line still states one. The zero `lab.StartupTime`, built by no form, is refused by the mechanism, as the domain refuses the zero Node capacity. The lab's own Startup time, its two constants and its run-start line are retired.

Option 6. The toleration setting, its four field names and `lab.UnresponsiveNodeWait` join the vocabulary a definition writes in. `scenario.NewPodSpecification` takes a Startup time and refuses an empty name. It holds the not-ready and unreachable tolerations at the lab's waiting time, replacing any time written for them and adding any not written. Option 5 hides a setting deciding a Placement, against REQ-SIM-009. Option 7 lets a Customization change the time, against REQ-SIM-008.

Since the amendment of 2026-10-18, a definition no longer writes the two tolerations itself. It names `LeaveUnresponsiveNodes`, a Named setting `scenarios/pods.go` defines beside it as exactly those two toleration settings grouped through `lab.Settings`, and the run reports both under their Kubernetes names. Option 5 stays rejected: the named setting stands visibly in every definition, where option 5 left no trace of the tolerations.

A source rule holds `scenarios/pods.go` to definitions alone, each name to one definition, and each definition to writing both tolerations by the lab's waiting time; since the amendment, to naming the Named setting, defined once there, and writing neither toleration. No source under `internal/` or `cmd/` defines a specification.

A Workload's Given line names its specification, then the shortest and the longest Startup time its Pods take. The run's start reports no Startup time.

## Consequences

A reader of `scenarios/pods.go` sees every value a specification gives its Pods: the spreads, the tolerations, any Request and the Startup time. Resilient placement and Default placement keep the Startup time the retired default held, so every Scenario keeps its asserted outcome.

A Pod template always carries the two Startup annotations from its specification's own Startup time. The lab's Stage set is unchanged.

Tests of the machinery define the specifications they need beside themselves, since `internal/` sources cannot import the Scenarios' place.

Two definitions under one name, each in its own Scenario source, are not refused within a run. The place rule keeps the lab's own Scenarios to one definition per name.

A further specification, such as Zone spread only, is one function in `scenarios/pods.go`, arriving with its first Scenario. The lab's code does not change.

## Implementation

**Requirements:** REQ-SIM-015, REQ-SIM-009, REQ-SIM-012

## References

- [system-design.md § Contracts](../system-design.md)
- [ADR: A Workload declares one of the lab's predefined Pod specifications](2026-10-05-workload-declares-a-predefined-pod-specification.md)
- [ADR: The lab owns the simulator's Stages to give a Pod its Startup time](2026-10-08-the-lab-owns-the-simulators-stages-for-a-startup-time.md)
- [ADR: Every NodePool states its Node capacity or names one its Scenarios define](2026-10-09-every-nodepool-states-or-names-its-node-capacity.md)
