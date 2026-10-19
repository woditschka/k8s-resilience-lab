// The keys and values of the taints the lab's own Scenarios state on their
// NodePools, one exported constant each. It is the one home of each, so a
// NodePool stating a taint and a toleration of it name the same key and value
// rather than writing the string again. The lab owns none of them: each is the
// Scenarios' own, per
// docs/adr/2026-10-19-a-nodepool-states-noschedule-taints-and-a-later-workload-deploys-beside-the-earlier.md.

package scenarios

// GPU marks a Node holding a gpu, under the key a gpu vendor's device plugin
// taints such a Node with, and GPUPresent is the value it states it under.
const (
	GPU        = "nvidia.com/gpu"
	GPUPresent = "present"
)
