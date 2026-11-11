//go:build !integration

package lab

import (
	"context"
	"testing"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// Waiting for the pass and then for Settled takes each the lab's waiting time at
// most, so the whole wait may outlast one waiting time.
const aPassWaitOutlastingOneWaitingTime = scenario.WaitingTime + 5*time.Second

const (
	evictedNothing = false
	evictedAPod    = true
)

// The interval's passes are published by hand here, so the order of passes
// and the Wait's start is the test's to state rather than a cluster's timing.
// The second pass is running as the Wait begins.
func TestAWaitForADeschedulerPassEvictingNothing_ShouldEndAtTheFirstSuchPassBegunAfterIt(t *testing.T) {
	descheduling := newDeschedulerInterval()
	descheduling.passBegun()
	descheduling.passEnded(evictedNothing)
	descheduling.passBegun()
	begunBefore := descheduling.passesBegun()
	descheduling.passEnded(evictedNothing)
	descheduling.passBegun()
	descheduling.passEnded(evictedAPod)
	expired, expire := context.WithCancel(t.Context())
	expire()

	notYet := descheduling.awaitAPassEvictingNothingAfter(expired, begunBefore)
	descheduling.passBegun()
	descheduling.passEnded(evictedNothing)
	reached := descheduling.awaitAPassEvictingNothingAfter(t.Context(), begunBefore)

	expect.Expect(t, notYet != nil).To(expect.Equal(true))
	expect.Expect(t, reached).To(expect.Equal[error](nil))
}

func TestAWaitForADeschedulerPassEvictingNothingReachedAfterTheLabsWaitingTime_ShouldBeReportedReached(t *testing.T) {
	waiting := aScenarioAtAWaitFor(aDeschedulerPassEvictingNothing)

	waiting.reached(scenario.NewWaitStatingNoTime(), aPassWaitOutlastingOneWaitingTime)

	reached := waiting.outcome()
	expect.Expect(t, reached.Ended()).To(expect.Equal(false))
	expect.Expect(t, reached.Steps[theWaitsOwnStep].String()).
		To(expect.Equal("Wait for " + aDeschedulerPassEvictingNothing + ", reached in " + aPassWaitOutlastingOneWaitingTime.String()))
}
