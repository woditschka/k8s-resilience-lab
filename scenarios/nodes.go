// The Named Node capacities the lab's own Scenarios name, one exported function
// each. It is the one home of each name and the cpu, memory and Pod count it
// stands for: the lab defines none, so a NodePool names one of these or states
// its capacity outright, per
// docs/adr/2026-10-09-every-nodepool-states-or-names-its-node-capacity.md.

package scenarios

import "github.com/woditschka/k8s-resilience-lab/internal/lab"

// StandardNodes are Nodes of Standard capacity. They leave room for more than
// one Pod per Node, so an even Placement is the scheduler honouring the
// Workload's declared spread rather than the Nodes running out of room, and
// enough cpu and memory for every one of those Pods to ask for an ordinary
// share, so the Pod count stays the tighter bound.
func StandardNodes() lab.DeclaredNodeCapacity {
	return lab.NamedNodeCapacity("Standard", lab.CPU(16), lab.MemoryGi(64), lab.Pods(10))
}
