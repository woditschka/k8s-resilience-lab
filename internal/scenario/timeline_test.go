package scenario

import (
	"testing"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
)

const (
	someElapsed   = 3470 * time.Millisecond
	elapsedAsRead = "+3.470s"
)

const (
	aPodName         = "web-5d8f9b7c6-2xk4m"
	scheduledReason  = "Scheduled"
	scheduledMessage = "Successfully assigned default/web-5d8f9b7c6-2xk4m to zone-2-node-2"
	aDeploymentName  = "web"
	readyReplicas    = "3 of 9 Pods ready"
	aNodeName        = "zone-2-node-2"
	aStreamGap       = "the Pod stream closed and could not be re-opened"
)

// The two Deployment names nest, and a probe Pod belongs to provisioning rather
// than to either.
const (
	someNodeCapacity          = "4 cpu, 16Gi memory and 10 Pods"
	aSecondNodeName           = "zone-3-node-1"
	aProbePodName             = "provisioning-probe-" + aNodeName
	aSecondPodName            = "web-5d8f9b7c6-9ptwz"
	aNestedDeploymentName     = aDeploymentName + "-cache"
	aPodOfTheNestedDeployment = aNestedDeploymentName + "-6c8d7f5b4-mk4qv"
)

const (
	oneOfOneReady = "1 of 1 Pods ready"

	refusedReason  = "FailedScheduling"
	refusedMessage = "0/2 nodes are available: 2 node(s) didn't match pod topology spread constraints"
)

const (
	whenTheFirstNodeWasReady = 120 * time.Millisecond
	whenTheLastNodeWasReady  = 220 * time.Millisecond
	whenTheDeploymentLanded  = 1000 * time.Millisecond
	whenThePodsWerePending   = 1100 * time.Millisecond
	whenTheFirstPodWasBound  = 1200 * time.Millisecond
	whenTheFirstPodRan       = 1300 * time.Millisecond
	whenTheSecondPodRan      = 1350 * time.Millisecond
	whenTheFirstWaitEnded    = 1500 * time.Millisecond
	whenTheNodeWasLost       = 2000 * time.Millisecond
	whenThePodWasEvicted     = 2100 * time.Millisecond
	whenTheReplacementRan    = 2200 * time.Millisecond
	whenTheSecondWaitEnded   = 2500 * time.Millisecond

	firstNodeReadyAsRead    = "+0.120s"
	lastNodeReadyAsRead     = "+0.220s"
	firstPodBoundAsRead     = "+1.200s"
	firstPodRanAsRead       = "+1.300s"
	secondPodRanAsRead      = "+1.350s"
	nodeLostAsRead          = "+2.000s"
	podEvictedAsRead        = "+2.100s"
	replacementRanAsRead    = "+2.200s"
	aWaitLine               = "Wait for all Pods Ready"
	aWhenLine               = "When 1 Node fails, zone-2-node-2"
	aWhenStepNamed          = "When"
	aPodAsTheBoardNamesIt   = aDeploymentName + "-2xk4m"
	aSecondPodAsNamed       = aDeploymentName + "-9ptwz"
	theNestedPodAsNamed     = aNestedDeploymentName + "-mk4qv"
	whenTheProbePodWasAdded = 150 * time.Millisecond
	whenTheProbePodRan      = 180 * time.Millisecond
)

func aWaitNoteAt(elapsed time.Duration) Entry {
	return Entry{Elapsed: elapsed, Change: ChangeNoted, Kind: StepObject, Name: WaitStepNamed, Detail: aWaitLine}
}

func theDeploymentDeployed() Entry {
	return Entry{
		Elapsed: whenTheDeploymentLanded, Change: ChangeAdded, Kind: DeploymentObject, Name: aDeploymentName,
		Detail: oneOfOneReady,
	}
}

func linesOf(narratives []WaitNarrative) [][]string {
	read := make([][]string, 0, len(narratives))
	for _, narrative := range narratives {
		read = append(read, narrative.Lines())
	}

	return read
}

