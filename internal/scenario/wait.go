package scenario

import (
	"fmt"
	"time"
)

// WaitingTime is the longest the lab waits for a state of the cluster and the
// longest time a Wait may state. The lab waits it out whatever a Wait states,
// so a run reports how long recovery really took rather than the bound it
// missed.
//
// It lives here because a stated time is refused against it, and this package
// validates the lab's values.
const WaitingTime = 90 * time.Second

// LeastStatedTime is the least time a Wait may state. A cluster already in the
// state still takes a poll and two agreeing observations to report it, so a
// shorter time could never be met.
//
// It is a code constant of its own because this package cannot import the
// cluster's poll interval.
const LeastStatedTime = 2 * time.Second

// StatedWait is resolved at construction and reads no clock, so judging a Wait
// reaches no cluster. Whether the time was stated travels with it, because a
// Wait's line marks a stated time and leaves the lab's own waiting time
// unmarked.
type StatedWait struct {
	heldTo time.Duration
	stated bool
}

// NewStatedWait refuses a time below LeastStatedTime, which no cluster can meet,
// and one above WaitingTime, which the lab cannot wait past. Either refusal
// names the range a reader has to choose inside.
func NewStatedWait(seconds int) (StatedWait, error) {
	heldTo := time.Duration(seconds) * time.Second
	if heldTo < LeastStatedTime || heldTo > WaitingTime {
		return StatedWait{}, fmt.Errorf("a stated time of %d seconds (%s) is refused; a Wait states between %s and %s",
			seconds, heldTo, LeastStatedTime, WaitingTime)
	}

	return StatedWait{heldTo: heldTo, stated: true}, nil
}

// NewWaitStatingNoTime refuses nothing, since the time is the lab's own rather
// than a Scenario's statement; that is why it is a second constructor rather
// than NewStatedWait called with WaitingTime.
func NewWaitStatingNoTime() StatedWait {
	return StatedWait{heldTo: WaitingTime, stated: false}
}

func (w StatedWait) HeldTo() time.Duration {
	return w.heldTo
}

func (w StatedWait) Stated() bool {
	return w.stated
}

// Reached judges a Wait once it is reached, so measured is the whole wait and
// one reached exactly at the bound is reached in it. A Wait stating no time is
// held to none, because the lab's waiting time already bounds each state it
// waits for.
func (w StatedWait) Reached(measured time.Duration) bool {
	return !w.stated || measured <= w.heldTo
}
