package cluster

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

const (
	deployedWorkloadName = "web"
	somePodsPerNode      = 10
	somePodCount         = 9
	evenSkew             = 1
	widerSkew            = 2

	someCPUPerNode      = scenario.CPU(4 * millicoresPerCore)
	someMemoryPerNode   = scenario.Memory(16 * mebibytesPerGibibyte)
	someRequestedCPU    = scenario.CPU(1 * millicoresPerCore)
	someRequestedMemory = scenario.Memory(2 * mebibytesPerGibibyte)

	// internal/lab converts whole units to the lab's quantities, and this suite
	// converts the same way rather than importing internal/lab, which imports
	// this package.
	millicoresPerCore    = 1000
	mebibytesPerGibibyte = 1024

	// No test of this suite turns on the specification name.
	someSpecificationName = "a Pod specification"
)

func aWorkload(t *testing.T, declaring scenario.PodSpecification) scenario.Workload {
	t.Helper()

	workload, err := scenario.NewWorkload(deployedWorkloadName, somePodCount, declaring)
	if err != nil {
		t.Fatalf("build a Workload of %d Pods: %v", somePodCount, err)
	}

	return workload
}

// The predefined specifications live in the Scenarios' place, which imports
// internal/lab, which imports this package, so this suite builds its own.
func aSpecificationOf(t *testing.T, settings ...scenario.Setting) scenario.PodSpecification {
	t.Helper()

	declared, err := scenario.NewPodSpecification(someSpecificationName,
		aStartupTimeBetween(t, someShortestStartup, someLongestStartup), settings...)
	if err != nil {
		t.Fatalf("build the Pod specification %q: %v", someSpecificationName, err)
	}

	return declared
}

const (
	someShortestStartup = 500 * time.Millisecond
	someLongestStartup  = time.Second
)

func anEvenSpreadOver(topologyKey string, maxSkew int) scenario.TopologySpread {
	return scenario.NewTopologySpread(topologyKey, maxSkew, scenario.DoNotSchedule, scenario.Honor)
}

func aNodeConditionToleration(taint string) scenario.Toleration {
	return scenario.NewToleration(taint, scenario.Exists, "", scenario.NoExecute, theLabsWaitInSeconds())
}

func theLabsWaitInSeconds() int64 {
	return int64(scenario.UnresponsiveNodeWait.Seconds())
}

func workloadSelector() *metav1.LabelSelector {
	return &metav1.LabelSelector{MatchLabels: map[string]string{workloadLabel: deployedWorkloadName}}
}

// These are written in the client library's own vocabulary so each field is
// stated independently of the mapper that fills it.
func theConstraintOver(topologyKey string, maxSkew int) corev1.TopologySpreadConstraint {
	honourTaints := corev1.NodeInclusionPolicyHonor

	return corev1.TopologySpreadConstraint{
		MaxSkew:           int32(maxSkew),
		TopologyKey:       topologyKey,
		WhenUnsatisfiable: corev1.DoNotSchedule,
		LabelSelector:     workloadSelector(),
		NodeTaintsPolicy:  &honourTaints,
	}
}

func theTolerationOf(taint string) corev1.Toleration {
	leavingAfter := theLabsWaitInSeconds()

	return corev1.Toleration{
		Key:               taint,
		Operator:          corev1.TolerationOpExists,
		Effect:            corev1.TaintEffectNoExecute,
		TolerationSeconds: &leavingAfter,
	}
}

// A specification setting no spread still carries the lab's two tolerations,
// which every specification holds.
func TestAWorkloadDeclaringAPredefinedSpecification_ShouldBeDeployedWithTheSettingsItsDefinitionNames(t *testing.T) {
	declared := map[string]struct {
		specification scenario.PodSpecification
		constraints   []corev1.TopologySpreadConstraint
		tolerations   []corev1.Toleration
	}{
		"a specification spreading its Pods over the Zones and over the Nodes": {
			specification: aSpecificationOf(t,
				aNodeConditionToleration(scenario.TaintNodeNotReady),
				aNodeConditionToleration(scenario.TaintNodeUnreachable),
				anEvenSpreadOver(scenario.ZoneLabel, evenSkew),
				anEvenSpreadOver(scenario.HostnameLabel, widerSkew),
			),
			constraints: []corev1.TopologySpreadConstraint{
				theConstraintOver(corev1.LabelTopologyZone, evenSkew),
				theConstraintOver(corev1.LabelHostname, widerSkew),
			},
			tolerations: []corev1.Toleration{
				theTolerationOf(corev1.TaintNodeNotReady),
				theTolerationOf(corev1.TaintNodeUnreachable),
			},
		},
		"a specification setting no spread of its own": {
			specification: aSpecificationOf(t, aNodeConditionToleration(scenario.TaintNodeNotReady)),
			tolerations: []corev1.Toleration{
				theTolerationOf(corev1.TaintNodeNotReady),
				theTolerationOf(corev1.TaintNodeUnreachable),
			},
		},
	}

	for what, mapped := range declared {
		t.Run(what, func(t *testing.T) {
			deployed := toDeployment(aWorkload(t, mapped.specification))

			expect.Expect(t, deployed.Spec.Template.Spec.TopologySpreadConstraints).To(expect.Equal(mapped.constraints))
			expect.Expect(t, deployed.Spec.Template.Spec.Tolerations).To(expect.Equal(mapped.tolerations))
		})
	}
}

// beforeEverySetting is the place of a full access written first: no setting is
// transcribed before it runs.
const (
	theCriticalPriorityClass = "critical"
	beforeEverySetting       = 0
)

func namingThePriorityClass(priorityClass string) PodSpecAccess {
	return func(pod *corev1.PodSpec) { pod.PriorityClassName = priorityClass }
}

func droppingTheSpreadConstraints() PodSpecAccess {
	return func(pod *corev1.PodSpec) { pod.TopologySpreadConstraints = nil }
}

