package scenarios

import (
	"github.com/woditschka/k8s-resilience-lab/internal/lab"
	"github.com/woditschka/k8s-resilience-lab/internal/lab/node"
)

// Capacity comes back after a whole Zone stops responding, on the Zones still
// responding.
func AZoneThatStopsResponding_ShouldRestoreTheNinePodsOnTheZonesStillResponding(s *lab.Scenario) {
	s.Given().Topology(lab.Zones(3), node.Pool("general", node.PerZone(3), StandardNodes()))
	s.WaitFor().AllNodesReady()
	s.Given().Workload(Checkout, lab.Pods(9), ResilientPlacement()).RequiredCapacity(lab.Pods(6))
	s.WaitFor().AllPodsReady()

	s.Then().PodsPerZone(3)

	s.When().ZonesFail(lab.Zones(1))
	s.WaitFor().AllPodsReadyWithinSeconds(30)

	s.Then().NoPodsInTheFailedZones()
	s.Then().RequiredCapacityHeld()
}
