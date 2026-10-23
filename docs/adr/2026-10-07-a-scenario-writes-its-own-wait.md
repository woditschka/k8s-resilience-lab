# A Scenario writes its own Wait, with its Given or as a Step of its own

**Status:** Accepted, amended 2026-10-07: the lab's own Scenarios write each Wait as a Step of its own, option 42; amended 2026-10-08: a Scenario's doc comment states its intention alone, option 46

## Context

On 2026-10-03, [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md) left every wait implicit, option 38. The first Then applied the Topology, deployed the Workload and settled. A When injected one Failure and discarded the observation it injected into, so the next Step needing a Placement awaited a fresh Settled one. Option 39, a settle Step a Scenario writes, was declined then: "Scenarios stay fast and need no wait setting." A Scenario studying a Failure read as the one before it plus one When line, and paid one settle wait per When.

On 2026-10-07, shown the Zone loss Scenario's opening lines, the owner revised that: "Look at this sequence: after Given and before Then there is something missing. A When(... status has settled ...) would make the timing and the precondition visible (possible with a timeout)." The time a Scenario waits is the lab's own subject — after a Failure it is how long the cluster took to place the Pods again — and no line of a Scenario reported it.

The owner asked for Kubernetes' words rather than the lab's: "Probably a more Kubernetes-native naming? AllPodsLive, AllPodsReady, AllPodsLiveAndReady?", then "Also WhenAllNodesLive?". And for a shorter form beside the Step of its own: "What we could do in addition, if this is possible: chain? `s.Given().Topology(lab.Zones(3), lab.NodesPerZone(3))` . when - AllNodesReady(). Is this possible with Go and this design?"

Seeing the implemented lines, the owner revised the chained spelling: "This does not fully resonate: `s.Given().Topology(lab.Zones(3), lab.NodesPerZone(3)).When().AllNodesReady()`. How about `.WaitFor().AllNodesReady()`?" A Wait spelled `WaitFor()` chained and `When()` standalone would be two spellings of one Step, with `When` still doing double duty for the Failure.

On the printed time: "'Wait for all Nodes Ready, after 3.0s' should be 'within 3.0s'. This means relative to the last change, not the total elapsed time from the start of the Scenario." The time a Wait reports is already relative to the Step before it. `within` states a bound rather than a measurement.

Kubernetes' Ready is a condition it reports for a Pod and for a Node, and `kubectl rollout status` waits on the Pod one. The lab's Settled is wider: it also holds where Pods stay Pending, or sit Terminating on a Node that stopped responding. KWOK runs no probe, so Live names no state the control plane reports.

Writing the Step surface showed an apply at the Given's own call colliding with that grammar. `Topology(lab.Zones(3), lab.NodesPerZone(3)).NodeCapacity(lab.Pods(2))` is one line whose capacity is known only at the second call, and the chained `.WaitFor()` extends the line further. An apply at the first call therefore creates the same Nodes twice, which the control plane refuses. The Step-surface tests drive a Scenario over no cluster, which an apply at the Given's call would need a double for.

The Wait Step left the time a Wait states for a later slice, and the owner opened it: "Should a When have an implicit or explicit timeout? I think yes; a timeout should fail the test." Then: "Let's make this more user-friendly: lab.Within(15 * time.Second) -> lab.WithinSeconds(15), or even constants?" Whole seconds match Kubernetes' own spelling of a wait. A Scenario imports `internal/lab` alone, and the time it states is its own statement rather than a set to pick from.

The owner then moved the time out of the argument: "Maybe we add a second version of AllPodsReady: AllPodsReadyWithinSeconds(15)?" Challenged once — it doubles the names per Wait and per option, and leaves the typed-argument pattern a Step's mandatory values follow — the owner answered: "True for the framework, but simpler for the user, and AllPodsReady() can just delegate to AllPodsReadyWithinSeconds with the default."

The lab waits its own waiting time whatever a Wait states, so a run reports how long recovery really took. A Wait cancelled at its stated time would report the bound instead of the measurement, which is the lab's own subject.

The same day the question was which form the lab's own Scenarios write: all three shipped with the chained one. Shown the two long Given lines that leaves, the owner picked the standalone form: "(c) standalone Waits". The owner asked whether every Step kind could chain: "I think it could make sense to allow all three to chain in any sequence, When Given Then." Challenged, the owner answered: "Looks good to me, keep it as is." A Scenario is the lab's own subject matter: an engineer reads it, inserts a Step, removes one, reorders them, and runs it again.

