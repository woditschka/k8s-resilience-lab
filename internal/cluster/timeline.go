package cluster

import (
	"context"
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// EntrySink is declared here, by the side producing the entries, so a recording
// names no file and this package imports none.
type EntrySink interface {
	Write(entry scenario.Entry)
}

// Record opens the cluster's five watch streams and starts the one goroutine
// owning the Timeline they feed. Watching rather than polling is what makes a
// Timeline complete, since polling misses every transition between two polls.
func (c *SimulatedCluster) Record(ctx context.Context, into EntrySink) error {
	if c.recording != nil {
		return fmt.Errorf("%s is already recording its Timeline", c.Name())
	}

	watching, stop := context.WithCancel(ctx)
	opened, err := c.openStreams(watching)
	if err != nil {
		stop()

		return err
	}

	recorded := &recording{stop: stop, started: time.Now(), done: make(chan struct{})}
	go recorded.own(readStreams(watching, opened), into)
	c.recording = recorded

	return nil
}

// StopRecording drains each stream to its close, so a caller reads every change
// received without a wait of its own. A second call returns the same Timeline.
func (c *SimulatedCluster) StopRecording() scenario.Timeline {
	if c.recording == nil {
		return scenario.Timeline{}
	}

	c.recording.stop()
	<-c.recording.done

	return c.recording.timeline
}

// The Timeline is written once, before done is closed, and read once after, so
// the record holds no shared state and needs no lock.
type recording struct {
	stop     context.CancelFunc
	started  time.Time
	done     chan struct{}
	timeline scenario.Timeline
}

// own times each change as it reads it, rather than in the streams, so the
// order of a Timeline is the order the lab received its changes.
func (r *recording) own(changes <-chan watched, into EntrySink) {
	var recorded []scenario.Entry
	for change := range changes {
		entry, carried := change.entry(time.Since(r.started))
		if !carried {
			continue
		}

		recorded = append(recorded, entry)
		into.Write(entry)
	}

	r.timeline = scenario.NewTimeline(recorded)
	close(r.done)
}

type watched struct {
	event watch.Event
	note  string
}

func (w watched) entry(elapsed time.Duration) (scenario.Entry, bool) {
	if w.note != "" {
		return scenario.Entry{Elapsed: elapsed, Change: scenario.ChangeNoted, Detail: w.note}, true
	}

	return toEntry(elapsed, w.event)
}

type stream struct {
	named    string
	watch    func(ctx context.Context, options metav1.ListOptions) (watch.Interface, error)
	opened   watch.Interface
	version  string
	reopened bool
}

// A stream that cannot be opened is an error rather than a gap: a Timeline
// missing a whole kind would explain a Placement wrongly instead of
// incompletely.
func (c *SimulatedCluster) openStreams(ctx context.Context) ([]*stream, error) {
	recorded := []*stream{
		{named: "Node", watch: func(ctx context.Context, options metav1.ListOptions) (watch.Interface, error) {
			return c.client.CoreV1().Nodes().Watch(ctx, options)
		}},
		{named: "Pod", watch: func(ctx context.Context, options metav1.ListOptions) (watch.Interface, error) {
			return c.client.CoreV1().Pods(workloadNamespace).Watch(ctx, options)
		}},
		{named: "Deployment", watch: func(ctx context.Context, options metav1.ListOptions) (watch.Interface, error) {
			return c.client.AppsV1().Deployments(workloadNamespace).Watch(ctx, options)
		}},
		{named: "ReplicaSet", watch: func(ctx context.Context, options metav1.ListOptions) (watch.Interface, error) {
			return c.client.AppsV1().ReplicaSets(workloadNamespace).Watch(ctx, options)
		}},
		{named: "Event", watch: func(ctx context.Context, options metav1.ListOptions) (watch.Interface, error) {
			return c.client.CoreV1().Events(workloadNamespace).Watch(ctx, options)
		}},
	}

	if err := openAllOrNone(ctx, recorded, c.Name()); err != nil {
		return nil, err
	}

	return recorded, nil
}

// openAllOrNone stops the streams already open when one fails, so no half-open
// watch is left behind.
func openAllOrNone(ctx context.Context, recorded []*stream, cluster string) error {
	for at, opening := range recorded {
		if err := opening.open(ctx); err != nil {
			stopStreams(recorded[:at])

			return fmt.Errorf("record the %s changes of %s: %w", opening.named, cluster, err)
		}
	}

	return nil
}

// The channel closes once every stream has been read to its close, which is
// what makes a stopped record complete.
func readStreams(ctx context.Context, opened []*stream) <-chan watched {
	changes := make(chan watched)

	var reading sync.WaitGroup
	for _, open := range opened {
		reading.Go(func() { open.read(ctx, changes) })
	}
	go func() {
		reading.Wait()
		close(changes)
	}()

	return changes
}

// A stream the API server closes is re-opened once, from the last version seen,
// so a closed connection does not truncate the Timeline. A failed re-open, or a
// second close, is noted on the Timeline, so an incomplete record says so.
func (s *stream) read(ctx context.Context, to chan<- watched) {
	for {
		s.drain(to)
		s.opened.Stop()

		if ctx.Err() != nil {
			return
		}
		if s.reopened {
			to <- watched{note: fmt.Sprintf("the %s stream closed a second time, so this Timeline holds no later %s change", s.named, s.named)}

			return
		}

		s.reopened = true
		if err := s.open(ctx); err != nil {
			to <- watched{note: fmt.Sprintf("the %s stream closed and could not be re-opened, so this Timeline holds no later %s change: %v", s.named, s.named, err)}

			return
		}
	}
}

func (s *stream) drain(to chan<- watched) {
	for event := range s.opened.ResultChan() {
		s.version = versionOf(event.Object, s.version)
		to <- watched{event: event}
	}
}

func (s *stream) open(ctx context.Context) error {
	opened, err := s.watch(ctx, metav1.ListOptions{ResourceVersion: s.version})
	if err != nil {
		return err
	}
	s.opened = opened

	return nil
}

func stopStreams(opened []*stream) {
	for _, open := range opened {
		open.opened.Stop()
	}
}

// An object carrying no version leaves the last one seen in place, because
// resuming from nothing would replay the whole stream.
func versionOf(object runtime.Object, lastSeen string) string {
	accessed, err := meta.Accessor(object)
	if err != nil {
		return lastSeen
	}
	if version := accessed.GetResourceVersion(); version != "" {
		return version
	}

	return lastSeen
}
