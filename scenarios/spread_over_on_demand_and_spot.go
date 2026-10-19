package scenarios

import (
	"github.com/woditschka/k8s-resilience-lab/internal/lab"
	"github.com/woditschka/k8s-resilience-lab/internal/lab/node"
)

// Every Zone holds an on-demand and a spot Node with room for more than one Pod,
// so an even Placement over Zones and over capacity types is the scheduler
// honouring both spreads at once.
func AWorkloadSpreadOverOnDemandAndSpot_ShouldPlaceTwoPodsPerZoneAndThreePerCapacityType(s *lab.Scenario) {
	s.Given().Topology(lab.Zones(3),
		node.Pool("on-demand", node.PerZone(1), StandardNodes()).Labels(node.Labels{CapacityType: "on-demand"}),
		node.Pool("spot", node.PerZone(1), StandardNodes()).Labels(node.Labels{CapacityType: "spot"}),
	)
	s.WaitFor().AllNodesReady()
	s.Given().Workload(Checkout, lab.Pods(6), SpreadOverOnDemandAndSpot())
	s.WaitFor().AllPodsReady()

	s.Then().AllPodsRunning()
	s.Then().PodsPerZone(2)
	s.Then().PodsPerValueOf(CapacityType, 3)
}
