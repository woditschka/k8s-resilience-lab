package scenario_test

import (
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// No test here turns on these numbers; the Deployment tests are where a capacity
// matters.
const (
	somePodsPerNode   = 10
	someCPUPerNode    = scenario.CPU(4000)
	someMemoryPerNode = scenario.Memory(16384)
)

func aCapacity(t *testing.T) scenario.NodeCapacity {
	t.Helper()

	capacity, err := scenario.NewNodeCapacity(somePodsPerNode, someCPUPerNode, someMemoryPerNode)
	if err != nil {
		t.Fatalf("build a Node capacity: %v", err)
	}

	return capacity
}

const theGeneralNodePool = "general"

func topologyOfTheGeneralNodePool(zones, nodesPerZone int, capacity scenario.NodeCapacity) (scenario.Topology, error) {
	pool, err := scenario.NewNodePool(theGeneralNodePool, nodesPerZone, capacity)
	if err != nil {
		return scenario.Topology{}, err
	}

	return scenario.NewTopology(zones, pool)
}

func TestATopology_ShouldExpandToNodesNamedAfterTheirZoneAndTheirIndexInIt(t *testing.T) {
	const twoZones, twoNodesPerZone = 2, 2
	capacity := aCapacity(t)

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

	topology, err := topologyOfTheGeneralNodePool(twoZones, twoNodesPerZone, aCapacity(t))
	if err != nil {
		t.Fatalf("build a Topology of %d Zones of %d Nodes: %v", twoZones, twoNodesPerZone, err)
	}

	expect.Expect(t, topology.Zones()).To(expect.Equal([]string{"zone-1", "zone-2"}))
}

func TestTwoRunsOfTheSameTopology_ShouldReportTheSameZoneAndNodeNames(t *testing.T) {
	const threeZones, threeNodesPerZone = 3, 3
	capacity := aCapacity(t)

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

// A Node always has room for a Pod that asks for something.
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

func aNodePool(t *testing.T, name string, nodesPerZone int, capacity scenario.NodeCapacity) scenario.NodePool {
	t.Helper()

	pool, err := scenario.NewNodePool(name, nodesPerZone, capacity)
	if err != nil {
		t.Fatalf("build a NodePool %q of %d Nodes per Zone: %v", name, nodesPerZone, err)
	}

	return pool
}

// A Node's name stays positional, so only its NodePool tells a Node of one kind
// from another.
func TestTheNodesOfAZone_ShouldBeNumberedThroughItsNodePoolsInDeclarationOrder(t *testing.T) {
	const twoZones, twoNodesPerZone, oneNodePerZone, gpu = 2, 2, 1, "gpu"
	const aLargerNodeHoldsTwiceAsMuch = 2
	generalCapacity := aCapacity(t)
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

// The names the rule admits and refuses are pinned to the client library's own
// in internal/cluster.
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
			_, err := scenario.NewNodePool(taken.name, taken.nodesPerZone, aCapacity(t))

			if err == nil {
				t.Fatalf("a NodePool of %s is refused, but it was admitted", pool)
			}
		})
	}
}

func TestANamedNodeCapacityOfNoNameOrNoCapacity_ShouldBeRefusedWhereItIsDefined(t *testing.T) {
	const someName, noName = "Large", ""
	var noNodeCapacity scenario.NodeCapacity

	defined := map[string]struct {
		name     string
		capacity scenario.NodeCapacity
	}{
		"no name":     {name: noName, capacity: aCapacity(t)},
		"no capacity": {name: someName, capacity: noNodeCapacity},
	}

	for named, taken := range defined {
		t.Run(named, func(t *testing.T) {
			_, err := scenario.NewNamedNodeCapacity(taken.name, taken.capacity)

			if err == nil {
				t.Fatalf("a Named Node capacity of %s is refused, but it was admitted", named)
			}
		})
	}
}

func TestANamedNodeCapacity_ShouldStandForTheNameAndCapacityItIsDefinedWith(t *testing.T) {
	const aNameOfItsOwn = "Large"
	capacity := aCapacity(t)

	named, err := scenario.NewNamedNodeCapacity(aNameOfItsOwn, capacity)
	if err != nil {
		t.Fatalf("define a Named Node capacity %q: %v", aNameOfItsOwn, err)
	}

	expect.Expect(t, named.Name()).To(expect.Equal(aNameOfItsOwn))
	expect.Expect(t, named.Capacity()).To(expect.Equal(capacity))
}

// On the Step surface a NodePool omitting its capacity does not compile, so
// this zero value is the one omission left to refuse at run time.
func TestANodePoolStatingNoCompleteNodeCapacity_ShouldBeRefusedWhereItIsStated(t *testing.T) {
	const someName, someNodesPerZone = "general", 1
	var noNodeCapacity scenario.NodeCapacity

	_, err := scenario.NewNodePool(someName, someNodesPerZone, noNodeCapacity)

	if err == nil {
		t.Fatal("a NodePool stating no Node capacity is refused, but it was admitted")
	}
}

func TestATopologyOfNoNodePoolOrOfTwoNodePoolsUnderOneName_ShouldBeRefused(t *testing.T) {
	const someZones, someNodesPerZone, someName = 2, 1, "gpu"

	stated := map[string][]scenario.NodePool{
		"no NodePool": nil,
		"two NodePools under one name": {
			aNodePool(t, someName, someNodesPerZone, aCapacity(t)),
			aNodePool(t, someName, someNodesPerZone, aCapacity(t)),
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
