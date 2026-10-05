package scenario

import (
	"testing"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
)

// someElapsed is how long after the record started the entries below were
// received, and elapsedAsRead is how that time reads in a printed line.
const (
	someElapsed   = 3470 * time.Millisecond
	elapsedAsRead = "+3.470s"
)

// The values the entries below name: the Pod a Placement was reported for, the
// reason the control plane gave for it, the Workload and the Node a change was
// about, and the gap a Timeline notes about itself.
const (
	aPodName         = "web-5d8f9b7c6-2xk4m"
	scheduledReason  = "Scheduled"
	scheduledMessage = "Successfully assigned default/web-5d8f9b7c6-2xk4m to zone-b-node-2"
	aWorkloadName    = "web"
	readyReplicas    = "3 of 9 Pods ready"
	aNodeName        = "zone-b-node-2"
	aStreamGap       = "the Pod stream closed and could not be re-opened"
)

// The Topology, the Workloads and the Pods the summaries below are read
// against: the Nodes the lab brought up, a probe Pod of its own provisioning,
// two Workloads whose names nest, the Pods placed for them, and the replica set
// the control plane made for the first Workload.
const (
	someNodeCapacity        = "10 Pods"
	aSecondNodeName         = "zone-c-node-1"
	aProbePodName           = "provisioning-probe-" + aNodeName
	aSecondPodName          = "web-5d8f9b7c6-9ptwz"
	aNestedWorkloadName     = aWorkloadName + "-cache"
	aPodOfTheNestedWorkload = aNestedWorkloadName + "-6c8d7f5b4-mk4qv"
	aReplicaSetName         = aWorkloadName + "-5d8f9b7c6"
)

// The words the control plane gave about those Workloads and Pods: how much of
// each Workload it reported ready, and why it left a Pod Pending.
const (
	noneOfTwoReady = "0 of 2 Pods ready"
	twoOfTwoReady  = "2 of 2 Pods ready"
	noneOfOneReady = "0 of 1 Pods ready"
	oneOfOneReady  = "1 of 1 Pods ready"

	refusedReason  = "FailedScheduling"
	refusedMessage = "0/2 nodes are available: 2 node(s) didn't match pod topology spread constraints"
)

// When each of those changes was received, and how that time reads in a summary.
const (
	whenTheFirstNodeWasReady = 120 * time.Millisecond
	whenTheLastNodeWasReady  = 220 * time.Millisecond
	whenTheWorkloadLanded    = 1000 * time.Millisecond
	whenThePodsWerePending   = 1100 * time.Millisecond
	whenTheFirstPodWasBound  = 1200 * time.Millisecond
	whenTheFirstPodRan       = 1300 * time.Millisecond
	whenTheSecondPodRan      = 1350 * time.Millisecond
	whenTheNodeWasLost       = 2000 * time.Millisecond
	whenThePodRanAgain       = 2200 * time.Millisecond
	whenTheWorkloadWasReady  = 3400 * time.Millisecond

	lastNodeReadyAsRead  = "0.220s"
	workloadLandedAsRead = "1.000s"
	podsPendingAsRead    = "+1.100s"
	firstPodRanAsRead    = "+1.300s"
	secondPodRanAsRead   = "+1.350s"
	nodeLostAsRead       = "+2.000s"
	podRanAgainAsRead    = "+2.200s"
	workloadReadyAsRead  = "3.400s"
)

// noPodLeftPending and noWorkloadDeployed are how a summary says a Scenario left
// nothing to name.
const (
	noPodLeftPending   = "Pending   none"
	noWorkloadDeployed = "Workload  none"
)

