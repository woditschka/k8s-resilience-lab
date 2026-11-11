//go:build !integration

package cluster

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// Which kinds these are does not matter to the paths below, only that a note
// and a refusal name them.
const (
	theKindWatched     = "Pod"
	theKindOpenedFirst = "Node"
)

const (
	theVersionLastSeen = "88142"

	noVersionYet = ""

	theRefusalTheServerGave = "the API server refused the watch"

	theSecondCloseNamed = "a second time"

	theClusterRecorded = "lab-resilient-placement"

	noVersionCarried = ""

	theNoteTakenAboutTheRecord = "the Pod stream closed and could not be re-opened"

	theOneNote = 1
	noChange   = 0
)

// aStreamConnection is the lab's one stand-in for a system boundary. It is
// hand-written so a test can close it, which a real KWOK control plane does not
// do on demand.
type aStreamConnection struct {
	changes chan watch.Event
	closing sync.Once
	stopped bool
}

func (c *aStreamConnection) ResultChan() <-chan watch.Event {
	return c.changes
}

// Stop tolerates a second call, since a test closes the same connection to
// stand in for the API server closing it.
func (c *aStreamConnection) Stop() {
	c.stopped = true
	c.closes()
}

func (c *aStreamConnection) closes() {
	c.closing.Do(func() { close(c.changes) })
}

func aConnectionCarrying(changes ...watch.Event) *aStreamConnection {
	carrying := &aStreamConnection{changes: make(chan watch.Event, len(changes))}
	for _, change := range changes {
		carrying.changes <- change
	}
	carrying.closes()

	return carrying
}

func anAlreadyClosedConnection() *aStreamConnection {
	return aConnectionCarrying()
}

func anOpenConnection() *aStreamConnection {
	return &aStreamConnection{changes: make(chan watch.Event)}
}

type anAnswer struct {
	connection *aStreamConnection
	refusal    error
}

func answeredWith(connection *aStreamConnection) anAnswer {
	return anAnswer{connection: connection}
}

func refusedWith(refusal error) anAnswer {
	return anAnswer{refusal: refusal}
}

// aWatchAnswering keeps the version each open asked to resume from, so a test
// reads what a re-open carried.
type aWatchAnswering struct {
	answers []anAnswer
	resumed []string
}

// An open past the answers given is the test's own mistake, so it is refused
// saying so.
func (w *aWatchAnswering) open(_ context.Context, options metav1.ListOptions) (watch.Interface, error) {
	w.resumed = append(w.resumed, options.ResourceVersion)
	if len(w.resumed) > len(w.answers) {
		return nil, fmt.Errorf("the stream was opened %d times, and this test gave %d answers", len(w.resumed), len(w.answers))
	}

	answered := w.answers[len(w.resumed)-1]
	if answered.refusal != nil {
		return nil, answered.refusal
	}

	return answered.connection, nil
}

func aStreamWatching(kind string, answers ...anAnswer) (*stream, *aWatchAnswering) {
	watching := &aWatchAnswering{answers: answers}

	return &stream{named: kind, watch: watching.open}, watching
}

func aStreamOpened(t *testing.T, ctx context.Context, kind string, answers ...anAnswer) (*stream, *aWatchAnswering) {
	t.Helper()

	reading, watching := aStreamWatching(kind, answers...)
	if err := reading.open(ctx); err != nil {
		t.Fatalf("open the %s stream: %v", kind, err)
	}

	return reading, watching
}

// changesRead hands the changes back once the read has ended, so a test asserts
// a complete read without a wait of its own.
func changesRead(t *testing.T, ctx context.Context, reading *stream) []watched {
	t.Helper()

	changes := make(chan watched)
	go func() {
		reading.read(ctx, changes)
		close(changes)
	}()

	var carried []watched
	for change := range changes {
		carried = append(carried, change)
	}

	return carried
}

func aPodChangedAt(version string) watch.Event {
	return watch.Event{Type: watch.Modified, Object: &corev1.Pod{
		Name: watchedPodName, ResourceVersion: version,
	}}
}

