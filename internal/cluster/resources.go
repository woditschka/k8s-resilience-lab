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
	// zoneLabel and hostnameLabel are the well-known labels a Node carries its
	// Zone and its own name under: a Zone is a failure domain, a Node is one host.
	// They are spelled in internal/scenario, beside the spread setting that counts
	// its domains over one of them, so the lab holds one spelling of each.
	zoneLabel     = scenario.ZoneLabel
	hostnameLabel = scenario.HostnameLabel

	// workloadLabel carries a Pod's Workload name, so a spread constraint counts
	// one Workload's Pods and an observation reads one Workload's Pods.
	workloadLabel = "k8s-resilience-lab/workload"

	// fakeNodeAnnotation marks a Node as one KWOK simulates, and fakeNodeLabel is
	// the marking the KWOK controller is started to select the Nodes it maintains
	// by (runtime.go manageMarkedNodesArgument). A Failure takes the label off its
	// Node and leaves the annotation, so the Node still reads as a simulated one.
	fakeNodeAnnotation      = "kwok.x-k8s.io/node"
	fakeNodeAnnotationValue = "fake"
	fakeNodeLabel           = "type"
	fakeNodeLabelValue      = "kwok"

	// nodeReadyReason is the reason a provisioned Node reports Ready with. A
	// provisioned Node is Ready at creation rather than after a heartbeat, per
	// docs/adr/2026-10-03-go-program-on-a-kwok-simulated-cluster.md.
	nodeReadyReason = "KubeletReady"

	// nodeLostHeartbeatReason and nodeLostHeartbeatMessage are what a Node that
	// stopped responding reports its Ready condition Unknown with: the words a
	// control plane uses for a Node it has stopped hearing from.
	nodeLostHeartbeatReason  = "NodeStatusUnknown"
	nodeLostHeartbeatMessage = "Kubelet stopped posting node status."

	// workloadImage is the image a Workload's Pod template names, and
	// workloadContainer the container's name. The image is never pulled: a
	// Simulated Cluster's Pods are fake, so the reference only has to be valid
	// (docs/system-design.md § Overview).
	workloadImage     = "registry.k8s.io/pause:3.10"
	workloadContainer = "workload"

	// probeLabel and probeLabelValue mark a Pod as one provisioning created to ask
	// the scheduler whether it holds a Node; probePodNamePrefix names such a Pod
	// after the Node it asks about, and probeContainer is its container. A probe
	// Pod carries no Workload label, so an observation of a Workload reads none of
	// them (docs/adr/2026-10-04-scheduler-observable-provisioning.md).
	probeLabel         = "k8s-resilience-lab/probe"
	probeLabelValue    = "provisioning"
	probePodNamePrefix = "provisioning-probe-"
	probeContainer     = "probe"
)

// observedPhases maps the phases the control plane reports onto the lab's own.
// A phase outside this map is read as PodUnknown, so no foreign value reaches
// internal/scenario.
var observedPhases = map[corev1.PodPhase]scenario.PodPhase{
	corev1.PodPending:   scenario.PodPending,
	corev1.PodRunning:   scenario.PodRunning,
	corev1.PodSucceeded: scenario.PodSucceeded,
	corev1.PodFailed:    scenario.PodFailed,
}

// workloadLabels are the labels one Workload's Pods carry. The Deployment's
// selector, each spread constraint and an observation's Pod selector are all
// built from this one value, so a Workload name is carried structurally and the
// client renders it, rather than being concatenated into a selector expression
// three times over (docs/architecture-principles.md § Persistence and boundary
// mapping).
func workloadLabels(workloadName string) labels.Set {
	return labels.Set{workloadLabel: workloadName}
}

// podSelectorOf is the selector that reads one Workload's Pods, built from the
// same structured labels the Deployment's own selector is.
func podSelectorOf(workload scenario.Workload) string {
	return workloadLabels(workload.Name()).String()
}

// probeLabels are the labels every probe Pod carries. Provisioning's own list
// calls are built from this one value, the same way a Workload's are built from
// workloadLabels.
func probeLabels() labels.Set {
	return labels.Set{probeLabel: probeLabelValue}
}

