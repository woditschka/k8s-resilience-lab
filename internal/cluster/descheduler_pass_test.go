package cluster

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// A constraint action the Descheduler validates and refuses, so a pass under
// it exits non-zero before it evicts anything.
const anActionTheDeschedulerRefuses = "NeverSchedule"

func aPolicyTheDeschedulerAccepts(t *testing.T) scenario.DeschedulerPolicy {
	t.Helper()

	return aPolicyBalancingTheSpreadOf(t, aConstraintAction)
}

func aPolicyTheDeschedulerRefuses(t *testing.T) scenario.DeschedulerPolicy {
	t.Helper()

	return aPolicyBalancingTheSpreadOf(t, anActionTheDeschedulerRefuses)
}

func aPolicyBalancingTheSpreadOf(t *testing.T, action string) scenario.DeschedulerPolicy {
	t.Helper()

	return aDeschedulerPolicyOf(t, anyLimitPerNode, aDeschedulerProfileOf(t, someProfileName,
		[]scenario.DeschedulerPluginConfig{scenario.NewDeschedulerPluginConfig(scenario.RemovePodsViolatingTopologySpreadConstraint,
			scenario.NewDeschedulerArgument(theConstraintsArgument, []string{action}))},
		scenario.NewDeschedulerPluginSet(scenario.BalanceExtensionPoint, scenario.RemovePodsViolatingTopologySpreadConstraint)))
}

func theDeschedulerPairedByDefault(t *testing.T) DeschedulerVersion {
	t.Helper()

	paired, err := DefaultKubernetesVersion().PairedDescheduler()
	if err != nil {
		t.Fatalf("pair a Descheduler with Kubernetes %s: %v", DefaultKubernetesVersion(), err)
	}

	return paired
}

// The pairing builds no such version; only this package's tests can, to stand
// for an image the registry never published.
const aTagTheRegistryNeverPublished = "v0.0.0-unpublished"

func aDeschedulerTheRegistryNeverPublished() DeschedulerVersion {
	return DeschedulerVersion{tag: aTagTheRegistryNeverPublished}
}

// A registry the engine cannot reach must not hold the test for the pull's own
// timeout.
const aRegistryAnswersWithin = 2 * time.Minute

func TestADeschedulerImageTheLabCannotObtain_ShouldNotBeObtainedAndShouldBeNamed(t *testing.T) {
	t.Parallel()
	engine := requireEngine(t)
	unpublished := aDeschedulerTheRegistryNeverPublished()
	obtaining, cancel := context.WithTimeout(t.Context(), aRegistryAnswersWithin)
	defer cancel()

	err := ObtainDescheduler(obtaining, engine, unpublished)

	if err == nil {
		t.Fatalf("no Descheduler image is published for %s, but it was obtained", unpublished)
	}
	expect.Expect(t, strings.Contains(err.Error(), aTagTheRegistryNeverPublished)).To(expect.Equal(true))
}

// theDeschedulerObtained makes the engine hold the paired Descheduler's image,
// as a run declaring a policy does before any Scenario starts, since a pass
// fetches none.
func theDeschedulerObtained(t *testing.T, engine Endpoint) DeschedulerVersion {
	t.Helper()

	paired := theDeschedulerPairedByDefault(t)
	if err := ObtainDescheduler(t.Context(), engine, paired); err != nil {
		t.Fatalf("obtain the Descheduler %s: %v", paired, err)
	}

	return paired
}

// The engine refuses the pass's creation for want of its image, which only a
// pass that fetched nothing first meets.
func TestADeschedulerPassInsideAScenario_ShouldFetchNoImage(t *testing.T) {
	t.Parallel()
	provisioned, engine := aProvisionedCluster(t)
	passing, cancel := context.WithTimeout(t.Context(), aRegistryAnswersWithin)
	defer cancel()

	_, err := provisioned.RunDeschedulerPass(passing, aPolicyTheDeschedulerAccepts(t), aDeschedulerTheRegistryNeverPublished())

	expect.Expect(t, refusedAtTheCreation{
		ForWantOfTheImage: isNoLongerHeld(err),
		AtTheCreation:     strings.Contains(fmt.Sprint(err), theCreationOfAPass),
		LeftOnTheEngine:   stillOnTheEngine(t, engine, deschedulerPassNameFor(provisioned.Name())),
	}).To(expect.Equal(refusedAtTheCreation{ForWantOfTheImage: true, AtTheCreation: true, LeftOnTheEngine: false}))
}

// A creation's refusal names the step it refused, as every step of a pass's
// errors does.
const theCreationOfAPass = "create the Descheduler pass"

// refusedAtTheCreation is how a pass on an image the engine lacks ended.
type refusedAtTheCreation struct {
	ForWantOfTheImage bool
	AtTheCreation     bool
	LeftOnTheEngine   bool
}

