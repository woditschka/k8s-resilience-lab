package scenario_test

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// More than one Node per Zone, so the first Nodes of the derivation and the
// first Nodes of a Zone are not the same set.
const (
	twoZones        = 2
	twoNodesPerZone = 2

	everyNode = twoZones * twoNodesPerZone

	everyZone = twoZones

	oneNode  = 1
	twoNodes = 2
	oneZone  = 1
)

func aTopologyOfTwoZones(t *testing.T) scenario.Topology {
	t.Helper()

	topology, err := topologyOfTheGeneralNodePool(twoZones, twoNodesPerZone, aCapacity(t))
	if err != nil {
		t.Fatalf("build a Topology of %d Zones of %d Nodes: %v", twoZones, twoNodesPerZone, err)
	}

	return topology
}

func aFailureOf(t *testing.T, topology scenario.Topology, count int) scenario.NodeFailure {
	t.Helper()

	failure, err := scenario.NewNodeFailure(topology, count)
	if err != nil {
		t.Fatalf("build a Failure stopping %d Nodes: %v", count, err)
	}

	return failure
}

func aZoneFailureOf(t *testing.T, topology scenario.Topology, count int) scenario.ZoneFailure {
	t.Helper()

	failure, err := scenario.NewZoneFailure(topology, count)
	if err != nil {
		t.Fatalf("build a Failure stopping %d Zones: %v", count, err)
	}

	return failure
}

// Derived here rather than written out, so the expectation follows the Topology
// asked.
func theFirstZonesOf(topology scenario.Topology, count int) []string {
	return topology.Zones()[:count]
}

func theNodesOfTheFirstZonesOf(topology scenario.Topology, count int) []string {
	stopping := theFirstZonesOf(topology, count)

	var names []string
	for _, node := range topology.Nodes() {
		if slices.Contains(stopping, node.Zone) {
			names = append(names, node.Name)
		}
	}

	return names
}

// Derived here rather than written out, so the expectation follows the Topology
// asked rather than repeating its naming rule.
func theFirstNamesOf(topology scenario.Topology, count int) []string {
	derived := topology.Nodes()[:count]
	names := make([]string, 0, count)
	for _, node := range derived {
		names = append(names, node.Name)
	}

	return names
}

func TestANodeFailure_ShouldStopTheFirstNodesItsTopologyDerives(t *testing.T) {
	declared := aTopologyOfTwoZones(t)

	failure := aFailureOf(t, declared, twoNodes)

	expect.Expect(t, failure.Names()).To(expect.Equal(theFirstNamesOf(declared, twoNodes)))
}

func TestTwoFailuresOfTheSameCount_ShouldStopTheSameNodes(t *testing.T) {
	declared := aTopologyOfTwoZones(t)

	firstRun, secondRun := aFailureOf(t, declared, oneNode), aFailureOf(t, declared, oneNode)

	expect.Expect(t, secondRun.Names()).To(expect.Equal(firstRun.Names()))
}

// A Zone Failure stops every Node the Zones hold rather than the first the
// Topology derives, which makes a Zone the unit it fails.
func TestAZoneFailure_ShouldStopEveryNodeOfThatZoneResponding(t *testing.T) {
	declared := aTopologyOfTwoZones(t)

	failure := aZoneFailureOf(t, declared, oneZone)

	expect.Expect(t, failure.Names()).To(expect.Equal(theFirstZonesOf(declared, oneZone)))
	expect.Expect(t, failure.NodeNames()).To(expect.Equal(theNodesOfTheFirstZonesOf(declared, oneZone)))
}

func TestTwoZoneFailuresOfTheSameCount_ShouldStopTheSameZones(t *testing.T) {
	declared := aTopologyOfTwoZones(t)

	firstRun, secondRun := aZoneFailureOf(t, declared, oneZone), aZoneFailureOf(t, declared, oneZone)

	expect.Expect(t, secondRun.Names()).To(expect.Equal(firstRun.Names()))
}

