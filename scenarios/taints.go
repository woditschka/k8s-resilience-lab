// The taint keys and values the lab's own Scenarios state. Each has one home,
// so a NodePool stating a taint and a toleration of it name the same key and
// value.

package scenarios

// GPU is the key a gpu vendor's device plugin taints a gpu Node with.
const (
	GPU        = "nvidia.com/gpu"
	GPUPresent = "present"
)