func TestAClosedStream_ShouldBeReopenedOnceFromTheLastVersionSeen(t *testing.T) {
	reading, watching := aStreamOpened(t, t.Context(), theKindWatched,
		answeredWith(aConnectionCarrying(aPodChangedAt(theVersionLastSeen))),
		answeredWith(anAlreadyClosedConnection()))

	changesRead(t, t.Context(), reading)

	expect.Expect(t, watching.resumed).To(expect.Equal([]string{noVersionYet, theVersionLastSeen}))
}

func TestAChangeCarryingNoVersion_ShouldLeaveTheReopenResumingFromTheLastVersionSeen(t *testing.T) {
	reading, watching := aStreamOpened(t, t.Context(), theKindWatched,
		answeredWith(aConnectionCarrying(aPodChangedAt(theVersionLastSeen), aPodChangedAt(noVersionCarried))),
		answeredWith(anAlreadyClosedConnection()))

	changesRead(t, t.Context(), reading)

	expect.Expect(t, watching.resumed).To(expect.Equal([]string{noVersionYet, theVersionLastSeen}))
}

func TestANoteAboutTheRecord_ShouldBecomeOneNotedEntryCarryingIt(t *testing.T) {
	noted := watched{note: theNoteTakenAboutTheRecord}

	entry, carried := noted.entry(observedAfter, newEntryMapper())

	expect.Expect(t, carried).To(expect.Equal(true))
	expect.Expect(t, entry).To(expect.Equal(scenario.Entry{
		Elapsed: observedAfter, Change: scenario.ChangeNoted, Detail: theNoteTakenAboutTheRecord,
	}))
}

func TestAStreamTheLabCannotReopen_ShouldBeNotedWithItsKindAndTheReason(t *testing.T) {
	reading, _ := aStreamOpened(t, t.Context(), theKindWatched,
		answeredWith(anAlreadyClosedConnection()),
		refusedWith(errors.New(theRefusalTheServerGave)))

	carried := changesRead(t, t.Context(), reading)

	expect.Expect(t, carried).To(expect.HaveLen[watched](theOneNote))
	expectNaming(t, carried[0].note, theKindWatched, theRefusalTheServerGave)
}

func TestAStreamThatClosesASecondTime_ShouldBeNotedAndEndTheRead(t *testing.T) {
	reading, _ := aStreamOpened(t, t.Context(), theKindWatched,
		answeredWith(anAlreadyClosedConnection()),
		answeredWith(anAlreadyClosedConnection()))

	carried := changesRead(t, t.Context(), reading)

	expect.Expect(t, carried).To(expect.HaveLen[watched](theOneNote))
	expectNaming(t, carried[0].note, theKindWatched, theSecondCloseNamed)
}

func TestARecordingWhoseContextIsCancelled_ShouldEndTheReadWithNoNote(t *testing.T) {
	watching, stop := context.WithCancel(t.Context())
	connection := anOpenConnection()
	reading, _ := aStreamOpened(t, watching, theKindWatched, answeredWith(connection))

	stop()
	// The API server closes a watch whose context is cancelled.
	connection.closes()
	carried := changesRead(t, watching, reading)

	expect.Expect(t, carried).To(expect.HaveLen[watched](noChange))
}

func TestAStreamTheLabCannotOpen_ShouldStopThoseAlreadyOpenAndNameTheKindAndTheCluster(t *testing.T) {
	alreadyOpen := anOpenConnection()
	openedFirst, _ := aStreamWatching(theKindOpenedFirst, answeredWith(alreadyOpen))
	refused, _ := aStreamWatching(theKindWatched, refusedWith(errors.New(theRefusalTheServerGave)))

	err := openAllOrNone(t.Context(), []*stream{openedFirst, refused}, theClusterRecorded)

	if err == nil {
		t.Fatalf("the %s stream could not be opened, and the record started regardless", theKindWatched)
	}
	expectNaming(t, err.Error(), theKindWatched, theClusterRecorded, theRefusalTheServerGave)
	expect.Expect(t, alreadyOpen.stopped).To(expect.Equal(true))
}

