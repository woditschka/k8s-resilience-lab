# A Timeline is recorded from the API's watch stream and filed as JSON Lines

**Status:** Accepted, amended 2026-10-05: a summary derived from the record and the whole Timeline on request, options 7 and 9 (REQ-SIM-005). Amended 2026-10-20: the record notes every Step, and a Wait narrative derived from it replaces the summary, options 11, 13 and 15 (REQ-SIM-021). Amended again 2026-10-20: a watch stream escapes the request deadline, option 18 (REQ-SIM-021)

## Context

A reported Placement says where the Pods landed, not how they got there (G-5). Explaining a Placement needs every Node, Pod and Workload change a Scenario's Simulated Cluster went through, and the control plane's reason for each Placement (REQ-SIM-005).

The record has to outlive the cluster it describes, because that cluster is removed when its Scenario ends. Two later slices read the same record: the live view (G-2) and a query path over a filed Timeline.

A Scenario runs for seconds and its cluster holds tens of objects, so the record's cost is not the constraint. Completeness is: a change the lab never saw cannot be explained afterwards.

A run first printed every entry when a Scenario ended. The Resilient placement Scenario records around 150 entries, about 60% of them the lab's own setup: its Nodes registering and the probe Pods provisioning asks the scheduler with. Later on 2026-10-05 the print became a summary, because the flat list buried the Placement a person came to read; the recording and the file stayed as they were. A Pod entry names its phase and its Node, not its Workload, and a probe Pod is a Pod like any other.

On 2026-10-20 the owner asked to make the Timeline easier to read: "what pod goes live in which node, zone, where do they land at the end". A Placement board under every Wait now shows where every Pod stands. The owner then asked "what would you propose for the timeline to reach the status?" The decision reads: "Under every Wait, the report shows the moments that changed the Placement since the previous Wait, in order". The same decision says "The Steps are noted into the Timeline at their moment" and "The old Timeline summary ... go[es]". A narrative must say which Wait each change came before, and an entry names no Step. A Pod a Failure evicts stays marked for deletion on a Node no simulator manages, so its removal never arrives.

Later on 2026-10-20 the narrative slice found every recording ending about 60 seconds in. The five streams shared the one control-plane client, whose 30-second request deadline closed each stream and its one re-open. An unreached Wait lasts the lab's 90-second waiting time, so its narrative was empty, and any longer Scenario filed a truncated Timeline.

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

How a run tells what changed the Placement before each Wait, since 2026-10-20:

11. **A Wait narrative derived from the recorded entries** — one pass over the Timeline, cut at each Wait, in place of the summary.
12. **The difference between two successive boards** — misses a Pod evicted and replaced between them, and holds no reason and no time.
13. **Note each Step into the Timeline as an entry** — the owning goroutine times it like a change, so a Wait's moment stands among the changes in receipt order.
14. **Compare each Step's own clock reading against the entries' elapsed times** — two clocks started apart, and a filed Timeline would hold no Step.

How a narrative reads an eviction, with option 11 in force:

15. **The entry marking a Pod for deletion** — the mapper reads a Pod carrying a deletion time as Terminating, the word `kubectl` shows.
16. **The control plane's eviction report** — an Event whose reason and wording belong to one Kubernetes version.
17. **The Pod's removal** — never arrives for a Pod on a stopped Node.

How a watch stream outlives the control-plane request deadline, since 2026-10-20:

18. **A watch client of its own, with no request deadline** — the same address and no credential. Each open is bounded by the deadline, each stream by its recording.
19. **Re-open a stream at every close** — cuts every stream mid-read each 30 seconds, and a close stops signalling a failure.
20. **The watch's own `TimeoutSeconds`** — a server-side close, still cut by the client deadline standing below it.
21. **A deadline per request in place of one on the client** — every call site carries its own, and one forgotten call wedges a Scenario again.

## Decision

Option 3, over five streams: Nodes, Pods, Deployments, ReplicaSets and Events in the Workload's namespace. One goroutine owns the record and selects over the five streams, so the Timeline needs no lock and its order is the order the lab received the changes. A stream the API server closes is re-opened once from the last version seen; a re-open that fails is a noted entry, so a Timeline is never silently incomplete. A Kubernetes Event is one entry among the changes, which is why the record is called a Timeline rather than an event log. Each entry is mapped into an immutable `internal/scenario` value by the same anti-corruption boundary every other Kubernetes read crosses, so no Kubernetes type reaches the printed or filed form.