func TestATimelineSummary_ShouldCollapseTheLabSetupOfTheTopologyToOneLine(t *testing.T) {
	recorded := []Entry{
		{
			Elapsed: whenTheFirstNodeWasReady, Change: ChangeAdded, Kind: NodeObject, Name: aNodeName,
			Reason: string(NodeReady), Detail: someNodeCapacity,
		},
		{
			Elapsed: whenTheLastNodeWasReady, Change: ChangeAdded, Kind: NodeObject, Name: aSecondNodeName,
			Reason: string(NodeReady), Detail: someNodeCapacity,
		},
		{
			Elapsed: someElapsed, Change: ChangeAdded, Kind: PodObject, Name: aProbePodName,
			Reason: string(PodPending),
		},
		{
			Elapsed: someElapsed, Change: ChangeModified, Kind: PodObject, Name: aProbePodName,
			Reason: string(PodRunning), Detail: aNodeName,
		},
		{
			Elapsed: whenTheWorkloadLanded, Change: ChangeAdded, Kind: DeploymentObject, Name: aWorkloadName,
			Detail: noneOfTwoReady,
		},
		{
			Elapsed: whenThePodsWerePending, Change: ChangeAdded, Kind: PodObject, Name: aPodName,
			Reason: string(PodPending),
		},
		{
			Elapsed: whenThePodsWerePending, Change: ChangeAdded, Kind: PodObject, Name: aSecondPodName,
			Reason: string(PodPending),
		},
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
		{
			Elapsed: whenTheWorkloadWasReady, Change: ChangeModified, Kind: DeploymentObject, Name: aWorkloadName,
			Detail: twoOfTwoReady,
		},
		{
			Elapsed: someElapsed, Change: ChangeModified, Kind: DeploymentObject, Name: aWorkloadName,
			Detail: twoOfTwoReady,
		},
	}

	summarised := NewTimeline(recorded).Summarise()

	expect.Expect(t, summarised.Lines()).To(expect.Equal([]string{
		"Topology  2 of 2 Nodes Ready after " + lastNodeReadyAsRead,
		"Workload  " + aWorkloadName + ", " + twoOfTwoReady + " after " + workloadReadyAsRead,
		"  " + aPodName + "  Pending " + podsPendingAsRead + " -> Running on " + aNodeName + " " + firstPodRanAsRead,
		"  " + aSecondPodName + "  Pending " + podsPendingAsRead + " -> Running on " + aSecondNodeName + " " + secondPodRanAsRead,
		noPodLeftPending,
	}))
}

func TestAPodThatReachedRunning_ShouldGetALineWithItsPathAndTheNodeItLandedOn(t *testing.T) {
	recorded := []Entry{
		{
			Elapsed: whenTheLastNodeWasReady, Change: ChangeAdded, Kind: NodeObject, Name: aNodeName,
			Reason: string(NodeReady), Detail: someNodeCapacity,
		},
		{
			Elapsed: whenTheWorkloadLanded, Change: ChangeAdded, Kind: DeploymentObject, Name: aWorkloadName,
			Detail: oneOfOneReady,
		},
		{
			Elapsed: whenThePodsWerePending, Change: ChangeAdded, Kind: PodObject, Name: aPodName,
			Reason: string(PodPending),
		},
		{
			Elapsed: whenTheFirstPodRan, Change: ChangeModified, Kind: PodObject, Name: aPodName,
			Reason: string(PodRunning), Detail: aNodeName,
		},
	}

	summarised := NewTimeline(recorded).Summarise()

	expect.Expect(t, summarised.Lines()).To(expect.Equal([]string{
		"Topology  1 of 1 Nodes Ready after " + lastNodeReadyAsRead,
		"Workload  " + aWorkloadName + ", " + oneOfOneReady + " after " + workloadLandedAsRead,
		"  " + aPodName + "  Pending " + podsPendingAsRead + " -> Running on " + aNodeName + " " + firstPodRanAsRead,
		noPodLeftPending,
	}))
}

func TestAPodLeftPending_ShouldGetALineNamingTheControlPlaneReason(t *testing.T) {
	recorded := []Entry{
		{
			Elapsed: whenTheLastNodeWasReady, Change: ChangeAdded, Kind: NodeObject, Name: aNodeName,
			Reason: string(NodeReady), Detail: someNodeCapacity,
		},
		{
			Elapsed: whenTheWorkloadLanded, Change: ChangeAdded, Kind: DeploymentObject, Name: aWorkloadName,
			Detail: noneOfOneReady,
		},
		{
			Elapsed: whenThePodsWerePending, Change: ChangeAdded, Kind: PodObject, Name: aPodName,
			Reason: string(PodPending),
		},
		{
			Elapsed: whenTheFirstPodRan, Change: ChangeReported, Kind: PodObject, Name: aPodName,
			Reason: refusedReason, Detail: refusedMessage,
		},
	}

	summarised := NewTimeline(recorded).Summarise()

	expect.Expect(t, summarised.Lines()).To(expect.Equal([]string{
		"Topology  1 of 1 Nodes Ready after " + lastNodeReadyAsRead,
		"Workload  " + aWorkloadName + ", " + noneOfOneReady + " after " + workloadLandedAsRead,
		"  " + aPodName + "  Pending " + podsPendingAsRead + ", " + refusedReason + ": " + refusedMessage,
		"Pending   " + aPodName,
	}))
}

