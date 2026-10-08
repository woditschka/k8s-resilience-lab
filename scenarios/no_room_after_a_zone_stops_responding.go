package scenarios

import "github.com/woditschka/k8s-resilience-lab/internal/lab"

// Capacity only partly comes back after a whole Zone stops responding, where each
// Node still responding has room for one Pod of the Workload: the Pods that no
// longer fit anywhere stay Pending, and the capacity the Workload declared is
// still met.
func NoRoomAfterAZoneStopsResponding_ShouldSettleAtSixPodsRunningAndThreePending(s *lab.Scenario) {
	s.Given().Topology(lab.Zones(3), lab.NodesPerZone(3)).NodeCapacity(lab.CPU(4), lab.MemoryGi(16))
	s.WaitFor().AllNodesReady()
	s.Given().Workload(Checkout, lab.Pods(9),
		lab.ResilientPlacement().Customize(lab.Request(lab.CPU(3), lab.MemoryGi(12)))).
		RequiredCapacity(lab.Pods(6))
	s.WaitFor().AllPodsReady()

	s.Then().PodsPerNode(1)

	s.When().ZonesFail(lab.Zones(1))
	s.WaitFor().PlacementSettledWithinSeconds(30)

	s.Then().NoPodsInTheFailedZones()
	s.Then().PodsPending(3)
	s.Then().RequiredCapacityHeld()
}
