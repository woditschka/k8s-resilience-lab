package scenarios

import (
	"github.com/woditschka/k8s-resilience-lab/internal/lab"
	"github.com/woditschka/k8s-resilience-lab/internal/lab/node"
	"github.com/woditschka/k8s-resilience-lab/internal/lab/pod"
)

// Capacity only partly comes back after a whole Zone stops responding, where each
// Node still responding has room for one Pod of the Workload: the Pods that no
// longer fit anywhere stay Pending, and the capacity the Workload declared is
// still met.
func NoRoomAfterAZoneStopsResponding_ShouldSettleAtSixPodsRunningAndThreePending(s *lab.Scenario) {
	s.Given().Topology(lab.Zones(3), node.Pool("general", node.PerZone(3),
		node.Capacity{CPU: 4, Memory: 16 * node.Gi, Pods: 10}))
	s.WaitFor().AllNodesReady()
	s.Given().Workload(Checkout, lab.Pods(9),
		ResilientPlacement().Customize(pod.Request{CPU: 3, Memory: 12 * pod.Gi})).
		RequiredCapacity(lab.Pods(6))
	s.WaitFor().AllPodsReady()

	s.Then().PodsPerNode(1)

	s.When().ZonesFail(lab.Zones(1))
	s.WaitFor().PlacementSettledWithinSeconds(30)

	s.Then().NoPodsInTheFailedZones()
	s.Then().PodsPending(3)
	s.Then().RequiredCapacityHeld()
}
