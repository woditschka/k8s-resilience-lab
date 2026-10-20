package scenario

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// NodeFailure resolves its Nodes from the Topology at construction, since a
// Scenario states how many Nodes fail rather than which.
type NodeFailure struct {
	nodes []Node
}

// NewNodeFailure resolves the same count to the same Nodes on every run, since
// the Topology derives its names in a fixed order. A count it cannot satisfy is
// refused, naming the Nodes it does hold.
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

func (f NodeFailure) Names() []string {
	return namesOf(f.nodes)
}

// Equal is the equality go-cmp compares through, since the type's field is
// unexported.
func (f NodeFailure) Equal(other NodeFailure) bool {
	return slices.EqualFunc(f.nodes, other.nodes, Node.Equal)
}

// LabelFailure resolves its Nodes from the observation at construction, since a
// Scenario states which label fails rather than which Nodes.
type LabelFailure struct {
	nodes []Node
}

// NewLabelFailure takes the Nodes in Topology order, so the same label resolves
// to the same Nodes on every run. It refuses a literal of other than one label,
// a key or value no NodePool states, and a value no observed Node still
// carries, since a Failure stops at least one Node. A key no NodePool states
// covers every key the lab owns, since a NodePool cannot state one. Each
// refusal names what the Scenario stated.
func NewLabelFailure(topology Topology, observed Placement, stated map[string]string) (LabelFailure, error) {
	named := slices.Sorted(maps.Keys(stated))
	if len(named) != 1 {
		return LabelFailure{}, fmt.Errorf("a Failure by label names one label, and %d were named: [%s]",
			len(named), strings.Join(named, ", "))
	}
	key := named[0]
	value := stated[key]

	values := topology.LabelValues(key)
	if len(values) == 0 {
		return LabelFailure{}, fmt.Errorf("a Failure by label names a label a NodePool of the Topology states, "+
			"and none states %q", key)
	}
	if !slices.Contains(values, value) {
		return LabelFailure{}, fmt.Errorf("a Failure by label names a value a NodePool states, and the NodePools "+
			"state %s for the label %q, so the value %q is refused", strings.Join(values, ", "), key, value)
	}

	carrying := carryingLabel(topology.Nodes(), observed.Nodes(), key, value)
	if len(carrying) == 0 {
		return LabelFailure{}, fmt.Errorf("a Failure stops at least one Node, and no Node still responding "+
			"carries the label %q with the value %q", key, value)
	}

	return LabelFailure{nodes: carrying}, nil
}

func (f LabelFailure) Names() []string {
	return namesOf(f.nodes)
}

// Equal is the equality go-cmp compares through, since the type's field is
// unexported.
func (f LabelFailure) Equal(other LabelFailure) bool {
	return slices.EqualFunc(f.nodes, other.nodes, Node.Equal)
}

// A Node is judged by what the observation read from it, so a label the control
// plane dropped is not carried.
func carryingLabel(derived, observed []Node, key, value string) []Node {
	var carrying []Node
	for _, node := range derived {
		seen := slices.IndexFunc(observed, func(o Node) bool { return o.Name == node.Name })
		if seen < 0 {
			continue
		}
		if held, labelled := observed[seen].Labels.Value(key); labelled && held == value {
			carrying = append(carrying, observed[seen])
		}
	}

	return carrying
}

// ZoneFailure carries both the Zones a report names and the Nodes a Scenario
// stops, since a Zone stops responding by every Node in it stopping.
type ZoneFailure struct {
	zones []string
	nodes []Node
}

// NewZoneFailure resolves the same count to the same Zones on every run, since
// the Topology derives its names in a fixed order. A count it cannot satisfy is
// refused, naming the Zones it does hold.
func NewZoneFailure(topology Topology, count int) (ZoneFailure, error) {
	derived := topology.Zones()
	if count < 1 {
		return ZoneFailure{}, fmt.Errorf("a Failure stops at least one Zone, so a count of %d Zones is refused", count)
	}
	if count > len(derived) {
		return ZoneFailure{}, fmt.Errorf("the Topology holds the %d Zones %s, so a Failure of %d Zones is refused",
			len(derived), strings.Join(derived, ", "), count)
	}
	stopping := derived[:count]

	return ZoneFailure{zones: slices.Clone(stopping), nodes: nodesIn(topology, stopping)}, nil
}

func (f ZoneFailure) Names() []string {
	return slices.Clone(f.zones)
}

func (f ZoneFailure) NodeNames() []string {
	return namesOf(f.nodes)
}

// Equal is the equality go-cmp compares through, since the type's fields are
// unexported.
func (f ZoneFailure) Equal(other ZoneFailure) bool {
	return slices.Equal(f.zones, other.zones) && slices.EqualFunc(f.nodes, other.nodes, Node.Equal)
}

func nodesIn(topology Topology, zones []string) []Node {
	var held []Node
	for _, node := range topology.Nodes() {
		if slices.Contains(zones, node.Zone) {
			held = append(held, node)
		}
	}

	return held
}

func namesOf(nodes []Node) []string {
	names := make([]string, 0, len(nodes))
	for _, node := range nodes {
		names = append(names, node.Name)
	}

	return names
}
