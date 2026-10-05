# A run prints a Timeline summary derived from the record

**Status:** Accepted

## Context

The Even spread Scenario records around 150 entries, and about 60% of them are the lab's own setup: its Nodes registering and the probe Pods provisioning asks the scheduler with. Printing every entry buries the Placement a person came to read (REQ-SIM-005, G-5).

The recording and the Timeline file stay as they are. Two later slices read the same record: the live view (G-2) and a query path over a filed Timeline, per [ADR: A Timeline is recorded from the API's watch stream and filed as JSON Lines](2026-10-05-timeline-from-the-watch-stream.md).

A Pod entry carries the Pod's name, its phase and the Node it is bound to. It does not say which Workload the Pod belongs to, and the lab's probe Pods are Pods like any other.

## Options Considered

1. **Print fewer kinds** — drop the Node and probe entries from the print. Cheapest, and it still prints a flat list with no counts and no per-Pod path.
2. **Record a summary as the watch runs** — the recording goroutine maintains a second structure beside the Timeline. Couples the record to one report, and a Timeline filed by an earlier run still has no summary.
3. **Derive the summary from the recorded entries** — one pass over the Timeline, printed in place of the entries, with every entry printed on request.
4. **Filter or page the print from the terminal** — the PRD declines it for this slice.

## Decision

Option 3. `Timeline.Summarise` in `internal/scenario` is a pure function of the entries the Timeline holds, so any reader of a filed Timeline derives the same summary without a Kubernetes import. `cmd/lab` prints it and decides nothing about its content.

The summary reads the record's own object graph. A Deployment entry names a Workload. A Pod entry belongs to the Workload whose name its own begins with, followed by a hyphen, taking the longest match where two Workload names nest. Every other object, Nodes included, is the lab's own setup and collapses to one Topology line.

The alternative is a Workload name on every Pod entry. It is not taken: the recording and the filed line do not change in this slice, and the attribution is a reading of the record rather than part of what the control plane said.

A run prints the whole Timeline when `--whole-timeline` is given, parsed in the one flag parse that already resolves `--k8s-version`. The flag is named for what it prints. The owner's `-v` example is not taken, because the lab has one record to print and a verbosity level would not say which.

The readiness words a Node entry carries move from `internal/cluster` into `internal/scenario` as one named type, the way a Pod's phase already does. The recorded and filed values are unchanged.

## Consequences

A Scenario's output is one screen: the entry count and the file, the Topology, each Workload with its count, one line per Workload Pod, and the Pods left Pending. This replaces the earlier ADR's consequence that a printed Timeline dominates a run's output.

The whole record stays one flag away, and in the file either way. A Scenario that failed part-way summarises to what its Timeline holds.

The attribution rule rests on the control plane's naming of a Deployment's Pods. A Pod it cannot attribute reads as the lab's own setup rather than as another Workload's Pod, and the entry count and the file are what say a line is missing.

Workloads and Pods are listed in the order the Timeline first names them, so a summary needs no sort and two readings of one Timeline agree.

A Timeline entry now carries two vocabularies that `internal/scenario` owns, a Pod's phase and a Node's readiness, and no literal of either is written twice.

## Implementation

**Requirements:** REQ-SIM-005

## References

- [system-design.md § Contracts](../system-design.md)
- [ADR: A Timeline is recorded from the API's watch stream and filed as JSON Lines](2026-10-05-timeline-from-the-watch-stream.md)
- [ADR: Provisioning ends when the scheduler has placed a Pod on every Node](2026-10-04-scheduler-observable-provisioning.md)