// probeSelector is the selector that reads provisioning's probe Pods, built from
// the same structured labels a probe Pod carries.
func probeSelector() string {
	return probeLabels().String()
}

// toProbePod is the Pod provisioning creates to ask the scheduler whether it
// holds one Node. The Pod names that Node by its hostname label rather than
// binding itself to it, so the scheduler alone can place it, and a Running probe
// Pod then proves the Node is in the scheduler's own snapshot
// (docs/adr/2026-10-04-scheduler-observable-provisioning.md).
func toProbePod(node scenario.Node) *corev1.Pod {
	return &corev1.Pod{
		Name: probePodNamePrefix + node.Name, Labels: probeLabels(),
		Spec: corev1.PodSpec{
			NodeSelector: map[string]string{hostnameLabel: node.Name},
			Containers: []corev1.Container{
				{Name: probeContainer, Image: workloadImage},
			},
		},
	}
}

// nodesHoldingRunningProbe are the Nodes an observation reports a Running probe
// Pod on. A probe Pod the scheduler has not placed, or has placed and the cluster
// has not started, says nothing about its Node, so only a Running one counts.
func nodesHoldingRunningProbe(observed []corev1.Pod) []string {
	holding := make([]string, 0, len(observed))
	for _, pod := range observed {
		if pod.Status.Phase == corev1.PodRunning && pod.Spec.NodeName != "" {
			holding = append(holding, pod.Spec.NodeName)
		}
	}

	return holding
}

// toNode is the Node the lab creates for one Node of a Topology: a fake Node,
// Ready at creation, reporting the Pod capacity the Topology declared, and
// carrying the Zone and hostname labels a spread rule's domain is read from.
func toNode(node scenario.Node) *corev1.Node {
	pods := *resource.NewQuantity(int64(node.Capacity.Pods()), resource.DecimalSI)

	return &corev1.Node{
		Name: node.Name,
		Labels: map[string]string{
			zoneLabel:     node.Zone,
			hostnameLabel: node.Name,
			fakeNodeLabel: fakeNodeLabelValue,
		},
		Annotations: map[string]string{fakeNodeAnnotation: fakeNodeAnnotationValue},
		Status: corev1.NodeStatus{
			Capacity:    corev1.ResourceList{corev1.ResourcePods: pods},
			Allocatable: corev1.ResourceList{corev1.ResourcePods: pods},
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue, Reason: nodeReadyReason},
			},
		},
	}
}

// PodSpecAccess is full access: a Scenario's own change to the Kubernetes Pod
// specification, for a setting the lab's own vocabulary does not name. It is
// declared here, with the mapper that runs it, so internal/scenario holds no
// function value and internal/lab names this type rather than Kubernetes' own,
// per docs/adr/2026-10-05-workload-declares-a-predefined-pod-specification.md.
type PodSpecAccess func(pod *corev1.PodSpec)

// PodSpecAccessInPlace is one full access in the place a Workload line wrote it:
// the change itself, and how many of the specification's settings the mapper
// transcribes before running it. The place is what makes the later of two
// Customizations naming one Kubernetes field the one that stands.
type PodSpecAccessInPlace struct {
	Access        PodSpecAccess
	AfterSettings int
}

// toDeployment is the Deployment the lab creates for a Workload, so the control
// plane's own controllers produce the Placement. The Pod specification the
// Workload declared is transcribed onto the Pod template field by field; the
// container, the Workload label and the selector are the mapper's own, because a
// specification is a package-level value and can name neither, per
// docs/adr/2026-10-05-workload-declares-a-predefined-pod-specification.md.
//
// The accesses are the full accesses the Workload line wrote, which reach the Pod
// specification alone: the Deployment, its selector and the label the Workload's
// Pods are counted by stay beyond them.
func toDeployment(workload scenario.Workload, accesses ...PodSpecAccessInPlace) *appsv1.Deployment {
	replicas := int32(workload.Pods())
	selector := workloadLabels(workload.Name())

	return &appsv1.Deployment{
		Name: workload.Name(), Labels: selector,
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: selector},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: selector},
				Spec:       toPodSpec(workload, accesses),
			},
		},
	}
}