func TestAWaitNarrative_ShouldNameEachPodsNodeAndWhenItReachedRunningInOrder(t *testing.T) {
	recorded := []Entry{
		{
			Elapsed: whenTheLastNodeWasReady, Change: ChangeAdded, Kind: NodeObject, Name: aNodeName,
			Reason: string(NodeReady), Detail: someNodeCapacity,
		},
		theDeploymentDeployed(),
		{Elapsed: whenThePodsWerePending, Change: ChangeAdded, Kind: PodObject, Name: aPodName, Reason: string(PodPending)},
		{Elapsed: whenThePodsWerePending, Change: ChangeAdded, Kind: PodObject, Name: aSecondPodName, Reason: string(PodPending)},
		{
			Elapsed: whenTheFirstPodWasBound, Change: ChangeModified, Kind: PodObject, Name: aPodName,
			Reason: string(PodPending), Detail: aNodeName,
		},
		{
			Elapsed: whenTheFirstPodRan, Change: ChangeModified, Kind: PodObject, Name: aPodName,
			Reason: string(PodRunning), Detail: aNodeName,
		},
		{
			Elapsed: whenTheSecondPodRan, Change: ChangeModified, Kind: PodObject, Name: aSecondPodName,
			Reason: string(PodRunning), Detail: aSecondNodeName,
		},
		aWaitNoteAt(whenTheFirstWaitEnded),
	}

	narrated := NewTimeline(recorded).WaitNarratives()

	expect.Expect(t, linesOf(narrated)).To(expect.Equal([][]string{{
		lastNodeReadyAsRead + "  Node " + aNodeName + " Ready",
		firstPodBoundAsRead + "  Pod " + aPodAsTheBoardNamesIt + " scheduled on " + aNodeName + ", Running at " + firstPodRanAsRead,
		secondPodRanAsRead + "  Pod " + aSecondPodAsNamed + " scheduled on " + aSecondNodeName + ", Running at " + secondPodRanAsRead,
	}}))
}

// The eviction and the replacement stand in the order they happened and
// nothing links them.
func TestAWaitNarrativeAfterAFailure_ShouldNameNodesNotReadyEvictionsAndReplacementsInOrder(t *testing.T) {
	recorded := []Entry{
		{
			Elapsed: whenTheFirstNodeWasReady, Change: ChangeAdded, Kind: NodeObject, Name: aNodeName,
			Reason: string(NodeReady), Detail: someNodeCapacity,
		},
		{
			Elapsed: whenTheLastNodeWasReady, Change: ChangeAdded, Kind: NodeObject, Name: aSecondNodeName,
			Reason: string(NodeReady), Detail: someNodeCapacity,
		},
		theDeploymentDeployed(),
		{
			Elapsed: whenTheFirstPodRan, Change: ChangeAdded, Kind: PodObject, Name: aPodName,
			Reason: string(PodRunning), Detail: aNodeName,
		},
		aWaitNoteAt(whenTheFirstWaitEnded),
		{Elapsed: whenTheFirstWaitEnded, Change: ChangeNoted, Kind: StepObject, Name: aWhenStepNamed, Detail: aWhenLine},
		{
			Elapsed: whenTheNodeWasLost, Change: ChangeModified, Kind: NodeObject, Name: aNodeName,
			Reason: string(NodeNotReady), Detail: someNodeCapacity,
		},
		{
			Elapsed: whenThePodWasEvicted, Change: ChangeModified, Kind: PodObject, Name: aPodName,
			Reason: string(PodTerminating), Detail: aNodeName,
		},
		{
			Elapsed: whenThePodWasEvicted, Change: ChangeAdded, Kind: PodObject, Name: aSecondPodName,
			Reason: string(PodPending),
		},
		{
			Elapsed: whenTheReplacementRan, Change: ChangeModified, Kind: PodObject, Name: aSecondPodName,
			Reason: string(PodRunning), Detail: aSecondNodeName,
		},
		{
			Elapsed: whenTheReplacementRan, Change: ChangeModified, Kind: PodObject, Name: aPodName,
			Reason: string(PodTerminating), Detail: aNodeName,
		},
		aWaitNoteAt(whenTheSecondWaitEnded),
	}

	narrated := NewTimeline(recorded).WaitNarratives()

	expect.Expect(t, linesOf(narrated)).To(expect.Equal([][]string{
		{
			firstNodeReadyAsRead + "  Node " + aNodeName + " Ready",
			lastNodeReadyAsRead + "  Node " + aSecondNodeName + " Ready",
			firstPodRanAsRead + "  Pod " + aPodAsTheBoardNamesIt + " scheduled on " + aNodeName + ", Running at " + firstPodRanAsRead,
		},
		{
			nodeLostAsRead + "  Node " + aNodeName + " NotReady",
			podEvictedAsRead + "  Pod " + aPodAsTheBoardNamesIt + " evicted from " + aNodeName,
			replacementRanAsRead + "  Pod " + aSecondPodAsNamed + " scheduled on " + aSecondNodeName + ", Running at " + replacementRanAsRead,
		},
	}))
}

