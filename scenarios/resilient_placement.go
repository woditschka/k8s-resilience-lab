package scenarios

import "github.com/woditschka/k8s-resilience-lab/internal/lab"

// An even Placement over Zones and Nodes with room for more than one Pod is the
// scheduler honouring Resilient placement.
func AWorkloadPlacedForResilience_ShouldPlaceThreePodsPerZoneAndOnePerNode(s *lab.Scenario) {
	s.Given().Topology(lab.Zones(3), lab.NodePool("general", lab.NodesPerZone(3), StandardNodes()))
	s.WaitFor().AllNodesReady()
	s.Given().Workload(Checkout, lab.Pods(9), lab.ResilientPlacement())
	s.WaitFor().AllPodsReady()

	s.Then().PodsPerZone(3)
	s.Then().PodsPerNode(1)
}