const (
	aLaterVersion = "88143"
	theStepNoted  = scenario.WaitStepNamed
	theLineNoted  = "Wait for all Pods Ready, reached in 3s"
)

// Elapsed times are the clock's, so an entry is compared without its own.
func withoutTheirTimes(entries []scenario.Entry) []scenario.Entry {
	timeless := slices.Clone(entries)
	for at := range timeless {
		timeless[at].Elapsed = 0
	}

	return timeless
}

func theStepNoteRecorded() scenario.Entry {
	return scenario.Entry{Change: scenario.ChangeNoted, Kind: scenario.StepObject, Name: theStepNoted, Detail: theLineNoted}
}

func aPodChangeRecorded() scenario.Entry {
	return scenario.Entry{Change: scenario.ChangeModified, Kind: scenario.PodObject, Name: watchedPodName, Reason: string(scenario.PodUnknown)}
}

// Each send below returns only once the owning goroutine has taken it, so the
// order sent is the order received.
func TestATimeline_ShouldRecordEachStepAtItsMoment(t *testing.T) {
	changes := make(chan watched)
	recorded := newRecording(t.Context(), func() {})
	go recorded.own(changes, &entriesDelivered{})

	changes <- watched{event: aPodChangedAt(theVersionLastSeen)}
	recorded.note(theStepNoted, theLineNoted)
	changes <- watched{event: aPodChangedAt(aLaterVersion)}
	close(changes)
	<-recorded.done

	expect.Expect(t, withoutTheirTimes(recorded.timeline.Entries())).To(expect.Equal([]scenario.Entry{
		aPodChangeRecorded(), theStepNoteRecorded(), aPodChangeRecorded(),
	}))
}

const theLineGivenOnceHeld = "Not injected: Node zone-1-node-1 stops responding, because the Node is gone"

func theHeldNoteGiven() scenario.Entry {
	return scenario.Entry{Change: scenario.ChangeNoted, Kind: scenario.StepObject, Name: scenario.NotInjectedNamed, Detail: theLineGivenOnceHeld}
}

// The change received while the note is held is what the click's own first
// change stands for, and the follower reads entries only once the note is
// given, so its first entry read is the note.
func TestAStepNoteHeldOpen_ShouldStandAtItsOpeningAheadOfTheEntriesHeldBackForTheFileAndTheFollowerAlike(t *testing.T) {
	changes := make(chan watched)
	delivered := &entriesDelivered{}
	recorded := newRecording(t.Context(), func() {})
	go recorded.own(changes, delivered)
	_, later := recorded.follow()
	held := recorded.hold()
	changes <- watched{event: aPodChangedAt(theVersionLastSeen)}

	go held.Note(scenario.NotInjectedNamed, theLineGivenOnceHeld)

	handedOn := []scenario.Entry{<-later, <-later}
	close(changes)
	<-recorded.done
	expect.Expect(t, filedAndHandedOn{
		Filed: withoutTheirTimes(delivered.entries), HandedOn: withoutTheirTimes(handedOn),
	}).To(expect.Equal(filedAndHandedOn{
		Filed:    []scenario.Entry{theHeldNoteGiven(), aPodChangeRecorded()},
		HandedOn: []scenario.Entry{theHeldNoteGiven(), aPodChangeRecorded()},
	}))
}

// filedAndHandedOn is what a record wrote into its sink and handed its
// follower.
type filedAndHandedOn struct {
	Filed    []scenario.Entry
	HandedOn []scenario.Entry
}