func TestANodeFailure_ShouldRefuseACountItsTopologyCannotSatisfy(t *testing.T) {
	refusals := map[string]struct {
		count int
		named string
	}{
		"one Node more than the Topology holds": {count: everyNode + 1, named: "zone-2-node-2"},
		"no Node at all":                        {count: 0, named: strconv.Itoa(0)},
		"a negative Node count":                 {count: -1, named: strconv.Itoa(-1)},
	}

	for refused, refusal := range refusals {
		t.Run(refused, func(t *testing.T) {
			_, err := scenario.NewNodeFailure(aTopologyOfTwoZones(t), refusal.count)

			if err == nil {
				t.Fatalf("a Failure of %d Nodes is refused, but it was admitted", refusal.count)
			}
			if !strings.Contains(err.Error(), refusal.named) {
				t.Errorf("the refusal names %q, but reads: %v", refusal.named, err)
			}
		})
	}
}

func TestAZoneFailure_ShouldRefuseACountItsTopologyCannotSatisfy(t *testing.T) {
	refusals := map[string]struct {
		count int
		named string
	}{
		"one Zone more than the Topology holds": {count: everyZone + 1, named: "zone-2"},
		"no Zone at all":                        {count: 0, named: strconv.Itoa(0)},
		"a negative Zone count":                 {count: -1, named: strconv.Itoa(-1)},
	}

	for refused, refusal := range refusals {
		t.Run(refused, func(t *testing.T) {
			_, err := scenario.NewZoneFailure(aTopologyOfTwoZones(t), refusal.count)

			if err == nil {
				t.Fatalf("a Failure of %d Zones is refused, but it was admitted", refusal.count)
			}
			if !strings.Contains(err.Error(), refusal.named) {
				t.Errorf("the refusal names %q, but reads: %v", refusal.named, err)
			}
		})
	}
}

const (
	aNameTheTopologyDoesNotDerive = "zone-9-node-9"
	// onAnyNode is irrelevant to a Pod's name, which is all a Pod Failure reads.
	onAnyNode = ""
)

func TestANodeFailureNamingANodeOfItsTopology_ShouldStopThatNodeAlone(t *testing.T) {
	declared := aTopologyOfTwoZones(t)
	named := declared.Nodes()[everyNode-1].Name

	failure, err := scenario.NewNodeFailureNaming(declared, named)

	expect.Expect(t, err).To(expect.Equal[error](nil))
	expect.Expect(t, failure.Names()).To(expect.Equal([]string{named}))
}

func TestAZoneFailureNamingAZoneOfItsTopology_ShouldStopEveryNodeOfThatZoneAlone(t *testing.T) {
	declared := aTopologyOfTwoZones(t)
	named := declared.Zones()[everyZone-1]

	failure, err := scenario.NewZoneFailureNaming(declared, named)

	expect.Expect(t, err).To(expect.Equal[error](nil))
	expect.Expect(t, [][]string{failure.Names(), failure.NodeNames()}).
		To(expect.Equal([][]string{{named}, nodeNamesOfZone(declared, named)}))
}

func TestAFailureNamingANodeOrZoneItsTopologyDoesNotDerive_ShouldBeRefusedNamingWhatItHolds(t *testing.T) {
	declared := aTopologyOfTwoZones(t)
	_, nodeRefused := scenario.NewNodeFailureNaming(declared, aNameTheTopologyDoesNotDerive)
	_, zoneRefused := scenario.NewZoneFailureNaming(declared, aNameTheTopologyDoesNotDerive)

	expect.Expect(t, []bool{
		nodeRefused != nil && strings.Contains(nodeRefused.Error(), declared.Nodes()[0].Name),
		zoneRefused != nil && strings.Contains(zoneRefused.Error(), declared.Zones()[0]),
	}).To(expect.Equal([]bool{true, true}))
}

// A Zone some of whose Nodes already stopped still has the others to stop.
func TestAZoneFailure_ShouldGiveTheNodesOfItsZoneNotYetStoppedInTopologyOrder(t *testing.T) {
	declared := aTopologyOfTwoZones(t)
	zone := declared.Zones()[0]
	ofTheZone := nodeNamesOfZone(declared, zone)
	failure, err := scenario.NewZoneFailureNaming(declared, zone)
	if err != nil {
		t.Fatalf("build a Failure of the Zone %s: %v", zone, err)
	}

	stillResponding := failure.NodeNamesStillResponding(ofTheZone[:1])

	expect.Expect(t, stillResponding).To(expect.Equal(ofTheZone[1:]))
}

