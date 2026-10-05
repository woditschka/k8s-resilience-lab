// Package scenario holds a run's description and its observed outcome as
// immutable values: the Topology a Scenario provisions, the Workload it deploys,
// and the Placement it observes.
//
// The package names no Kubernetes type. The mapping between this vocabulary and
// the control plane's lives in internal/cluster, per
// docs/system-design.md § Package Structure.
package scenario

import "fmt"

// maxZones is the highest Zone count a Topology admits: Zones are lettered from
// zone-a, so the alphabet bounds how many a Topology can name.
const maxZones = 'z' - 'a' + 1

// DefaultNodeCapacity is what a declared Node holds when a Scenario names no
// capacity. It leaves room for more than one Pod per Node, so an even Placement
// is the scheduler honouring the Workload's declared spread rather than the
// Nodes running out of room.
const DefaultNodeCapacity = 10

// NodeCapacity is what one Node can hold, so a Workload can declare more Pods
// than its Topology has room for.
type NodeCapacity struct {
	pods int
}

// NewNodeCapacity builds the capacity of one Node, refusing a Pod count no Node
// could hold.
func NewNodeCapacity(pods int) (NodeCapacity, error) {
	if pods < 1 {
		return NodeCapacity{}, fmt.Errorf("a Node holds at least one Pod, so a capacity of %d Pods is refused", pods)
	}

	return NodeCapacity{pods: pods}, nil
}

// Pods is how many Pods one Node with this capacity holds.
func (c NodeCapacity) Pods() int {
	return c.pods
}

// Equal reports whether two capacities hold the same number of Pods. It is the
// equality go-cmp compares through, since the type's field is unexported.
func (c NodeCapacity) Equal(other NodeCapacity) bool {
	return c.pods == other.pods
}

// Node is one simulated machine a Pod can be placed on, named after the Zone it
// belongs to and its index within that Zone. A Topology derives every Node it
// holds, so the type carries no invariant of its own.
type Node struct {
	Name     string
	Zone     string
	Capacity NodeCapacity
}

// Topology is the arrangement of Zones and Nodes a Scenario provisions. The same
// Topology always expands to the same Zone and Node names, because every name is
// derived from the Zone's letter and the Node's index rather than generated.
type Topology struct {
	zones        int
	nodesPerZone int
	capacity     NodeCapacity
}

// NewTopology builds the Topology a Scenario provisions, refusing a count it
// could not provision or could not name.
func NewTopology(zones, nodesPerZone int, capacity NodeCapacity) (Topology, error) {
	if zones < 1 {
		return Topology{}, fmt.Errorf("a Topology holds at least one Zone, so a count of %d Zones is refused", zones)
	}
	if zones > maxZones {
		return Topology{}, fmt.Errorf("a Zone is lettered from zone-a, so a Topology names at most %d Zones and a count of %d is refused", maxZones, zones)
	}
	if nodesPerZone < 1 {
		return Topology{}, fmt.Errorf("a Zone holds at least one Node, so a count of %d Nodes per Zone is refused", nodesPerZone)
	}

	return Topology{zones: zones, nodesPerZone: nodesPerZone, capacity: capacity}, nil
}

// Nodes expands the Topology to every Node it holds, Zone by Zone and in index
// order within each Zone.
func (t Topology) Nodes() []Node {
	nodes := make([]Node, 0, t.zones*t.nodesPerZone)
	for zone := range t.zones {
		zoneName := zoneNameAt(zone)
		for node := range t.nodesPerZone {
			nodes = append(nodes, Node{
				Name:     nodeNameAt(zoneName, node),
				Zone:     zoneName,
				Capacity: t.capacity,
			})
		}
	}

	return nodes
}

// zoneNameAt is the name of the Zone at index, lettered from zone-a.
func zoneNameAt(index int) string {
	return fmt.Sprintf("zone-%c", 'a'+rune(index))
}

// nodeNameAt is the name of the Node at index within zoneName, numbered from one.
func nodeNameAt(zoneName string, index int) string {
	return fmt.Sprintf("%s-node-%d", zoneName, index+1)
}