// The follower takes the note and then stops reading, as a stalled terminal
// does, so the change held back waits on it; the test takes that change only
// once the note has returned, then stops the record.
func TestAStepNoteGivenOnceHeld_ShouldReturnOnceItsOwnEntryIsHandedOnWhileTheEntriesHeldBackWait(t *testing.T) {
	changes := make(chan watched)
	recorded := newRecording(t.Context(), func() {})
	go recorded.own(changes, &entriesDelivered{})
	_, later := recorded.follow()
	held := recorded.hold()
	changes <- watched{event: aPodChangedAt(theVersionLastSeen)}
	given := make(chan struct{})

	go func() {
		held.Note(scenario.NotInjectedNamed, theLineGivenOnceHeld)
		close(given)
	}()

	handedOnFirst := <-later
	<-given
	<-later
	close(changes)
	<-recorded.done
	expect.Expect(t, withoutTheirTimes([]scenario.Entry{handedOnFirst})).To(expect.Equal([]scenario.Entry{theHeldNoteGiven()}))
}

// The record stops before the note is given, as it would for a click that
// never returned; the change held back is recorded all the same.
func TestARecordStoppedWhileAStepNoteIsHeldOpen_ShouldStillRecordEveryEntryHeldBack(t *testing.T) {
	changes := make(chan watched)
	recorded := newRecording(t.Context(), func() {})
	go recorded.own(changes, &entriesDelivered{})
	held := recorded.hold()
	changes <- watched{event: aPodChangedAt(theVersionLastSeen)}

	close(changes)
	<-recorded.done
	held.Note(scenario.NotInjectedNamed, theLineGivenOnceHeld)

	expect.Expect(t, withoutTheirTimes(recorded.timeline.Entries())).To(expect.Equal([]scenario.Entry{aPodChangeRecorded()}))
}

// followedToTheStop is what a follower of a record reads: the Timeline so far,
// each entry handed on after it, and whether its channel was still open once
// the record was done.
type followedToTheStop struct {
	SoFar        []scenario.Entry
	HandedOn     []scenario.Entry
	OpenWhenDone bool
}

// The note returns once the owning goroutine has taken it, so the ask after it
// meets it whether or not the owner has delivered it yet. The owner hands the
// later change on before it takes the next, so the test reads it before it
// closes the streams.
func TestATimelineFollowed_ShouldHoldEveryStepNotedBeforeTheAskAndHandOnEachLaterEntryUntilTheRecordStops(t *testing.T) {
	changes := make(chan watched)
	recorded := newRecording(t.Context(), func() {})
	go recorded.own(changes, &entriesDelivered{})
	changes <- watched{event: aPodChangedAt(theVersionLastSeen)}
	recorded.note(theStepNoted, theLineNoted)

	soFar, later := recorded.follow()

	changes <- watched{event: aPodChangedAt(aLaterVersion)}
	handedOn := []scenario.Entry{<-later}
	close(changes)
	<-recorded.done
	_, open := <-later
	expect.Expect(t, followedToTheStop{
		SoFar: withoutTheirTimes(soFar.Entries()), HandedOn: withoutTheirTimes(handedOn), OpenWhenDone: open,
	}).To(expect.Equal(followedToTheStop{
		SoFar:    []scenario.Entry{aPodChangeRecorded(), theStepNoteRecorded()},
		HandedOn: []scenario.Entry{aPodChangeRecorded()},
	}))
}

// The follower's channel is never read, so the owner is held on its first
// change until the lab stops; the note and the change after the stop are taken
// all the same.
func TestATimelineFollowedByAReaderThatStopsTakingEntries_ShouldStillStopRecordingAndHoldEveryEntry(t *testing.T) {
	changes := make(chan watched)
	following, theLabStops := context.WithCancel(t.Context())
	recorded := newRecording(following, func() {})
	go recorded.own(changes, &entriesDelivered{})
	recorded.follow()

	changes <- watched{event: aPodChangedAt(theVersionLastSeen)}
	theLabStops()
	recorded.note(theStepNoted, theLineNoted)
	changes <- watched{event: aPodChangedAt(aLaterVersion)}
	close(changes)
	<-recorded.done

	expect.Expect(t, withoutTheirTimes(recorded.timeline.Entries())).To(expect.Equal([]scenario.Entry{
		aPodChangeRecorded(), theStepNoteRecorded(), aPodChangeRecorded(),
	}))
}

