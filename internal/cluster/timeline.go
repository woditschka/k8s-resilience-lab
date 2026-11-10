package cluster

import (
	"context"
	"fmt"
	"iter"
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

	recorded := newRecording(stop)
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

// FollowTimeline leaves the record running and hands back the Timeline so far
// with a channel carrying each later entry, in order, until the record stops.
// The Timeline holds every Step noted before the ask, since the goroutine owning
// the record answers it in turn with the notes, so no entry is missed or handed
// twice. The owner waits on the channel's reader before it takes the next
// change, so the channel is read to its close. A cluster that is not recording
// holds an empty Timeline and hands on nothing.
func (c *SimulatedCluster) FollowTimeline() (scenario.Timeline, <-chan scenario.Entry) {
	if c.recording == nil {
		nothing := make(chan scenario.Entry)
		close(nothing)

		return scenario.Timeline{}, nothing
	}

	return c.recording.follow()
}

// NoteStep notes a Step into the Timeline, carrying its line, timed on receipt
// among the changes. A cluster that is not recording, or no cluster at all,
// notes nothing.
func (c *SimulatedCluster) NoteStep(step, line string) {
	if c == nil || c.recording == nil {
		return
	}

	c.recording.note(step, line)
}

// The Timeline is written once, before done is closed, and read once after, and
// one recorded so far is handed over on its own channel, so the record holds no
// shared state and needs no lock.
type recording struct {
	stop     context.CancelFunc
	started  time.Time
	done     chan struct{}
	notes    chan stepNoted
	asks     chan followAsked
	timeline scenario.Timeline
}

// followAsked carries room for the Timeline so far, and the channel the owner
// hands each later entry on.
type followAsked struct {
	soFar chan<- scenario.Timeline
	later chan<- scenario.Entry
}

type stepNoted struct {
	step string
	line string
}

func newRecording(stop context.CancelFunc) *recording {
	return &recording{
		stop: stop, started: time.Now(), done: make(chan struct{}),
		notes: make(chan stepNoted), asks: make(chan followAsked),
	}
}

// note returns once the owning goroutine has taken the note, so a change
// received after it stands after it. A record already stopped takes none.
func (r *recording) note(step, line string) {
	select {
	case r.notes <- stepNoted{step: step, line: line}:
	case <-r.done:
	}
}

// follow is answered with room for the Timeline so far, so the owning goroutine
// never waits on the asker for it. A record already stopped answers with its
// whole Timeline and hands on nothing.
func (r *recording) follow() (scenario.Timeline, <-chan scenario.Entry) {
	soFar, later := make(chan scenario.Timeline, 1), make(chan scenario.Entry)
	select {
	case r.asks <- followAsked{soFar: soFar, later: later}:
		return <-soFar, later
	case <-r.done:
		close(later)

		return r.timeline, later
	}
}

// own times each change and each Step's note as it reads it, rather than in the
// streams, so the order of a Timeline is the order the lab received them.
//
// One follower is handed each entry once it is recorded and written; a second
// ask closes the first follower's channel, so no reader waits on it forever.
func (r *recording) own(changes <-chan watched, into EntrySink) {
	var recorded []scenario.Entry
	var follower chan<- scenario.Entry
	for received := range r.received(changes) {
		if received.asked != nil {
			received.asked.soFar <- scenario.NewTimeline(recorded)
			if follower != nil {
				close(follower)
			}
			follower = received.asked.later

			continue
		}
		entry, carried := received.entry(time.Since(r.started))
		if !carried {
			continue
		}

		recorded = append(recorded, entry)
		into.Write(entry)
		if follower != nil {
			follower <- entry
		}
	}

	if follower != nil {
		close(follower)
	}
	r.timeline = scenario.NewTimeline(recorded)
	close(r.done)
}

// received ends with the changes, since the streams closing is what stops a
// record.
func (r *recording) received(changes <-chan watched) iter.Seq[watched] {
	return func(yield func(watched) bool) {
		for {
			select {
			case change, open := <-changes:
				if !open || !yield(change) {
					return
				}
			case noted := <-r.notes:
				if !yield(watched{step: noted}) {
					return
				}
			case asked := <-r.asks:
				if !yield(watched{asked: &asked}) {
					return
				}
			}
		}
	}
}

type watched struct {
	event watch.Event
	note  string
	step  stepNoted
	asked *followAsked
}

func (w watched) entry(elapsed time.Duration) (scenario.Entry, bool) {
	if w.step.step != "" {
		return scenario.Entry{
			Elapsed: elapsed, Change: scenario.ChangeNoted, Kind: scenario.StepObject, Name: w.step.step, Detail: w.step.line,
		}, true
	}
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
			return c.watches.CoreV1().Nodes().Watch(ctx, options)
		}},
		{named: "Pod", watch: func(ctx context.Context, options metav1.ListOptions) (watch.Interface, error) {
			return c.watches.CoreV1().Pods(clusterNamespace).Watch(ctx, options)
		}},
		{named: "Deployment", watch: func(ctx context.Context, options metav1.ListOptions) (watch.Interface, error) {
			return c.watches.AppsV1().Deployments(clusterNamespace).Watch(ctx, options)
		}},
		{named: "ReplicaSet", watch: func(ctx context.Context, options metav1.ListOptions) (watch.Interface, error) {
			return c.watches.AppsV1().ReplicaSets(clusterNamespace).Watch(ctx, options)
		}},
		{named: "Event", watch: func(ctx context.Context, options metav1.ListOptions) (watch.Interface, error) {
			return c.watches.CoreV1().Events(clusterNamespace).Watch(ctx, options)
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

// open holds the opening to controlPlaneTimeout and leaves the stream to run
// until the recording's context ends. Watch returns once the response headers
// arrive, so a deadline on its context would end the stream as well; the timer
// is stopped instead. An opening the timer cut is a failed open.
func (s *stream) open(ctx context.Context) error {
	opening, cancel := context.WithCancel(ctx)
	deadline := time.AfterFunc(controlPlaneTimeout, cancel)

	opened, err := s.watch(opening, metav1.ListOptions{ResourceVersion: s.version})
	inTime := deadline.Stop()
	if err != nil {
		cancel()

		return err
	}
	if !inTime {
		opened.Stop()

		return fmt.Errorf("the %s stream did not open within %s", s.named, controlPlaneTimeout)
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
