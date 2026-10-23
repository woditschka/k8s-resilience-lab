# A Scenario writes its own Wait, with its Given or as a Step of its own

**Status:** Accepted

## Context

The time a Scenario waits is the lab's own subject: after a Failure it is how long the cluster took to place the Pods again. The owner asked to see it: "A When(... status has settled ...) would make the timing and the precondition visible (possible with a timeout)". The owner asked for Kubernetes' words, "AllPodsReady", and a chained spelling, "How about `.WaitFor().AllNodesReady()`?". On a stated time: "a timeout should fail the test", written as "a second version of AllPodsReady: AllPodsReadyWithinSeconds(15)".

A Scenario expecting Pods to stay Pending (REQ-SIM-011) needs the lab's own Settled state, since no Pod there is Ready. The owner found a printed time ambiguous: "is it the upper limit or the real time it took?" For the lab's own Scenarios the owner chose "(c) standalone Waits", and Scenario comments carrying "just a short description of the high-level intention".

A Scenario may place a second Deployment beside the first (REQ-SIM-019), and the owner asked for "a board of the whole cluster" under every Wait (REQ-SIM-021).

## Options Considered

What a Scenario waits for:

1. **The lab's own predicate alone** — a state only this lab defines.
2. **Kubernetes' Ready condition**, `AllPodsReady` and `AllNodesReady`.
3. **Live** — no condition behind it, since no probe runs.
4. **The lab's Settled state, as a third Wait**, `PlacementSettled`, for a Placement holding Pending Pods.
5. **A Wait naming the Pending count** — judges where a Then judges.
6. **`AllPodsReady` widened to Pending Pods** — one name for two states.

How a Wait is written:

7. **`WaitFor()` on the Scenario, or chained on the Given before it**, offering the one Wait that fits what that Given declared.
8. **Folded into the Given** — declined as noise.
9. **`When()` both ways, or `WaitFor()` chained and `When()` standalone** — two meanings or two spellings of one Step.
10. **Any Step kind chainable in any sequence** — syntax rather than scope.

When a declaration reaches the cluster:

11. **At the first Step after the Given's line, a Wait** — one line reaches the cluster once.
12. **At the Given's own call** — an optional further call re-creates the same Nodes.
13. **A Then judging the Placement first settled** — after a Failure it judges the Placement from before.

How a Wait states and reports its time:

14. **A typed argument, or a constant set** — one more type, or no Scenario's own statement.
15. **A second name per Wait**, `AllPodsReadyWithinSeconds(15)`, in whole seconds.
16. **Cancelled at the stated time** — reports the bound rather than the recovery.
17. **The lab's waiting time waited out, the measured time judged against the stated**.
18. **A time above the waiting time clamped** — a miss read against a time never waited.
19. **`after`, `within` or `in` alone, `took … within`, or a bracketed bound** — none says which time a reader acts on.
20. **`reached in 6.5s of the 30s stated`**, the stated part only where the Scenario stated one.

What a later Deployment Given means:

21. **It replaces the earlier** — two Deployments can never share a Topology.
22. **It deploys beside the earlier, and later Steps judge the newest**.
23. **Every Then reads every Deployment** — one line's Thens read different subjects.

Where a Wait's board comes from:

24. **Folded from the Timeline** — no Wait owns the Placement it reports.
25. **One observation of the whole cluster per Wait, reached or not**.

What a Scenario's doc comment states:

26. **The Steps in prose, or the function name first** — restates the lines below.
27. **The intention alone**.

## Decision

Options 2, 4, 7, 11, 15, 17, 20, 22, 25 and 27.

- A Wait is the fourth Step kind: a Given declares, a When changes the cluster, a Wait waits, a Then judges. `When()` keeps the Failures alone.
- `AllNodesReady`, `AllPodsReady` and `PlacementSettled` each have a `…WithinSeconds(n)` form. Beneath each the lab observes the Placement stop changing, so a Then judges a Settled Placement. A Pending count is a Then, `PodsPending`.
- The first Wait after a Given applies it, and `ApplyTopology` stays create-only. A refusal names that Wait. A Failure When applies nothing, reading the last Wait's Placement. A Then or Failure with no Wait since the last change ends the Scenario.
- The lab's waiting time lives in `internal/scenario`. A stated time of zero or above it is refused at the Wait. Every Wait waits it out, and a stated time reached later ends the Scenario at that Wait.
- A Wait's line names its time: `reached in 1.4s`, `reached in 6.5s of the 30s stated`, or `not reached in the 30s stated`. The Step carries the measured time, resolved time, stated marker and whether it was reached.
- A later Deployment Given deploys at the next Wait beside the earlier ones. Later Waits, Whens and Thens judge the newest, `EnoughPodsStayedReady` aside. A repeated name is refused.
- Every Wait, reached or not, observes the whole cluster once for its Placement board: every Deployment's Pods on every Topology Node, no probe Pod. The board judges nothing.
- The lab's Scenarios write each Wait as a Step of its own, enforced by the Scenario-place rule test; the chained form stays offered. A Scenario's doc comment states its intention in a sentence or two, and the place carries no package comment.

## Consequences

Nothing waits silently, and a restore time is on a line of its own. A Failure Scenario states thirty seconds, a third of the lab's waiting time, so a loaded machine reports a restore rather than a Finding. A miss is known only once the Wait ends, so a short stated time still waits the full waiting time out. The judgement is a pure function, tested without a cluster.

Three Waits make six calls, and a further Wait weighs the argument form again. A Wait costs one more observation for its board, which may read the newest Deployment a poll apart from the Thens. `revive`'s `exported` and `package-comments` rules are off, and review holds the doc comment.

## Implementation

**Requirements:** REQ-SIM-006, REQ-SIM-008, REQ-SIM-011, REQ-SIM-019, REQ-SIM-021

## References

- [system-design.md § Contracts](../system-design.md#contracts)
- [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md)
- [ADR: A Deployment declares the Zones or Nodes it survives losing, judged over the whole Timeline](2026-10-06-a-deployment-declares-the-zones-or-nodes-it-survives-losing.md)
