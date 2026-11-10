package cluster

import (
	"bytes"
	"context"
	"encoding/binary"
	"slices"
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

// The endings run in order, so the first pulls the image and the last is
// stopped a moment into its pass rather than while pulling.
func TestAScenarioThatRanADeschedulerPass_ShouldLeaveNothingOfItsDeschedulerWhenItEnds(t *testing.T) {
	t.Parallel()
	endings := []struct {
		ending string
		policy func(*testing.T) scenario.DeschedulerPolicy
		within func(context.Context) (context.Context, context.CancelFunc)
	}{
		{ending: "a pass that exits zero", policy: aPolicyTheDeschedulerAccepts, within: context.WithCancel},
		{ending: "a pass that exits non-zero", policy: aPolicyTheDeschedulerRefuses, within: context.WithCancel},
		{ending: "a pass whose Scenario stops while it runs", policy: aPolicyTheDeschedulerAccepts, within: stoppedAMomentIntoThePass},
	}
	provisioned, engine := aProvisionedCluster(t)

	for _, passing := range endings {
		t.Run(passing.ending, func(t *testing.T) {
			within, stop := passing.within(t.Context())
			defer stop()

			_, _ = provisioned.RunDeschedulerPass(within, passing.policy(t), theDeschedulerPairedByDefault(t))

			expect.Expect(t, stillOnTheEngine(t, engine, deschedulerPassNameFor(provisioned.Name()))).To(expect.Equal(false))
		})
	}
}

// The moment is past creating the container and short of a pass ending.
const aMomentIntoThePass = 500 * time.Millisecond

func stoppedAMomentIntoThePass(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, aMomentIntoThePass)
}

const (
	theStdoutStream = 1
	theStderrStream = 2
)

// framed writes a line as the engine frames a log read without a terminal.
func framed(stream byte, line string) []byte {
	header := []byte{stream, 0, 0, 0, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(header[logFrameLengthAt:], uint32(len(line)))

	return append(header, line...)
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
