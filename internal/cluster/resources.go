package cluster

import (
	"fmt"
	"maps"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

const (
	// zoneLabel and hostnameLabel are spelled in internal/scenario, beside the
	// spread setting that counts over them, so the lab holds one spelling of
	// each.
	zoneLabel     = scenario.ZoneLabel
	hostnameLabel = scenario.HostnameLabel

	// workloadLabel lets a spread constraint and an observation each count one
	// Workload's Pods.
	workloadLabel = "k8s-resilience-lab/workload"

	// Every Node carries nodePoolLabel, so one selector names one NodePool's
	// Nodes; internal/scenario refuses a NodePool stating it.
	nodePoolLabel = scenario.NodePoolLabel

	// fakeNodeLabel is the marking the KWOK controller selects the Nodes it
	// maintains by (manageMarkedNodesArgument). A Failure takes the label off
	// and leaves the annotation, so the Node still reads as simulated. The
	// label stays a literal of this package, because the container's command is
	// composed from this package's own constants alone.
	fakeNodeAnnotation      = "kwok.x-k8s.io/node"
	fakeNodeAnnotationValue = "fake"
	fakeNodeLabel           = "type"
	fakeNodeLabelValue      = "kwok"

	// The lab's pod-ready Stage reads a Startup time from these annotations:
	// the delay before a placed Pod is Ready, and the bound of a time drawn at
	// random above it.
	podReadyDelayAnnotation       = "pod-ready.stage.kwok.x-k8s.io/delay"
	podReadyJitterDelayAnnotation = "pod-ready.stage.kwok.x-k8s.io/jitter-delay"

	// A provisioned Node is Ready at creation rather than after a heartbeat.
	nodeReadyReason = "KubeletReady"

	// These are the words a control plane uses for a Node it has stopped
	// hearing from.
	nodeLostHeartbeatReason  = "NodeStatusUnknown"
	nodeLostHeartbeatMessage = "Kubelet stopped posting node status."

	// The image is never pulled: a Simulated Cluster's Pods are fake, so the
	// reference only has to be valid.
	workloadImage     = "registry.k8s.io/pause:3.10"
	workloadContainer = "workload"

	// A probe Pod carries no Workload label, so an observation of a Workload
	// reads none of them.
	//
	// bytesPerMebibyte is the one place the lab's mebibytes and Kubernetes'
	// bytes meet.
	bytesPerMebibyte = 1024 * 1024

	probeLabel         = "k8s-resilience-lab/probe"
	probeLabelValue    = "provisioning"
	probePodNamePrefix = "provisioning-probe-"
	probeContainer     = "probe"
)

// A phase outside this map is read as PodUnknown, so no foreign value reaches
// internal/scenario.
var observedPhases = map[corev1.PodPhase]scenario.PodPhase{
	corev1.PodPending:   scenario.PodPending,
	corev1.PodRunning:   scenario.PodRunning,
	corev1.PodSucceeded: scenario.PodSucceeded,
	corev1.PodFailed:    scenario.PodFailed,
}

// The Deployment's selector, each spread constraint and an observation's
// selector are built from this one value, so a Workload name is carried
// structurally and the client renders it rather than being concatenated into a
// selector expression.
func workloadLabels(workloadName string) labels.Set {
	return labels.Set{workloadLabel: workloadName}
}

func podSelectorOf(workload scenario.Workload) string {
	return workloadLabels(workload.Name()).String()
}

func probeLabels() labels.Set {
	return labels.Set{probeLabel: probeLabelValue}
}

func probeSelector() string {
	return probeLabels().String()
}

// The probe Pod names its Node by the hostname label rather than binding to it,
// so the scheduler alone places it and a Running probe proves the Node is in
// the scheduler's snapshot. It tolerates the Node's own taints, so a tainted
// Node is provisioned as any other.
func toProbePod(node scenario.Node) *corev1.Pod {
	return &corev1.Pod{
		Name: probePodNamePrefix + node.Name, Labels: probeLabels(),
		Spec: corev1.PodSpec{
			NodeSelector: map[string]string{hostnameLabel: node.Name},
			Tolerations:  toleratingEach(node.Taints),
			Containers: []corev1.Container{
				{Name: probeContainer, Image: workloadImage},
			},
		},
	}
}

func toleratingEach(taints scenario.NodeTaints) []corev1.Toleration {
	var tolerations []corev1.Toleration
	for _, taint := range taints.Taints() {
		tolerations = append(tolerations, corev1.Toleration{
			Key: taint.Key, Operator: corev1.TolerationOpEqual, Value: taint.Value, Effect: corev1.TaintEffect(taint.Effect),
		})
	}

	return tolerations
}

// A probe Pod not yet placed or not yet started says nothing about its Node, so
// only a Running one counts.
func nodesHoldingRunningProbe(observed []corev1.Pod) []string {
	holding := make([]string, 0, len(observed))
	for _, pod := range observed {
		if pod.Status.Phase == corev1.PodRunning && pod.Spec.NodeName != "" {
			holding = append(holding, pod.Spec.NodeName)
		}
	}

	return holding
}

// The lab's own labels are written after the stated ones, so they hold whatever
// was stated.
//
// The capacity is reported allocatable as well as whole, because the scheduler
// fits requests against what a Node reports allocatable and nothing of a
// Simulated Cluster's Node is reserved.
func toNode(node scenario.Node) *corev1.Node {
	stated := node.Labels.Pairs()
	labelled := make(map[string]string, len(stated)+len(scenario.LabsOwnNodeLabelKeys()))
	for _, label := range stated {
		labelled[label.Key] = label.Value
	}
	maps.Copy(labelled, map[string]string{
		zoneLabel:     node.Zone,
		hostnameLabel: node.Name,
		nodePoolLabel: node.NodePool,
		fakeNodeLabel: fakeNodeLabelValue,
	})

	return &corev1.Node{
		Name:        node.Name,
		Labels:      labelled,
		Annotations: map[string]string{fakeNodeAnnotation: fakeNodeAnnotationValue},
		Spec:        corev1.NodeSpec{Taints: toTaints(node.Taints)},
		Status: corev1.NodeStatus{
			Capacity:    toResourceList(node.Capacity),
			Allocatable: toResourceList(node.Capacity),
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue, Reason: nodeReadyReason},
			},
		},
	}
}

