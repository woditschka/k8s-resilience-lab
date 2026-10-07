package scenario_test

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

const (
	// theWaitingTimeInSeconds is the lab's own waiting time in the whole seconds a
	// Wait states a time in, which is the longest time a Wait may state.
	theWaitingTimeInSeconds = int(scenario.WaitingTime / time.Second)

	// aTimeAWaitStates is a time well inside that waiting time, and aMomentOfIt the
	// margin the judgement below is read on either side of it.
	aTimeAWaitStates = 15
	aMomentOfIt      = 100 * time.Millisecond

	// noSecondsAtAll and aNegativeTime are the times no state of the cluster is
	// reached in, so a Wait stating one of them is refused.
	noSecondsAtAll = 0
	aNegativeTime  = -1
)

// aStatedWaitOf is the time a Wait of that many whole seconds is held to.
func aStatedWaitOf(t *testing.T, seconds int) scenario.StatedWait {
	t.Helper()

	stated, err := scenario.NewStatedWait(seconds)
	if err != nil {
		t.Fatalf("build the time a Wait of %d seconds is held to: %v", seconds, err)
	}

	return stated
}

func TestAWaitStatingATimeOfNoSeconds_ShouldBeRefused(t *testing.T) {
	refusals := map[string]int{
		"no seconds at all": noSecondsAtAll,
		"a negative time":   aNegativeTime,
	}

	for refused, seconds := range refusals {
		t.Run(refused, func(t *testing.T) {
			_, err := scenario.NewStatedWait(seconds)

			if err == nil {
				t.Fatalf("a Wait stating a time of %d seconds is refused, but it was admitted", seconds)
			}
			if !strings.Contains(err.Error(), strconv.Itoa(seconds)) {
				t.Errorf("the refusal names %d, but reads: %v", seconds, err)
			}
		})
	}
}

func TestAWaitStatingATimeAboveTheLabsOwnWaitingTime_ShouldBeRefusedNamingThatWaitingTime(t *testing.T) {
	aTimeTheLabNeverWaits := theWaitingTimeInSeconds + 1

	_, err := scenario.NewStatedWait(aTimeTheLabNeverWaits)

	if err == nil {
		t.Fatalf("a Wait stating a time of %d seconds is refused, but it was admitted", aTimeTheLabNeverWaits)
	}
	if !strings.Contains(err.Error(), scenario.WaitingTime.String()) {
		t.Errorf("the refusal names the lab's own waiting time %s, but reads: %v", scenario.WaitingTime, err)
	}
}

// The lab's own waiting time is the longest a Wait may state and the time a Wait
// stating none is held to, so it is admitted rather than refused as above itself.
func TestAWaitStatingTheLabsOwnWaitingTime_ShouldBeHeldToExactlyThatTime(t *testing.T) {
	stated := aStatedWaitOf(t, theWaitingTimeInSeconds)

	expect.Expect(t, stated.Within()).To(expect.Equal(scenario.WaitingTime))
}

// A Wait is judged once it is reached, so the time read against the time stated
// is the whole wait and a Wait reached at the time it states is reached in it.
func TestTheTimeAWaitTook_ShouldBeReachedOnlyWhereItIsWithinTheTimeStated(t *testing.T) {
	stated := aStatedWaitOf(t, aTimeAWaitStates)

	measured := map[string]struct {
		took    time.Duration
		reached bool
	}{
		"inside the time it states":        {took: stated.Within() - aMomentOfIt, reached: true},
		"exactly at the time it states":    {took: stated.Within(), reached: true},
		"a moment past the time it states": {took: stated.Within() + aMomentOfIt, reached: false},
	}

	for read, one := range measured {
		t.Run(read, func(t *testing.T) {
			expect.Expect(t, stated.Reached(one.took)).To(expect.Equal(one.reached))
		})
	}
}
