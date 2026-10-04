package scenario

import (
	"math"
	"slices"
)

// PodPhase is the phase the control plane reports for one observed Pod.
type PodPhase string

const (
	// PodPending is a Pod the control plane has not placed on a Node.
	PodPending PodPhase = "Pending"
	// PodRunning is a Pod placed on a Node and running there.
	PodRunning PodPhase = "Running"
	// PodSucceeded is a Pod whose containers all ran to completion.
	PodSucceeded PodPhase = "Succeeded"
	// PodFailed is a Pod whose containers stopped and at least one failed.
	PodFailed PodPhase = "Failed"
	// PodUnknown is a Pod in a phase the lab does not name, so a phase the
	// control plane reports can never reach a Scenario unread.
	PodUnknown PodPhase = "Unknown"
)

// Pod is one observed replica of a Workload, placed on a Node or left unplaced.
// The name comes from the control plane, so the type carries no invariant of its
// own.
type Pod struct {
	Name string
	// Node is the Node this Pod is placed on, empty while it is unplaced.
	Node  string
	Phase PodPhase
}

// Placed reports whether the control plane has put this Pod on a Node.
func (p Pod) Placed() bool {
	return p.Node != ""
}

// Placement is where a Workload's Pods sit at one observation, read against the
// Nodes that could have held them. A Node or a Zone holding no Pod is counted as
// zero, so an empty domain is visible in the report rather than missing from it.
type Placement struct {
	nodes []Node
	pods  []Pod
}

// NewPlacement records one observation of a Workload against the Nodes observed
// with it.
func NewPlacement(nodes []Node, pods []Pod) Placement {
	return Placement{nodes: slices.Clone(nodes), pods: slices.Clone(pods)}
}

// Nodes are the Nodes this observation was read against.
func (p Placement) Nodes() []Node {
	return slices.Clone(p.nodes)
}

// Pods are the Pods this observation found, in the order they were observed.
func (p Placement) Pods() []Pod {
	return slices.Clone(p.pods)
}

// PodsInPhase are the observed Pods the control plane reports in phase.
func (p Placement) PodsInPhase(phase PodPhase) []Pod {
	var inPhase []Pod
	for _, pod := range p.pods {
		if pod.Phase == phase {
			inPhase = append(inPhase, pod)
		}
	}

	return inPhase
}

// PodsPerNode counts this Workload's Pods on each observed Node, an empty Node
// included as zero. A Pod on a Node this observation does not hold is counted
// under that Node's name, because the report states what was observed.
func (p Placement) PodsPerNode() map[string]int {
	counts := make(map[string]int, len(p.nodes))
	for _, node := range p.nodes {
		counts[node.Name] = 0
	}
	for _, pod := range p.pods {
		if pod.Placed() {
			counts[pod.Node]++
		}
	}

	return counts
}

// PodsPerZone counts this Workload's Pods in each observed Zone, an empty Zone
// included as zero. A Pod on a Node this observation does not hold belongs to no
// known Zone and is counted in none.
func (p Placement) PodsPerZone() map[string]int {
	zoneOf := make(map[string]string, len(p.nodes))
	counts := make(map[string]int)
	for _, node := range p.nodes {
		zoneOf[node.Name] = node.Zone
		counts[node.Zone] = 0
	}
	for _, pod := range p.pods {
		if zone, known := zoneOf[pod.Node]; known && pod.Placed() {
			counts[zone]++
		}
	}

	return counts
}

// Skew is the gap between the fullest and the emptiest domain of this
// observation: zero when every domain holds the same number of Pods.
func (p Placement) Skew(domain SpreadDomain) int {
	counts := p.countsOver(domain)
	if len(counts) == 0 {
		return 0
	}

	fullest, emptiest := 0, math.MaxInt
	for _, count := range counts {
		fullest = max(fullest, count)
		emptiest = min(emptiest, count)
	}

	return fullest - emptiest
}

// countsOver is this observation's Pod count per domain member.
func (p Placement) countsOver(domain SpreadDomain) map[string]int {
	if domain == SpreadOverNodes {
		return p.PodsPerNode()
	}

	return p.PodsPerZone()
}

// Equal reports whether two observations found the same Pods on the same Nodes,
// which is what AwaitSettled compares consecutive observations through. It is
// also the equality go-cmp compares this type through, since its fields are
// unexported.
func (p Placement) Equal(other Placement) bool {
	return slices.Equal(p.nodes, other.nodes) && slices.Equal(p.pods, other.pods)
}
