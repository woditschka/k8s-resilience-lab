package cluster

import (
	"context"
	"fmt"
	"iter"
	"slices"
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

	recorded := newRecording(ctx, stop)
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
// change, until the context Record was given ends: the channel then closes,
// and every later entry is recorded and handed to no one, so a reader that
// stopped reading holds up no stop. A cluster that is not recording holds an
// empty Timeline and hands on nothing.
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

// HoldStepNote opens a Step's note timed now among the changes, for a Step whose
// line is known only once its changes are made. Every entry received after it
// is held back from the sink and the follower alike until the note is given. A
// cluster that is not recording, or no cluster at all, holds nothing.
func (c *SimulatedCluster) HoldStepNote() StepNoteHeld {
	if c == nil || c.recording == nil {
		return StepNoteHeld{}
	}

	return c.recording.hold()
}

// StepNoteHeld is a Step's note held open in a recording, its zero value one
// held in none.
type StepNoteHeld struct {
	recording *recording
}

// Note records the note held open as the Step given, at the moment it was
// opened and ahead of every entry held back, and returns once handed on as
// NoteStep does. A record stopped meanwhile has recorded the entries held back
// without it.
func (h StepNoteHeld) Note(step, line string) {
	if h.recording == nil {
		return
	}

	h.recording.give(step, line)
}

// The Timeline is written once, before done is closed, and read once after, and
// one recorded so far is handed over on its own channel, so the record holds no
// shared state and needs no lock.
type recording struct {
	// following ends the hand-over to the follower, and nothing else: the record
	// runs on until its streams close.
	following context.Context
	stop      context.CancelFunc
	started   time.Time
	done      chan struct{}
	notes     chan stepNoted
	asks      chan followAsked
	// holds opens a note held open, and given gives it; a note is given only
	// once its opening was taken, so the two never cross.
	holds    chan struct{}
	given    chan stepNoted
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
	// handedOn is closed once the note's entry is with the follower, or with no
	// one where none follows.
	handedOn chan struct{}
}

func newRecording(following context.Context, stop context.CancelFunc) *recording {
	return &recording{
		following: following, stop: stop, started: time.Now(), done: make(chan struct{}),
		notes: make(chan stepNoted), asks: make(chan followAsked), holds: make(chan struct{}), given: make(chan stepNoted),
	}
}

// hold returns once the owning goroutine has taken the opening, so a change
// received after it is held back. A record already stopped holds nothing.
func (r *recording) hold() StepNoteHeld {
	select {
	case r.holds <- struct{}{}:
		return StepNoteHeld{recording: r}
	case <-r.done:
		return StepNoteHeld{}
	}
}

func (r *recording) give(step, line string) {
	handedOn := make(chan struct{})
	select {
	case r.given <- stepNoted{step: step, line: line, handedOn: handedOn}:
		<-handedOn
	case <-r.done:
	}
}

// note returns once the owning goroutine has taken the note, so a change
// received after it stands after it, and has handed its entry on, so a lab
// stopping after the note still hands its line to the terminal. A record
// already stopped takes none.
func (r *recording) note(step, line string) {
	handedOn := make(chan struct{})
	select {
	case r.notes <- stepNoted{step: step, line: line, handedOn: handedOn}:
		<-handedOn
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
// Entries held back for a note held open are released when the record stops,
// so a note never given loses none.
func (r *recording) own(changes <-chan watched, into EntrySink) {
	owned := &ownedRecord{recording: r, into: into, mapper: newEntryMapper()}
	for received := range r.received(changes) {
		owned.take(received, time.Since(r.started))
	}
	owned.release()

	closeFollower(owned.follower)
	r.timeline = scenario.NewTimeline(owned.recorded)
	close(r.done)
}

// ownedRecord is what the owning goroutine alone reads and writes.
type ownedRecord struct {
	recording *recording
	into      EntrySink
	mapper    *entryMapper
	recorded  []scenario.Entry
	follower  chan<- scenario.Entry
	// held is nil unless a note is held open.
	held *heldBack
}

// heldBack is a note held open: the moment it was opened, and the entries
// received since, in order.
type heldBack struct {
	at      time.Duration
	entries []entryHeld
}

// entryHeld carries a Step note's hand-over beside its entry, nil for any other
// entry.
type entryHeld struct {
	entry    scenario.Entry
	handedOn chan struct{}
}

// A Step's note is always carried, so an entry not carried has no hand-over to
// close.
func (o *ownedRecord) take(received watched, elapsed time.Duration) {
	switch {
	case received.asked != nil:
		o.answer(*received.asked)
	case received.holds:
		o.hold(elapsed)
	case received.gives:
		o.give(received.step, elapsed)
	default:
		if entry, carried := received.entry(elapsed, o.mapper); carried {
			o.deliver(entryHeld{entry: entry, handedOn: received.step.handedOn})
		}
	}
}

func (o *ownedRecord) answer(asked followAsked) {
	asked.soFar <- scenario.NewTimeline(o.recorded)
	closeFollower(o.follower)
	o.follower = o.recording.followedFrom(asked.later)
}

// hold keeps a note already held open, and the entries it holds back, where a
// second is opened before the first is given.
func (o *ownedRecord) hold(elapsed time.Duration) {
	if o.held == nil {
		o.held = &heldBack{at: elapsed}
	}
}

// give stands the note ahead of what was held back, at the moment it was held
// open, and releases them after it. A note given with none held open stands at
// its receipt.
func (o *ownedRecord) give(noted stepNoted, elapsed time.Duration) {
	o.hold(elapsed)
	entry, _ := watched{step: noted}.entry(o.held.at, o.mapper)
	o.held.entries = slices.Insert(o.held.entries, 0, entryHeld{entry: entry, handedOn: noted.handedOn})
	o.release()
}

func (o *ownedRecord) release() {
	held := o.held
	if held == nil {
		return
	}

	o.held = nil
	for _, entry := range held.entries {
		o.deliver(entry)
	}
}

// deliver closes a Step note's hand-over once its own entry is handed on, so
// the note returns while a reader that stopped taking entries still holds the
// ones after it.
func (o *ownedRecord) deliver(delivered entryHeld) {
	if o.held != nil {
		o.held.entries = append(o.held.entries, delivered)

		return
	}

	o.recorded = append(o.recorded, delivered.entry)
	o.into.Write(delivered.entry)
	o.follower = o.recording.handOn(o.follower, delivered.entry)
	if delivered.handedOn != nil {
		close(delivered.handedOn)
	}
}

// followedFrom hands on nothing to a follower asking once the lab stopped
// following.
func (r *recording) followedFrom(later chan<- scenario.Entry) chan<- scenario.Entry {
	if r.following.Err() != nil {
		close(later)

		return nil
	}

	return later
}

// handOn waits on the follower until the lab stops following, then closes its
// channel and hands on nothing more, so a reader that stopped taking entries
// holds up the record only while the lab runs. The entry stays recorded.
func (r *recording) handOn(follower chan<- scenario.Entry, entry scenario.Entry) chan<- scenario.Entry {
	if follower == nil {
		return nil
	}
	if r.following.Err() == nil {
		select {
		case follower <- entry:
			return follower
		case <-r.following.Done():
		}
	}
	close(follower)

	return nil
}

func closeFollower(follower chan<- scenario.Entry) {
	if follower != nil {
		close(follower)
	}
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
			case <-r.holds:
				if !yield(watched{holds: true}) {
					return
				}
			case given := <-r.given:
				if !yield(watched{step: given, gives: true}) {
					return
				}
			}
		}
	}
}

// watched is one thing the owning goroutine receives: a change, a note about
// the record, a Step's note, an ask to follow, or a note held open or given.
type watched struct {
	event watch.Event
	note  string
	step  stepNoted
	asked *followAsked
	holds bool
	gives bool
}

func (w watched) entry(elapsed time.Duration, mapper *entryMapper) (scenario.Entry, bool) {
	if w.step.step != "" {
		return scenario.Entry{
			Elapsed: elapsed, Change: scenario.ChangeNoted, Kind: scenario.StepObject, Name: w.step.step, Detail: w.step.line,
		}, true
	}
	if w.note != "" {
		return scenario.Entry{Elapsed: elapsed, Change: scenario.ChangeNoted, Detail: w.note}, true
	}

	return mapper.toEntry(elapsed, w.event)
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
