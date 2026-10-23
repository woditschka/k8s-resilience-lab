package scenario_test

import (
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

const (
	fullestNode  = "zone-1-node-1"
	emptiestNode = "zone-1-node-2"
	nextNode     = "zone-2-node-1"
	lastNode     = "zone-2-node-2"

	fullestZone = "zone-1"
	nextZone    = "zone-2"
)

const (
	firstPodOnTheFullestNode  = "web-1"
	secondPodOnTheFullestNode = "web-2"
	podOnTheNextNode          = "web-3"
	podOnNoNodeAtAll          = "web-4"
)

// Enough Nodes for one to stay empty and for two Zones to differ.
func twoZonesOfTwoNodes(t *testing.T) []scenario.Node {
	t.Helper()

	const twoZones, twoNodesPerZone = 2, 2
	topology, err := topologyOfTheGeneralNodePool(twoZones, twoNodesPerZone, aCapacity(t))
	if err != nil {
		t.Fatalf("build a Topology of %d Zones of %d Nodes: %v", twoZones, twoNodesPerZone, err)
	}

	return topology.Nodes()
}

// Every count below is distinguishable from every other.
func threePlacedPodsOverTwoZones() []scenario.Pod {
	return []scenario.Pod{
		{Name: firstPodOnTheFullestNode, Node: fullestNode, Phase: scenario.PodRunning},
		{Name: secondPodOnTheFullestNode, Node: fullestNode, Phase: scenario.PodRunning},
		{Name: podOnTheNextNode, Node: nextNode, Phase: scenario.PodRunning},
	}
}

func TestAPlacement_ShouldCountEveryNodeOfTheObservationIncludingTheEmptyOnes(t *testing.T) {
	placement := scenario.NewPlacement(twoZonesOfTwoNodes(t), threePlacedPodsOverTwoZones())

	expect.Expect(t, placement.PodsPerNode()).To(expect.Equal(map[string]int{
		fullestNode:  2,
		emptiestNode: 0,
		nextNode:     1,
		lastNode:     0,
	}))
}

func TestAPlacement_ShouldCountEveryZoneOfTheObservationIncludingTheEmptyOnes(t *testing.T) {
	const oneZoneOfOneNode, oneNodePerZone = 1, 1
	emptyZoneTopology, err := topologyOfTheGeneralNodePool(oneZoneOfOneNode, oneNodePerZone, aCapacity(t))
	if err != nil {
		t.Fatalf("build a Topology whose only Zone stays empty: %v", err)
	}

	placement := scenario.NewPlacement(emptyZoneTopology.Nodes(), nil)

	expect.Expect(t, placement.PodsPerZone()).To(expect.Equal(map[string]int{fullestZone: 0}))
}

func TestAPlacementSkew_ShouldBeTheGapBetweenTheFullestAndTheEmptiestDomain(t *testing.T) {
	placement := scenario.NewPlacement(twoZonesOfTwoNodes(t), threePlacedPodsOverTwoZones())

	podsInTheFullestZone, podsInTheNextZone := 2, 1
	expect.Expect(t, placement.Skew(scenario.SpreadOverZones)).To(expect.Equal(podsInTheFullestZone - podsInTheNextZone))

	podsOnTheFullestNode, podsOnTheEmptiestNode := 2, 0
	expect.Expect(t, placement.Skew(scenario.SpreadOverNodes)).To(expect.Equal(podsOnTheFullestNode - podsOnTheEmptiestNode))
}

func TestAnUnplacedPod_ShouldBeCountedOnNoNodeAndInNoZone(t *testing.T) {
	unplaced := scenario.Pod{Name: podOnNoNodeAtAll, Phase: scenario.PodPending}
	placement := scenario.NewPlacement(twoZonesOfTwoNodes(t), append(threePlacedPodsOverTwoZones(), unplaced))

	expect.Expect(t, placement.PodsPerZone()).To(expect.Equal(map[string]int{fullestZone: 2, nextZone: 1}))
	expect.Expect(t, unplaced.Placed()).To(expect.Equal(false))
}

func TestAPodOnANodeTheObservationDoesNotHold_ShouldBeCountedInNoZone(t *testing.T) {
	const nodeOutsideTheObservation = "zone-9-node-1"
	strayPod := scenario.Pod{Name: podOnNoNodeAtAll, Node: nodeOutsideTheObservation, Phase: scenario.PodRunning}

	placement := scenario.NewPlacement(twoZonesOfTwoNodes(t), []scenario.Pod{strayPod})

	expect.Expect(t, placement.PodsPerNode()[nodeOutsideTheObservation]).To(expect.Equal(1))
	expect.Expect(t, placement.PodsPerZone()).To(expect.Equal(map[string]int{fullestZone: 0, nextZone: 0}))
}

func TestAPodOnANodeTheObservationDoesNotHold_ShouldBeReportedAsUnaccountedFor(t *testing.T) {
	const nodeThatLeftTheObservation = "zone-9-node-1"
	stillOnTheNodeThatLeft := scenario.Pod{Name: podOnNoNodeAtAll, Node: nodeThatLeftTheObservation, Phase: scenario.PodRunning}
	unplaced := scenario.Pod{Name: podOnTheNextNode, Phase: scenario.PodPending}

	placement := scenario.NewPlacement(twoZonesOfTwoNodes(t), append(threePlacedPodsOverTwoZones(), stillOnTheNodeThatLeft, unplaced))

	expect.Expect(t, placement.PodsItCannotAccountFor()).To(expect.Equal([]scenario.Pod{stillOnTheNodeThatLeft}))
}

func TestAPlacementOfPlacedAndUnplacedPodsAlone_ShouldAccountForEveryOne(t *testing.T) {
	unplaced := scenario.Pod{Name: podOnNoNodeAtAll, Phase: scenario.PodPending}

	placement := scenario.NewPlacement(twoZonesOfTwoNodes(t), append(threePlacedPodsOverTwoZones(), unplaced))

	expect.Expect(t, placement.PodsItCannotAccountFor()).To(expect.HaveLen[scenario.Pod](0))
}

func TestAPlacement_ShouldReportThePodsOfOnePhaseAlone(t *testing.T) {
	pending := scenario.Pod{Name: podOnNoNodeAtAll, Phase: scenario.PodPending}
	placement := scenario.NewPlacement(twoZonesOfTwoNodes(t), append(threePlacedPodsOverTwoZones(), pending))

	expect.Expect(t, placement.PodsInPhase(scenario.PodRunning)).To(expect.Equal(threePlacedPodsOverTwoZones()))
	expect.Expect(t, placement.PodsInPhase(scenario.PodPending)).To(expect.Equal([]scenario.Pod{pending}))
}

func TestAPlacement_ShouldNameThePodsItDoesNotReportReady(t *testing.T) {
	ready := scenario.Pod{Name: firstPodOnTheFullestNode, Node: fullestNode, Phase: scenario.PodRunning, Ready: true}
	notReadyYet := scenario.Pod{Name: podOnTheNextNode, Node: nextNode, Phase: scenario.PodRunning}
	pending := scenario.Pod{Name: podOnNoNodeAtAll, Phase: scenario.PodPending}

	placement := scenario.NewPlacement(twoZonesOfTwoNodes(t), []scenario.Pod{ready, notReadyYet, pending})

	expect.Expect(t, placement.PodsNotReady()).To(expect.Equal([]scenario.Pod{notReadyYet, pending}))
}

func TestTwoObservationsOfTheSamePodsOnTheSameNodes_ShouldAgree(t *testing.T) {
	firstObservation := scenario.NewPlacement(twoZonesOfTwoNodes(t), threePlacedPodsOverTwoZones())
	secondObservation := scenario.NewPlacement(twoZonesOfTwoNodes(t), threePlacedPodsOverTwoZones())

	expect.Expect(t, secondObservation).To(expect.Equal(firstObservation))
}

func TestAPodInsideItsStartupTime_ShouldBeCountedPendingOnItsNode(t *testing.T) {
	const theStartingPodAlone = 1
	starting := scenario.Pod{Name: podOnTheNextNode, Node: nextNode, Phase: scenario.PodPending}
	unplaced := scenario.Pod{Name: podOnNoNodeAtAll, Phase: scenario.PodPending}
	running := scenario.Pod{Name: firstPodOnTheFullestNode, Node: fullestNode, Phase: scenario.PodRunning}

	placement := scenario.NewPlacement(twoZonesOfTwoNodes(t), []scenario.Pod{running, starting, unplaced})

	expect.Expect(t, placement.PodsStarting()).To(expect.Equal([]scenario.Pod{starting}))
	expect.Expect(t, placement.PodsPerNode()[nextNode]).To(expect.Equal(theStartingPodAlone))
	expect.Expect(t, placement.PodsInPhase(scenario.PodPending)).To(expect.Equal([]scenario.Pod{starting, unplaced}))
}

func TestAPlacement_ShouldCountPodsPerValueOfALabelTheirNodesCarry(t *testing.T) {
	const oneZone, oneNodePerZone = 1, 1
	topology, err := scenario.NewTopology(oneZone,
		labelledNodePool(t, onDemand, labelsOf(t, map[string]string{capacityType: onDemand})),
		labelledNodePool(t, spot, labelsOf(t, map[string]string{capacityType: spot})),
		aNodePool(t, theGeneralNodePool, oneNodePerZone, aCapacity(t)),
	)
	if err != nil {
		t.Fatalf("build a Topology of labelled NodePools: %v", err)
	}
	const onTheOnDemandNode, onTheUnlabelledNode, onANodeNotObserved = "zone-1-node-1", "zone-1-node-3", "zone-9-node-1"
	const podLeftUnplaced = "web-5"
	placed := scenario.NewPlacement(topology.Nodes(), []scenario.Pod{
		{Name: firstPodOnTheFullestNode, Node: onTheOnDemandNode, Phase: scenario.PodRunning},
		{Name: secondPodOnTheFullestNode, Node: onTheOnDemandNode, Phase: scenario.PodRunning},
		{Name: podOnTheNextNode, Node: onTheUnlabelledNode, Phase: scenario.PodRunning},
		{Name: podOnNoNodeAtAll, Node: onANodeNotObserved, Phase: scenario.PodRunning},
		{Name: podLeftUnplaced, Phase: scenario.PodPending},
	})

	expect.Expect(t, placed.PodsPerLabelValue(capacityType)).To(expect.Equal(map[string]int{onDemand: 2, spot: 0}))
}

func TestAPlacement_ShouldCountPodsPerNodePoolOfTheirNodes(t *testing.T) {
	const oneZone, oneNodePerZone, twoNodesPerZone, gpu = 1, 1, 2, "gpu"
	topology, err := scenario.NewTopology(oneZone,
		aNodePool(t, theGeneralNodePool, twoNodesPerZone, aCapacity(t)),
		aNodePool(t, gpu, oneNodePerZone, aCapacity(t)),
	)
	if err != nil {
		t.Fatalf("build a Topology of two NodePools: %v", err)
	}
	const onTheFirstGeneralNode, onTheSecondGeneralNode, onANodeNotObserved = "zone-1-node-1", "zone-1-node-2", "zone-9-node-1"
	placed := scenario.NewPlacement(topology.Nodes(), []scenario.Pod{
		{Name: firstPodOnTheFullestNode, Node: onTheFirstGeneralNode, Phase: scenario.PodRunning},
		{Name: podOnTheNextNode, Node: onTheSecondGeneralNode, Phase: scenario.PodRunning},
		{Name: podOnNoNodeAtAll, Node: onANodeNotObserved, Phase: scenario.PodRunning},
		{Name: secondPodOnTheFullestNode, Phase: scenario.PodPending},
	})

	expect.Expect(t, placed.PodsPerNodePool()).To(expect.Equal(map[string]int{theGeneralNodePool: 2, gpu: 0}))
}

func TestTwoObservationsThatDifferInTheNodesNotReadyAlone_ShouldNotAgreeUnlessTheyNameTheSameNodes(t *testing.T) {
	allReady := scenario.NewPlacement(twoZonesOfTwoNodes(t), threePlacedPodsOverTwoZones())
	oneNotReady := allReady.WithNodesNotReady([]string{fullestNode})
	twoNotReady := allReady.WithNodesNotReady([]string{fullestNode, nextNode})
	twoNotReadyInAnotherOrder := allReady.WithNodesNotReady([]string{nextNode, fullestNode})

	expect.Expect(t, allReady.Equal(oneNotReady)).To(expect.Equal(false))
	expect.Expect(t, oneNotReady.Equal(twoNotReady)).To(expect.Equal(false))
	expect.Expect(t, twoNotReady.Equal(twoNotReadyInAnotherOrder)).To(expect.Equal(true))
}
