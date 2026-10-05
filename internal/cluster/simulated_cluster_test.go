package cluster

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

	// twoNodesInIt is the one Zone's Node count where a Workload has to fit into
	// the Nodes still responding, and noPodAtAll what a Node that stopped
	// responding holds once the control plane has moved its Pods away.
	twoNodesInIt = 2
	noPodAtAll   = 0

	// nodesStillResponding are the Ready Nodes left once one Node of that Topology
	// has stopped responding, and onePodLeftBehind the Pod still on the Node that
	// stopped, which is what an expired settle wait reports it could not account
	// for.
	nodesStillResponding   = threeZones*threeNodesPerZone - 1
	onePodLeftBehind       = 1
	podLeftOnTheFailedNode = "checkout-left-behind"

	// onePodNotYetCreated is the Pod a Deployment's last replica is short by in the
	// moment after the Deployment, which is the other observation no await stops at.
	onePodNotYetCreated = 1
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
		"zone-1": podsPerZone, "zone-2": podsPerZone, "zone-3": podsPerZone,
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

// The Failure tests below are what says whether the control plane does the moving:
// the lab withholds a Node's status and reports its Ready condition Unknown, and
// everything after that — the unreachable taint, the eviction and the replacement
// — is the control plane's own doing, per
// docs/adr/2026-10-05-node-failure-withholds-the-nodes-status.md. They assert no
// wall-clock bound: the lab sets the waiting time a Pod tolerates the taint for
// and not the period the control plane takes to notice, so the budget they run in
// is the settle wait.
func TestANodeThatStopsResponding_ShouldLeaveNinePodsRunningThreePerZoneAndNoneOnIt(t *testing.T) {
	provisioned, _ := aProvisionedCluster(t)
	topology, workload := anEvenSpreadTopology(t), anEvenlySpreadWorkload(t, ninePods)
	stopped := theFirstNodeOf(topology)
	settleInto(t, provisioned, topology, workload)

	if err := provisioned.FailNode(t.Context(), stopped); err != nil {
		t.Fatalf("stop Node %s responding: %v", stopped, err)
	}

	settled, err := provisioned.AwaitSettled(t.Context(), workload)
	if err != nil {
		t.Fatalf("await the Placement settling after the Failure: %v", err)
	}
	podsPerZone := ninePods / threeZones
	expect.Expect(t, readinessOf(theNodeNamed(t, provisioned, stopped))).To(expect.Equal(scenario.NodeNotReady))
	expect.Expect(t, settled.PodsInPhase(scenario.PodRunning)).To(expect.HaveLen[scenario.Pod](ninePods))
	expect.Expect(t, settled.PodsPerZone()).To(expect.Equal(map[string]int{
		"zone-1": podsPerZone, "zone-2": podsPerZone, "zone-3": podsPerZone,
	}))
	expect.Expect(t, settled.PodsPerNode()[stopped]).To(expect.Equal(noPodAtAll))
}

func TestAPodOfAnUnresponsiveNodeThatCannotBePlaced_ShouldStayPendingWithTheControlPlanesReason(t *testing.T) {
	provisioned, _ := aProvisionedCluster(t)
	topology := aTopologyOfTwoNodesHoldingOnePod(t)
	workload := anEvenlySpreadWorkload(t, twoPods)
	stopped := theFirstNodeOf(topology)
	delivered := &entriesDelivered{}
	if err := provisioned.Record(t.Context(), delivered); err != nil {
		t.Fatalf("record the Timeline of %s: %v", provisioned.Name(), err)
	}
	settleInto(t, provisioned, topology, workload)

	if err := provisioned.FailNode(t.Context(), stopped); err != nil {
		t.Fatalf("stop Node %s responding: %v", stopped, err)
	}

	settled, err := provisioned.AwaitSettled(t.Context(), workload)
	if err != nil {
		t.Fatalf("await the Placement settling after the Failure: %v", err)
	}
	recorded := provisioned.StopRecording()
	pending := settled.PodsInPhase(scenario.PodPending)
	expect.Expect(t, settled.PodsInPhase(scenario.PodRunning)).To(expect.HaveLen[scenario.Pod](roomForOnePod))
	expect.Expect(t, pending).To(expect.HaveLen[scenario.Pod](twoPods - roomForOnePod))
	expectReported(t, recorded, pending[0].Name, podSchedulingFailed)
}

// An expired settle wait is the one place the lab hands back a Placement it is
// not satisfied with, and what it reports is read from the observation alone. So
// the report is asserted here rather than by provoking a wait that runs its whole
// budget out: the budget is what the two tests above run in, and no Placement the
// lab can bring about keeps a wait open for it (PRD edge case 3).
func TestAPlacementThatDoesNotSettleAfterTheFailure_ShouldReportTheObservedPlacement(t *testing.T) {
	observed := aPlacementStillHoldingAPodOnTheFailedNode(t)

	reported := didNotSettle(anEvenlySpreadWorkload(t, ninePods), observed)

	expect.Expect(t, reported.Error()).To(expect.Equal(fmt.Sprintf(
		"the Placement of the Workload %s did not settle within %s; it last held %d of %d Pods over %d Ready Nodes, %d of them on a Node it does not hold",
		deployedWorkloadName, settleTimeout, ninePods, ninePods, nodesStillResponding, onePodLeftBehind)))
}

