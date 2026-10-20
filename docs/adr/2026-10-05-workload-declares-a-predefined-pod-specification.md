# A Workload declares one of the lab's predefined Pod specifications

**Status:** Accepted 2026-10-05, folding in the spread decision of 2026-10-04, options 13 and 25; amended 2026-10-18: the Scenarios define each predefined specification with its own Startup time, options 15 and 17; amended 2026-10-18: a definition names the Named setting for leaving an unresponsive Node, and a run reports every setting a Workload's Pods carry, options 22 and 24; amended 2026-10-18: the names moved to the subject packages `pod` and `node`, and a setting is a keyed literal, per [ADR: Scenario code stays idiomatic Go, read by subject](2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md)

## Context

On 2026-10-04 the simplest Scenario, Resilient placement, had to show resilience to Node and Zone loss: its nine Pods sit three in every Zone and one on every Node. An even spread is one Scenario's goal rather than the lab's law. A later Scenario declares another specification, or none with a spread, and the lab reports the Placement it produces. A spread also has to survive the loss it is built for: one the surviving Nodes cannot satisfy leaves Pods Pending.

On 2026-10-05 a Scenario declared its spread itself: `SpreadEvenly(lab.OverZones, lab.OverNodes)`. That line reads as a rule, so a reader infers the intent behind it, while each Scenario exists to show one intent, configured well or badly. The owner named the intents rather than their mechanics: a specification conveys "that the Pod placement should target high resilience against Failure", not that it spreads evenly. The owner also asked for predefined specifications to be changed before use: "I feel there should be multiple predefined Pod specifications (as methods) ... that I can get and then still tinker with". The owner chose one `Customize` call taking the changes as values over a chain of named steps.

The lab adds settings of its own, the tolerations that make Pods leave an unresponsive Node. Those decide a Placement as much as a spread does. On 2026-10-05 they sat in a base in `internal/lab` that every definition started from.

On 2026-10-18 REQ-SIM-015 held that every value deciding a Placement stands on a Scenario's own lines, stated there or named there. The owner asked: "the size of astandard node and standard pod lives deep in the framework - should it not live in the scenarios?" The definitions moved to `scenarios/pods.go`, each with its own Startup time, and each wrote the lab's two tolerations itself. REQ-SIM-009 holds that a definition shows every setting it holds under its Kubernetes name, and REQ-SIM-008 that a Scenario states no waiting time of its own.

The same day the owner read Resilient placement in `scenarios/pods.go` and found it hard to read. Two of its four settings repeated in every definition and decided nothing there. Asked whether REQ-SIM-009 changes, the owner answered: "we need to stay as close as possibe to the k8s naming , this does not mean we cannot create a method on a higher level that names the lower level config". A first design placed that name in `internal/lab`. The owner refused it: "ResilientPlacement had the settings directly, LeaveUnresponsiveNodes just aggregates the direct settings on a higher level, so i think it would alos belong to pods.go".

Later that day the names moved to the subject packages `pod` and `node`, and each setting became a keyed literal, per [ADR: Scenario code stays idiomatic Go, read by subject](2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md).

`internal/scenario` imports no Kubernetes package, per [ADR: The lab is a Go program on a KWOK Simulated Cluster](2026-10-03-go-program-on-a-kwok-simulated-cluster.md).

## Options Considered

What a specification is:

1. **Named helpers over the declared rules**: `SpreadEvenly` stays and a helper expands to the same rules.
2. **A spread abstraction of the lab's own**: a rule names a domain, the Zones or the Nodes, and the mapper picks its label.
3. **Settings under Kubernetes' own names, in the lab's own types**: each predefined specification is the settings it sets.
4. **Predefined Kubernetes Pod templates in `internal/cluster`**, picked by name.
5. **A `*corev1.PodSpec` builder in `internal/lab`**, the predefined ones being its starting points.

How a Scenario changes one:

6. **A chained method per ready-made change**: `ResilientPlacement().ZoneSpread(ScheduleAnyway)`.
7. **One `Customize` call taking the changes as values**.

Where a Workload states its specification:

8. **A clause on the Workload line**, `.With(lab.ResilientPlacement())`, with a rule that every line carries one.
9. **A mandatory argument of its Given**.

How Resilient placement spreads its Pods:

10. **Even spread in the mapper**: every Workload spread alike, and no Scenario able to declare otherwise.
11. **Required Pod anti-affinity over the hostname label**: at most one Pod per Node.
12. **Preferred spread**: the scheduler places the Pod anyway when it misses evenness.
13. **Topology spread constraints declared per Workload, enforced**: a maximum Skew per domain, unmet as Pending.

Where a predefined specification is defined:

14. **In `internal/lab`**: the values deciding a Placement stay in the framework.
15. **In the Scenarios' place, through a mechanism the lab offers**, as `scenarios/nodes.go` defines each Named Node capacity.

How a definition states its Startup time:

16. **As one more Customization**: a definition omitting it compiles and is refused at run time.
17. **As a mandatory argument after the name**: a definition omitting it does not compile.

