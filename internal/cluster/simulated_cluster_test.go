package cluster

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

const (
	// Each container on the engine says which Scenario it serves.
	resilientPlacementScenario = "resilient-placement"

	threeZones        = 3
	threeNodesPerZone = 3
	ninePods          = 9

	// The second Deployment has a label of its own, so each is spread over the
	// Nodes on its own.
	secondDeploymentName = "basket"
	threePods            = 3

	onePodPerNode = 1
	onePodTooMany = 1

	twoNodesInIt = 2
	noPodAtAll   = 0

	nodesStillResponding   = threeZones*threeNodesPerZone - 1
	onePodLeftBehind       = 1
	podLeftOnTheFailedNode = "checkout-left-behind"

	onePodNotYetCreated = 1

	onePodNotReadyYet = 1

	noPodStillStarting = 0
)

// Against a Node of four cores, half a Node each makes the cpu a tighter bound
// than the Pod count, and more than a whole Node leaves no Node room for a
// single such Pod.
const (
	someNodesCores                 = 4
	halfTheNodesCPU                = scenario.CPU(2 * millicoresPerCore)
	moreCPUThanANodeHas            = scenario.CPU((someNodesCores + 1) * millicoresPerCore)
	threePodsRequestingHalfANode   = 3
	twoPodsThatFit                 = 2
	onePodThatDoesNotFit           = 1
	twoPodsRequestingMoreThanANode = 2
)

// Each Node can hold more Pods than the Deployment declares.
func anEvenSpreadTopology(t *testing.T) scenario.Topology {
	t.Helper()

	topology, err := topologyOfTheGeneralNodePool(threeZones, threeNodesPerZone, aCapacity(t))
	if err != nil {
		t.Fatalf("build a Topology of %d Zones of %d Nodes: %v", threeZones, threeNodesPerZone, err)
	}

	return topology
}

// The predefined specifications live in the Scenarios' place, which imports
// internal/lab, which imports this package, so this suite builds the same
// settings with internal/scenario's constructors.
func resilientPlacement(t *testing.T) scenario.PodSpecification {
	t.Helper()

	return defaultPlacement(t).With(
		anEvenSpreadOver(scenario.ZoneLabel, evenSkew),
		anEvenSpreadOver(scenario.HostnameLabel, evenSkew),
	)
}

func defaultPlacement(t *testing.T) scenario.PodSpecification {
	t.Helper()

	return aSpecificationOf(t,
		aNodeConditionToleration(scenario.TaintNodeNotReady),
		aNodeConditionToleration(scenario.TaintNodeUnreachable),
	)
}

var noDeploymentDeclaredYet scenario.Deployment

func aDeploymentDeclaring(t *testing.T, pods int, declaring scenario.PodSpecification) scenario.Deployment {
	t.Helper()

	return aDeploymentNamed(t, deployedDeploymentName, pods, declaring)
}

func aSecondDeploymentDeclaring(t *testing.T, pods int, declaring scenario.PodSpecification) scenario.Deployment {
	t.Helper()

	return aDeploymentNamed(t, secondDeploymentName, pods, declaring)
}

func aDeploymentNamed(t *testing.T, name string, pods int, declaring scenario.PodSpecification) scenario.Deployment {
	t.Helper()

	deployment, err := scenario.NewDeployment(name, pods, declaring)
	if err != nil {
		t.Fatalf("build the Deployment %s of %d Pods: %v", name, pods, err)
	}

	return deployment
}

// On one Zone a spread over Zones is satisfied by every Placement, so the
// spread over Nodes is the only one left to honour.
func aTopologyOfOneZone(t *testing.T) scenario.Topology {
	t.Helper()

	topology, err := topologyOfTheGeneralNodePool(theOneZone, threeNodesPerZone, aCapacity(t))
	if err != nil {
		t.Fatalf("build a Topology of one Zone of %d Nodes: %v", threeNodesPerZone, err)
	}

	return topology
}

func aTopologyOfOneNode(t *testing.T) scenario.Topology {
	t.Helper()

	topology, err := topologyOfTheGeneralNodePool(theOneZone, theOneNodeInIt, aCapacity(t))
	if err != nil {
		t.Fatalf("build a Topology of one Node: %v", err)
	}

	return topology
}

func evenlyOverTheNodesOf(topology scenario.Topology) map[string]int {
	counts := make(map[string]int, len(topology.Nodes()))
	for _, node := range topology.Nodes() {
		counts[node.Name] = onePodPerNode
	}

	return counts
}

func podsCounted(counts map[string]int) int {
	counted := 0
	for _, count := range counts {
		counted += count
	}

	return counted
}

// Reading a Pod's own spec is this suite's call: the lab reads a Placement,
// which names a Pod and its Node and not the settings it was deployed with.
func aDeployedPodOf(t *testing.T, provisioned *SimulatedCluster, deployment scenario.Deployment) corev1.PodSpec {
	t.Helper()

	held, err := provisioned.client.CoreV1().Pods(clusterNamespace).List(t.Context(), metav1.ListOptions{
		LabelSelector: podSelectorOf(deployment),
	})
	if err != nil {
		t.Fatalf("read the Pods of the Deployment %s: %v", deployment.Name(), err)
	}
	if len(held.Items) == 0 {
		t.Fatalf("the cluster holds a Pod of the Deployment %s, but it reported none", deployment.Name())
	}

	return held.Items[0].Spec
}

// Only the fields the specification names are read, so what the API server
// defaults beside them is passed over.
func theSettingsOf(deployed corev1.PodSpec) []scenario.Setting {
	read := make([]scenario.Setting, 0, len(deployed.Tolerations)+len(deployed.TopologySpreadConstraints))
	for _, tolerated := range deployed.Tolerations {
		read = append(read, scenario.NewToleration(tolerated.Key,
			scenario.TolerationOperator(tolerated.Operator), tolerated.Value, scenario.TaintEffect(tolerated.Effect),
			secondsTolerated(tolerated)))
	}
	for _, constraint := range deployed.TopologySpreadConstraints {
		read = append(read, scenario.NewTopologySpread(constraint.TopologyKey, int(constraint.MaxSkew),
			scenario.UnsatisfiableConstraintAction(constraint.WhenUnsatisfiable), taintsPolicyOf(constraint)))
	}

	return read
}

// A field the deployed Pod left unset reads as the zero value, so it shows up
// as a mismatch rather than as a panic.
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

