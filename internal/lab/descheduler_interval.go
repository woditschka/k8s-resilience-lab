package lab

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/cluster"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// deschedulerInterval is the one goroutine running a Scenario's Descheduler
// passes on its interval. It publishes each pass under its lock, since the
// Steps read what it published on another goroutine.
type deschedulerInterval struct {
	stop func()

	guard sync.Mutex
	// begun counts every pass started, the one running included, and
	// evictedPerPass holds whether each pass ended evicted a Pod, in pass order.
	// Passes never overlap, so the k-th pass ended is the k-th begun.
	begun          int
	evictedPerPass []bool
	failed         error
	// failing is closed as a pass fails, which only the one pass that stops the
	// interval does.
	failing chan struct{}
	// published is closed and replaced at every change, which wakes each Wait
	// holding the one closed.
	published chan struct{}
}

func newDeschedulerInterval() *deschedulerInterval {
	return &deschedulerInterval{stop: func() {}, failing: make(chan struct{}), published: make(chan struct{})}
}

// startDeschedulerInterval starts the first pass one interval from now and each
// later one an interval after the last began, or as the last exits where it
// outlasted the interval. A failed pass stops the interval. The pass's own
// command runs it once, so the interval is the lab's alone.
func startDeschedulerInterval(ctx context.Context, provisioned *cluster.SimulatedCluster,
	policy scenario.DeschedulerPolicy, version cluster.DeschedulerVersion, interval scenario.DeschedulerInterval,
) *deschedulerInterval {
	descheduling := newDeschedulerInterval()
	running, stopRunning := context.WithCancel(ctx)
	var ended sync.WaitGroup
	ended.Go(func() {
		next := time.NewTimer(interval.Every())
		defer next.Stop()

		for {
			select {
			case <-running.Done():
				return
			case <-next.C:
			}

			next.Reset(interval.Every())
			descheduling.passBegun()
			evicted, err := provisioned.RunDeschedulerPass(running, policy, version)
			if running.Err() != nil {
				return
			}
			if err != nil {
				descheduling.passFailed(fmt.Errorf("run a Descheduler pass every %s under %s: %w",
					interval.Every(), policy.Name(), err))

				return
			}
			descheduling.passEnded(evicted)
		}
	})
	descheduling.stop = func() {
		stopRunning()
		ended.Wait()
	}

	return descheduling
}

func (d *deschedulerInterval) passBegun() {
	d.publish(func() { d.begun++ })
}

func (d *deschedulerInterval) passEnded(evicted bool) {
	d.publish(func() { d.evictedPerPass = append(d.evictedPerPass, evicted) })
}

func (d *deschedulerInterval) passFailed(failed error) {
	d.publish(func() {
		d.failed = failed
		close(d.failing)
	})
}

func (d *deschedulerInterval) publish(change func()) {
	d.guard.Lock()
	defer d.guard.Unlock()

	change()
	close(d.published)
	d.published = make(chan struct{})
}

func (d *deschedulerInterval) passesBegun() int {
	d.guard.Lock()
	defer d.guard.Unlock()

	return d.begun
}

// failure is nil for a Scenario running no interval, which holds none.
func (d *deschedulerInterval) failure() error {
	if d == nil {
		return nil
	}

	d.guard.Lock()
	defer d.guard.Unlock()

	return d.failed
}

// aPassFailed is closed once a pass failed, so failure holds it from then on.
// A Scenario running no interval has a nil channel, which is never closed.
func (d *deschedulerInterval) aPassFailed() <-chan struct{} {
	if d == nil {
		return nil
	}

	return d.failing
}

// awaitAPassEvictingNothingAfter ends at the first pass numbered past
// begunBefore that ended evicting nothing, so a pass already running when the
// count was read never ends it, or at the interval's failure.
func (d *deschedulerInterval) awaitAPassEvictingNothingAfter(ctx context.Context, begunBefore int) error {
	for {
		d.guard.Lock()
		reached := begunBefore < len(d.evictedPerPass) && slices.Contains(d.evictedPerPass[begunBefore:], false)
		failed, published := d.failed, d.published
		d.guard.Unlock()

		switch {
		case reached:
			return nil
		case failed != nil:
			return failed
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("no Descheduler pass begun after this Wait began has evicted nothing: %w", ctx.Err())
		case <-published:
		}
	}
}
