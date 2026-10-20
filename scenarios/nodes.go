// The Named Node capacities the lab's own Scenarios name. The lab defines none,
// so a NodePool names one of these or states its capacity outright.

package scenarios

import "github.com/woditschka/k8s-resilience-lab/internal/lab/node"

// StandardNodes leave room for more than one Pod per Node, so an even Placement
// is the scheduler honouring the declared spread rather than the Nodes running
// out of room. Their cpu and memory cover an ordinary share for each of those
// Pods, so the Pod count stays the tighter bound.
func StandardNodes() node.Capacity {
	return node.Named("Standard", node.Capacity{CPU: 16, Memory: 64 * node.Gi, Pods: 10})
}

// GPUNodes have twice a Standard Node's cpu and memory, so a Pod asking for
// more than a Standard Node holds fits on one of them alone.
func GPUNodes() node.Capacity {
	return node.Named("GPU", node.Capacity{CPU: 32, Memory: 128 * node.Gi, Pods: 10})
}
