# A Workload declares its Required Capacity, judged at the moment of the Failure and once Settled

**Status:** Accepted

## Context

The owner named the property the lab is ultimately for: a cluster serves its Required Capacity through a Failure, and the assertions carry that as a first-class citizen. The owner placed the declaration on the Workload and named the property Failure tolerance.

Every Then awaits the Settled Placement, so the moment a Failure was injected has no observation of its own. The control plane goes on reporting a failed Node's Pods Running for the eviction wait, per [ADR: A Node Failure withholds the Node's status and shortens the eviction wait](2026-10-05-node-failure-withholds-the-nodes-status.md). An observation taken right after the Failure would therefore read every Pod Running and prove nothing.

The When already holds two things at that moment: the Settled Placement from before the Failure, and the Nodes it is stopping.

A Workload line declares one predefined Pod specification and carries nothing after it, per [ADR: A Workload declares one of the lab's predefined Pod specifications](2026-10-05-workload-declares-a-predefined-pod-specification.md). A Required Capacity is optional: Resilient placement declares none.

## Options Considered

1. **A second cluster observation** taken immediately after the Failure.
2. **A count derived from the Placement the When discards**, read against the Nodes it stopped.
3. **A fourth argument of the Workload Step**, `Workload(name, pods, required, specification)`.
4. **A further call on the Workload line**, `Workload(...).RequiredCapacity(lab.Pods(6))`.
5. **A Given line of its own**, `Given().RequiredCapacity(lab.Pods(6))`.
6. **Two Thens**, one per moment.
7. **One Then judging both moments**.

## Decision

Options 2, 4 and 7.

The Workload Step returns the Workload it declared, as the Topology Step returns the Topology it declared. `RequiredCapacity` is written on that value and renames the Given line to the minimum it names. `scenario.Workload` holds the minimum as an optional attribute routed through its one constructor, which refuses a minimum above the Pods declared and one below one Pod.

The Workload-line rule admits a line ending at one further call of the Workload's own closed set, `RequiredCapacity` today. A setting of the Scenario's own, a further argument, and a Topology call written after the Workload Step stay refused. This narrows the specification ADR's consequence that the rule admits nothing after the Step.

The When derives what the Failure cost from the Placement it discards: the Pods that observation found Running on Nodes other than the ones being stopped. The Step surface keeps that one count, which a later When overwrites.

One Then, `RequiredCapacityHeld`, judges the minimum at both moments and renames its Step to the minimum and the two counts, so a run states them whether met or not. A moment missed is a Finding naming that moment, reported through a matcher the Step surface declares over `expect.Matcher`.

Option 1 races the control plane and reads a first moment that proves nothing. Option 3 makes an optional declaration mandatory on every Workload line. Option 5 reads apart from the Pods it qualifies, against the owner's placing it on the Workload. Option 6 reads as two Steps for one judgement.

## Consequences

The first moment is a derivation and not an observation. It holds because the Placement before a Failure is Settled, which accounts for every declared Pod.

A Scenario with several Whens judges the latest Failure at the first moment, while the Thens reading what failed judge every Failure so far.

The rename precedes the judgement, because a Finding carries the Step as it was recorded.

Resilient placement declares no minimum, so the further call is exercised by the two Failure Scenarios and by the rule's own cases.

A later slice judging spread or restore time at the moment of the Failure takes the same derivation, or records its own reason to observe the cluster instead.

`internal/expect` keeps its general matchers alone, and the wording naming a moment stays beside the Step wording in `internal/lab`.

A declared minimum that no When exercises is a declaration the run does not prove; the Failures a Workload declares it survives stay an Open Question of `docs/prd.md`.

## Implementation

**Requirements:** REQ-SIM-010

## References

- [system-design.md § Contracts](../system-design.md)
- [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md)
- [ADR: A Workload declares one of the lab's predefined Pod specifications](2026-10-05-workload-declares-a-predefined-pod-specification.md)
- [ADR: A Node Failure withholds the Node's status and shortens the eviction wait](2026-10-05-node-failure-withholds-the-nodes-status.md)
