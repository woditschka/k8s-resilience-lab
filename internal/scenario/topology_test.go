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

// theGeneralNodePool is the name of the one NodePool a test turns on nothing but
// the Zones and Nodes of.
const theGeneralNodePool = "general"

// topologyOfTheGeneralNodePool is a Topology of zones Zones each holding the one
// NodePool general of nodesPerZone Nodes at capacity.
func topologyOfTheGeneralNodePool(zones, nodesPerZone int, capacity scenario.NodeCapacity) (scenario.Topology, error) {
	pool, err := scenario.NewNodePool(theGeneralNodePool, nodesPerZone, capacity)
	if err != nil {
		return scenario.Topology{}, err
	}

	return scenario.NewTopology(zones, pool)
}

func TestATopology_ShouldExpandToNodesNamedAfterTheirZoneAndTheirIndexInIt(t *testing.T) {
	const twoZones, twoNodesPerZone = 2, 2
	capacity := aCapacity()

	topology, err := topologyOfTheGeneralNodePool(twoZones, twoNodesPerZone, capacity)
	if err != nil {
		t.Fatalf("build a Topology of %d Zones of %d Nodes: %v", twoZones, twoNodesPerZone, err)
	}

	expect.Expect(t, topology.Nodes()).To(expect.Equal([]scenario.Node{
		{Name: "zone-1-node-1", Zone: "zone-1", NodePool: theGeneralNodePool, Capacity: capacity},
		{Name: "zone-1-node-2", Zone: "zone-1", NodePool: theGeneralNodePool, Capacity: capacity},
		{Name: "zone-2-node-1", Zone: "zone-2", NodePool: theGeneralNodePool, Capacity: capacity},
		{Name: "zone-2-node-2", Zone: "zone-2", NodePool: theGeneralNodePool, Capacity: capacity},
	}))
}

func TestATopology_ShouldNameEachOfItsZonesOnceInZoneOrder(t *testing.T) {
	const twoZones, twoNodesPerZone = 2, 2

	topology, err := topologyOfTheGeneralNodePool(twoZones, twoNodesPerZone, aCapacity())
	if err != nil {
		t.Fatalf("build a Topology of %d Zones of %d Nodes: %v", twoZones, twoNodesPerZone, err)
	}

	expect.Expect(t, topology.Zones()).To(expect.Equal([]string{"zone-1", "zone-2"}))
}

func TestTwoRunsOfTheSameTopology_ShouldReportTheSameZoneAndNodeNames(t *testing.T) {
	const threeZones, threeNodesPerZone = 3, 3
	capacity := aCapacity()

	firstRun, err := topologyOfTheGeneralNodePool(threeZones, threeNodesPerZone, capacity)
	if err != nil {
		t.Fatalf("build the first run's Topology: %v", err)
	}
	secondRun, err := topologyOfTheGeneralNodePool(threeZones, threeNodesPerZone, capacity)
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

// aNodePool is a NodePool of the name, Node count and capacity stated.
func aNodePool(t *testing.T, name string, nodesPerZone int, capacity scenario.NodeCapacity) scenario.NodePool {
	t.Helper()

	pool, err := scenario.NewNodePool(name, nodesPerZone, capacity)
	if err != nil {
		t.Fatalf("build a NodePool %q of %d Nodes per Zone: %v", name, nodesPerZone, err)
	}

	return pool
}

// A Zone's Nodes are counted from one through its NodePools in the order the
// Topology declares them, so a Node's name stays positional and only its NodePool
// tells a Node of one kind from another (docs/prd.md REQ-SIM-013).
func TestTheNodesOfAZone_ShouldBeNumberedThroughItsNodePoolsInDeclarationOrder(t *testing.T) {
	const twoZones, twoNodesPerZone, oneNodePerZone, gpu = 2, 2, 1, "gpu"
	const aLargerNodeHoldsTwiceAsMuch = 2
	generalCapacity := aCapacity()
	gpuCapacity, err := scenario.NewNodeCapacity(somePodsPerNode,
		aLargerNodeHoldsTwiceAsMuch*someCPUPerNode, aLargerNodeHoldsTwiceAsMuch*someMemoryPerNode)
	if err != nil {
		t.Fatalf("build the gpu NodePool's capacity: %v", err)
	}

	topology, err := scenario.NewTopology(twoZones,
		aNodePool(t, theGeneralNodePool, twoNodesPerZone, generalCapacity),
		aNodePool(t, gpu, oneNodePerZone, gpuCapacity))
	if err != nil {
		t.Fatalf("build a Topology of two NodePools: %v", err)
	}

	expect.Expect(t, topology.Nodes()).To(expect.Equal([]scenario.Node{
		{Name: "zone-1-node-1", Zone: "zone-1", NodePool: theGeneralNodePool, Capacity: generalCapacity},
		{Name: "zone-1-node-2", Zone: "zone-1", NodePool: theGeneralNodePool, Capacity: generalCapacity},
		{Name: "zone-1-node-3", Zone: "zone-1", NodePool: gpu, Capacity: gpuCapacity},
		{Name: "zone-2-node-1", Zone: "zone-2", NodePool: theGeneralNodePool, Capacity: generalCapacity},
		{Name: "zone-2-node-2", Zone: "zone-2", NodePool: theGeneralNodePool, Capacity: generalCapacity},
		{Name: "zone-2-node-3", Zone: "zone-2", NodePool: gpu, Capacity: gpuCapacity},
	}))
}

// A NodePool of no Node, one stating no name, and one whose name no Kubernetes
// label value can hold, are refused where they are stated (docs/prd.md
// REQ-SIM-013 edge cases 1, 3 and 5). The names the rule admits and refuses are
// pinned to the client library's own in internal/cluster.
func TestANodePoolOfNoNodeNoNameOrANameNoLabelCanHold_ShouldBeRefused(t *testing.T) {
	const noNode, someNodesPerZone, someName, noName, aNameNoLabelCanHold = 0, 1, "gpu", "", "gpu nodes"

	stated := map[string]struct {
		name         string
		nodesPerZone int
	}{
		"no Node":                  {name: someName, nodesPerZone: noNode},
		"no name":                  {name: noName, nodesPerZone: someNodesPerZone},
		"a name no label can hold": {name: aNameNoLabelCanHold, nodesPerZone: someNodesPerZone},
	}

	for pool, taken := range stated {
		t.Run(pool, func(t *testing.T) {
			_, err := scenario.NewNodePool(taken.name, taken.nodesPerZone, aCapacity())

			if err == nil {
				t.Fatalf("a NodePool of %s is refused, but it was admitted", pool)
			}
		})
	}
}

// A Topology holds at least one NodePool, and each of its NodePools can be told
// from every other, so no two share a name (docs/prd.md REQ-SIM-013 edge cases 2
// and 5).
func TestATopologyOfNoNodePoolOrOfTwoNodePoolsUnderOneName_ShouldBeRefused(t *testing.T) {
	const someZones, someNodesPerZone, someName = 2, 1, "gpu"

	stated := map[string][]scenario.NodePool{
		"no NodePool": nil,
		"two NodePools under one name": {
			aNodePool(t, someName, someNodesPerZone, aCapacity()),
			aNodePool(t, someName, someNodesPerZone, aCapacity()),
		},
	}

	for topology, pools := range stated {
		t.Run(topology, func(t *testing.T) {
			_, err := scenario.NewTopology(someZones, pools...)

			if err == nil {
				t.Fatalf("a Topology of %s is refused, but it was admitted", topology)
			}
		})
	}
}