On 2026-10-08, reading the three sources once the layout had landed, the owner found the prose above the Steps restating them: "I find the comments in the Scenarios way too long. How about just a short description of the high-level intention, like in the README?" Reading the trimmed comments, the owner withdrew the name-first form: "The Scenario comment repeats the function name ANodeThatStopsResponding_ShouldLeaveThePodsOnTheNodesStillResponding. This is not needed and not great; just describe the intention of the Scenario." Shown the comment above `package scenarios`, the owner answered: "I would remove this comment also."

The slice left one case open: the Scenario whose Placement settles with Pods still Pending. REQ-SIM-011's Scenario is that case — one Zone stops responding, and the Nodes still responding have room for six of the nine Pods. A Wait for every Pod Ready would end such a Scenario at that Wait, after waiting the lab's whole waiting time. The state it needs is the one `AwaitSettled` already holds for, and the glossary already names that state Settled.

Option 1's objection stands and no longer decides: a state only this lab defines is exactly what such a Scenario is about.

The owner then drew the line at the refusal rather than at the call. The lab's own refusal — a Zone count above the limit, a minimum above the Pod count — is checkable at the call and names its Given. What the control plane refuses may surface at the Step that reaches the cluster, as long as that Step names what was deployed.

Reading a run of the Zone loss Scenario, the owner found the printed time ambiguous: "In the output the time information is unclear: is it the upper limit or the real time it took? Let's make this clearer." Its lines read `Wait for all Nodes Ready in 1.4s` and `Wait for all Pods Ready in 6.5s`. A Wait stating thirty seconds printed the same shape as one stating none. One preposition carried a measurement, and the time the Wait was held to was absent unless missed.

The plain name of each Wait delegated with the lab's own waiting time, so the Step surface held a stated time for every Wait. A line could therefore not tell a time a Scenario chose from the lab's own budget, because nothing recorded which it was.

## Options Considered

What a Scenario waits for:

1. **The lab's own predicate** — `PlacementSettles`, which names a state only this lab defines.
2. **Kubernetes' Ready condition** — `AllPodsReady` and `AllNodesReady`, the states `kubectl` waits on.
3. **Live, or Live and Ready** — no condition behind Live, since no probe runs.

How a Scenario writes a Wait:

4. **A Step of its own** — `s.WaitFor().AllPodsReady()`, one line.
5. **Chained on the Given before it** — `s.Given().Topology(...).WaitFor().AllNodesReady()`, the Given's own call offering the one Wait fitting what it declared.
6. **Folded into the Given** — `s.Given().Topology(...).AllNodesReady()`, which the owner declined as noise.

Where the readiness condition is read:

7. **Inside `AwaitSettled`** — a settled Placement then requires every Pod Ready, which leaves no wait for a Scenario expecting Pods to stay Pending.
8. **A wait beside it over the same polling loop** — `AwaitSettled` keeps the lab's own predicate.

When the declaration reaches the cluster:

9. **At the first Step after the Given's line**, which this grammar makes a Wait — a line's further calls then reach the cluster once. A control-plane refusal names that Wait rather than the Given.
10. **At the Given's own line** — the Topology creates its Nodes and returns, the Workload deploys and returns.
11. **At the Given's first call, with `ApplyTopology` create-or-update** — an optional setting re-applies the same Node names. The apply then tolerates `AlreadyExists`, re-reports the capacity through the status subresource and pays the probe-Pod wait twice.
12. **At the Given's first call, with every setting a mandatory argument** — no further call re-declares a line, at the cost of the Step grammar [the Scenario ADR](2026-10-03-scenarios-as-go-functions.md) fixed.

How a Wait is spelled:

13. **`When()` both ways** — a Wait is a When Step, and `When` names both the state waited for and the Failure.
14. **`WaitFor()` both ways** — one call for the chained and the standalone form, Wait a Step kind of its own.
15. **`WaitFor()` chained, `When()` standalone** — two spellings of one Wait, which the owner declined.

What a Wait's line reports:

16. **`after 3.0s`** — read as the time since the Scenario started, which the owner found hard to reason about.
17. **`within 3.0s`** — the word a bound takes, spent on a measurement.
18. **`in 3.0s`** — the measured time, leaving `within` for the bound a later Wait states.

How a Wait states its time:

19. **A typed argument** — `AllPodsReady(lab.WithinSeconds(15))`, one name per Wait and one more argument type.
20. **A second name per Wait** — `AllPodsReadyWithinSeconds(15)`, the plain name delegating to it.
21. **A constant set** — `lab.FifteenSeconds`, which names no Scenario's own statement.

