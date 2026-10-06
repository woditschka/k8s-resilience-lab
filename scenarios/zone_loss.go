package scenarios

import "github.com/woditschka/k8s-resilience-lab/internal/lab"

// AZoneThatStopsResponding_ShouldRestoreTheNinePodsOnTheZonesStillResponding is
// the lab's Zone experiment. It declares the Resilient placement Scenario's world
// and judges the Placement it settles at — the Workload whole, three Pods in every
// Zone — so the world the Failure is injected into is judged rather than assumed.
// Then every Node of one Zone stops responding at once, and it judges what the
// control plane reaches next: the Workload whole again, and no Pod left in the
// Zone that stopped.
//
// It judges no count per Zone after the Failure, and deliberately so. Two Zones
// take the nine Pods, so the Placement is skewed by one Pod per Zone and two
// Nodes carry two — the impact on reliability a lost Zone is accepted to have.
// What has to hold is the capacity: nine Pods Running, off the Zone that failed.
//
// The Workload declares that capacity outright: six of its nine Pods Running
// through a Failure. A lost Zone takes three Pods with it, so six are left
// Running at the moment it stops — exactly the minimum — and all nine are
// Running once the control plane has settled, which the last Then states beside
// the minimum and judges at each moment.
//
// The When names no Zone. It says how many fail, and the lab picks the first
// Zones the declared Topology derives — the same pick on every run — and renames
// the Step to the Zones it resolved, so the run reports which one stopped.
//
// It runs as zone-loss, the stem of this file.
func AZoneThatStopsResponding_ShouldRestoreTheNinePodsOnTheZonesStillResponding(s *lab.Scenario) {
	s.Given().Topology(lab.Zones(3), lab.NodesPerZone(3))
	s.Given().Workload(Checkout, lab.Pods(9), lab.ResilientPlacement()).RequiredCapacity(lab.Pods(6))

	s.Then().AllPodsRunning()
	s.Then().PodsPerZone(3)

	s.When().ZonesFail(lab.Zones(1))

	s.Then().AllPodsRunning()
	s.Then().NoPodsInTheFailedZones()
	s.Then().RequiredCapacityHeld()
}
