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
	// resilientPlacementScenario names the Scenario whose Topology and Workload the
	// tests below drive, so each container on the engine says which Scenario it
	// serves.
	resilientPlacementScenario = "resilient-placement"

	threeZones        = 3
	threeNodesPerZone = 3
	ninePods          = 9

	// secondWorkloadName is what the second Workload of a Scenario deploying two
	// is labelled with, so each is spread over the Nodes on its own, and threePods
	// is what that second Workload declares where its own count does not matter.
	secondWorkloadName = "basket"
	threePods          = 3

	// onePodPerNode is the share an even spread over the Nodes gives each of them,
	// and onePodTooMany the one Pod a Topology has no room left for.
	onePodPerNode = 1
	onePodTooMany = 1

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

	// onePodNotReadyYet is the Pod the control plane has placed and not yet
	// reported Ready, which is what an expired Wait for every Pod Ready names.
	onePodNotReadyYet = 1
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

// resilientPlacement and defaultPlacement are the two specifications the lab
// defines, as this suite declares them: an even spread over the Zones and over
// the Nodes above the lab's own tolerations, and those tolerations alone. They
// are built with internal/scenario's constructors rather than named from
// internal/lab, which imports this package, so what is deployed here are the
// settings those definitions hold.
func resilientPlacement() scenario.PodSpecification {
	return defaultPlacement().With(
		anEvenSpreadOver(scenario.ZoneLabel, evenSkew),
		anEvenSpreadOver(scenario.HostnameLabel, evenSkew),
	)
}

func defaultPlacement() scenario.PodSpecification {
	return aSpecificationOf(
		aNodeConditionToleration(scenario.TaintNodeNotReady),
		aNodeConditionToleration(scenario.TaintNodeUnreachable),
	)
}

// noWorkloadDeclaredYet is the Workload a Scenario holds before its Workload
// Given has run, which is what a Wait for every Node Ready reads its Placement
// against.
var noWorkloadDeclaredYet scenario.Workload

// aWorkloadDeclaring is the Workload the Scenarios below deploy: pods replicas
// under the one Workload name this suite deploys, declaring the specification
// named. aSecondWorkloadDeclaring is one deployed beside it under a name of its
// own, which is what a Scenario deploying two Workloads declares.
func aWorkloadDeclaring(t *testing.T, pods int, declaring scenario.PodSpecification) scenario.Workload {
	t.Helper()

	return aWorkloadNamed(t, deployedWorkloadName, pods, declaring)
}

func aSecondWorkloadDeclaring(t *testing.T, pods int, declaring scenario.PodSpecification) scenario.Workload {
	t.Helper()

	return aWorkloadNamed(t, secondWorkloadName, pods, declaring)
}

func aWorkloadNamed(t *testing.T, name string, pods int, declaring scenario.PodSpecification) scenario.Workload {
	t.Helper()

	workload, err := scenario.NewWorkload(name, pods, declaring)
	if err != nil {
		t.Fatalf("build the Workload %s of %d Pods: %v", name, pods, err)
	}

	return workload
}

// aTopologyOfOneZone is the Topology edge case 2 is read on: one Zone of the same
// Nodes, so a spread over the Zones is satisfied by every Placement and the
// spread over the Nodes is the only one left to honour. aTopologyOfOneNode is the
// smallest there is, which a Workload larger than its room cannot fit into.
func aTopologyOfOneZone(t *testing.T) scenario.Topology {
	t.Helper()

	topology, err := scenario.NewTopology(theOneZone, threeNodesPerZone, aCapacity(t))
	if err != nil {
		t.Fatalf("build a Topology of one Zone of %d Nodes: %v", threeNodesPerZone, err)
	}

	return topology
}

func aTopologyOfOneNode(t *testing.T) scenario.Topology {
	t.Helper()

	topology, err := scenario.NewTopology(theOneZone, theOneNodeInIt, aCapacity(t))
	if err != nil {
		t.Fatalf("build a Topology of one Node: %v", err)
	}

	return topology
}

