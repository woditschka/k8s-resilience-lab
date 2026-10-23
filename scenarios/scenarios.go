// The lab's Scenarios, in the order All names them. Adding a Scenario is a
// function in this file and a line in All.

package scenarios

import (
	"github.com/woditschka/k8s-resilience-lab/internal/lab"
	"github.com/woditschka/k8s-resilience-lab/internal/lab/node"
	"github.com/woditschka/k8s-resilience-lab/internal/lab/pod"
)

// All is a function rather than a variable, because no package of the lab holds
// mutable process-wide state.
func All() []func(*lab.Scenario) {
	return []func(*lab.Scenario){
		AWorkloadPlacedForResilience_ShouldPlaceThreePodsPerZoneAndOnePerNode,
		ANodeThatStopsResponding_ShouldLeaveThePodsOnTheNodesStillResponding,
		AZoneThatStopsResponding_ShouldRestoreTheNinePodsOnTheZonesStillResponding,
		NoRoomAfterAZoneStopsResponding_ShouldSettleAtSixPodsRunningAndThreePending,
		AWorkloadSpreadOverOnDemandAndSpot_ShouldPlaceTwoPodsPerZoneAndThreePerCapacityType,
		SpotNodesThatStopResponding_ShouldHoldTheRequiredCapacityAndRestoreNinePodsOnTheOnDemandNodes,
		GeneralPodsBesideTaintedGPUNodes_ShouldStayOffThemWhileTheTolerantPodsReachThemByTheirRequest,
	}
}

// An even Placement over Zones and Nodes with room for more than one Pod is the
// scheduler honouring Resilient placement.
func AWorkloadPlacedForResilience_ShouldPlaceThreePodsPerZoneAndOnePerNode(s *lab.Scenario) {
	s.Given().Topology(lab.Zones(3), node.Pool("general", node.PerZone(3), StandardNode()))
	s.WaitFor().AllNodesReady()
	s.Given().Workload(Checkout, lab.Pods(9), StandardPod(), ResilientPlacement())
	s.WaitFor().AllPodsReady()

	s.Then().PodsPerZone(3)
	s.Then().PodsPerNode(1)
}

// Capacity comes back after one Node stops responding, on the Nodes still
// responding.
func ANodeThatStopsResponding_ShouldLeaveThePodsOnTheNodesStillResponding(s *lab.Scenario) {
	s.Given().Topology(lab.Zones(3), node.Pool("general", node.PerZone(3), StandardNode()))
	s.WaitFor().AllNodesReady()
	s.Given().Workload(Checkout, lab.Pods(9), StandardPod(), ResilientPlacement()).RequiredCapacity(lab.Pods(6))
	s.WaitFor().AllPodsReady()

	s.Then().PodsPerZone(3)

	s.When().NodesFail(lab.Nodes(1))
	s.WaitFor().AllPodsReadyWithinSeconds(30)

	s.Then().PodsPerZone(3)
	s.Then().NoPodsOnTheFailedNodes()
	s.Then().RequiredCapacityHeld()
}

// Capacity comes back after a whole Zone stops responding, on the Zones still
// responding.
func AZoneThatStopsResponding_ShouldRestoreTheNinePodsOnTheZonesStillResponding(s *lab.Scenario) {
	s.Given().Topology(lab.Zones(3), node.Pool("general", node.PerZone(3), StandardNode()))
	s.WaitFor().AllNodesReady()
	s.Given().Workload(Checkout, lab.Pods(9), StandardPod(), ResilientPlacement()).RequiredCapacity(lab.Pods(6))
	s.WaitFor().AllPodsReady()

	s.Then().PodsPerZone(3)

	s.When().ZonesFail(lab.Zones(1))
	s.WaitFor().AllPodsReadyWithinSeconds(30)

	s.Then().NoPodsInTheFailedZones()
	s.Then().RequiredCapacityHeld()
}

