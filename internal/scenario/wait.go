package scenario

import (
	"fmt"
	"time"
)

// WaitingTime is the lab's own waiting time: the longest it waits for a state of
// the cluster, and the longest time a Wait may state. A Wait stating none is held
// to it, and the lab waits it out whatever a Wait states, so a run reports how
// long recovery really took rather than the bound it missed, per
// docs/adr/2026-10-07-a-scenario-writes-its-own-wait.md.
//
// It lives here because it is what a stated time is refused against, and this
// package is where the lab's own values are validated. The unresponsive-Node wait
// beside it is a second waiting time of the lab, and a shorter one: it is how
// long a Pod stays on a Node that stopped responding.
const WaitingTime = 90 * time.Second

// StatedWait is the time a Wait is held to and whether the Scenario stated it:
// the whole seconds written on the Wait itself, or the lab's own waiting time
// where the Wait states none, resolved at construction. It judges the time a Wait
// took against itself and holds nothing else, so the judgement reaches no cluster
// and reads no clock.
//
// The statement travels with the time rather than being read back off it, because
// a Wait's line marks a stated time and leaves the lab's own waiting time
// unmarked: a reader of a line then never reads that bound for a time a Scenario
// chose (docs/prd.md REQ-SIM-006).
type StatedWait struct {
	heldTo time.Duration
	stated bool
}

// NewStatedWait builds the time a Wait of that many whole seconds is held to.
//
// A time of no seconds is refused, because no state of the cluster is reached in
// no time, and so is one above the lab's own waiting time: the lab cannot wait
// longer than it does, and a Wait held to a time never waited would report a miss
// against it. Either refusal names that waiting time, because the time a Wait may
// state is what a reader has to choose inside.
func NewStatedWait(seconds int) (StatedWait, error) {
	if seconds < 1 {
		return StatedWait{}, fmt.Errorf("no state of the cluster is reached in no time, so a stated time of %d seconds is refused; a Wait states between %s and %s",
			seconds, time.Second, WaitingTime)
	}

	heldTo := time.Duration(seconds) * time.Second
	if heldTo > WaitingTime {
		return StatedWait{}, fmt.Errorf("the lab waits %s for a state of the cluster, so a stated time of %s is refused",
			WaitingTime, heldTo)
	}

	return StatedWait{heldTo: heldTo, stated: true}, nil
}

// NewWaitStatingNoTime builds the time a Wait stating none of its own is held to:
// the lab's own waiting time, which is the longest the lab waits, so every Wait
// has a time to be judged against.
//
// It is refused nothing, since the time is the lab's own rather than a Scenario's
// statement, which is why it is a second way into this value rather than the
// constructor above called with that waiting time.
func NewWaitStatingNoTime() StatedWait {
	return StatedWait{heldTo: WaitingTime, stated: false}
}

// HeldTo is the time a Wait held to this must be reached in.
func (w StatedWait) HeldTo() time.Duration {
	return w.heldTo
}

// Stated reports whether a Scenario stated that time on the Wait itself, as
// against the lab's own waiting time a Wait stating none is held to.
func (w StatedWait) Stated() bool {
	return w.stated
}

// Reached reports whether a Wait that took measured was reached in the time it is
// held to. A Wait is judged once it is reached, so measured is the whole wait and
// one reached at that time is reached in it.
func (w StatedWait) Reached(measured time.Duration) bool {
	return measured <= w.heldTo
}
