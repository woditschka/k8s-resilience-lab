// The names of the Workloads the lab's own Scenarios deploy. Each is a business
// capability of an example domain, not a vocabulary the lab fixes.

package scenarios

import "github.com/woditschka/k8s-resilience-lab/internal/lab"

// Checkout is deployed by every Scenario, and Inference beside it on the gpu
// Nodes.
const (
	Checkout  lab.WorkloadName = "checkout"
	Inference lab.WorkloadName = "inference"
)
