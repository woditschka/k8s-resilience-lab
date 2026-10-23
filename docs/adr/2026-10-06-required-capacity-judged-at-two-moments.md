# A Deployment declares the Zones or Nodes it survives losing, judged over the whole Timeline

**Status:** Accepted, amended 2026-10-22: the declaration counted the Pods lost, option 9. Amended 2026-10-23: it counts Zones or Nodes, sets a Floor, and one Then judges the whole Timeline, options 10, 12, 14 and 16

## Context

The owner named the property the lab is ultimately for: a cluster serves its Required Capacity through a Failure, and the assertions carry that as a first-class citizen. The owner placed the declaration on the Workload and named the property Failure tolerance.

When this was decided, every Then awaited the Settled Placement, so the moment a Failure was injected has no observation of its own. The control plane goes on reporting a failed Node's Pods Running for the eviction wait, per [ADR: A Node Failure withholds the Node's status and shortens the eviction wait](2026-10-05-node-failure-withholds-the-nodes-status.md). An observation taken right after the Failure would therefore read every Pod Running and prove nothing.

A Workload line declares one predefined Pod specification and carries nothing after it, per [ADR: A Deployment declares one of the lab's predefined Pod specifications](2026-10-05-workload-declares-a-predefined-pod-specification.md). A Required Capacity is optional: Resilient placement declares none.

On 2026-10-22 the owner read `.RequiredCapacity(lab.Pods(6))` and asked: "what capacity memor,cpu, why 6?" The owner chose to state what a Failure takes, written `.SurvivesLosingPods(3)`, judged by `Survived` at the moment of the Failure and once Settled.

On 2026-10-23 the owner asked to "think bigger": "we can loose 33% and still serve (1 AZ)". The assertion "shall check the whole timeline if all pods at all times had 100 - 33%". The owner chose `SurvivesLosing(Zones(n))` and `SurvivesLosing(Nodes(n))` on the Deployment line, and found that `Survived` "doesnt carry the whole timeline perspectve". The owner named the Then `EnoughPodsStayedReady`, ruled "A Pod counts while Ready", and "One failing promise fails the whole Then".

## Options Considered

1. **A second cluster observation** taken immediately after the Failure.
2. **A count derived from the Placement the When discards**, read against the Nodes it stopped.
3. **A fourth argument of the Workload Step**, `Workload(name, pods, required, specification)`.
4. **A further call on the Workload line**, `Workload(...).RequiredCapacity(lab.Pods(6))`.
5. **A Given line of its own**, `Given().RequiredCapacity(lab.Pods(6))`.
6. **Two Thens**, one per moment.
7. **One Then judging both moments**.

What the declaration counts:

8. **The fewest Pods kept Running**, `RequiredCapacity(lab.Pods(6))`, the 2026-10-06 form.
9. **The most Pods lost**, `SurvivesLosingPods(3)`, the 2026-10-22 form.
10. **The Zones or Nodes lost**, `SurvivesLosing(lab.Zones(1))` or `SurvivesLosing(lab.Nodes(1))`.
11. **A percentage of the replicas**, the owner's first sketch.

Where the Floor comes from:

12. **The replicas and the Topology**, less the most Pods the declared Zones or Nodes hold under an even spread.
13. **Where the Pods landed**, read from the Placement before the Failure.

What the Then reads:

14. **Every moment from the first Failure**, replayed from the Timeline the run records.
15. **The two moments** of options 2 and 7.

How a Then reads the Timeline before the run ends:

16. **The Scenario keeps a copy** of each entry the recording delivers to its sink.
17. **The recording answers a query** from its owning goroutine.

## Decision

Options 4, 10, 12, 14 and 16.

The Deployment Step returns the Deployment it declared. `SurvivesLosing` takes a `lab.FailureTolerance`, written `lab.Zones(n)` or `lab.Nodes(n)`, and renames the Given line to the loss it names. `scenario.Deployment` holds the tolerance and its Floor through its one constructor, given the Topology declared before it.

The Floor is the replicas less the most Pods n of the Topology's Zones, or n of its Nodes, hold when the replicas spread evenly over all of them. Nine replicas over three Zones of three Nodes keep six through one Zone and eight through one Node. A count below zero, or reaching the Topology's own Zones or Nodes, is refused at that Given. A count of zero is accepted, its Floor every replica.

The Deployment-line rule admits a line ending at one further call of the Deployment's own closed set, `SurvivesLosing` today. This narrows the specification ADR's consequence that the rule admits nothing after the Step. `lab.Zones` and `lab.Nodes` name which domain a count counts, narrowing the bare-count rule of [ADR: Scenario code stays idiomatic Go, read by subject](2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md) for this one call.

The Scenario keeps every Deployment declaring a tolerance. `EnoughPodsStayedReady` judges each of them, narrowing [the taint ADR](2026-10-19-a-nodepool-states-noschedule-taints-and-a-later-workload-deploys-beside-the-earlier.md)'s rule that a Then judges the newest Deployment alone. A Then with no When before it, or no declaring Deployment, ends the Scenario there.

The Then replays every entry received so far. It counts a Deployment's Pods Ready at each entry from the first When's note onward. A Pod counts while its latest entry reads Running. The lab's pod-ready Stage sets Running and Ready together, per [ADR: The lab owns the simulator's Stages to give a Pod its Startup time](2026-10-08-the-lab-owns-the-simulators-stages-for-a-startup-time.md). A Pod on a Node a When stopped counts as lost from that When's note, whatever later entries report.

