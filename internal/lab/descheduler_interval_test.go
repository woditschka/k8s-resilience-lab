package lab

import (
	"context"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
)

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