func droppingTheTolerations() PodSpecAccess {
	return func(pod *corev1.PodSpec) { pod.Tolerations = nil }
}

func leavingNothingOfThePodSpecification() PodSpecAccess {
	return func(pod *corev1.PodSpec) { *pod = corev1.PodSpec{} }
}

func afterEverySettingOf(declared scenario.PodSpecification) int {
	return len(declared.Settings())
}

func TestASpecificationCustomizedWithFullAccess_ShouldHoldTheSettingsTheScenarioNamed(t *testing.T) {
	declared := aSpecificationOf(t, aNodeConditionToleration(scenario.TaintNodeNotReady))

	deployed := toDeployment(aWorkload(t, declared), PodSpecAccessInPlace{
		Access:        namingThePriorityClass(theCriticalPriorityClass),
		AfterSettings: afterEverySettingOf(declared),
	})

	expect.Expect(t, deployed.Spec.Template.Spec.PriorityClassName).To(expect.Equal(theCriticalPriorityClass))
	expect.Expect(t, deployed.Spec.Template.Spec.Tolerations).
		To(expect.Equal([]corev1.Toleration{
			theTolerationOf(corev1.TaintNodeNotReady), theTolerationOf(corev1.TaintNodeUnreachable),
		}))
}

func TestFullAccessWrittenBesideTheLabsOwnCustomizations_ShouldWinWhereItIsWrittenLater(t *testing.T) {
	declared := aSpecificationOf(t, anEvenSpreadOver(scenario.ZoneLabel, evenSkew))

	written := map[string]struct {
		place       int
		constraints []corev1.TopologySpreadConstraint
	}{
		"full access written before the lab's own spread setting": {
			place:       beforeEverySetting,
			constraints: []corev1.TopologySpreadConstraint{theConstraintOver(corev1.LabelTopologyZone, evenSkew)},
		},
		"full access written after it": {
			place: afterEverySettingOf(declared),
		},
	}

	for order, taken := range written {
		t.Run(order, func(t *testing.T) {
			deployed := toDeployment(aWorkload(t, declared), PodSpecAccessInPlace{
				Access: droppingTheSpreadConstraints(), AfterSettings: taken.place,
			})

			expect.Expect(t, deployed.Spec.Template.Spec.TopologySpreadConstraints).To(expect.Equal(taken.constraints))
		})
	}
}

// The specification's tolerations are transcribed after the last full access,
// so the unresponsive-Node wait holds whatever full access left out.
func TestFullAccessLeavingOutASettingTheLabDependsOn_ShouldLeaveThatSettingHolding(t *testing.T) {
	declared := aSpecificationOf(t,
		aNodeConditionToleration(scenario.TaintNodeNotReady),
		aNodeConditionToleration(scenario.TaintNodeUnreachable),
	)

	deployed := toDeployment(aWorkload(t, declared), PodSpecAccessInPlace{
		Access: droppingTheTolerations(), AfterSettings: afterEverySettingOf(declared),
	})

	expect.Expect(t, deployed.Spec.Template.Spec.Tolerations).To(expect.Equal([]corev1.Toleration{
		theTolerationOf(corev1.TaintNodeNotReady),
		theTolerationOf(corev1.TaintNodeUnreachable),
	}))
}

func TestAWorkloadCustomizedWithFullAccess_ShouldStillHaveItsPodsCountedAsItsOwn(t *testing.T) {
	declared := aSpecificationOf(t, anEvenSpreadOver(scenario.ZoneLabel, evenSkew))

	deployed := toDeployment(aWorkload(t, declared), PodSpecAccessInPlace{
		Access: leavingNothingOfThePodSpecification(), AfterSettings: beforeEverySetting,
	})

	expect.Expect(t, deployed.Name).To(expect.Equal(deployedWorkloadName))
	expect.Expect(t, deployed.Labels).To(expect.Equal(map[string]string{workloadLabel: deployedWorkloadName}))
	expect.Expect(t, deployed.Spec.Selector).To(expect.Equal(workloadSelector()))
	expect.Expect(t, deployed.Spec.Template.Labels).To(expect.Equal(map[string]string{workloadLabel: deployedWorkloadName}))
}

// A mistyped label, taint or value would drop a spread or a toleration silently
// and produce a Placement nobody asked for.
func TestEveryKubernetesNameTheLabSpells_ShouldEqualTheClientLibrarysOwnConstant(t *testing.T) {
	expect.Expect(t, scenario.ZoneLabel).To(expect.Equal(corev1.LabelTopologyZone))
	expect.Expect(t, scenario.HostnameLabel).To(expect.Equal(corev1.LabelHostname))
	expect.Expect(t, scenario.TaintNodeNotReady).To(expect.Equal(corev1.TaintNodeNotReady))
	expect.Expect(t, scenario.TaintNodeUnreachable).To(expect.Equal(corev1.TaintNodeUnreachable))
	expect.Expect(t, string(scenario.DoNotSchedule)).To(expect.Equal(string(corev1.DoNotSchedule)))
	expect.Expect(t, string(scenario.Honor)).To(expect.Equal(string(corev1.NodeInclusionPolicyHonor)))
	expect.Expect(t, string(scenario.Exists)).To(expect.Equal(string(corev1.TolerationOpExists)))
	expect.Expect(t, string(scenario.NoExecute)).To(expect.Equal(string(corev1.TaintEffectNoExecute)))
	expect.Expect(t, string(scenario.Equal)).To(expect.Equal(string(corev1.TolerationOpEqual)))
	expect.Expect(t, string(scenario.NoSchedule)).To(expect.Equal(string(corev1.TaintEffectNoSchedule)))
}

