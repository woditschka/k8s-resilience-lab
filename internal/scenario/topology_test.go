package scenario_test

import (
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// somePodsPerNode is the Node capacity every Topology in this suite is given. No
// test here turns on the number; the Workload tests are where capacity matters.
const somePodsPerNode = 10

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
		{Name: "zone-1-node-1", Zone: "zone-1", Capacity: capacity},
		{Name: "zone-1-node-2", Zone: "zone-1", Capacity: capacity},
		{Name: "zone-2-node-1", Zone: "zone-2", Capacity: capacity},
		{Name: "zone-2-node-2", Zone: "zone-2", Capacity: capacity},
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

func TestANodeCapacity_ShouldRefuseAPodCountNoNodeCouldHold(t *testing.T) {
	const noPodAtAll = 0

	_, err := scenario.NewNodeCapacity(noPodAtAll)

	if err == nil {
		t.Fatal("a capacity of no Pod at all is refused, but it was admitted")
	}
}
