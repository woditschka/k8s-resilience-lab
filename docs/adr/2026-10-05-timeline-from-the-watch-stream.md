# A Timeline is recorded from the API's watch stream and filed as JSON Lines

**Status:** Accepted

## Context

A reported Placement says where the Pods landed, not how they got there (G-5). Explaining it needs every Node, Pod and Deployment change a Scenario's cluster went through, with the control plane's reason (REQ-SIM-005). The record must outlive a cluster removed when its Scenario ends, and the Live view and a later query path read it. Completeness is the constraint: a change the lab never saw cannot be explained afterwards.

A Scenario records about 150 entries, most of them its own setup. The owner asked to see "what pod goes live in which node, zone, where do they land at the end". The owner decided: "Under every Wait, the report shows the moments that changed the Placement since the previous Wait, in order", and "The Steps are noted into the Timeline at their moment" (REQ-SIM-021).

The control-plane client carries a 30-second request deadline, which would close every watch stream mid-Scenario.

## Options Considered

How the lab sees changes:

1. **Poll and diff** — misses every transition between two polls.
2. **Shared informers** — the delta queue coalesces repeated updates, so the record loses changes.
3. **Watch streams** — one `Watch` per kind, pushing each change as the API server commits it.
4. **Kubernetes Events alone** — no object changes, and a retention of the server's own.

What a run prints:

5. **Every entry** — the setup buries the Placement a person came to read.
6. **Fewer kinds** — still a flat list.
7. **A summary derived from the entries** — counts with no order of events.
8. **A Wait narrative derived from the entries**, cut at each Wait.
9. **The difference between two successive boards** — misses a Pod evicted and replaced between them.

How a narrative knows where each Wait stands:

10. **Each Step noted into the Timeline as an entry**, timed by the owning goroutine.
11. **Each Step's own clock against the entries' times** — two clocks, and a filed Timeline holds no Step.

How a Pod is attributed to its Deployment:

12. **The record's own naming**: a Pod belongs to the Deployment whose name begins its own.
13. **A Deployment name on every Pod entry** — records a reading rather than what the control plane said.

How an eviction is read:

14. **The entry marking a Pod for deletion**, read as Terminating.
15. **The control plane's eviction Event** — wording that belongs to one Kubernetes version.
16. **The Pod's removal** — never arrives for a Pod on a stopped Node.

How a stream outlives the request deadline:

17. **A watch client of its own, with no request deadline**, each open still bounded.
18. **Re-open at every close** — cuts every stream each 30 seconds, and a close stops signalling a failure.
19. **The watch's `TimeoutSeconds`** — still cut by the client deadline below it.
20. **A deadline per request** — one forgotten call wedges a Scenario.

## Decision

Options 3, 8, 10, 12, 14 and 17.

- Five streams: Nodes, Pods, Deployments, ReplicaSets, and Events in the Deployment's namespace. One goroutine owns the record and selects over them, so receipt order is the Timeline's order and no lock is needed.
- A closed stream is re-opened once from the last version seen; a failed re-open is a noted entry. Each entry is mapped into an immutable `internal/scenario` value.
- A Timeline is filed as JSON Lines in a `timelines` folder: a header line, then one line per entry as it arrives. The file is named for its Simulated Cluster, and the lab removes none. A write failure is the lab's own, named by the run.
- A Pod belongs to the Deployment whose name and a hyphen begin its own, at the longest match.
- The Scenario notes each Step through the owning goroutine: a Given or Then once complete, a When before its first Node stops, a Wait as it ends.
- A Wait narrative is a pure function in `internal/scenario` of the entries after the previous Wait's note up to its own. A provisioning Pod is in none. A Pod's first entry marked for deletion reads as its eviction.
- `cmd/lab` prints each narrative above its Wait's board. `--whole-timeline` adds every entry.
- Every control-plane request, a watch's opening included, keeps the 30-second deadline. A stream runs on a second client with the same address, no credential and no deadline, ending with its recording.

## Consequences

Every change the API server pushed before the Scenario ended is in the Timeline, and stopping the record drains each stream. Cross-stream order is receipt order, not causal order. A filed Timeline narrates as the run that recorded it did. A Wait settles on two observations a poll interval apart, far above a stream's delivery on loopback, so its narrative holds the changes it reflects.

Timeline files accumulate until an engineer deletes them; retention is an open question in the PRD. A hand-written `watch.Interface` in `internal/cluster`'s test drives the closed-stream and failed-open paths, a real control plane producing none on demand.

## Implementation

**Requirements:** REQ-SIM-005, REQ-SIM-021

## References

- [system-design.md § Contracts](../system-design.md#contracts)
- [ADR: The lab provisions the Simulated Cluster it drives](2026-10-03-lab-provisioned-simulated-cluster.md)