func TestAWaitNarrative_ShouldNameEachPodLeftPendingWithTheSchedulersReason(t *testing.T) {
	recorded := []Entry{
		theDeploymentDeployed(),
		{Elapsed: whenThePodsWerePending, Change: ChangeAdded, Kind: PodObject, Name: aPodName, Reason: string(PodPending)},
		{
			Elapsed: whenTheFirstPodRan, Change: ChangeReported, Kind: PodObject, Name: aPodName,
			Reason: refusedReason, Detail: refusedMessage,
		},
		aWaitNoteAt(whenTheFirstWaitEnded),
	}

	narrated := NewTimeline(recorded).WaitNarratives()

	expect.Expect(t, linesOf(narrated)).To(expect.Equal([][]string{{
		"Pod " + aPodAsTheBoardNamesIt + " left Pending, " + refusedReason + ": " + refusedMessage,
	}}))
}

func TestAWaitNarrative_ShouldLeaveOutThePodsTheLabPlacesWhileProvisioning(t *testing.T) {
	recorded := []Entry{
		{
			Elapsed: whenTheFirstNodeWasReady, Change: ChangeAdded, Kind: NodeObject, Name: aNodeName,
			Reason: string(NodeReady), Detail: someNodeCapacity,
		},
		{Elapsed: whenTheProbePodWasAdded, Change: ChangeAdded, Kind: PodObject, Name: aProbePodName, Reason: string(PodPending)},
		{
			Elapsed: whenTheProbePodRan, Change: ChangeModified, Kind: PodObject, Name: aProbePodName,
			Reason: string(PodRunning), Detail: aNodeName,
		},
		aWaitNoteAt(whenTheFirstWaitEnded),
	}

	narrated := NewTimeline(recorded).WaitNarratives()

	expect.Expect(t, linesOf(narrated)).To(expect.Equal([][]string{{
		firstNodeReadyAsRead + "  Node " + aNodeName + " Ready",
	}}))
}

// A change after the last Wait's note stands under no Wait, since no board
// follows it.
func TestAWaitAfterWhichNothingChangedThePlacement_ShouldHaveNoNarrative(t *testing.T) {
	recorded := []Entry{
		{
			Elapsed: whenTheFirstNodeWasReady, Change: ChangeAdded, Kind: NodeObject, Name: aNodeName,
			Reason: string(NodeReady), Detail: someNodeCapacity,
		},
		aWaitNoteAt(whenTheFirstWaitEnded),
		aWaitNoteAt(whenTheSecondWaitEnded),
		{
			Elapsed: whenTheNodeWasLost, Change: ChangeModified, Kind: NodeObject, Name: aNodeName,
			Reason: string(NodeNotReady), Detail: someNodeCapacity,
		},
	}

	narrated := NewTimeline(recorded).WaitNarratives()

	expect.Expect(t, linesOf(narrated)).To(expect.Equal([][]string{
		{firstNodeReadyAsRead + "  Node " + aNodeName + " Ready"},
		{},
	}))
}

func TestAPodScheduledBeforeOneWaitAndRunningBeforeTheNext_ShouldReadAsRunningUnderTheNext(t *testing.T) {
	recorded := []Entry{
		theDeploymentDeployed(),
		{
			Elapsed: whenTheFirstPodWasBound, Change: ChangeAdded, Kind: PodObject, Name: aPodName,
			Reason: string(PodPending), Detail: aNodeName,
		},
		aWaitNoteAt(whenTheFirstWaitEnded),
		{
			Elapsed: whenTheReplacementRan, Change: ChangeModified, Kind: PodObject, Name: aPodName,
			Reason: string(PodRunning), Detail: aNodeName,
		},
		aWaitNoteAt(whenTheSecondWaitEnded),
	}

	narrated := NewTimeline(recorded).WaitNarratives()

	expect.Expect(t, linesOf(narrated)).To(expect.Equal([][]string{
		{firstPodBoundAsRead + "  Pod " + aPodAsTheBoardNamesIt + " scheduled on " + aNodeName},
		{replacementRanAsRead + "  Pod " + aPodAsTheBoardNamesIt + " Running on " + aNodeName},
	}))
}