func toTaints(stated scenario.NodeTaints) []corev1.Taint {
	var taints []corev1.Taint
	for _, taint := range stated.Taints() {
		taints = append(taints, corev1.Taint{Key: taint.Key, Value: taint.Value, Effect: corev1.TaintEffect(taint.Effect)})
	}

	return taints
}

// Each quantity is built from an integer, never parsed from a string, so this
// mapper cannot panic on a quantity.
func toResourceList(capacity scenario.NodeCapacity) corev1.ResourceList {
	return corev1.ResourceList{
		corev1.ResourcePods:   *resource.NewQuantity(int64(capacity.Pods()), resource.DecimalSI),
		corev1.ResourceCPU:    *toCPUQuantity(capacity.CPU()),
		corev1.ResourceMemory: *toMemoryQuantity(capacity.Memory()),
	}
}

func toCPUQuantity(cpu scenario.CPU) *resource.Quantity {
	return resource.NewMilliQuantity(int64(cpu), resource.DecimalSI)
}

func toMemoryQuantity(memory scenario.Memory) *resource.Quantity {
	return resource.NewQuantity(int64(memory)*bytesPerMebibyte, resource.BinarySI)
}

// PodSpecAccess is full access: a Scenario's own change to the Kubernetes Pod
// specification. It is declared here, beside the mapper that runs it, so
// internal/scenario holds no function value and internal/lab names this type
// rather than Kubernetes' own.
type PodSpecAccess func(pod *corev1.PodSpec)

// PodSpecAccessInPlace pairs a full access with how many settings the mapper
// transcribes before running it, which makes the later of two Customizations
// naming one field the one that stands.
type PodSpecAccessInPlace struct {
	Access        PodSpecAccess
	AfterSettings int
}

// The container, the Workload label and the selector are the mapper's own,
// because a specification is a package-level value and can name neither. A full
// access reaches the Pod specification alone, so the Deployment, its selector
// and the counted label stay beyond it.
func toDeployment(workload scenario.Workload, accesses ...PodSpecAccessInPlace) *appsv1.Deployment {
	replicas := int32(workload.Pods())
	selector := workloadLabels(workload.Name())

	return &appsv1.Deployment{
		Name: workload.Name(), Labels: selector,
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: selector},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      selector,
					Annotations: toStartupAnnotations(workload.PodSpecification().StartupTime()),
				},
				Spec: toPodSpec(workload, accesses),
			},
		},
	}
}

