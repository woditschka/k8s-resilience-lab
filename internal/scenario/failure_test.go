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
