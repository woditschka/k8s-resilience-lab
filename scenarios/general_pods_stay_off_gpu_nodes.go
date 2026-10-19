package scenarios

import (
	"github.com/woditschka/k8s-resilience-lab/internal/lab"
	"github.com/woditschka/k8s-resilience-lab/internal/lab/node"
)

// Every Zone holds two general Nodes and one larger gpu Node tainted NoSchedule.
// The general Pods stay off the gpu Nodes, and the Pods tolerating the taint
// reach them because only a gpu Node has room for what each asks for.
func GeneralPodsBesideTaintedGPUNodes_ShouldStayOffThemWhileTheTolerantPodsReachThemByTheirRequest(s *lab.Scenario) {
	s.Given().Topology(lab.Zones(3),
		node.Pool("general", node.PerZone(2), StandardNodes()),
		node.Pool("gpu", node.PerZone(1), GPUNodes()).Taints(node.Taint{Key: GPU, Value: GPUPresent, Effect: node.NoSchedule}),
	)
	s.WaitFor().AllNodesReady()
	s.Given().Workload(Checkout, lab.Pods(6), ResilientPlacement())
	s.WaitFor().AllPodsReady()

	s.Then().AllPodsRunning()
	s.Then().PodsOnNodePool("general", 6)
	s.Then().PodsOnNodePool("gpu", 0)

	s.Given().Workload(Inference, lab.Pods(3), ToleratingGPUNodes())
	s.WaitFor().AllPodsReady()

	s.Then().AllPodsRunning()
	s.Then().PodsOnNodePool("gpu", 3)
	s.Then().PodsOnNodePool("general", 0)
}
