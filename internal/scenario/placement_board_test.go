package scenario_test

import (
	"slices"
	"testing"
	"unicode/utf8"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// The Topology of on-demand and spot Nodes gives each Zone one Node of each
// NodePool, in this order.
const (
	firstZone           = "zone-1"
	secondZone          = "zone-2"
	firstOnDemandNode   = "zone-1-node-1"
	firstSpotNode       = "zone-1-node-2"
	secondOnDemandNode  = "zone-2-node-1"
	secondSpotNode      = "zone-2-node-2"
	webWorkload         = "web"
	batchWorkload       = "batch"
	someReplicaSetHash  = "5d8f9b7c6"
	firstWebSuffix      = "2xk4m"
	secondWebSuffix     = "8qz7p"
	replacementSuffix   = "c4n2w"
	pendingWebSuffix    = "t6r9d"
	firstBatchSuffix    = "h3l5v"
	theNewcomerMark     = "+"
	markedNew           = true
	notMarkedNew        = false
	theBoardsNodeIndent = "  "
)

func aPodOf(workload, suffix, node string) scenario.Pod {
	return scenario.Pod{
		Name:     workload + "-" + someReplicaSetHash + "-" + suffix,
		Workload: workload,
		Node:     node,
		Phase:    scenario.PodRunning,
	}
}

func anUnplacedPodOf(workload, suffix string) scenario.Pod {
	return scenario.Pod{Name: workload + "-" + someReplicaSetHash + "-" + suffix, Workload: workload, Phase: scenario.PodPending}
}

func read(workload, suffix string) string {
	return workload + "-" + suffix
}

func readAsNew(workload, suffix string) string {
	return theNewcomerMark + read(workload, suffix)
}

// A board after one holding the same Pods marks none of them new, so only the
// grouping is left to read.
func aBoardAfterOneHoldingTheSame(topology scenario.Topology, pods []scenario.Pod) scenario.PlacementBoard {
	observed := observedOn(topology, pods)

	return scenario.NewPlacementBoard(topology, observed, scenario.NewPlacementBoard(topology, observed, scenario.PlacementBoard{}))
}

// observedOn reads every Node of the Topology Ready, as an observation of a
// cluster no Failure has touched does.
func observedOn(topology scenario.Topology, pods []scenario.Pod) scenario.Placement {
	return scenario.NewPlacement(topology.Nodes(), pods)
}

func requesting(pod scenario.Pod, request scenario.Request) scenario.Pod {
	pod.Request = request

	return pod
}

func readied(pod scenario.Pod) scenario.Pod {
	pod.Ready = true

	return pod
}

func shownAs(pod scenario.Pod, newcomer bool) scenario.BoardPod {
	return scenario.BoardPod{Name: pod.Name, Workload: pod.Workload, Ready: pod.Ready, Request: pod.Request, Newcomer: newcomer}
}

// A Node left out of the observation is one the control plane does not hold,
// so the board reads it NotReady beside the one the observation names NotReady.
func TestAPlacementBoard_ShouldGiveEachNodeItsNodePoolLabelsReadinessAndRoomCapacityRequestsAndLeft(t *testing.T) {
	declared := aTopologyOfOnDemandAndSpotNodes(t)
	capacity := aCapacity(t)
	const halfACore, aCore, aGibibyte, twoGibibytes = scenario.CPU(500), scenario.CPU(1000), scenario.Memory(1024), scenario.Memory(2048)
	const theTwoPodsOnIt, theOnePodOnIt = 2, 1
	webRequest := scenario.NewRequest(halfACore, aGibibyte)
	batchRequest := scenario.NewRequest(aCore, twoGibibytes)
	keptWeb := readied(requesting(aPodOf(webWorkload, firstWebSuffix, firstOnDemandNode), webRequest))
	startingBatch := requesting(aPodOf(batchWorkload, firstBatchSuffix, firstOnDemandNode), batchRequest)
	movedWeb := readied(requesting(aPodOf(webWorkload, secondWebSuffix, secondSpotNode), webRequest))
	pendingWeb := requesting(anUnplacedPodOf(webWorkload, pendingWebSuffix), webRequest)
	everyNodeButTheSecondOnDemand := slices.DeleteFunc(declared.Nodes(),
		func(node scenario.Node) bool { return node.Name == secondOnDemandNode })
	observed := scenario.NewPlacement(everyNodeButTheSecondOnDemand, []scenario.Pod{keptWeb, startingBatch, movedWeb, pendingWeb}).
		WithNodesNotReady([]string{firstSpotNode})
	previous := scenario.NewPlacementBoard(declared, observedOn(declared, []scenario.Pod{keptWeb}), scenario.PlacementBoard{})

	board := scenario.NewPlacementBoard(declared, observed, previous)

	onDemandLabels := labelsOf(t, map[string]string{capacityType: onDemand})
	spotLabels := labelsOf(t, map[string]string{capacityType: spot})
	untouched := scenario.RoomLeft{Pods: capacity.Pods(), CPU: capacity.CPU(), Memory: capacity.Memory()}
	expect.Expect(t, board.Zones()).To(expect.Equal([]scenario.BoardZone{
		{Name: firstZone, Nodes: []scenario.BoardNode{
			{
				Name: firstOnDemandNode, NodePool: onDemand, Labels: onDemandLabels, Readiness: scenario.NodeReady,
				Capacity: capacity,
				Requests: scenario.RoomTaken{
					Pods:   theTwoPodsOnIt,
					CPU:    webRequest.CPU() + batchRequest.CPU(),
					Memory: webRequest.Memory() + batchRequest.Memory(),
				},
				Left: scenario.RoomLeft{
					Pods:   capacity.Pods() - theTwoPodsOnIt,
					CPU:    capacity.CPU() - webRequest.CPU() - batchRequest.CPU(),
					Memory: capacity.Memory() - webRequest.Memory() - batchRequest.Memory(),
				},
				Pods: []scenario.BoardPod{shownAs(startingBatch, markedNew), shownAs(keptWeb, notMarkedNew)},
			},
			{
				Name: firstSpotNode, NodePool: spot, Labels: spotLabels, Readiness: scenario.NodeNotReady,
				Capacity: capacity, Left: untouched,
			},
		}},
		{Name: secondZone, Nodes: []scenario.BoardNode{
			{
				Name: secondOnDemandNode, NodePool: onDemand, Labels: onDemandLabels, Readiness: scenario.NodeNotReady,
				Capacity: capacity, Left: untouched,
			},
			{
				Name: secondSpotNode, NodePool: spot, Labels: spotLabels, Readiness: scenario.NodeReady,
				Capacity: capacity,
				Requests: scenario.RoomTaken{Pods: theOnePodOnIt, CPU: webRequest.CPU(), Memory: webRequest.Memory()},
				Left: scenario.RoomLeft{
					Pods:   capacity.Pods() - theOnePodOnIt,
					CPU:    capacity.CPU() - webRequest.CPU(),
					Memory: capacity.Memory() - webRequest.Memory(),
				},
				Pods: []scenario.BoardPod{shownAs(movedWeb, markedNew)},
			},
		}},
	}))
	expect.Expect(t, board.Unplaced()).To(expect.Equal([]scenario.BoardPod{shownAs(pendingWeb, markedNew)}))
}

func TestAPodOnAPlacementBoard_ShouldBeNamedByItsWorkloadAndOwnSuffix(t *testing.T) {
	pod := shownAs(aPodOf(webWorkload, firstWebSuffix, firstOnDemandNode), notMarkedNew)

	expect.Expect(t, pod.BoardName()).To(expect.Equal(read(webWorkload, firstWebSuffix)))
}

func TestAPlacementBoardOfTwoWorkloads_ShouldListEveryPodOnItsNodeGroupedByZoneAndNodePool(t *testing.T) {
	declared := aTopologyOfOnDemandAndSpotNodes(t)
	observed := []scenario.Pod{
		aPodOf(webWorkload, firstWebSuffix, firstOnDemandNode),
		aPodOf(webWorkload, secondWebSuffix, secondSpotNode),
		aPodOf(batchWorkload, firstBatchSuffix, firstOnDemandNode),
	}

	board := aBoardAfterOneHoldingTheSame(declared, observed)

	expect.Expect(t, board.Lines()).To(expect.Equal([]string{
		firstZone,
		theBoardsNodeIndent + firstOnDemandNode + "  " + onDemand + "  " +
			read(batchWorkload, firstBatchSuffix) + ", " + read(webWorkload, firstWebSuffix),
		theBoardsNodeIndent + firstSpotNode + "  " + spot,
		secondZone,
		theBoardsNodeIndent + secondOnDemandNode + "  " + onDemand,
		theBoardsNodeIndent + secondSpotNode + "  " + spot + "       " + read(webWorkload, secondWebSuffix),
	}))
}

func TestAPlacementBoard_ShouldBeAsWideAsItsWidestLine(t *testing.T) {
	boards := map[string]struct {
		observed      []scenario.Pod
		theWidestLine string
	}{
		"the first Node line widest": {
			observed:      []scenario.Pod{aPodOf(webWorkload, firstWebSuffix, firstOnDemandNode)},
			theWidestLine: theBoardsNodeIndent + firstOnDemandNode + "  " + onDemand + "  " + readAsNew(webWorkload, firstWebSuffix),
		},
		"a later Node line widest": {
			observed: []scenario.Pod{
				aPodOf(webWorkload, firstWebSuffix, firstOnDemandNode),
				aPodOf(webWorkload, secondWebSuffix, secondSpotNode),
				aPodOf(batchWorkload, firstBatchSuffix, secondSpotNode),
			},
			theWidestLine: theBoardsNodeIndent + secondSpotNode + "  " + spot + "       " +
				readAsNew(batchWorkload, firstBatchSuffix) + ", " + readAsNew(webWorkload, secondWebSuffix),
		},
		"the Pending line widest": {
			observed: []scenario.Pod{
				aPodOf(webWorkload, firstWebSuffix, firstOnDemandNode),
				anUnplacedPodOf(webWorkload, pendingWebSuffix),
				anUnplacedPodOf(webWorkload, replacementSuffix),
				anUnplacedPodOf(batchWorkload, firstBatchSuffix),
			},
			theWidestLine: "Pending  " + readAsNew(batchWorkload, firstBatchSuffix) + ", " +
				readAsNew(webWorkload, replacementSuffix) + ", " + readAsNew(webWorkload, pendingWebSuffix),
		},
	}

	for widest, board := range boards {
		t.Run(widest, func(t *testing.T) {
			declared := aTopologyOfOnDemandAndSpotNodes(t)
			drawn := scenario.NewPlacementBoard(declared, observedOn(declared, board.observed), scenario.PlacementBoard{})

			expect.Expect(t, slices.Contains(drawn.Lines(), board.theWidestLine)).To(expect.Equal(true))
			expect.Expect(t, drawn.Width()).To(expect.Equal(utf8.RuneCountInString(board.theWidestLine)))
		})
	}
}

func TestTheFirstBoardHoldingAWorkload_ShouldMarkEveryOneOfItsPodsNew(t *testing.T) {
	declared := aTopologyOfOnDemandAndSpotNodes(t)
	observed := []scenario.Pod{
		aPodOf(webWorkload, firstWebSuffix, firstOnDemandNode),
		aPodOf(webWorkload, secondWebSuffix, secondSpotNode),
		anUnplacedPodOf(webWorkload, pendingWebSuffix),
	}

	board := scenario.NewPlacementBoard(declared, observedOn(declared, observed), scenario.PlacementBoard{})

	expect.Expect(t, board.Lines()).To(expect.Equal([]string{
		firstZone,
		theBoardsNodeIndent + firstOnDemandNode + "  " + onDemand + "  " + readAsNew(webWorkload, firstWebSuffix),
		theBoardsNodeIndent + firstSpotNode + "  " + spot,
		secondZone,
		theBoardsNodeIndent + secondOnDemandNode + "  " + onDemand,
		theBoardsNodeIndent + secondSpotNode + "  " + spot + "       " + readAsNew(webWorkload, secondWebSuffix),
		"Pending  " + readAsNew(webWorkload, pendingWebSuffix),
	}))
}

// A Pod is new where the previous board did not hold it on that Node, so an
// evicted Pod is gone and its replacement reads as new, as does the first Pod
// of a Workload deployed since.
func TestAPlacementBoard_ShouldMarkThePodsNewSinceThePreviousBoardAndListPendingPodsBeneath(t *testing.T) {
	declared := aTopologyOfOnDemandAndSpotNodes(t)
	kept := aPodOf(webWorkload, firstWebSuffix, firstOnDemandNode)
	evicted := aPodOf(webWorkload, secondWebSuffix, secondOnDemandNode)
	previous := scenario.NewPlacementBoard(declared, observedOn(declared, []scenario.Pod{kept, evicted}), scenario.PlacementBoard{})
	observed := []scenario.Pod{
		kept,
		aPodOf(webWorkload, replacementSuffix, firstSpotNode),
		aPodOf(batchWorkload, firstBatchSuffix, secondSpotNode),
		anUnplacedPodOf(webWorkload, pendingWebSuffix),
	}

	board := scenario.NewPlacementBoard(declared, observedOn(declared, observed), previous)

	expect.Expect(t, board.Lines()).To(expect.Equal([]string{
		firstZone,
		theBoardsNodeIndent + firstOnDemandNode + "  " + onDemand + "  " + read(webWorkload, firstWebSuffix),
		theBoardsNodeIndent + firstSpotNode + "  " + spot + "       " + readAsNew(webWorkload, replacementSuffix),
		secondZone,
		theBoardsNodeIndent + secondOnDemandNode + "  " + onDemand,
		theBoardsNodeIndent + secondSpotNode + "  " + spot + "       " + readAsNew(batchWorkload, firstBatchSuffix),
		"Pending  " + readAsNew(webWorkload, pendingWebSuffix),
	}))
}

func TestAPlacementBoardWithNoPodPlaced_ShouldStillGiveEveryNodeItsLine(t *testing.T) {
	declared := aTopologyOfOnDemandAndSpotNodes(t)

	board := scenario.NewPlacementBoard(declared, scenario.Placement{}, scenario.PlacementBoard{})

	expect.Expect(t, board.Lines()).To(expect.Equal([]string{
		firstZone,
		theBoardsNodeIndent + firstOnDemandNode + "  " + onDemand,
		theBoardsNodeIndent + firstSpotNode + "  " + spot,
		secondZone,
		theBoardsNodeIndent + secondOnDemandNode + "  " + onDemand,
		theBoardsNodeIndent + secondSpotNode + "  " + spot,
	}))
}