What a stated time is measured against:

22. **The wait cancelled at the stated time** — the run then reports the bound rather than the time recovery took.
23. **The lab's own waiting time waited out, the measured time judged against the stated one** — the run reports both.

Where a stated time the lab cannot honour is caught:

24. **Clamped to the lab's own waiting time** — a miss then reads against a time the lab never waited.
25. **Refused where it is stated, naming that waiting time** — a Scenario states a time the lab can hold it to.

What a Scenario expecting Pods to stay Pending waits for:

26. **`PlacementSettled`** — the lab's own state, under the word the glossary gives it.
27. **A Wait naming the Pending outcome** — `PodsPending(3)`, which states in the Wait what a Then asserts.
28. **`AllPodsReady` widened** — its readiness clause dropped where Pods stay Pending, so one name covers two states.

Where that Wait is offered:

29. **The Scenario's own `WaitFor()` alone** — such a Wait is always a Step of its own.
30. **A Workload Given's `WaitFor()` as well** — the chained form the two Ready Waits have.

How a Wait's line tells the time it took from the time it was held to:

31. **`reached in 6.5s of the 30s stated`** — one word per time, and the stated marker on the time a Scenario chose.
32. **`took 6.5s, within 30s`** — two prepositions, neither saying which time a reader acts on.
33. **`in 6.5s (at most 30s)`** — the bound in brackets, read as an aside rather than as the Wait's own statement.

Where the line reads the stated time from:

34. **The plain form states none** — a Wait carries a stated time only where its line wrote one, and the lab's own waiting time reaches no Step.
35. **A marker beside the budget** — the plain form keeps delegating with the waiting time, and the Step records that the Scenario did not state it.
36. **The budget compared on the line** — `internal/lab` reads the waiting time and prints no stated time where the two agree, which a Wait stating ninety seconds reads wrong.

How a Then after a When judges, before a Scenario wrote its Wait:

37. **One observation per Scenario** — every Then judges the Placement the Scenario first settled at, so a Then after a When judges the Placement from before the Failure.
38. **A When discards the observation it injected into** — the next Step needing a Placement awaits a fresh Settled one, and no Scenario line states a wait.
39. **A settle Step a Scenario writes** — the Scenario carries a waiting time of its own, which the owner declined on 2026-10-03.

Which form the lab's own Scenarios write, with options 4 and 5 in force:

40. **The chained form wherever a Given precedes the Wait** — the shortest Scenario, the form first committed.
41. **The chained form wrapped over several lines** — the same chain, its calls no longer on one line.
42. **A Step of its own throughout** — every line one Step, opened on the Scenario.
43. **Any Step kind chainable in any sequence** — `Given`, `When` and `Then` in one expression.

What a Scenario's doc comment states:

44. **The Steps in prose** — the comments first written, which restate the lines below them.
45. **The function's name first** — Go's doc convention, which the owner withdrew.
46. **The intention alone** — a sentence or two, as the README beside the Scenarios gives it.

## Decision

Options 2, 4 and 5, 8 and 9, 14 and 18. A Wait is the fourth Step kind: a Given declares, a When changes the cluster, a Wait waits for a state of it, a Then judges. It is written `WaitFor()`, on the Scenario or on the Given before it. A Given's `WaitFor()` returns a type offering the one Wait that fits what it declared. A chain therefore reaches no other Step, and every Wait is written through `WaitFor()`. The Scenario's own `WaitFor()` offers both Waits, and `When()` keeps the Failures alone. Either form reports the same line, after the Given's own, with the time it took: `Wait for all Pods Ready, reached in 6.5s`. That time is measured from the Step before it, so a reader of a restore time reasons about one Wait rather than about the run. The Step carries it as a value of its own rather than inside its name, so what a Step is named stays what its line says. `within` names no measurement: it names the bound on the Wait's own name, `AllPodsReadyWithinSeconds`.

`AllPodsReady` holds until every Pod the Workloads declared reports the Ready condition, `AllNodesReady` until every Node the Topology declared reports it. Beneath either the lab observes that the Placement has stopped changing, so the Thens after a Wait judge a Settled Placement. The Pod wait is a second wait over the polling loop `AwaitSettled` already runs. A Wait for a Placement that settles with Pods Pending is a later slice, and so is a waiting time a Wait names itself.

