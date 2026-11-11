//go:build !integration

package scenario

import (
	"slices"
	"testing"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
)

const (
	aThirdPodName          = "web-5d8f9b7c6-q7zrt"
	aThirdPodAsNamed       = aDeploymentName + "-q7zrt"
	whenTheMarkWasRead     = 2150 * time.Millisecond
	whenTheReplacementWent = 2160 * time.Millisecond
	whenTheReplacementWas  = 2180 * time.Millisecond
	whenTheNodeWasBack     = 2300 * time.Millisecond
	replacementBoundAsRead = "+2.180s"
	nodeBackAsRead         = "+2.300s"
	noLineFollowed         = 0
)

// theTimelineAtTheHold places one Pod on each Node, both Running before the
// Wait the Steps end at.
func theTimelineAtTheHold() []Entry {
	return []Entry{
		{
			Elapsed: whenTheFirstNodeWasReady, Change: ChangeAdded, Kind: NodeObject, Name: aNodeName,
			Reason: string(NodeReady), Detail: someNodeCapacity,
		},
		{
			Elapsed: whenTheLastNodeWasReady, Change: ChangeAdded, Kind: NodeObject, Name: aSecondNodeName,
			Reason: string(NodeReady), Detail: someNodeCapacity,
		},
		theDeploymentDeployed(),
		{Elapsed: whenTheFirstPodRan, Change: ChangeAdded, Kind: PodObject, Name: aPodName, Reason: string(PodRunning), Detail: aNodeName},
		{
			Elapsed: whenTheSecondPodRan, Change: ChangeAdded, Kind: PodObject, Name: aSecondPodName,
			Reason: string(PodRunning), Detail: aSecondNodeName,
		},
		aWaitNoteAt(whenTheFirstWaitEnded),
	}
}

func aWhenNotedWhileHeld() Entry {
	return Entry{Elapsed: whenTheNodeWasLost, Change: ChangeNoted, Kind: StepObject, Name: WhenStepNamed, Detail: aWhenLine}
}

func theReadinessOf(node string, readiness NodeReadiness, at time.Duration) Entry {
	return Entry{Elapsed: at, Change: ChangeModified, Kind: NodeObject, Name: node, Reason: string(readiness), Detail: someNodeCapacity}
}

func theDescheduledMarkOf(pod string, at time.Duration) Entry {
	return Entry{Elapsed: at, Change: ChangeReported, Kind: PodObject, Name: pod, Reason: EvictedByTheDescheduler, Detail: aNodeName}
}

func theMarkingForDeletionOf(pod, node string, at time.Duration) Entry {
	return Entry{Elapsed: at, Change: ChangeModified, Kind: PodObject, Name: pod, Reason: string(PodTerminating), Detail: node}
}

// theReplacementAdded is the third Pod, added Pending on no Node.
func theReplacementAdded() Entry {
	return Entry{Elapsed: whenTheReplacementWent, Change: ChangeAdded, Kind: PodObject, Name: aThirdPodName, Reason: string(PodPending)}
}

func theReplacementBoundTo(node string) Entry {
	return Entry{Elapsed: whenTheReplacementWas, Change: ChangeModified, Kind: PodObject, Name: aThirdPodName, Reason: string(PodPending), Detail: node}
}

func theReplacementRunningOn(node string) Entry {
	return Entry{Elapsed: whenTheReplacementRan, Change: ChangeModified, Kind: PodObject, Name: aThirdPodName, Reason: string(PodRunning), Detail: node}
}

func theReplacementScheduledOn(node string) []Entry {
	return []Entry{theReplacementAdded(), theReplacementBoundTo(node), theReplacementRunningOn(node)}
}

func theLinesFollowing(soFar []Entry, later ...Entry) []string {
	following := NewFollowing(NewTimeline(soFar))
	lines := []string{}
	for _, entry := range later {
		lines = append(lines, following.Read(entry)...)
	}

	return lines
}

const (
	theFirstPodEvictedByAFailure           = podEvictedAsRead + "  Pod " + aPodAsTheBoardNamesIt + " evicted from " + aNodeName
	theFirstPodEvictedByTheDescheduler     = theFirstPodEvictedByAFailure + " by the Descheduler"
	theSecondPodEvictedByAFailure          = podEvictedAsRead + "  Pod " + aSecondPodAsNamed + " evicted from " + aSecondNodeName
	theReplacementScheduledOnTheSecondNode = replacementBoundAsRead + "  Pod " + aThirdPodAsNamed + " scheduled on " + aSecondNodeName +
		", Running at " + replacementRanAsRead
	theReplacementScheduledOnTheFirstNode = replacementBoundAsRead + "  Pod " + aThirdPodAsNamed + " scheduled on " + aNodeName +
		", Running at " + replacementRanAsRead
)

