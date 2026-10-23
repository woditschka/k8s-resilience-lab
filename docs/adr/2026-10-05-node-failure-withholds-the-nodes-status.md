# A Node Failure withholds the Node's status and shortens the eviction wait

**Status:** Accepted

## Context

REQ-SIM-008 stops a Node responding and watches the control plane move its Pods away. The owner chose that shape over a removed Node: "The Node stays in the cluster but goes NotReady, as when a machine hangs or loses network." The owner also chose who sets the wait: "The lab deploys Workloads so Pods leave an unresponsive Node after a few seconds."

Three things stand in the way, each measured on the pinned image, KWOK v0.8.0 with Kubernetes v1.36.1:

- The KWOK controller runs with `--manage-all-nodes=true` and restores Ready=True within two seconds of a written Unknown. No annotation, label or Stage served through the API exempts a Node.
- The image runs kube-controller-manager and kwok-controller at five times Kubernetes' own heartbeat timings, so a stopped heartbeat takes minutes to detect.
- A Pod carries the admission default of 300 seconds before leaving an unreachable Node. The node-lifecycle controller paces a Zone's tainting at 0.1 Nodes per second.

The container's entrypoint forwards its command to `kwokctl create cluster`, so the lab can set controller arguments. With the arguments below, a failed Node stayed Unknown, its NoExecute taint came a second after the write, and its Pod was deleted 2.5 seconds later. A Zone of three Nodes re-placed its Pods in 9.3 seconds.

## Options Considered

How a Node stops responding:

1. **Write the Ready condition Unknown** — the KWOK controller restores it.
2. **Annotate the Node as carrying its own status, then write it** — measured: the controller ignores the annotation.
3. **Narrow every KWOK Node Stage through the API** — measured: the image serves no Stage to narrow.
4. **The controller manages marked Nodes alone, and a Failure unmarks the Node** — measured: the written condition stands.
5. **Enable the Stage CRD and narrow the Stages through the API** — the lab then owns every Stage the controller applies.
6. **Write the condition faster than the controller restores it** — a race that differs from run to run.
7. **Taint the Node unreachable from the lab** — the node-lifecycle controller removes that taint from a Node it reads Ready.
8. **Taint the Node with a key of the lab's own** — a drained Node rather than an unresponsive one.
9. **Delete that Node's Pods from the lab** — the lab, not Kubernetes, then moves them, the opposite of G-1.
10. **Pin another image version** — nothing known says another release behaves differently.

How long a Pod stays on it:

11. **The admission default of 300 seconds** — longer than every other wait in the lab.
12. **A toleration on the Pod template** — the lab's value replaces the admission default.
13. **Control-plane arguments on the cluster container** — they reach the detection period no toleration reaches.

How fast a whole Zone's Pods leave:

14. **The stock eviction rate** — one Node per ten seconds.
15. **Fail a Zone's Nodes in parallel** — the pacing is the controller's queue, so nothing changes.
16. **An eviction rate on the cluster container** — measured: the Zone's Pods trail the Failure by one monitor pass.
17. **A shorter monitor period** — leaves stock Kubernetes behind on a timing every Node shares.
18. **A longer settle wait** — every Scenario waits out the pacing.

## Decision

Options 4, 12, 13 and 16. The Stage CRD of option 5 is enabled for another reason, per [ADR: The lab owns the simulator's Stages to give a Pod its Startup time](2026-10-08-the-lab-owns-the-simulators-stages-for-a-startup-time.md).

- The cluster container carries the lab's own arguments, composed from constants alone. The KWOK controller manages Nodes carrying the lab's `type=kwok` label alone. The control plane keeps Kubernetes' own heartbeat timings and a lifted per-Zone eviction rate.
- A Failure removes that label from its Node, then writes Ready Unknown with a lost heartbeat's reason. The Node keeps its KWOK annotation.
- The node-lifecycle controller taints the Node unreachable, and the taint-eviction controller deletes the Pods; the ReplicaSet replaces them.
- Every Deployment's Pod template tolerates the not-ready and unreachable taints, NoExecute, for the lab's waiting time.
- A Pod carrying a deletion timestamp is in no Placement.
- Every run prints the waiting time and the eviction rate against a default cluster's, beside the heartbeat timings, and claims no total.

## Consequences

Pods leave about four seconds after the Failure rather than minutes. The lab owns a failed Node's status from then on, so a Pod deleted there stays Terminating, in no Placement. A Zone Failure's Pods are back on the surviving Zones within seconds, one Pod skewed rather than even.

The Node-condition taints carry NoSchedule too, and a spread honours taints, so the unreachable Node leaves the domain set. A Failure of three or more Nodes leaving one Ready in their Zone stops the controller tainting that Zone; no Topology the lab runs reaches it.

## Implementation

**Requirements:** REQ-SIM-008

## References

- [system-design.md § Contracts](../system-design.md#contracts)
- [ADR: The lab provisions the Simulated Cluster it drives](2026-10-03-lab-provisioned-simulated-cluster.md)
- [ADR: A Deployment declares one of the lab's predefined Pod specifications](2026-10-05-workload-declares-a-predefined-pod-specification.md)
