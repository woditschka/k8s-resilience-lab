package scenarios

import "github.com/woditschka/k8s-resilience-lab/internal/lab"

// ANodeThatStopsResponding_ShouldLeaveThePodsOnTheNodesStillResponding is the
// lab's first Failure experiment. It declares the Resilient placement Scenario's
// world and judges the Placement it settles at — the Workload whole, three Pods
// in every Zone — so the world the Failure is injected into is judged rather than
// assumed. Then it stops one Node responding and judges the Placement the
// control plane reaches next: the Workload whole again, still three Pods in every
// Zone, and none left on the Node that stopped.
//
// The When names no Node. It says how many fail, and the lab picks the first
// Nodes the declared Topology derives — the same pick on every run — and renames
// the Step to the Nodes it resolved, so the run reports which one stopped.
//
// The Workload declares the capacity it has to keep through a Failure: six of
// its nine Pods Running. Every Node here holds one Pod, so eight are left
// Running at the moment the Failure is injected and the ninth is placed again
// once the control plane has settled — both above the minimum, which the last
// Then states beside it and judges at each moment.
//
// Nothing here moves a Pod. The lab stops a Node responding; noticing it,
// tainting it and replacing its Pods elsewhere are the control plane's own, which
// is what makes the judged Placement an observation about Kubernetes.
//
// It runs as node-loss, the stem of this file.
func ANodeThatStopsResponding_ShouldLeaveThePodsOnTheNodesStillResponding(s *lab.Scenario) {
	s.Given().Topology(lab.Zones(3), lab.NodesPerZone(3))
	s.Given().Workload(Checkout, lab.Pods(9), lab.ResilientPlacement()).RequiredCapacity(lab.Pods(6))

	s.Then().AllPodsRunning()
	s.Then().PodsPerZone(3)

	s.When().NodesFail(lab.Nodes(1))

	s.Then().AllPodsRunning()
	s.Then().PodsPerZone(3)
	s.Then().NoPodsOnTheFailedNodes()
	s.Then().RequiredCapacityHeld()
}
