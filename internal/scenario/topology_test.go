package scenario_test

import (
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// The dimensions a Node capacity is stated in, where a test here states one at
// all: no test turns on the numbers, the Workload tests being where a capacity
// matters. The cpu is the millicores and the memory the mebibytes the two
// quantity types hold.
const (
	somePodsPerNode   = 10
	someCPUPerNode    = scenario.CPU(4000)
	someMemoryPerNode = scenario.Memory(16384)
)

// aCapacity is the named default for a Node capacity no test cares about.
func aCapacity() scenario.NodeCapacity {
	return scenario.DefaultNodeCapacity()
}

func TestATopology_ShouldExpandToNodesNamedAfterTheirZoneAndTheirIndexInIt(t *testing.T) {
	const twoZones, twoNodesPerZone = 2, 2
	capacity := aCapacity()

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

func TestATopology_ShouldNameEachOfItsZonesOnceInZoneOrder(t *testing.T) {
	const twoZones, twoNodesPerZone = 2, 2

	topology, err := scenario.NewTopology(twoZones, twoNodesPerZone, aCapacity())
	if err != nil {
		t.Fatalf("build a Topology of %d Zones of %d Nodes: %v", twoZones, twoNodesPerZone, err)
	}

	expect.Expect(t, topology.Zones()).To(expect.Equal([]string{"zone-1", "zone-2"}))
}

func TestTwoRunsOfTheSameTopology_ShouldReportTheSameZoneAndNodeNames(t *testing.T) {
	const threeZones, threeNodesPerZone = 3, 3
	capacity := aCapacity()

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

// A Node always has room for a Pod that asks for something, so each of the three
// dimensions is refused at none where it is stated.
func TestANodeCapacityOfNoPodNoCpuOrNoMemory_ShouldBeRefusedWhereItIsStated(t *testing.T) {
	const noPodAtAll, noCPUAtAll, noMemoryAtAll = 0, scenario.CPU(0), scenario.Memory(0)

	stated := map[string]struct {
		pods   int
		cpu    scenario.CPU
		memory scenario.Memory
	}{
		"no Pod at all":    {pods: noPodAtAll, cpu: someCPUPerNode, memory: someMemoryPerNode},
		"no cpu at all":    {pods: somePodsPerNode, cpu: noCPUAtAll, memory: someMemoryPerNode},
		"no memory at all": {pods: somePodsPerNode, cpu: someCPUPerNode, memory: noMemoryAtAll},
	}

	for capacity, taken := range stated {
		t.Run(capacity, func(t *testing.T) {
			_, err := scenario.NewNodeCapacity(taken.pods, taken.cpu, taken.memory)

			if err == nil {
				t.Fatalf("a capacity of %s is refused, but it was admitted", capacity)
			}
		})
	}
}
