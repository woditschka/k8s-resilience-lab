package cluster

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

const (
	deployedWorkloadName = "web"
	somePodsPerNode      = 10
	somePodCount         = 9
	evenSkew             = 1
	widerSkew            = 2
)

// aWorkload is the named default for a Workload whose name and Pod count the test
// does not turn on; the rules it declares are what each test varies.
func aWorkload(t *testing.T, rules ...scenario.SpreadRule) scenario.Workload {
	t.Helper()

	workload, err := scenario.NewWorkload(deployedWorkloadName, somePodCount, rules...)
	if err != nil {
		t.Fatalf("build a Workload of %d Pods: %v", somePodCount, err)
	}

	return workload
}

func spreadRule(t *testing.T, domain scenario.SpreadDomain, maxSkew int) scenario.SpreadRule {
	t.Helper()

	rule, err := scenario.NewSpreadRule(domain, maxSkew)
	if err != nil {
		t.Fatalf("build a spread rule over %s at a Skew of %d: %v", domain, maxSkew, err)
	}

	return rule
}

// workloadSelector is the selector every spread constraint carries, so a
// constraint counts this Workload's Pods and no others.
func workloadSelector() *metav1.LabelSelector {
	return &metav1.LabelSelector{MatchLabels: map[string]string{workloadLabel: deployedWorkloadName}}
}

func TestEachDeclaredSpreadRule_ShouldBecomeOneEnforcedConstraintAtItsSkew(t *testing.T) {
	overZones := spreadRule(t, scenario.SpreadOverZones, evenSkew)
	overNodes := spreadRule(t, scenario.SpreadOverNodes, widerSkew)
	honourTaints := corev1.NodeInclusionPolicyHonor

	deployed := toDeployment(aWorkload(t, overZones, overNodes))

	expect.Expect(t, deployed.Spec.Template.Spec.TopologySpreadConstraints).To(expect.Equal([]corev1.TopologySpreadConstraint{
		{
			MaxSkew:           int32(evenSkew),
			TopologyKey:       zoneLabel,
			WhenUnsatisfiable: corev1.DoNotSchedule,
			LabelSelector:     workloadSelector(),
			NodeTaintsPolicy:  &honourTaints,
		},
		{
			MaxSkew:           int32(widerSkew),
			TopologyKey:       hostnameLabel,
			WhenUnsatisfiable: corev1.DoNotSchedule,
			LabelSelector:     workloadSelector(),
			NodeTaintsPolicy:  &honourTaints,
		},
	}))
}

func TestAWorkloadDeclaringNoRule_ShouldCarryNoSpreadConstraint(t *testing.T) {
	deployed := toDeployment(aWorkload(t))

	expect.Expect(t, deployed.Spec.Template.Spec.TopologySpreadConstraints).To(expect.HaveLen[corev1.TopologySpreadConstraint](0))
}

// No Pod affinity rule is set: a required anti-affinity caps one Pod per Node
// absolutely, which would leave a Pod Pending after a Node is lost, while a
// maximum Skew of one keeps every Pod Running
// (docs/adr/2026-10-04-scenario-declared-pod-spread.md).
func TestADeployedPodTemplate_ShouldCarryNoPodAffinityRule(t *testing.T) {
	overZones := spreadRule(t, scenario.SpreadOverZones, evenSkew)

	deployed := toDeployment(aWorkload(t, overZones))

	expect.Expect(t, deployed.Spec.Template.Spec.Affinity).To(expect.Equal[*corev1.Affinity](nil))
}

func TestADeployedWorkload_ShouldLabelAndSelectItsPodsByItsName(t *testing.T) {
	deployed := toDeployment(aWorkload(t))

	expect.Expect(t, deployed.Name).To(expect.Equal(deployedWorkloadName))
	expect.Expect(t, int(*deployed.Spec.Replicas)).To(expect.Equal(somePodCount))
	expect.Expect(t, deployed.Spec.Selector).To(expect.Equal(workloadSelector()))
	expect.Expect(t, deployed.Spec.Template.Labels).To(expect.Equal(map[string]string{workloadLabel: deployedWorkloadName}))
}

func TestAProvisionedNode_ShouldBeFakeAndCarryItsZoneAndHostnameLabels(t *testing.T) {
	provisioned := aNode(t)

	mapped := toNode(provisioned)

	expect.Expect(t, mapped.Name).To(expect.Equal(provisioned.Name))
	expect.Expect(t, mapped.Labels).To(expect.Equal(map[string]string{
		zoneLabel:     provisioned.Zone,
		hostnameLabel: provisioned.Name,
		fakeNodeLabel: fakeNodeLabelValue,
	}))
	expect.Expect(t, mapped.Annotations).To(expect.Equal(map[string]string{fakeNodeAnnotation: fakeNodeAnnotationValue}))
}