A Given declares, and the first Step after its line applies what it declared, which this grammar makes a Wait. One line's further calls therefore reach the cluster once, and `ApplyTopology` stays create-only. A Failure When applies nothing and waits for nothing beforehand. It discards the observation it injected into, as option 38 had it. It reads the Placement the last Wait reached. A Failure tolerance is judged over the whole Timeline instead, per [ADR: A Deployment declares the Zones or Nodes it survives losing, judged over the whole Timeline](2026-10-06-required-capacity-judged-at-two-moments.md). A Then, or a Failure, with no Wait since the Given or the Failure before it ends the Scenario at that Step. A Scenario with no Wait after its Givens therefore ends at its first Then, with nothing deployed. A Wait the lab's waiting time outlasts ends it there, naming what was not Ready. Two Waits in a row are admitted, and a Wait for every Pod before any Workload is declared waits for no Pod.

Options 20, 23 and 25. A Wait states the time it must be reached in as a second name per Wait, `AllPodsReadyWithinSeconds` and `AllNodesReadyWithinSeconds`, in whole seconds, on the Scenario's own `WaitFor()` and on a Given's alike. The plain name states none, and is held to the lab's own waiting time, so every Wait is held to a time. The time belongs to the name rather than to an argument: a Scenario reads one call, and the Step vocabulary takes no further type.

The lab's own waiting time moves to `internal/scenario`, beside the unresponsive-Node wait already there. It is what a stated time is refused against, so that refusal is the domain core's own and the Step surface carries no bound of its own. A time of no seconds, and one above that waiting time, are refused at the Wait, named with the waiting time the lab can hold it to.

A Wait waits the lab's own waiting time whatever it states, and the time it took is judged against the time it stated once reached. A Wait reached later ends the Scenario at that Step, its line naming the time it was held to. A Wait the lab's own waiting time outlasts ends it as before, named with what was not Ready: what held the Wait open is more use than the time it missed. Both Failure Scenarios state thirty seconds on the Wait after their Failure — some three times the slowest restore measured, and a third of the lab's own waiting time — so a loaded machine reports a restore rather than a Finding. Every run reports the lab's own waiting time at its start, beside the control-plane settings it sets.

Option 42, since 2026-10-07. Every Scenario of the lab writes each Wait as a Step of its own, so each of its lines is one Step opened on `s`. Such a line is inserted, removed or reordered on its own, which is what tinkering with a Scenario means. Options 40 and 41 are set aside: a chain reads as an expression and keeps the Step kind off the column a reader scans. The chained form stays offered to an engineer writing a Scenario, and the Step-surface tests exercise both. A Given's `WaitFor()` earns its place by scope: the compiler offers the one Wait fitting what that Given declared. Option 43 is declined: a general chain buys syntax rather than scope, and a line's Step kind stops being its opener. The Scenarios' own rule test reads the layout off the sources, as it reads the Step-ordering rules.

Option 46, since 2026-10-08. A Scenario's doc comment states in a sentence or two what it is for. It names neither its own function nor a Step, Wait, time or count: the lines below it hold those. The place the Scenarios sit in carries no package comment, the README beside them holding what it holds.

Options 26 and 29. `PlacementSettled` is the third Wait, with `PlacementSettledWithinSeconds` beside it as options 20, 23 and 25 fixed for the two before it. Its line reads `Wait for the Placement Settled, reached in 12.4s`. It holds for what `AwaitSettled` already holds for, so the predicate reaching the cluster is the one the two Ready Waits sit on. Every declared Pod is accounted for there, placed or not, so a Placement holding Pending Pods settles. One written before any Workload is declared is reached at once, as a Wait with nothing to wait for is.

How many Pods a Placement leaves Pending is a Then, `Then().PodsPending(3)`, beside the counts per Zone and per Node. Option 27 is rejected because a Wait holding for a count would judge where a Then judges. A count such a Wait missed would report what was not Ready rather than a Finding.

Option 28 is rejected because one name for both states says less than the line it prints. A reader of `Wait for all Pods Ready` could no longer tell whether every Pod was Ready.

Option 30 is rejected. A Workload line declares Pods, and the Wait that fits what it declared is every one of them Ready; a Settled Placement is the Scenario's state rather than that line's. The lab's own Scenarios write each Wait as a Step of its own, so the chained form would ship unused.

Options 31 and 34. A Wait's line names one time and says what that time is. A Wait that was reached names the time it took: `Wait for all Nodes Ready, reached in 1.4s`. Where the Scenario stated a time, that time follows: `Wait for all Pods Ready, reached in 6.5s of the 30s stated`. A Wait that was not reached names the time it was held to and not the time it took: `Wait for all Pods Ready, not reached in the 30s stated`. The marker is dropped where the Scenario stated none. Option 31 narrows option 18: `in` still carries the measurement, and `reached` says whether the cluster got there. Options 32 and 33 are rejected because neither says which time a reader acts on.