func nodeNamesOfZone(topology scenario.Topology, zone string) []string {
	var names []string
	for _, node := range topology.Nodes() {
		if node.Zone == zone {
			names = append(names, node.Name)
		}
	}

	return names
}

func aBoardOfOnePlacedAndOnePendingPod(t *testing.T) scenario.PlacementBoard {
	t.Helper()

	declared := aTopologyOfTwoZones(t)
	pods := []scenario.Pod{
		aPodOf(webDeployment, firstWebSuffix, declared.Nodes()[0].Name),
		anUnplacedPodOf(webDeployment, pendingWebSuffix),
	}

	return scenario.NewPlacementBoard(declared, observedOn(declared, pods), scenario.PlacementBoard{})
}

func TestAPodFailureNamingAPlacedPodOfTheBoard_ShouldLoseThatPod(t *testing.T) {
	placed := aPodOf(webDeployment, firstWebSuffix, onAnyNode).Name

	failure, err := scenario.NewPodFailure(aBoardOfOnePlacedAndOnePendingPod(t), placed)

	expect.Expect(t, err).To(expect.Equal[error](nil))
	expect.Expect(t, failure.Name()).To(expect.Equal(placed))
}

func TestAClickedPodFailuresWhen_ShouldNameThePodByDeploymentNameAndOwnSuffix(t *testing.T) {
	placed := aPodOf(webDeployment, firstWebSuffix, onAnyNode).Name

	failure, err := scenario.NewPodFailure(aBoardOfOnePlacedAndOnePendingPod(t), placed)

	expect.Expect(t, err).To(expect.Equal[error](nil))
	expect.Expect(t, failure.BoardName()).To(expect.Equal(webDeployment + "-" + firstWebSuffix))
}

func TestAPodFailureNamingAPendingPodOrOneTheBoardDoesNotHold_ShouldBeRefused(t *testing.T) {
	refusals := map[string]string{
		"a Pending Pod":                 anUnplacedPodOf(webDeployment, pendingWebSuffix).Name,
		"a Pod the board does not hold": aPodOf(batchDeployment, firstBatchSuffix, onAnyNode).Name,
	}

	for refused, named := range refusals {
		t.Run(refused, func(t *testing.T) {
			_, err := scenario.NewPodFailure(aBoardOfOnePlacedAndOnePendingPod(t), named)

			expect.Expect(t, err != nil && strings.Contains(err.Error(), named)).To(expect.Equal(true))
		})
	}
}

const (
	aNodeAfterTheFirst = 1
	// The refusal of no Node stopped names it so, whichever Whens refuse it.
	noNodeStopped = "no Node"
)

// The Nodes stopped are given out of the Topology's order, as two Whens may
// stop them, so the order resolved is the Topology's own.
func TestNodesRespondingAgain_ShouldBeEveryStoppedNodeInTheTopologysOrder(t *testing.T) {
	declared := aTopologyOfTwoZones(t)
	nodes := theNodeNamesOf(declared)
	earlier, later := nodes[aNodeAfterTheFirst], nodes[everyNode-1]

	responding, err := scenario.NewNodesRespondingAgain(declared, []string{later, earlier})

	expect.Expect(t, err).To(expect.Equal[error](nil))
	expect.Expect(t, responding.Names()).To(expect.Equal([]string{earlier, later}))
}

func TestZonesRespondingAgain_ShouldBeEveryZoneWhoseNodesAllStoppedWithItsNodesInTheTopologysOrder(t *testing.T) {
	declared := aTopologyOfTwoZones(t)
	zones := declared.Zones()
	stopped := slices.Concat(nodeNamesOfZone(declared, zones[1]), nodeNamesOfZone(declared, zones[0]))

	responding, err := scenario.NewZonesRespondingAgain(declared, stopped)

	expect.Expect(t, err).To(expect.Equal[error](nil))
	expect.Expect(t, [][]string{responding.Names(), responding.NodeNames()}).
		To(expect.Equal([][]string{zones, theNodeNamesOf(declared)}))
}