func TestAPodOfTwoNestedDeploymentNames_ShouldBeNamedForTheLongestItBeginsWith(t *testing.T) {
	recorded := []Entry{
		theDeploymentDeployed(),
		{
			Elapsed: whenTheDeploymentLanded, Change: ChangeAdded, Kind: DeploymentObject, Name: aNestedDeploymentName,
			Detail: oneOfOneReady,
		},
		{
			Elapsed: whenTheFirstPodRan, Change: ChangeAdded, Kind: PodObject, Name: aPodOfTheNestedDeployment,
			Reason: string(PodRunning), Detail: aNodeName,
		},
		aWaitNoteAt(whenTheFirstWaitEnded),
	}

	narrated := NewTimeline(recorded).WaitNarratives()

	expect.Expect(t, linesOf(narrated)).To(expect.Equal([][]string{{
		firstPodRanAsRead + "  Pod " + theNestedPodAsNamed + " scheduled on " + aNodeName + ", Running at " + firstPodRanAsRead,
	}}))
}

// Only that the two entries differ matters.
func anEntry() Entry {
	return Entry{Elapsed: someElapsed, Change: ChangeNoted, Detail: aStreamGap}
}

func aDifferentEntry() Entry {
	return Entry{Elapsed: someElapsed, Change: ChangeAdded, Kind: NodeObject, Name: aNodeName}
}

func TestAnEntryOfATimeline_ShouldReadAsTheTimeTheChangeAndTheReasonGivenForIt(t *testing.T) {
	lines := map[string]struct {
		entry Entry
		reads string
	}{
		"a change the control plane reported a reason for": {
			entry: Entry{
				Elapsed: someElapsed, Change: ChangeReported, Kind: PodObject, Name: aPodName,
				Reason: scheduledReason, Detail: scheduledMessage,
			},
			reads: elapsedAsRead + "  Pod " + aPodName + " reported, " + scheduledReason + ": " + scheduledMessage,
		},
		"a change carrying a detail and no reason": {
			entry: Entry{
				Elapsed: someElapsed, Change: ChangeModified, Kind: DeploymentObject, Name: aDeploymentName,
				Detail: readyReplicas,
			},
			reads: elapsedAsRead + "  Deployment " + aDeploymentName + " modified, " + readyReplicas,
		},
		"a change carrying neither": {
			entry: Entry{Elapsed: someElapsed, Change: ChangeAdded, Kind: NodeObject, Name: aNodeName},
			reads: elapsedAsRead + "  Node " + aNodeName + " added",
		},
		"a note the Timeline makes about itself, naming no object": {
			entry: Entry{Elapsed: someElapsed, Change: ChangeNoted, Detail: aStreamGap},
			reads: elapsedAsRead + "  noted, " + aStreamGap,
		},
		"a Step's note, carrying its line": {
			entry: Entry{Elapsed: someElapsed, Change: ChangeNoted, Kind: StepObject, Name: WaitStepNamed, Detail: aWaitLine},
			reads: elapsedAsRead + "  Step Wait noted, " + aWaitLine,
		},
	}

	for shape, line := range lines {
		t.Run(shape, func(t *testing.T) {
			expect.Expect(t, line.entry.Line()).To(expect.Equal(line.reads))
		})
	}
}

func TestATimelineOverRecordedEntries_ShouldCloneThemAtItsBoundary(t *testing.T) {
	recorded := []Entry{anEntry()}
	timeline := NewTimeline(recorded)

	recorded[0] = aDifferentEntry()
	timeline.Entries()[0] = aDifferentEntry()

	expect.Expect(t, timeline.Entries()).To(expect.Equal([]Entry{anEntry()}))
}
