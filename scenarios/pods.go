// The predefined Pod specifications the lab's own Scenarios declare, one
// exported function each, and the Named settings they name. It is the one home of
// each name, the Startup time its Pods take and the settings it sets, each naming
// LeaveUnresponsiveNodes for its Pods leaving a Node that stopped responding after
// the lab's waiting time: the lab defines no specification and no Named setting,
// so a Workload declares one of these, per
// docs/adr/2026-10-18-every-pod-specification-is-defined-by-the-scenarios.md and
// docs/adr/2026-10-18-the-scenarios-name-leaving-an-unresponsive-node-as-one-setting.md.

package scenarios

import (
	"github.com/woditschka/k8s-resilience-lab/internal/lab/node"
	"github.com/woditschka/k8s-resilience-lab/internal/lab/pod"
)

// ResilientPlacement spreads a Workload's Pods as evenly over the Zones of its
// Topology, and over its Nodes, as the control plane can place them, so the loss
// of one Node or one Zone takes the fewest Pods with it. Each Pod takes its own
// time between half a second and a second to start, so the Pending-on-Node state
// stays visible and two runs may record different Timelines.
func ResilientPlacement() pod.Specification {
	return pod.Predefined("Resilient placement", pod.StartupMillisBetween(500, 1000),
		LeaveUnresponsiveNodes(),
		EvenSpreadOver(node.Zone),
		EvenSpreadOver(node.Hostname),
	)
}

// DefaultPlacement sets no spread at all. Where its Workload's Pods land is the
// scheduler's own choice, so a Scenario declaring it asserts only what holds of
// every Placement. Its Pods start as Resilient placement's do.
func DefaultPlacement() pod.Specification {
	return pod.Predefined("Default placement", pod.StartupMillisBetween(500, 1000),
		LeaveUnresponsiveNodes())
}

// SpreadOverOnDemandAndSpot spreads a Workload's Pods as evenly over the Zones of
// its Topology, and over the values of the CapacityType label its NodePools
// state, as the control plane can place them, so losing the spot Nodes takes the
// fewest Pods with it. Its Pods start as Resilient placement's do.
func SpreadOverOnDemandAndSpot() pod.Specification {
	return pod.Predefined("Spread over on-demand and spot", pod.StartupMillisBetween(500, 1000),
		LeaveUnresponsiveNodes(),
		EvenSpreadOver(node.Zone),
		EvenSpreadOver(CapacityType),
	)
}

// ToleratingGPUNodes lets a Workload's Pods onto the Nodes tainted GPU, and asks
// for more cpu and memory than a Standard Node holds, so only a GPU Node has room
// for one of its Pods: the toleration permits them there, and the request is what
// draws them there. Its Pods start as Resilient placement's do.
func ToleratingGPUNodes() pod.Specification {
	return pod.Predefined("Tolerating GPU Nodes", pod.StartupMillisBetween(500, 1000),
		LeaveUnresponsiveNodes(),
		pod.Toleration{Key: GPU, Operator: pod.Equal, Value: GPUPresent, Effect: pod.NoSchedule},
		pod.Request{CPU: 24, Memory: 96 * pod.Gi},
	)
}

// LeaveUnresponsiveNodes is the Named setting for Pods leaving a Node that
// stopped responding after the lab's waiting time. It stands for exactly the
// not-ready and the unreachable toleration, under their Kubernetes names, and
// every specification above names it in their place. A Workload's Given line
// reports both tolerations it stands for.
func LeaveUnresponsiveNodes() pod.Customization {
	return pod.Settings(
		pod.Toleration{Key: node.NotReady, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: node.UnresponsiveWait},
		pod.Toleration{Key: node.Unreachable, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: node.UnresponsiveWait},
	)
}

// EvenSpreadOver is the Named setting for the Even spread: a Workload's Pods
// spread over the domains of the label given, differing by at most one Pod between
// any two of them, a Pod no Node can take within that left Pending, and only the
// Nodes whose taints the Pods tolerate counted. A Scenario wanting another skew,
// or a Pod placed past it, writes the topology spread constraint itself.
func EvenSpreadOver(topologyKey string) pod.TopologySpreadConstraint {
	return pod.TopologySpreadConstraint{
		TopologyKey: topologyKey, MaxSkew: 1, WhenUnsatisfiable: pod.DoNotSchedule, NodeTaintsPolicy: pod.Honor,
	}
}
