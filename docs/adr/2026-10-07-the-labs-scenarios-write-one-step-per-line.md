# The lab's Scenarios write one Step per line

**Status:** Accepted

## Context

[The Wait ADR](2026-10-07-a-scenario-writes-its-own-wait.md) offers two forms: a Wait on the Scenario, and a Wait chained onto the Given before it. It left open which form the lab's own Scenarios write, and all three shipped with the chained one.

Shown the two long Given lines that leaves, the owner picked the standalone form: "(c) standalone Waits". The owner asked whether every Step kind could chain in any sequence: "I think it could make sense to allow all three to chain in any sequence, When Given Then." Challenged, the owner answered: "Looks good to me, keep it as is."

A Scenario is the lab's own subject matter: an engineer reads it, inserts a Step, removes one, reorders them, and runs it again.

Reading the three sources once the layout had landed, the owner found the prose above the Steps restating them: "I find the comments in the Scenarios way too long. How about just a short description of the high-level intention, like in the README?"

Reading the trimmed comments, the owner withdrew the name-first form: "The Scenario comment repeats the function name ANodeThatStopsResponding_ShouldLeaveThePodsOnTheNodesStillResponding. This is not needed and not great; just describe the intention of the Scenario." Shown the comment above `package scenarios`, the owner answered: "I would remove this comment also."

## Options Considered

1. **The chained form wherever a Given precedes the Wait** — the shortest Scenario, the form first committed.
2. **The chained form wrapped over several lines** — the same chain, its calls no longer on one line.
3. **A Step of its own throughout** — every line one Step, opened on the Scenario.
4. **Any Step kind chainable in any sequence** — `Given`, `When` and `Then` in one expression.

## Decision

Option 3. Every Scenario of the lab writes each Wait as a Step of its own, so each of its lines is one Step opened on `s`. One Step per line is insertable, removable and reorderable on its own, which is what tinkering with a Scenario means. A chain reads as an expression and keeps the Step kind off the fixed column a reader scans.

The chained form stays offered to an engineer writing a Scenario, and the Step-surface tests keep exercising both. A Given's `WaitFor()` earns its place by scope: the compiler offers the one Wait fitting what that Given declared.

Option 4 is declined. A general chain buys syntax rather than scope, and the Step kind of a line stops being its opener.

The Scenarios' own rule test reads the layout off the sources, as the Step-ordering rules are read.

The prose above the Steps is held to the same split. A Scenario's doc comment states in a sentence or two what it is for. It names neither its own function nor a Step, Wait, time or count: those are read in the lines below it. The place the Scenarios sit in states nothing above them, the README beside them holding what it holds.

## Consequences

A Failure Scenario reads two lines longer, and every line of a Scenario starts with `s.`. The chained form is written by no Scenario of the lab, so its scoping is asserted by the Step-surface tests alone. A Scenario added later is held to the layout by the rule test rather than by review. The prose above its Steps is held the other way round. Review reads it, because the sources the rule tests scan carry no comment: the scanner parses each file without comments and its model holds none. Go's name-first doc convention is set aside here, and a package comment with it: `revive`'s `exported` and `package-comments` rules are off, and `godot` checks the closing period alone.

## Implementation

**Requirements:** REQ-SIM-006, REQ-SIM-008

## References

- [system-design.md § Contracts](../system-design.md)
- [ADR: A Scenario writes its own Wait, with its Given or as a Step of its own](2026-10-07-a-scenario-writes-its-own-wait.md)
- [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md)
