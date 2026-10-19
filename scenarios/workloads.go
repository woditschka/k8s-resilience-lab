// The names of the Workloads the lab's own Scenarios deploy, one exported
// constant each. It is the one home of those names, so a Scenario names a
// Workload rather than writing the string into its Step. Each is a business
// capability of the domain the Scenarios beside it model, which is an example
// and not a vocabulary the lab fixes, per
// docs/adr/2026-10-03-scenarios-as-go-functions.md.

package scenarios

import "github.com/woditschka/k8s-resilience-lab/internal/lab"

// Checkout is the Workload every Scenario of the lab deploys, and Inference the
// one a Scenario deploys beside it on the gpu Nodes.
const (
	Checkout  lab.WorkloadName = "checkout"
	Inference lab.WorkloadName = "inference"
)
