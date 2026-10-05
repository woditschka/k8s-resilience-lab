package scenario

import (
	"fmt"
	"slices"
	"strings"
)

// NodeFailure is the Nodes one Failure stops responding, resolved at construction
// from the count a Scenario stated. A Scenario states how many Nodes fail rather
// than which, so the Nodes are the Topology's to derive and this value's to hold.
type NodeFailure struct {
	nodes []Node
}

// NewNodeFailure builds the Failure that stops count Nodes of the Topology: the
// first ones it derives. A Topology derives its own names in a fixed order, so
// the same count resolves to the same Nodes on every run.
//
// A count the Topology cannot satisfy is refused and named, with the Nodes it
// does hold, because that count is what a reader has to change.
func NewNodeFailure(topology Topology, count int) (NodeFailure, error) {
	derived := topology.Nodes()
	if count < 1 {
		return NodeFailure{}, fmt.Errorf("a Failure stops at least one Node, so a count of %d Nodes is refused", count)
	}
	if count > len(derived) {
		return NodeFailure{}, fmt.Errorf("the Topology holds the %d Nodes %s, so a Failure of %d Nodes is refused",
			len(derived), strings.Join(namesOf(derived), ", "), count)
	}

	return NodeFailure{nodes: slices.Clone(derived[:count])}, nil
}

// Names are the Nodes this Failure stops, by name and in the order the Topology
// derived them.
func (f NodeFailure) Names() []string {
	return namesOf(f.nodes)
}

// Equal reports whether two Failures stop the same Nodes. It is the equality
// go-cmp compares through, since the type's field is unexported.
func (f NodeFailure) Equal(other NodeFailure) bool {
	return slices.Equal(f.nodes, other.nodes)
}

// namesOf are the names of these Nodes, in the order they are held.
func namesOf(nodes []Node) []string {
	names := make([]string, 0, len(nodes))
	for _, node := range nodes {
		names = append(names, node.Name)
	}

	return names
}
