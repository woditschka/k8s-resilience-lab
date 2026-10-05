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

// EntrySink is where a recording delivers each entry it maps. It is declared
// here, by the side that produces the entries, so a recording names no file and
// this package imports none (docs/architecture-principles.md § Module Layout).
type EntrySink interface {
	Write(entry scenario.Entry)
}

// Record opens this cluster's five watch streams — its Nodes, its Pods, its
// Deployments, its ReplicaSets and the Events of the Workload's namespace — and
// starts the one goroutine owning the Timeline they feed, delivering every entry
// it maps to the sink given.
//
// The watch streams are what makes a Timeline complete: polling misses every
// transition between two polls, per
// docs/adr/2026-10-05-timeline-from-the-watch-stream.md. Recording starts before
// the Scenario's first Step, so the first Node of its Topology is already in the
// record.
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

// StopRecording cancels the five watches, drains each stream to its close and
// returns the complete Timeline, so a caller reads every change the lab had
// received without a wait of its own. A second call returns the same Timeline,
// and a cluster that recorded nothing returns an empty one.
func (c *SimulatedCluster) StopRecording() scenario.Timeline {
	if c.recording == nil {
		return scenario.Timeline{}
	}

	c.recording.stop()
	<-c.recording.done

	return c.recording.timeline
}

// recording is one Scenario's record as it runs: the stop that cancels its
// streams, when it started, and the Timeline the goroutine owning it leaves
// behind.
//
// That Timeline is written once, before done is closed, and read once, after it
// is closed, so the record holds no shared state and needs no lock
// (docs/adr/2026-10-04-race-detector-in-the-gate.md).
type recording struct {
	stop     context.CancelFunc
	started  time.Time
	done     chan struct{}
	timeline scenario.Timeline
}

// own is the one goroutine owning a Timeline. It times each change at the moment
// it reads it, maps it into an entry, keeps that entry and hands it to the sink.
// Timing here rather than in the streams is what makes the order of a Timeline
// the order the lab received its changes.
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

// watched is one change a stream read, waiting for the goroutine owning the
// Timeline to time and map it, or a note about the record itself that goroutine
// takes as it stands.
type watched struct {
	event watch.Event
	note  string
}

// entry is this change as one entry of the Timeline, timed at elapsed. A change
// of a kind a Timeline does not record carries nothing out.
func (w watched) entry(elapsed time.Duration) (scenario.Entry, bool) {
	if w.note != "" {
		return scenario.Entry{Elapsed: elapsed, Change: scenario.ChangeNoted, Detail: w.note}, true
	}

	return toEntry(elapsed, w.event)
}

// stream is one of the five watch streams a Timeline is recorded from: what it
// watches, how that watch is opened, the stream currently open, and the version
// the lab last saw on it.
type stream struct {
	named    string
	watch    func(ctx context.Context, options metav1.ListOptions) (watch.Interface, error)
	opened   watch.Interface
	version  string
	reopened bool
}

// openStreams opens the five streams a Timeline is recorded from. A stream it
// cannot open is an error rather than a gap: a Timeline missing a whole kind
// would explain a Placement wrongly instead of incompletely.
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

// openAllOrNone opens every stream given, and stops those already open when one
// of them cannot be opened, so a record that could not open all five leaves no
// half-open watch behind. The error names the kind that could not be opened and
// the cluster it was to be recorded from.
func openAllOrNone(ctx context.Context, recorded []*stream, cluster string) error {
	for at, opening := range recorded {
		if err := opening.open(ctx); err != nil {
			stopStreams(recorded[:at])

			return fmt.Errorf("record the %s changes of %s: %w", opening.named, cluster, err)
		}
	}

	return nil
}

// readStreams reads the streams given, each in a goroutine of its own, and hands
// back the one channel the goroutine owning the Timeline reads them through. The
// channel closes once every stream has been read to its close, which is what
// makes a stopped record complete.
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

// read reads this stream to its close, handing every change it carries to the
// goroutine owning the Timeline.
//
// A stream the API server closes is re-opened once, from the last version the
// lab saw, so a Timeline is not truncated by a closed connection. A re-open the
// lab cannot make, and a second close, are noted changes: a Timeline that is
// incomplete says so rather than reading complete
// (docs/adr/2026-10-05-timeline-from-the-watch-stream.md).
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

// drain reads the open stream to its close, remembering the version each change
// was committed at so a re-opened stream resumes from it.
func (s *stream) drain(to chan<- watched) {
	for event := range s.opened.ResultChan() {
		s.version = versionOf(event.Object, s.version)
		to <- watched{event: event}
	}
}

// open starts watching this stream's kind, from the version the lab last saw on
// it.
func (s *stream) open(ctx context.Context) error {
	opened, err := s.watch(ctx, metav1.ListOptions{ResourceVersion: s.version})
	if err != nil {
		return err
	}
	s.opened = opened

	return nil
}

// stopStreams closes the streams already open, so a record that could not open
// all five leaves none of them watching.
func stopStreams(opened []*stream) {
	for _, open := range opened {
		open.opened.Stop()
	}
}

// versionOf is the version the API server committed one watched object at, which
// a re-opened stream resumes from. An object carrying none leaves the last
// version seen in place, because resuming from nothing would replay the whole
// stream.
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