// evenlyOverTheNodesOf is the Placement an even spread over the Nodes settles at:
// one Pod on each Node the Topology derives.
func evenlyOverTheNodesOf(topology scenario.Topology) map[string]int {
	counts := make(map[string]int, len(topology.Nodes()))
	for _, node := range topology.Nodes() {
		counts[node.Name] = onePodPerNode
	}

	return counts
}

// podsCounted is how many Pods a count per domain holds altogether, which is what
// a Placement promising no count per Zone can still be read for.
func podsCounted(counts map[string]int) int {
	counted := 0
	for _, count := range counts {
		counted += count
	}

	return counted
}

// aDeployedPodOf is the spec of one Pod of this Workload as the cluster holds it.
// Reading a Pod's own spec is this suite's call: the lab reads a Placement, which
// names a Pod and the Node it landed on and not the settings it was deployed
// with.
func aDeployedPodOf(t *testing.T, provisioned *SimulatedCluster, workload scenario.Workload) corev1.PodSpec {
	t.Helper()

	held, err := provisioned.client.CoreV1().Pods(workloadNamespace).List(t.Context(), metav1.ListOptions{
		LabelSelector: podSelectorOf(workload),
	})
	if err != nil {
		t.Fatalf("read the Pods of the Workload %s: %v", workload.Name(), err)
	}
	if len(held.Items) == 0 {
		t.Fatalf("the cluster holds a Pod of the Workload %s, but it reported none", workload.Name())
	}

	return held.Items[0].Spec
}

// theSettingsOf are the settings one deployed Pod carries, read back in the lab's
// own vocabulary: the fields its specification names and no other, so what the
// API server defaults beside them is passed over rather than asserted.
func theSettingsOf(deployed corev1.PodSpec) []scenario.Setting {
	read := make([]scenario.Setting, 0, len(deployed.Tolerations)+len(deployed.TopologySpreadConstraints))
	for _, tolerated := range deployed.Tolerations {
		read = append(read, scenario.NewToleration(tolerated.Key,
			scenario.TolerationOperator(tolerated.Operator), scenario.TaintEffect(tolerated.Effect),
			secondsTolerated(tolerated)))
	}
	for _, constraint := range deployed.TopologySpreadConstraints {
		read = append(read, scenario.NewTopologySpread(constraint.TopologyKey, int(constraint.MaxSkew),
			scenario.UnsatisfiableConstraintAction(constraint.WhenUnsatisfiable), taintsPolicyOf(constraint)))
	}

	return read
}

// secondsTolerated and taintsPolicyOf read the two fields the client library
// carries as pointers. A field the deployed Pod left unset reads as the zero
// value, so it shows up as a mismatch against the specification rather than as a
// panic.
func secondsTolerated(tolerated corev1.Toleration) int64 {
	if tolerated.TolerationSeconds == nil {
		return 0
	}

	return *tolerated.TolerationSeconds
}

func taintsPolicyOf(constraint corev1.TopologySpreadConstraint) scenario.NodeInclusionPolicy {
	if constraint.NodeTaintsPolicy == nil {
		return ""
	}

	return scenario.NodeInclusionPolicy(*constraint.NodeTaintsPolicy)
}