Beneath the Then, one line per judged Deployment states kept or fell, its Floor and declared loss. It adds the lowest Ready count, when first reached and how long it lasted. A Deployment below its Floor at any moment records one Finding, for its fall reaching the lowest count. The Finding states how low, the fall's start and span, the Pods lost against the most declared, and those Pods with their Nodes. A fall still open at the Then lasts "for the rest of the Scenario".

Option 1 races the control plane and reads a first moment that proves nothing. Option 3 makes an optional declaration mandatory on every Deployment line. Option 5 reads apart from the Pods it qualifies. Options 6, 7 and 15 judge two moments, so a dip between them passes unseen. Option 8 reads as a cpu or memory capacity. Option 9 states a count the reader must derive from the Topology, where the owner thinks in Zones. Option 11 rounds, and still hides which loss it means. Option 13 lets a skewed Placement lower its own Floor. Option 17 adds a query channel to a goroutine that takes notes alone.

## Consequences

The judgement reads the Timeline the lab already records, so every moment is an observation and no derivation of the first moment remains.

A Floor derives from every Zone or Node of the Topology. Which Nodes count for a Deployment confined to one NodePool stays an Open Question of `docs/prd.md`.

Running stands for Ready while the lab owns the pod-ready Stage. A Stage setting them apart records a Pod's readiness in its entry first.

The rename precedes the judgement, because a Finding carries the Step as it was recorded.

`internal/expect` keeps its general matchers alone. The reading is a pure function in `internal/scenario`, and its wording stays beside the Step wording in `internal/lab`.

A declared Failure tolerance no When exercises is a declaration the run does not prove.

The Then reads the copy while the recording runs, with no barrier of its own. Every Then follows a Wait, and every Wait returns a Placement two observations a poll interval apart agreed on. The changes it reflects therefore reached the API server a poll interval before the Then, far above a stream's delivery on the loopback. The Wait narratives rely on the same margin, per [ADR: A Timeline is recorded from the API's watch stream and filed as JSON Lines](2026-10-05-timeline-from-the-watch-stream.md). A delivery slower than that reads as a Pod not yet Ready.

## Implementation

**Requirements:** REQ-SIM-027, superseding REQ-SIM-010; REQ-SIM-025

## References

- [system-design.md § Contracts](../system-design.md)
- [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md)
- [ADR: A Deployment declares one of the lab's predefined Pod specifications](2026-10-05-workload-declares-a-predefined-pod-specification.md)
- [ADR: A Node Failure withholds the Node's status and shortens the eviction wait](2026-10-05-node-failure-withholds-the-nodes-status.md)
- [ADR: A Timeline is recorded from the API's watch stream and filed as JSON Lines](2026-10-05-timeline-from-the-watch-stream.md) — the record the Then replays
- [ADR: A Scenario writes its own Wait, with its Given or as a Step of its own](2026-10-07-a-scenario-writes-its-own-wait.md)