How a definition shows the lab's two tolerations:

18. **The mechanism adds them**: the definition no longer shows them.
19. **The definition writes both itself**: every definition repeats two settings it cannot change. Chosen in the morning of 2026-10-18.
20. **The mechanism refuses a definition lacking them**: a Customization could still write another time for either taint.
21. **One Named setting the lab defines**, composed in `internal/lab`.
22. **One Named setting the Scenarios define, through a grouping means the lab offers**.

Where a run reports the settings a Workload's Pods carry:

23. **In the Given Step's name**: one line of several hundred characters, quoted again by every Finding naming that Step.
24. **On lines of their own under the Given**.

Whether a spread declares a minimum domain count:

25. **None**.
26. **A declared count**.

## Decision

Options 3, 7, 9, 13, 15, 17, 22, 24 and 25.

- A Workload's Given takes exactly one `pod.Specification` as a mandatory argument, with zero or more Customizations through `Customize` and nothing else. `Customize` returns a changed copy and keeps the last setting written for a field.
- A specification is an immutable value of the domain core holding its name, its Startup time and its settings: one topology spread per topology key, one toleration per taint, one Request. Each setting is a keyed literal under Kubernetes' Go type and field names: `pod.TopologySpreadConstraint`, `pod.Toleration` and `pod.Request`.
- `internal/scenario` spells the Kubernetes label, taint and value names itself, and the mapping test pins each one to the client library's constant. The domain holds a quantity as the integer it carries exactly: millicores for cpu, mebibytes for memory. A Node capacity uses the same two types.
- `scenarios/pods.go` defines each predefined specification once, as an exported function taking nothing and named for its intent, such as `ResilientPlacement()` and `DefaultPlacement()`. Its body is one `pod.Predefined(name, startupTime, settings...)` call, carrying the domain value or its first refusal, as `node.Named` does. The lab defines no specification.
- The Startup time is the mandatory second argument. The four Startup forms return one `pod.StartupTime`, which is also a Customization. The domain refuses the zero Startup time and an empty name, and the lab holds no Startup time of its own.
- Every definition names `LeaveUnresponsiveNodes()` and writes neither toleration itself. `scenarios/pods.go` defines that Named setting as one `pod.Settings` call of exactly the not-ready and unreachable tolerations at `node.UnresponsiveWait`. It is a function rather than a variable, because no package of the lab holds mutable state.
- `pod.Settings` takes `pod.Setting` values alone, a sealed type: a Startup time, a full access or another group does not compile there. It is the shortcut [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md) admits, expanding to existing settings. The lab defines no Named setting.
- The domain holds the not-ready and unreachable tolerations at the lab's waiting time, replacing any time written for them and adding any not written.
- Resilient placement spreads over the Zone and the hostname label through the Even spread, `EvenSpreadOver`, which the Scenarios define. Each constraint has a maximum Skew of one, `DoNotSchedule`, and `Honor` for taints. No constraint declares a minimum domain count. Every field is generally available in the pinned Kubernetes version, per [ADR: Kubernetes version pin for the Simulated Cluster](2026-10-03-kubernetes-version-pin.md).
- Full access is the Customization a Scenario writes when the lab names no setting it needs: `pod.FullAccess(func(pod *corev1.PodSpec) { ... })`. The named wrapper keeps the Customization set closed, since Go passes no bare function literal where a closed interface is expected.
- The function type is declared with the mapper in `internal/cluster`. The Step surface carries the functions a Workload line wrote beside the domain value, so `internal/scenario` names no Kubernetes type and holds no function value.
- A Scenario writing full access imports the one Kubernetes package naming the Pod specification. The depguard rule for the Scenarios' place admits that package and no other.
- The mapper transcribes a specification onto the Deployment's Pod template field by field, in the order held, each full access in its written place. It then re-applies the lab's two tolerations. The container, the Workload label and the selector stay its own, beyond full access.
- The control plane validates a setting's values. A value it refuses ends the Scenario at the Step that deployed the Workload, naming it. A value it accepts and the cluster cannot satisfy is a Placement the lab reports: Pods Pending with the scheduler's reason.
- A Given declaring a Workload names its specification, then the shortest and longest Startup time its Pods take. The run prints every setting the domain holds, in order, on a line of its own under that Given, as a manifest writes it:

  ```text
      tolerations: {key: node.kubernetes.io/not-ready, operator: Exists, effect: NoExecute, tolerationSeconds: 2}
  ```

- A Named setting reports as the settings it stands for. A full access reports as one line saying it applies, since the lab cannot read its change back. A refused specification reports no setting.
- A source rule holds `scenarios/pods.go` to definitions and Named settings alone, one per name. Each definition names `LeaveUnresponsiveNodes` and writes neither toleration. No production source under `internal/` or `cmd/` defines a specification or calls `pod.Settings`.

