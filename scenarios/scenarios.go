// The list of the lab's Scenarios, one line each. Adding a Scenario is a file
// beside this one and a line in it, so the place an engineer finds the
// experiments in is the only place a Scenario is written down, per
// docs/adr/2026-10-03-scenarios-as-go-functions.md.

package scenarios

import "github.com/woditschka/k8s-resilience-lab/internal/lab"

// All is every Scenario of the lab, in the order a run carries them out. It is a
// function rather than a variable, because no package of the lab holds mutable
// process-wide state.
func All() []func(*lab.Scenario) {
	return []func(*lab.Scenario){
		AWorkloadPlacedForResilience_ShouldPlaceThreePodsPerZoneAndOnePerNode,
		ANodeThatStopsResponding_ShouldLeaveThePodsOnTheNodesStillResponding,
		AZoneThatStopsResponding_ShouldRestoreTheNinePodsOnTheZonesStillResponding,
	}
}