func TestAProvisionedNode_ShouldBeReadyWithItsDeclaredPodCapacity(t *testing.T) {
	provisioned := aNode(t)

	mapped := toNode(provisioned)

	expect.Expect(t, int(mapped.Status.Capacity.Pods().Value())).To(expect.Equal(somePodsPerNode))
	expect.Expect(t, int(mapped.Status.Allocatable.Pods().Value())).To(expect.Equal(somePodsPerNode))
	expect.Expect(t, mapped.Status.Conditions).To(expect.Equal([]corev1.NodeCondition{
		{Type: corev1.NodeReady, Status: corev1.ConditionTrue, Reason: nodeReadyReason},
	}))
}

func TestAProbePod_ShouldNameItsOwnNodeByHostnameAndLeaveThePlacingToTheScheduler(t *testing.T) {
	probed := aNode(t)

	mapped := toProbePod(probed)

	expect.Expect(t, mapped.Name).To(expect.Equal(probePodNamePrefix + probed.Name))
	expect.Expect(t, mapped.Labels).To(expect.Equal(map[string]string{probeLabel: probeLabelValue}))
	expect.Expect(t, mapped.Spec.NodeSelector).To(expect.Equal(map[string]string{hostnameLabel: probed.Name}))
	expect.Expect(t, mapped.Spec.NodeName).To(expect.Equal(""))
}

func TestOnlyAProbePodReportedRunning_ShouldCountAsItsNodeBeingHeld(t *testing.T) {
	heldNode := aNode(t)
	const unplacedProbeName = probePodNamePrefix + "zone-b-node-1"

	holding := nodesHoldingRunningProbe([]corev1.Pod{
		{
			Name:   probePodNamePrefix + heldNode.Name,
			Spec:   corev1.PodSpec{NodeName: heldNode.Name},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			Name:   unplacedProbeName,
			Status: corev1.PodStatus{Phase: corev1.PodPending},
		},
	})

	expect.Expect(t, holding).To(expect.Equal([]string{heldNode.Name}))
}

func TestAnObservedPod_ShouldBeReadWithTheNodeItLandedOn(t *testing.T) {
	const placedPodName, pendingPodName = "web-abc", "web-def"
	placedOn := aNode(t)

	observed := fromPods([]corev1.Pod{
		{
			Name:   placedPodName,
			Spec:   corev1.PodSpec{NodeName: placedOn.Name},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			Name:   pendingPodName,
			Status: corev1.PodStatus{Phase: corev1.PodPending},
		},
	})

	expect.Expect(t, observed).To(expect.Equal([]scenario.Pod{
		{Name: placedPodName, Node: placedOn.Name, Phase: scenario.PodRunning},
		{Name: pendingPodName, Phase: scenario.PodPending},
	}))
}

func TestAPhaseTheLabDoesNotName_ShouldBeReadAsUnknown(t *testing.T) {
	const somePodName = "web-abc"

	observed := fromPods([]corev1.Pod{{
		Name:   somePodName,
		Status: corev1.PodStatus{Phase: corev1.PodPhase("Decommissioning")},
	}})

	expect.Expect(t, observed).To(expect.Equal([]scenario.Pod{
		{Name: somePodName, Phase: scenario.PodUnknown},
	}))
}

func TestAnObservedNode_ShouldTakeItsZoneFromItsLabelRatherThanItsName(t *testing.T) {
	const observedNodeName, labelledZone = "zone-a-node-1", "zone-c"

	observed, err := fromNodes([]corev1.Node{{
		Name:   observedNodeName,
		Labels: map[string]string{zoneLabel: labelledZone},
		Status: corev1.NodeStatus{Capacity: corev1.ResourceList{
			corev1.ResourcePods: *resource.NewQuantity(somePodsPerNode, resource.DecimalSI),
		}},
	}})
	if err != nil {
		t.Fatalf("read an observed Node: %v", err)
	}

	expect.Expect(t, observed).To(expect.Equal([]scenario.Node{
		{Name: observedNodeName, Zone: labelledZone, Capacity: aCapacity(t)},
	}))
}

func TestAnObservedNodeWithNoUsableCapacity_ShouldBeRefused(t *testing.T) {
	const observedNodeName = "zone-a-node-1"

	_, err := fromNodes([]corev1.Node{{
		Name: observedNodeName,
	}})

	if err == nil {
		t.Fatal("a Node reporting no Pod capacity is refused, but it was read")
	}
}

// aNode is the named default for the one Node a mapping test maps.
func aNode(t *testing.T) scenario.Node {
	t.Helper()

	const oneZone, oneNodePerZone = 1, 1
	topology, err := scenario.NewTopology(oneZone, oneNodePerZone, aCapacity(t))
	if err != nil {
		t.Fatalf("build a Topology of one Node: %v", err)
	}

	return topology.Nodes()[0]
}

// aCapacity is the named default for the Node capacity this suite provisions with.
func aCapacity(t *testing.T) scenario.NodeCapacity {
	t.Helper()

	capacity, err := scenario.NewNodeCapacity(somePodsPerNode)
	if err != nil {
		t.Fatalf("build a Node capacity of %d Pods: %v", somePodsPerNode, err)
	}

	return capacity
}