// Equal bounds are one time. The annotations are formatted from validated whole
// seconds, never from a string a Scenario wrote, and sit on the template's
// metadata, which full access does not reach.
func toStartupAnnotations(startup scenario.StartupTime) map[string]string {
	return map[string]string{
		podReadyDelayAnnotation:       startup.Shortest().String(),
		podReadyJitterDelayAnnotation: startup.Longest().String(),
	}
}

// Each full access runs in the place it was written, so what it reads is what
// the settings before it wrote.
//
// The specification's tolerations are written once more after the last full
// access, so a toleration a full access writes does not stand and the waiting
// time before leaving an unresponsive Node stays the lab's to decide.
func toPodSpec(workload scenario.Workload, accesses []PodSpecAccessInPlace) corev1.PodSpec {
	declared := workload.PodSpecification()
	settings := declared.Settings()

	pod := corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: workloadContainer, Image: workloadImage},
		},
	}
	for transcribed, setting := range settings {
		runAccessesWrittenAfter(&pod, transcribed, accesses)
		transcribeSetting(&pod, setting, workload.Name())
	}
	runAccessesWrittenAfter(&pod, len(settings), accesses)
	pod.Tolerations = toTolerations(declared.Tolerations())

	return pod
}

func runAccessesWrittenAfter(pod *corev1.PodSpec, transcribed int, accesses []PodSpecAccessInPlace) {
	for _, written := range accesses {
		if written.AfterSettings == transcribed {
			written.Access(pod)
		}
	}
}

// The settings are sealed in internal/scenario, so these three are every kind
// there is.
func transcribeSetting(pod *corev1.PodSpec, declared scenario.Setting, workloadName string) {
	switch setting := declared.(type) {
	case scenario.TopologySpread:
		pod.TopologySpreadConstraints = append(pod.TopologySpreadConstraints,
			toSpreadConstraint(setting, workloadName))
	case scenario.Toleration:
		pod.Tolerations = append(pod.Tolerations, toToleration(setting))
	case scenario.Request:
		transcribeRequest(pod, setting)
	}
}

// The request goes onto every container, which is the mapper's own unless an
// earlier full access replaced them. It stands where the specification holds
// it, so a later Customization decides.
func transcribeRequest(pod *corev1.PodSpec, declared scenario.Request) {
	for at := range pod.Containers {
		pod.Containers[at].Resources.Requests = toRequestedResources(declared)
	}
}

// The list is built fresh for each container so no two share one.
func toRequestedResources(declared scenario.Request) corev1.ResourceList {
	return corev1.ResourceList{
		corev1.ResourceCPU:    *toCPUQuantity(declared.CPU()),
		corev1.ResourceMemory: *toMemoryQuantity(declared.Memory()),
	}
}

// The selector is the mapper's own, so a constraint counts one Workload's Pods.
func toSpreadConstraint(declared scenario.TopologySpread, workloadName string) corev1.TopologySpreadConstraint {
	taintsPolicy := corev1.NodeInclusionPolicy(declared.NodeTaintsPolicy())

	return corev1.TopologySpreadConstraint{
		MaxSkew:           int32(declared.MaxSkew()),
		TopologyKey:       declared.TopologyKey(),
		WhenUnsatisfiable: corev1.UnsatisfiableConstraintAction(declared.WhenUnsatisfiable()),
		LabelSelector:     &metav1.LabelSelector{MatchLabels: workloadLabels(workloadName)},
		NodeTaintsPolicy:  &taintsPolicy,
	}
}

func toTolerations(declared []scenario.Toleration) []corev1.Toleration {
	tolerations := make([]corev1.Toleration, 0, len(declared))
	for _, tolerated := range declared {
		tolerations = append(tolerations, toToleration(tolerated))
	}
	if len(tolerations) == 0 {
		return nil
	}

	return tolerations
}

// A NoExecute toleration carries the waiting time the setting names rather than
// the admission default. A NoSchedule toleration carries none, since Kubernetes
// takes it for NoExecute alone.
func toToleration(declared scenario.Toleration) corev1.Toleration {
	tolerated := corev1.Toleration{
		Key:      declared.Key(),
		Operator: corev1.TolerationOperator(declared.Operator()),
		Value:    declared.Value(),
		Effect:   corev1.TaintEffect(declared.Effect()),
	}
	if declared.Effect().TakesAWaitingTime() {
		leavingAfter := declared.TolerationSeconds()
		tolerated.TolerationSeconds = &leavingAfter
	}

	return tolerated
}