A Timeline is filed as JSON Lines: one header line naming the Scenario and its cluster, then one line per entry, written as the entry arrives. The lines are a published format, so the later query path and the live view read a filed Timeline without the lab parsing its own prose. Writing as entries arrive leaves a usable file behind when a Scenario fails part-way. The file is named for the Simulated Cluster it describes, in a `timelines` folder under the working directory. That name carries the Scenario and a random suffix per provisioning, per [ADR: The lab provisions the Simulated Cluster it drives](2026-10-03-lab-provisioned-simulated-cluster.md), so concurrent runs need no further rule. The lab removes no Timeline file. A Timeline the lab cannot write is the lab's own failure: the file keeps its first write error, the Scenario's outcome stands, and the run names the Timeline it could not write.

Option 9, since 2026-10-05. A Deployment entry names a Workload. A Pod entry belongs to the Workload whose name, followed by a hyphen, begins its own, at the longest match where two names nest. Option 7's summary printed from 2026-10-05 until option 11 replaced it on 2026-10-20. `--whole-timeline`, parsed in the one flag parse that resolves `--k8s-version`, adds every entry. The owner's `-v` example is not taken, because the lab has one record to print and a verbosity level would not say which. A Node's readiness words are one named type in `internal/scenario`, the way a Pod's phase is; the recorded and filed values are unchanged.

Options 11, 13 and 15, since 2026-10-20, retire the summary and keep option 9's attribution. The Scenario notes every Step it records into its Timeline, carrying the Step's printed line, through the goroutine owning the record. A Given or a Then is noted once its line is complete, a When before it stops its first Node, and a Wait as it ends, after its board. A narrative is a pure function of the entries in `internal/scenario`, so a filed Timeline narrates as the run that recorded it did. A Wait's narrative holds the moments after the previous Wait's note and up to its own. A Pod the lab places while provisioning belongs to no Workload and is in no narrative. The lab deletes no Workload Pod, so a Pod's first entry marked for deletion reads as its eviction. The narrative states the order of an eviction and a replacement and never links them. `cmd/lab` prints each narrative above its Wait's board and decides nothing about its content. The Timeline's filing line, its entry count and file, stays, and `--whole-timeline` adds every entry beneath it.

Option 18, since 2026-10-20. Every control-plane request keeps the 30-second deadline, a watch's opening included. A watch's stream then runs on a second client of the same library, built from the same address with no credential and no deadline. The stream ends when its recording's context ends, which stopping the record and closing the cluster cancel, so no stream holds up the cluster's removal. A stream the API server closes is still re-opened once.

## Consequences

Every change the API server pushed before the Scenario ended is in the Timeline. Stopping the record cancels the five watches and drains each stream to its close, so a test reads a complete Timeline without a wait of its own. Cross-stream order is receipt order, not causal order: two changes committed in the same millisecond on different streams can be recorded either way round.

Since 2026-10-20 each Wait reads as the changes that led to its board, and the whole record stays one flag away, and in the file either way. A Wait that ends the Scenario narrates up to the moment it ended. A note crosses the same receipt order as the changes, so a change still in flight on its stream as a Wait ends reads under the next Wait. A Wait settles on two agreeing observations a poll interval apart, far above a stream's delivery on the loopback. The attribution rests on the control plane's naming of a Deployment's Pods: a Pod it cannot attribute reads as setup and is in no narrative. A Step entry and the Terminating reading are new values in the filed lines, whose fields are unchanged.

Since option 18 a Timeline lasts as long as its Scenario. A stream that stops delivering without closing meets no deadline of its own: the Scenario's requests still meet theirs, and stopping the record ends it.

Timeline files accumulate until an engineer deletes them, against the lab's preference for a run that leaves nothing behind. Retention is an open question in the PRD.

`internal/timeline` owns the folder, the name and the line format, and is unit-testable without a cluster. The stream connection is the one place the lab's tests stand in for a system boundary. A closed stream, a failed re-open and a failed `Watch` call are behaviours a real KWOK control plane does not produce on demand. A hand-written `watch.Interface` in `internal/cluster`'s own test drives those paths, while a Timeline's entries stay asserted against a real Simulated Cluster (`docs/testing-principles.md` § Mocking Policy).

## Implementation

**Requirements:** REQ-SIM-005, REQ-SIM-021

## References

- [system-design.md § Contracts](../system-design.md)
- [ADR: The lab provisions the Simulated Cluster it drives](2026-10-03-lab-provisioned-simulated-cluster.md)
- [ADR: The lab is a Go program on a KWOK Simulated Cluster](2026-10-03-go-program-on-a-kwok-simulated-cluster.md)
- [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md)
- [ADR: Provisioning ends when the scheduler has placed a Pod on every Node](2026-10-04-scheduler-observable-provisioning.md)