func TestRespondingAgainWithNoNodeStoppedOrAStoppedNodeBesideOneStillResponding_ShouldBeRefusedNamingIt(t *testing.T) {
	declared := aTopologyOfTwoZones(t)
	ofTheFirstZone := nodeNamesOfZone(declared, declared.Zones()[0])
	stoppedAlone, stillResponding := ofTheFirstZone[0], ofTheFirstZone[1]
	refusals := map[string]struct {
		resolve func() error
		named   []string
	}{
		"the Nodes with no Node stopped": {
			resolve: func() error {
				_, err := scenario.NewNodesRespondingAgain(declared, nil)
				return err
			},
			named: []string{noNodeStopped},
		},
		"the Zones with no Node stopped": {
			resolve: func() error {
				_, err := scenario.NewZonesRespondingAgain(declared, nil)
				return err
			},
			named: []string{noNodeStopped},
		},
		"the Zones with a stopped Node sharing its Zone with one still responding": {
			resolve: func() error {
				_, err := scenario.NewZonesRespondingAgain(declared, []string{stoppedAlone})
				return err
			},
			named: []string{stoppedAlone, stillResponding},
		},
	}

	for refused, refusal := range refusals {
		t.Run(refused, func(t *testing.T) {
			err := refusal.resolve()

			expect.Expect(t, namedIn(err, refusal.named)).To(expect.Equal(true))
		})
	}
}

func namedIn(err error, named []string) bool {
	return err != nil && !slices.ContainsFunc(named, func(name string) bool { return !strings.Contains(err.Error(), name) })
}

const (
	aSecondPlacedWebPod = 2
	aCountBelowOne      = 0
)

// The Pods sit on the Nodes out of name order, so the pick follows the board's
// order of Zones, Nodes and Pods rather than the order the Pods were observed.
func aBoardOfThreePlacedWebPodsAndOneBatchPod(t *testing.T) scenario.PlacementBoard {
	t.Helper()

	declared := aTopologyOfTwoZones(t)
	nodes := theNodeNamesOf(declared)
	pods := []scenario.Pod{
		aPodOf(webDeployment, secondWebSuffix, nodes[1]),
		aPodOf(batchDeployment, firstBatchSuffix, nodes[0]),
		aPodOf(webDeployment, firstWebSuffix, nodes[1]),
		aPodOf(webDeployment, replacementSuffix, nodes[0]),
		anUnplacedPodOf(webDeployment, pendingWebSuffix),
	}

	return scenario.NewPlacementBoard(declared, observedOn(declared, pods), scenario.PlacementBoard{})
}

func TestAFailureOfSomePodsOfADeployment_ShouldLoseItsFirstPlacedPodsInTheBoardsOrderAndNoOtherDeploymentsPod(t *testing.T) {
	board := aBoardOfThreePlacedWebPodsAndOneBatchPod(t)

	failure, err := scenario.NewPodsFailure(board, aDeployment(t), aSecondPlacedWebPod)

	expect.Expect(t, err).To(expect.Equal[error](nil))
	expect.Expect(t, failure.BoardNames()).To(expect.Equal([]string{
		webDeployment + "-" + replacementSuffix, webDeployment + "-" + firstWebSuffix,
	}))
}

func TestAFailureOfPodsOfADeploymentDeclaringAFailureToleranceOrOfACountBelowOneOrAbovePlaced_ShouldBeRefusedNamingIt(t *testing.T) {
	declaring, err := aDeployment(t).WithFailureTolerance(
		scenario.FailureTolerance{Lost: scenario.NodesLost, Count: oneLost}, aTopologyOfTwoZones(t))
	if err != nil {
		t.Fatalf("declare a Failure tolerance: %v", err)
	}
	board := aBoardOfThreePlacedWebPodsAndOneBatchPod(t)
	refusals := map[string]struct {
		deployment scenario.Deployment
		count      int
		named      string
	}{
		"a Failure tolerance declared": {deployment: declaring, count: oneLost, named: "Failure tolerance"},
		"a count below one":            {deployment: aDeployment(t), count: aCountBelowOne, named: strconv.Itoa(aCountBelowOne)},
		"a count above those placed":   {deployment: aDeployment(t), count: everyNode, named: strconv.Itoa(everyNode)},
	}

	for refused, one := range refusals {
		t.Run(refused, func(t *testing.T) {
			_, err := scenario.NewPodsFailure(board, one.deployment, one.count)

			expect.Expect(t, err != nil && strings.Contains(err.Error(), webDeployment) && strings.Contains(err.Error(), one.named)).
				To(expect.Equal(true))
		})
	}
}
