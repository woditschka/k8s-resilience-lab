// Package scenarios holds the lab's own Scenarios, one file each, the names of
// the Workloads they deploy, the list of them, and the README naming them. It is
// the place an engineer opening the repository finds the experiments in, and it
// holds nothing of the machinery that runs them: the Step surface and the runner
// are internal/lab, per
// docs/adr/2026-10-03-scenarios-as-go-functions.md.
package scenarios

import "github.com/woditschka/k8s-resilience-lab/internal/lab"

// AnEvenlySpreadWorkload_ShouldPlaceThreePodsPerZoneAndOnePerNode is the lab's
// main Scenario, and the one every later Scenario starts from: a Workload of one
// Pod per Node, declaring even spread over the Zones and over the Nodes, placed
// on a Topology whose Nodes each have room for more than one of its Pods. The
// Placement it asserts is therefore the scheduler honouring that declaration,
// not the Nodes running out of room.
//
// It runs as even-spread, the stem of this file.
func AnEvenlySpreadWorkload_ShouldPlaceThreePodsPerZoneAndOnePerNode(s *lab.Scenario) {
	s.Given().Topology(lab.Zones(3), lab.NodesPerZone(3))
	s.Given().Workload(Checkout, lab.Pods(9)).SpreadEvenly(lab.OverZones, lab.OverNodes)

	s.Then().AllPodsRunning()
	s.Then().PodsPerZone(3)
	s.Then().PodsPerNode(1)
}
