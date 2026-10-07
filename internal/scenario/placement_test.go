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

// twoZonesOfTwoNodes is the named default for the Nodes an observation is read
// against: enough Nodes for one to stay empty and for two Zones to differ.
func twoZonesOfTwoNodes(t *testing.T) []scenario.Node {
	t.Helper()

	const twoZones, twoNodesPerZone = 2, 2
	topology, err := scenario.NewTopology(twoZones, twoNodesPerZone, aCapacity(t))
	if err != nil {
		t.Fatalf("build a Topology of %d Zones of %d Nodes: %v", twoZones, twoNodesPerZone, err)
	}

	return topology.Nodes()
}

// threePlacedPodsOverTwoZones leaves two Nodes of four empty and the two Zones
// one Pod apart, so every count below is distinguishable from every other.
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
	emptyZoneTopology, err := scenario.NewTopology(oneZoneOfOneNode, oneNodePerZone, aCapacity(t))
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

// A Wait for every Pod Ready ends the Scenario naming the Pods it did not reach,
// so a Placement reports the ones it does not hold Ready (docs/prd.md
// REQ-SIM-006).
func TestAPlacement_ShouldNameThePodsItDoesNotReportReady(t *testing.T) {
	ready := scenario.Pod{Name: firstPodOnTheFullestNode, Node: fullestNode, Phase: scenario.PodRunning, Ready: true}
	notReadyYet := scenario.Pod{Name: podOnTheNextNode, Node: nextNode, Phase: scenario.PodRunning}
	pending := scenario.Pod{Name: podOnNoNodeAtAll, Phase: scenario.PodPending}

	placement := scenario.NewPlacement(twoZonesOfTwoNodes(t), []scenario.Pod{ready, notReadyYet, pending})

	expect.Expect(t, placement.PodsNotReady()).To(expect.Equal([]scenario.Pod{notReadyYet, pending}))
}

// What a Workload kept at the moment a Failure stopped a Node is read off the
// observation taken before it: the Pods Running anywhere but on the Nodes that
// stopped (docs/prd.md REQ-SIM-010).
func TestAPlacement_ShouldReportThePodsRunningApartFromTheNodesNamed(t *testing.T) {
	placement := scenario.NewPlacement(twoZonesOfTwoNodes(t), threePlacedPodsOverTwoZones())

	expect.Expect(t, placement.PodsRunningApartFrom([]string{fullestNode})).To(expect.Equal([]scenario.Pod{
		{Name: podOnTheNextNode, Node: nextNode, Phase: scenario.PodRunning},
	}))
}

func TestAPodThatIsNotRunningOnARespondingNode_ShouldBeAmongNoneOfThem(t *testing.T) {
	pending := scenario.Pod{Name: podOnNoNodeAtAll, Phase: scenario.PodPending}
	onTheNodeThatStopped := scenario.Pod{Name: firstPodOnTheFullestNode, Node: fullestNode, Phase: scenario.PodRunning}
	placement := scenario.NewPlacement(twoZonesOfTwoNodes(t), []scenario.Pod{pending, onTheNodeThatStopped})

	expect.Expect(t, placement.PodsRunningApartFrom([]string{fullestNode})).To(expect.HaveLen[scenario.Pod](0))
}

func TestTwoObservationsOfTheSamePodsOnTheSameNodes_ShouldAgree(t *testing.T) {
	firstObservation := scenario.NewPlacement(twoZonesOfTwoNodes(t), threePlacedPodsOverTwoZones())
	secondObservation := scenario.NewPlacement(twoZonesOfTwoNodes(t), threePlacedPodsOverTwoZones())

	expect.Expect(t, secondObservation).To(expect.Equal(firstObservation))
}