func TestAScenarioThatDeployedNoWorkload_ShouldBeSummarisedByItsTopologyAlone(t *testing.T) {
	recorded := []Entry{
		{
			Elapsed: whenTheFirstNodeWasReady, Change: ChangeAdded, Kind: NodeObject, Name: aNodeName,
			Reason: string(NodeReady), Detail: someNodeCapacity,
		},
		{
			Elapsed: whenTheLastNodeWasReady, Change: ChangeAdded, Kind: NodeObject, Name: aSecondNodeName,
			Reason: string(NodeReady), Detail: someNodeCapacity,
		},
		{
			Elapsed: someElapsed, Change: ChangeAdded, Kind: PodObject, Name: aProbePodName,
			Reason: string(PodRunning), Detail: aNodeName,
		},
	}

	summarised := NewTimeline(recorded).Summarise()

	expect.Expect(t, summarised.Lines()).To(expect.Equal([]string{
		"Topology  2 of 2 Nodes Ready after " + lastNodeReadyAsRead,
		noWorkloadDeployed,
		noPodLeftPending,
	}))
}

func TestATimelineOfAScenarioThatEndedEarly_ShouldSummariseTheTopologyItHolds(t *testing.T) {
	endings := map[string]struct {
		recorded []Entry
		reads    string
	}{
		"a Scenario that ended before a Node was brought up": {
			recorded: []Entry{{Elapsed: someElapsed, Change: ChangeNoted, Detail: aStreamGap}},
			reads:    "Topology  no Nodes",
		},
		"a Scenario whose Nodes never reported Ready": {
			recorded: []Entry{{
				Elapsed: whenTheFirstNodeWasReady, Change: ChangeAdded, Kind: NodeObject, Name: aNodeName,
				Reason: string(NodeNotReady), Detail: someNodeCapacity,
			}},
			reads: "Topology  0 of 1 Nodes Ready",
		},
	}

	for ending, topology := range endings {
		t.Run(ending, func(t *testing.T) {
			summarised := NewTimeline(topology.recorded).Summarise()

			expect.Expect(t, summarised.Lines()).To(expect.Equal([]string{
				topology.reads,
				noWorkloadDeployed,
				noPodLeftPending,
			}))
		})
	}
}

func TestAPodPlacedAgainAfterAFailure_ShouldGetALineWithItsWholePath(t *testing.T) {
	recorded := []Entry{
		{
			Elapsed: whenTheLastNodeWasReady, Change: ChangeAdded, Kind: NodeObject, Name: aNodeName,
			Reason: string(NodeReady), Detail: someNodeCapacity,
		},
		{
			Elapsed: whenTheWorkloadLanded, Change: ChangeAdded, Kind: DeploymentObject, Name: aWorkloadName,
			Detail: oneOfOneReady,
		},
		{
			Elapsed: whenThePodsWerePending, Change: ChangeAdded, Kind: PodObject, Name: aPodName,
			Reason: string(PodPending),
		},
		{
			Elapsed: whenTheFirstPodRan, Change: ChangeModified, Kind: PodObject, Name: aPodName,
			Reason: string(PodRunning), Detail: aNodeName,
		},
		{
			Elapsed: whenTheNodeWasLost, Change: ChangeModified, Kind: PodObject, Name: aPodName,
			Reason: string(PodPending),
		},
		{
			Elapsed: whenThePodRanAgain, Change: ChangeModified, Kind: PodObject, Name: aPodName,
			Reason: string(PodRunning), Detail: aSecondNodeName,
		},
	}

	summarised := NewTimeline(recorded).Summarise()

	expect.Expect(t, summarised.Lines()).To(expect.Equal([]string{
		"Topology  1 of 1 Nodes Ready after " + lastNodeReadyAsRead,
		"Workload  " + aWorkloadName + ", " + oneOfOneReady + " after " + workloadLandedAsRead,
		"  " + aPodName + "  Pending " + podsPendingAsRead +
			" -> Running on " + aNodeName + " " + firstPodRanAsRead +
			" -> Pending " + nodeLostAsRead +
			" -> Running on " + aSecondNodeName + " " + podRanAgainAsRead,
		noPodLeftPending,
	}))
}