// A required anti-affinity caps one Pod per Node absolutely, which would leave
// a Pod Pending after a Node is lost, while a maximum Skew of one keeps every
// Pod Running.
func TestADeployedPodTemplate_ShouldCarryNoPodAffinityRule(t *testing.T) {
	deployed := toDeployment(aWorkload(t, aSpecificationOf(t, anEvenSpreadOver(scenario.ZoneLabel, evenSkew))))

	expect.Expect(t, deployed.Spec.Template.Spec.Affinity).To(expect.Equal[*corev1.Affinity](nil))
}

// A specification is a package-level value and cannot name the container.
func TestADeployedPodTemplate_ShouldCarryTheContainerNoSpecificationCanName(t *testing.T) {
	deployed := toDeployment(aWorkload(t, aSpecificationOf(t, anEvenSpreadOver(scenario.ZoneLabel, evenSkew))))

	expect.Expect(t, deployed.Spec.Template.Spec.Containers).To(expect.Equal([]corev1.Container{
		{Name: workloadContainer, Image: workloadImage},
	}))
}

func aReportedStatus(conditions ...corev1.NodeCondition) corev1.NodeStatus {
	return corev1.NodeStatus{Capacity: aReportedCapacity(), Conditions: conditions}
}

func aReportedCapacity() corev1.ResourceList {
	return corev1.ResourceList{
		corev1.ResourcePods:   *resource.NewQuantity(somePodsPerNode, resource.DecimalSI),
		corev1.ResourceCPU:    *resource.NewMilliQuantity(int64(someCPUPerNode), resource.DecimalSI),
		corev1.ResourceMemory: *resource.NewQuantity(int64(someMemoryPerNode)*bytesPerMebibyte, resource.BinarySI),
	}
}

func theReadyCondition() corev1.NodeCondition {
	return corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionTrue, Reason: nodeReadyReason}
}

func someOtherCondition() corev1.NodeCondition {
	return corev1.NodeCondition{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse}
}

func theLostHeartbeatCondition() corev1.NodeCondition {
	return corev1.NodeCondition{
		Type:    corev1.NodeReady,
		Status:  corev1.ConditionUnknown,
		Reason:  nodeLostHeartbeatReason,
		Message: nodeLostHeartbeatMessage,
	}
}

// The lab writes a changed copy back and reads the same Node again on the next
// Failure, so each case also asserts the status it read is left as it was.
func TestTheStatusOfANodeThatStoppedResponding_ShouldReportReadyUnknownAndLeaveEveryOtherConditionAsReported(t *testing.T) {
	nodes := map[string]struct {
		reported, withheld []corev1.NodeCondition
	}{
		"a Node reporting Ready beside another condition": {
			reported: []corev1.NodeCondition{theReadyCondition(), someOtherCondition()},
			withheld: []corev1.NodeCondition{someOtherCondition(), theLostHeartbeatCondition()},
		},
		"a Node reporting its Ready condition alone": {
			reported: []corev1.NodeCondition{theReadyCondition()},
			withheld: []corev1.NodeCondition{theLostHeartbeatCondition()},
		},
		"a Node reporting no condition at all": {
			reported: nil,
			withheld: []corev1.NodeCondition{theLostHeartbeatCondition()},
		},
	}

	for node, status := range nodes {
		t.Run(node, func(t *testing.T) {
			reported := aReportedStatus(status.reported...)

			withheld := toUnresponsive(reported)

			expect.Expect(t, withheld).To(expect.Equal(aReportedStatus(status.withheld...)))
			expect.Expect(t, reported).To(expect.Equal(aReportedStatus(status.reported...)))
		})
	}
}

// The lab writes a changed copy back, so the labels it read keep the marking.
func TestANodeTheKWOKControllerNoLongerMaintains_ShouldKeepEveryLabelButTheMarkingItSelectsBy(t *testing.T) {
	provisioned := aNode(t)
	labelled := toNode(provisioned).Labels

	unmarked := withoutTheKWOKMarking(labelled)

	expect.Expect(t, unmarked).To(expect.Equal(map[string]string{
		zoneLabel:     provisioned.Zone,
		hostnameLabel: provisioned.Name,
		nodePoolLabel: provisioned.NodePool,
	}))
	expect.Expect(t, labelled[fakeNodeLabel]).To(expect.Equal(fakeNodeLabelValue))
}

func TestADeployedWorkload_ShouldLabelAndSelectItsPodsByItsName(t *testing.T) {
	deployed := toDeployment(aWorkload(t, aSpecificationOf(t, anEvenSpreadOver(scenario.ZoneLabel, evenSkew))))

	expect.Expect(t, deployed.Name).To(expect.Equal(deployedWorkloadName))
	expect.Expect(t, int(*deployed.Spec.Replicas)).To(expect.Equal(somePodCount))
	expect.Expect(t, deployed.Spec.Selector).To(expect.Equal(workloadSelector()))
	expect.Expect(t, deployed.Spec.Template.Labels).To(expect.Equal(map[string]string{workloadLabel: deployedWorkloadName}))
}

func TestAProvisionedNode_ShouldBeFakeAndCarryItsZoneAndHostnameLabels(t *testing.T) {
	provisioned := aNode(t)

	mapped := toNode(provisioned)

	expect.Expect(t, mapped.Name).To(expect.Equal(provisioned.Name))
	expect.Expect(t, mapped.Labels[zoneLabel]).To(expect.Equal(provisioned.Zone))
	expect.Expect(t, mapped.Labels[hostnameLabel]).To(expect.Equal(provisioned.Name))
	expect.Expect(t, mapped.Labels[fakeNodeLabel]).To(expect.Equal(fakeNodeLabelValue))
	expect.Expect(t, mapped.Annotations).To(expect.Equal(map[string]string{fakeNodeAnnotation: fakeNodeAnnotationValue}))
}

