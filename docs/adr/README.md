<!-- harness: 2026-09-30 -->
# Architecture Decision Records

This directory is the project's decision log. ADRs document the path to each decision — the options considered, trade-offs evaluated, and rationale for the choice. The current state of all accepted decisions is reflected in [`system-design.md`](../system-design.md), the authoritative reference for implementation.

## Format

Each ADR is a markdown file named `YYYY-MM-DD-title-in-kebab-case.md`.

### Template

```markdown
# [Title]

**Status:** Proposed | Accepted | Deprecated | Superseded by [ADR-YYYY-MM-DD]

## Context

[Why is this decision needed? What problem are we solving?]

## Options Considered

1. **Option A** - [Brief description]
2. **Option B** - [Brief description]

## Decision

[What did we choose and why?]

## Consequences

[What are the results? Both positive and negative.]

## Implementation

**Requirements:** REQ-XX-NNN

## References

- [Link to the system-design.md section realizing this decision](../system-design.md)
```

### Guidelines

- One decision per file
- Document the path: what options existed, why we chose this one
- Keep it concise: aim for under 60 lines
- Write in present tense ("We use X" not "We will use X")
- Link related ADRs when decisions interact
- Update status when decisions change; supersede, don't delete

### Non-Goal ADRs

A non-goal ADR records a *product* decision not to build something — distinct from an architectural ADR. Two conventions apply:

- **Filename:** `YYYY-MM-DD-non-goal-<slug>.md` — the `non-goal-` infix is load-bearing: the product-requirements-expert agent's write scope matches this pattern; standard ADRs remain owned by the system-design-expert.
- **Implementation section:** use `**Non-goal:** NG-X` (referencing the PRD's Non-Goals table) instead of `**Requirements:**`.

## Index

| Date | Decision | Status |
|------|----------|--------|
| 2026-10-03 | [Non-goal — real-cluster runs](2026-10-03-non-goal-real-cluster-runs.md) | Accepted |
| 2026-10-03 | [Non-goal — application-level recovery](2026-10-03-non-goal-application-level-recovery.md) | Accepted |
| 2026-10-03 | [The lab is a Go program on a KWOK Simulated Cluster](2026-10-03-go-program-on-a-kwok-simulated-cluster.md) | Accepted, amended 2026-10-04 |
| 2026-10-03 | [Kubernetes version pin for the Simulated Cluster](2026-10-03-kubernetes-version-pin.md) | Accepted |
| 2026-10-03 | [The lab provisions the Simulated Cluster it drives](2026-10-03-lab-provisioned-simulated-cluster.md) | Accepted, amended 2026-10-04, 2026-10-09 and 2026-10-21 |
| 2026-10-03 | [Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md) | Accepted, amended 2026-10-07, 2026-10-09 and 2026-10-19 |
| 2026-10-04 | [Provisioning ends when the scheduler has placed a Pod on every Node](2026-10-04-scheduler-observable-provisioning.md) | Accepted |
| 2026-10-04 | [The gate runs the tests under the race detector](2026-10-04-race-detector-in-the-gate.md) | Accepted |
| 2026-10-05 | [A Timeline is recorded from the API's watch stream and filed as JSON Lines](2026-10-05-timeline-from-the-watch-stream.md) | Accepted, amended 2026-10-05 and 2026-10-20 |
| 2026-10-05 | [A Node Failure withholds the Node's status and shortens the eviction wait](2026-10-05-node-failure-withholds-the-nodes-status.md) | Accepted |
| 2026-10-05 | [A Workload declares one of the lab's predefined Pod specifications](2026-10-05-workload-declares-a-predefined-pod-specification.md) | Accepted, amended 2026-10-18 (three times) |
| 2026-10-06 | [A Workload declares its Required Capacity, judged at the moment of the Failure and once Settled](2026-10-06-required-capacity-judged-at-two-moments.md) | Accepted |
| 2026-10-07 | [A Scenario writes its own Wait, with its Given or as a Step of its own](2026-10-07-a-scenario-writes-its-own-wait.md) | Accepted, amended 2026-10-07 and 2026-10-08 |
| 2026-10-08 | [The lab owns the simulator's Stages to give a Pod its Startup time](2026-10-08-the-lab-owns-the-simulators-stages-for-a-startup-time.md) | Accepted, amended 2026-10-09 and 2026-10-18 |
| 2026-10-08 | [A Topology declares the NodePools every Zone holds](2026-10-08-a-topology-declares-the-nodepools-every-zone-holds.md) | Accepted, amended 2026-10-09 (twice) and 2026-10-18 (twice) |
| 2026-10-18 | [Scenario code stays idiomatic Go, read by subject](2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md) | Accepted, amended 2026-10-18 and 2026-10-19 |
| 2026-10-19 | [A NodePool states NoSchedule taints, and a later Workload deploys beside the earlier, judged alone](2026-10-19-a-nodepool-states-noschedule-taints-and-a-later-workload-deploys-beside-the-earlier.md) | Accepted, amended 2026-10-20 |
| 2026-10-21 | [The Live view is served on loopback from one shared observation](2026-10-21-the-live-view-is-served-on-loopback-from-one-shared-observation.md) | Accepted |
