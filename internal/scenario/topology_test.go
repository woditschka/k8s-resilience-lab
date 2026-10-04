package scenario_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// somePodsPerNode is the Node capacity every Topology in this suite is given. No
// test here turns on the number; the Workload tests are where capacity matters.
const somePodsPerNode = 10

// oneZonePerLetter is the rule the Zone-count limit follows: a Zone is lettered
// from zone-a, so the alphabet is what bounds how many Zones a Topology can name.
// Derived from the rule rather than copied from the constant that carries it.
const oneZonePerLetter = 'z' - 'a' + 1

// aCapacity is the named default for a Node capacity no test cares about.
func aCapacity(t *testing.T) scenario.NodeCapacity {
	t.Helper()

	capacity, err := scenario.NewNodeCapacity(somePodsPerNode)
	if err != nil {
		t.Fatalf("build a Node capacity of %d Pods: %v", somePodsPerNode, err)
	}

	return capacity
}

func TestATopology_ShouldExpandToNodesNamedAfterTheirZoneAndTheirIndexInIt(t *testing.T) {
	const twoZones, twoNodesPerZone = 2, 2
	capacity := aCapacity(t)

	topology, err := scenario.NewTopology(twoZones, twoNodesPerZone, capacity)
	if err != nil {
		t.Fatalf("build a Topology of %d Zones of %d Nodes: %v", twoZones, twoNodesPerZone, err)
	}

	expect.Expect(t, topology.Nodes()).To(expect.Equal([]scenario.Node{
		{Name: "zone-a-node-1", Zone: "zone-a", Capacity: capacity},
		{Name: "zone-a-node-2", Zone: "zone-a", Capacity: capacity},
		{Name: "zone-b-node-1", Zone: "zone-b", Capacity: capacity},
		{Name: "zone-b-node-2", Zone: "zone-b", Capacity: capacity},
	}))
}

func TestTwoRunsOfTheSameTopology_ShouldReportTheSameZoneAndNodeNames(t *testing.T) {
	const threeZones, threeNodesPerZone = 3, 3
	capacity := aCapacity(t)

	firstRun, err := scenario.NewTopology(threeZones, threeNodesPerZone, capacity)
	if err != nil {
		t.Fatalf("build the first run's Topology: %v", err)
	}
	secondRun, err := scenario.NewTopology(threeZones, threeNodesPerZone, capacity)
	if err != nil {
		t.Fatalf("build the second run's Topology: %v", err)
	}

	expect.Expect(t, secondRun.Nodes()).To(expect.Equal(firstRun.Nodes()))
}

func TestATopologyOfOneZonePerLetter_ShouldBeAdmitted(t *testing.T) {
	const oneNodePerZone = 1
	capacity := aCapacity(t)

	topology, err := scenario.NewTopology(oneZonePerLetter, oneNodePerZone, capacity)
	if err != nil {
		t.Fatalf("build a Topology of one Zone per letter: %v", err)
	}

	expect.Expect(t, topology.Nodes()).To(expect.HaveLen[scenario.Node](oneZonePerLetter))
}

func TestATopology_ShouldRefuseACountItCannotProvision(t *testing.T) {
	const someNodesPerZone, someZones = 3, 3

	refusals := map[string]struct {
		zones        int
		nodesPerZone int
		named        string
	}{
		"no Zone at all":             {zones: 0, nodesPerZone: someNodesPerZone, named: "Zone"},
		"a Zone holding no Node":     {zones: someZones, nodesPerZone: 0, named: "Node"},
		"one Zone more than letters": {zones: oneZonePerLetter + 1, nodesPerZone: someNodesPerZone, named: strconv.Itoa(oneZonePerLetter)},
		"a negative Node count":      {zones: someZones, nodesPerZone: -1, named: "Node"},
	}

	for refused, refusal := range refusals {
		t.Run(refused, func(t *testing.T) {
			_, err := scenario.NewTopology(refusal.zones, refusal.nodesPerZone, aCapacity(t))

			if err == nil {
				t.Fatalf("a Topology of %d Zones of %d Nodes is refused, but it was admitted", refusal.zones, refusal.nodesPerZone)
			}
			if !strings.Contains(err.Error(), refusal.named) {
				t.Errorf("the refusal names %q, but reads: %v", refusal.named, err)
			}
		})
	}
}

func TestANodeCapacity_ShouldRefuseAPodCountNoNodeCouldHold(t *testing.T) {
	const noPodAtAll = 0

	_, err := scenario.NewNodeCapacity(noPodAtAll)

	if err == nil {
		t.Fatal("a capacity of no Pod at all is refused, but it was admitted")
	}
}
