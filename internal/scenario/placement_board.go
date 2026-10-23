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

// PlacementBoard is one observation's view of the whole Simulated Cluster
// across every Workload. It is derived from what was declared and observed, so
// it carries no invariant of its own; its zero value is the board before the
// first Wait.
type PlacementBoard struct {
	zones    []BoardZone
	unplaced []BoardPod
}

// BoardZone holds its Nodes in the Topology's order.
type BoardZone struct {
	Name  string
	Nodes []BoardNode
}

// BoardNode is a Topology Node as one observation found it. Its labels are the
// ones its NodePool states, and its Pods are in board order.
type BoardNode struct {
	Name      string
	NodePool  string
	Labels    NodeLabels
	Readiness NodeReadiness
	Capacity  NodeCapacity
	Requests  RoomTaken
	Left      RoomLeft
	Pods      []BoardPod
}

// RoomTaken is the count and the summed Requests of the Pods placed on a Node.
// A Pod declaring no Request takes no cpu and no memory.
type RoomTaken struct {
	Pods   int
	CPU    CPU
	Memory Memory
}

// RoomLeft is a Node capacity less the count and the Requests of the Pods
// placed on the Node. It may fall below zero where the observed Requests
// exceed what the Node holds, so it is no Node capacity.
type RoomLeft struct {
	Pods   int
	CPU    CPU
	Memory Memory
}

// BoardPod is one Pod as an observation found it.
type BoardPod struct {
	Name     string
	Workload string
	Ready    bool
	// Request is the zero Request where the Pod declares none.
	Request Request
	// Newcomer marks a Pod the previous board did not hold on that Node.
	Newcomer bool
}

type podOnNode struct {
	pod  string
	node string
}

// NewPlacementBoard gives every Node of the Topology its line, holding a Pod or
// not. A Pod is new where previous did not hold it on that Node, so a Pod placed
// since, or a replacement for one evicted, reads as new. A Topology Node the
// observation does not hold reads NotReady.
func NewPlacementBoard(topology Topology, observed Placement, previous PlacementBoard) PlacementBoard {
	heldBefore := previous.held()
	onNode := make(map[string][]Pod)
	var unplaced []BoardPod
	for _, pod := range slices.SortedFunc(slices.Values(observed.pods), inBoardOrder) {
		if !pod.Placed() {
			unplaced = append(unplaced, shownOnBoard(pod, heldBefore))

			continue
		}
		onNode[pod.Node] = append(onNode[pod.Node], pod)
	}

	readiness := observed.readiness()
	// A Topology lists its Nodes Zone by Zone, so one pass groups them.
	var zones []BoardZone
	for _, node := range topology.Nodes() {
		if len(zones) == 0 || zones[len(zones)-1].Name != node.Zone {
			zones = append(zones, BoardZone{Name: node.Zone})
		}
		last := &zones[len(zones)-1]
		last.Nodes = append(last.Nodes, boardNodeOf(node, cmp.Or(readiness[node.Name], NodeNotReady), onNode[node.Name], heldBefore))
	}

	return PlacementBoard{zones: zones, unplaced: unplaced}
}

func boardNodeOf(node Node, readiness NodeReadiness, placed []Pod, heldBefore map[podOnNode]struct{}) BoardNode {
	taken := RoomTaken{Pods: len(placed)}
	var shown []BoardPod
	for _, pod := range placed {
		taken.CPU += pod.Request.CPU()
		taken.Memory += pod.Request.Memory()
		shown = append(shown, shownOnBoard(pod, heldBefore))
	}
	left := RoomLeft{
		Pods: node.Capacity.Pods() - taken.Pods, CPU: node.Capacity.CPU() - taken.CPU, Memory: node.Capacity.Memory() - taken.Memory,
	}

	return BoardNode{
		Name: node.Name, NodePool: node.NodePool, Labels: node.Labels, Readiness: readiness,
		Capacity: node.Capacity, Requests: taken, Left: left, Pods: shown,
	}
}

func shownOnBoard(pod Pod, heldBefore map[podOnNode]struct{}) BoardPod {
	_, held := heldBefore[podOnNode{pod: pod.Name, node: pod.Node}]

	return BoardPod{Name: pod.Name, Workload: pod.Workload, Ready: pod.Ready, Request: pod.Request, Newcomer: !held}
}

// Zones are copies, so a reader changes no board.
func (b PlacementBoard) Zones() []BoardZone {
	zones := slices.Clone(b.zones)
	for at, zone := range zones {
		zones[at].Nodes = slices.Clone(zone.Nodes)
		for nodeAt, node := range zone.Nodes {
			zones[at].Nodes[nodeAt].Pods = slices.Clone(node.Pods)
		}
	}

	return zones
}

// Unplaced returns copies of the Pending Pods, so a reader changes no board.
func (b PlacementBoard) Unplaced() []BoardPod {
	return slices.Clone(b.unplaced)
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
		for _, node := range zone.Nodes {
			for _, pod := range node.Pods {
				held[podOnNode{pod: pod.Name, node: node.Name}] = struct{}{}
			}
		}
	}
	for _, pod := range b.unplaced {
		held[podOnNode{pod: pod.Name}] = struct{}{}
	}

	return held
}

// Lines carry no indent of their own beyond the Nodes' under their Zone, so the
// caller prints them under its own.
func (b PlacementBoard) Lines() []string {
	nodeWidth, poolWidth := b.columnWidths()

	var lines []string
	for _, zone := range b.zones {
		lines = append(lines, zone.Name)
		for _, node := range zone.Nodes {
			line := fmt.Sprintf("%s%-*s%s%-*s%s%s",
				boardIndent, nodeWidth, node.Name, columnGap, poolWidth, node.NodePool, columnGap, podsRead(node.Pods))
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
		for _, node := range zone.Nodes {
			nodeWidth = max(nodeWidth, utf8.RuneCountInString(node.Name))
			poolWidth = max(poolWidth, utf8.RuneCountInString(node.NodePool))
		}
	}

	return nodeWidth, poolWidth
}

func podsRead(pods []BoardPod) string {
	read := make([]string, 0, len(pods))
	for _, pod := range pods {
		read = append(read, pod.read())
	}

	return strings.Join(read, ", ")
}

func (p BoardPod) read() string {
	if p.Newcomer {
		return newcomerMark + p.BoardName()
	}

	return p.BoardName()
}

// BoardName is how a board and a Wait narrative name a Pod: its Workload name
// and its own suffix.
func (p BoardPod) BoardName() string {
	return boardNameOf(p.Workload, p.Name)
}

// Equal is what go-cmp compares a board through, since its fields are
// unexported.
func (b PlacementBoard) Equal(other PlacementBoard) bool {
	return slices.EqualFunc(b.zones, other.zones, BoardZone.Equal) && slices.Equal(b.unplaced, other.unplaced)
}

// Equal compares the Nodes of two Zones in order.
func (z BoardZone) Equal(other BoardZone) bool {
	return z.Name == other.Name && slices.EqualFunc(z.Nodes, other.Nodes, BoardNode.Equal)
}

// Equal compares the labels a Node carries by value.
func (n BoardNode) Equal(other BoardNode) bool {
	return n.Name == other.Name && n.NodePool == other.NodePool && n.Labels.Equal(other.Labels) &&
		n.Readiness == other.Readiness && n.Capacity == other.Capacity && n.Requests == other.Requests && n.Left == other.Left &&
		slices.Equal(n.Pods, other.Pods)
}