// The scheduler fits requests against what a Node reports allocatable.
func TestATopologyStatingItsNodesCpuMemoryAndPodCount_ShouldProvisionNodesWithThatCapacity(t *testing.T) {
	provisioned := aNode(t)

	mapped := toNode(provisioned)

	expect.Expect(t, theCapacityReportedIn(t, mapped.Status.Capacity)).To(expect.Equal(provisioned.Capacity))
	expect.Expect(t, theCapacityReportedIn(t, mapped.Status.Allocatable)).To(expect.Equal(provisioned.Capacity))
	expect.Expect(t, mapped.Status.Conditions).To(expect.Equal([]corev1.NodeCondition{
		{Type: corev1.NodeReady, Status: corev1.ConditionTrue, Reason: nodeReadyReason},
	}))
}

func theCapacityReportedIn(t *testing.T, reported corev1.ResourceList) scenario.NodeCapacity {
	t.Helper()

	return aCapacityOf(t,
		int(reported.Pods().Value()),
		scenario.CPU(reported.Cpu().MilliValue()),
		scenario.Memory(reported.Memory().Value()/bytesPerMebibyte))
}

// A capacity is read off a Step line the lab spells and off a Timeline the
// client library spells, so one amount stated two ways would read as two.
func TestEveryQuantityTheLabSpells_ShouldEqualTheClientLibrarysOwnSpelling(t *testing.T) {
	stated := map[string]struct {
		cores, gibibytes int
	}{
		"a Node of one core and one gibibyte":              {cores: 1, gibibytes: 1},
		"a Node of four cores and sixteen gibibytes":       {cores: 4, gibibytes: 16},
		"a Node of sixteen cores and a tebibyte of memory": {cores: 16, gibibytes: 1024},
	}

	for capacity, held := range stated {
		t.Run(capacity, func(t *testing.T) {
			cpu := scenario.CPU(held.cores * millicoresPerCore)
			memory := scenario.Memory(held.gibibytes * mebibytesPerGibibyte)

			mapped := toNode(aNodeHolding(t, aCapacityOf(t, somePodsPerNode, cpu, memory)))

			expect.Expect(t, mapped.Status.Capacity.Cpu().String()).To(expect.Equal(cpu.String()))
			expect.Expect(t, mapped.Status.Capacity.Memory().String()).To(expect.Equal(memory.String()))
		})
	}
}

// The scheduler places a Pod by what it asks for.
func TestAPodSpecificationStatingARequest_ShouldDeployItsPodsWithThatRequest(t *testing.T) {
	declared := aSpecificationOf(t, scenario.NewRequest(someRequestedCPU, someRequestedMemory))

	deployed := toDeployment(aWorkload(t, declared))

	expect.Expect(t, theQuantitiesRequestedIn(deployed)).To(expect.Equal([]map[corev1.ResourceName]string{{
		corev1.ResourceCPU:    someRequestedCPU.String(),
		corev1.ResourceMemory: someRequestedMemory.String(),
	}}))
}

func TestAPodSpecificationStatingNoRequest_ShouldDeployItsPodsAskingForNothing(t *testing.T) {
	declared := aSpecificationOf(t, anEvenSpreadOver(scenario.ZoneLabel, evenSkew))

	deployed := toDeployment(aWorkload(t, declared))

	expect.Expect(t, theQuantitiesRequestedIn(deployed)).To(expect.Equal([]map[corev1.ResourceName]string{{}}))
}

func theQuantitiesRequestedIn(deployed *appsv1.Deployment) []map[corev1.ResourceName]string {
	containers := deployed.Spec.Template.Spec.Containers
	requested := make([]map[corev1.ResourceName]string, 0, len(containers))
	for _, container := range containers {
		spelled := make(map[corev1.ResourceName]string, len(container.Resources.Requests))
		for name, quantity := range container.Resources.Requests {
			spelled[name] = quantity.String()
		}
		requested = append(requested, spelled)
	}

	return requested
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
	const unplacedProbeName = probePodNamePrefix + "zone-2-node-1"

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

// The Workload comes from the Pod's label, so a Workload name holding a hyphen
// is never cut at one.
func TestAnObservedPod_ShouldBeReadWithItsWorkloadAndTheNodeItLandedOn(t *testing.T) {
	const placedPodName, pendingPodName, aHyphenatedWorkload = "web-abc", "web-def", "web-frontend"
	placedOn := aNode(t)

	observed := fromPods([]corev1.Pod{
		{
			Name:   placedPodName,
			Labels: map[string]string{workloadLabel: aHyphenatedWorkload},
			Spec:   corev1.PodSpec{NodeName: placedOn.Name},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			Name:   pendingPodName,
			Labels: map[string]string{workloadLabel: aHyphenatedWorkload},
			Status: corev1.PodStatus{Phase: corev1.PodPending},
		},
	})

	expect.Expect(t, observed).To(expect.Equal([]scenario.Pod{
		{Name: placedPodName, Workload: aHyphenatedWorkload, Node: placedOn.Name, Phase: scenario.PodRunning},
		{Name: pendingPodName, Workload: aHyphenatedWorkload, Phase: scenario.PodPending},
	}))
}

// A Pod reporting no Ready condition is not Ready, so a Wait holds for it
// rather than passing it over.
func TestAnObservedPodsReadyCondition_ShouldBeReadAsThatPodBeingReady(t *testing.T) {
	const somePodName = "web-abc"
	reported := map[string]struct {
		conditions []corev1.PodCondition
		ready      bool
	}{
		"the Ready condition true": {
			conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
			ready:      true,
		},
		"the Ready condition false": {
			conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse}},
			ready:      false,
		},
		"no condition at all": {
			conditions: nil,
			ready:      false,
		},
		"another condition alone": {
			conditions: []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionTrue}},
			ready:      false,
		},
	}

	for condition, one := range reported {
		t.Run(condition, func(t *testing.T) {
			observed := fromPods([]corev1.Pod{{
				Name:   somePodName,
				Status: corev1.PodStatus{Phase: corev1.PodRunning, Conditions: one.conditions},
			}})

			expect.Expect(t, observed).To(expect.Equal([]scenario.Pod{
				{Name: somePodName, Phase: scenario.PodRunning, Ready: one.ready},
			}))
		})
	}
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
	const observedNodeName, labelledZone = "zone-1-node-1", "zone-3"

	observed, err := fromNodes([]corev1.Node{{
		Name:   observedNodeName,
		Labels: map[string]string{zoneLabel: labelledZone},
		Status: corev1.NodeStatus{Capacity: aReportedCapacity()},
	}})
	if err != nil {
		t.Fatalf("read an observed Node: %v", err)
	}

	expect.Expect(t, observed).To(expect.Equal([]scenario.Node{
		{Name: observedNodeName, Zone: labelledZone, Capacity: aCapacity(t)},
	}))
}