// aProvisionedCluster provisions one Simulated Cluster for the test that asks,
// and removes it when that test ends however it ends.
func aProvisionedCluster(t *testing.T) (*SimulatedCluster, Endpoint) {
	t.Helper()

	engine := requireEngine(t)
	provisioned, err := Provision(t.Context(), engine, DefaultKubernetesVersion(), resilientPlacementScenario)
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

	unheld := nodesMissingFrom(declared, []string{theOneNodeTheSchedulerHolds})

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

// A Workload declaring Resilient placement settles at one Pod on every Node of
// every Zone, and the Pods it was deployed with carry the settings that
// specification names — read back off a Pod the cluster holds, which is
// acceptance criterion three of REQ-SIM-009.
func TestAWorkloadDeclaringResilientPlacement_ShouldPlaceOnePodOnEveryNodeOfEveryZone(t *testing.T) {
	provisioned, _ := aProvisionedCluster(t)
	workload := aWorkloadDeclaring(t, ninePods, resilientPlacement())
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
	expect.Expect(t, theSettingsOf(aDeployedPodOf(t, provisioned, workload))).
		To(expect.Equal(workload.PodSpecification().Settings()))
}

// A Workload declaring a specification it customized is deployed with the
// Customization's settings and with the specification's others standing: the Zone
// spread at the Skew the Scenario wrote, and the spread over the Nodes and the
// lab's own tolerations as Resilient placement defines them. Read back off a Pod
// the cluster holds, which is acceptance criterion two of REQ-SIM-009.
func TestAWorkloadDeployedFromACustomizedSpecification_ShouldCarryTheCustomizedSettings(t *testing.T) {
	provisioned, _ := aProvisionedCluster(t)
	theZoneSpreadTheScenarioWrote := anEvenSpreadOver(scenario.ZoneLabel, widerSkew)
	workload := aWorkloadDeclaring(t, ninePods, resilientPlacement().With(theZoneSpreadTheScenarioWrote))
	deployInto(t, provisioned, anEvenSpreadTopology(t), workload)

	settled, err := provisioned.AwaitSettled(t.Context(), workload)
	if err != nil {
		t.Fatalf("await the Placement settling: %v", err)
	}

	expect.Expect(t, settled.PodsInPhase(scenario.PodRunning)).To(expect.HaveLen[scenario.Pod](ninePods))
	expect.Expect(t, theSettingsOf(aDeployedPodOf(t, provisioned, workload))).To(expect.Equal([]scenario.Setting{
		aNodeConditionToleration(scenario.TaintNodeNotReady),
		aNodeConditionToleration(scenario.TaintNodeUnreachable),
		theZoneSpreadTheScenarioWrote,
		anEvenSpreadOver(scenario.HostnameLabel, evenSkew),
	}))
}

// Where a Workload declaring Default placement has its Pods put is the scheduler's
// own choice, so what holds of every such Placement is all this asserts: every
// Pod Running, and every one of them on a Node the Topology holds.
func TestAWorkloadDeclaringDefaultPlacement_ShouldPlaceEveryPodWithNoCountPromisedPerZone(t *testing.T) {
	provisioned, _ := aProvisionedCluster(t)
	workload := aWorkloadDeclaring(t, ninePods, defaultPlacement())
	deployInto(t, provisioned, anEvenSpreadTopology(t), workload)

	settled, err := provisioned.AwaitSettled(t.Context(), workload)
	if err != nil {
		t.Fatalf("await the Placement settling: %v", err)
	}

	expect.Expect(t, settled.PodsInPhase(scenario.PodRunning)).To(expect.HaveLen[scenario.Pod](ninePods))
	expect.Expect(t, podsCounted(settled.PodsPerZone())).To(expect.Equal(ninePods))
	expect.Expect(t, settled.PodsItCannotAccountFor()).To(expect.HaveLen[scenario.Pod](noPodAtAll))
}

// Two Workloads of one Scenario are each placed by the specification they
// declare: the one declaring Resilient placement is spread one Pod per Node,
// and the one declaring Default placement is placed wherever the scheduler puts it.
func TestTwoWorkloadsDeclaringDifferentSpecifications_ShouldEachBePlacedByTheOneItDeclares(t *testing.T) {
	provisioned, _ := aProvisionedCluster(t)
	spreadEvenly := aWorkloadDeclaring(t, ninePods, resilientPlacement())
	spreadNowhere := aSecondWorkloadDeclaring(t, threePods, defaultPlacement())
	deployBoth(t, provisioned, anEvenSpreadTopology(t), spreadEvenly, spreadNowhere)

	evenly, nowhere := bothSettled(t, provisioned, spreadEvenly, spreadNowhere)

	expect.Expect(t, evenly.Skew(scenario.SpreadOverNodes)).To(expect.Equal(0))
	expect.Expect(t, nowhere.PodsInPhase(scenario.PodRunning)).To(expect.HaveLen[scenario.Pod](threePods))
	expect.Expect(t, nowhere.PodsItCannotAccountFor()).To(expect.HaveLen[scenario.Pod](noPodAtAll))
}

// Edge case 3: two Workloads declaring the same specification are each spread on
// their own. A spread constraint selects its Workload's Pods by their Workload
// label, so the other's Pods are not counted towards the Skew and both reach one
// Pod per Node.
func TestTwoWorkloadsDeclaringTheSameSpecification_ShouldEachBeSpreadWithoutCountingTheOthersPods(t *testing.T) {
	provisioned, _ := aProvisionedCluster(t)
	first := aWorkloadDeclaring(t, ninePods, resilientPlacement())
	second := aSecondWorkloadDeclaring(t, ninePods, resilientPlacement())
	deployBoth(t, provisioned, anEvenSpreadTopology(t), first, second)

	itsOwn, theOthers := bothSettled(t, provisioned, first, second)

	onePodPerNode := evenlyOverTheNodesOf(anEvenSpreadTopology(t))
	expect.Expect(t, itsOwn.PodsPerNode()).To(expect.Equal(onePodPerNode))
	expect.Expect(t, theOthers.PodsPerNode()).To(expect.Equal(onePodPerNode))
}

// Edge case 2: Resilient placement on a Topology of one Zone still lands its
// Pods one per Node, and that one Zone holds them all, because a spread over the
// Zones of a Topology that has one is satisfied by every Placement.
func TestResilientPlacementOnATopologyOfOneZone_ShouldStillPlaceOnePodPerNode(t *testing.T) {
	provisioned, _ := aProvisionedCluster(t)
	oneZone := aTopologyOfOneZone(t)
	workload := aWorkloadDeclaring(t, threeNodesPerZone, resilientPlacement())
	deployInto(t, provisioned, oneZone, workload)

	settled, err := provisioned.AwaitSettled(t.Context(), workload)
	if err != nil {
		t.Fatalf("await the Placement settling: %v", err)
	}

	expect.Expect(t, settled.PodsPerNode()).To(expect.Equal(evenlyOverTheNodesOf(oneZone)))
	expect.Expect(t, settled.PodsPerZone()).To(expect.Equal(map[string]int{"zone-1": threeNodesPerZone}))
}

func TestARunAfterAnEarlierRun_ShouldReportOnlyItsOwnNodesAndPods(t *testing.T) {
	provisioned, _ := aProvisionedCluster(t)
	topology := anEvenSpreadTopology(t)
	workload := aWorkloadDeclaring(t, ninePods, resilientPlacement())
	deployInto(t, provisioned, topology, workload)

	settled, err := provisioned.AwaitSettled(t.Context(), workload)
	if err != nil {
		t.Fatalf("await the Placement settling: %v", err)
	}

	expect.Expect(t, settled.Nodes()).To(expect.Equal(topology.Nodes()))
	expect.Expect(t, settled.Pods()).To(expect.HaveLen[scenario.Pod](ninePods))
}

// Edge case 1: a Workload declaring Default placement on a Topology too small for
// its Pods has no spread to blame. The Nodes run out of room, so the Pods the
// scheduler cannot place stay Pending and the control plane's own reason for each
// is on the Timeline.
func TestAWorkloadWithoutSpreadRulesOnATooSmallTopology_ShouldLeaveTheUnplaceablePodsPending(t *testing.T) {
	morePodsThanTheTopologyHolds := somePodsPerNode + onePodTooMany
	workload := aWorkloadDeclaring(t, morePodsThanTheTopologyHolds, defaultPlacement())

	recorded, _, settled := aRecordedScenario(t, aTopologyOfOneNode(t), workload)

	pending := settled.PodsInPhase(scenario.PodPending)
	expect.Expect(t, settled.PodsInPhase(scenario.PodRunning)).To(expect.HaveLen[scenario.Pod](somePodsPerNode))
	expect.Expect(t, pending).To(expect.HaveLen[scenario.Pod](onePodTooMany))
	expectReported(t, recorded, pending[0].Name, podSchedulingFailed)
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
	topology, workload := anEvenSpreadTopology(t), aWorkloadDeclaring(t, ninePods, resilientPlacement())
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

// A whole Zone that stops responding is the Failure the capacity has to survive:
// the Workload's nine Pods are Running again on the Zones still responding, none
// of them in the Zone that stopped, and none left on a Node the Placement can no
// longer account for.
//
// How the nine are distributed over the two Zones left is not asserted, and
// deliberately so. Both spread constraints honour the taints the stopped Nodes
// carry, so the Zone that failed leaves the spread calculation and two Zones take
// nine Pods: the Placement is skewed by one Pod per Zone, which is the impact on
// reliability a Zone Failure is accepted to have (docs/prd.md REQ-SIM-008).
func TestAZoneThatStopsResponding_ShouldRestoreTheNinePodsOnTheZonesStillResponding(t *testing.T) {
	provisioned, _ := aProvisionedCluster(t)
	topology, workload := anEvenSpreadTopology(t), aWorkloadDeclaring(t, ninePods, resilientPlacement())
	stopped := theFirstZoneOf(topology)
	settleInto(t, provisioned, topology, workload)

	stopEveryNodeIn(t, provisioned, topology, stopped)

	settled, err := provisioned.AwaitSettled(t.Context(), workload)
	if err != nil {
		t.Fatalf("await the Placement settling after the Failure: %v", err)
	}
	expect.Expect(t, settled.PodsInPhase(scenario.PodRunning)).To(expect.HaveLen[scenario.Pod](ninePods))
	expect.Expect(t, settled.PodsItCannotAccountFor()).To(expect.HaveLen[scenario.Pod](noPodAtAll))
	expect.Expect(t, settled.PodsPerZone()[stopped]).To(expect.Equal(noPodAtAll))
	expect.Expect(t, podsCounted(settled.PodsPerZone())).To(expect.Equal(ninePods))
}

func TestAPodOfAnUnresponsiveNodeThatCannotBePlaced_ShouldStayPendingWithTheControlPlanesReason(t *testing.T) {
	provisioned, _ := aProvisionedCluster(t)
	topology := aTopologyOfTwoNodesHoldingOnePod(t)
	workload := aWorkloadDeclaring(t, twoPods, resilientPlacement())
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

// A Wait for every Pod Ready holds until the control plane reports the Ready
// condition on every Pod the Workload declares, which is the condition kubectl
// rollout status waits on (docs/adr/2026-10-07-a-scenario-writes-its-own-wait.md).
func TestAWaitForEveryPodOfADeployedWorkload_ShouldReachThePlacementWithEveryPodReady(t *testing.T) {
	provisioned, _ := aProvisionedCluster(t)
	workload := aWorkloadDeclaring(t, ninePods, resilientPlacement())
	deployInto(t, provisioned, anEvenSpreadTopology(t), workload)

	ready, err := provisioned.AwaitPodsReady(t.Context(), workload)
	if err != nil {
		t.Fatalf("await every Pod of the Workload reporting Ready: %v", err)
	}

	expect.Expect(t, ready.PodsNotReady()).To(expect.HaveLen[scenario.Pod](0))
	expect.Expect(t, ready.PodsInPhase(scenario.PodRunning)).To(expect.HaveLen[scenario.Pod](ninePods))
}

// A Wait for every Node Ready is written with the Topology Given, before any
// Workload is declared, so the Placement it reaches holds the Nodes the Topology
// declared and no Pod at all.
func TestAWaitForEveryNodeBeforeAWorkloadIsDeployed_ShouldReachThePlacementOfThoseNodesAndNoPod(t *testing.T) {
	provisioned, _ := aProvisionedCluster(t)
	topology := anEvenSpreadTopology(t)
	if err := provisioned.ApplyTopology(t.Context(), topology); err != nil {
		t.Fatalf("apply the Scenario's Topology: %v", err)
	}

	ready, err := provisioned.AwaitNodesReady(t.Context(), topology, noWorkloadDeclaredYet)
	if err != nil {
		t.Fatalf("await every Node of the Topology reporting Ready: %v", err)
	}

	expect.Expect(t, ready.Nodes()).To(expect.Equal(topology.Nodes()))
	expect.Expect(t, ready.Pods()).To(expect.HaveLen[scenario.Pod](0))
}

// What each expired Wait reports is read from the last observation alone, so it
// is asserted here rather than by provoking a Wait that runs its whole budget
// out, the way the expired settle wait below is (REQ-SIM-006 edge case: a Wait
// the lab's waiting time outlasts names what was not Ready).
func TestAWaitForEveryPodTheWaitingTimeOutlasts_ShouldNameThePodsThatWereNotReady(t *testing.T) {
	observed := aPlacementWithOnePodNotReadyYet(t)

	reported := podsDidNotBecomeReady(aWorkloadDeclaring(t, ninePods, resilientPlacement()), observed)

	expect.Expect(t, reported.Error()).To(expect.Equal(fmt.Sprintf(
		"the Pods of the Workload %s were not all Ready within %s; it last held %d of %d Pods Ready, and these were not: %s",
		deployedWorkloadName, scenario.WaitingTime, ninePods-onePodNotReadyYet, ninePods, observed.PodsNotReady()[0].Name)))
}

func TestAWaitForEveryNodeTheWaitingTimeOutlasts_ShouldNameTheNodesThatWereNotReady(t *testing.T) {
	topology := anEvenSpreadTopology(t)
	observed := aPlacementShortOfOneReadyNode(t)

	reported := nodesDidNotBecomeReady(topology, observed)

	expect.Expect(t, reported.Error()).To(expect.Equal(fmt.Sprintf(
		"the Nodes of the Topology were not all Ready within %s; these were not: %s",
		scenario.WaitingTime, theFirstNodeOf(topology))))
}

// An expired settle wait is the one place the lab hands back a Placement it is
// not satisfied with, and what it reports is read from the observation alone. So
// the report is asserted here rather than by provoking a wait that runs its whole
// budget out: the budget is what the two tests above run in, and no Placement the
// lab can bring about keeps a wait open for it (PRD edge case 3).
func TestAPlacementThatDoesNotSettleAfterTheFailure_ShouldReportTheObservedPlacement(t *testing.T) {
	observed := aPlacementStillHoldingAPodOnTheFailedNode(t)

	reported := didNotSettle(aWorkloadDeclaring(t, ninePods, resilientPlacement()), observed)

	expect.Expect(t, reported.Error()).To(expect.Equal(fmt.Sprintf(
		"the Placement of the Workload %s did not settle within %s; it last held %d of %d Pods over %d Ready Nodes, %d of them on a Node it does not hold",
		deployedWorkloadName, scenario.WaitingTime, ninePods, ninePods, nodesStillResponding, onePodLeftBehind)))
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
			settled := hasSettled(taken.previous, taken.current, aWorkloadDeclaring(t, ninePods, resilientPlacement()))

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

// aPlacementWithOnePodNotReadyYet is the observation taken while every Pod the
// Workload declares is placed and the control plane has yet to report one of them
// Ready, which is the clause a Wait for every Pod Ready holds open.
func aPlacementWithOnePodNotReadyYet(t *testing.T) scenario.Placement {
	t.Helper()

	nodes := anEvenSpreadTopology(t).Nodes()
	placed := onePodOnEachOf(nodes)
	placed[0].Ready = false

	return scenario.NewPlacement(nodes, placed)
}

// aPlacementShortOfOneReadyNode is the observation taken while one Node the
// Topology declares has yet to report Ready, which is the clause a Wait for every
// Node Ready holds open: a Node that is not Ready is in no Placement the lab
// observes.
func aPlacementShortOfOneReadyNode(t *testing.T) scenario.Placement {
	t.Helper()

	nodes := anEvenSpreadTopology(t).Nodes()
	stillResponding := nodes[1:]

	return scenario.NewPlacement(stillResponding, onePodOnEachOf(stillResponding))
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
			Ready: true,
		})
	}

	return placed
}

func TestAScenarioThatEnds_ShouldLeaveNoSimulatedClusterBehind(t *testing.T) {
	endings := map[string]func(*testing.T, *SimulatedCluster){
		"a Scenario whose steps all passed": func(t *testing.T, provisioned *SimulatedCluster) {
			t.Helper()
			deployInto(t, provisioned, anEvenSpreadTopology(t), aWorkloadDeclaring(t, ninePods, resilientPlacement()))
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
			provisioned, err := Provision(t.Context(), engine, DefaultKubernetesVersion(), resilientPlacementScenario)
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

	provisioned, err := Provision(t.Context(), engine, DefaultKubernetesVersion(), resilientPlacementScenario)
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

// theFirstZoneOf is the Zone a Failure of one Zone stops, named from the Topology
// the same way theFirstNodeOf names its Node.
func theFirstZoneOf(topology scenario.Topology) string {
	return topology.Zones()[0]
}

// stopEveryNodeIn stops every Node of one Zone responding, which is what a
// Failure of a whole Zone leaves the control plane to notice.
func stopEveryNodeIn(t *testing.T, provisioned *SimulatedCluster, topology scenario.Topology, zone string) {
	t.Helper()

	for _, node := range topology.Nodes() {
		if node.Zone != zone {
			continue
		}
		if err := provisioned.FailNode(t.Context(), node.Name); err != nil {
			t.Fatalf("stop Node %s of Zone %s responding: %v", node.Name, zone, err)
		}
	}
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

// deployBoth applies a Topology once and deploys two Workloads into it, which is
// the sequence a Scenario deploying two follows: a Topology is applied once per
// cluster, and each Workload is deployed on its own.
func deployBoth(t *testing.T, provisioned *SimulatedCluster, topology scenario.Topology, workloads ...scenario.Workload) {
	t.Helper()

	if err := provisioned.ApplyTopology(t.Context(), topology); err != nil {
		t.Fatalf("apply the Scenario's Topology: %v", err)
	}
	for _, workload := range workloads {
		if err := provisioned.Deploy(t.Context(), workload); err != nil {
			t.Fatalf("deploy the Workload %s: %v", workload.Name(), err)
		}
	}
}

// bothSettled is the Settled Placement of each of two Workloads, each awaited
// through AwaitSettled and read against its own Pods alone.
func bothSettled(t *testing.T, provisioned *SimulatedCluster, first, second scenario.Workload) (scenario.Placement, scenario.Placement) {
	t.Helper()

	return settledPlacementOf(t, provisioned, first), settledPlacementOf(t, provisioned, second)
}

func settledPlacementOf(t *testing.T, provisioned *SimulatedCluster, workload scenario.Workload) scenario.Placement {
	t.Helper()

	settled, err := provisioned.AwaitSettled(t.Context(), workload)
	if err != nil {
		t.Fatalf("await the Placement of the Workload %s settling: %v", workload.Name(), err)
	}

	return settled
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
