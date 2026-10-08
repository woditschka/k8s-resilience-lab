// Package scenario holds a run's description and its observed outcome as
// immutable values: the Topology a Scenario provisions, the Workload it deploys,
// and the Placement it observes.
//
// The package names no Kubernetes type. The mapping between this vocabulary and
// the control plane's lives in internal/cluster, per
// docs/system-design.md § Package Structure.
package scenario

import "fmt"

// maxZones is the highest Zone count a Topology admits. It is a limit of the
// lab's own: what should bound a Zone count is an open question in
// docs/prd.md, so the count admitted stands until that question is settled.
const maxZones = 26

// The capacity a declared Node holds when a Scenario names none. It leaves room
// for more than one Pod per Node, so an even Placement is the scheduler honouring
// the Workload's declared spread rather than the Nodes running out of room, and
// enough cpu and memory for every one of those Pods to ask for an ordinary share,
// so the Pod count stays the tighter bound.
const (
	defaultPodsPerNode   = 10
	defaultCPUPerNode    = CPU(16 * millicoresPerCore)
	defaultMemoryPerNode = Memory(64 * mebibytesPerNextUnit)
)

// NodeCapacity is what one Node can hold: the Pods it has room for, and the cpu
// and memory the scheduler fits their requests against. A Workload can therefore
// declare more Pods, or ask for more of either, than its Topology has room for.
type NodeCapacity struct {
	pods   int
	cpu    CPU
	memory Memory
}

// NewNodeCapacity builds the capacity of one Node, refusing each dimension at
// none: a Node always has room for a Pod that asks for something.
func NewNodeCapacity(pods int, cpu CPU, memory Memory) (NodeCapacity, error) {
	if pods < 1 {
		return NodeCapacity{}, fmt.Errorf("a Node holds at least one Pod, so a capacity of %d Pods is refused", pods)
	}
	if cpu < 1 {
		return NodeCapacity{}, fmt.Errorf("a Node has some cpu, so a capacity of %s cpu is refused", cpu)
	}
	if memory < 1 {
		return NodeCapacity{}, fmt.Errorf("a Node has some memory, so a capacity of %s memory is refused", memory)
	}

	return NodeCapacity{pods: pods, cpu: cpu, memory: memory}, nil
}

// DefaultNodeCapacity is the whole capacity a Node holds where its Topology
// states none. It is built from the package's own literals, every one of them
// above what NewNodeCapacity refuses, so it needs no error of its own.
func DefaultNodeCapacity() NodeCapacity {
	return NodeCapacity{pods: defaultPodsPerNode, cpu: defaultCPUPerNode, memory: defaultMemoryPerNode}
}

// Pods is how many Pods one Node with this capacity holds.
func (c NodeCapacity) Pods() int {
	return c.pods
}

// CPU is how much processor time one Node with this capacity has.
func (c NodeCapacity) CPU() CPU {
	return c.cpu
}

// Memory is how much memory one Node with this capacity has.
func (c NodeCapacity) Memory() Memory {
	return c.memory
}

// Equal reports whether two capacities hold the same of every dimension. It is
// the equality go-cmp compares through, since the type's fields are unexported.
func (c NodeCapacity) Equal(other NodeCapacity) bool {
	return c == other
}

// Node is one simulated machine a Pod can be placed on, named after the Zone it
// belongs to and its position within that Zone. A Topology derives every Node it
// holds, so the type carries no invariant of its own.
type Node struct {
	Name     string
	Zone     string
	Capacity NodeCapacity
}

// Topology is the arrangement of Zones and Nodes a Scenario provisions. The same
// Topology always expands to the same Zone and Node names, because every name is
// derived from positions counted from one rather than generated: a Zone zone-<n>,
// a Node <zone>-node-<n>.
type Topology struct {
	zones        int
	nodesPerZone int
	capacity     NodeCapacity
}

// NewTopology builds the Topology a Scenario provisions, refusing a count it
// could not provision and a Zone count past the lab's own limit.
func NewTopology(zones, nodesPerZone int, capacity NodeCapacity) (Topology, error) {
	if zones < 1 {
		return Topology{}, fmt.Errorf("a Topology holds at least one Zone, so a count of %d Zones is refused", zones)
	}
	if zones > maxZones {
		return Topology{}, fmt.Errorf("the lab's limit is %d Zones, so a count of %d Zones is refused", maxZones, zones)
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

// Zones are the Zones the Topology holds, by name and in the order it derives
// them. A Zone is named once however many Nodes it holds, so it is the Zone axis
// a spread and a Failure are read over.
func (t Topology) Zones() []string {
	zones := make([]string, 0, t.zones)
	for zone := range t.zones {
		zones = append(zones, zoneNameAt(zone))
	}

	return zones
}

// zoneNameAt is the name of the Zone at index, numbered from one.
func zoneNameAt(index int) string {
	return fmt.Sprintf("zone-%d", index+1)
}

// nodeNameAt is the name of the Node at index within zoneName, numbered from one.
func nodeNameAt(zoneName string, index int) string {
	return fmt.Sprintf("%s-node-%d", zoneName, index+1)
}