func TestNodesTurningAfterAClickOnAHeldCluster_ShouldEachBeNamedInTheTerminalAsAWaitNarrativeNamesThem(t *testing.T) {
	lines := theLinesFollowing(theTimelineAtTheHold(),
		aWhenNotedWhileHeld(),
		theReadinessOf(aNodeName, NodeNotReady, whenTheNodeWasLost),
		theReadinessOf(aNodeName, NodeReady, whenTheNodeWasBack),
	)

	expect.Expect(t, lines).To(expect.Equal([]string{
		aWhenLine,
		nodeLostAsRead + "  Node " + aNodeName + " NotReady",
		nodeBackAsRead + "  Node " + aNodeName + " Ready",
	}))
}

func TestPodsEvictedAndScheduledAfterAClickOnAHeldCluster_ShouldBeNamedWithTheirNodeAndRunningAndNoLinkBetweenThem(t *testing.T) {
	lines := theLinesFollowing(theTimelineAtTheHold(), slices.Concat([]Entry{
		aWhenNotedWhileHeld(),
		theMarkingForDeletionOf(aPodName, aNodeName, whenThePodWasEvicted),
	}, theReplacementScheduledOn(aSecondNodeName))...)

	expect.Expect(t, lines).To(expect.Equal([]string{aWhenLine, theFirstPodEvictedByAFailure, theReplacementScheduledOnTheSecondNode}))
}

// The second Pod is marked for deletion with no descheduled mark, as a Failure
// evicts it.
func TestAPassEvictingPodsOnAHeldCluster_ShouldNameEachEvictionApartFromAFailuresAndEachReplacementsNode(t *testing.T) {
	lines := theLinesFollowing(theTimelineAtTheHold(), slices.Concat([]Entry{
		theDescheduledMarkOf(aPodName, whenThePodWasEvicted),
		theMarkingForDeletionOf(aPodName, aNodeName, whenThePodWasEvicted),
		theMarkingForDeletionOf(aSecondPodName, aSecondNodeName, whenThePodWasEvicted),
	}, theReplacementScheduledOn(aNodeName))...)

	expect.Expect(t, lines).To(expect.Equal([]string{
		theFirstPodEvictedByTheDescheduler, theSecondPodEvictedByAFailure, theReplacementScheduledOnTheFirstNode,
	}))
}

// A pass evicting nothing records nothing, so the entries below stand for
// every one that changes no Placement: a Deployment's and a ReplicaSet's, the
// control plane's word on a Pod, a Pod and a Node read again as they were, and
// a note about the record.
func TestAPassEvictingNothingOrAnEntryChangingNoPlacement_ShouldPrintNoTerminalLine(t *testing.T) {
	lines := theLinesFollowing(theTimelineAtTheHold(),
		Entry{Elapsed: whenTheNodeWasLost, Change: ChangeModified, Kind: DeploymentObject, Name: aDeploymentName, Detail: oneOfOneReady},
		Entry{Elapsed: whenTheNodeWasLost, Change: ChangeModified, Kind: ReplicaSetObject, Name: aDeploymentName},
		Entry{
			Elapsed: whenTheNodeWasLost, Change: ChangeReported, Kind: PodObject, Name: aPodName,
			Reason: scheduledReason, Detail: scheduledMessage,
		},
		Entry{Elapsed: whenTheNodeWasLost, Change: ChangeModified, Kind: PodObject, Name: aPodName, Reason: string(PodRunning), Detail: aNodeName},
		theReadinessOf(aNodeName, NodeReady, whenTheNodeWasLost),
		Entry{Elapsed: whenTheNodeWasLost, Change: ChangeNoted, Detail: aStreamGap},
	)

	expect.Expect(t, lines).To(expect.HaveLen[string](noLineFollowed))
}

// The moments a Wait narrative under a later Wait names are what Following
// names, line for line.
func TestALineFollowed_ShouldBeTheWaitNarrativesLineForTheSameEntries(t *testing.T) {
	later := slices.Concat([]Entry{
		theReadinessOf(aNodeName, NodeNotReady, whenTheNodeWasLost),
		theDescheduledMarkOf(aPodName, whenThePodWasEvicted),
		theMarkingForDeletionOf(aPodName, aNodeName, whenThePodWasEvicted),
	}, theReplacementScheduledOn(aSecondNodeName))
	narrated := NewTimeline(slices.Concat(theTimelineAtTheHold(), later, []Entry{aWaitNoteAt(whenTheSecondWaitEnded)})).WaitNarratives()
	theLaterWaitsNarrative := narrated[1]

	lines := theLinesFollowing(theTimelineAtTheHold(), later...)

	expect.Expect(t, lines).To(expect.Equal(theLaterWaitsNarrative.Lines()))
}

func TestAClickedWhenInTheTerminal_ShouldBeTheWhenItsTimelineHolds(t *testing.T) {
	lines := theLinesFollowing(theTimelineAtTheHold(), aWhenNotedWhileHeld())

	expect.Expect(t, lines).To(expect.Equal([]string{aWhenNotedWhileHeld().Detail}))
}

const aFailureNotInjectedLine = "Not injected: Node " + aNodeName + " stops responding, because the Node is gone"

