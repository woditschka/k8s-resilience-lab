package cluster

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

const (
	// zoneLabel and hostnameLabel are the well-known labels a spread rule's
	// domain is expressed as: a Zone is a failure domain, a Node is one host.
	zoneLabel     = "topology.kubernetes.io/zone"
	hostnameLabel = "kubernetes.io/hostname"

	// workloadLabel carries a Pod's Workload name, so a spread constraint counts
	// one Workload's Pods and an observation reads one Workload's Pods.
	workloadLabel = "k8s-resilience-lab/workload"

	// fakeNodeAnnotation marks a Node for the KWOK controller to maintain, and
	// fakeNodeLabel is the label KWOK's own stages select such a Node by.
	fakeNodeAnnotation      = "kwok.x-k8s.io/node"
	fakeNodeAnnotationValue = "fake"
	fakeNodeLabel           = "type"
	fakeNodeLabelValue      = "kwok"

	// nodeReadyReason is the reason a provisioned Node reports Ready with. A
	// provisioned Node is Ready at creation rather than after a heartbeat, per
	// docs/adr/2026-10-03-go-program-on-a-kwok-simulated-cluster.md.
	nodeReadyReason = "KubeletReady"

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

// toDeployment is the Deployment the lab creates for a Workload, so the control
// plane's own controllers produce the Placement. Each rule the Workload declares
// becomes one topology spread constraint on the Pod template; a Workload
// declaring no rule carries none.
func toDeployment(workload scenario.Workload) *appsv1.Deployment {
	replicas := int32(workload.Pods())
	selector := workloadLabels(workload.Name())

	var constraints []corev1.TopologySpreadConstraint
	for _, rule := range workload.SpreadRules() {
		constraints = append(constraints, toSpreadConstraint(rule, workload.Name()))
	}

	return &appsv1.Deployment{
		Name: workload.Name(), Labels: selector,
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: selector},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: selector},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: workloadContainer, Image: workloadImage},
					},
					TopologySpreadConstraints: constraints,
				},
			},
		},
	}
}

// toSpreadConstraint is the one constraint a spread rule becomes. It is enforced
// rather than preferred, so an unmet rule is visible as a Pending Pod; it counts
// only the Nodes whose taints these Pods tolerate; and it selects the Pods by
// their Workload label, per docs/adr/2026-10-04-scenario-declared-pod-spread.md.
func toSpreadConstraint(rule scenario.SpreadRule, workloadName string) corev1.TopologySpreadConstraint {
	honourTaints := corev1.NodeInclusionPolicyHonor

	return corev1.TopologySpreadConstraint{
		MaxSkew:           int32(rule.MaxSkew()),
		TopologyKey:       topologyKeyOf(rule.Domain()),
		WhenUnsatisfiable: corev1.DoNotSchedule,
		LabelSelector:     &metav1.LabelSelector{MatchLabels: workloadLabels(workloadName)},
		NodeTaintsPolicy:  &honourTaints,
	}
}

// topologyKeyOf is the label a spread domain is counted over.
func topologyKeyOf(domain scenario.SpreadDomain) string {
	if domain == scenario.SpreadOverNodes {
		return hostnameLabel
	}

	return zoneLabel
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
// plane placed it on and the phase it reports.
func fromPods(observed []corev1.Pod) []scenario.Pod {
	pods := make([]scenario.Pod, 0, len(observed))
	for _, pod := range observed {
		phase, named := observedPhases[pod.Status.Phase]
		if !named {
			phase = scenario.PodUnknown
		}

		pods = append(pods, scenario.Pod{
			Name:  pod.Name,
			Node:  pod.Spec.NodeName,
			Phase: phase,
		})
	}

	return pods
}
