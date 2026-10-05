# A Node Failure withholds the Node's status and shortens the eviction wait

**Status:** Accepted

## Context

REQ-SIM-008 needs a Scenario to stop the Nodes it asks for responding, and then watch the control plane move their Pods away. The owner chose the unresponsive shape over a removed Node: "The Node stays in the cluster but goes NotReady, as when a machine hangs or loses network. Kubernetes moves its Pods away only after a waiting period." The owner also chose who sets that waiting period: "The lab deploys Workloads so Pods leave an unresponsive Node after a few seconds. Scenarios stay fast and need no wait setting. Differs from a default cluster, and the report says so."

Two things stand in the way. The KWOK controller maintains the status of every Node the lab creates, so a Ready condition the lab writes is restored. And a Pod carries the admission default of 300 seconds before it leaves a Node the control plane cannot reach, longer than the lab's whole settle wait.

Measured on the pinned image, KWOK v0.8.0 with Kubernetes v1.36.1: within a second of a Ready=Unknown write, the node-lifecycle controller taints the Node `node.kubernetes.io/unreachable` with the NoSchedule effect. One to two seconds later the KWOK controller restores Ready=True, with the condition set it writes at a Node's creation. The NoExecute taint and the eviction never followed. Four variants restored alike: the `kwok.x-k8s.io/status=custom` annotation written at the Failure, the same annotation carried from creation, the KWOK annotation and label removed, and Ready written False.

The image serves no `kwok.x-k8s.io` API group, and holds no CustomResourceDefinition. Its kwok-controller reads its Stages from a configuration file inside the container, and runs with `--manage-all-nodes=true`. That file's node-initialize Stage selects a Node by its Ready condition alone, never by a label. So there is no Stage to narrow through the API, and no marking on a Node exempts it.

The container's entrypoint runs `kwokctl create cluster "$@"`. The container's command is therefore forwarded verbatim as that command's arguments, and the image's own `KWOK_`-prefixed environment variables carry the same settings in environment form. The same configuration file shows kube-controller-manager running with `--node-monitor-period=25s` and `--node-monitor-grace-period=3m20s`, and kwok-controller with `--node-lease-duration-seconds=200`: five times Kubernetes' own values. Detecting a stopped heartbeat there takes minutes, not seconds.

Measured again with the arguments below in force: kwokctl accepted all three as written, and the failed Node's Ready condition read Unknown for the whole settle window. The unreachable taint carried the NoExecute effect a second after the status write, and the Pod on that Node carried a deletion timestamp two and a half seconds later. The other eight Nodes stayed Ready and untainted, and nothing finalized the deleted Pod. So the node-lifecycle controller takes a written condition as the observed one, rather than waiting out its grace period.

## Options Considered

How a Node stops responding:

1. **Write the Ready condition Unknown** — the KWOK controller restores it.
2. **Annotate the Node as carrying a status of its own, then write the condition** — measured on KWOK v0.8.0: the annotation changes nothing the controller does.
3. **Narrow every KWOK Node Stage through the API, then unmark the Node** — measured: this image serves no Stage to narrow.
4. **Start the cluster container with the controller managing marked Nodes alone, then unmark the Node** — measured: the controller maintains that Node no longer, and the written condition stands.
5. **Enable the Stage CRD on the cluster container, then narrow the Stages through the API** — the lab then owns every Stage the controller applies, in place of the image's own.
6. **Write the condition faster than the controller restores it** — a race whose outcome differs from run to run.
7. **Taint the Node unreachable from the lab** — the node-lifecycle controller owns that taint and removes it from a Node it reads Ready, which this Node would be.
8. **Taint the Node with a key of the lab's own** — nothing contests the taint, but the Node reads Ready: a drained Node rather than an unresponsive one.
9. **Delete that Node's Pods from the lab** — the lab, not Kubernetes, then moves them, which is the opposite of what the lab is for (G-1).
10. **Pin another cluster image version** — any version of that image is allowed, and nothing known says another release maintains a Node differently.

How long a Pod stays on it:

11. **The admission default of 300 seconds** — longer than every other wait in the lab.
12. **A toleration on the Workload's Pod template** — the admission default is only applied to a Pod declaring no toleration of its own, so the lab's value replaces it.
13. **Control-plane arguments on the cluster container** — they reach the control plane's own detection period, which no toleration reaches.