// toPodSpec is the Pod specification a Workload's Pods are deployed with: the
// container the mapper owns, the declared settings transcribed in the order the
// specification holds them, and each full access run in the place it was written,
// so what it reads is what the settings before it wrote.
//
// The tolerations the Pod template carries are the specification's own, written
// once more after the last full access: they are the lab's, and the waiting time
// before a Pod leaves an unresponsive Node holds whatever a full access left out,
// per the same ADR. A toleration a full access writes itself therefore does not
// stand, which is what keeps that waiting time the lab's to decide.
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

// runAccessesWrittenAfter runs every full access written once that many settings
// are transcribed, in the order the Workload line wrote them.
func runAccessesWrittenAfter(pod *corev1.PodSpec, transcribed int, accesses []PodSpecAccessInPlace) {
	for _, written := range accesses {
		if written.AfterSettings == transcribed {
			written.Access(pod)
		}
	}
}

// transcribeSetting transcribes one setting of a specification onto the Pod
// specification: the one constraint or the one toleration it is, appended in the
// place the specification holds it. The settings are sealed in internal/scenario,
// so these two are every kind there is.
func transcribeSetting(pod *corev1.PodSpec, declared scenario.Setting, workloadName string) {
	switch setting := declared.(type) {
	case scenario.TopologySpread:
		pod.TopologySpreadConstraints = append(pod.TopologySpreadConstraints,
			toSpreadConstraint(setting, workloadName))
	case scenario.Toleration:
		pod.Tolerations = append(pod.Tolerations, toToleration(setting))
	}
}

// toSpreadConstraint is the one constraint a spread setting becomes, each field
// as that setting names it. The selector is the mapper's own: it selects the Pods
// by their Workload label, so a constraint counts one Workload's Pods, per
// docs/adr/2026-10-04-scenario-declared-pod-spread.md.
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

// toTolerations are the tolerations a specification's toleration settings become,
// one each and in definition order.
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

// toToleration is the one toleration a toleration setting becomes, each field as
// that setting names it: the taint it names, matched the way it says, for the
// waiting time it sets rather than the admission default, per
// docs/adr/2026-10-05-node-failure-withholds-the-nodes-status.md.
func toToleration(declared scenario.Toleration) corev1.Toleration {
	leavingAfter := declared.TolerationSeconds()

	return corev1.Toleration{
		Key:               declared.Key(),
		Operator:          corev1.TolerationOperator(declared.Operator()),
		Effect:            corev1.TaintEffect(declared.Effect()),
		TolerationSeconds: &leavingAfter,
	}
}

// toUnresponsive is the status of a Node that stopped responding: the status it
// reported, with its Ready condition Unknown and the reason a lost heartbeat
// carries. Every other condition is left as reported, because the lab stops the
// Node answering and does not decide what else is true of it.
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

// conditionsOtherThanReady are the conditions a Node reports apart from its Ready
// one, in a slice of their own, so replacing that one condition does not write
// through the Node the lab read.
func conditionsOtherThanReady(reported []corev1.NodeCondition) []corev1.NodeCondition {
	others := make([]corev1.NodeCondition, 0, len(reported)+1)
	for _, condition := range reported {
		if condition.Type != corev1.NodeReady {
			others = append(others, condition)
		}
	}

	return others
}

// withoutTheKWOKMarking are the labels of a Node the KWOK controller no longer
// maintains: the labels it carried, less the marking the controller selects by.
// Its Zone and hostname labels stay, so the Node keeps its place in every
// Topology read and every spread domain.
func withoutTheKWOKMarking(labelled map[string]string) map[string]string {
	unmarked := maps.Clone(labelled)
	delete(unmarked, fakeNodeLabel)

	return unmarked
}

// readyNodesIn are the observed Nodes the control plane reports Ready. Only a
// Ready Node can hold a Pod, so a Placement is read against these alone.
func readyNodesIn(observed []corev1.Node) []corev1.Node {
	ready := make([]corev1.Node, 0, len(observed))
	for _, node := range observed {
		if isReady(node) {
			ready = append(ready, node)
		}
	}

	return ready
}

// isReady reports whether a Node carries a Ready condition that is true.
func isReady(node corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}

	return false
}

