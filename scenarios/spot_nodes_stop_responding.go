package scenarios

import (
	"github.com/woditschka/k8s-resilience-lab/internal/lab"
	"github.com/woditschka/k8s-resilience-lab/internal/lab/node"
)

// The capacity bought as spot disappears together, and the Pods it held are
// restored on the on-demand Nodes in every Zone.
func SpotNodesThatStopResponding_ShouldHoldTheRequiredCapacityAndRestoreNinePodsOnTheOnDemandNodes(s *lab.Scenario) {
	s.Given().Topology(lab.Zones(3),
		node.Pool("on-demand", node.PerZone(2), StandardNodes()).Labels(node.Labels{CapacityType: "on-demand"}),
		node.Pool("spot", node.PerZone(1), StandardNodes()).Labels(node.Labels{CapacityType: "spot"}),
	)
	s.WaitFor().AllNodesReady()
	s.Given().Workload(Checkout, lab.Pods(9), ResilientPlacement()).RequiredCapacity(lab.Pods(6))
	s.WaitFor().AllPodsReady()

	s.Then().PodsPerNode(1)

	s.When().NodesLabelledFail(node.Labels{CapacityType: "spot"})
	s.WaitFor().AllPodsReadyWithinSeconds(30)

	s.Then().PodsPerZone(3)
	s.Then().NoPodsOnTheFailedNodes()
	s.Then().RequiredCapacityHeld()
}