// Every other condition is left as reported, because the lab stops the Node
// answering and does not decide what else is true of it.
func toUnresponsive(reported corev1.NodeStatus) corev1.NodeStatus {
	unresponsive := reported
	unresponsive.Conditions = append(conditionsOtherThanReady(reported.Conditions), corev1.NodeCondition{
		Type:    corev1.NodeReady,
		Status:  corev1.ConditionUnknown,
		Reason:  nodeLostHeartbeatReason,
		Message: nodeLostHeartbeatMessage,
	})

	return unresponsive
}

// The conditions are copied, so replacing the Ready one does not write through
// the Node the lab read.
func conditionsOtherThanReady(reported []corev1.NodeCondition) []corev1.NodeCondition {
	others := make([]corev1.NodeCondition, 0, len(reported)+1)
	for _, condition := range reported {
		if condition.Type != corev1.NodeReady {
			others = append(others, condition)
		}
	}

	return others
}

// The Zone and hostname labels stay, so the Node keeps its place in every
// Topology read and spread domain.
func withoutTheKWOKMarking(labelled map[string]string) map[string]string {
	unmarked := maps.Clone(labelled)
	delete(unmarked, fakeNodeLabel)

	return unmarked
}

// Only a Ready Node can hold a Pod, so a Placement is read against these alone.
func readyNodesIn(observed []corev1.Node) []corev1.Node {
	ready := make([]corev1.Node, 0, len(observed))
	for _, node := range observed {
		if isReady(node) {
			ready = append(ready, node)
		}
	}

	return ready
}

func isReady(node corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}

	return false
}

// A Pod's Ready condition is the one kubectl rollout status waits on.
func isPodReady(pod corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}

	return false
}

// A Node's Zone, NodePool and labels come from its labels rather than its name,
// so an observation reports what the control plane holds and not what the
// Topology asked for.
func fromNodes(observed []corev1.Node) ([]scenario.Node, error) {
	nodes := make([]scenario.Node, 0, len(observed))
	for _, node := range observed {
		capacity, err := fromReportedCapacity(node.Status.Capacity)
		if err != nil {
			return nil, fmt.Errorf("read the capacity of Node %s: %w", node.Name, err)
		}
		stated, err := scenario.NewNodeLabels(statedLabelsOf(node.Labels))
		if err != nil {
			return nil, fmt.Errorf("read the labels of Node %s: %w", node.Name, err)
		}
		tainted, err := scenario.NewNodeTaints(statedTaintsOf(node.Spec.Taints)...)
		if err != nil {
			return nil, fmt.Errorf("read the taints of Node %s: %w", node.Name, err)
		}

		nodes = append(nodes, scenario.Node{
			Name:     node.Name,
			Zone:     node.Labels[zoneLabel],
			NodePool: node.Labels[nodePoolLabel],
			Capacity: capacity,
			Labels:   stated,
			Taints:   tainted,
		})
	}

	return nodes, nil
}

func statedTaintsOf(carried []corev1.Taint) []scenario.Taint {
	var stated []scenario.Taint
	for _, taint := range carried {
		if !scenario.IsControlPlanesTaint(taint.Key) {
			stated = append(stated, scenario.Taint{Key: taint.Key, Value: taint.Value, Effect: scenario.TaintEffect(taint.Effect)})
		}
	}

	return stated
}

func statedLabelsOf(carried map[string]string) map[string]string {
	stated := maps.Clone(carried)
	maps.DeleteFunc(stated, func(key, _ string) bool { return scenario.IsLabsOwnNodeLabel(key) })

	return stated
}

// This round-trips exactly against toResourceList, since every Node of a
// Simulated Cluster is one the lab created.
func fromReportedCapacity(reported corev1.ResourceList) (scenario.NodeCapacity, error) {
	return scenario.NewNodeCapacity(
		int(reported.Pods().Value()),
		scenario.CPU(reported.Cpu().MilliValue()),
		scenario.Memory(reported.Memory().Value()/bytesPerMebibyte))
}