// A well-formed version with no published image passes validation and fails at
// the pull.
const aVersionWithNoClusterImage = "v1.0.0"

func TestAVersionWithNoClusterImage_ShouldNotBeObtainedAndShouldBeNamed(t *testing.T) {
	t.Parallel()
	engine := requireEngine(t)
	unpublished, err := ParseKubernetesVersion(aVersionWithNoClusterImage)
	if err != nil {
		t.Fatalf("parse the version %s: %v", aVersionWithNoClusterImage, err)
	}

	err = ObtainVersion(t.Context(), engine, unpublished)

	if err == nil {
		t.Fatalf("no cluster image is published for %s, but the version was obtained", aVersionWithNoClusterImage)
	}
	if !strings.Contains(err.Error(), aVersionWithNoClusterImage) {
		t.Errorf("the error names the version it could not obtain, but reads: %v", err)
	}
}

func TestThePinnedVersion_ShouldBeObtainedFromTheEngine(t *testing.T) {
	t.Parallel()
	engine := requireEngine(t)

	if err := ObtainVersion(t.Context(), engine, DefaultKubernetesVersion()); err != nil {
		t.Fatalf("obtain the pinned version %s: %v", DefaultKubernetesVersion(), err)
	}
}

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

// A read of one named container lives in a test, because an exported
// by-identifier call would widen the engine surface the lab holds.
func stillOnTheEngine(t *testing.T, endpoint Endpoint, name string) bool {
	t.Helper()

	var inspected struct {
		ID string `json:"Id"`
	}
	err := engineRequest(t.Context(), endpoint, http.MethodGet, "/containers/"+name+"/json", nil, &inspected)

	return err == nil
}

