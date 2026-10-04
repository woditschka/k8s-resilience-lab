// Package scenarios holds the lab's own Scenarios. Each is an exported function
// over a Simulated Cluster the runner provisioned for it, per
// docs/adr/2026-10-03-scenarios-as-go-functions.md.
package scenarios

import (
	"context"
	"fmt"

	"github.com/woditschka/k8s-resilience-lab/internal/cluster"
	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// EvenSpreadName is the name of the Even spread Scenario. The container of the
// Simulated Cluster provisioned for it carries this name.
const EvenSpreadName = "even-spread"

// The Even spread Scenario's own shape: three Zones of three Nodes, and a
// Workload of one Pod per Node spread evenly over both domains.
const (
	evenSpreadZones        = 3
	evenSpreadNodesPerZone = 3
	evenSpreadWorkloadName = "web"

	// evenSpreadNodeCapacity leaves room for more than one Pod per Node, so the
	// Placement this Scenario asserts is the scheduler's doing and not what the
	// Nodes had room for.
	evenSpreadNodeCapacity = 10

	// evenSkew is the maximum Skew that makes a spread even: the fullest and the
	// emptiest domain differ by at most one Pod.
	evenSkew = 1
)

// EvenSpread is the lab's main Scenario. It provisions three Zones of three
// Nodes, deploys a Workload of one Pod per Node declaring even spread over the
// Zones and over the Nodes, and asserts that the Settled Placement puts the same
// number of Pods in every Zone and one on every Node.
//
// It returns the Settled Placement so the runner can report it. Failed
// assertions are reported through reported rather than returned: an assertion
// that does not hold is a finding about Kubernetes, not an error in the lab.
func EvenSpread(ctx context.Context, provisioned *cluster.SimulatedCluster, reported expect.T) (scenario.Placement, error) {
	capacity, err := scenario.NewNodeCapacity(evenSpreadNodeCapacity)
	if err != nil {
		return scenario.Placement{}, err
	}
	topology, err := scenario.NewTopology(evenSpreadZones, evenSpreadNodesPerZone, capacity)
	if err != nil {
		return scenario.Placement{}, err
	}
	workload, err := evenlySpreadWorkload(len(topology.Nodes()))
	if err != nil {
		return scenario.Placement{}, err
	}

	if err := provisioned.ApplyTopology(ctx, topology); err != nil {
		return scenario.Placement{}, err
	}
	if err := provisioned.Deploy(ctx, workload); err != nil {
		return scenario.Placement{}, err
	}
	settled, err := provisioned.AwaitSettled(ctx, workload)
	if err != nil {
		return scenario.Placement{}, err
	}

	expect.Expect(reported, settled.PodsInPhase(scenario.PodRunning)).To(expect.HaveLen[scenario.Pod](workload.Pods()))
	expect.Expect(reported, settled.PodsPerZone()).To(expect.Equal(evenlyOver(zonesOf(topology), workload.Pods()/evenSpreadZones)))
	expect.Expect(reported, settled.PodsPerNode()).To(expect.Equal(evenlyOver(nodesOf(topology), 1)))

	return settled, nil
}

// evenlySpreadWorkload declares pods replicas spread evenly over the Zones and
// over the Nodes.
func evenlySpreadWorkload(pods int) (scenario.Workload, error) {
	overZones, err := scenario.NewSpreadRule(scenario.SpreadOverZones, evenSkew)
	if err != nil {
		return scenario.Workload{}, err
	}
	overNodes, err := scenario.NewSpreadRule(scenario.SpreadOverNodes, evenSkew)
	if err != nil {
		return scenario.Workload{}, err
	}

	workload, err := scenario.NewWorkload(evenSpreadWorkloadName, pods, overZones, overNodes)
	if err != nil {
		return scenario.Workload{}, fmt.Errorf("declare the Even spread Workload: %w", err)
	}

	return workload, nil
}

// evenlyOver is the Placement this Scenario expects: each of names holding the
// same count.
func evenlyOver(names []string, count int) map[string]int {
	expected := make(map[string]int, len(names))
	for _, name := range names {
		expected[name] = count
	}

	return expected
}

// zonesOf are the Zone names of the Topology, each named once.
func zonesOf(topology scenario.Topology) []string {
	var zones []string
	seen := make(map[string]bool)
	for _, node := range topology.Nodes() {
		if !seen[node.Zone] {
			seen[node.Zone] = true
			zones = append(zones, node.Zone)
		}
	}

	return zones
}

// nodesOf are the Node names of the Topology.
func nodesOf(topology scenario.Topology) []string {
	nodes := topology.Nodes()
	names := make([]string, 0, len(nodes))
	for _, node := range nodes {
		names = append(names, node.Name)
	}

	return names
}
