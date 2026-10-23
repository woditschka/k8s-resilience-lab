# A Deployment declares its Failure tolerance, judged at the moment of the Failure and once Settled

**Status:** Accepted, amended 2026-10-22: the declaration counts the Pods lost, `SurvivesLosingPods`, and its Then is `Survived`, option 9

## Context

The owner named the property the lab is ultimately for: a cluster serves its Required Capacity through a Failure, and the assertions carry that as a first-class citizen. The owner placed the declaration on the Workload and named the property Failure tolerance.

When this was decided, every Then awaited the Settled Placement, so the moment a Failure was injected has no observation of its own. The control plane goes on reporting a failed Node's Pods Running for the eviction wait, per [ADR: A Node Failure withholds the Node's status and shortens the eviction wait](2026-10-05-node-failure-withholds-the-nodes-status.md). An observation taken right after the Failure would therefore read every Pod Running and prove nothing.

The When already holds two things at that moment: the Settled Placement from before the Failure, and the Nodes it is stopping.

A Workload line declares one predefined Pod specification and carries nothing after it, per [ADR: A Deployment declares one of the lab's predefined Pod specifications](2026-10-05-workload-declares-a-predefined-pod-specification.md). A Required Capacity is optional: Resilient placement declares none.

On 2026-10-22 the owner read `.RequiredCapacity(lab.Pods(6))` and asked: "what capacity memor,cpu, why 6?", then: "i still don't get this line dreat 0 standard pods then reduce to 6?" The owner chose to state what a Failure takes, written `.SurvivesLosingPods(3)`, with Failure tolerance as the term. The Workload became a Deployment, per [ADR: A Deployment declares one of the lab's predefined Pod specifications](2026-10-05-workload-declares-a-predefined-pod-specification.md).

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
9. **The most Pods lost**, `SurvivesLosingPods(3)`, a Pod not Running at a moment lost there.

## Decision

Options 2, 4, 7 and 9.

The Deployment Step returns the Deployment it declared, as the Zones Step returns the Topology it declared. `SurvivesLosingPods` is written on that value with a bare count and renames the Given line to the loss it names. `scenario.Deployment` holds the Failure tolerance as an optional attribute routed through its one constructor. It refuses a tolerance below zero and one of every replica or more, and accepts zero.

The Deployment-line rule admits a line ending at one further call of the Deployment's own closed set, `SurvivesLosingPods` today. A setting of the Scenario's own, a further argument, and a Topology call written after the Deployment Step stay refused. This narrows the specification ADR's consequence that the rule admits nothing after the Step.

The When derives what the Failure cost from the Placement it discards: the Pods that observation found Running on Nodes other than the ones being stopped. The Step surface keeps that one count, which a later When overwrites.

One Then, `Survived`, judges the tolerance at both moments. The Pods lost at a moment are the replicas less the Pods Running there, so a Pod on a stopped Node or Pending for want of room is lost. The Then renames its Step to the tolerance and the two losses, so a run states them whether met or not. A moment losing more is a Finding naming that moment, reported through a matcher the Step surface declares over `expect.Matcher`.

Option 1 races the control plane and reads a first moment that proves nothing. Option 3 makes an optional declaration mandatory on every Deployment line. Option 5 reads apart from the Pods it qualifies, against the owner's placing it on the Deployment. Option 6 reads as two Steps for one judgement. Option 8 counts the survivors, which reads as a cpu or memory capacity and as a scale-down the owner could not follow, while the owner chose to state what a Failure takes.

## Consequences

The first moment is a derivation and not an observation. It holds because the Placement before a Failure is Settled, which accounts for every declared Pod.

A Scenario with several Whens judges the latest Failure at the first moment, while the Thens reading what failed judge every Failure so far.

The rename precedes the judgement, because a Finding carries the Step as it was recorded.

Resilient placement declares no Failure tolerance, so the further call is exercised by the two Failure Scenarios and by the rule's own cases.

A later slice judging spread or restore time at the moment of the Failure takes the same derivation, or records its own reason to observe the cluster instead.

`internal/expect` keeps its general matchers alone, and the wording naming a moment stays beside the Step wording in `internal/lab`.

A declared Failure tolerance that no When exercises is a declaration the run does not prove; the Failures a Deployment declares it survives stay an Open Question of `docs/prd.md`.

## Implementation

**Requirements:** REQ-SIM-010, REQ-SIM-025

## References

- [system-design.md § Contracts](../system-design.md)
- [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md)
- [ADR: A Deployment declares one of the lab's predefined Pod specifications](2026-10-05-workload-declares-a-predefined-pod-specification.md)
- [ADR: A Node Failure withholds the Node's status and shortens the eviction wait](2026-10-05-node-failure-withholds-the-nodes-status.md)
- [ADR: A Scenario writes its own Wait, with its Given or as a Step of its own](2026-10-07-a-scenario-writes-its-own-wait.md) — where each moment's observation comes from, now that a Scenario writes its Wait
