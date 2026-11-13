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

How far an Interactive run's record reaches (REQ-SIM-033). The owner asked whether the Descheduler's actions while held are "part of the recorded timeline", and chose "One Timeline, all of it":

21. **The record stops with the Steps**, the terminal reading a held cluster from the Live view's boards — a second account of the same changes, none of it filed.
22. **The record runs until the lab stops**, each click noted as a When — one account, filed whole, from which the terminal can read.

How a held run's terminal reads that Timeline as it grows:

23. **Asking for the Timeline so far on an interval and narrating it again** — a delay up to the interval, and a diff to find the new lines.
24. **The owning goroutine handing each entry on, in Timeline order, to one reader** — each line as it is recorded, and a reader that stalls holds up the record.

What a held run's terminal names. The owner asked for "all changes", then accepted more lines as long as they are not noisy:

25. **Every Entry, as `--whole-timeline` prints them** — complete, and noisy with changes that move no Pod.
26. **The Descheduler's evictions and their replacements alone** — quiet, and silent on what a click causes.
27. **Every moment a Wait narrative names, each line printed once its moment is complete** — a click's effects and a pass's alike, worded as a run's narrative.

What the stop does with a terminal that stopped taking lines, the owner asking for a bounded wait. Any unbounded wait leaves the cluster running, and no option drops the line of an entry handed before the stop (REQ-SIM-040):

28. **Holding the lines not yet printed** — printed once the terminal resumes, the lab ending only then.
29. **Dropping them after a bounded wait** — the stop waits for the lines of each entry handed before it, then drops the rest, and the filed Timeline holds their entries.

How a clicked Failure that cannot be injected, wholly or in part, stands in the Timeline (REQ-SIM-028 edge case 5). The PRD names it not injected, and why, in place of its When:

30. **The When noted once the Failure is injected** — the click's own changes may stand before it, read as moments ahead of the click.
31. **The When noted before, a not-injected note after it** — the Timeline keeps a When for a Failure never injected.
32. **The note held open from before the first change until the injection returns** — the owning goroutine holds later entries back, then records the When or the not-injected note at the moment held, ahead of them.

How fast a Failure's Node writes reach the control plane (REQ-SIM-008 edge case 3). client-go's default client-side limit of 5 requests a second spaces a Zone's Nodes about 0.8 seconds apart:

33. **A higher client-side limit** — a number still scaling with the Zone, against a bound no lab user set.
34. **No client-side limit** — the API server is the lab's own, with one client, and its own limits stand.

## Decision

Options 3, 8, 10, 12, 14, 17, 22, 24, 27, 29, 32 and 34.

- Five streams: Nodes, Pods, Deployments, ReplicaSets, and Events in the Deployment's namespace. One goroutine owns the record and selects over them, so receipt order is the Timeline's order and no lock is needed.
- A closed stream is re-opened once from the last version seen; a failed re-open is a noted entry. Each entry is mapped into an immutable `internal/scenario` value.
- A Timeline is filed as JSON Lines in a `timelines` folder: a header line, then one line per entry as it arrives. The file is named for its Simulated Cluster, and the lab removes none. A write failure is the lab's own, named by the run.
- A Pod belongs to the Deployment whose name and a hyphen begin its own, at the longest match.
- The Scenario notes each Step through the owning goroutine: a Given or Then once complete, a When before its first Node stops, a Wait as it ends.
- A Wait narrative is a pure function in `internal/scenario` of the entries after the previous Wait's note up to its own. A provisioning Pod is in none. A Pod's first entry marked for deletion reads as its eviction.
- `cmd/lab` prints each narrative above its Wait's board. `--whole-timeline` adds every entry.
- Every control-plane request, a watch's opening included, keeps the 30-second deadline. A stream runs on a second client with the same address, no credential and no deadline, ending with its recording.
- An Interactive run records until the lab stops. Each click changing the cluster is noted as a When through the owning goroutine. Its Wait narratives come from the Timeline recorded so far at the hold, and the Timeline is filed at the stop.
- At the hold the owning goroutine hands over the Timeline so far and then each later entry, until the lab stops. From the stop it hands on none. The run's reader still prints the lines of each entry handed before, the stop waiting for it at most `followingStopWait` (option 29). Then no line starts, and one printing is abandoned. A pure reader in `internal/scenario` turns them into the terminal's lines through the Wait narratives' own reading. A click's When is worded as its Step, and every moment as a Wait narrative's line.
- A click injecting a Failure holds its note open from before its first change (option 32). Once the injection returns, the note is recorded as the When, or as the Failure not injected with its cause. A Node the click stopped stays stopped. A click making Nodes respond again notes its When before its first change.
- Both control-plane clients set no client-side rate limit (option 34).

## Consequences

Every change the API server pushed before the Scenario ended, or before an Interactive run's lab stopped, is in the Timeline, and stopping the record drains each stream. A held Timeline grows for as long as the cluster is held; a bound is an open question in the PRD. A held run's terminal stalling holds up its record until the lab stops, as a stalled file write does. Stopping files the Timeline and removes the cluster whatever the terminal does, a stalled terminal delaying both by the bounded wait at most. A moment recorded after the stop prints no line on any terminal. Cross-stream order is receipt order, not causal order. A filed Timeline narrates as the run that recorded it did. A Wait settles on two observations a poll interval apart, far above a stream's delivery on loopback, so its narrative holds the changes it reflects.

While a click's note is held open, the file and the terminal lag by the injection's length, bounded by the request deadline and its conflict retries. The API server alone paces the lab's requests, so a burst of them waits on its limits rather than the client's.

Timeline files accumulate until an engineer deletes them; retention is an open question in the PRD. A hand-written `watch.Interface` in `internal/cluster`'s test drives the closed-stream and failed-open paths, a real control plane producing none on demand.

## Implementation

**Requirements:** REQ-SIM-005, REQ-SIM-008, REQ-SIM-021, REQ-SIM-028, REQ-SIM-033, REQ-SIM-034, REQ-SIM-040

## References

- [system-design.md § Contracts](../system-design.md#contracts)
- [ADR: The lab provisions the Simulated Cluster it drives](2026-10-03-lab-provisioned-simulated-cluster.md)