Option 1 keeps the reading problem: the line still states rules. Option 2 hides Kubernetes' own names, which a definition exists to show. Option 4 leaves the domain core nothing to hold. Option 5 makes every Scenario a Kubernetes author. Option 6 needs a method before a Scenario can set a field, and a value needs none. Option 8 needs a rule to make a clause mandatory, which an argument is for free.

Option 10 lets no Scenario declare otherwise. Option 11 leaves the ninth Pod Pending once a Node is lost, while a maximum Skew of one lets it double up on a survivor. Option 12 hides an unmet rule behind a Running Pod in the wrong place. Option 26 fails for the reason option 11 does, per [ADR: Provisioning ends when the scheduler has placed a Pod on every Node](2026-10-04-scheduler-observable-provisioning.md). That ADR guarantees the scheduler has seen every Node before a Workload is deployed. Honouring taints lets a cordoned Node leave the count rather than making every rule unsatisfiable.

Option 14 keeps a Placement-deciding value in the framework, which REQ-SIM-015 refuses. Option 16 compiles a definition missing a mandatory value. Option 18 hides a setting deciding a Placement, against REQ-SIM-009. Option 19 is what the owner found hard to read. Option 20 lets a Customization change the waiting time, against REQ-SIM-008. Option 21 puts a Scenario-level name in the machinery, which the owner refused. Option 23 makes a Step's name carry configuration wherever it is quoted.

## Consequences

A Workload line reads as an intent, and its Step names that intent. The setting lines under the Given carry the configuration, so two Workloads under one intent share a Step name and differ there where one was customized.

A reader of `scenarios/pods.go` sees every value a specification gives its Pods: the spreads, the Request, the Startup time, and the tolerations through one named line beside it. Every run prints that line expanded.

The Resilient placement Scenario's assertion follows from its own declaration. Default placement carries no spread, so a Scenario declaring it asserts only what holds for every Placement.

Spread is evaluated when a Pod is scheduled and never after. A Node returning from a Failure stays empty until Pods are recreated, so no Scenario asserts a Placement re-balancing itself. A Topology too small for its Workload reports Pending Pods rather than an uneven Placement, the PRD's edge case 1.

A Customization writing another time for either toleration changes nothing, and the setting line reports the lab's time. A Scenario wanting another Skew writes the constraint itself rather than the Even spread.

A Scenario handles no error of its own and the lab keeps no copy of Kubernetes' admission rules. A Given's refusals are a Pod count below one, a specification holding no setting, an empty name, the zero Startup time, and a setting with a field left out. The mapping test's pinning keeps the lab from spelling a setting wrong.

The Workload-line rule reads that Step's own expression, admitting a `Customize` call of any length. [ADR: A Workload declares its Required Capacity, judged at the moment of the Failure and once Settled](2026-10-06-required-capacity-judged-at-two-moments.md) admits one further call of the Workload's own.

A Pod template carries the two Startup annotations from its specification's own Startup time, and the lab's Stage set is unchanged, per [ADR: The lab owns the simulator's Stages to give a Pod its Startup time](2026-10-08-the-lab-owns-the-simulators-stages-for-a-startup-time.md).

Tests of the machinery define the specifications they need beside themselves, writing the two tolerations where a fixture needs them, since `internal/` sources cannot import the Scenarios' place.

Two definitions under one name, each in its own Scenario source, are not refused within a run. The place rule keeps the lab's own Scenarios to one per name. A further specification or Named setting is one function in `scenarios/pods.go`, arriving with its first Scenario, and the lab's code does not change.

A Scenario using full access reads in two vocabularies. A lab setting re-written after a full access keeps its first place, so the full access stands where both name one field. No Scenario of the lab uses full access, so rule tests beside the machinery exercise the import exception on a Scenario source of their own.

## Implementation

**Requirements:** REQ-SIM-001, REQ-SIM-009, REQ-SIM-011, REQ-SIM-012, REQ-SIM-015, REQ-SIM-016

## References

- [system-design.md § Contracts](../system-design.md#contracts)
- [ADR: The lab is a Go program on a KWOK Simulated Cluster](2026-10-03-go-program-on-a-kwok-simulated-cluster.md)
- [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md)
- [ADR: Kubernetes version pin for the Simulated Cluster](2026-10-03-kubernetes-version-pin.md)
- [ADR: Provisioning ends when the scheduler has placed a Pod on every Node](2026-10-04-scheduler-observable-provisioning.md)
- [ADR: A Node Failure withholds the Node's status and shortens the eviction wait](2026-10-05-node-failure-withholds-the-nodes-status.md)
- [ADR: A Workload declares its Required Capacity, judged at the moment of the Failure and once Settled](2026-10-06-required-capacity-judged-at-two-moments.md)
- [ADR: The lab owns the simulator's Stages to give a Pod its Startup time](2026-10-08-the-lab-owns-the-simulators-stages-for-a-startup-time.md)
- [ADR: A Topology declares the NodePools every Zone holds](2026-10-08-a-topology-declares-the-nodepools-every-zone-holds.md)
- [ADR: Scenario code stays idiomatic Go, read by subject](2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md)
