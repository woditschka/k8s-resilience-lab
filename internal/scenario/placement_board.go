package scenario

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	newcomerMark = "+"
	boardIndent  = "  "
	columnGap    = "  "
)

// PlacementBoard is one Wait's view of the whole Simulated Cluster across every
// Workload. It is derived from what was declared and observed, so it carries no
// invariant of its own; its zero value is the board before the first Wait.
type PlacementBoard struct {
	zones    []boardZone
	unplaced []boardPod
}

type boardZone struct {
	name  string
	nodes []boardNode
}

type boardNode struct {
	name     string
	nodePool string
	pods     []boardPod
}

type boardPod struct {
	name     string
	workload string
	newcomer bool
}

type podOnNode struct {
	pod  string
	node string
}

// NewPlacementBoard gives every Node of the Topology its line, holding a Pod or
// not. A Pod is new where previous did not hold it on that Node, so a Pod placed
// since, or a replacement for one evicted, reads as new.
func NewPlacementBoard(topology Topology, pods []Pod, previous PlacementBoard) PlacementBoard {
	heldBefore := previous.held()
	onNode := make(map[string][]boardPod)
	var unplaced []boardPod
	for _, pod := range slices.SortedFunc(slices.Values(pods), inBoardOrder) {
		_, held := heldBefore[podOnNode{pod: pod.Name, node: pod.Node}]
		shown := boardPod{name: pod.Name, workload: pod.Workload, newcomer: !held}
		if !pod.Placed() {
			unplaced = append(unplaced, shown)

			continue
		}
		onNode[pod.Node] = append(onNode[pod.Node], shown)
	}

	// A Topology lists its Nodes Zone by Zone, so one pass groups them.
	var zones []boardZone
	for _, node := range topology.Nodes() {
		if len(zones) == 0 || zones[len(zones)-1].name != node.Zone {
			zones = append(zones, boardZone{name: node.Zone})
		}
		last := &zones[len(zones)-1]
		last.nodes = append(last.nodes, boardNode{name: node.Name, nodePool: node.NodePool, pods: onNode[node.Name]})
	}

	return PlacementBoard{zones: zones, unplaced: unplaced}
}

func inBoardOrder(one, other Pod) int {
	return cmp.Or(
		cmp.Compare(one.Workload, other.Workload),
		cmp.Compare(ownSuffixOf(one.Name), ownSuffixOf(other.Name)),
		cmp.Compare(one.Name, other.Name))
}

// The own suffix is the part the ReplicaSet controller generates for each Pod,
// which tells one Pod of a Workload from another.
func ownSuffixOf(podName string) string {
	return podName[strings.LastIndex(podName, "-")+1:]
}

// boardNameOf is how a board and a Wait narrative name a Pod: its Workload
// name and its own suffix.
func boardNameOf(workload, podName string) string {
	return workload + "-" + ownSuffixOf(podName)
}

func (b PlacementBoard) held() map[podOnNode]struct{} {
	held := make(map[podOnNode]struct{})
	for _, zone := range b.zones {
		for _, node := range zone.nodes {
			for _, pod := range node.pods {
				held[podOnNode{pod: pod.name, node: node.name}] = struct{}{}
			}
		}
	}
	for _, pod := range b.unplaced {
		held[podOnNode{pod: pod.name}] = struct{}{}
	}

	return held
}

// Lines carry no indent of their own beyond the Nodes' under their Zone, so the
// caller prints them under its own.
func (b PlacementBoard) Lines() []string {
	nodeWidth, poolWidth := b.columnWidths()

	var lines []string
	for _, zone := range b.zones {
		lines = append(lines, zone.name)
		for _, node := range zone.nodes {
			line := fmt.Sprintf("%s%-*s%s%-*s%s%s",
				boardIndent, nodeWidth, node.name, columnGap, poolWidth, node.nodePool, columnGap, podsRead(node.pods))
			lines = append(lines, strings.TrimRight(line, " "))
		}
	}
	if len(b.unplaced) > 0 {
		lines = append(lines, pendingLabel+columnGap+podsRead(b.unplaced))
	}

	return lines
}

// Width is what a separator above or below the board spans at least.
func (b PlacementBoard) Width() int {
	widest := 0
	for _, line := range b.Lines() {
		widest = max(widest, utf8.RuneCountInString(line))
	}

	return widest
}

func (b PlacementBoard) columnWidths() (nodeWidth, poolWidth int) {
	for _, zone := range b.zones {
		for _, node := range zone.nodes {
			nodeWidth = max(nodeWidth, utf8.RuneCountInString(node.name))
			poolWidth = max(poolWidth, utf8.RuneCountInString(node.nodePool))
		}
	}

	return nodeWidth, poolWidth
}

func podsRead(pods []boardPod) string {
	read := make([]string, 0, len(pods))
	for _, pod := range pods {
		read = append(read, pod.read())
	}

	return strings.Join(read, ", ")
}

func (p boardPod) read() string {
	named := boardNameOf(p.workload, p.name)
	if p.newcomer {
		return newcomerMark + named
	}

	return named
}

// Equal is what go-cmp compares a board through, since its fields are
// unexported.
func (b PlacementBoard) Equal(other PlacementBoard) bool {
	return slices.EqualFunc(b.zones, other.zones, boardZone.equal) && slices.Equal(b.unplaced, other.unplaced)
}

func (z boardZone) equal(other boardZone) bool {
	return z.name == other.name && slices.EqualFunc(z.nodes, other.nodes, boardNode.equal)
}

func (n boardNode) equal(other boardNode) bool {
	return n.name == other.name && n.nodePool == other.nodePool && slices.Equal(n.pods, other.pods)
}
