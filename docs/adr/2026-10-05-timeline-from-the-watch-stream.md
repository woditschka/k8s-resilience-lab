# A Timeline is recorded from the API's watch stream and filed as JSON Lines

**Status:** Accepted, amended 2026-10-05: a run prints a Timeline summary derived from the record and the whole Timeline on request, options 7 and 9 (REQ-SIM-005)

## Context

A reported Placement says where the Pods landed, not how they got there (G-5). Explaining a Placement needs every Node, Pod and Workload change a Scenario's Simulated Cluster went through, and the control plane's reason for each Placement (REQ-SIM-005).

The record has to outlive the cluster it describes, because that cluster is removed when its Scenario ends. Two later slices read the same record: the live view (G-2) and a query path over a filed Timeline.

A Scenario runs for seconds and its cluster holds tens of objects, so the record's cost is not the constraint. Completeness is: a change the lab never saw cannot be explained afterwards.

A run first printed every entry when a Scenario ended. The Resilient placement Scenario records around 150 entries, about 60% of them the lab's own setup: its Nodes registering and the probe Pods provisioning asks the scheduler with. Later on 2026-10-05 the print became a summary, because the flat list buried the Placement a person came to read; the recording and the file stayed as they were. A Pod entry names its phase and its Node, not its Workload, and a probe Pod is a Pod like any other.

## Options Considered

1. **Poll and diff** — one List per kind per poll interval, compared against the previous List. Needs no new call shape, and misses every transition between two polls.
2. **Shared informers** (`k8s.io/client-go/tools/cache`) — a cache, a resync and handler registration. Its delta queue coalesces repeated updates to one object, so the record loses changes.
3. **Watch streams** — one `Watch` call per kind, each pushing an add, a modify or a delete as the API server commits it.
4. **Kubernetes Events alone** — the scheduler's own messages, with no object changes and a server-side retention of its own.

How a run prints the Timeline, with option 3 in force:

5. **Print fewer kinds** — drop the Node and probe entries. Still a flat list, with no counts and no per-Pod path.
6. **Record a summary as the watch runs** — a second structure beside the Timeline. Couples the record to one report, and an earlier filed Timeline has none.
7. **Derive the summary from the recorded entries** — one pass over the Timeline, printed in place of the entries.
8. **Filter or page the print from the terminal** — the PRD declines it.

How a summary attributes a Pod to its Workload, with option 7 in force:

9. **Read the record's own object graph** — a Pod belongs to the Workload whose name begins its own.
10. **A Workload name on every Pod entry** — changes the recording and the filed line, and records a reading rather than what the control plane said.

## Decision

Option 3, over five streams: Nodes, Pods, Deployments, ReplicaSets and Events in the Workload's namespace. One goroutine owns the record and selects over the five streams, so the Timeline needs no lock and its order is the order the lab received the changes. A stream the API server closes is re-opened once from the last version seen; a re-open that fails is a noted entry, so a Timeline is never silently incomplete. A Kubernetes Event is one entry among the changes, which is why the record is called a Timeline rather than an event log. Each entry is mapped into an immutable `internal/scenario` value by the same anti-corruption boundary every other Kubernetes read crosses, so no Kubernetes type reaches the printed or filed form.

A Timeline is filed as JSON Lines: one header line naming the Scenario and its cluster, then one line per entry, written as the entry arrives. The lines are a published format, so the later query path and the live view read a filed Timeline without the lab parsing its own prose. Writing as entries arrive leaves a usable file behind when a Scenario fails part-way. The file is named for the Simulated Cluster it describes, in a `timelines` folder under the working directory. That name carries the Scenario and a random suffix per provisioning, per [ADR: The lab provisions the Simulated Cluster it drives](2026-10-03-lab-provisioned-simulated-cluster.md), so concurrent runs need no further rule. The lab removes no Timeline file. A Timeline the lab cannot write is the lab's own failure: the file keeps its first write error, the Scenario's outcome stands, and the run names the Timeline it could not write.

Options 7 and 9, since 2026-10-05. `Timeline.Summarise` in `internal/scenario` is a pure function of the entries, so any reader of a filed Timeline derives the same summary without a Kubernetes import. `cmd/lab` prints it and decides nothing about its content. A Deployment entry names a Workload. A Pod entry belongs to the Workload whose name, followed by a hyphen, begins its own, at the longest match where two names nest. Every other object, Nodes included, collapses to one Topology line. `--whole-timeline`, parsed in the one flag parse that resolves `--k8s-version`, adds every entry. The owner's `-v` example is not taken, because the lab has one record to print and a verbosity level would not say which. A Node's readiness words are one named type in `internal/scenario`, the way a Pod's phase is; the recorded and filed values are unchanged.

## Consequences

Every change the API server pushed before the Scenario ended is in the Timeline. Stopping the record cancels the five watches and drains each stream to its close, so a test reads a complete Timeline without a wait of its own. Cross-stream order is receipt order, not causal order: two changes committed in the same millisecond on different streams can be recorded either way round.

A Scenario's output is one screen: the entry count and the file, the Topology, each Workload with its count, one line per Workload Pod, and the Pods left Pending. The whole record stays one flag away, and in the file either way; a Scenario that failed part-way summarises to what its Timeline holds. Workloads and Pods are listed in the order the Timeline first names them, so two readings of one Timeline agree without a sort. The attribution rests on the control plane's naming of a Deployment's Pods: a Pod it cannot attribute reads as setup, and the entry count and the file say a line is missing.

Timeline files accumulate until an engineer deletes them, against the lab's preference for a run that leaves nothing behind. Retention is an open question in the PRD.

`internal/timeline` owns the folder, the name and the line format, and is unit-testable without a cluster. The stream connection is the one place the lab's tests stand in for a system boundary. A closed stream, a failed re-open and a failed `Watch` call are behaviours a real KWOK control plane does not produce on demand. A hand-written `watch.Interface` in `internal/cluster`'s own test drives those paths, while a Timeline's entries stay asserted against a real Simulated Cluster (`docs/testing-principles.md` § Mocking Policy).

## Implementation

**Requirements:** REQ-SIM-005

## References

- [system-design.md § Contracts](../system-design.md)
- [ADR: The lab provisions the Simulated Cluster it drives](2026-10-03-lab-provisioned-simulated-cluster.md)
- [ADR: The lab is a Go program on a KWOK Simulated Cluster](2026-10-03-go-program-on-a-kwok-simulated-cluster.md)
- [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md)
- [ADR: Provisioning ends when the scheduler has placed a Pod on every Node](2026-10-04-scheduler-observable-provisioning.md)