func TestEveryNodeOfANodePool_ShouldCarryALabelNamingItsNodePool(t *testing.T) {
	const gpu = "gpu"
	provisioned := aNodeOfTheNodePool(t, gpu)

	mapped := toNode(provisioned)

	expect.Expect(t, mapped.Labels[nodePoolLabel]).To(expect.Equal(gpu))
}

func TestAnObservedNode_ShouldBeReadWithThatNodePool(t *testing.T) {
	const gpu = "gpu"
	declared := aNodeOfTheNodePool(t, gpu)

	observed, err := fromNodes([]corev1.Node{*toNode(declared)})
	if err != nil {
		t.Fatalf("read an observed Node: %v", err)
	}

	expect.Expect(t, observed).To(expect.Equal([]scenario.Node{declared}))
}

// The domain names no Kubernetes package and holds the label syntax itself, so
// it is held to the client library's own validation.
func TestTheNodePoolNameRule_ShouldAgreeWithTheClientLibrarysLabelValueValidation(t *testing.T) {
	const someNodesPerZone = 1

	for _, name := range someLabelValues() {
		t.Run(name, func(t *testing.T) {
			_, refusal := scenario.NewNodePool(name, someNodesPerZone, aCapacity(t))

			expect.Expect(t, refusal == nil).To(expect.Equal(name != "" && len(validation.IsValidLabelValue(name)) == 0))
		})
	}
}

func TestTheNodeLabelValueRule_ShouldAgreeWithTheClientLibrarysLabelValueValidation(t *testing.T) {
	const someKey = "some-key"

	for _, value := range someLabelValues() {
		t.Run(value, func(t *testing.T) {
			_, refusal := scenario.NewNodeLabels(map[string]string{someKey: value})

			expect.Expect(t, refusal == nil).To(expect.Equal(len(validation.IsValidLabelValue(value)) == 0))
		})
	}
}

func TestTheNodeLabelKeyRule_ShouldAgreeWithTheClientLibrarysQualifiedNameValidation(t *testing.T) {
	const someValue = "some-value"
	prefixes := []string{"", "example.com/", "a/", strings.Repeat("p", 253) + "/", strings.Repeat("p", 254) + "/",
		"Example.com/", "-example.com/", "example..com/", "/", "a/b/"}

	for _, prefix := range prefixes {
		for _, name := range someLabelValues() {
			key := prefix + name
			t.Run(key, func(t *testing.T) {
				_, refusal := scenario.NewNodeLabels(map[string]string{key: someValue})

				expect.Expect(t, refusal == nil).To(expect.Equal(len(validation.IsQualifiedName(key)) == 0))
			})
		}
	}
}

func someLabelValues() []string {
	return []string{
		"gpu", "g", "GPU-a100", "a.b_c-d", "0pool9", strings.Repeat("a", validation.LabelValueMaxLength), "",
		strings.Repeat("a", validation.LabelValueMaxLength+1),
		"-gpu", "gpu-", ".gpu", "gpu_", "gpu nodes", "gpu/a100", "gpü",
	}
}

// A stated label can then neither overwrite one of the lab's nor be overwritten
// by one the domain let through.
func TestTheLabsOwnNodeLabelKeys_ShouldBeExactlyTheOnesTheMapperWritesOnANodeStatingNone(t *testing.T) {
	mapped := toNode(aNode(t))

	expect.Expect(t, slices.Sorted(maps.Keys(mapped.Labels))).To(expect.Equal(slices.Sorted(slices.Values(scenario.LabsOwnNodeLabelKeys()))))
}

func TestANodeStatingLabels_ShouldBeMappedCarryingThemBesideTheLabsOwn(t *testing.T) {
	labelled := aNodeLabelled(t, map[string]string{capacityTypeLabel: spotCapacity})

	mapped := toNode(labelled)

	expect.Expect(t, mapped.Labels).To(expect.Equal(map[string]string{
		capacityTypeLabel: spotCapacity,
		zoneLabel:         labelled.Zone,
		hostnameLabel:     labelled.Name,
		nodePoolLabel:     labelled.NodePool,
		fakeNodeLabel:     fakeNodeLabelValue,
	}))
}

func TestAnObservedLabelledNode_ShouldBeReadWithTheLabelsItsNodePoolStated(t *testing.T) {
	declared := aNodeLabelled(t, map[string]string{capacityTypeLabel: spotCapacity})

	observed, err := fromNodes([]corev1.Node{*toNode(declared)})
	if err != nil {
		t.Fatalf("read an observed Node: %v", err)
	}

	expect.Expect(t, observed).To(expect.Equal([]scenario.Node{declared}))
}

const (
	capacityTypeLabel = "capacity-type"
	spotCapacity      = "spot"
)

