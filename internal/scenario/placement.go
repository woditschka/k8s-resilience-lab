package scenario

import (
	"math"
	"slices"
)

// SpreadDomain is a way of reading an observation and not a declaration, since
// a Workload declares its spread through its Pod specification.
type SpreadDomain string

const (
	SpreadOverZones SpreadDomain = "Zones"
	SpreadOverNodes SpreadDomain = "Nodes"
)

type PodPhase string

const (
	// PodPending also covers a placed Pod still inside its Startup time.
	PodPending   PodPhase = "Pending"
	PodRunning   PodPhase = "Running"
	PodSucceeded PodPhase = "Succeeded"
	PodFailed    PodPhase = "Failed"
	// PodUnknown covers a phase the lab does not name, so no reported phase
	// reaches a Scenario unread.
	PodUnknown PodPhase = "Unknown"
)

// Pod carries no invariant of its own, since its name comes from the control
// plane.
type Pod struct {
	Name string
	// Node is empty while the Pod is unplaced.
	Node  string
	Phase PodPhase
	Ready bool
}

func (p Pod) Placed() bool {
	return p.Node != ""
}

// Placement counts a Node or a Zone holding no Pod as zero, so an empty domain
// is visible in the report rather than missing from it.
type Placement struct {
	nodes []Node
	pods  []Pod
}

func NewPlacement(nodes []Node, pods []Pod) Placement {
	return Placement{nodes: slices.Clone(nodes), pods: slices.Clone(pods)}
}

func (p Placement) Nodes() []Node {
	return slices.Clone(p.nodes)
}

func (p Placement) Pods() []Pod {
	return slices.Clone(p.pods)
}

func (p Placement) PodsInPhase(phase PodPhase) []Pod {
	var inPhase []Pod
	for _, pod := range p.pods {
		if pod.Phase == phase {
			inPhase = append(inPhase, pod)
		}
	}

	return inPhase
}

// PodsNotReady are what a Wait for every Pod Ready names when it expires, so a
// reader sees which Pods held it open.
func (p Placement) PodsNotReady() []Pod {
	var notReady []Pod
	for _, pod := range p.pods {
		if !pod.Ready {
			notReady = append(notReady, pod)
		}
	}

	return notReady
}

// PodsStarting are Pods that have not stopped changing, so a Placement holding
// one is not yet Settled. An unplaced Pod is not starting: it stays Pending
// until a Node has room for it.
func (p Placement) PodsStarting() []Pod {
	var starting []Pod
	for _, pod := range p.pods {
		if pod.Placed() && pod.Phase == PodPending {
			starting = append(starting, pod)
		}
	}

	return starting
}

// PodsRunningApartFrom is what a Workload kept at the moment a Failure stopped
// those Nodes. An unplaced Pod is among none of them.
func (p Placement) PodsRunningApartFrom(nodes []string) []Pod {
	stopped := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		stopped[node] = struct{}{}
	}

	var kept []Pod
	for _, pod := range p.pods {
		if _, onAStoppedNode := stopped[pod.Node]; pod.Phase == PodRunning && pod.Placed() && !onAStoppedNode {
			kept = append(kept, pod)
		}
	}

	return kept
}

// PodsPerNode still counts a Pod on a Node this observation does not hold under
// that Node's name, because the report states what was observed.
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

// PodsPerZone counts a Pod on a Node this observation does not hold in no Zone,
// since it belongs to no known Zone.
func (p Placement) PodsPerZone() map[string]int {
	return p.podsPerGroup(func(node Node) string { return node.Zone })
}

// PodsPerLabelValue counts under none a Pod on a Node carrying no value of key,
// or on a Node this observation does not hold.
func (p Placement) PodsPerLabelValue(key string) map[string]int {
	valueOf := make(map[string]string, len(p.nodes))
	counts := make(map[string]int)
	for _, node := range p.nodes {
		if value, carried := node.Labels.Value(key); carried {
			valueOf[node.Name] = value
			counts[value] = 0
		}
	}
	for _, pod := range p.pods {
		if value, known := valueOf[pod.Node]; known && pod.Placed() {
			counts[value]++
		}
	}

	return counts
}

// PodsPerNodePool counts a Pod on a Node this observation does not hold in no
// NodePool, since it belongs to no known NodePool.
func (p Placement) PodsPerNodePool() map[string]int {
	return p.podsPerGroup(func(node Node) string { return node.NodePool })
}

func (p Placement) podsPerGroup(groupOf func(Node) string) map[string]int {
	groupOfNode := make(map[string]string, len(p.nodes))
	counts := make(map[string]int)
	for _, node := range p.nodes {
		groupOfNode[node.Name] = groupOf(node)
		counts[groupOf(node)] = 0
	}
	for _, pod := range p.pods {
		if group, known := groupOfNode[pod.Node]; known && pod.Placed() {
			counts[group]++
		}
	}

	return counts
}

// PodsItCannotAccountFor reports the Pods the observation can name no Zone for
// rather than passing them over. Such a Pod is still held by a Node that
// stopped responding, and the control plane has yet to move it; an unplaced
// Pod is accounted for.
func (p Placement) PodsItCannotAccountFor() []Pod {
	held := make(map[string]struct{}, len(p.nodes))
	for _, node := range p.nodes {
		held[node.Name] = struct{}{}
	}

	var unaccounted []Pod
	for _, pod := range p.pods {
		if _, known := held[pod.Node]; pod.Placed() && !known {
			unaccounted = append(unaccounted, pod)
		}
	}

	return unaccounted
}

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

func (p Placement) countsOver(domain SpreadDomain) map[string]int {
	if domain == SpreadOverNodes {
		return p.PodsPerNode()
	}

	return p.PodsPerZone()
}

// Equal is what AwaitSettled compares consecutive observations through, and
// go-cmp too, since the fields are unexported.
func (p Placement) Equal(other Placement) bool {
	return slices.EqualFunc(p.nodes, other.nodes, Node.Equal) && slices.Equal(p.pods, other.pods)
}