// Capacity only partly comes back after a whole Zone stops responding, where each
// Node still responding has room for one Pod of the Workload: the Pods that no
// longer fit anywhere stay Pending, and the capacity the Workload declared is
// still met.
func NoRoomAfterAZoneStopsResponding_ShouldSettleAtSixPodsRunningAndThreePending(s *lab.Scenario) {
	s.Given().Topology(lab.Zones(3), node.Pool("general", node.PerZone(3),
		node.Capacity{CPU: 4, Memory: 16 * node.Gi, Pods: 10}))
	s.WaitFor().AllNodesReady()
	s.Given().Workload(Checkout, lab.Pods(9), pod.Request{CPU: 3, Memory: 12 * pod.Gi}, ResilientPlacement()).
		RequiredCapacity(lab.Pods(6))
	s.WaitFor().AllPodsReady()

	s.Then().PodsPerNode(1)

	s.When().ZonesFail(lab.Zones(1))
	s.WaitFor().PlacementSettledWithinSeconds(30)

	s.Then().NoPodsInTheFailedZones()
	s.Then().PodsPending(3)
	s.Then().RequiredCapacityHeld()
}

// Every Zone holds an on-demand and a spot Node with room for more than one Pod,
// so an even Placement over Zones and over capacity types is the scheduler
// honouring both spreads at once.
func AWorkloadSpreadOverOnDemandAndSpot_ShouldPlaceTwoPodsPerZoneAndThreePerCapacityType(s *lab.Scenario) {
	s.Given().Topology(lab.Zones(3),
		node.Pool("on-demand", node.PerZone(1), StandardNode()).Labels(node.Labels{CapacityType: "on-demand"}),
		node.Pool("spot", node.PerZone(1), StandardNode()).Labels(node.Labels{CapacityType: "spot"}),
	)
	s.WaitFor().AllNodesReady()
	s.Given().Workload(Checkout, lab.Pods(6), StandardPod(), SpreadOverOnDemandAndSpot())
	s.WaitFor().AllPodsReady()

	s.Then().AllPodsRunning()
	s.Then().PodsPerZone(2)
	s.Then().PodsPerValueOf(CapacityType, 3)
}

// The capacity bought as spot disappears together, and the Pods it held are
// restored on the on-demand Nodes in every Zone.
func SpotNodesThatStopResponding_ShouldHoldTheRequiredCapacityAndRestoreNinePodsOnTheOnDemandNodes(s *lab.Scenario) {
	s.Given().Topology(lab.Zones(3),
		node.Pool("on-demand", node.PerZone(2), StandardNode()).Labels(node.Labels{CapacityType: "on-demand"}),
		node.Pool("spot", node.PerZone(1), StandardNode()).Labels(node.Labels{CapacityType: "spot"}),
	)
	s.WaitFor().AllNodesReady()
	s.Given().Workload(Checkout, lab.Pods(9), StandardPod(), ResilientPlacement()).RequiredCapacity(lab.Pods(6))
	s.WaitFor().AllPodsReady()

	s.Then().PodsPerNode(1)

	s.When().NodesLabelledFail(node.Labels{CapacityType: "spot"})
	s.WaitFor().AllPodsReadyWithinSeconds(30)

	s.Then().PodsPerZone(3)
	s.Then().NoPodsOnTheFailedNodes()
	s.Then().RequiredCapacityHeld()
}

// Every Zone holds two general Nodes and one larger gpu Node tainted NoSchedule.
// The general Pods stay off the gpu Nodes, and the Pods tolerating the taint
// reach them because only a gpu Node has room for what each asks for.
func GeneralPodsBesideTaintedGPUNodes_ShouldStayOffThemWhileTheTolerantPodsReachThemByTheirRequest(s *lab.Scenario) {
	s.Given().Topology(lab.Zones(3),
		node.Pool("general", node.PerZone(2), StandardNode()),
		node.Pool("gpu", node.PerZone(1), GPUNode()).Taints(node.Taint{Key: GPU, Value: GPUPresent, Effect: node.NoSchedule}),
	)
	s.WaitFor().AllNodesReady()
	s.Given().Workload(Checkout, lab.Pods(6), StandardPod(), ResilientPlacement())
	s.WaitFor().AllPodsReady()

	s.Then().AllPodsRunning()
	s.Then().PodsOnNodePool("general", 6)
	s.Then().PodsOnNodePool("gpu", 0)

	s.Given().Workload(Inference, lab.Pods(3), GPUPod(), ToleratingGPUNodes())
	s.WaitFor().AllPodsReady()

	s.Then().AllPodsRunning()
	s.Then().PodsOnNodePool("gpu", 3)
	s.Then().PodsOnNodePool("general", 0)
}
