# A Workload declares one of the lab's predefined Pod specifications

**Status:** Accepted, amended 2026-10-18: the Scenarios define each predefined specification, per [ADR: Every predefined Pod specification is defined by the Scenarios, with its own Startup time](2026-10-18-every-pod-specification-is-defined-by-the-scenarios.md); amended again 2026-10-18: a run reports every setting a Workload's Pods carry, per [ADR: The Scenarios name leaving an unresponsive Node as one setting, and a run reports every setting a Workload's Pods carry](2026-10-18-the-scenarios-name-leaving-an-unresponsive-node-as-one-setting.md); amended a third time 2026-10-18: `lab.FullAccess`, `lab.PredefinedPodSpecification` and `lab.Settings` are now `pod.FullAccess`, `pod.Predefined` and `pod.Settings`, and `lab.CPU(1)` and `lab.MemoryGi(2)` are now the `CPU` and `Memory` fields of a keyed literal, memory written in `pod.Gi` for a Pod and `node.Gi` for a Node, per [ADR: Pod and Node settings are keyed literals under Kubernetes' names, refused at their Given](2026-10-18-pod-and-node-settings-are-keyed-literals-refused-at-their-given.md)

## Context

A Scenario used to declare its Workload's spread itself: `SpreadEvenly(lab.OverZones, lab.OverNodes)`. That line reads as a rule, so a reader infers the intent behind it. The lab's Scenarios exist to show one intent each, configured well or badly.

An engineer who opens a predefined specification reads which setting it sets and to which value, in Kubernetes' terms. The definition is therefore a worked example, written in the vocabulary a Scenario changes one with.

A Scenario also needs a specification that is nearly what it wants. The owner asked for the predefined ones to be tinkered with before use — "I feel there should be multiple predefined Pod specifications (as methods) ... that I can get and then still tinker with" — and chose one `Customize` call taking the changes as values over a chain of named steps.

The owner named the two intents rather than their mechanics: a specification conveys "that the Pod placement should target high resilience against Failure", not that it spreads evenly.

The lab adds settings of its own, the tolerations that make Pods leave an unresponsive Node. Those decide a Placement as much as a spread does, so the definition names them too.

`internal/scenario` imports no Kubernetes package, per [ADR: The lab is a Go program on a KWOK Simulated Cluster](2026-10-03-go-program-on-a-kwok-simulated-cluster.md), and a Scenario source imports `internal/lab` alone, per [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md).

## Options Considered

1. **Named helpers over the declared rules** — `SpreadEvenly` stays and a helper expands to the same rules.
2. **A spread abstraction of the lab's own** — a specification holds rules naming a domain, the Zones or the Nodes, and the mapper picks the label each domain is counted over.
3. **Settings under Kubernetes' own names, in the lab's own types** — a specification holds the fields Kubernetes names, and each predefined one is defined as the settings it sets.
4. **Predefined Kubernetes Pod templates in `internal/cluster`**, picked by name.
5. **A `*corev1.PodSpec` builder in `internal/lab`**, the predefined ones being its starting points.
6. **A chained method per ready-made change** — `ResilientPlacement().ZoneSpread(ScheduleAnyway)`, one method per change the lab offers.
7. **One `Customize` call taking the changes as values** — a change is a value of the lab's vocabulary, and a Scenario writes zero or more of them.
8. **The specification as a clause on the Workload line** — `.With(lab.ResilientPlacement())`, which needs a rule that every Workload line carries one.

## Decision

Option 3 for the specification, option 7 for changing one. A Workload declares exactly one Pod specification, as a mandatory argument of its Given, with zero or more Customizations of it and nothing else. Option 8 is rejected with it: an optional clause needs a rule to make it mandatory, which the argument is for free.

Each specification is named for the intent it serves: `ResilientPlacement` and `DefaultPlacement`, reported as Resilient placement and Default placement. The earlier names, Maximum availability and No spread rules, state a mechanism and a degree rather than an intent.

A specification is an immutable value of the domain core carrying its name and its settings: one topology spread per topology key, one toleration per taint. A setting names the Kubernetes fields it sets — `topologyKey`, `maxSkew`, `whenUnsatisfiable` and `nodeTaintsPolicy` for a spread, `key`, `operator`, `effect` and `tolerationSeconds` for a toleration — in types of the lab's own. `internal/scenario` spells the Kubernetes label, taint and value names itself, and the mapping test pins each one to the client library's constant.

A setting stating a Kubernetes quantity holds it as the integer that quantity carries exactly: millicores for cpu, mebibytes for memory. The Step surface spells it in whole units, `lab.CPU(1)` and `lab.MemoryGi(2)`, the way the toleration's whole seconds are spelled from a duration. A Node capacity states its cpu and memory in the same two types, so one spelling serves the room a Node has and the share a Pod asks for.

Each predefined specification is defined in `internal/lab`, over the base every one starts from, which names it and carries the lab's own settings:

```go
func specification(name string) PodSpecification {
	return newSpecification(name,
		toleration(key(NodeNotReady), operator(Exists), effect(NoExecute), tolerationSeconds(UnresponsiveNodeWait)),
		toleration(key(NodeUnreachable), operator(Exists), effect(NoExecute), tolerationSeconds(UnresponsiveNodeWait)),
	)
}

func DefaultPlacement() PodSpecification {
	return specification("Default placement")
}

func ResilientPlacement() PodSpecification {
	return specification("Resilient placement").Customize(
		TopologySpread(TopologyKey(ZoneLabel), MaxSkew(evenSkew), WhenUnsatisfiable(DoNotSchedule), NodeTaintsPolicy(Honor)),
		TopologySpread(TopologyKey(HostnameLabel), MaxSkew(evenSkew), WhenUnsatisfiable(DoNotSchedule), NodeTaintsPolicy(Honor)),
	)
}
```

`Customize` takes zero or more Customizations, returns a changed copy, and keeps the last setting written for a field. Each definition is therefore a worked example of the surface a Scenario changes a specification through.

A Customization is one setting written under Kubernetes' own field names, so `internal/lab` exports the spread setting and the names it takes. The toleration setting stayed unexported until the amendment of 2026-10-18, below, so that a Scenario states no waiting time of its own, per REQ-SIM-008. Since that amendment the setting is offered, and the domain holds the lab's waiting time whatever a definition writes. Every exported name a Scenario may write stands in one file, which is the vocabulary `internal/lab` offers Scenarios. That single-import rule is kept for the one vocabulary it gives a Scenario — one prefix, one curated list — rather than for hiding the machinery.

A Scenario holds no declaration and no local value, so it writes its Customizations on the Workload line itself. A change worth reusing becomes a predefined specification instead.

Full access is the Customization a Scenario writes when the lab's own vocabulary names no setting it needs: `lab.FullAccess(func(pod *corev1.PodSpec) { ... })`, written among the other Customizations. Go cannot pass a bare function literal where a closed interface is expected, so that named wrapper is what keeps the Customization set closed.

The function type is declared with the mapper, in `internal/cluster`, and the Step surface carries the functions a Workload line wrote beside the domain value it customized. `internal/scenario` therefore names no Kubernetes type and holds no function value, and `internal/lab` names the mapper's type rather than Kubernetes' own.

The mapper transcribes the specification's settings in the order it holds them, running each full access in the place it was written. A later Customization therefore stands where two name one Kubernetes field. The mapper then re-applies the specification's own tolerations, which are the lab's: the waiting time holds whatever full access left out. Full access takes the Pod specification alone, so the Workload label, the selector and the Deployment stay beyond it.

A Scenario writing full access imports the one Kubernetes package naming the Pod specification. That is the stated exception to the single-import rule: the depguard rule for the Scenarios' place admits that package, and the import rule beside it admits no other.

The control plane validates a Customization's values. Kubernetes rejects an empty topology key, a maximum Skew below one, and an unknown action or policy, so the Scenario ends at the Step that deployed the Workload, naming it. A setting the control plane accepts and the cluster cannot satisfy is a Placement the lab reports: Pods Pending with the scheduler's reason.

The mapper transcribes a specification onto the Pod template field by field. The container, the Workload label and the selector stay its own, because a specification is a package-level value and cannot name the Workload the selector reads.

Option 1 keeps the reading problem: the line still states rules. Option 2 hides Kubernetes' own names behind a vocabulary of the lab's, which is what the definition exists to show. Option 4 leaves the domain core with nothing to hold, and option 5 makes every Scenario a Kubernetes author, inverting "the lab's own vocabulary first, full access as an extension". Option 6 needs a method before a Scenario can set a field, and a value needs none.

Since the amendment of 2026-10-18, the definitions above live in `scenarios/pods.go`, written through `lab.PredefinedPodSpecification` with their own Startup time. The base and the definitions in `internal/lab` are retired. That amendment had a definition write the lab's two tolerations itself, through the toleration setting it offered, and the domain holds them at the lab's waiting time whatever is written, per [ADR: Every predefined Pod specification is defined by the Scenarios, with its own Startup time](2026-10-18-every-pod-specification-is-defined-by-the-scenarios.md). Since the second amendment of 2026-10-18, a definition names the Named setting for leaving an unresponsive Node and writes neither toleration itself. The Scenarios define that setting beside the specifications, as exactly the two tolerations under their Kubernetes names grouped through `lab.Settings`, per [ADR: The Scenarios name leaving an unresponsive Node as one setting](2026-10-18-the-scenarios-name-leaving-an-unresponsive-node-as-one-setting.md).

## Consequences

A Workload line reads as an intent, and the Step a run reports names that intent. A customized Workload reads the same way, since the Step names the specification and not its settings. Two Workloads under one intent therefore read alike where one was customized, which is what keeps the report free of configuration.

Since the second amendment of 2026-10-18, that holds of the Step name alone. The run prints every setting the Workload's Pods carry on lines of their own under the Given, so a customized Workload's lines differ there.

The definition is the whole of what decides a Placement. Until the amendment of 2026-10-18 the lab's own settings sat one hop away in the base. Since then a definition states every setting its Pods carry. Since the second amendment it writes each one itself except the lab's two tolerations, which it names through the Named setting for leaving an unresponsive Node.

`SpreadEvenly`, the Scenario-named spread domains and the spread-rule type are gone, and `scenario.SpreadDomain` stays beside `Placement` as the axis its Skew is read over. A Given's refusals are a Pod count below one and a specification holding no setting.

The eviction wait and the Kubernetes names the lab spells live in `internal/scenario`, beside the settings carrying them, and the mapper names neither.

A setting the lab spells wrong would be a Placement nobody asked for, which the mapping test's pinning prevents.

Default placement carries no spread, so where its Pods land is the scheduler's choice. A Scenario declaring it asserts only what holds for every Placement.

A setting's constructor returns no error, for either builder. The control plane is the validator, so a Scenario handles no error of its own and the lab keeps no copy of Kubernetes' admission rules.

The Workload-line rule reads that Step's own expression rather than the source line, so it admits a `Customize` call of any length and nothing after it. That last part is narrowed by [ADR: A Workload declares its Required Capacity, judged at the moment of the Failure and once Settled](2026-10-06-required-capacity-judged-at-two-moments.md): the rule admits one further call of the Workload's own, and no setting of the Scenario's own.

The specification set grows one entry per Scenario that needs one. The settings offered are the spread and the Request, and no predefined definition sets a Request. Zone spread only, soft spread and affinity between a Workload's Pods are not offered yet.

No Scenario of the lab names a Customization: the callers are the predefined definitions and the tests. One arrives with the Scenario whose intent needs it.

Full access leaves every domain value as it was: the function a Scenario wrote is held by the Step surface and read by the mapper alone.

A Scenario using full access reads in two vocabularies, the lab's under one prefix and Kubernetes' under another. A full access the control plane refuses ends the Scenario at the Step that deployed its Workload, as any refused Customization does.

A setting of the lab's own re-written after a full access keeps the place it was first written, so that full access stands where both name one field. Writing each field once leaves the order a Scenario reads.

No Scenario of the lab uses full access, so the single-import exception is unexercised by the sources the lab ships. The rule tests beside the machinery carry it meanwhile, on a Scenario source of their own.

## Implementation

**Requirements:** REQ-SIM-009, REQ-SIM-011

## References

- [system-design.md § Contracts](../system-design.md)
- [ADR: A Scenario's Workload declares the spread of its own Pods](2026-10-04-scenario-declared-pod-spread.md)
- [ADR: A Node Failure withholds the Node's status and shortens the eviction wait](2026-10-05-node-failure-withholds-the-nodes-status.md)
- [ADR: Every predefined Pod specification is defined by the Scenarios, with its own Startup time](2026-10-18-every-pod-specification-is-defined-by-the-scenarios.md)