func TestAClickedFailureNotInjectedInTheTerminal_ShouldBeTheNoteItsTimelineHoldsInPlaceOfItsWhen(t *testing.T) {
	notInjected := Entry{Elapsed: whenTheNodeWasLost, Change: ChangeNoted, Kind: StepObject, Name: NotInjectedNamed, Detail: aFailureNotInjectedLine}

	lines := theLinesFollowing(theTimelineAtTheHold(), notInjected)

	expect.Expect(t, lines).To(expect.Equal([]string{aFailureNotInjectedLine}))
}

// The pass evicted before the Wait the Steps end at, so that Wait's narrative
// names the eviction; its removal and a mark a restarted watch delivers late
// follow once the cluster is held.
func TestAPassWhileTheStepsRun_ShouldBeNamedOnlyByTheNextWaitsNarrative(t *testing.T) {
	evictedBeforeTheHold := slices.Concat(theTimelineAtTheHold(), []Entry{
		theDescheduledMarkOf(aPodName, whenThePodWasEvicted),
		theMarkingForDeletionOf(aPodName, aNodeName, whenThePodWasEvicted),
		aWaitNoteAt(whenTheSecondWaitEnded),
	})

	lines := theLinesFollowing(evictedBeforeTheHold,
		Entry{Elapsed: whenTheReplacementRan, Change: ChangeRemoved, Kind: PodObject, Name: aPodName, Reason: string(PodTerminating), Detail: aNodeName},
		theDescheduledMarkOf(aPodName, whenTheMarkWasRead),
	)

	expect.Expect(t, lines).To(expect.HaveLen[string](noLineFollowed))
}

func TestAReplacementNoNodeHasRoomFor_ShouldGetNoTerminalLine(t *testing.T) {
	lines := theLinesFollowing(theTimelineAtTheHold(),
		theDescheduledMarkOf(aPodName, whenThePodWasEvicted),
		theMarkingForDeletionOf(aPodName, aNodeName, whenThePodWasEvicted),
		theReplacementAdded(),
		Entry{Elapsed: whenTheReplacementWas, Change: ChangeReported, Kind: PodObject, Name: aThirdPodName, Reason: refusedReason, Detail: refusedMessage},
	)

	expect.Expect(t, lines).To(expect.Equal([]string{theFirstPodEvictedByTheDescheduler}))
}

func TestThePodsTheLabPlacesWhileProvisioning_ShouldAppearInNoTerminalLine(t *testing.T) {
	lines := theLinesFollowing(theTimelineAtTheHold(),
		Entry{Elapsed: whenTheNodeWasLost, Change: ChangeAdded, Kind: PodObject, Name: aProbePodName, Reason: string(PodPending), Detail: aNodeName},
		Entry{Elapsed: whenTheNodeWasBack, Change: ChangeModified, Kind: PodObject, Name: aProbePodName, Reason: string(PodRunning), Detail: aNodeName},
	)

	expect.Expect(t, lines).To(expect.HaveLen[string](noLineFollowed))
}

// The Pod was bound to its Node before the hold and reaches Running after it,
// as a Pod scheduled before one Wait reads under the next.
func TestAPodScheduledBeforeTheHoldThatRunsOnceHeld_ShouldBeNamedRunningAlone(t *testing.T) {
	boundBeforeTheHold := slices.Concat(theTimelineAtTheHold(), []Entry{
		theReplacementAdded(), theReplacementBoundTo(aSecondNodeName), aWaitNoteAt(whenTheSecondWaitEnded),
	})

	lines := theLinesFollowing(boundBeforeTheHold, theReplacementRunningOn(aSecondNodeName))

	expect.Expect(t, lines).To(expect.Equal([]string{
		replacementRanAsRead + "  Pod " + aThirdPodAsNamed + " Running on " + aSecondNodeName,
	}))
}

// A Pod's scheduling waits for its Running, which an eviction first ends.
func TestAPodEvictedBeforeItRuns_ShouldBeNamedScheduledAndThenEvicted(t *testing.T) {
	lines := theLinesFollowing(theTimelineAtTheHold(),
		theReplacementAdded(),
		theReplacementBoundTo(aSecondNodeName),
		theMarkingForDeletionOf(aThirdPodName, aSecondNodeName, whenTheReplacementRan),
	)

	expect.Expect(t, lines).To(expect.Equal([]string{
		replacementBoundAsRead + "  Pod " + aThirdPodAsNamed + " scheduled on " + aSecondNodeName,
		replacementRanAsRead + "  Pod " + aThirdPodAsNamed + " evicted from " + aSecondNodeName,
	}))
}

// A watch that restarts may deliver the Pod marked for deletion before its
// descheduled mark, by when its eviction is on the terminal.
func TestADescheduledMarkReadAfterItsEvictionWasNamed_ShouldNameTheEvictionNoSecondTime(t *testing.T) {
	lines := theLinesFollowing(theTimelineAtTheHold(),
		theMarkingForDeletionOf(aPodName, aNodeName, whenThePodWasEvicted),
		theDescheduledMarkOf(aPodName, whenTheMarkWasRead),
	)

	expect.Expect(t, lines).To(expect.Equal([]string{theFirstPodEvictedByAFailure}))
}
