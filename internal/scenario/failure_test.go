package scenario_test

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// theTopologyAFailureIsResolvedAgainst is the shape of the Topology the Failures
// below are resolved against: small enough for a reader to hold every derived
// name at once, and holding more than one Node per Zone, so the first Nodes of
// the derivation and the first Nodes of a Zone are not the same set.
const (
	twoZones        = 2
	twoNodesPerZone = 2

	// everyNode is how many Nodes that Topology holds, which is the largest
	// Failure it admits.
	everyNode = twoZones * twoNodesPerZone

	// everyZone is how many Zones that Topology holds, which is the largest Zone
	// Failure it admits.
	everyZone = twoZones

	// oneNode and twoNodes are the counts a Failure is asked for below, and oneZone
	// the count a Zone Failure is asked for.
	oneNode  = 1
	twoNodes = 2
	oneZone  = 1
)

// aTopologyOfTwoZones is the Topology every Failure below is resolved against.
func aTopologyOfTwoZones(t *testing.T) scenario.Topology {
	t.Helper()

	topology, err := topologyOfTheGeneralNodePool(twoZones, twoNodesPerZone, aCapacity(t))
	if err != nil {
		t.Fatalf("build a Topology of %d Zones of %d Nodes: %v", twoZones, twoNodesPerZone, err)
	}

	return topology
}

// aFailureOf is the Failure stopping count Nodes of that Topology.
func aFailureOf(t *testing.T, topology scenario.Topology, count int) scenario.NodeFailure {
	t.Helper()

	failure, err := scenario.NewNodeFailure(topology, count)
	if err != nil {
		t.Fatalf("build a Failure stopping %d Nodes: %v", count, err)
	}

	return failure
}

// aZoneFailureOf is the Failure stopping count Zones of that Topology.
func aZoneFailureOf(t *testing.T, topology scenario.Topology, count int) scenario.ZoneFailure {
	t.Helper()

	failure, err := scenario.NewZoneFailure(topology, count)
	if err != nil {
		t.Fatalf("build a Failure stopping %d Zones: %v", count, err)
	}

	return failure
}

// theFirstZonesOf and theNodesOfTheFirstZonesOf are the Zones a Zone Failure of
// count stops and the Nodes it stops in them, derived here rather than written
// out, so the expectation follows the Topology that is asked.
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

// theFirstNamesOf are the names of the first count Nodes the Topology derives,
// derived here rather than written out, so the expectation follows the Topology
// that is asked rather than repeating its naming rule.
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

// A Zone Failure is the Node Failure over a whole Zone: it names the Zones it
// stops, and the Nodes it stops are every Node those Zones hold rather than the
// first the Topology derives, which is what makes a Zone the unit it fails.
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