The plain form of each Wait states no time, so the lab's own waiting time is never carried through the Step surface and keeps its one home in `internal/scenario`. That waiting time and the stated seconds are one domain value, and so is whether a Scenario stated them: the Step records the resolved time beside the time it took, the stated marker, and whether the Wait was reached. Option 35 is rejected because a budget travelling as a stated time is the conflation this line had to undo. Option 36 is rejected because a Wait stating the whole waiting time would then read as having stated nothing. A Wait with nothing to wait for reports that nothing was waited for, as before: no measurable time reaches its line.

## Consequences

Nothing waits silently, and the restore time after a Failure is on a line of the Scenario rather than in the lab. A Scenario of the lab reads one line longer per Wait, and `Then().AllPodsRunning()` leaves all three: right after a Wait for every Pod Ready it says nothing new. It stays offered, for a Scenario that waits on something else. A Given the lab itself refuses is the Step named, its constructor running at the call. A declaration the control plane refuses names the Wait that applied it, with what that Wait applied. The Step-surface tests stay over no cluster, because a Scenario whose Placement is already observed applies nothing. The Ready condition enters the observed Pod, so a Placement compares it and a readiness change counts as the Placement still changing. A Scenario that never Waits reaches no Then: the grammar is explicit in both directions. The Scenario-place rules gain the ordering they read off the sources: a Wait before the first Then and after every Failure. The Step kind and the call opening it differ for a Wait alone. The rule reading a line's kind off its opener therefore carries the four openers, rather than matching the kind's own word. The Workload-line rule admits the Wait chained on it. KWOK reporting no Ready condition for a fake Pod would have left every Wait to outlast its budget. The slice's first cluster-backed test found the condition reported on every Pod and Node.

A Scenario states a restore time, so the lab judges recovery rather than only reporting it. The two Waits are four names, and a later Wait option multiplies them again: the next one weighs that against the argument form once more. A miss is known only once the Wait is reached, so a Scenario stating a short time still waits the lab's own waiting time out. The judgement is a pure function of two durations, so the Step-surface tests exercise it over no cluster and no clock of their own. The lab's own waiting time leaving `internal/cluster` renames nothing a Scenario reads, and gives `cmd/lab` a settings line it could not print before. A Wait's Step carries the time it was held to beside the time it took, and states it only where it was missed, so a Scenario that meets its time reads as it always did.

The lab's Waits are three states and six calls, which the next Wait option weighs again. A Scenario expecting Pods to stay Pending reads its intent on two lines: the Wait that holds for a Settled Placement, and the Then that counts the Pods left Pending. A Workload meeting its minimum with Pods Pending reports no Finding. A Failure tolerance is judged from the first Failure to its Then, per [ADR: A Deployment declares the Zones or Nodes it survives losing, judged over the whole Timeline](2026-10-06-required-capacity-judged-at-two-moments.md). The Scenario-place rules carry the new Wait in the names they read a line's kind by, so a Wait those rules do not know is still reported.

A reader tells the time the cluster took from the time the Scenario allowed it, on the Wait's own line and without reading the Scenario. The three Wait kinds read alike. A Wait the lab's own waiting time outlasts names that time on its line as well as what was not Ready, so the two misses read the same way. `internal/lab` derives the waiting time no longer, and the Step surface carries no bound of its own. A Wait's Step carries four values where it carried two, because a line that drops the measured time on a miss cannot be read off two durations. Every line constant a test matches on moves with the wording, and so does every line quoted beside the Scenarios.

A Failure Scenario of the lab reads two lines longer than the chained form, and every line of it starts with `s.`. No Scenario of the lab writes the chained form, so the Step-surface tests alone assert its scoping. A Scenario added later is held to the layout by the rule test rather than by review. Its doc comment is held by review, because the scanner the rule tests run parses each file without comments. Go's name-first doc convention is set aside, and a package comment with it: `revive`'s `exported` and `package-comments` rules are off, and `godot` checks the closing period alone.

## Implementation

**Requirements:** REQ-SIM-006, REQ-SIM-008, REQ-SIM-011

## References

- [system-design.md § Contracts](../system-design.md)
- [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md)
- [ADR: Provisioning ends when the scheduler has placed a Pod on every Node](2026-10-04-scheduler-observable-provisioning.md)
- [ADR: A Node Failure withholds the Node's status and shortens the eviction wait](2026-10-05-node-failure-withholds-the-nodes-status.md)