func aNodeLabelled(t *testing.T, stated map[string]string) scenario.Node {
	t.Helper()

	const oneZone, oneNodePerZone = 1, 1
	labels, err := scenario.NewNodeLabels(stated)
	if err != nil {
		t.Fatalf("build the Node labels %v: %v", stated, err)
	}
	pool, err := scenario.NewNodePool("general", oneNodePerZone, aCapacity(t))
	if err != nil {
		t.Fatalf("build a NodePool: %v", err)
	}
	labelled, err := pool.Labelled(labels)
	if err != nil {
		t.Fatalf("label the NodePool %v: %v", stated, err)
	}
	topology, err := scenario.NewTopology(oneZone, labelled)
	if err != nil {
		t.Fatalf("build a Topology of a labelled NodePool: %v", err)
	}

	return topology.Nodes()[0]
}

func TestAnObservedNodeWithNoUsableCapacity_ShouldBeRefused(t *testing.T) {
	const observedNodeName = "zone-1-node-1"

	_, err := fromNodes([]corev1.Node{{
		Name: observedNodeName,
	}})

	if err == nil {
		t.Fatal("a Node reporting no Pod capacity is refused, but it was read")
	}
}

const (
	observedAfter     = 2500 * time.Millisecond
	watchedNodeName   = "zone-2-node-2"
	watchedPodName    = "web-5d8f9b7c6-2xk4m"
	watchedReplicaSet = "web-5d8f9b7c6"
	replicasRunning   = 3
	scheduledReason   = "Scheduled"
	scheduledMessage  = "Successfully assigned default/web-5d8f9b7c6-2xk4m to zone-2-node-2"
)

const (
	reportedPodKind   = "Pod"
	someUnwatchedKind = "Service"
	someUnwatchedName = "web-service"
)

func theCapacityReported() string {
	return someCPUPerNode.String() + " cpu, " + someMemoryPerNode.String() + " memory and " +
		strconv.Itoa(somePodsPerNode) + " Pods"
}

func aWatchedNode(ready corev1.ConditionStatus) *corev1.Node {
	return &corev1.Node{
		Name: watchedNodeName,
		Status: corev1.NodeStatus{
			Capacity:   aReportedCapacity(),
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: ready}},
		},
	}
}

// Only that a deletion time is set matters, not which.
func markedForDeletion() *metav1.Time {
	return &metav1.Time{}
}

func TestAnObservedChangeToAKindATimelineReads_ShouldBecomeOneEntryOfThatChange(t *testing.T) {
	declaredReplicas := int32(somePodCount)

	changes := map[string]struct {
		observed watch.Event
		recorded scenario.Entry
	}{
		"a Node the control plane reports Ready": {
			observed: watch.Event{Type: watch.Added, Object: aWatchedNode(corev1.ConditionTrue)},
			recorded: scenario.Entry{
				Elapsed: observedAfter, Change: scenario.ChangeAdded, Kind: scenario.NodeObject,
				Name: watchedNodeName, Reason: string(scenario.NodeReady),
				Detail: theCapacityReported(),
			},
		},
		"a Node the control plane no longer reports Ready": {
			observed: watch.Event{Type: watch.Modified, Object: aWatchedNode(corev1.ConditionFalse)},
			recorded: scenario.Entry{
				Elapsed: observedAfter, Change: scenario.ChangeModified, Kind: scenario.NodeObject,
				Name: watchedNodeName, Reason: string(scenario.NodeNotReady),
				Detail: theCapacityReported(),
			},
		},
		"a Pod the scheduler placed on a Node": {
			observed: watch.Event{Type: watch.Modified, Object: &corev1.Pod{
				Name:   watchedPodName,
				Spec:   corev1.PodSpec{NodeName: watchedNodeName},
				Status: corev1.PodStatus{Phase: corev1.PodRunning},
			}},
			recorded: scenario.Entry{
				Elapsed: observedAfter, Change: scenario.ChangeModified, Kind: scenario.PodObject,
				Name: watchedPodName, Reason: string(scenario.PodRunning), Detail: watchedNodeName,
			},
		},
		"a Pod marked for deletion on the Node it stood on": {
			observed: watch.Event{Type: watch.Modified, Object: &corev1.Pod{
				Name: watchedPodName, DeletionTimestamp: markedForDeletion(),
				Spec:   corev1.PodSpec{NodeName: watchedNodeName},
				Status: corev1.PodStatus{Phase: corev1.PodRunning},
			}},
			recorded: scenario.Entry{
				Elapsed: observedAfter, Change: scenario.ChangeModified, Kind: scenario.PodObject,
				Name: watchedPodName, Reason: string(scenario.PodTerminating), Detail: watchedNodeName,
			},
		},
		"a Pod the cluster no longer holds": {
			observed: watch.Event{Type: watch.Deleted, Object: &corev1.Pod{
				Name:   watchedPodName,
				Status: corev1.PodStatus{Phase: corev1.PodRunning},
			}},
			recorded: scenario.Entry{
				Elapsed: observedAfter, Change: scenario.ChangeRemoved, Kind: scenario.PodObject,
				Name: watchedPodName, Reason: string(scenario.PodRunning),
			},
		},
		"a Deployment declaring how many replicas it wants": {
			observed: watch.Event{Type: watch.Modified, Object: &appsv1.Deployment{
				Name:   deployedWorkloadName,
				Spec:   appsv1.DeploymentSpec{Replicas: &declaredReplicas},
				Status: appsv1.DeploymentStatus{ReadyReplicas: replicasRunning},
			}},
			recorded: scenario.Entry{
				Elapsed: observedAfter, Change: scenario.ChangeModified, Kind: scenario.DeploymentObject,
				Name:   deployedWorkloadName,
				Detail: strconv.Itoa(replicasRunning) + " of " + strconv.Itoa(somePodCount) + " Pods ready",
			},
		},
		"a ReplicaSet declaring no replica count": {
			observed: watch.Event{Type: watch.Added, Object: &appsv1.ReplicaSet{
				Name:   watchedReplicaSet,
				Status: appsv1.ReplicaSetStatus{ReadyReplicas: replicasRunning},
			}},
			recorded: scenario.Entry{
				Elapsed: observedAfter, Change: scenario.ChangeAdded, Kind: scenario.ReplicaSetObject,
				Name: watchedReplicaSet, Detail: strconv.Itoa(replicasRunning) + " Pods ready",
			},
		},
		"an Event the control plane reported about a Pod it placed": {
			observed: watch.Event{Type: watch.Added, Object: &corev1.Event{
				InvolvedObject: corev1.ObjectReference{Kind: reportedPodKind, Name: watchedPodName},
				Reason:         scheduledReason,
				Message:        scheduledMessage,
			}},
			recorded: scenario.Entry{
				Elapsed: observedAfter, Change: scenario.ChangeReported, Kind: scenario.PodObject,
				Name: watchedPodName, Reason: scheduledReason, Detail: scheduledMessage,
			},
		},
	}

	for change, observed := range changes {
		t.Run(change, func(t *testing.T) {
			entry, recorded := toEntry(observedAfter, observed.observed)

			expect.Expect(t, recorded).To(expect.Equal(true))
			expect.Expect(t, entry).To(expect.Equal(observed.recorded))
		})
	}
}

