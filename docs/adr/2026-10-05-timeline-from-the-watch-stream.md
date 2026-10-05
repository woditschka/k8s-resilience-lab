# A Timeline is recorded from the API's watch stream and filed as JSON Lines

**Status:** Accepted

## Context

A reported Placement says where the Pods landed, not how they got there (G-5). Explaining a Placement needs every Node, Pod and Workload change a Scenario's Simulated Cluster went through, and the control plane's reason for each Placement (REQ-SIM-005).

The record has to outlive the cluster it describes, because that cluster is removed when its Scenario ends. Two later slices read the same record: the live view (G-2) and a query path over a filed Timeline.

A Scenario runs for seconds and its cluster holds tens of objects, so the record's cost is not the constraint. Completeness is: a change the lab never saw cannot be explained afterwards.

## Options Considered

1. **Poll and diff** — one List per kind per poll interval, compared against the previous List. Needs no new call shape, and misses every transition between two polls.
2. **Shared informers** (`k8s.io/client-go/tools/cache`) — a cache, a resync and handler registration. Its delta queue coalesces repeated updates to one object, so the record loses changes.
3. **Watch streams** — one `Watch` call per kind, each pushing an add, a modify or a delete as the API server commits it.
4. **Kubernetes Events alone** — the scheduler's own messages, with no object changes and a server-side retention of its own.

## Decision

Option 3, over five streams: Nodes, Pods, Deployments, ReplicaSets and Events in the Workload's namespace. One goroutine owns the record and selects over the five streams, so the Timeline needs no lock and its order is the order the lab received the changes. A stream the API server closes is re-opened once from the last version seen; a re-open that fails is a noted entry, so a Timeline is never silently incomplete.

A Kubernetes Event is one entry among the changes, which is why the record is called a Timeline rather than an event log.

Each entry is mapped into an immutable `internal/scenario` value by the same anti-corruption boundary every other Kubernetes read crosses, so no Kubernetes type reaches the printed or filed form.

A Timeline is filed as JSON Lines: one header line naming the Scenario and its cluster, then one line per entry, written as the entry arrives. The lines are a published format, so the later query path and the live view read a filed Timeline without the lab parsing its own prose. Writing as entries arrive is also what leaves a usable file behind when a Scenario fails part-way.

The file is named for the Simulated Cluster the Timeline describes, in a `timelines` folder under the working directory. That name already carries the Scenario it serves and a suffix unique to the invocation, per [ADR: Each Scenario provisions its own Simulated Cluster](2026-10-04-cluster-per-scenario.md), so two runs at once need no further naming rule. The lab removes no Timeline file.

A Timeline the lab cannot write is the lab's own failure, not the Scenario's: the file keeps its first write error, the Scenario's reported outcome stands, and the run names the Timeline it could not write.

## Consequences

Every change the API server pushed before the Scenario ended is in the Timeline. Stopping the record cancels the five watches and drains each stream to its close, so a test reads a complete Timeline without a wait of its own.

The Even spread Scenario records around 150 entries. A run prints a summary of them and the whole record on request, per [ADR: A run prints a Timeline summary derived from the record](2026-10-05-timeline-summary-derived-from-the-record.md).

Timeline files accumulate until an engineer deletes them, against the lab's preference for a run that leaves nothing behind. Retention is an open question in the PRD.

Cross-stream order is receipt order, not causal order: two changes committed in the same millisecond on different streams can be recorded either way round.

`internal/timeline` owns the folder, the name and the line format, and is unit-testable without a cluster.

The stream connection is the one place the lab's tests stand in for a system boundary. A closed stream, a failed re-open and a failed `Watch` call are the API server's behaviour, which a real KWOK control plane does not produce on demand. A hand-written `watch.Interface` in `internal/cluster`'s own test drives those paths, while the entries a Timeline holds stay asserted against a real Simulated Cluster (`docs/testing-principles.md` § Mocking Policy).

## Implementation

**Requirements:** REQ-SIM-005

## References

- [system-design.md § Contracts](../system-design.md)
- [ADR: Each Scenario provisions its own Simulated Cluster](2026-10-04-cluster-per-scenario.md)
- [ADR: The lab is a Go program on a KWOK Simulated Cluster](2026-10-03-go-program-on-a-kwok-simulated-cluster.md)
- [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md)
