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

// The smallest Topology there is, and a Workload too large for it: one Zone of
// one Node with room for a single Pod, and two Pods, so the control plane places
// one of them and leaves the other Pending whatever it does.
const (
	theOneZone     = 1
	theOneNodeInIt = 1
	roomForOnePod  = 1
	twoPods        = 2

	// theOneReplicaSet is what a Deployment of one Workload creates, and so how
	// many ReplicaSets a Timeline of one Scenario names.
	theOneReplicaSet = 1
)

// The reasons the control plane gives for the two Pods: one it placed on a Node,
// and one it could not place.
const (
	podScheduled        = "Scheduled"
	podSchedulingFailed = "FailedScheduling"
)

// entriesDelivered is the sink a recording below delivers its entries to. It
// keeps them, which is what a filed Timeline is written from: internal/timeline's
// File is the other implementation of the same one method, and this one needs no
// folder to assert the delivery through.
type entriesDelivered struct {
	entries []scenario.Entry
}

func (e *entriesDelivered) Write(entry scenario.Entry) {
	e.entries = append(e.entries, entry)
}

// aTopologyOfOneNodeHoldingOnePod is the Topology a Workload of two Pods cannot
// fit into.
func aTopologyOfOneNodeHoldingOnePod(t *testing.T) scenario.Topology {
	t.Helper()

	capacity := aCapacityOf(t, roomForOnePod, someCPUPerNode, someMemoryPerNode)
	topology, err := scenario.NewTopology(theOneZone, theOneNodeInIt, capacity)
	if err != nil {
		t.Fatalf("build a Topology of one Node: %v", err)
	}

	return topology
}

// aRecordedScenario runs one Scenario on a Simulated Cluster with its Timeline
// recording from before the first Node is created until after the Placement has
// settled, which is the sequence the runner follows. It hands back the Timeline,
// the entries the sink was given and the Settled Placement.
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
	topology, workload := anEvenSpreadTopology(t), aWorkloadDeclaring(t, ninePods, resilientPlacement())

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
	recorded, _, _ := aRecordedScenario(t, anEvenSpreadTopology(t), aWorkloadDeclaring(t, ninePods, resilientPlacement()))

	expect.Expect(t, slices.IsSortedFunc(recorded.Entries(), byReceipt)).To(expect.Equal(true))
}

func TestAPlacedPodAndAPendingPod_ShouldCarryTheReasonTheControlPlaneGave(t *testing.T) {
	workload := aWorkloadDeclaring(t, twoPods, resilientPlacement())

	recorded, _, settled := aRecordedScenario(t, aTopologyOfOneNodeHoldingOnePod(t), workload)

	placed, pending := settled.PodsInPhase(scenario.PodRunning), settled.PodsInPhase(scenario.PodPending)
	expect.Expect(t, placed).To(expect.HaveLen[scenario.Pod](roomForOnePod))
	expect.Expect(t, pending).To(expect.HaveLen[scenario.Pod](twoPods - roomForOnePod))
	expectReported(t, recorded, placed[0].Name, podScheduled)
	expectReported(t, recorded, pending[0].Name, podSchedulingFailed)
}

