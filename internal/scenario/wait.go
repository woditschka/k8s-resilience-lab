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

// StatedWait is the time a Wait must be reached in: the whole seconds a Scenario
// states on the Wait itself, resolved at construction. It judges the time a Wait
// took against itself and holds nothing else, so the judgement reaches no cluster
// and reads no clock.
type StatedWait struct {
	within time.Duration
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

	within := time.Duration(seconds) * time.Second
	if within > WaitingTime {
		return StatedWait{}, fmt.Errorf("the lab waits %s for a state of the cluster, so a stated time of %s is refused",
			WaitingTime, within)
	}

	return StatedWait{within: within}, nil
}

// Within is the time a Wait held to this must be reached in.
func (w StatedWait) Within() time.Duration {
	return w.within
}

// Reached reports whether a Wait that took measured was reached in the time it
// states. A Wait is judged once it is reached, so measured is the whole wait and
// one reached at the time it states is reached in it.
func (w StatedWait) Reached(measured time.Duration) bool {
	return measured <= w.within
}