// isPodReady reports whether a Pod carries a Ready condition that is true, which
// is the condition kubectl rollout status waits on and the one a Wait for every
// Pod Ready holds until. A Pod reporting no such condition is not Ready, the way
// a Node reporting none is no Ready Node.
func isPodReady(pod corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}

	return false
}

// fromNodes reads the Nodes of one observation. A Node's Zone comes from its Zone
// label rather than from its name, so an observation reports what the control
// plane holds and not what the Topology asked for.
func fromNodes(observed []corev1.Node) ([]scenario.Node, error) {
	nodes := make([]scenario.Node, 0, len(observed))
	for _, node := range observed {
		capacity, err := scenario.NewNodeCapacity(int(node.Status.Capacity.Pods().Value()))
		if err != nil {
			return nil, fmt.Errorf("read the capacity of Node %s: %w", node.Name, err)
		}

		nodes = append(nodes, scenario.Node{
			Name:     node.Name,
			Zone:     node.Labels[zoneLabel],
			Capacity: capacity,
		})
	}

	return nodes, nil
}

// fromPods reads the Pods of one observation, each with the Node the control
// plane placed it on and the phase it reports. A Pod the control plane has
// marked for deletion is left out: it is leaving, which is how the Workload's
// own ReplicaSet counts it, so a Pod still terminating on a Node that stopped
// responding holds no place in the Placement, per
// docs/adr/2026-10-05-node-failure-withholds-the-nodes-status.md.
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

// observedPhaseOf is the phase the control plane reports for one Pod, read
// through the lab's own phases, so a phase it does not name reaches
// internal/scenario as PodUnknown rather than unread.
func observedPhaseOf(pod corev1.Pod) scenario.PodPhase {
	phase, named := observedPhases[pod.Status.Phase]
	if !named {
		return scenario.PodUnknown
	}

	return phase
}

// recordedChanges maps the watch event types a Timeline records onto the lab's
// own changes. A type outside this map — a bookmark, or a stream's own error —
// is no change to an object and becomes no entry.
var recordedChanges = map[watch.EventType]scenario.Change{
	watch.Added:    scenario.ChangeAdded,
	watch.Modified: scenario.ChangeModified,
	watch.Deleted:  scenario.ChangeRemoved,
}

// reportedKinds maps the kinds a Kubernetes Event can be about onto the lab's
// own. An Event about a kind no Timeline records becomes no entry, so no foreign
// kind reaches internal/scenario.
var reportedKinds = map[string]scenario.ObjectKind{
	"Node":       scenario.NodeObject,
	"Pod":        scenario.PodObject,
	"Deployment": scenario.DeploymentObject,
	"ReplicaSet": scenario.ReplicaSetObject,
}

// toEntry is the one place a watched Kubernetes object becomes an entry of a
// Timeline: the named fields of that object alone cross into internal/scenario,
// so no foreign type and no unread field reaches a printed or filed Timeline
// (docs/security-principles.md § Language Realization).
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

// fromReportedEvent is the entry one Kubernetes Event becomes: the control
// plane's own reason for what it did to the object the Event names, which is
// where the reason for a Placement comes from. An Event is always read as
// reported, because an Event is the control plane reporting rather than the
// object changing.
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

// readinessOf is the Ready condition the control plane reports for a Node, in
// the words internal/scenario owns, so neither side of this mapper writes them.
func readinessOf(node corev1.Node) scenario.NodeReadiness {
	if isReady(node) {
		return scenario.NodeReady
	}

	return scenario.NodeNotReady
}

// capacityOf is how many Pods a Node reports room for.
func capacityOf(node corev1.Node) string {
	return fmt.Sprintf("%d Pods", node.Status.Capacity.Pods().Value())
}

// replicasReady is how much of a Workload's own object the control plane reports
// ready, which is how a Deployment and a ReplicaSet change reads. An object
// declaring no replica count reports only what is ready.
func replicasReady(ready int32, declared *int32) string {
	if declared == nil {
		return fmt.Sprintf("%d Pods ready", ready)
	}

	return fmt.Sprintf("%d of %d Pods ready", ready, *declared)
}