func TestAnObservedChangeNoTimelineRecords_ShouldBecomeNoEntry(t *testing.T) {
	changes := map[string]watch.Event{
		"a change to an object of a kind no Timeline records": {
			Type:   watch.Added,
			Object: &corev1.Service{Name: someUnwatchedName},
		},
		"an Event reported about a kind no Timeline records": {
			Type: watch.Added,
			Object: &corev1.Event{
				InvolvedObject: corev1.ObjectReference{Kind: someUnwatchedKind, Name: someUnwatchedName},
				Reason:         scheduledReason,
				Message:        scheduledMessage,
			},
		},
		"a bookmark the stream sends about no change": {
			Type:   watch.Bookmark,
			Object: &corev1.Pod{Name: watchedPodName},
		},
	}

	for change, observed := range changes {
		t.Run(change, func(t *testing.T) {
			entry, recorded := toEntry(observedAfter, observed)

			expect.Expect(t, recorded).To(expect.Equal(false))
			expect.Expect(t, entry).To(expect.Equal(scenario.Entry{}))
		})
	}
}

func aNode(t *testing.T) scenario.Node {
	t.Helper()

	return aNodeHolding(t, aCapacity(t))
}

func aNodeHolding(t *testing.T, capacity scenario.NodeCapacity) scenario.Node {
	t.Helper()

	const oneZone, oneNodePerZone = 1, 1
	topology, err := topologyOfTheGeneralNodePool(oneZone, oneNodePerZone, capacity)
	if err != nil {
		t.Fatalf("build a Topology of one Node: %v", err)
	}

	return topology.Nodes()[0]
}

func topologyOfTheGeneralNodePool(zones, nodesPerZone int, capacity scenario.NodeCapacity) (scenario.Topology, error) {
	pool, err := scenario.NewNodePool("general", nodesPerZone, capacity)
	if err != nil {
		return scenario.Topology{}, err
	}

	return scenario.NewTopology(zones, pool)
}

func aNodeOfTheNodePool(t *testing.T, name string) scenario.Node {
	t.Helper()

	const oneZone, oneNodePerZone = 1, 1
	pool, err := scenario.NewNodePool(name, oneNodePerZone, aCapacity(t))
	if err != nil {
		t.Fatalf("build a NodePool %q: %v", name, err)
	}
	topology, err := scenario.NewTopology(oneZone, pool)
	if err != nil {
		t.Fatalf("build a Topology of the NodePool %q: %v", name, err)
	}

	return topology.Nodes()[0]
}

func aCapacity(t *testing.T) scenario.NodeCapacity {
	t.Helper()

	return aCapacityOf(t, somePodsPerNode, someCPUPerNode, someMemoryPerNode)
}

func aCapacityOf(t *testing.T, pods int, cpu scenario.CPU, memory scenario.Memory) scenario.NodeCapacity {
	t.Helper()

	capacity, err := scenario.NewNodeCapacity(pods, cpu, memory)
	if err != nil {
		t.Fatalf("build a Node capacity of %d Pods, %s cpu and %s memory: %v", pods, cpu, memory, err)
	}

	return capacity
}

// The times differ from each other and from this suite's default, so what a
// template carries is read back as the one stated.
const (
	aShortStartupMillis  = 300
	aLongerStartupMillis = 1700
)

func aStartupTimeBetween(t *testing.T, shortest, longest time.Duration) scenario.StartupTime {
	t.Helper()

	stated, err := scenario.NewStartupTime(int(shortest.Milliseconds()), int(longest.Milliseconds()))
	if err != nil {
		t.Fatalf("build a Startup time between %s and %s: %v", shortest, longest, err)
	}

	return stated
}

func theStartupAnnotations(shortest, longest time.Duration) map[string]string {
	return map[string]string{
		podReadyDelayAnnotation:       shortest.String(),
		podReadyJitterDelayAnnotation: longest.String(),
	}
}