// The rule an await stops at, asserted a clause at a time: an observation can be
// the settled Placement when every Pod the Workload declares has been created,
// every Pod it found is on a Node it was read against, and it agrees with the
// observation before it. Each row below meets every clause but the one it names,
// so the row says which clause decided it.
func TestAnObservedPlacement_ShouldCountAsSettledOnlyWhenItAccountsForEveryDeclaredPodAndAgreesWithTheOneBeforeIt(t *testing.T) {
	accountedFor := aPlacementOfEveryDeclaredPod(t)
	stillHoldingAPodOnTheFailedNode := aPlacementStillHoldingAPodOnTheFailedNode(t)
	shortOfOnePod := aPlacementShortOfOnePod(t)

	observations := map[string]struct {
		previous, current scenario.Placement
		settled           bool
	}{
		"two agreeing observations of every declared Pod on a Node they hold": {
			previous: accountedFor, current: accountedFor, settled: true,
		},
		"an observation still holding a Pod on the Node that stopped responding": {
			previous: stillHoldingAPodOnTheFailedNode, current: stillHoldingAPodOnTheFailedNode, settled: false,
		},
		"an observation holding fewer Pods than the Workload declares": {
			previous: shortOfOnePod, current: shortOfOnePod, settled: false,
		},
		"two observations that do not agree": {
			previous: shortOfOnePod, current: accountedFor, settled: false,
		},
	}

	for observation, taken := range observations {
		t.Run(observation, func(t *testing.T) {
			settled := hasSettled(taken.previous, taken.current, anEvenlySpreadWorkload(t, ninePods))

			expect.Expect(t, settled).To(expect.Equal(taken.settled))
		})
	}
}

// aPlacementOfEveryDeclaredPod is the observation an await can stop at: every Pod
// the Workload declares, each on a Node the observation was read against.
func aPlacementOfEveryDeclaredPod(t *testing.T) scenario.Placement {
	t.Helper()

	nodes := anEvenSpreadTopology(t).Nodes()

	return scenario.NewPlacement(nodes, onePodOnEachOf(nodes))
}

// aPlacementShortOfOnePod is the observation taken in the moment after a
// Deployment, before the control plane has created the last Pod it declares.
func aPlacementShortOfOnePod(t *testing.T) scenario.Placement {
	t.Helper()

	nodes := anEvenSpreadTopology(t).Nodes()
	created := onePodOnEachOf(nodes)

	return scenario.NewPlacement(nodes, created[:len(created)-onePodNotYetCreated])
}

// aPlacementStillHoldingAPodOnTheFailedNode is the observation taken inside the
// window a Failure opens: every Pod the Workload declares exists, the Node that
// stopped responding has left the Ready Nodes, and the Pod it held is still on it
// because the control plane has not deleted it yet. That last Pod is the clause
// that holds the settle wait open.
func aPlacementStillHoldingAPodOnTheFailedNode(t *testing.T) scenario.Placement {
	t.Helper()

	topology := anEvenSpreadTopology(t)
	stopped, stillResponding := theFirstNodeOf(topology), topology.Nodes()[1:]
	leftBehind := scenario.Pod{Name: podLeftOnTheFailedNode, Node: stopped, Phase: scenario.PodRunning}

	return scenario.NewPlacement(stillResponding, append([]scenario.Pod{leftBehind}, onePodOnEachOf(stillResponding)...))
}

// onePodOnEachOf is one Running Pod of the Workload on each of these Nodes, named
// by its place among them as the control plane's own Pod names are not this
// suite's to pin.
func onePodOnEachOf(nodes []scenario.Node) []scenario.Pod {
	placed := make([]scenario.Pod, 0, len(nodes))
	for at, node := range nodes {
		placed = append(placed, scenario.Pod{
			Name:  fmt.Sprintf("%s-%d", deployedWorkloadName, at),
			Node:  node.Name,
			Phase: scenario.PodRunning,
		})
	}

	return placed
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

// settleInto applies a Topology, deploys a Workload and waits for its Placement
// to settle, which is the world a Failure is injected into: a Scenario stops a
// Node responding only once everything it declared has settled.
func settleInto(t *testing.T, provisioned *SimulatedCluster, topology scenario.Topology, workload scenario.Workload) {
	t.Helper()

	deployInto(t, provisioned, topology, workload)
	if _, err := provisioned.AwaitSettled(t.Context(), workload); err != nil {
		t.Fatalf("await the Placement settling before the Failure: %v", err)
	}
}

// theFirstNodeOf is the Node a Failure of one Node stops: the first the Topology
// derives, named here the same way rather than written out, so the expectation
// follows the Topology that is asked.
func theFirstNodeOf(topology scenario.Topology) string {
	return topology.Nodes()[0].Name
}

// theNodeNamed is the Node the cluster holds under this name, whatever it reports
// about it. Reading one Node by name is this suite's own read: the lab reads the
// Nodes a Placement is observed against, which a Node that stopped responding is
// not one of, so a test is what says it is still in the cluster.
func theNodeNamed(t *testing.T, provisioned *SimulatedCluster, name string) corev1.Node {
	t.Helper()

	held, err := provisioned.client.CoreV1().Nodes().Get(t.Context(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("read the Node %s the cluster holds: %v", name, err)
	}

	return *held
}

// aTopologyOfTwoNodesHoldingOnePod is the Topology a Workload of two Pods fills
// exactly, so a Pod of a Node that stops responding has nowhere left to go.
func aTopologyOfTwoNodesHoldingOnePod(t *testing.T) scenario.Topology {
	t.Helper()

	capacity, err := scenario.NewNodeCapacity(roomForOnePod)
	if err != nil {
		t.Fatalf("build a Node capacity of %d Pod: %v", roomForOnePod, err)
	}
	topology, err := scenario.NewTopology(theOneZone, twoNodesInIt, capacity)
	if err != nil {
		t.Fatalf("build a Topology of %d Nodes holding one Pod: %v", twoNodesInIt, err)
	}

	return topology
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
