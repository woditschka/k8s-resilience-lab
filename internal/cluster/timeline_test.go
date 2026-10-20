package cluster

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// One Node with room for a single Pod and two Pods, so the control plane leaves
// one Pending whatever it does.
const (
	theOneZone     = 1
	theOneNodeInIt = 1
	roomForOnePod  = 1
	twoPods        = 2

	theOneReplicaSet = 1
)

const (
	podScheduled        = "Scheduled"
	podSchedulingFailed = "FailedScheduling"
)

// entriesDelivered keeps the entries, so the delivery is asserted without the
// folder internal/timeline's File needs.
type entriesDelivered struct {
	entries []scenario.Entry
}

func (e *entriesDelivered) Write(entry scenario.Entry) {
	e.entries = append(e.entries, entry)
}

func aTopologyOfOneNodeHoldingOnePod(t *testing.T) scenario.Topology {
	t.Helper()

	capacity := aCapacityOf(t, roomForOnePod, someCPUPerNode, someMemoryPerNode)
	topology, err := topologyOfTheGeneralNodePool(theOneZone, theOneNodeInIt, capacity)
	if err != nil {
		t.Fatalf("build a Topology of one Node: %v", err)
	}

	return topology
}

// The recording starts before the first Node is created and stops after the
// Placement settles, the sequence the runner follows.
func aRecordedScenario(t *testing.T, topology scenario.Topology, workload scenario.Workload) (scenario.Timeline, []scenario.Entry, scenario.Placement) {
	t.Helper()

	provisioned, _ := aProvisionedCluster(t)
	delivered := &entriesDelivered{}
	if err := provisioned.Record(t.Context(), delivered); err != nil {
		t.Fatalf("record the Timeline of %s: %v", provisioned.Name(), err)
	}

	deployInto(t, provisioned, topology, workload)
	settled, err := provisioned.AwaitSettled(t.Context(), workload)
	if err != nil {
		t.Fatalf("await the Placement settling: %v", err)
	}

	return provisioned.StopRecording(), delivered.entries, settled
}

func TestAScenarioWithATopologyAndAWorkload_ShouldRecordEveryNodePodAndWorkloadChange(t *testing.T) {
	t.Parallel()
	topology, workload := anEvenSpreadTopology(t), aWorkloadDeclaring(t, ninePods, resilientPlacement(t))

	recorded, delivered, settled := aRecordedScenario(t, topology, workload)

	expect.Expect(t, namesRecorded(recorded, scenario.NodeObject)).To(expect.Equal(nodesOf(topology)))
	expect.Expect(t, namesRecorded(recorded, scenario.PodObject)).
		To(expect.Equal(everyPodNameOf(topology, podsOf(settled))))
	expect.Expect(t, namesRecorded(recorded, scenario.DeploymentObject)).
		To(expect.Equal([]string{deployedWorkloadName}))
	expect.Expect(t, namesRecorded(recorded, scenario.ReplicaSetObject)).To(expect.HaveLen[string](theOneReplicaSet))
	expect.Expect(t, delivered).To(expect.Equal(recorded.Entries()))
}

func TestARecordedTimeline_ShouldHoldItsEntriesInTheOrderTheLabReceivedThem(t *testing.T) {
	t.Parallel()
	recorded, _, _ := aRecordedScenario(t, anEvenSpreadTopology(t), aWorkloadDeclaring(t, ninePods, resilientPlacement(t)))

	expect.Expect(t, slices.IsSortedFunc(recorded.Entries(), byReceipt)).To(expect.Equal(true))
}

func TestAPlacedPodAndAPendingPod_ShouldCarryTheReasonTheControlPlaneGave(t *testing.T) {
	t.Parallel()
	workload := aWorkloadDeclaring(t, twoPods, resilientPlacement(t))

	recorded, _, settled := aRecordedScenario(t, aTopologyOfOneNodeHoldingOnePod(t), workload)

	placed, pending := settled.PodsInPhase(scenario.PodRunning), settled.PodsInPhase(scenario.PodPending)
	expect.Expect(t, placed).To(expect.HaveLen[scenario.Pod](roomForOnePod))
	expect.Expect(t, pending).To(expect.HaveLen[scenario.Pod](twoPods - roomForOnePod))
	expectReported(t, recorded, placed[0].Name, podScheduled)
	expectReported(t, recorded, pending[0].Name, podSchedulingFailed)
}

func TestAClusterAlreadyRecording_ShouldRefuseASecondRecord(t *testing.T) {
	t.Parallel()
	provisioned, _ := aProvisionedCluster(t)
	delivered := &entriesDelivered{}
	if err := provisioned.Record(t.Context(), delivered); err != nil {
		t.Fatalf("record the Timeline of %s: %v", provisioned.Name(), err)
	}
	t.Cleanup(func() { provisioned.StopRecording() })

	refused := provisioned.Record(t.Context(), delivered)

	if refused == nil {
		t.Fatalf("%s took a second record of its Timeline rather than refusing one", provisioned.Name())
	}
	expectNaming(t, refused.Error(), provisioned.Name())
}

func expectNaming(t *testing.T, written string, named ...string) {
	t.Helper()

	for _, name := range named {
		if !strings.Contains(written, name) {
			t.Errorf("the lab wrote %q, which does not name %s", written, name)
		}
	}
}

func expectReported(t *testing.T, recorded scenario.Timeline, pod, reason string) {
	t.Helper()

	reported := reasonsReportedFor(recorded, pod)
	if !slices.Contains(reported, reason) {
		t.Errorf("the Timeline carries the reason %s for the Pod %s, but reports %v", reason, pod, reported)
	}
}

func reasonsReportedFor(recorded scenario.Timeline, pod string) []string {
	var reported []string
	for _, entry := range recorded.Entries() {
		if entry.Change == scenario.ChangeReported && entry.Name == pod {
			reported = append(reported, entry.Reason)
		}
	}

	return reported
}

func namesRecorded(recorded scenario.Timeline, kind scenario.ObjectKind) []string {
	named := make(map[string]bool)
	for _, entry := range recorded.Entries() {
		if entry.Kind == kind {
			named[entry.Name] = true
		}
	}

	return slices.Sorted(maps.Keys(named))
}

func nodesOf(topology scenario.Topology) []string {
	declared := topology.Nodes()
	names := make([]string, 0, len(declared))
	for _, node := range declared {
		names = append(names, node.Name)
	}

	return slices.Sorted(slices.Values(names))
}

func podsOf(settled scenario.Placement) []string {
	observed := settled.Pods()
	names := make([]string, 0, len(observed))
	for _, pod := range observed {
		names = append(names, pod.Name)
	}

	return names
}

func everyPodNameOf(topology scenario.Topology, workloadPods []string) []string {
	named := make([]string, 0, len(workloadPods))
	for _, node := range topology.Nodes() {
		named = append(named, probePodNamePrefix+node.Name)
	}

	return slices.Sorted(slices.Values(append(named, workloadPods...)))
}

func byReceipt(one, next scenario.Entry) int {
	return cmp.Compare(one.Elapsed, next.Elapsed)
}

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

	entry, carried := noted.entry(observedAfter)

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