func TestAScenarioWithTwoWorkloads_ShouldGiveEachWorkloadItsOwnCountAndPodLines(t *testing.T) {
	recorded := []Entry{
		{
			Elapsed: whenTheLastNodeWasReady, Change: ChangeAdded, Kind: NodeObject, Name: aNodeName,
			Reason: string(NodeReady), Detail: someNodeCapacity,
		},
		{
			Elapsed: whenTheWorkloadLanded, Change: ChangeAdded, Kind: DeploymentObject, Name: aWorkloadName,
			Detail: oneOfOneReady,
		},
		{
			Elapsed: whenTheWorkloadLanded, Change: ChangeAdded, Kind: DeploymentObject, Name: aNestedWorkloadName,
			Detail: oneOfOneReady,
		},
		{
			Elapsed: whenThePodsWerePending, Change: ChangeAdded, Kind: PodObject, Name: aPodOfTheNestedWorkload,
			Reason: string(PodPending),
		},
		{
			Elapsed: whenThePodsWerePending, Change: ChangeAdded, Kind: PodObject, Name: aPodName,
			Reason: string(PodPending),
		},
		{
			Elapsed: whenTheFirstPodRan, Change: ChangeModified, Kind: PodObject, Name: aPodOfTheNestedWorkload,
			Reason: string(PodRunning), Detail: aNodeName,
		},
		{
			Elapsed: whenTheSecondPodRan, Change: ChangeModified, Kind: PodObject, Name: aPodName,
			Reason: string(PodRunning), Detail: aSecondNodeName,
		},
	}

	summarised := NewTimeline(recorded).Summarise()

	expect.Expect(t, summarised.Lines()).To(expect.Equal([]string{
		"Topology  1 of 1 Nodes Ready after " + lastNodeReadyAsRead,
		"Workload  " + aWorkloadName + ", " + oneOfOneReady + " after " + workloadLandedAsRead,
		"  " + aPodName + "  Pending " + podsPendingAsRead + " -> Running on " + aSecondNodeName + " " + secondPodRanAsRead,
		"Workload  " + aNestedWorkloadName + ", " + oneOfOneReady + " after " + workloadLandedAsRead,
		"  " + aPodOfTheNestedWorkload + "  Pending " + podsPendingAsRead + " -> Running on " + aNodeName + " " + firstPodRanAsRead,
		noPodLeftPending,
	}))
}

func TestAnEntryNamingNoObjectTheSummaryGroupsBy_ShouldReadInTheEntryCountAlone(t *testing.T) {
	// The count is every entry the Timeline holds: the four grouped under a Node,
	// a Workload and a Pod below, and the fifth that is grouped under none.
	const everyEntryRecorded = 5

	ungrouped := map[string]Entry{
		"a ReplicaSet, which the Workload's own Deployment entries already name": {
			Elapsed: whenTheWorkloadLanded, Change: ChangeAdded, Kind: ReplicaSetObject,
			Name: aReplicaSetName, Detail: oneOfOneReady,
		},
		"a note the Timeline makes about itself, which names no object at all": {
			Elapsed: someElapsed, Change: ChangeNoted, Detail: aStreamGap,
		},
	}

	for ungroupedAs, entry := range ungrouped {
		t.Run(ungroupedAs, func(t *testing.T) {
			recorded := []Entry{
				{
					Elapsed: whenTheLastNodeWasReady, Change: ChangeAdded, Kind: NodeObject, Name: aNodeName,
					Reason: string(NodeReady), Detail: someNodeCapacity,
				},
				{
					Elapsed: whenTheWorkloadLanded, Change: ChangeAdded, Kind: DeploymentObject, Name: aWorkloadName,
					Detail: oneOfOneReady,
				},
				{
					Elapsed: whenThePodsWerePending, Change: ChangeAdded, Kind: PodObject, Name: aPodName,
					Reason: string(PodPending),
				},
				{
					Elapsed: whenTheFirstPodRan, Change: ChangeModified, Kind: PodObject, Name: aPodName,
					Reason: string(PodRunning), Detail: aNodeName,
				},
				entry,
			}

			summarised := NewTimeline(recorded).Summarise()

			expect.Expect(t, summarised.Lines()).To(expect.Equal([]string{
				"Topology  1 of 1 Nodes Ready after " + lastNodeReadyAsRead,
				"Workload  " + aWorkloadName + ", " + oneOfOneReady + " after " + workloadLandedAsRead,
				"  " + aPodName + "  Pending " + podsPendingAsRead + " -> Running on " + aNodeName + " " + firstPodRanAsRead,
				noPodLeftPending,
			}))
			expect.Expect(t, summarised.EntryCount()).To(expect.Equal(everyEntryRecorded))
		})
	}
}

// anEntry and aDifferentEntry are entries whose values do not matter to the test
// that asks for one, only that the two are not the same entry.
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
				Elapsed: someElapsed, Change: ChangeModified, Kind: DeploymentObject, Name: aWorkloadName,
				Detail: readyReplicas,
			},
			reads: elapsedAsRead + "  Deployment " + aWorkloadName + " modified, " + readyReplicas,
		},
		"a change carrying neither": {
			entry: Entry{Elapsed: someElapsed, Change: ChangeAdded, Kind: NodeObject, Name: aNodeName},
			reads: elapsedAsRead + "  Node " + aNodeName + " added",
		},
		"a note the Timeline makes about itself, naming no object": {
			entry: Entry{Elapsed: someElapsed, Change: ChangeNoted, Detail: aStreamGap},
			reads: elapsedAsRead + "  noted, " + aStreamGap,
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
