package scenario

import (
	"math"
	"slices"
)

// SpreadDomain is the axis a Placement's Skew is read over: how evenly a
// Workload's Pods sit over the Zones of its Topology, or over its Nodes. A
// Workload declares its spread as the settings of its Pod specification, so this
// is a way of reading an observation and not a declaration.
type SpreadDomain string

const (
	// SpreadOverZones reads a Placement over the Zones of its Topology.
	SpreadOverZones SpreadDomain = "Zones"
	// SpreadOverNodes reads a Placement over the Nodes of its Topology.
	SpreadOverNodes SpreadDomain = "Nodes"
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
	// Ready is whether the control plane reports this Pod's Ready condition true,
	// which is the condition a Wait for every Pod Ready holds until, per
	// docs/adr/2026-10-07-a-scenario-writes-its-own-wait.md.
	Ready bool
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

// PodsNotReady are the observed Pods the control plane does not report Ready, in
// the order they were observed. A Wait for every Pod Ready names them when the
// lab's waiting time outlasts it, so a reader of an expired Wait sees which Pods
// held it open.
func (p Placement) PodsNotReady() []Pod {
	var notReady []Pod
	for _, pod := range p.pods {
		if !pod.Ready {
			notReady = append(notReady, pod)
		}
	}

	return notReady
}

// PodsRunningApartFrom are the observed Pods the control plane reports Running on
// a Node other than the ones named, which is what a Workload kept at the moment a
// Failure stopped those Nodes. A Pod the control plane has placed nowhere is on no
// such Node and is among none of them.
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

// PodsItCannotAccountFor are the Pods this observation found on a Node it was
// not read against. It can name no Zone for such a Pod, so it counts it in none,
// which is why the Placement reports it rather than passing it over.
//
// An unplaced Pod is accounted for: the observation holds no Node for it because
// the control plane has placed it nowhere, which the Placement already states.
// A Pod on a Node that has left the observation is the other case — a Node that
// stopped responding still holding a Pod the control plane has yet to move.
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