## Decision

Options 4, 12 and 13, with option 5 held in reserve.

A Failure carries the Nodes it stops, resolved from the count a When states, per [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md).

Provisioning creates the cluster container with arguments of the lab's own. Two of them take the KWOK controller off every Node and onto the Nodes carrying the lab's own marking, which is the `type=kwok` label its Nodes already carry. The third puts the control plane's heartbeat timings back to Kubernetes' own, in place of the image's five-fold scale. Every argument is composed from the lab's own constants, and the selector from the same label pair a Node is created with. So no run input reaches an argument, and the marking cannot drift. The engine surface keeps the property [ADR: The lab provisions the Simulated Cluster it drives](2026-10-03-lab-provisioned-simulated-cluster.md) records.

A Failure removes that label from its Node, and then writes that Node's Ready condition Unknown, with the reason a lost heartbeat carries. The Node keeps its KWOK Node annotation, so it still reads as a fake Node where that annotation is read.

The control plane does the rest. Its node-lifecycle controller taints the Node unreachable, with the NoSchedule and the NoExecute effect. Its taint-eviction controller deletes the Pods that stop tolerating that taint, and the Workload's ReplicaSet replaces them.

A Pod carrying a deletion timestamp is in no Placement. A Pod being deleted is leaving, which is how the Workload's own ReplicaSet counts it, so an evicted Pod holds no Placement open even where nothing finalizes it.

Every Workload's Pod template tolerates the two Node-condition taints, not-ready and unreachable, with the NoExecute effect and the lab's own waiting time. A constant in `internal/cluster` holds that time. Every run prints it against the 300 seconds a default cluster waits, beside the heartbeat timings the lab sets, and claims no total. The lab sets one part of a run's wait, the control plane the other.

A spread rule honours taints, per [ADR: A Scenario's Workload declares the spread of its own Pods](2026-10-04-scenario-declared-pod-spread.md). The Pods tolerate the NoExecute taint for seconds and the NoSchedule taint not at all, so the unreachable Node leaves the domain set and holds no replacement.

Where this kwokctl release refuses an argument, the same settings reach it as `KWOK_`-prefixed environment variables on the container, the precedence layer above its configuration file. Where neither takes the Node off the controller, option 5 follows and the lab owns the Stages.

## Consequences

An unresponsive Node reads NotReady wherever the lab reports a Node, and a Placement is read against Ready Nodes alone, so that Node leaves the observation. One surviving Node of its Zone then holds two Pods, and no Scenario asserts one Pod per Node across a Failure. Pods leave about four seconds after the Failure rather than minutes, so a Scenario stays fast. The lab owns a failed Node's status from the Failure on, and nothing refreshes that Node's heartbeat, which is what a hung machine leaves behind. The KWOK controller maintains nothing on a failed Node, its Pods included. So a Pod the control plane deletes there stays Terminating, in no Placement and read by no Scenario. That Node leaves the observation at once, while its Pod is deleted seconds later. Two observations inside that window agree, so a settled Placement must account for every Pod it found. A Pod on a Node the observation was not read against is an early Placement, not a settled one. The control plane notices a written condition within a period of Kubernetes' own length rather than the image's five-fold one, which is the lab's doing and is printed as such. Those timings hold for every Node of the cluster, so a Node whose heartbeat the lab never stops is unaffected. Zone loss is this Failure over every Node of one Zone, and meets the node-lifecycle controller's own per-Zone eviction limits, which that slice measures. A removed Node, and a Pod Failure, are other Failures and later slices.

## Implementation

**Requirements:** REQ-SIM-008

## References

- [system-design.md § Contracts](../system-design.md)
- [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md)
- [ADR: The lab is a Go program on a KWOK Simulated Cluster](2026-10-03-go-program-on-a-kwok-simulated-cluster.md)
- [ADR: The lab provisions the Simulated Cluster it drives](2026-10-03-lab-provisioned-simulated-cluster.md)
- [ADR: A Scenario's Workload declares the spread of its own Pods](2026-10-04-scenario-declared-pod-spread.md)
