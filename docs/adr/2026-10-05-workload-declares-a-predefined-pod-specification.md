# A Deployment declares one of the lab's predefined Pod specifications

**Status:** Accepted

## Context

Each Scenario exists to show one placement intent, configured well or badly. The owner named intents rather than mechanics: a specification conveys "that the Pod placement should target high resilience against Failure". The owner asked for "multiple predefined Pod specifications (as methods) ... that I can get and then still tinker with".

Every value deciding a Placement stands on a Scenario's own lines (REQ-SIM-015): "should it not live in the scenarios?" A definition shows each setting under its Kubernetes name (REQ-SIM-009), and "this does not mean we cannot create a method on a higher level that names the lower level config". A Scenario states no waiting time of its own (REQ-SIM-008).

The owner wanted each Pod's share of its Node visible on its Deployment line: "the cpu and memory requests of pods are important in my view to see how full the node is". The lab creates a Kubernetes Deployment and no other kind, so the Given declares one (REQ-SIM-025).

## Options Considered

What a specification is:

1. **Named helpers over declared rules** — the line still states rules.
2. **A spread abstraction of the lab's own** — hides Kubernetes' names.
3. **Settings under Kubernetes' own names, in the lab's own types**.
4. **Kubernetes Pod templates in `internal/cluster`** — leaves the domain core nothing to hold.
5. **A `*corev1.PodSpec` builder** — makes every Scenario a Kubernetes author.

How a Scenario changes one, and where it is stated:

6. **A chained method per change** — a method before a Scenario can set a field.
7. **One `Customize` call taking the changes as values**.
8. **A clause on the line, made mandatory by a rule**.
9. **A mandatory argument of its Given**.

How Resilient placement spreads its Pods:

10. **Even spread in the mapper** — no Scenario can declare otherwise.
11. **Required Pod anti-affinity** — leaves a Pod Pending once a Node is lost.
12. **Preferred spread** — hides an unmet rule behind a Running Pod.
13. **Topology spread constraints, enforced**, with no minimum domain count.
14. **A declared minimum domain count** — fails as option 11 does, per [ADR: Provisioning ends when the scheduler has placed a Pod on every Node](2026-10-04-scheduler-observable-provisioning.md).

Where definitions and the lab's tolerations live:

15. **Definitions in `internal/lab`** — a Placement-deciding value in the framework.
16. **Definitions in the Scenarios' place**, through a mechanism the lab offers.
17. **The Startup time as a Customization** — a definition omitting it compiles.
18. **The Startup time as a mandatory argument**.
19. **The mechanism adds the tolerations unseen** — against REQ-SIM-009.
20. **Each definition writes both tolerations** — two settings it cannot change, hard to read.
21. **A Named setting composed in `internal/lab`** — refused by the owner.
22. **A Named setting the Scenarios define**, through a grouping means the lab offers.
23. **Settings reported in the Given's name** — configuration quoted in every Finding.
24. **Settings reported on lines of their own under the Given**.

Where a Deployment's Request stands, and what the Given declares:

25. **A mandatory Customization between the count and the specification**.
26. **A mandatory `pod.Request`** — refuses a line asking for nothing.
27. **One variadic list** — compiles a line with no specification.
28. **The GPU Request inside its toleration's specification** — a Request the lines do not show.
29. **A Workload, Pods, or a typed count** — below the floor of Kubernetes naming, or a lab type for a parameter's job.
30. **A Deployment of a plain `int` replica count**.

## Decision

Options 3, 7, 9, 13, 16, 18, 22, 24, 25 and 30.

- `Deployment(name, replicas, request, specification)`: an `int` count, one `pod.Customization` for the Request, then one `pod.Specification`. A line naming no Request writes `pod.Settings()`, and its Pods ask for nothing.
- `Customize` returns a changed copy, the last setting per field winning, and the line's Request applies last.
- A specification is an immutable domain value of name, Startup time and settings: one spread per topology key, one toleration per taint, one Request. Each setting is a keyed literal under Kubernetes' Go names. `internal/scenario` spells the Kubernetes names itself, pinned by the mapping test.
- `scenarios/pods.go` defines each specification once, as a function taking nothing, through `pod.Predefined(name, startupTime, settings...)`. The lab defines none.
- Every definition names `LeaveUnresponsiveNodes()`, the Named setting of the not-ready and unreachable tolerations at the lab's waiting time. The domain holds both at that time, whatever is written.
- No definition holds a Request. Standard Pod and GPU Pod are Named settings of one Request each.
- `pod.Settings` takes sealed `pod.Setting` values alone. Resilient placement names the Even spread, `EvenSpreadOver`, over the Zone and hostname labels.
- `pod.FullAccess` reaches every other Pod field. Its function type lives with the mapper in `internal/cluster`, and the Scenarios' depguard rule admits the one package naming the Pod specification.
- The mapper transcribes the settings in order, each full access in place, then re-applies the lab's tolerations. The container, label and selector stay its own. The control plane validates values: a refusal ends the Scenario at the Wait applying it.
- The run prints every setting the domain holds on a line of its own under the Given, as a manifest writes it.

## Consequences

A Deployment line reads as an intent, and the lines under it carry the configuration. Spread is evaluated at scheduling and never after, so no Scenario asserts a Placement re-balancing itself. A Topology too small for its Deployment reports Pending Pods. A Scenario using full access reads in two vocabularies, and a source rule holds `scenarios/pods.go` to definitions and Named settings. A further specification is one function there, and the lab's code does not change.

## Implementation

**Requirements:** REQ-SIM-001, REQ-SIM-009, REQ-SIM-011, REQ-SIM-012, REQ-SIM-015, REQ-SIM-016, REQ-SIM-024, REQ-SIM-025

## References

- [system-design.md § Contracts](../system-design.md#contracts)
- [ADR: Scenario code stays idiomatic Go, read by subject](2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md)
- [ADR: A Node Failure withholds the Node's status and shortens the eviction wait](2026-10-05-node-failure-withholds-the-nodes-status.md)
- [ADR: The lab owns the simulator's Stages to give a Pod its Startup time](2026-10-08-the-lab-owns-the-simulators-stages-for-a-startup-time.md)
