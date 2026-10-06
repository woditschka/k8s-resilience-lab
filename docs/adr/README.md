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
| 2026-10-03 | [The lab is a Go program on a KWOK Simulated Cluster](2026-10-03-go-program-on-a-kwok-simulated-cluster.md) | Accepted |
| 2026-10-03 | [Kubernetes version pin for the Simulated Cluster](2026-10-03-kubernetes-version-pin.md) | Accepted |
| 2026-10-03 | [The lab provisions the Simulated Cluster it drives](2026-10-03-lab-provisioned-simulated-cluster.md) | Accepted |
| 2026-10-03 | [Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md) | Accepted |
| 2026-10-04 | [A Scenario's Workload declares the spread of its own Pods](2026-10-04-scenario-declared-pod-spread.md) | Accepted |
| 2026-10-04 | [Each Scenario provisions its own Simulated Cluster](2026-10-04-cluster-per-scenario.md) | Accepted |
| 2026-10-04 | [Provisioning ends when the scheduler has placed a Pod on every Node](2026-10-04-scheduler-observable-provisioning.md) | Accepted |
| 2026-10-04 | [The gate runs the tests under the race detector](2026-10-04-race-detector-in-the-gate.md) | Accepted |
| 2026-10-04 | [The module-rule tests share one test-support package](2026-10-04-module-rule-test-support.md) | Accepted |
| 2026-10-05 | [A Timeline is recorded from the API's watch stream and filed as JSON Lines](2026-10-05-timeline-from-the-watch-stream.md) | Accepted |
| 2026-10-05 | [A run prints a Timeline summary derived from the record](2026-10-05-timeline-summary-derived-from-the-record.md) | Accepted |
| 2026-10-05 | [A Node Failure withholds the Node's status and shortens the eviction wait](2026-10-05-node-failure-withholds-the-nodes-status.md) | Accepted |
| 2026-10-05 | [A Workload declares one of the lab's predefined Pod specifications](2026-10-05-workload-declares-a-predefined-pod-specification.md) | Accepted |
| 2026-10-06 | [A Workload declares its Required Capacity, judged at the moment of the Failure and once Settled](2026-10-06-required-capacity-judged-at-two-moments.md) | Accepted |
