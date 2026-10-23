# A Deployment declares the Zones or Nodes it survives losing, judged over the whole Timeline

**Status:** Accepted

## Context

The owner named the property the lab is ultimately for: a cluster keeps serving through a Failure, and the assertions carry that as a first-class citizen, declared on the Deployment. The owner thinks in failure domains: "we can loose 33% and still serve (1 AZ)". The check "shall check the whole timeline if all pods at all times had 100 - 33%". The owner named the Then `EnoughPodsStayedReady`, ruled "A Pod counts while Ready", and "One failing promise fails the whole Then".

The control plane reports a failed Node's Pods Running for the eviction wait, per [ADR: A Node Failure withholds the Node's status and shortens the eviction wait](2026-10-05-node-failure-withholds-the-nodes-status.md). A single observation after the Failure therefore proves nothing.

## Options Considered

Where the declaration stands:

1. **A further argument of the Deployment Given** — makes an optional declaration mandatory on every line.
2. **A further call on the Deployment line**.
3. **A Given line of its own** — reads apart from the Pods it qualifies.

What it counts:

4. **The fewest Pods kept Running** — reads as a cpu or memory capacity.
5. **The most Pods lost** — a count the reader derives from the Topology.
6. **The Zones or Nodes lost**, `SurvivesLosing(lab.Zones(1))` or `SurvivesLosing(lab.Nodes(1))`.
7. **A percentage of the replicas** — rounds, and hides which loss it means.

Where the Floor comes from:

8. **The replicas and the Topology**, less the most Pods the declared Zones or Nodes hold under an even spread.
9. **Where the Pods landed** before the Failure — lets a skewed Placement lower its own Floor.

What the Then reads:

10. **Every moment from the first Failure**, replayed from the Timeline.
11. **An observation right after the Failure and one once Settled** — races the control plane, and a dip between them passes unseen.

How a Then reads the Timeline before the run ends:

12. **The Scenario keeps a copy** of each entry the recording delivers.
13. **The recording answers a query** — a query channel on a goroutine that takes notes alone.

## Decision

Options 2, 6, 8, 10 and 12.

- The Deployment Step returns the Deployment it declared. `SurvivesLosing` takes a `lab.FailureTolerance`, `lab.Zones(n)` or `lab.Nodes(n)`, and renames the Given line to the loss it names.
- `scenario.Deployment` holds the tolerance and its Floor through its constructor, given the Topology. Nine replicas over three Zones of three Nodes keep six through one Zone, eight through one Node. A count below zero, or reaching the Topology's Zones or Nodes, is refused.
- The Deployment-line rule admits one further call of the Deployment's closed set, `SurvivesLosing` today. `lab.Zones` and `lab.Nodes` are the one exception to bare counts.
- `EnoughPodsStayedReady` judges every declaring Deployment. With no When before it, or no declaring Deployment, it ends the Scenario.
- The Then replays every entry so far from the first When's note. A Pod counts while its latest entry reads Running, and one on a Node a When stopped is lost from that note.
- One line per judged Deployment states kept or fell, its Floor, the declared loss, and the lowest Ready count with when and how long. A fall below the Floor records one Finding for the deepest fall, naming the Pods lost and their Nodes.

## Consequences

Every moment is an observation from the Timeline already recorded. Running stands for Ready while the lab owns the pod-ready Stage, per [ADR: The lab owns the simulator's Stages to give a Pod its Startup time](2026-10-08-the-lab-owns-the-simulators-stages-for-a-startup-time.md).

Which Nodes count for a Deployment confined to one NodePool stays an Open Question of `docs/prd.md`. A declared tolerance no When exercises is not proven by the run.

The Then reads the copy with no barrier. Every Then follows a Wait settled a poll interval after its changes, far above a stream's delivery, per [ADR: A Timeline is recorded from the API's watch stream and filed as JSON Lines](2026-10-05-timeline-from-the-watch-stream.md). The reading is a pure function in `internal/scenario`.

## Implementation

**Requirements:** REQ-SIM-025, REQ-SIM-027

## References

- [system-design.md § Contracts](../system-design.md#contracts)
- [ADR: A Deployment declares one of the lab's predefined Pod specifications](2026-10-05-workload-declares-a-predefined-pod-specification.md)
- [ADR: A Scenario writes its own Wait, with its Given or as a Step of its own](2026-10-07-a-scenario-writes-its-own-wait.md)
