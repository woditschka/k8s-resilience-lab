//go:build !unit

package cluster

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
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

var noDeploymentDeclaredYet scenario.Deployment

func aSecondDeploymentDeclaring(t *testing.T, pods int, declaring scenario.PodSpecification) scenario.Deployment {
	t.Helper()

	return aDeploymentNamed(t, secondDeploymentName, pods, declaring)
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

// stoppedProvisioning is what a stop during provisioning leaves: whether the
// provisioning was refused, and whether its container is still on the engine.
type stoppedProvisioning struct {
	Refused          bool
	StillOnTheEngine bool
}

// The container starts under a live context and the rest of the provisioning
// runs under a cancelled one, as when the lab stops while the control plane is
// still coming up.
func TestAClusterWhoseProvisioningIsStopped_ShouldBeRemovedAndLeaveNoContainer(t *testing.T) {
	t.Parallel()
	endpoint := requireEngine(t)
	engine, err := newEngine(endpoint, resilientPlacementScenario)
	if err != nil {
		t.Fatalf("name the Simulated Cluster's container: %v", err)
	}
	started, err := engine.StartCluster(t.Context(), DefaultKubernetesVersion())
	if err != nil {
		t.Fatalf("start a Simulated Cluster on the engine at %s: %v", endpoint.Socket(), err)
	}
	t.Cleanup(func() {
		if stillOnTheEngine(t, endpoint, started.Name()) {
			_ = started.Remove(context.WithoutCancel(t.Context()))
		}
	})
	stopped, theLabStops := context.WithCancel(t.Context())
	theLabStops()

	_, err = provisionStarted(stopped, started)

	expect.Expect(t, stoppedProvisioning{
		Refused: errors.Is(err, context.Canceled), StillOnTheEngine: stillOnTheEngine(t, endpoint, started.Name()),
	}).To(expect.Equal(stoppedProvisioning{Refused: true}))
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

const (
	manyNodesInTheZone       = 10
	laterNodesTakenOffToo    = true
	onlyTheFirstNodeTakenOff = 1
)

// Stopping a Node takes a write taking it off the KWOK controller and then a
// status write. Nodes stopped one after the other take each Node off only once
// the Node before reports NotReady, so a later Node taken off before the first
// reports NotReady shows the Nodes stopping together.
func TestAZoneOfManyNodesFailingByAClickOrAWhen_ShouldStopItsLastNodeAsSoonAfterItsFirstAsAZoneOfTwo(t *testing.T) {
	t.Parallel()
	provisioned, _ := aProvisionedCluster(t)
	topology, err := topologyOfTheGeneralNodePool(theOneZone, manyNodesInTheZone, aCapacity(t))
	if err != nil {
		t.Fatalf("build a Topology of %d Nodes in one Zone: %v", manyNodesInTheZone, err)
	}
	if err := provisioned.ApplyTopology(t.Context(), topology); err != nil {
		t.Fatalf("apply the Topology of %d Nodes: %v", manyNodesInTheZone, err)
	}
	failing := nodeNamesOf(topology.Nodes())
	watching := watchingTheNodesOf(t, provisioned)

	if _, err := provisioned.FailNodes(t.Context(), failing); err != nil {
		t.Fatalf("stop the Nodes %v responding: %v", failing, err)
	}

	takenOff := watching.theNodesTakenOffBeforeNotReady(t, failing[0])
	expect.Expect(t, len(takenOff) > onlyTheFirstNodeTakenOff).To(expect.Equal(laterNodesTakenOffToo))
}

type nodesWatched struct {
	events <-chan watch.Event
}

// The watch starts from the Nodes listed now, so every write after it reaches
// the test in the order the API server committed it.
func watchingTheNodesOf(t *testing.T, provisioned *SimulatedCluster) nodesWatched {
	t.Helper()

	listed, err := provisioned.client.CoreV1().Nodes().List(t.Context(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list the Nodes of %s: %v", provisioned.Name(), err)
	}
	watched, err := provisioned.client.CoreV1().Nodes().Watch(t.Context(), metav1.ListOptions{ResourceVersion: listed.ResourceVersion})
	if err != nil {
		t.Fatalf("watch the Nodes of %s: %v", provisioned.Name(), err)
	}
	t.Cleanup(watched.Stop)

	return nodesWatched{events: watched.ResultChan()}
}

// theNodesTakenOffBeforeNotReady reads the writes up to the first one reporting
// the Node NotReady, naming every Node taken off the KWOK controller by then.
func (w nodesWatched) theNodesTakenOffBeforeNotReady(t *testing.T, first string) []string {
	t.Helper()

	takenOff := []string{}
	giveUp := time.After(scenario.WaitingTime)
	for {
		select {
		case event, open := <-w.events:
			if !open {
				t.Fatalf("the watch of the Nodes ended before Node %s reported NotReady", first)
			}
			changed, isNode := event.Object.(*corev1.Node)
			if !isNode {
				continue
			}
			if _, marked := changed.Labels[fakeNodeLabel]; !marked && !slices.Contains(takenOff, changed.Name) {
				takenOff = append(takenOff, changed.Name)
			}
			if changed.Name == first && readinessOf(*changed) == scenario.NodeNotReady {
				return takenOff
			}
		case <-giveUp:
			t.Fatalf("Node %s reported no NotReady within %s", first, scenario.WaitingTime)
		}
	}
}

// Each Node has room for one Pod, so the Pod the Failure left Pending can run
// on the restored Node alone. Its running there proves the Node Ready,
// untainted and maintained again: the scheduler places no Pod on a Node
// tainted unreachable, and only the KWOK controller starts a Pod on a Node.
func TestANodeRestoredAfterItStoppedResponding_ShouldBeReadyUntaintedAndMaintainedAgain(t *testing.T) {
	t.Parallel()
	provisioned, _ := aProvisionedCluster(t)
	topology := aTopologyOfTwoNodesHoldingOnePod(t)
	deployment := aDeploymentDeclaring(t, twoPods, resilientPlacement(t))
	restored := theFirstNodeOf(topology)
	settleInto(t, provisioned, topology, deployment)
	if err := provisioned.FailNode(t.Context(), restored); err != nil {
		t.Fatalf("stop Node %s responding: %v", restored, err)
	}
	if _, err := provisioned.AwaitSettled(t.Context(), deployment); err != nil {
		t.Fatalf("await the Placement settling after the Failure: %v", err)
	}

	if err := provisioned.RestoreNode(t.Context(), restored); err != nil {
		t.Fatalf("make Node %s respond again: %v", restored, err)
	}

	ready, err := provisioned.AwaitPodsReady(t.Context(), deployment)
	if err != nil {
		t.Fatalf("await the Pod left Pending running on the restored Node %s: %v", restored, err)
	}
	expect.Expect(t, readinessOf(theNodeNamed(t, provisioned, restored))).To(expect.Equal(scenario.NodeReady))
	expect.Expect(t, ready.PodsPerNode()[restored]).To(expect.Equal(roomForOnePod))
}

// A Pod on a Node that stopped responding is the case a graceful delete would
// leave Terminating, since no kubelet confirms it stopped.
func TestAPodLostOnANodeThatStoppedResponding_ShouldBeGoneAndReplacedByItsDeployment(t *testing.T) {
	t.Parallel()
	provisioned, _ := aProvisionedCluster(t)
	topology, deployment := aTopologyOfOneZone(t), aDeploymentDeclaring(t, threePods, resilientPlacement(t))
	stopped := theFirstNodeOf(topology)
	settleInto(t, provisioned, topology, deployment)
	lost := thePodPlacedOn(t, provisioned, deployment, stopped)
	if err := provisioned.FailNode(t.Context(), stopped); err != nil {
		t.Fatalf("stop Node %s responding: %v", stopped, err)
	}

	gone, err := provisioned.LosePod(t.Context(), lost)
	if err != nil {
		t.Fatalf("lose the Pod %s: %v", lost, err)
	}

	replaced, err := provisioned.AwaitPodsReady(t.Context(), deployment)
	if err != nil {
		t.Fatalf("await the Deployment %s replacing the Pod %s: %v", deployment.Name(), lost, err)
	}
	expect.Expect(t, gone).To(expect.Equal(true))
	expect.Expect(t, replaced.Pods()).To(expect.HaveLen[scenario.Pod](threePods))
	expect.Expect(t, slices.Contains(podNamesOf(replaced.Pods()), lost)).To(expect.Equal(false))
}

const aPodTheClusterNeverHeld = "checkout-5fc87ff7c8-n0n3x"

func TestLosingAPodTheClusterDoesNotHold_ShouldLoseNothing(t *testing.T) {
	t.Parallel()
	provisioned, _ := aProvisionedCluster(t)

	gone, err := provisioned.LosePod(t.Context(), aPodTheClusterNeverHeld)

	expect.Expect(t, err).To(expect.Equal[error](nil))
	expect.Expect(t, gone).To(expect.Equal(false))
}

func thePodPlacedOn(t *testing.T, provisioned *SimulatedCluster, deployment scenario.Deployment, node string) string {
	t.Helper()

	observed, err := provisioned.Observe(t.Context(), deployment)
	if err != nil {
		t.Fatalf("observe the Deployment %s: %v", deployment.Name(), err)
	}
	at := slices.IndexFunc(observed.Pods(), func(pod scenario.Pod) bool { return pod.Node == node })
	if at < 0 {
		t.Fatalf("no Pod of the Deployment %s is placed on Node %s", deployment.Name(), node)
	}

	return observed.Pods()[at].Name
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
