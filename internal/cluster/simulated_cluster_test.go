package cluster

import (
	"context"
	"net/http"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

const (
	// evenSpreadScenario names the Scenario whose Topology and Workload the tests
	// below drive, so each container on the engine says which Scenario it serves.
	evenSpreadScenario = "even-spread"

	threeZones        = 3
	threeNodesPerZone = 3
	ninePods          = 9
)

// anEvenSpreadTopology is the Topology the main Scenario provisions: three Zones
// of three Nodes, each Node able to hold more Pods than the Workload declares.
func anEvenSpreadTopology(t *testing.T) scenario.Topology {
	t.Helper()

	topology, err := scenario.NewTopology(threeZones, threeNodesPerZone, aCapacity(t))
	if err != nil {
		t.Fatalf("build a Topology of %d Zones of %d Nodes: %v", threeZones, threeNodesPerZone, err)
	}

	return topology
}

// anEvenlySpreadWorkload declares the nine Pods of the main Scenario, spread
// evenly over the Zones and over the Nodes.
func anEvenlySpreadWorkload(t *testing.T, pods int) scenario.Workload {
	t.Helper()

	workload, err := scenario.NewWorkload(
		deployedWorkloadName, pods,
		spreadRule(t, scenario.SpreadOverZones, evenSkew),
		spreadRule(t, scenario.SpreadOverNodes, evenSkew),
	)
	if err != nil {
		t.Fatalf("build a Workload of %d evenly spread Pods: %v", pods, err)
	}

	return workload
}

// aProvisionedCluster provisions one Simulated Cluster for the test that asks,
// and removes it when that test ends however it ends.
func aProvisionedCluster(t *testing.T) (*SimulatedCluster, Endpoint) {
	t.Helper()

	engine := requireEngine(t)
	provisioned, err := Provision(t.Context(), engine, DefaultKubernetesVersion(), evenSpreadScenario)
	if err != nil {
		t.Fatalf("provision a Simulated Cluster on the engine at %s: %v", engine.Socket(), err)
	}
	t.Cleanup(func() {
		if err := provisioned.Close(context.WithoutCancel(t.Context())); err != nil {
			t.Errorf("remove the Simulated Cluster %s: %v", provisioned.Name(), err)
		}
	})

	return provisioned, engine
}

// stillOnTheEngine reports whether the engine still holds the container named.
// The request is a read of one named container, which is why it lives in a test
// and not in the lab: an exported by-identifier call would widen the engine
// surface the lab holds (docs/system-design.md § Threat Model).
func stillOnTheEngine(t *testing.T, endpoint Endpoint, name string) bool {
	t.Helper()

	var inspected struct {
		ID string `json:"Id"`
	}
	err := engineRequest(t.Context(), endpoint, http.MethodGet, "/containers/"+name+"/json", nil, &inspected)

	return err == nil
}

func TestAProvisionedTopology_ShouldHoldThreeZonesOfThreeReadyNodes(t *testing.T) {
	provisioned, _ := aProvisionedCluster(t)
	topology := anEvenSpreadTopology(t)

	if err := provisioned.ApplyTopology(t.Context(), topology); err != nil {
		t.Fatalf("apply the Scenario's Topology: %v", err)
	}

	ready, err := provisioned.ReadyNodes(t.Context())
	if err != nil {
		t.Fatalf("read the Nodes the cluster reports Ready: %v", err)
	}
	expect.Expect(t, ready).To(expect.Equal(topology.Nodes()))
}

func TestADeclaredNodeHoldingNoRunningProbePod_ShouldBeNamedAsStillUnheld(t *testing.T) {
	const oneZone, twoNodesPerZone = 1, 2
	topology, err := scenario.NewTopology(oneZone, twoNodesPerZone, aCapacity(t))
	if err != nil {
		t.Fatalf("build a Topology of %d Nodes: %v", twoNodesPerZone, err)
	}
	declared := topology.Nodes()
	theOneNodeTheSchedulerHolds := declared[1].Name

	unheld := nodesWithoutProbe(declared, []string{theOneNodeTheSchedulerHolds})

	expect.Expect(t, unheld).To(expect.Equal([]string{declared[0].Name}))
}

func TestProvisioning_ShouldLeaveNoProbePodOnTheClusterItHandsBack(t *testing.T) {
	provisioned, _ := aProvisionedCluster(t)

	if err := provisioned.ApplyTopology(t.Context(), anEvenSpreadTopology(t)); err != nil {
		t.Fatalf("apply the Scenario's Topology: %v", err)
	}

	left, err := provisioned.probePods(t.Context())
	if err != nil {
		t.Fatalf("list the probe Pods of %s: %v", provisioned.Name(), err)
	}
	expect.Expect(t, left).To(expect.HaveLen[corev1.Pod](0))
}

func TestAnEvenlySpreadWorkload_ShouldSettleWithThreePodsPerZoneAndOnePerNode(t *testing.T) {
	provisioned, _ := aProvisionedCluster(t)
	workload := anEvenlySpreadWorkload(t, ninePods)
	deployInto(t, provisioned, anEvenSpreadTopology(t), workload)

	settled, err := provisioned.AwaitSettled(t.Context(), workload)
	if err != nil {
		t.Fatalf("await the Placement settling: %v", err)
	}

	podsPerZone := ninePods / threeZones
	expect.Expect(t, settled.PodsInPhase(scenario.PodRunning)).To(expect.HaveLen[scenario.Pod](ninePods))
	expect.Expect(t, settled.PodsPerZone()).To(expect.Equal(map[string]int{
		"zone-a": podsPerZone, "zone-b": podsPerZone, "zone-c": podsPerZone,
	}))
	expect.Expect(t, settled.Skew(scenario.SpreadOverNodes)).To(expect.Equal(0))
}

func TestARunAfterAnEarlierRun_ShouldReportOnlyItsOwnNodesAndPods(t *testing.T) {
	provisioned, _ := aProvisionedCluster(t)
	topology := anEvenSpreadTopology(t)
	workload := anEvenlySpreadWorkload(t, ninePods)
	deployInto(t, provisioned, topology, workload)

	settled, err := provisioned.AwaitSettled(t.Context(), workload)
	if err != nil {
		t.Fatalf("await the Placement settling: %v", err)
	}

	expect.Expect(t, settled.Nodes()).To(expect.Equal(topology.Nodes()))
	expect.Expect(t, settled.Pods()).To(expect.HaveLen[scenario.Pod](ninePods))
}

func TestAWorkloadLargerThanItsTopology_ShouldReportPendingPods(t *testing.T) {
	provisioned, _ := aProvisionedCluster(t)
	const oneZoneOfOneNode, oneNodePerZone = 1, 1
	tooSmall, err := scenario.NewTopology(oneZoneOfOneNode, oneNodePerZone, aCapacity(t))
	if err != nil {
		t.Fatalf("build a Topology of one Node: %v", err)
	}
	morePodsThanTheTopologyHolds := somePodsPerNode + 1
	workload := anEvenlySpreadWorkload(t, morePodsThanTheTopologyHolds)
	deployInto(t, provisioned, tooSmall, workload)

	settled, err := provisioned.AwaitSettled(t.Context(), workload)
	if err != nil {
		t.Fatalf("await the Placement settling: %v", err)
	}

	expect.Expect(t, settled.PodsInPhase(scenario.PodRunning)).To(expect.HaveLen[scenario.Pod](somePodsPerNode))
	expect.Expect(t, settled.PodsInPhase(scenario.PodPending)).To(expect.HaveLen[scenario.Pod](1))
}

func TestAScenarioThatEnds_ShouldLeaveNoSimulatedClusterBehind(t *testing.T) {
	endings := map[string]func(*testing.T, *SimulatedCluster){
		"a Scenario whose steps all passed": func(t *testing.T, provisioned *SimulatedCluster) {
			t.Helper()
			deployInto(t, provisioned, anEvenSpreadTopology(t), anEvenlySpreadWorkload(t, ninePods))
		},
		"a Scenario that failed part-way through its steps": func(t *testing.T, provisioned *SimulatedCluster) {
			t.Helper()
			topology := anEvenSpreadTopology(t)
			if err := provisioned.ApplyTopology(t.Context(), topology); err != nil {
				t.Fatalf("apply the Scenario's Topology: %v", err)
			}
			if err := provisioned.ApplyTopology(t.Context(), topology); err == nil {
				t.Fatal("applying a Topology twice fails part-way, but it succeeded")
			}
		},
	}

	for ending, steps := range endings {
		t.Run(ending, func(t *testing.T) {
			engine := requireEngine(t)
			provisioned, err := Provision(t.Context(), engine, DefaultKubernetesVersion(), evenSpreadScenario)
			if err != nil {
				t.Fatalf("provision a Simulated Cluster: %v", err)
			}
			name := provisioned.Name()
			steps(t, provisioned)

			if err := provisioned.Close(t.Context()); err != nil {
				t.Fatalf("remove the Simulated Cluster %s: %v", name, err)
			}

			expect.Expect(t, stillOnTheEngine(t, engine, name)).To(expect.Equal(false))
		})
	}
}

func TestConcurrentRuns_ShouldEachRemoveOnlyTheClusterTheyCreated(t *testing.T) {
	engine := requireEngine(t)
	firstRun, secondRun := aClusterOfItsOwnInvocation(t, engine), aClusterOfItsOwnInvocation(t, engine)

	if firstRun.Name() == secondRun.Name() {
		t.Fatalf("two invocations name their containers apart, but both are %s", firstRun.Name())
	}
	if err := firstRun.Close(t.Context()); err != nil {
		t.Fatalf("remove the first run's Simulated Cluster: %v", err)
	}

	expect.Expect(t, stillOnTheEngine(t, engine, firstRun.Name())).To(expect.Equal(false))
	expect.Expect(t, stillOnTheEngine(t, engine, secondRun.Name())).To(expect.Equal(true))
}

// aClusterOfItsOwnInvocation stands in for one invocation of the lab: a Simulated
// Cluster provisioned beside another, removed when the test ends if its own test
// did not remove it.
func aClusterOfItsOwnInvocation(t *testing.T, engine Endpoint) *SimulatedCluster {
	t.Helper()

	provisioned, err := Provision(t.Context(), engine, DefaultKubernetesVersion(), evenSpreadScenario)
	if err != nil {
		t.Fatalf("provision a Simulated Cluster beside another: %v", err)
	}
	t.Cleanup(func() { _ = provisioned.Close(context.WithoutCancel(t.Context())) })

	return provisioned
}

// deployInto applies a Topology and deploys a Workload into a provisioned
// cluster, which is the sequence every Placement observation follows.
func deployInto(t *testing.T, provisioned *SimulatedCluster, topology scenario.Topology, workload scenario.Workload) {
	t.Helper()

	if err := provisioned.ApplyTopology(t.Context(), topology); err != nil {
		t.Fatalf("apply the Scenario's Topology: %v", err)
	}
	if err := provisioned.Deploy(t.Context(), workload); err != nil {
		t.Fatalf("deploy the Scenario's Workload: %v", err)
	}
}