// A Pod marked for deletion is left out, as the ReplicaSet counts it, so a Pod
// still terminating on a failed Node holds no place in the Placement.
func fromPods(observed []corev1.Pod) []scenario.Pod {
	pods := make([]scenario.Pod, 0, len(observed))
	for _, pod := range observed {
		if pod.DeletionTimestamp != nil {
			continue
		}
		pods = append(pods, scenario.Pod{
			Name:  pod.Name,
			Node:  pod.Spec.NodeName,
			Phase: observedPhaseOf(pod),
			Ready: isPodReady(pod),
		})
	}

	return pods
}

func observedPhaseOf(pod corev1.Pod) scenario.PodPhase {
	phase, named := observedPhases[pod.Status.Phase]
	if !named {
		return scenario.PodUnknown
	}

	return phase
}

// A bookmark or a stream's own error is no change to an object and becomes no
// entry.
var recordedChanges = map[watch.EventType]scenario.Change{
	watch.Added:    scenario.ChangeAdded,
	watch.Modified: scenario.ChangeModified,
	watch.Deleted:  scenario.ChangeRemoved,
}

// An Event about a kind no Timeline records becomes no entry, so no foreign
// kind reaches internal/scenario.
var reportedKinds = map[string]scenario.ObjectKind{
	"Node":       scenario.NodeObject,
	"Pod":        scenario.PodObject,
	"Deployment": scenario.DeploymentObject,
	"ReplicaSet": scenario.ReplicaSetObject,
}

// toEntry is the one place a watched object becomes a Timeline entry: only the
// named fields cross into internal/scenario, so no foreign type or unread field
// reaches a printed or filed Timeline.
func toEntry(elapsed time.Duration, event watch.Event) (scenario.Entry, bool) {
	if reported, isEvent := event.Object.(*corev1.Event); isEvent {
		return fromReportedEvent(elapsed, reported)
	}

	change, recorded := recordedChanges[event.Type]
	if !recorded {
		return scenario.Entry{}, false
	}

	switch object := event.Object.(type) {
	case *corev1.Node:
		return scenario.Entry{
			Elapsed: elapsed, Change: change, Kind: scenario.NodeObject, Name: object.Name,
			Reason: string(readinessOf(*object)), Detail: capacityOf(*object),
		}, true
	case *corev1.Pod:
		return scenario.Entry{
			Elapsed: elapsed, Change: change, Kind: scenario.PodObject, Name: object.Name,
			Reason: string(observedPhaseOf(*object)), Detail: object.Spec.NodeName,
		}, true
	case *appsv1.Deployment:
		return scenario.Entry{
			Elapsed: elapsed, Change: change, Kind: scenario.DeploymentObject, Name: object.Name,
			Detail: replicasReady(object.Status.ReadyReplicas, object.Spec.Replicas),
		}, true
	case *appsv1.ReplicaSet:
		return scenario.Entry{
			Elapsed: elapsed, Change: change, Kind: scenario.ReplicaSetObject, Name: object.Name,
			Detail: replicasReady(object.Status.ReadyReplicas, object.Spec.Replicas),
		}, true
	default:
		return scenario.Entry{}, false
	}
}

// An Event is always read as reported, because it is the control plane
// reporting rather than the object changing.
func fromReportedEvent(elapsed time.Duration, reported *corev1.Event) (scenario.Entry, bool) {
	kind, recorded := reportedKinds[reported.InvolvedObject.Kind]
	if !recorded {
		return scenario.Entry{}, false
	}

	return scenario.Entry{
		Elapsed: elapsed, Change: scenario.ChangeReported, Kind: kind,
		Name: reported.InvolvedObject.Name, Reason: reported.Reason, Detail: reported.Message,
	}, true
}

func readinessOf(node corev1.Node) scenario.NodeReadiness {
	if isReady(node) {
		return scenario.NodeReady
	}

	return scenario.NodeNotReady
}

// The order cpu, memory, Pods matches every reported capacity, so a reader can
// check the capacity a Topology stated.
func capacityOf(node corev1.Node) string {
	reported := node.Status.Capacity

	return fmt.Sprintf("%s cpu, %s memory and %d Pods",
		reported.Cpu(), reported.Memory(), reported.Pods().Value())
}

func replicasReady(ready int32, declared *int32) string {
	if declared == nil {
		return fmt.Sprintf("%d Pods ready", ready)
	}

	return fmt.Sprintf("%d of %d Pods ready", ready, *declared)
}
