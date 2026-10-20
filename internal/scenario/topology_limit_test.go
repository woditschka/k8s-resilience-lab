package scenario

import (
	"strconv"
	"strings"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
)

// maxZones is unexported, so the tests holding a Topology to it live inside the
// package rather than restating the count.

func aCapacity(t *testing.T) NodeCapacity {
	t.Helper()

	const somePodsPerNode, someCPUPerNode, someMemoryPerNode = 10, CPU(4000), Memory(16384)
	capacity, err := NewNodeCapacity(somePodsPerNode, someCPUPerNode, someMemoryPerNode)
	if err != nil {
		t.Fatalf("build a Node capacity: %v", err)
	}

	return capacity
}

func topologyOfOneNodePool(zones, nodesPerZone int, capacity NodeCapacity) (Topology, error) {
	pool, err := NewNodePool("general", nodesPerZone, capacity)
	if err != nil {
		return Topology{}, err
	}

	return NewTopology(zones, pool)
}

func TestATopologyAtTheLabsZoneLimit_ShouldBeAdmitted(t *testing.T) {
	const oneNodePerZone = 1
	capacity := aCapacity(t)

	topology, err := topologyOfOneNodePool(maxZones, oneNodePerZone, capacity)
	if err != nil {
		t.Fatalf("build a Topology of %d Zones: %v", maxZones, err)
	}

	expect.Expect(t, topology.Nodes()).To(expect.HaveLen[Node](maxZones))
}

func TestATopology_ShouldRefuseACountItCannotProvision(t *testing.T) {
	const someNodesPerZone, someZones = 3, 3

	refusals := map[string]struct {
		zones        int
		nodesPerZone int
		named        string
	}{
		"no Zone at all":          {zones: 0, nodesPerZone: someNodesPerZone, named: "Zone"},
		"a Zone holding no Node":  {zones: someZones, nodesPerZone: 0, named: "Node"},
		"one Zone past the limit": {zones: maxZones + 1, nodesPerZone: someNodesPerZone, named: strconv.Itoa(maxZones)},
		"a negative Node count":   {zones: someZones, nodesPerZone: -1, named: "Node"},
	}

	for refused, refusal := range refusals {
		t.Run(refused, func(t *testing.T) {
			_, err := topologyOfOneNodePool(refusal.zones, refusal.nodesPerZone, aCapacity(t))

			if err == nil {
				t.Fatalf("a Topology of %d Zones of %d Nodes is refused, but it was admitted", refusal.zones, refusal.nodesPerZone)
			}
			if !strings.Contains(err.Error(), refusal.named) {
				t.Errorf("the refusal names %q, but reads: %v", refusal.named, err)
			}
		})
	}
}
