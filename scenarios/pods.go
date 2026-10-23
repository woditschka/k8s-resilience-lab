// The predefined Pod specifications and Named settings the lab's own Scenarios
// declare. The lab defines none, so a Workload declares one of these.

package scenarios

import (
	"github.com/woditschka/k8s-resilience-lab/internal/lab/node"
	"github.com/woditschka/k8s-resilience-lab/internal/lab/pod"
)

// ResilientPlacement spreads a Workload's Pods evenly over Zones and Nodes, so
// the loss of one Node or one Zone takes the fewest Pods with it. Each Pod
// takes between half a second and a second to start, so the Pending-on-Node
// state stays visible and two runs may record different Timelines.
func ResilientPlacement() pod.Specification {
	return pod.Predefined("Resilient placement", pod.StartupMillisBetween(500, 1000),
		LeaveUnresponsiveNodes(),
		EvenSpreadOver(node.Zone),
		EvenSpreadOver(node.Hostname),
	)
}

// DefaultPlacement sets no spread, so where its Pods land is the scheduler's
// own choice and a Scenario declaring it asserts only what holds of every
// Placement.
func DefaultPlacement() pod.Specification {
	return pod.Predefined("Default placement", pod.StartupMillisBetween(500, 1000),
		LeaveUnresponsiveNodes())
}

// SpreadOverOnDemandAndSpot spreads a Workload's Pods evenly over Zones and
// over the CapacityType label, so losing the spot Nodes takes the fewest Pods
// with it.
func SpreadOverOnDemandAndSpot() pod.Specification {
	return pod.Predefined("Spread over on-demand and spot", pod.StartupMillisBetween(500, 1000),
		LeaveUnresponsiveNodes(),
		EvenSpreadOver(node.Zone),
		EvenSpreadOver(CapacityType),
	)
}

// ToleratingGPUNodes tolerates the GPU taint, which permits its Pods on a GPU
// Node and draws them to none.
func ToleratingGPUNodes() pod.Specification {
	return pod.Predefined("Tolerating GPU Nodes", pod.StartupMillisBetween(500, 1000),
		LeaveUnresponsiveNodes(),
		pod.Toleration{Key: GPU, Operator: pod.Equal, Value: GPUPresent, Effect: pod.NoSchedule},
	)
}

// LeaveUnresponsiveNodes makes Pods leave a Node that stopped responding after
// the lab's waiting time. It stands for exactly the not-ready and the
// unreachable toleration.
func LeaveUnresponsiveNodes() pod.Customization {
	return pod.Settings(
		pod.Toleration{Key: node.NotReady, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: node.UnresponsiveWait},
		pod.Toleration{Key: node.Unreachable, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: node.UnresponsiveWait},
	)
}

// StandardPod makes each Pod ask for a quarter of a Standard Node's cpu and
// memory. It stands for exactly one Request of 4 cpu and 16Gi.
func StandardPod() pod.Customization {
	return pod.Settings(pod.Request{CPU: 4, Memory: 16 * pod.Gi})
}

// GPUPod makes each Pod ask for more than a Standard Node holds, which is what
// draws a Pod tolerating the GPU taint onto a GPU Node. It stands for exactly
// one Request of 24 cpu and 96Gi.
func GPUPod() pod.Customization {
	return pod.Settings(pod.Request{CPU: 24, Memory: 96 * pod.Gi})
}

// EvenSpreadOver spreads Pods over the domains of the label given with a skew
// of at most one, leaves a Pod no Node can take Pending, and counts only Nodes
// whose taints the Pods tolerate. A Scenario wanting another skew writes the
// topology spread constraint itself.
func EvenSpreadOver(topologyKey string) pod.TopologySpreadConstraint {
	return pod.TopologySpreadConstraint{
		TopologyKey: topologyKey, MaxSkew: 1, WhenUnsatisfiable: pod.DoNotSchedule, NodeTaintsPolicy: pod.Honor,
	}
}
