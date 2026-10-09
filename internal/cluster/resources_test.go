package cluster

import (
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

	// The cpu and memory the Nodes of this suite hold, and the cpu and memory one
	// Pod of it asks for, in the millicores and mebibytes the lab's own quantities
	// hold: four cores and sixteen gibibytes a Node, one core and two gibibytes a
	// Pod.
	someCPUPerNode      = scenario.CPU(4 * millicoresPerCore)
	someMemoryPerNode   = scenario.Memory(16 * mebibytesPerGibibyte)
	someRequestedCPU    = scenario.CPU(1 * millicoresPerCore)
	someRequestedMemory = scenario.Memory(2 * mebibytesPerGibibyte)

	// The whole units a capacity and a request are stated in on the Step surface,
	// as the lab's own quantities hold them. internal/lab states them in whole
	// units and converts there, and this suite converts the same way rather than
	// importing the Step surface, which imports this package.
	millicoresPerCore    = 1000
	mebibytesPerGibibyte = 1024

	// someSpecificationName is the name the specifications below carry into the
	// Step that declares one, which no test of this suite turns on.
	someSpecificationName = "a Pod specification"
)

// aWorkload is the named default for a Workload whose name and Pod count the test
// does not turn on; the specification it declares is what each test varies.
func aWorkload(t *testing.T, declaring scenario.PodSpecification) scenario.Workload {
	t.Helper()

	workload, err := scenario.NewWorkload(deployedWorkloadName, somePodCount, declaring)
	if err != nil {
		t.Fatalf("build a Workload of %d Pods: %v", somePodCount, err)
	}

	return workload
}

// aSpecificationOf is a specification holding the settings named. This suite
// builds its own with internal/scenario's constructors rather than naming one of
// the lab's predefined definitions: those are defined in internal/lab, which
// imports this package, so what is mapped here are settings and not a catalogue.
func aSpecificationOf(settings ...scenario.Setting) scenario.PodSpecification {
	return scenario.NewPodSpecification(someSpecificationName, settings...)
}

// anEvenSpreadOver is a spread setting counted over one label, enforced and
// reading the Nodes' taints, which is the shape every spread the lab defines has.
func anEvenSpreadOver(topologyKey string, maxSkew int) scenario.TopologySpread {
	return scenario.NewTopologySpread(topologyKey, maxSkew, scenario.DoNotSchedule, scenario.Honor)
}

// aNodeConditionToleration is a toleration of one taint a Node that stopped
// responding carries, for the waiting time the lab sets.
func aNodeConditionToleration(taint string) scenario.Toleration {
	return scenario.NewToleration(taint, scenario.Exists, scenario.NoExecute, theLabsWaitInSeconds())
}

// theLabsWaitInSeconds is the lab's own waiting time as the field Kubernetes
// holds it in, read from the constant that sets it rather than written out.
func theLabsWaitInSeconds() int64 {
	return int64(scenario.UnresponsiveNodeWait.Seconds())
}

// workloadSelector is the selector every spread constraint carries, so a
// constraint counts this Workload's Pods and no others.
func workloadSelector() *metav1.LabelSelector {
	return &metav1.LabelSelector{MatchLabels: map[string]string{workloadLabel: deployedWorkloadName}}
}

// theConstraintOver and theTolerationOf are the one constraint and the one
// toleration a setting becomes, written in the client library's own vocabulary so
// each field is stated independently of the mapper that fills it.
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