func TestATimelineFollowedASecondTime_ShouldCloseTheFirstFollowersChannel(t *testing.T) {
	changes := make(chan watched)
	recorded := newRecording(t.Context(), func() {})
	go recorded.own(changes, &entriesDelivered{})
	t.Cleanup(func() { close(changes) })
	_, first := recorded.follow()

	recorded.follow()

	_, open := <-first
	expect.Expect(t, open).To(expect.Equal(false))
}

func TestATimelineFollowedOnceStopped_ShouldHoldTheWholeTimelineAndHandOnNothing(t *testing.T) {
	changes := make(chan watched)
	recorded := newRecording(t.Context(), func() {})
	go recorded.own(changes, &entriesDelivered{})
	changes <- watched{event: aPodChangedAt(theVersionLastSeen)}
	close(changes)
	<-recorded.done

	soFar, later := recorded.follow()

	_, open := <-later
	expect.Expect(t, soFar.Entries()).To(expect.Equal(recorded.timeline.Entries()))
	expect.Expect(t, open).To(expect.Equal(false))
}

func TestAClusterNotRecording_ShouldBeFollowedAsAnEmptyTimelineHandingOnNothing(t *testing.T) {
	notRecording := &SimulatedCluster{}

	soFar, later := notRecording.FollowTimeline()

	_, open := <-later
	expect.Expect(t, soFar.Entries()).To(expect.HaveLen[scenario.Entry](noChange))
	expect.Expect(t, open).To(expect.Equal(false))
}

func TestAStepNotedAfterTheRecordStopped_ShouldBeNotedNowhere(t *testing.T) {
	changes := make(chan watched)
	recorded := newRecording(t.Context(), func() {})
	go recorded.own(changes, &entriesDelivered{})
	close(changes)
	<-recorded.done

	recorded.note(theStepNoted, theLineNoted)

	expect.Expect(t, recorded.timeline.Entries()).To(expect.HaveLen[scenario.Entry](noChange))
}

func TestAStepNotedOnAClusterNotRecording_ShouldBeNotedNowhere(t *testing.T) {
	var noCluster *SimulatedCluster
	notRecording := &SimulatedCluster{}

	noCluster.NoteStep(theStepNoted, theLineNoted)
	notRecording.NoteStep(theStepNoted, theLineNoted)

	expect.Expect(t, notRecording.StopRecording().Entries()).To(expect.HaveLen[scenario.Entry](noChange))
}

func withoutElapsed(entries []scenario.Entry) []scenario.Entry {
	untimed := slices.Clone(entries)
	for i := range untimed {
		untimed[i].Elapsed = 0
	}

	return untimed
}

func TestAPodTheDeschedulerMarkedThatChangesTwiceThenIsRemoved_ShouldBeRecordedAsOneEvictionThenModifiedThenRemoved(t *testing.T) {
	t.Parallel()
	marked := aPodMarkedADisruptionTarget(theEvictionAPIsReason)
	changes := make(chan watched, 3)
	changes <- watched{event: watch.Event{Type: watch.Modified, Object: marked}}
	changes <- watched{event: watch.Event{Type: watch.Modified, Object: marked}}
	changes <- watched{event: watch.Event{Type: watch.Deleted, Object: marked}}
	close(changes)
	delivered := &entriesDelivered{}
	recorded := newRecording(t.Context(), func() {})

	recorded.own(changes, delivered)

	untimed := func(change scenario.Change, reason string) scenario.Entry {
		return scenario.Entry{
			Change: change, Kind: scenario.PodObject, Name: watchedPodName, Reason: reason, Detail: watchedNodeName,
		}
	}
	expect.Expect(t, withoutElapsed(recorded.timeline.Entries())).To(expect.Equal([]scenario.Entry{
		untimed(scenario.ChangeReported, scenario.EvictedByTheDescheduler),
		untimed(scenario.ChangeModified, string(scenario.PodRunning)),
		untimed(scenario.ChangeRemoved, string(scenario.PodRunning)),
	}))
	expect.Expect(t, delivered.entries).To(expect.Equal(recorded.timeline.Entries()))
}