// The endings run in order, so the third is stopped a moment into its pass
// rather than before it.
//
// A Scenario that stops while its pass's container is created leaves the
// container the engine created with no answer reaching the lab, so that row
// starts from such a container and a Scenario already stopped.
func TestAScenarioThatRanADeschedulerPass_ShouldLeaveNothingOfItsDeschedulerWhenItEnds(t *testing.T) {
	t.Parallel()
	endings := []struct {
		ending string
		left   func(*testing.T, *SimulatedCluster)
		policy func(*testing.T) scenario.DeschedulerPolicy
		within func(context.Context) (context.Context, context.CancelFunc)
	}{
		{ending: "a pass that exits zero", left: nothingOfAnEarlierPass, policy: aPolicyTheDeschedulerAccepts, within: context.WithCancel},
		{ending: "a pass that exits non-zero", left: nothingOfAnEarlierPass, policy: aPolicyTheDeschedulerRefuses, within: context.WithCancel},
		{ending: "a pass whose Scenario stops while it runs", left: nothingOfAnEarlierPass, policy: aPolicyTheDeschedulerAccepts, within: stoppedAMomentIntoThePass},
		{ending: "a pass whose Scenario stops while its container is created", left: aPassContainerWhoseCreationWentUnanswered, policy: aPolicyTheDeschedulerAccepts, within: alreadyStopped},
	}
	provisioned, engine := aProvisionedCluster(t)
	paired := theDeschedulerObtained(t, engine)

	for _, passing := range endings {
		t.Run(passing.ending, func(t *testing.T) {
			passing.left(t, provisioned)
			within, stop := passing.within(t.Context())
			defer stop()

			_, _ = provisioned.RunDeschedulerPass(within, passing.policy(t), paired)

			expect.Expect(t, stillOnTheEngine(t, engine, deschedulerPassNameFor(provisioned.Name()))).To(expect.Equal(false))
		})
	}
}

// The engine answering the creation only after the lab stopped waiting leaves
// the same container behind as a Scenario stopping while it is created, so the
// test starts from that container and runs the next pass beside it.
func TestADeschedulerPassWhoseCreationTheEngineAnswersLate_ShouldLeaveNothingOfItsDeschedulerForTheNextPass(t *testing.T) {
	t.Parallel()
	provisioned, engine := aProvisionedCluster(t)
	aPassContainerWhoseCreationWentUnanswered(t, provisioned)

	_, err := provisioned.RunDeschedulerPass(t.Context(), aPolicyTheDeschedulerAccepts(t), theDeschedulerPairedByDefault(t))

	if err != nil {
		t.Fatalf("run the Descheduler pass after one whose creation was answered late: %v", err)
	}
	expect.Expect(t, stillOnTheEngine(t, engine, deschedulerPassNameFor(provisioned.Name()))).To(expect.Equal(false))
}

// The moment is past creating the container and short of a pass ending.
const aMomentIntoThePass = 500 * time.Millisecond

func stoppedAMomentIntoThePass(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, aMomentIntoThePass)
}

func alreadyStopped(ctx context.Context) (context.Context, context.CancelFunc) {
	stopped, stop := context.WithCancel(ctx)
	stop()

	return stopped, stop
}

func nothingOfAnEarlierPass(*testing.T, *SimulatedCluster) {}

// aPassContainerWhoseCreationWentUnanswered creates the pass's container and
// drops its handle, the state a creation leaves when the lab stopped waiting
// for the engine's answer. Its removal runs before the cluster's, so the
// container goes even while the lab leaves it behind. The removal is not
// guarded by a read under t.Context, which is cancelled before cleanups run.
func aPassContainerWhoseCreationWentUnanswered(t *testing.T, provisioned *SimulatedCluster) {
	t.Helper()

	image := deschedulerImage(theDeschedulerObtained(t, provisioned.container.endpoint))
	unanswered, err := provisioned.container.createDeschedulerPass(t.Context(), image)
	if err != nil {
		t.Fatalf("create the Descheduler pass of %s: %v", provisioned.Name(), err)
	}
	t.Cleanup(func() {
		err := removeContainer(context.WithoutCancel(t.Context()), unanswered.endpoint, unanswered.identifier, "the Descheduler pass "+unanswered.name)
		if err != nil && !isNoLongerHeld(err) {
			t.Errorf("remove the Descheduler pass %s: %v", unanswered.name, err)
		}
	})
}

const (
	theStdoutStream = 1
	theStderrStream = 2
)

// framed writes a line as the engine frames a log read without a terminal.
func framed(stream byte, line string) []byte {
	return append(aFrameHeaderStating(stream, uint32(len(line))), line...)
}

