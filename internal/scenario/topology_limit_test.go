package scenario

import (
	"strconv"
	"strings"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
)

// The Zone limit is a constant of the package's own, so the tests that hold a
// Topology to it live here, inside the package, and read maxZones rather than
// restating the count a second time.

// aCapacity is the named default for a Node capacity no test in this file cares
// about: what these tests turn on is the Zone count.
func aCapacity() NodeCapacity {
	return DefaultNodeCapacity()
}

// topologyOfOneNodePool is a Topology of zones Zones each holding one NodePool of
// nodesPerZone Nodes at capacity, or the refusal either constructor gave.
func topologyOfOneNodePool(zones, nodesPerZone int, capacity NodeCapacity) (Topology, error) {
	pool, err := NewNodePool("general", nodesPerZone, capacity)
	if err != nil {
		return Topology{}, err
	}

	return NewTopology(zones, pool)
}

func TestATopologyAtTheLabsZoneLimit_ShouldBeAdmitted(t *testing.T) {
	const oneNodePerZone = 1
	capacity := aCapacity()

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
			_, err := topologyOfOneNodePool(refusal.zones, refusal.nodesPerZone, aCapacity())

			if err == nil {
				t.Fatalf("a Topology of %d Zones of %d Nodes is refused, but it was admitted", refusal.zones, refusal.nodesPerZone)
			}
			if !strings.Contains(err.Error(), refusal.named) {
				t.Errorf("the refusal names %q, but reads: %v", refusal.named, err)
			}
		})
	}
}