// A Pod template is the specification the Workload declared, transcribed field by
// field: one enforced constraint per spread setting at the Skew that setting
// names, and one toleration per toleration setting for the time it names, per
// docs/adr/2026-10-05-workload-declares-a-predefined-pod-specification.md. A
// specification setting no spread carries no constraint and still its own
// tolerations.
func TestAWorkloadDeclaringAPredefinedSpecification_ShouldBeDeployedWithTheSettingsItsDefinitionNames(t *testing.T) {
	declared := map[string]struct {
		specification scenario.PodSpecification
		constraints   []corev1.TopologySpreadConstraint
		tolerations   []corev1.Toleration
	}{
		"a specification spreading its Pods over the Zones and over the Nodes": {
			specification: aSpecificationOf(
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
			specification: aSpecificationOf(aNodeConditionToleration(scenario.TaintNodeNotReady)),
			tolerations:   []corev1.Toleration{theTolerationOf(corev1.TaintNodeNotReady)},
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

// theCriticalPriorityClass is a Pod setting the lab's own vocabulary does not
// name, which is what a Scenario reaches through full access, and
// beforeEverySetting is the place a full access written first on a Workload line
// holds: no setting of the specification is transcribed before it runs.
const (
	theCriticalPriorityClass = "critical"
	beforeEverySetting       = 0
)

// The changes a full access makes, each one as a Scenario writes it: a setting
// the lab's own vocabulary does not name, one it does, the lab's own waiting
// time dropped, and the whole specification left empty.
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

// afterEverySettingOf is the place a full access written last on a Workload line
// holds: every setting the specification holds is transcribed before it runs.
func afterEverySettingOf(declared scenario.PodSpecification) int {
	return len(declared.Settings())
}

// Full access reaches every Kubernetes Pod setting, the ones the lab's own
// vocabulary does not name included, so what a Scenario wrote stands on the Pod
// template its Workload is deployed with.
func TestASpecificationCustomizedWithFullAccess_ShouldHoldTheSettingsTheScenarioNamed(t *testing.T) {
	declared := aSpecificationOf(aNodeConditionToleration(scenario.TaintNodeNotReady))

	deployed := toDeployment(aWorkload(t, declared), PodSpecAccessInPlace{
		Access:        namingThePriorityClass(theCriticalPriorityClass),
		AfterSettings: afterEverySettingOf(declared),
	})

	expect.Expect(t, deployed.Spec.Template.Spec.PriorityClassName).To(expect.Equal(theCriticalPriorityClass))
	expect.Expect(t, deployed.Spec.Template.Spec.Tolerations).
		To(expect.Equal([]corev1.Toleration{theTolerationOf(corev1.TaintNodeNotReady)}))
}

// A full access runs in the place it was written, so where it and a setting of
// the lab's own name one Kubernetes field, the one written later is what the Pod
// template carries.
func TestFullAccessWrittenBesideTheLabsOwnCustomizations_ShouldWinWhereItIsWrittenLater(t *testing.T) {
	declared := aSpecificationOf(anEvenSpreadOver(scenario.ZoneLabel, evenSkew))

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

// Edge case 7: the specification's own tolerations are transcribed after the last
// full access, so the unresponsive-Node wait holds whatever full access left out.
func TestFullAccessLeavingOutASettingTheLabDependsOn_ShouldLeaveThatSettingHolding(t *testing.T) {
	declared := aSpecificationOf(
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

// Full access reaches the Pod specification alone, so the label a Workload's Pods
// are counted by, the selector reading them and the Deployment's own name and
// labels stand even where a full access leaves nothing of that specification.
func TestAWorkloadCustomizedWithFullAccess_ShouldStillHaveItsPodsCountedAsItsOwn(t *testing.T) {
	declared := aSpecificationOf(anEvenSpreadOver(scenario.ZoneLabel, evenSkew))

	deployed := toDeployment(aWorkload(t, declared), PodSpecAccessInPlace{
		Access: leavingNothingOfThePodSpecification(), AfterSettings: beforeEverySetting,
	})

	expect.Expect(t, deployed.Name).To(expect.Equal(deployedWorkloadName))
	expect.Expect(t, deployed.Labels).To(expect.Equal(map[string]string{workloadLabel: deployedWorkloadName}))
	expect.Expect(t, deployed.Spec.Selector).To(expect.Equal(workloadSelector()))
	expect.Expect(t, deployed.Spec.Template.Labels).To(expect.Equal(map[string]string{workloadLabel: deployedWorkloadName}))
}

// Every Kubernetes name internal/scenario spells itself is pinned to the client
// library's own constant: a mistyped label, taint or value would drop a spread or
// a toleration silently and produce a Placement nobody asked for
// (docs/architecture-principles.md § Domain Core).
func TestEveryKubernetesNameTheLabSpells_ShouldEqualTheClientLibrarysOwnConstant(t *testing.T) {
	expect.Expect(t, scenario.ZoneLabel).To(expect.Equal(corev1.LabelTopologyZone))
	expect.Expect(t, scenario.HostnameLabel).To(expect.Equal(corev1.LabelHostname))
	expect.Expect(t, scenario.TaintNodeNotReady).To(expect.Equal(corev1.TaintNodeNotReady))
	expect.Expect(t, scenario.TaintNodeUnreachable).To(expect.Equal(corev1.TaintNodeUnreachable))
	expect.Expect(t, string(scenario.DoNotSchedule)).To(expect.Equal(string(corev1.DoNotSchedule)))
	expect.Expect(t, string(scenario.Honor)).To(expect.Equal(string(corev1.NodeInclusionPolicyHonor)))
	expect.Expect(t, string(scenario.Exists)).To(expect.Equal(string(corev1.TolerationOpExists)))
	expect.Expect(t, string(scenario.NoExecute)).To(expect.Equal(string(corev1.TaintEffectNoExecute)))
}

// No Pod affinity rule is set: a required anti-affinity caps one Pod per Node
// absolutely, which would leave a Pod Pending after a Node is lost, while a
// maximum Skew of one keeps every Pod Running
// (docs/adr/2026-10-04-scenario-declared-pod-spread.md).
func TestADeployedPodTemplate_ShouldCarryNoPodAffinityRule(t *testing.T) {
	deployed := toDeployment(aWorkload(t, aSpecificationOf(anEvenSpreadOver(scenario.ZoneLabel, evenSkew))))

	expect.Expect(t, deployed.Spec.Template.Spec.Affinity).To(expect.Equal[*corev1.Affinity](nil))
}

// The container a Workload's Pods run is the mapper's own, as the Workload label
// and the selector are: a specification is a package-level value and can name
// neither, per
// docs/adr/2026-10-05-workload-declares-a-predefined-pod-specification.md.
func TestADeployedPodTemplate_ShouldCarryTheContainerNoSpecificationCanName(t *testing.T) {
	deployed := toDeployment(aWorkload(t, aSpecificationOf(anEvenSpreadOver(scenario.ZoneLabel, evenSkew))))

	expect.Expect(t, deployed.Spec.Template.Spec.Containers).To(expect.Equal([]corev1.Container{
		{Name: workloadContainer, Image: workloadImage},
	}))
}

// aReportedStatus is the status a Node reports: the capacity this suite
// provisions with, and the conditions named.
func aReportedStatus(conditions ...corev1.NodeCondition) corev1.NodeStatus {
	return corev1.NodeStatus{Capacity: aReportedCapacity(), Conditions: conditions}
}

// aReportedCapacity is the capacity a Node of this suite reports room for, in all
// three dimensions: a Node the lab created reports every one of them, which is
// what an observation reads back.
func aReportedCapacity() corev1.ResourceList {
	return corev1.ResourceList{
		corev1.ResourcePods:   *resource.NewQuantity(somePodsPerNode, resource.DecimalSI),
		corev1.ResourceCPU:    *resource.NewMilliQuantity(int64(someCPUPerNode), resource.DecimalSI),
		corev1.ResourceMemory: *resource.NewQuantity(int64(someMemoryPerNode)*bytesPerMebibyte, resource.BinarySI),
	}
}

// theReadyCondition is what a Node answering reports about itself, and
// someOtherCondition one further condition the control plane sets on it, which the
// lab reads and does not decide.
func theReadyCondition() corev1.NodeCondition {
	return corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionTrue, Reason: nodeReadyReason}
}

func someOtherCondition() corev1.NodeCondition {
	return corev1.NodeCondition{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse}
}

// theLostHeartbeatCondition is the Ready condition the lab writes for a Node that
// stopped responding, which is a lost heartbeat as the control plane reads it.
func theLostHeartbeatCondition() corev1.NodeCondition {
	return corev1.NodeCondition{
		Type:    corev1.NodeReady,
		Status:  corev1.ConditionUnknown,
		Reason:  nodeLostHeartbeatReason,
		Message: nodeLostHeartbeatMessage,
	}
}

// The status of a Node that stopped responding reports its Ready condition
// Unknown and leaves every other condition as the Node reported it: the lab stops
// the Node answering and does not decide what else is true of it, per
// docs/adr/2026-10-05-node-failure-withholds-the-nodes-status.md. Each case also
// asserts the status the lab read is left as it was, since the lab writes a
// changed copy back and reads the same Node again on the next Failure.
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

// A Node the KWOK controller no longer maintains keeps its place in every Topology
// read and every spread domain, so the marking the controller selects by is the
// one label the lab takes off. The labels it read keep it, because it writes a
// changed copy back.
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
	deployed := toDeployment(aWorkload(t, aSpecificationOf(anEvenSpreadOver(scenario.ZoneLabel, evenSkew))))

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

// A provisioned Node reports the capacity its Topology stated in all three
// dimensions, and reports it as allocatable too: the scheduler fits the requests
// of the Pods it places against what a Node reports allocatable.
func TestATopologyStatingItsNodesCpuMemoryAndPodCount_ShouldProvisionNodesWithThatCapacity(t *testing.T) {
	provisioned := aNode(t)

	mapped := toNode(provisioned)

	expect.Expect(t, theCapacityReportedIn(t, mapped.Status.Capacity)).To(expect.Equal(provisioned.Capacity))
	expect.Expect(t, theCapacityReportedIn(t, mapped.Status.Allocatable)).To(expect.Equal(provisioned.Capacity))
	expect.Expect(t, mapped.Status.Conditions).To(expect.Equal([]corev1.NodeCondition{
		{Type: corev1.NodeReady, Status: corev1.ConditionTrue, Reason: nodeReadyReason},
	}))
}

// theCapacityReportedIn is the Node capacity one resource list of a mapped Node
// reports, read back through the dimensions the lab wrote, so what a test
// compares is one whole capacity rather than three quantities picked apart.
func theCapacityReportedIn(t *testing.T, reported corev1.ResourceList) scenario.NodeCapacity {
	t.Helper()

	return aCapacityOf(t,
		int(reported.Pods().Value()),
		scenario.CPU(reported.Cpu().MilliValue()),
		scenario.Memory(reported.Memory().Value()/bytesPerMebibyte))
}

// A cpu or memory amount the lab spells itself is spelled the way the client
// library spells the same amount: a Scenario reads its Node capacity off a Step
// line the lab spells, and reads it again off a Timeline the client library
// spells, so one amount stated two ways would read as two
// (docs/architecture-principles.md § Domain Core).
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

// The Request a specification states reaches the container the mapper owns, under
// the names Kubernetes gives it, because the scheduler places a Pod by what it
// asks for. A specification stating none leaves its Pods asking for nothing, so
// the spread alone decides where they land.
func TestAPodSpecificationStatingARequest_ShouldDeployItsPodsWithThatRequest(t *testing.T) {
	declared := aSpecificationOf(scenario.NewRequest(someRequestedCPU, someRequestedMemory))

	deployed := toDeployment(aWorkload(t, declared))

	expect.Expect(t, theQuantitiesRequestedIn(deployed)).To(expect.Equal([]map[corev1.ResourceName]string{{
		corev1.ResourceCPU:    someRequestedCPU.String(),
		corev1.ResourceMemory: someRequestedMemory.String(),
	}}))
}

func TestAPodSpecificationStatingNoRequest_ShouldDeployItsPodsAskingForNothing(t *testing.T) {
	declared := aSpecificationOf(anEvenSpreadOver(scenario.ZoneLabel, evenSkew))

	deployed := toDeployment(aWorkload(t, declared))

	expect.Expect(t, theQuantitiesRequestedIn(deployed)).To(expect.Equal([]map[corev1.ResourceName]string{{}}))
}

// theQuantitiesRequestedIn are the resource requests the containers of a deployed
// Pod template carry, each amount as Kubernetes spells it, which is how a request
// is read back without a quantity being picked apart.
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

// A Pod's Ready condition is what a Wait for every Pod Ready holds until, so the
// mapper reads it off the Pod the way a Node's own Ready condition is read. A Pod
// the control plane reports no such condition for is not Ready: a Wait holds for
// it rather than passing it over.
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

// A Node carries its NodePool's name as a label, so the cluster tells a Node's
// NodePool from the Node alone (docs/prd.md REQ-SIM-013).
func TestEveryNodeOfANodePool_ShouldCarryALabelNamingItsNodePool(t *testing.T) {
	const gpu = "gpu"
	provisioned := aNodeOfTheNodePool(t, gpu)

	mapped := toNode(provisioned)

	expect.Expect(t, mapped.Labels[nodePoolLabel]).To(expect.Equal(gpu))
}

// An observed Node is read with the NodePool its label names, so a Node read back
// from the cluster equals the one its Topology declared.
func TestAnObservedNode_ShouldBeReadWithThatNodePool(t *testing.T) {
	const gpu = "gpu"
	declared := aNodeOfTheNodePool(t, gpu)

	observed, err := fromNodes([]corev1.Node{*toNode(declared)})
	if err != nil {
		t.Fatalf("read an observed Node: %v", err)
	}

	expect.Expect(t, observed).To(expect.Equal([]scenario.Node{declared}))
}

// The domain holds Kubernetes' label-value syntax itself, since it names no
// Kubernetes package, so it is held to the client library's own validation over
// names it admits and names it refuses (docs/adr/
// 2026-10-09-every-nodepool-is-named-where-it-is-declared.md).
func TestTheNodePoolNameRule_ShouldAgreeWithTheClientLibrarysLabelValueValidation(t *testing.T) {
	const someNodesPerZone = 1

	namesALabelHolds := []string{
		"gpu", "g", "GPU-a100", "a.b_c-d", "0pool9", strings.Repeat("a", validation.LabelValueMaxLength),
	}
	namesNoLabelHolds := []string{
		strings.Repeat("a", validation.LabelValueMaxLength+1),
		"-gpu", "gpu-", ".gpu", "gpu_", "gpu nodes", "gpu/a100", "gpü",
	}

	for _, name := range slices.Concat(namesALabelHolds, namesNoLabelHolds) {
		t.Run(name, func(t *testing.T) {
			_, refusal := scenario.NewNodePool(name, someNodesPerZone, aCapacity(t))

			expect.Expect(t, refusal == nil).To(expect.Equal(len(validation.IsValidLabelValue(name)) == 0))
		})
	}
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

// The values the watched objects below carry: how long after the record started
// the lab received the change, the objects a Timeline reads a change to, how much
// of the Workload the control plane reports ready, and what it reported about a
// Pod it placed.
const (
	observedAfter     = 2500 * time.Millisecond
	watchedNodeName   = "zone-2-node-2"
	watchedPodName    = "web-5d8f9b7c6-2xk4m"
	watchedReplicaSet = "web-5d8f9b7c6"
	replicasRunning   = 3
	scheduledReason   = "Scheduled"
	scheduledMessage  = "Successfully assigned default/web-5d8f9b7c6-2xk4m to zone-2-node-2"
)

// The kind a Kubernetes Event below is about: one a Timeline records, and one
// this lab watches no stream for, along with the name such an unwatched object
// carries.
const (
	reportedPodKind   = "Pod"
	someUnwatchedKind = "Service"
	someUnwatchedName = "web-service"
)

// theCapacityReported is how a Node entry of a Timeline states the capacity this
// suite provisions with: the cpu and memory the scheduler fits a request
// against, then the Pods it has room for, each as Kubernetes spells it.
func theCapacityReported() string {
	return someCPUPerNode.String() + " cpu, " + someMemoryPerNode.String() + " memory and " +
		strconv.Itoa(somePodsPerNode) + " Pods"
}

// aWatchedNode is the Node a Timeline reads a change to, reporting the Ready
// condition given and room for the capacity this suite provisions with.
func aWatchedNode(ready corev1.ConditionStatus) *corev1.Node {
	return &corev1.Node{
		Name: watchedNodeName,
		Status: corev1.NodeStatus{
			Capacity:   aReportedCapacity(),
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: ready}},
		},
	}
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

// aNode is the named default for the one Node a mapping test maps.
func aNode(t *testing.T) scenario.Node {
	t.Helper()

	return aNodeHolding(t, aCapacity(t))
}

// aNodeHolding is the one Node of a Topology whose Nodes hold the capacity named,
// which is how a test that turns on a capacity states it.
func aNodeHolding(t *testing.T, capacity scenario.NodeCapacity) scenario.Node {
	t.Helper()

	const oneZone, oneNodePerZone = 1, 1
	topology, err := topologyOfTheGeneralNodePool(oneZone, oneNodePerZone, capacity)
	if err != nil {
		t.Fatalf("build a Topology of one Node: %v", err)
	}

	return topology.Nodes()[0]
}

// topologyOfTheGeneralNodePool is a Topology of zones Zones each holding the one
// NodePool general of nodesPerZone Nodes at capacity, or the refusal either
// constructor gave.
func topologyOfTheGeneralNodePool(zones, nodesPerZone int, capacity scenario.NodeCapacity) (scenario.Topology, error) {
	pool, err := scenario.NewNodePool("general", nodesPerZone, capacity)
	if err != nil {
		return scenario.Topology{}, err
	}

	return scenario.NewTopology(zones, pool)
}

// aNodeOfTheNodePool is the first Node of a Topology of one Zone holding one
// NodePool of the name stated, which is how a test that turns on a NodePool's
// name states it.
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

// aCapacity is the named default for the Node capacity this suite provisions with.
func aCapacity(t *testing.T) scenario.NodeCapacity {
	t.Helper()

	return aCapacityOf(t, somePodsPerNode, someCPUPerNode, someMemoryPerNode)
}

// aCapacityOf is the Node capacity of the dimensions named, which is what a test
// turning on one of them states.
func aCapacityOf(t *testing.T, pods int, cpu scenario.CPU, memory scenario.Memory) scenario.NodeCapacity {
	t.Helper()

	capacity, err := scenario.NewNodeCapacity(pods, cpu, memory)
	if err != nil {
		t.Fatalf("build a Node capacity of %d Pods, %s cpu and %s memory: %v", pods, cpu, memory, err)
	}

	return capacity
}

// The Startup times a specification below states, in whole seconds: a shortest
// and a longest apart from each other and from the lab's own, so what a template
// carries is read back as the specification's.
const (
	aShortStartup  = 3
	aLongerStartup = 7
)

// aStartupTimeBetween is the Startup time of that shortest and that longest.
func aStartupTimeBetween(t *testing.T, shortest, longest int) scenario.StartupTime {
	t.Helper()

	stated, err := scenario.NewStartupTime(shortest, longest)
	if err != nil {
		t.Fatalf("build a Startup time between %d and %d seconds: %v", shortest, longest, err)
	}

	return stated
}

// theStartupAnnotations are the two annotations the simulator's pod-ready Stage
// reads a Pod's Startup time from: the shortest as its delay and the longest as
// the bound of the time drawn above it, per
// docs/adr/2026-10-08-the-lab-owns-the-simulators-stages-for-a-startup-time.md.
func theStartupAnnotations(shortest, longest time.Duration) map[string]string {
	return map[string]string{
		podReadyDelayAnnotation:       shortest.String(),
		podReadyJitterDelayAnnotation: longest.String(),
	}
}

// Every Workload's Pod template carries a Startup time, the lab's own where its
// specification states none. The annotations sit on the template's metadata,
// which full access does not reach, so a full access leaving nothing of the Pod
// specification leaves them standing (docs/prd.md REQ-SIM-012).
func TestAWorkloadsPodTemplate_ShouldCarryItsStartupTimeWhateverFullAccessNames(t *testing.T) {
	statingNone := aSpecificationOf(anEvenSpreadOver(scenario.ZoneLabel, evenSkew))
	statingARange := statingNone.WithStartupTime(aStartupTimeBetween(t, aShortStartup, aLongerStartup))

	deployed := map[string]struct {
		template *appsv1.Deployment
		carries  map[string]string
	}{
		"a specification stating none": {
			template: toDeployment(aWorkload(t, statingNone)),
			carries:  theStartupAnnotations(scenario.DefaultStartupTime, scenario.DefaultStartupTime),
		},
		"a specification stating a shortest and a longest": {
			template: toDeployment(aWorkload(t, statingARange)),
			carries:  theStartupAnnotations(aShortStartup*time.Second, aLongerStartup*time.Second),
		},
		"a full access leaving nothing of the Pod specification": {
			template: toDeployment(aWorkload(t, statingARange), PodSpecAccessInPlace{
				Access: leavingNothingOfThePodSpecification(), AfterSettings: beforeEverySetting,
			}),
			carries: theStartupAnnotations(aShortStartup*time.Second, aLongerStartup*time.Second),
		},
	}

	for how, taken := range deployed {
		t.Run(how, func(t *testing.T) {
			expect.Expect(t, taken.template.Spec.Template.Annotations).To(expect.Equal(taken.carries))
		})
	}
}
