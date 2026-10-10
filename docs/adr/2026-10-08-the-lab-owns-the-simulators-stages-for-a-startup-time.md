# The lab owns the simulator's Stages to give a Pod its Startup time

**Status:** Accepted, amended 2026-10-09: the lab's own Startup time is a range, and a Startup time is stated in seconds or milliseconds (REQ-SIM-012)

## Context

REQ-SIM-012 gives every placed Pod a Startup time before it is Ready. The owner asked for it "on the pod definition so the scenarions are persitent", as one time or a shortest and a longest.

In a KWOK Simulated Cluster no kubelet runs, so a Pod becomes Ready when the KWOK controller applies its pod-ready Stage. The image runs KWOK's fast Stage set, whose pod-ready Stage fires at once and reads no delay. KWOK's general pod-ready Stage reads two per-Pod annotations instead: a delay, and a jitter delay bounding a time drawn at random above it.

The image serves no `kwok.x-k8s.io` API group, and its controller reads its Stages from a configuration file inside the container, per [ADR: A Node Failure withholds the Node's status and shortens the eviction wait](2026-10-05-node-failure-withholds-the-nodes-status.md). That ADR held the Stage CRD in reserve: with it enabled, the controller applies the Stages served through the API. Whether kwokctl then also seeds its own Stages there was unmeasured when this decision was taken.

The amendment's context: the owner found the fixed two seconds slow and asked for "0.5 to 1 s", stated through "a second one ...Millis" beside the seconds forms. Whole seconds cannot state a time under one second.

## Options Considered

1. **Per-Pod annotations alone** — the image's fast pod-ready Stage reads none, so nothing changes.
2. **A kwokctl configuration file carrying the Stages** — the file must reach the container, through a mount or an upload. Either widens the engine surface [ADR: The lab provisions the Simulated Cluster it drives](2026-10-03-lab-provisioned-simulated-cluster.md) restricts to pull, create and start.
3. **The lab writes each Pod's Ready condition itself after its time** — it races the controller's own pod-ready Stage, and the lab rather than the simulator then decides readiness.
4. **Enable the Stage CRD and apply the lab's own Stage set through the API** — one more constant container argument, and the client library already holds the boundary.

## Decision

Option 4. The cluster container is created with the Stage CRD enabled, among the lab's constant arguments. Provisioning then makes the served Stages exactly the lab's own set before it creates any Node. Each Stage of that set is created or replaced by name, and any other Stage is removed. So the outcome holds whether or not kwokctl seeds Stages of its own.

The lab's set follows KWOK's published Stage set at the pinned release, except pod-ready, which is KWOK's general form reading the two delay annotations. The set is held as the lab's constants, applied through `k8s.io/client-go`'s dynamic client, and moves with the KWOK release pin. That Stage's own delay is none, so a Pod carrying no annotation, a provisioning probe among them, is Ready at once.

A Workload's Pod template always carries both annotations. Both are the Startup time its specification states, or the lab's own when it states none. A shortest and a longest set the delay and the jitter delay. The annotations sit on the template's metadata, which full access does not reach, so a Startup time stands whatever full access names.

Since the amendment of 2026-10-09, the lab's own Startup time is a range, a shortest and a longest both above zero, held as two constants in `internal/scenario`. Each Pod draws its own time within it through the same two annotations, and every run prints both bounds beside the other times the lab sets. A shortest of zero was declined: some Pods would then be Ready at placement, hiding the Pending-on-Node state REQ-SIM-012 exists to show.

A Scenario states a Startup time in whole seconds or whole milliseconds, through a pair of Customizations per unit. Both pairs build the one Startup time, so the later Customization wins whatever its unit. A range is written in one unit, milliseconds when its bounds would mix units. The domain value is built from whole milliseconds, the finest unit a Scenario states. A duration value at the Scenario surface was declined at the owner's choice of plain counts with the unit in the name.

The two facts this rests on were measured on the pinned image, and both hold:

- With the Stage CRD enabled, the controller applies only the Stages served through the API. kwokctl serves none of its own, and the configuration file's Stages go unapplied.
- The general pod-ready Stage reads the delay as the lower bound and the jitter delay as the upper bound of the time it draws. Equal values give exactly that time.

The lab's Stage set was measured on the same image: probe Pods are Ready at once, a deleted Pod is removed, and Node leases renew.

## Consequences

The lab owns every Stage the KWOK controller applies, Node Stages included, rather than the image's set. A KWOK upgrade is therefore a review of the lab's Stage set, as it already was for node-initialize, per [ADR: The lab is a Go program on a KWOK Simulated Cluster](2026-10-03-go-program-on-a-kwok-simulated-cluster.md). A later Startup-like time, such as a Pod's time to stop, is one more delay-reading Stage in the same set.

A placed Pod inside its Startup time reads Pending on its Node, so a Placement holding one has not stopped changing and is not Settled. Every Workload Pod, a replacement among them, takes the lab's own Startup time, so every Scenario waits that much longer for Ready Pods.

Since the amendment, Pods under the lab's own Startup time take different times, so two runs of one Scenario may record different Timelines. Each Scenario must still reach its asserted outcome, and no outcome promises an identical Timeline. The annotations carry sub-second durations, which the general pod-ready Stage reads as it reads whole seconds. Kubernetes reports a condition's transition in whole seconds, so a sub-second bound is judged on the observer's own clock.

The engine surface is unchanged: the Stages travel through the API the lab already drives, and no run input reaches a Stage.

## Implementation

**Requirements:** REQ-SIM-012

## References

- [system-design.md § Contracts](../system-design.md)
- [ADR: A Node Failure withholds the Node's status and shortens the eviction wait](2026-10-05-node-failure-withholds-the-nodes-status.md)
- [ADR: The lab is a Go program on a KWOK Simulated Cluster](2026-10-03-go-program-on-a-kwok-simulated-cluster.md)
- [ADR: The lab provisions the Simulated Cluster it drives](2026-10-03-lab-provisioned-simulated-cluster.md)
- [ADR: A Workload declares one of the lab's predefined Pod specifications](2026-10-05-workload-declares-a-predefined-pod-specification.md)
