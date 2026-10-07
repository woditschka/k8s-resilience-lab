package scenarios

import "github.com/woditschka/k8s-resilience-lab/internal/lab"

// Capacity comes back after one Node stops responding, on the Nodes still
// responding.
func ANodeThatStopsResponding_ShouldLeaveThePodsOnTheNodesStillResponding(s *lab.Scenario) {
	s.Given().Topology(lab.Zones(3), lab.NodesPerZone(3))
	s.WaitFor().AllNodesReady()
	s.Given().Workload(Checkout, lab.Pods(9), lab.ResilientPlacement()).RequiredCapacity(lab.Pods(6))
	s.WaitFor().AllPodsReady()

	s.Then().PodsPerZone(3)

	s.When().NodesFail(lab.Nodes(1))
	s.WaitFor().AllPodsReadyWithinSeconds(30)

	s.Then().PodsPerZone(3)
	s.Then().NoPodsOnTheFailedNodes()
	s.Then().RequiredCapacityHeld()
}