func TestAProvisionedTopology_ShouldHoldThreeZonesOfThreeReadyNodes(t *testing.T) {
	t.Parallel()
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

// Comparing whole Nodes means a label the control plane added or dropped
// surfaces here rather than as a miscount.
func TestANodePoolStatingLabels_ShouldProvisionNodesCarryingThemBesideTheLabsOwn(t *testing.T) {
	t.Parallel()
	provisioned, _ := aProvisionedCluster(t)
	const oneZone = 1
	topology, err := scenario.NewTopology(oneZone, aNodePoolLabelled(t, "on-demand"), aNodePoolLabelled(t, "spot"))
	if err != nil {
		t.Fatalf("build a Topology of two labelled NodePools: %v", err)
	}

	if err := provisioned.ApplyTopology(t.Context(), topology); err != nil {
		t.Fatalf("apply the Scenario's Topology: %v", err)
	}

	ready, err := provisioned.ReadyNodes(t.Context())
	if err != nil {
		t.Fatalf("read the Nodes the cluster reports Ready: %v", err)
	}
	expect.Expect(t, ready).To(expect.Equal(topology.Nodes()))
}

func aNodePoolLabelled(t *testing.T, capacityType string) scenario.NodePool {
	t.Helper()

	const oneNodePerZone = 1
	labels, err := scenario.NewNodeLabels(map[string]string{capacityTypeLabel: capacityType})
	if err != nil {
		t.Fatalf("build the label %s=%s: %v", capacityTypeLabel, capacityType, err)
	}
	pool, err := scenario.NewNodePool(capacityType, oneNodePerZone, aCapacity(t))
	if err != nil {
		t.Fatalf("build the NodePool %s: %v", capacityType, err)
	}
	labelled, err := pool.Labelled(labels)
	if err != nil {
		t.Fatalf("label the NodePool %s: %v", capacityType, err)
	}

	return labelled
}

func TestADeclaredNodeHoldingNoRunningProbePod_ShouldBeNamedAsStillUnheld(t *testing.T) {
	const oneZone, twoNodesPerZone = 1, 2
	topology, err := topologyOfTheGeneralNodePool(oneZone, twoNodesPerZone, aCapacity(t))
	if err != nil {
		t.Fatalf("build a Topology of %d Nodes: %v", twoNodesPerZone, err)
	}
	declared := topology.Nodes()
	theOneNodeTheSchedulerHolds := declared[1].Name

	unheld := nodesMissingFrom(declared, []string{theOneNodeTheSchedulerHolds})

	expect.Expect(t, unheld).To(expect.Equal([]string{declared[0].Name}))
}

func TestProvisioning_ShouldLeaveNoProbePodOnTheClusterItHandsBack(t *testing.T) {
	t.Parallel()
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

func TestADeploymentDeclaringResilientPlacement_ShouldPlaceOnePodOnEveryNodeOfEveryZone(t *testing.T) {
	t.Parallel()
	provisioned, _ := aProvisionedCluster(t)
	deployment := aDeploymentDeclaring(t, ninePods, resilientPlacement(t))
	deployInto(t, provisioned, anEvenSpreadTopology(t), deployment)

	settled, err := provisioned.AwaitSettled(t.Context(), deployment)
	if err != nil {
		t.Fatalf("await the Placement settling: %v", err)
	}

	podsPerZone := ninePods / threeZones
	expect.Expect(t, settled.PodsInPhase(scenario.PodRunning)).To(expect.HaveLen[scenario.Pod](ninePods))
	expect.Expect(t, settled.PodsPerZone()).To(expect.Equal(map[string]int{
		"zone-1": podsPerZone, "zone-2": podsPerZone, "zone-3": podsPerZone,
	}))
	expect.Expect(t, settled.Skew(scenario.SpreadOverNodes)).To(expect.Equal(0))
	expect.Expect(t, theSettingsOf(aDeployedPodOf(t, provisioned, deployment))).
		To(expect.Equal(deployment.PodSpecification().Settings()))
}

func TestADeploymentDeployedFromACustomizedSpecification_ShouldCarryTheCustomizedSettings(t *testing.T) {
	t.Parallel()
	provisioned, _ := aProvisionedCluster(t)
	theZoneSpreadTheScenarioWrote := anEvenSpreadOver(scenario.ZoneLabel, widerSkew)
	deployment := aDeploymentDeclaring(t, ninePods, resilientPlacement(t).With(theZoneSpreadTheScenarioWrote))
	deployInto(t, provisioned, anEvenSpreadTopology(t), deployment)

	settled, err := provisioned.AwaitSettled(t.Context(), deployment)
	if err != nil {
		t.Fatalf("await the Placement settling: %v", err)
	}

	expect.Expect(t, settled.PodsInPhase(scenario.PodRunning)).To(expect.HaveLen[scenario.Pod](ninePods))
	expect.Expect(t, theSettingsOf(aDeployedPodOf(t, provisioned, deployment))).To(expect.Equal([]scenario.Setting{
		aNodeConditionToleration(scenario.TaintNodeNotReady),
		aNodeConditionToleration(scenario.TaintNodeUnreachable),
		theZoneSpreadTheScenarioWrote,
		anEvenSpreadOver(scenario.HostnameLabel, evenSkew),
	}))
}

// Where Default placement puts its Pods is the scheduler's own choice, so only
// what holds of every Placement is asserted.
func TestADeploymentDeclaringDefaultPlacement_ShouldPlaceEveryPodWithNoCountPromisedPerZone(t *testing.T) {
	t.Parallel()
	provisioned, _ := aProvisionedCluster(t)
	deployment := aDeploymentDeclaring(t, ninePods, defaultPlacement(t))
	deployInto(t, provisioned, anEvenSpreadTopology(t), deployment)

	settled, err := provisioned.AwaitSettled(t.Context(), deployment)
	if err != nil {
		t.Fatalf("await the Placement settling: %v", err)
	}

	expect.Expect(t, settled.PodsInPhase(scenario.PodRunning)).To(expect.HaveLen[scenario.Pod](ninePods))
	expect.Expect(t, podsCounted(settled.PodsPerZone())).To(expect.Equal(ninePods))
	expect.Expect(t, settled.PodsItCannotAccountFor()).To(expect.HaveLen[scenario.Pod](noPodAtAll))
}

func TestTwoDeploymentsDeclaringDifferentSpecifications_ShouldEachBePlacedByTheOneItDeclares(t *testing.T) {
	t.Parallel()
	provisioned, _ := aProvisionedCluster(t)
	spreadEvenly := aDeploymentDeclaring(t, ninePods, resilientPlacement(t))
	spreadNowhere := aSecondDeploymentDeclaring(t, threePods, defaultPlacement(t))
	deployBoth(t, provisioned, anEvenSpreadTopology(t), spreadEvenly, spreadNowhere)

	evenly, nowhere := bothSettled(t, provisioned, spreadEvenly, spreadNowhere)

	expect.Expect(t, evenly.Skew(scenario.SpreadOverNodes)).To(expect.Equal(0))
	expect.Expect(t, nowhere.PodsInPhase(scenario.PodRunning)).To(expect.HaveLen[scenario.Pod](threePods))
	expect.Expect(t, nowhere.PodsItCannotAccountFor()).To(expect.HaveLen[scenario.Pod](noPodAtAll))
}

// A spread constraint selects its Deployment's Pods by their Deployment label, so
// the other's Pods are not counted towards the Skew.
func TestTwoDeploymentsDeclaringTheSameSpecification_ShouldEachBeSpreadWithoutCountingTheOthersPods(t *testing.T) {
	t.Parallel()
	provisioned, _ := aProvisionedCluster(t)
	first := aDeploymentDeclaring(t, ninePods, resilientPlacement(t))
	second := aSecondDeploymentDeclaring(t, ninePods, resilientPlacement(t))
	deployBoth(t, provisioned, anEvenSpreadTopology(t), first, second)

	itsOwn, theOthers := bothSettled(t, provisioned, first, second)

	onePodPerNode := evenlyOverTheNodesOf(anEvenSpreadTopology(t))
	expect.Expect(t, itsOwn.PodsPerNode()).To(expect.Equal(onePodPerNode))
	expect.Expect(t, theOthers.PodsPerNode()).To(expect.Equal(onePodPerNode))
}

// A spread over the Zones of a Topology that has one is satisfied by every
// Placement.
func TestResilientPlacementOnATopologyOfOneZone_ShouldStillPlaceOnePodPerNode(t *testing.T) {
	t.Parallel()
	provisioned, _ := aProvisionedCluster(t)
	oneZone := aTopologyOfOneZone(t)
	deployment := aDeploymentDeclaring(t, threeNodesPerZone, resilientPlacement(t))
	deployInto(t, provisioned, oneZone, deployment)

	settled, err := provisioned.AwaitSettled(t.Context(), deployment)
	if err != nil {
		t.Fatalf("await the Placement settling: %v", err)
	}

	expect.Expect(t, settled.PodsPerNode()).To(expect.Equal(evenlyOverTheNodesOf(oneZone)))
	expect.Expect(t, settled.PodsPerZone()).To(expect.Equal(map[string]int{"zone-1": threeNodesPerZone}))
}

func TestARunAfterAnEarlierRun_ShouldReportOnlyItsOwnNodesAndPods(t *testing.T) {
	t.Parallel()
	provisioned, _ := aProvisionedCluster(t)
	topology := anEvenSpreadTopology(t)
	deployment := aDeploymentDeclaring(t, ninePods, resilientPlacement(t))
	deployInto(t, provisioned, topology, deployment)

	settled, err := provisioned.AwaitSettled(t.Context(), deployment)
	if err != nil {
		t.Fatalf("await the Placement settling: %v", err)
	}

	expect.Expect(t, settled.Nodes()).To(expect.Equal(topology.Nodes()))
	expect.Expect(t, settled.Pods()).To(expect.HaveLen[scenario.Pod](ninePods))
}

// Without spread rules the Pods the scheduler cannot place stay Pending, and
// the control plane's own reason for each is on the Timeline.
func TestADeploymentWithoutSpreadRulesOnATooSmallTopology_ShouldLeaveTheUnplaceablePodsPending(t *testing.T) {
	t.Parallel()

	morePodsThanTheTopologyHolds := somePodsPerNode + onePodTooMany
	deployment := aDeploymentDeclaring(t, morePodsThanTheTopologyHolds, defaultPlacement(t))

	recorded, _, settled := aRecordedScenario(t, aTopologyOfOneNode(t), deployment)

	pending := settled.PodsInPhase(scenario.PodPending)
	expect.Expect(t, settled.PodsInPhase(scenario.PodRunning)).To(expect.HaveLen[scenario.Pod](somePodsPerNode))
	expect.Expect(t, pending).To(expect.HaveLen[scenario.Pod](onePodTooMany))
	expectReported(t, recorded, pending[0].Name, podSchedulingFailed)
}

// Both Deployments are deployed onto one provisioned Topology, because
// provisioning is what this suite's time goes on.
func TestADeploymentRequestingMoreThanItsNodesHaveRoomFor_ShouldLeaveThePodsThatDoNotFitPending(t *testing.T) {
	t.Parallel()
	provisioned, _ := aProvisionedCluster(t)
	fitting := aDeploymentDeclaring(t, threePodsRequestingHalfANode, requestingCPUOf(t, halfTheNodesCPU))
	tooLarge := aSecondDeploymentDeclaring(t, twoPodsRequestingMoreThanANode, requestingCPUOf(t, moreCPUThanANodeHas))
	deployBoth(t, provisioned, aTopologyOfOneNodeOfFourCores(t), fitting, tooLarge)

	placed, refused := bothSettled(t, provisioned, fitting, tooLarge)

	expect.Expect(t, placed.PodsInPhase(scenario.PodRunning)).To(expect.HaveLen[scenario.Pod](twoPodsThatFit))
	expect.Expect(t, placed.PodsInPhase(scenario.PodPending)).To(expect.HaveLen[scenario.Pod](onePodThatDoesNotFit))
	expect.Expect(t, refused.PodsInPhase(scenario.PodRunning)).To(expect.HaveLen[scenario.Pod](noPodAtAll))
	expect.Expect(t, refused.PodsInPhase(scenario.PodPending)).
		To(expect.HaveLen[scenario.Pod](twoPodsRequestingMoreThanANode))
}

// These tests say whether the control plane does the moving: the lab only
// withholds a Node's status, and the taint, the eviction and the replacement
// are the control plane's own doing. They assert no wall-clock bound, since the
// lab does not set the period the control plane takes to notice.
func TestANodeThatStopsResponding_ShouldLeaveNinePodsRunningThreePerZoneAndNoneOnIt(t *testing.T) {
	t.Parallel()
	provisioned, _ := aProvisionedCluster(t)
	topology, deployment := anEvenSpreadTopology(t), aDeploymentDeclaring(t, ninePods, resilientPlacement(t))
	stopped := theFirstNodeOf(topology)
	settleInto(t, provisioned, topology, deployment)

	if err := provisioned.FailNode(t.Context(), stopped); err != nil {
		t.Fatalf("stop Node %s responding: %v", stopped, err)
	}

	settled, err := provisioned.AwaitSettled(t.Context(), deployment)
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

// How the nine Pods are distributed over the two Zones left is deliberately not
// asserted. Both spread constraints honour the taints the stopped Nodes carry,
// so the failed Zone leaves the spread calculation and two Zones take nine
// Pods, skewed by one, which is the impact a Zone Failure is accepted to have.
func TestAZoneThatStopsResponding_ShouldRestoreTheNinePodsOnTheZonesStillResponding(t *testing.T) {
	t.Parallel()
	provisioned, _ := aProvisionedCluster(t)
	topology, deployment := anEvenSpreadTopology(t), aDeploymentDeclaring(t, ninePods, resilientPlacement(t))
	stopped := theFirstZoneOf(topology)
	settleInto(t, provisioned, topology, deployment)

	stopEveryNodeIn(t, provisioned, topology, stopped)

	settled, err := provisioned.AwaitSettled(t.Context(), deployment)
	if err != nil {
		t.Fatalf("await the Placement settling after the Failure: %v", err)
	}
	expect.Expect(t, settled.PodsInPhase(scenario.PodRunning)).To(expect.HaveLen[scenario.Pod](ninePods))
	expect.Expect(t, settled.PodsItCannotAccountFor()).To(expect.HaveLen[scenario.Pod](noPodAtAll))
	expect.Expect(t, settled.PodsPerZone()[stopped]).To(expect.Equal(noPodAtAll))
	expect.Expect(t, podsCounted(settled.PodsPerZone())).To(expect.Equal(ninePods))
}

func TestAPodOfAnUnresponsiveNodeThatCannotBePlaced_ShouldStayPendingWithTheControlPlanesReason(t *testing.T) {
	t.Parallel()
	provisioned, _ := aProvisionedCluster(t)
	topology := aTopologyOfTwoNodesHoldingOnePod(t)
	deployment := aDeploymentDeclaring(t, twoPods, resilientPlacement(t))
	stopped := theFirstNodeOf(topology)
	delivered := &entriesDelivered{}
	if err := provisioned.Record(t.Context(), delivered); err != nil {
		t.Fatalf("record the Timeline of %s: %v", provisioned.Name(), err)
	}
	settleInto(t, provisioned, topology, deployment)

	if err := provisioned.FailNode(t.Context(), stopped); err != nil {
		t.Fatalf("stop Node %s responding: %v", stopped, err)
	}

	settled, err := provisioned.AwaitSettled(t.Context(), deployment)
	if err != nil {
		t.Fatalf("await the Placement settling after the Failure: %v", err)
	}
	recorded := provisioned.StopRecording()
	pending := settled.PodsInPhase(scenario.PodPending)
	expect.Expect(t, settled.PodsInPhase(scenario.PodRunning)).To(expect.HaveLen[scenario.Pod](roomForOnePod))
	expect.Expect(t, pending).To(expect.HaveLen[scenario.Pod](twoPods - roomForOnePod))
	expectReported(t, recorded, pending[0].Name, podSchedulingFailed)
}

func TestAWaitForEveryPodOfADeployedDeployment_ShouldReachThePlacementWithEveryPodReady(t *testing.T) {
	t.Parallel()
	provisioned, _ := aProvisionedCluster(t)
	deployment := aDeploymentDeclaring(t, ninePods, resilientPlacement(t))
	deployInto(t, provisioned, anEvenSpreadTopology(t), deployment)

	ready, err := provisioned.AwaitPodsReady(t.Context(), deployment)
	if err != nil {
		t.Fatalf("await every Pod of the Deployment reporting Ready: %v", err)
	}

	expect.Expect(t, ready.PodsNotReady()).To(expect.HaveLen[scenario.Pod](0))
	expect.Expect(t, ready.PodsInPhase(scenario.PodRunning)).To(expect.HaveLen[scenario.Pod](ninePods))
}

// Each time is longer than the poll interval, so a Pod Ready at placement is
// told from one that waited.
const (
	oneStartupTime  = 4
	shortestStartup = 2
	longestStartup  = 5
)

// The control plane records conditions in whole seconds; the longest is allowed
// one poll interval for the simulator's own round trip.
func TestAPodSpecificationCustomizedWithOneStartupTime_ShouldHaveNoPodReadySoonerThanThatTime(t *testing.T) {
	t.Parallel()
	provisioned, _ := aProvisionedCluster(t)
	deployment := aDeploymentDeclaring(t, threePods, resilientPlacement(t).WithStartupTime(aStartupTimeBetween(t, oneStartupTime*time.Second, oneStartupTime*time.Second)))
	deployInto(t, provisioned, anEvenSpreadTopology(t), deployment)

	if _, err := provisioned.AwaitPodsReady(t.Context(), deployment); err != nil {
		t.Fatalf("await every Pod of the Deployment reporting Ready: %v", err)
	}

	taken := theTimesTakenToBeReady(t, provisioned, deployment)
	expect.Expect(t, takenSoonerThan(taken, oneStartupTime*time.Second)).To(expect.HaveLen[time.Duration](0))
	expect.Expect(t, takenLaterThan(taken, oneStartupTime*time.Second+pollInterval)).To(expect.HaveLen[time.Duration](0))
}

func TestAPodSpecificationCustomizedWithAStartupTimeRange_ShouldHaveEachPodReadyWithinTheRange(t *testing.T) {
	t.Parallel()
	provisioned, _ := aProvisionedCluster(t)
	deployment := aDeploymentDeclaring(t, ninePods, resilientPlacement(t).WithStartupTime(aStartupTimeBetween(t, shortestStartup*time.Second, longestStartup*time.Second)))
	deployInto(t, provisioned, anEvenSpreadTopology(t), deployment)

	if _, err := provisioned.AwaitPodsReady(t.Context(), deployment); err != nil {
		t.Fatalf("await every Pod of the Deployment reporting Ready: %v", err)
	}

	taken := theTimesTakenToBeReady(t, provisioned, deployment)
	expect.Expect(t, takenSoonerThan(taken, shortestStartup*time.Second)).To(expect.HaveLen[time.Duration](0))
	expect.Expect(t, takenLaterThan(taken, longestStartup*time.Second+pollInterval)).To(expect.HaveLen[time.Duration](0))
}

// A Startup time under a second is finer than the whole seconds the control
// plane records, so each Pod's time is read on this test's own clock off a
// watch opened before the deployment. The longest is allowed one poll interval
// for the simulator's own round trip.
func TestADeploymentWhosePodsStartInUnderASecond_ShouldHaveEachPodReadyWithinThatRange(t *testing.T) {
	t.Parallel()
	provisioned, _ := aProvisionedCluster(t)
	deployment := aDeploymentDeclaring(t, ninePods, resilientPlacement(t))
	watched := watchingTheStartupsOf(t, provisioned, deployment)
	deployInto(t, provisioned, anEvenSpreadTopology(t), deployment)

	if _, err := provisioned.AwaitPodsReady(t.Context(), deployment); err != nil {
		t.Fatalf("await every Pod of the Deployment reporting Ready: %v", err)
	}

	taken := watched.timesTaken(t, deployment)
	expect.Expect(t, takenSoonerThan(taken, someShortestStartup)).To(expect.HaveLen[time.Duration](0))
	expect.Expect(t, takenLaterThan(taken, someLongestStartup+pollInterval)).To(expect.HaveLen[time.Duration](0))
}

type startupsWatched struct {
	stop    func()
	stopped <-chan struct{}

	mu     sync.Mutex
	placed map[string]time.Time
	ready  map[string]time.Time
}

// The watch only records; the test waits through AwaitPodsReady alone.
func watchingTheStartupsOf(t *testing.T, provisioned *SimulatedCluster, deployment scenario.Deployment) *startupsWatched {
	t.Helper()

	changes, err := provisioned.client.CoreV1().Pods(clusterNamespace).Watch(t.Context(), metav1.ListOptions{
		LabelSelector: podSelectorOf(deployment),
	})
	if err != nil {
		t.Fatalf("watch the Pods of the Deployment %s: %v", deployment.Name(), err)
	}
	stopped := make(chan struct{})
	watched := &startupsWatched{
		stop: changes.Stop, stopped: stopped,
		placed: map[string]time.Time{}, ready: map[string]time.Time{},
	}
	go func() {
		defer close(stopped)
		for change := range changes.ResultChan() {
			if pod, isPod := change.Object.(*corev1.Pod); isPod {
				watched.record(*pod, time.Now())
			}
		}
	}()
	t.Cleanup(watched.stop)

	return watched
}

func (w *startupsWatched) record(pod corev1.Pod, seen time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if _, already := w.placed[pod.Name]; !already && pod.Spec.NodeName != "" {
		w.placed[pod.Name] = seen
	}
	if _, already := w.ready[pod.Name]; !already && isPodReady(pod) {
		w.ready[pod.Name] = seen
	}
}

// A Pod the watch did not see both ways fails the test by name, since its time
// cannot be judged.
func (w *startupsWatched) timesTaken(t *testing.T, deployment scenario.Deployment) []time.Duration {
	t.Helper()

	w.stop()
	<-w.stopped
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.ready) != deployment.Replicas() {
		t.Fatalf("the Deployment %s declares %d Pods, but the watch saw %d Ready", deployment.Name(), deployment.Replicas(), len(w.ready))
	}
	taken := make([]time.Duration, 0, len(w.ready))
	for pod, ready := range w.ready {
		placed, seen := w.placed[pod]
		if !seen {
			t.Fatalf("the watch saw the Pod %s Ready but never saw it placed", pod)
		}
		taken = append(taken, ready.Sub(placed))
	}

	return taken
}

func theTimesTakenToBeReady(t *testing.T, provisioned *SimulatedCluster, deployment scenario.Deployment) []time.Duration {
	t.Helper()

	held, err := provisioned.client.CoreV1().Pods(clusterNamespace).List(t.Context(), metav1.ListOptions{
		LabelSelector: podSelectorOf(deployment),
	})
	if err != nil {
		t.Fatalf("read the Pods of the Deployment %s: %v", deployment.Name(), err)
	}
	if len(held.Items) != deployment.Replicas() {
		t.Fatalf("the Deployment %s declares %d Pods, but the cluster holds %d", deployment.Name(), deployment.Replicas(), len(held.Items))
	}

	taken := make([]time.Duration, 0, len(held.Items))
	for _, pod := range held.Items {
		taken = append(taken, conditionSince(t, pod, corev1.PodReady).Sub(conditionSince(t, pod, corev1.PodScheduled)))
	}

	return taken
}

func conditionSince(t *testing.T, pod corev1.Pod, kind corev1.PodConditionType) time.Time {
	t.Helper()

	for _, condition := range pod.Status.Conditions {
		if condition.Type == kind && condition.Status == corev1.ConditionTrue {
			return condition.LastTransitionTime.Time
		}
	}
	t.Fatalf("the Pod %s reports no %s condition", pod.Name, kind)

	return time.Time{}
}

func takenSoonerThan(taken []time.Duration, bound time.Duration) []time.Duration {
	return slices.DeleteFunc(slices.Clone(taken), func(took time.Duration) bool { return took >= bound })
}

func takenLaterThan(taken []time.Duration, bound time.Duration) []time.Duration {
	return slices.DeleteFunc(slices.Clone(taken), func(took time.Duration) bool { return took <= bound })
}

func TestAWaitForEveryNodeBeforeADeploymentIsDeployed_ShouldReachThePlacementOfThoseNodesAndNoPod(t *testing.T) {
	t.Parallel()
	provisioned, _ := aProvisionedCluster(t)
	topology := anEvenSpreadTopology(t)
	if err := provisioned.ApplyTopology(t.Context(), topology); err != nil {
		t.Fatalf("apply the Scenario's Topology: %v", err)
	}

	ready, err := provisioned.AwaitNodesReady(t.Context(), topology, noDeploymentDeclaredYet)
	if err != nil {
		t.Fatalf("await every Node of the Topology reporting Ready: %v", err)
	}

	expect.Expect(t, ready.Nodes()).To(expect.Equal(topology.Nodes()))
	expect.Expect(t, ready.Pods()).To(expect.HaveLen[scenario.Pod](0))
}

// What an expired Wait reports is read from the last observation alone, so it
// is asserted here rather than by provoking a Wait that runs its whole budget
// out.
func TestAWaitForEveryPodTheWaitingTimeOutlasts_ShouldNameThePodsThatWereNotReady(t *testing.T) {
	observed := aPlacementWithOnePodNotReadyYet(t)

	reported := podsDidNotBecomeReady(aDeploymentDeclaring(t, ninePods, resilientPlacement(t)), observed)

	expect.Expect(t, reported.Error()).To(expect.Equal(fmt.Sprintf(
		"the Pods of the Deployment %s were not all Ready within %s; it last held %d of %d Pods Ready, and these were not: %s",
		deployedDeploymentName, scenario.WaitingTime, ninePods-onePodNotReadyYet, ninePods, observed.PodsNotReady()[0].Name)))
}

func TestAWaitForEveryNodeTheWaitingTimeOutlasts_ShouldNameTheNodesThatWereNotReady(t *testing.T) {
	topology := anEvenSpreadTopology(t)
	observed := aPlacementShortOfOneReadyNode(t)

	reported := nodesDidNotBecomeReady(topology, observed)

	expect.Expect(t, reported.Error()).To(expect.Equal(fmt.Sprintf(
		"the Nodes of the Topology were not all Ready within %s; these were not: %s",
		scenario.WaitingTime, theFirstNodeOf(topology))))
}

// What an expired settle wait reports is read from the observation alone, and
// no Placement the lab can bring about keeps a wait open, so the report is
// asserted here rather than by running the budget out.
func TestAPlacementThatDoesNotSettleAfterTheFailure_ShouldReportTheObservedPlacement(t *testing.T) {
	observed := aPlacementStillHoldingAPodOnTheFailedNode(t)

	reported := didNotSettle(aDeploymentDeclaring(t, ninePods, resilientPlacement(t)), observed)

	expect.Expect(t, reported.Error()).To(expect.Equal(fmt.Sprintf(
		"the Placement of the Deployment %s did not settle within %s; it last held %d of %d Pods over %d Ready Nodes, %d of them on a Node it does not hold and %d still starting",
		deployedDeploymentName, scenario.WaitingTime, ninePods, ninePods, nodesStillResponding, onePodLeftBehind, noPodStillStarting)))
}

// Each row meets every clause but the one it names, so the row says which
// clause decided it.
func TestAnObservedPlacement_ShouldCountAsSettledOnlyWhenItAccountsForEveryDeclaredPodAndAgreesWithTheOneBeforeIt(t *testing.T) {
	accountedFor := aPlacementOfEveryDeclaredPod(t)
	stillHoldingAPodOnTheFailedNode := aPlacementStillHoldingAPodOnTheFailedNode(t)
	shortOfOnePod := aPlacementShortOfOnePod(t)
	withOnePodStillStarting := aPlacementWithOnePodStillStarting(t)

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
		"an observation holding fewer Pods than the Deployment declares": {
			previous: shortOfOnePod, current: shortOfOnePod, settled: false,
		},
		"two observations that do not agree": {
			previous: shortOfOnePod, current: accountedFor, settled: false,
		},
		"an observation holding a placed Pod still inside its Startup time": {
			previous: withOnePodStillStarting, current: withOnePodStillStarting, settled: false,
		},
	}

	for observation, taken := range observations {
		t.Run(observation, func(t *testing.T) {
			settled := hasSettled(taken.previous, taken.current, aDeploymentDeclaring(t, ninePods, resilientPlacement(t)))

			expect.Expect(t, settled).To(expect.Equal(taken.settled))
		})
	}
}

func aPlacementOfEveryDeclaredPod(t *testing.T) scenario.Placement {
	t.Helper()

	nodes := anEvenSpreadTopology(t).Nodes()

	return scenario.NewPlacement(nodes, onePodOnEachOf(nodes))
}

func aPlacementShortOfOnePod(t *testing.T) scenario.Placement {
	t.Helper()

	nodes := anEvenSpreadTopology(t).Nodes()
	created := onePodOnEachOf(nodes)

	return scenario.NewPlacement(nodes, created[:len(created)-onePodNotYetCreated])
}

func aPlacementWithOnePodNotReadyYet(t *testing.T) scenario.Placement {
	t.Helper()

	nodes := anEvenSpreadTopology(t).Nodes()
	placed := onePodOnEachOf(nodes)
	placed[0].Ready = false

	return scenario.NewPlacement(nodes, placed)
}

func aPlacementWithOnePodStillStarting(t *testing.T) scenario.Placement {
	t.Helper()

	nodes := anEvenSpreadTopology(t).Nodes()
	placed := onePodOnEachOf(nodes)
	placed[0].Phase, placed[0].Ready = scenario.PodPending, false

	return scenario.NewPlacement(nodes, placed)
}

// A Node that is not Ready is in no observed Placement.
func aPlacementShortOfOneReadyNode(t *testing.T) scenario.Placement {
	t.Helper()

	nodes := anEvenSpreadTopology(t).Nodes()
	stillResponding := nodes[1:]

	return scenario.NewPlacement(stillResponding, onePodOnEachOf(stillResponding))
}

// The Pod on the failed Node is the clause that holds the settle wait open.
func aPlacementStillHoldingAPodOnTheFailedNode(t *testing.T) scenario.Placement {
	t.Helper()

	topology := anEvenSpreadTopology(t)
	stopped, stillResponding := theFirstNodeOf(topology), topology.Nodes()[1:]
	leftBehind := scenario.Pod{Name: podLeftOnTheFailedNode, Node: stopped, Phase: scenario.PodRunning}

	return scenario.NewPlacement(stillResponding, append([]scenario.Pod{leftBehind}, onePodOnEachOf(stillResponding)...))
}

// The control plane's own Pod names are not this suite's to pin.
func onePodOnEachOf(nodes []scenario.Node) []scenario.Pod {
	placed := make([]scenario.Pod, 0, len(nodes))
	for at, node := range nodes {
		placed = append(placed, scenario.Pod{
			Name:  fmt.Sprintf("%s-%d", deployedDeploymentName, at),
			Node:  node.Name,
			Phase: scenario.PodRunning,
			Ready: true,
		})
	}

	return placed
}

func TestAScenarioThatEnds_ShouldLeaveNoSimulatedClusterBehind(t *testing.T) {
	t.Parallel()
	endings := map[string]func(*testing.T, *SimulatedCluster){
		"a Scenario whose steps all passed": func(t *testing.T, provisioned *SimulatedCluster) {
			t.Helper()
			deployInto(t, provisioned, anEvenSpreadTopology(t), aDeploymentDeclaring(t, ninePods, resilientPlacement(t)))
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
			t.Parallel()
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
	t.Parallel()
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

func aClusterOfItsOwnInvocation(t *testing.T, engine Endpoint) *SimulatedCluster {
	t.Helper()

	provisioned, err := Provision(t.Context(), engine, DefaultKubernetesVersion(), resilientPlacementScenario)
	if err != nil {
		t.Fatalf("provision a Simulated Cluster beside another: %v", err)
	}
	t.Cleanup(func() { _ = provisioned.Close(context.WithoutCancel(t.Context())) })

	return provisioned
}

// A Scenario stops a Node responding only once everything it declared has
// settled.
func settleInto(t *testing.T, provisioned *SimulatedCluster, topology scenario.Topology, deployment scenario.Deployment) {
	t.Helper()

	deployInto(t, provisioned, topology, deployment)
	if _, err := provisioned.AwaitSettled(t.Context(), deployment); err != nil {
		t.Fatalf("await the Placement settling before the Failure: %v", err)
	}
}

// The Node is derived rather than written out, so the expectation follows the
// Topology asked.
func theFirstNodeOf(topology scenario.Topology) string {
	return topology.Nodes()[0].Name
}

func theFirstZoneOf(topology scenario.Topology) string {
	return topology.Zones()[0]
}

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

// A Node that stopped responding is not among the Nodes the lab observes, so a
// test reads it by name to say it is still in the cluster.
func theNodeNamed(t *testing.T, provisioned *SimulatedCluster, name string) corev1.Node {
	t.Helper()

	held, err := provisioned.client.CoreV1().Nodes().Get(t.Context(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("read the Node %s the cluster holds: %v", name, err)
	}

	return *held
}

func aTopologyOfTwoNodesHoldingOnePod(t *testing.T) scenario.Topology {
	t.Helper()

	capacity := aCapacityOf(t, roomForOnePod, someCPUPerNode, someMemoryPerNode)
	topology, err := topologyOfTheGeneralNodePool(theOneZone, twoNodesInIt, capacity)
	if err != nil {
		t.Fatalf("build a Topology of %d Nodes holding one Pod: %v", twoNodesInIt, err)
	}

	return topology
}

// The Node has room for more Pods than both Deployments declare, so the cpu their
// Pods ask for decides the Placement.
func aTopologyOfOneNodeOfFourCores(t *testing.T) scenario.Topology {
	t.Helper()

	capacity := aCapacityOf(t, somePodsPerNode, scenario.CPU(someNodesCores*millicoresPerCore), someMemoryPerNode)
	topology, err := topologyOfTheGeneralNodePool(theOneZone, theOneNodeInIt, capacity)
	if err != nil {
		t.Fatalf("build a Topology of one Node of %d cores: %v", someNodesCores, err)
	}

	return topology
}

// With no spread, the request alone decides the Placement.
func requestingCPUOf(t *testing.T, cpu scenario.CPU) scenario.PodSpecification {
	t.Helper()

	return defaultPlacement(t).With(scenario.NewRequest(cpu, someRequestedMemory))
}

// A Topology is applied once per cluster, and each Deployment is deployed on its
// own.
func deployBoth(t *testing.T, provisioned *SimulatedCluster, topology scenario.Topology, deployments ...scenario.Deployment) {
	t.Helper()

	if err := provisioned.ApplyTopology(t.Context(), topology); err != nil {
		t.Fatalf("apply the Scenario's Topology: %v", err)
	}
	for _, deployment := range deployments {
		if err := provisioned.Deploy(t.Context(), deployment); err != nil {
			t.Fatalf("deploy the Deployment %s: %v", deployment.Name(), err)
		}
	}
}

func bothSettled(t *testing.T, provisioned *SimulatedCluster, first, second scenario.Deployment) (scenario.Placement, scenario.Placement) {
	t.Helper()

	return settledPlacementOf(t, provisioned, first), settledPlacementOf(t, provisioned, second)
}

func settledPlacementOf(t *testing.T, provisioned *SimulatedCluster, deployment scenario.Deployment) scenario.Placement {
	t.Helper()

	settled, err := provisioned.AwaitSettled(t.Context(), deployment)
	if err != nil {
		t.Fatalf("await the Placement of the Deployment %s settling: %v", deployment.Name(), err)
	}

	return settled
}

func deployInto(t *testing.T, provisioned *SimulatedCluster, topology scenario.Topology, deployment scenario.Deployment) {
	t.Helper()

	if err := provisioned.ApplyTopology(t.Context(), topology); err != nil {
		t.Fatalf("apply the Scenario's Topology: %v", err)
	}
	if err := provisioned.Deploy(t.Context(), deployment); err != nil {
		t.Fatalf("deploy the Scenario's Deployment: %v", err)
	}
}

// Both Nodes have the same capacity, so what keeps a Pod off the gpu Node is
// its taint alone.
func aTopologyOfAGeneralAndATaintedNode(t *testing.T) scenario.Topology {
	t.Helper()

	capacity := aCapacityOf(t, somePodsPerNode, scenario.CPU(someNodesCores*millicoresPerCore), someMemoryPerNode)
	general, err := scenario.NewNodePool(generalNodePool, theOneNodeInIt, capacity)
	if err != nil {
		t.Fatalf("build the general NodePool: %v", err)
	}
	gpu, err := scenario.NewNodePool(gpuNodePool, theOneNodeInIt, capacity)
	if err != nil {
		t.Fatalf("build the gpu NodePool: %v", err)
	}
	taints, err := scenario.NewNodeTaints(aGPUTaint())
	if err != nil {
		t.Fatalf("build the gpu taint: %v", err)
	}
	tainted, err := gpu.Tainted(taints)
	if err != nil {
		t.Fatalf("taint the gpu NodePool: %v", err)
	}
	topology, err := scenario.NewTopology(theOneZone, general, tainted)
	if err != nil {
		t.Fatalf("build a Topology of a general and a tainted Node: %v", err)
	}

	return topology
}

const (
	generalNodePool = "general"
	gpuNodePool     = "gpu"
)

func TestANodePoolStatingANoScheduleTaint_ShouldProvisionNodesCarryingIt(t *testing.T) {
	t.Parallel()
	provisioned, _ := aProvisionedCluster(t)
	topology := aTopologyOfAGeneralAndATaintedNode(t)

	if err := provisioned.ApplyTopology(t.Context(), topology); err != nil {
		t.Fatalf("apply the Scenario's Topology: %v", err)
	}

	ready, err := provisioned.ReadyNodes(t.Context())
	if err != nil {
		t.Fatalf("read the Nodes the cluster reports Ready: %v", err)
	}
	expect.Expect(t, ready).To(expect.Equal(topology.Nodes()))
}

func TestADeploymentNotToleratingATaint_ShouldKeepItsPodsOffTheTaintedNodes(t *testing.T) {
	t.Parallel()
	provisioned, _ := aProvisionedCluster(t)
	deployment := aDeploymentDeclaring(t, threePodsRequestingHalfANode, requestingCPUOf(t, halfTheNodesCPU))
	deployInto(t, provisioned, aTopologyOfAGeneralAndATaintedNode(t), deployment)

	settled := settledPlacementOf(t, provisioned, deployment)

	expect.Expect(t, settled.PodsInPhase(scenario.PodPending)).To(expect.HaveLen[scenario.Pod](onePodThatDoesNotFit))
	expect.Expect(t, settled.PodsPerNodePool()).
		To(expect.Equal(map[string]int{generalNodePool: twoPodsThatFit, gpuNodePool: noPodAtAll}))
}

// A probe Pod is created here beside the Deployments, since provisioning removes
// its own before it returns and an observation would otherwise find none to
// leave out. A Node is failed after both settle, and the observation follows
// before any Pod could move off it.
func TestAnObservationOfEveryDeployment_ShouldReadEachNodesReadinessAndEachPodsRequestButNoProbePod(t *testing.T) {
	t.Parallel()
	provisioned, _ := aProvisionedCluster(t)
	topology := anEvenSpreadTopology(t)
	spreadEvenly := aDeploymentDeclaring(t, ninePods, resilientPlacement(t))
	declaredRequest := scenario.NewRequest(halfTheNodesCPU, someRequestedMemory)
	requestingHalfANode := aSecondDeploymentDeclaring(t, threePods, requestingCPUOf(t, halfTheNodesCPU))
	deployBoth(t, provisioned, topology, spreadEvenly, requestingHalfANode)
	bothSettled(t, provisioned, spreadEvenly, requestingHalfANode)
	aProbePodBeside(t, provisioned, topology.Nodes()[0])
	failed := theFirstNodeOf(topology)
	if err := provisioned.FailNode(t.Context(), failed); err != nil {
		t.Fatalf("fail the Node %s: %v", failed, err)
	}

	observed, err := provisioned.ObserveEveryDeployment(t.Context())

	expect.Expect(t, err).To(expect.Equal[error](nil))
	expect.Expect(t, podsPerDeploymentIn(observed)).To(expect.Equal(map[string]int{
		spreadEvenly.Name():        ninePods,
		requestingHalfANode.Name(): threePods,
	}))
	expect.Expect(t, observed.Nodes()).To(expect.HaveLen[scenario.Node](threeZones * threeNodesPerZone))
	expect.Expect(t, observed.NodesNotReady()).To(expect.Equal([]string{failed}))
	expect.Expect(t, requestsOf(observed, requestingHalfANode)).To(expect.Equal(map[scenario.Request]int{declaredRequest: threePods}))
}

func aProbePodBeside(t *testing.T, provisioned *SimulatedCluster, on scenario.Node) {
	t.Helper()

	if _, err := provisioned.client.CoreV1().Pods(clusterNamespace).Create(t.Context(), toProbePod(on), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create a probe Pod on Node %s: %v", on.Name, err)
	}
}

func requestsOf(observed scenario.Placement, deployment scenario.Deployment) map[scenario.Request]int {
	counted := make(map[scenario.Request]int)
	for _, pod := range observed.Pods() {
		if pod.Deployment == deployment.Name() {
			counted[pod.Request]++
		}
	}

	return counted
}

func podsPerDeploymentIn(observed scenario.Placement) map[string]int {
	counted := make(map[string]int)
	for _, pod := range observed.Pods() {
		counted[pod.Deployment]++
	}

	return counted
}
