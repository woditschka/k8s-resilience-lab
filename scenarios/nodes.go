// The Named Node capacities the lab's own Scenarios name. The lab defines none,
// so a NodePool names one of these or states its capacity outright.

package scenarios

import "github.com/woditschka/k8s-resilience-lab/internal/lab/node"

// StandardNode leaves room for more than one Pod, so an even Placement is the
// scheduler honouring the declared spread rather than the Nodes running out of
// room. A Standard Pod takes a quarter of it, so four fit by cpu and memory,
// more than any Scenario spreads onto one Node, and the spread still decides.
func StandardNode() node.Capacity {
	return node.Named("Standard", node.Capacity{CPU: 16, Memory: 64 * node.Gi, Pods: 10})
}

// GPUNode has twice a Standard Node's cpu and memory, so a Pod asking for more
// than a Standard Node holds fits on a GPU Node alone.
func GPUNode() node.Capacity {
	return node.Named("GPU", node.Capacity{CPU: 32, Memory: 128 * node.Gi, Pods: 10})
}