func TestAClusterAlreadyRecording_ShouldRefuseASecondRecord(t *testing.T) {
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

// expectNaming asserts one line the lab wrote — a note of a Timeline, or the
// error of a record it could not start — names each of the words given, reading
// the whole line back when one of them is absent.
func expectNaming(t *testing.T, written string, named ...string) {
	t.Helper()

	for _, name := range named {
		if !strings.Contains(written, name) {
			t.Errorf("the lab wrote %q, which does not name %s", written, name)
		}
	}
}

// expectReported asserts the Timeline carries one reason the control plane gave
// about one Pod, naming every reason it does carry when that one is absent.
func expectReported(t *testing.T, recorded scenario.Timeline, pod, reason string) {
	t.Helper()

	reported := reasonsReportedFor(recorded, pod)
	if !slices.Contains(reported, reason) {
		t.Errorf("the Timeline carries the reason %s for the Pod %s, but reports %v", reason, pod, reported)
	}
}

// reasonsReportedFor are the reasons the control plane reported about one Pod,
// in the order the lab received them.
func reasonsReportedFor(recorded scenario.Timeline, pod string) []string {
	var reported []string
	for _, entry := range recorded.Entries() {
		if entry.Change == scenario.ChangeReported && entry.Name == pod {
			reported = append(reported, entry.Reason)
		}
	}

	return reported
}

// namesRecorded are the objects of one kind the Timeline holds an entry for,
// each named once, in name order.
func namesRecorded(recorded scenario.Timeline, kind scenario.ObjectKind) []string {
	named := make(map[string]bool)
	for _, entry := range recorded.Entries() {
		if entry.Kind == kind {
			named[entry.Name] = true
		}
	}

	return slices.Sorted(maps.Keys(named))
}

// nodesOf are the Node names the Topology declares, in name order.
func nodesOf(topology scenario.Topology) []string {
	declared := topology.Nodes()
	names := make([]string, 0, len(declared))
	for _, node := range declared {
		names = append(names, node.Name)
	}

	return slices.Sorted(slices.Values(names))
}

// podsOf are the names of the Pods one observation found.
func podsOf(settled scenario.Placement) []string {
	observed := settled.Pods()
	names := make([]string, 0, len(observed))
	for _, pod := range observed {
		names = append(names, pod.Name)
	}

	return names
}

// everyPodNameOf are the Pods a Scenario's cluster held: the probe Pod
// provisioning created for each declared Node, and the Pods of the Workload
// itself, in name order.
func everyPodNameOf(topology scenario.Topology, workloadPods []string) []string {
	named := make([]string, 0, len(workloadPods))
	for _, node := range topology.Nodes() {
		named = append(named, probePodNamePrefix+node.Name)
	}

	return slices.Sorted(slices.Values(append(named, workloadPods...)))
}

// byReceipt orders two entries by the time the lab received them, which is the
// order a Timeline is read in.
func byReceipt(one, next scenario.Entry) int {
	return cmp.Compare(one.Elapsed, next.Elapsed)
}

// The two kinds the stream tests watch. Which kinds they are does not matter to
// the paths below, only that a note and a refusal name the kind they are about.
const (
	theKindWatched     = "Pod"
	theKindOpenedFirst = "Node"
)

const (
	// theVersionLastSeen is the version the API server committed the one change a
	// closed stream carried at, which its re-open resumes from.
	theVersionLastSeen = "88142"

	// noVersionYet is what the first open of a stream asks to resume from, having
	// seen no change on it.
	noVersionYet = ""

	// theRefusalTheServerGave is the reason an open the lab cannot make carries,
	// which a note and an error name.
	theRefusalTheServerGave = "the API server refused the watch"

	// theSecondCloseNamed is what a note says of a stream closing a second time;
	// the rest of that note is the kind it was watching.
	theSecondCloseNamed = "a second time"

	// theClusterRecorded names the cluster a refused open is reported against.
	theClusterRecorded = "lab-resilient-placement"

	// noVersionCarried is what an object the API server committed no version on
	// carries, which leaves the version last seen in place.
	noVersionCarried = ""

	// theNoteTakenAboutTheRecord is a note about the record itself rather than
	// about a change in the cluster, which is what a Timeline says its gaps with.
	theNoteTakenAboutTheRecord = "the Pod stream closed and could not be re-opened"

	theOneNote = 1
	noChange   = 0
)

// aStreamConnection is the lab's one stand-in for a system boundary: the watch
// connection the API server pushes one kind's changes down. It is hand-written
// so a test can close it, which a real KWOK control plane does not do on demand
// (docs/system-design.md § Contracts).
type aStreamConnection struct {
	changes chan watch.Event
	closing sync.Once
	stopped bool
}

func (c *aStreamConnection) ResultChan() <-chan watch.Event {
	return c.changes
}

// Stop closes this connection as the real watch does, and tolerates a second
// call: a stream stops every connection it has drained, and a test closes the
// same connection to stand in for the API server closing it.
func (c *aStreamConnection) Stop() {
	c.stopped = true
	c.closes()
}

// closes is the API server closing this connection, which is what ends a read of
// it. A connection already closed stays closed.
func (c *aStreamConnection) closes() {
	c.closing.Do(func() { close(c.changes) })
}

// aConnectionCarrying is a connection the API server pushed the changes given
// down and then closed, which a stream reads to its close.
func aConnectionCarrying(changes ...watch.Event) *aStreamConnection {
	carrying := &aStreamConnection{changes: make(chan watch.Event, len(changes))}
	for _, change := range changes {
		carrying.changes <- change
	}
	carrying.closes()

	return carrying
}

// anAlreadyClosedConnection is the named default for a connection that carried
// no change before the API server closed it.
func anAlreadyClosedConnection() *aStreamConnection {
	return aConnectionCarrying()
}

// anOpenConnection is the named default for a connection the API server has not
// closed and pushes no change down.
func anOpenConnection() *aStreamConnection {
	return &aStreamConnection{changes: make(chan watch.Event)}
}

// anAnswer is what one open of a stream is answered with: the connection the API
// server pushes changes down, or the error it refuses the watch with.
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

// aWatchAnswering stands in for the Watch call that opens one stream: it answers
// each open with the next answer given and keeps the version that open asked to
// resume from, so a test reads what a re-open carried.
type aWatchAnswering struct {
	answers []anAnswer
	resumed []string
}

// open answers the next open of this stream. An open past the answers given is
// the test's own mistake rather than a path under test, so it is refused with
// that said.
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

// aStreamWatching is one of the five streams a Timeline is recorded from, opened
// by hand-written connections rather than by the API server.
func aStreamWatching(kind string, answers ...anAnswer) (*stream, *aWatchAnswering) {
	watching := &aWatchAnswering{answers: answers}

	return &stream{named: kind, watch: watching.open}, watching
}

// aStreamOpened is such a stream already reading its first connection, which is
// the state a recording's read starts from.
func aStreamOpened(t *testing.T, ctx context.Context, kind string, answers ...anAnswer) (*stream, *aWatchAnswering) {
	t.Helper()

	reading, watching := aStreamWatching(kind, answers...)
	if err := reading.open(ctx); err != nil {
		t.Fatalf("open the %s stream: %v", kind, err)
	}

	return reading, watching
}

// changesRead are the changes one stream carried, read the way a recording reads
// it: in a goroutine of its own, to the stream's close. It hands them back once
// the read has ended, so a test asserts a complete read without a wait of its
// own.
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

// aPodChangedAt is one change the API server pushed down a stream, committed at
// the version given.
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