func aFrameHeaderStating(stream byte, length uint32) []byte {
	header := []byte{stream, 0, 0, 0, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(header[logFrameLengthAt:], length)

	return header
}

func TestAPassesLog_ShouldBeReadAsItsLastLinesOfBothStreamsInTheOrderWritten(t *testing.T) {
	const (
		anEarlierLine  = "I1109 the first line, beyond the lines kept"
		aStdoutLine    = "I1109 a line written to stdout"
		aStderrLine    = "E1109 a line written to stderr"
		linesKept      = 2
		aCutFramesHalf = "E1109 a frame the read limit cut"
	)
	written := slices.Concat(
		framed(theStdoutStream, anEarlierLine+"\n"),
		framed(theStdoutStream, aStdoutLine+"\n"),
		framed(theStderrStream, aStderrLine+"\n"),
		framed(theStderrStream, aCutFramesHalf)[:logFrameHeaderLength+1],
	)

	lines, err := lastLinesOf(bytes.NewReader(written), linesKept)

	if err != nil {
		t.Fatalf("read the frames of a pass's log: %v", err)
	}
	expect.Expect(t, lines).To(expect.Equal([]string{aStdoutLine, aStderrLine}))
}

// largestReadRecorded keeps the largest buffer a reader handed it to fill,
// which is the most the reader allocated for what it read. It stands in for the
// engine's log stream, unlimited, since io.LimitReader shortens the buffers it
// passes on and would hide the allocation.
type largestReadRecorded struct {
	unread  *bytes.Reader
	largest int
}

func (r *largestReadRecorded) Read(buffer []byte) (int, error) {
	r.largest = max(r.largest, len(buffer))

	return r.unread.Read(buffer)
}

// readWithinTheLimit is what reading a log leaves: the lines kept, and whether
// no buffer handed to the stream exceeded the read limit.
type readWithinTheLimit struct {
	Lines              []string
	WithinTheReadLimit bool
}

// Unframed log text read as a header states a length of hundreds of megabytes,
// so it falls under the same rule as a frame stating one. The length stated is
// far beyond the limit and still harmless to allocate.
func TestAPassesLogWhoseFrameStatesALengthBeyondTheReadLimit_ShouldBeReadWithinTheReadLimit(t *testing.T) {
	const (
		aLineBeforeIt         = "E1109 a line before the frame stating too much"
		aLineAfterIt          = "E1109 a line after the frame stating too much"
		anyLinesKept          = 20
		aLengthBeyondTheLimit = 1024 * logBytesReadAtMost
	)
	stream := &largestReadRecorded{unread: bytes.NewReader(slices.Concat(
		framed(theStdoutStream, aLineBeforeIt+"\n"),
		aFrameHeaderStating(theStderrStream, aLengthBeyondTheLimit),
		framed(theStderrStream, aLineAfterIt+"\n"),
	))}

	lines, err := lastLinesOf(stream, anyLinesKept)

	if err != nil {
		t.Fatalf("read the frames of a pass's log: %v", err)
	}
	expect.Expect(t, readWithinTheLimit{Lines: lines, WithinTheReadLimit: stream.largest <= logBytesReadAtMost}).
		To(expect.Equal(readWithinTheLimit{Lines: []string{aLineBeforeIt}, WithinTheReadLimit: true}))
}

// No test here turns on a Pod's name or UID beyond telling the Pods apart.
const (
	aPodsUID       = types.UID("a-pod")
	anotherPodsUID = types.UID("another-pod")
)

func aPodOf(uid types.UID) corev1.Pod {
	return corev1.Pod{UID: uid}
}

func withDeletionMarked(pod corev1.Pod) corev1.Pod {
	marked := *pod.DeepCopy()
	marked.DeletionTimestamp = markedForDeletion()

	return marked
}

func withDisruptionTargetMarked(pod corev1.Pod) corev1.Pod {
	marked := *pod.DeepCopy()
	marked.Status.Conditions = []corev1.PodCondition{{Type: corev1.DisruptionTarget, Status: corev1.ConditionTrue}}

	return marked
}

func TestAPassAfterWhichEveryPodItCouldEvictIsListedUnmarkedOrAPodAlreadyLeavingStillLeaves_ShouldHaveEvictedNothing(t *testing.T) {
	passes := map[string]struct{ before, after []corev1.Pod }{
		"every Pod listed again unmarked, a new one beside them": {
			before: []corev1.Pod{aPodOf(aPodsUID)},
			after:  []corev1.Pod{aPodOf(aPodsUID), aPodOf(anotherPodsUID)},
		},
		"a Pod marked for deletion before the pass gone after it": {
			before: []corev1.Pod{aPodOf(aPodsUID), withDeletionMarked(aPodOf(anotherPodsUID))},
			after:  []corev1.Pod{aPodOf(aPodsUID)},
		},
	}

	for named, pass := range passes {
		t.Run(named, func(t *testing.T) {
			expect.Expect(t, anyEvicted(pass.before, pass.after)).To(expect.Equal(false))
		})
	}
}

func TestAPassAfterWhichAPodItCouldEvictIsGoneMarkedForDeletionOrADisruptionTarget_ShouldHaveEvictedAPod(t *testing.T) {
	before := []corev1.Pod{aPodOf(aPodsUID), aPodOf(anotherPodsUID)}
	afters := map[string][]corev1.Pod{
		"a Pod listed no more":      {aPodOf(aPodsUID)},
		"a Pod marked for deletion": {aPodOf(aPodsUID), withDeletionMarked(aPodOf(anotherPodsUID))},
		"a Pod a disruption target": {aPodOf(aPodsUID), withDisruptionTargetMarked(aPodOf(anotherPodsUID))},
	}

	for named, after := range afters {
		t.Run(named, func(t *testing.T) {
			expect.Expect(t, anyEvicted(before, after)).To(expect.Equal(true))
		})
	}
}
