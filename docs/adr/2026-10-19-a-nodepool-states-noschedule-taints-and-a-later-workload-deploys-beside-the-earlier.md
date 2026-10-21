# A NodePool states NoSchedule taints, and a later Workload deploys beside the earlier, judged alone

**Status:** Accepted, amended 2026-10-20: every Wait's Placement board observes every deployed Workload, option 9

## Context

REQ-SIM-019 lets a NodePool state taints its Nodes carry and a Pod specification tolerate one. Its Scenario keeps a general Workload off tainted gpu Nodes, then places a tolerating Workload on them by its Request. The owner answered the scoping questions with "yes": NoSchedule alone, and the tolerating Workload reaches the gpu NodePool by its Request, with no node selection setting.

Four points constrain later slices. They are how a NodePool states a taint, which fields a toleration requires, how provisioning ends on a tainted Node, and what a second Workload Given in one Scenario means. Before this slice a second Workload Given replaced the first, and no Wait deployed it.

On 2026-10-20, REQ-SIM-021 asked for a board of the whole cluster under every Wait. The owner noted "there could also be multiple workloads". The decision asked for "a board of the whole cluster: one Node per line, grouped by Zone with its NodePool, listing every Pod on it as Workload name plus the Pod's own suffix". Option 6 had every Wait observe the newest Workload alone, so no Wait saw an earlier Workload's Pods.

## Options Considered

How a NodePool states a taint:

1. **A further call on the declared NodePool**, `.Taints(node.Taint{…})`, each taint a keyed literal of key, value and effect, as Kubernetes' own taint type names them.
2. **A taint folded into `node.Labels`**: a taint is not a label in Kubernetes, and its effect has no place in a label map.

Which fields a toleration requires:

3. **Every field always**: a NoSchedule toleration would state a waiting time Kubernetes refuses.
4. **The fields its operator and effect take**: Equal takes a value and Exists none, NoExecute takes a waiting time and NoSchedule none.

What a later Workload Given means:

5. **It replaces the earlier**: the earlier Workload's Pods vanish before the later one is placed, so the Scenario can never show both on one Topology.
6. **It deploys beside the earlier, judged alone**: the next Wait deploys it, the earlier stays deployed, and every Wait, When and Then after it judges the newest Workload.
7. **Every Then reads every deployed Workload**: Thens on one line would read different subjects, and a Then would need a Workload argument.

Where a Wait's Placement board comes from (2026-10-20):

8. **Folded from the Timeline's entries up to the Wait**: the Timeline orders streams by the lab's own receipt, and no Wait would own the Placement it reports.
9. **One observation of the whole cluster each Wait takes, reached or not**: every Topology Node and every Pod carrying the lab's Workload label, apart from the observation the Wait judges.

## Decision

Options 1, 4, 6 and 9.

- `node.Taint` is a keyed literal of key, value and effect. `.Taints(…)` on a declared NodePool adds them, and may be written more than once.
- Only the NoSchedule effect is admitted. Any other effect, a field left out, a key or value outside Kubernetes' syntax, one key stated twice, and a key under the control plane's Node-condition prefix are refused at the Topology Given.
- The domain holds the taint syntax as its own rules, naming no Kubernetes package. A mapper test pins them to the client library.
- Each probe Pod tolerates its own Node's taints. Without it the scheduler never places a probe on a tainted Node, and provisioning fails.
- A toleration requires the fields its operator and effect take. A field stated against them is refused at the Workload's Given, as one left out is.
- A later Workload Given deploys at the next Wait beside the earlier ones. Every Wait, When and Then after it judges the newest Workload alone. A Workload name already declared in the Scenario is refused at its Given.
- Every Wait, reached or not, also observes the whole cluster once for its Placement board: every Workload's Pods on every Topology Node, and no probe Pod. The board judges nothing.
- A Then counts Pods on one NodePool, and a name the Topology lacks ends the Scenario at that Then.

Option 8 reports a Placement no Wait observed. Option 2 departs from Kubernetes' names. Option 3 makes a Scenario state a value Kubernetes refuses. Option 5 cannot show two Workloads sharing a Topology. Option 7 gives one line's Thens different subjects and widens every Then.

## Consequences

A Scenario's gpu NodePool reads as a manifest's taint: `node.Pool("gpu", node.PerZone(1), GPUNodes()).Taints(node.Taint{Key: GPU, Value: GPUPresent, Effect: node.NoSchedule})`.

A Scenario may judge one Workload, then place another beside it and judge that. An earlier Workload is never judged again, yet every board after it shows its Pods.

A Wait costs one more observation, and a Wait the Scenario ends at still shows the cluster as it stood. The board and the Thens may read the newest Workload at two moments a poll apart.

PreferNoSchedule and NoExecute taints stay refused until a slice admits them. NoExecute meets the Failure path, whose tolerations every specification holds.

Drawing a Workload onto a NodePool by node selection or affinity stays an Open Question.

## Implementation

**Requirements:** REQ-SIM-019, REQ-SIM-021

## References

- [system-design.md § Contracts](../system-design.md#contracts)
- [ADR: A Topology declares the NodePools every Zone holds](2026-10-08-a-topology-declares-the-nodepools-every-zone-holds.md)
- [ADR: Scenario code stays idiomatic Go, read by subject](2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md)
- [ADR: Provisioning ends when the scheduler has placed a Pod on every Node](2026-10-04-scheduler-observable-provisioning.md)