// The annotations sit on the template's metadata, which full access does not
// reach.
func TestAWorkloadsPodTemplate_ShouldCarryItsStartupTimeWhateverFullAccessNames(t *testing.T) {
	namingSome := aSpecificationOf(t, anEvenSpreadOver(scenario.ZoneLabel, evenSkew))
	statingARange := namingSome.WithStartupTime(aStartupTimeBetween(t, aShortStartupMillis*time.Millisecond, aLongerStartupMillis*time.Millisecond))

	deployed := map[string]struct {
		template *appsv1.Deployment
		carries  map[string]string
	}{
		"the Startup time its definition names": {
			template: toDeployment(aWorkload(t, namingSome)),
			carries:  theStartupAnnotations(someShortestStartup, someLongestStartup),
		},
		"a shortest and a longest stated after": {
			template: toDeployment(aWorkload(t, statingARange)),
			carries:  theStartupAnnotations(aShortStartupMillis*time.Millisecond, aLongerStartupMillis*time.Millisecond),
		},
		"a full access leaving nothing of the Pod specification": {
			template: toDeployment(aWorkload(t, statingARange), PodSpecAccessInPlace{
				Access: leavingNothingOfThePodSpecification(), AfterSettings: beforeEverySetting,
			}),
			carries: theStartupAnnotations(aShortStartupMillis*time.Millisecond, aLongerStartupMillis*time.Millisecond),
		},
	}

	for how, taken := range deployed {
		t.Run(how, func(t *testing.T) {
			expect.Expect(t, taken.template.Spec.Template.Annotations).To(expect.Equal(taken.carries))
		})
	}
}

const (
	gpuTaintKey   = "nvidia.com/gpu"
	gpuTaintValue = "present"
)

func aGPUTaint() scenario.Taint {
	return scenario.Taint{Key: gpuTaintKey, Value: gpuTaintValue, Effect: scenario.NoSchedule}
}

func aNodeTainted(t *testing.T, stated ...scenario.Taint) scenario.Node {
	t.Helper()

	const oneZone, oneNodePerZone = 1, 1
	taints, err := scenario.NewNodeTaints(stated...)
	if err != nil {
		t.Fatalf("build the Node taints %v: %v", stated, err)
	}
	pool, err := scenario.NewNodePool("gpu", oneNodePerZone, aCapacity(t))
	if err != nil {
		t.Fatalf("build a NodePool: %v", err)
	}
	tainted, err := pool.Tainted(taints)
	if err != nil {
		t.Fatalf("taint the NodePool %v: %v", stated, err)
	}
	topology, err := scenario.NewTopology(oneZone, tainted)
	if err != nil {
		t.Fatalf("build a Topology of a tainted NodePool: %v", err)
	}

	return topology.Nodes()[0]
}

func TestANodeStatingTaints_ShouldBeMappedCarryingThemAsItsTaints(t *testing.T) {
	expect.Expect(t, toNode(aNodeTainted(t, aGPUTaint())).Spec.Taints).To(expect.Equal([]corev1.Taint{
		{Key: gpuTaintKey, Value: gpuTaintValue, Effect: corev1.TaintEffectNoSchedule},
	}))
	expect.Expect(t, toNode(aNode(t)).Spec.Taints).To(expect.HaveLen[corev1.Taint](0))
}

func TestAnObservedTaintedNode_ShouldBeReadWithTheTaintsItsNodePoolStated(t *testing.T) {
	declared := aNodeTainted(t, aGPUTaint())
	reported := toNode(declared)
	reported.Spec.Taints = append(reported.Spec.Taints,
		corev1.Taint{Key: corev1.TaintNodeNotReady, Effect: corev1.TaintEffectNoSchedule})

	observed, err := fromNodes([]corev1.Node{*reported})
	if err != nil {
		t.Fatalf("read an observed Node: %v", err)
	}

	expect.Expect(t, observed).To(expect.Equal([]scenario.Node{declared}))
}

func TestAProbePodOfATaintedNode_ShouldTolerateThatNodesTaints(t *testing.T) {
	expect.Expect(t, toProbePod(aNodeTainted(t, aGPUTaint())).Spec.Tolerations).To(expect.Equal([]corev1.Toleration{
		{Key: gpuTaintKey, Operator: corev1.TolerationOpEqual, Value: gpuTaintValue, Effect: corev1.TaintEffectNoSchedule},
	}))
	expect.Expect(t, toProbePod(aNode(t)).Spec.Tolerations).To(expect.HaveLen[corev1.Toleration](0))
}

// Kubernetes takes a waiting time for NoExecute alone.
func TestAPodSpecificationToleratingANoScheduleTaint_ShouldDeployItsPodsToleratingItWithNoWaitingTime(t *testing.T) {
	const noWaitingTime = 0
	tolerating := scenario.NewToleration(gpuTaintKey, scenario.Equal, gpuTaintValue, scenario.NoSchedule, noWaitingTime)

	deployed := toDeployment(aWorkload(t, aSpecificationOf(t, tolerating)))

	expect.Expect(t, deployed.Spec.Template.Spec.Tolerations).To(expect.Equal([]corev1.Toleration{
		{Key: gpuTaintKey, Operator: corev1.TolerationOpEqual, Value: gpuTaintValue, Effect: corev1.TaintEffectNoSchedule},
		theTolerationOf(corev1.TaintNodeNotReady),
		theTolerationOf(corev1.TaintNodeUnreachable),
	}))
}

// The lab additionally refuses keys under the control plane's prefix.
func TestTheTaintKeyAndValueRules_ShouldAgreeWithTheClientLibrarysValidation(t *testing.T) {
	prefixes := []string{"", "example.com/", strings.Repeat("p", 254) + "/", "Example.com/", "a/b/"}

	for _, prefix := range prefixes {
		for _, name := range someLabelValues() {
			key := prefix + name
			t.Run("key "+key, func(t *testing.T) {
				_, refusal := scenario.NewNodeTaints(scenario.Taint{Key: key, Value: gpuTaintValue, Effect: scenario.NoSchedule})

				expect.Expect(t, refusal == nil).To(expect.Equal(len(validation.IsQualifiedName(key)) == 0))
			})
		}
	}
	for _, value := range someLabelValues() {
		t.Run("value "+value, func(t *testing.T) {
			_, refusal := scenario.NewNodeTaints(scenario.Taint{Key: gpuTaintKey, Value: value, Effect: scenario.NoSchedule})

			expect.Expect(t, refusal == nil).To(expect.Equal(value != "" && len(validation.IsValidLabelValue(value)) == 0))
		})
	}
}
