// Package scenario holds a run's description and its observed outcome as
// immutable values: the Topology a Scenario provisions, the Workload it deploys,
// and the Placement it observes.
//
// The package names no Kubernetes type. The mapping between this vocabulary and
// the control plane's lives in internal/cluster, per
// docs/system-design.md § Package Structure.
package scenario

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
)

// maxZones is the highest Zone count a Topology admits. It is a limit of the
// lab's own: what should bound a Zone count is an open question in
// docs/prd.md, so the count admitted stands until that question is settled.
const maxZones = 26

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

// NamedNodeCapacity is a Node capacity a Scenario defines once under a name of
// its own, so a NodePool names it instead of stating its cpu, memory and Pod
// count. The package defines no name and holds no capacity of its own, per
// docs/adr/2026-10-09-every-nodepool-states-or-names-its-node-capacity.md.
type NamedNodeCapacity struct {
	name     string
	capacity NodeCapacity
}

// NewNamedNodeCapacity defines capacity under name, refusing an empty name and
// the zero capacity no constructor builds, which is a definition stating none.
func NewNamedNodeCapacity(name string, capacity NodeCapacity) (NamedNodeCapacity, error) {
	if name == "" {
		return NamedNodeCapacity{}, errors.New("a Named Node capacity states its name, so one without a name is refused")
	}
	if capacity == (NodeCapacity{}) {
		return NamedNodeCapacity{}, fmt.Errorf("a Named Node capacity states its Node capacity, "+
			"so %q without one is refused", name)
	}

	return NamedNodeCapacity{name: name, capacity: capacity}, nil
}

// Name is the name a NodePool names this capacity by.
func (n NamedNodeCapacity) Name() string {
	return n.name
}

// Capacity is the cpu, memory and Pod count this name stands for.
func (n NamedNodeCapacity) Capacity() NodeCapacity {
	return n.capacity
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
// belongs to and its position within that Zone, and holding the name of the
// NodePool it belongs to. A Topology derives every Node it holds, so the type
// carries no invariant of its own.
type Node struct {
	Name     string
	Zone     string
	NodePool string
	Capacity NodeCapacity
}

// nodePoolNameSyntax is Kubernetes' label-value syntax, which a NodePool name
// must meet because a Node carries that name as a label value. The domain names
// no Kubernetes package, so the rule is held here and pinned to the client
// library's own validation by a test in internal/cluster, per
// docs/adr/2026-10-09-every-nodepool-is-named-where-it-is-declared.md.
var nodePoolNameSyntax = regexp.MustCompile(`^[A-Za-z0-9]([-A-Za-z0-9_.]*[A-Za-z0-9])?$`)

// maxNodePoolNameLength is the longest label value Kubernetes holds.
const maxNodePoolNameLength = 63

// NodePool is a group of identical Nodes every Zone of a Topology holds: how many
// each Zone holds, the capacity each of them has, and the name its Nodes carry.
type NodePool struct {
	name         string
	nodesPerZone int
	capacity     NodeCapacity
}

// NewNodePool builds one NodePool, refusing a count of no Node, an empty name, a
// name no Kubernetes label value can hold, and the zero capacity no constructor
// builds, which is a NodePool stating none.
func NewNodePool(name string, nodesPerZone int, capacity NodeCapacity) (NodePool, error) {
	if nodesPerZone < 1 {
		return NodePool{}, fmt.Errorf("a Zone holds at least one Node, so a count of %d Nodes per Zone is refused",
			nodesPerZone)
	}
	if name == "" {
		return NodePool{}, errors.New("a NodePool states its name, so a NodePool without one is refused")
	}
	if len(name) > maxNodePoolNameLength || !nodePoolNameSyntax.MatchString(name) {
		return NodePool{}, fmt.Errorf("a NodePool name is a Kubernetes label value: at most %d letters, digits, "+
			"'-', '_' or '.', opening and closing on a letter or digit, so the name %q is refused",
			maxNodePoolNameLength, name)
	}
	if capacity == (NodeCapacity{}) {
		return NodePool{}, errors.New("a NodePool states its Node capacity, so a NodePool without one is refused")
	}

	return NodePool{name: name, nodesPerZone: nodesPerZone, capacity: capacity}, nil
}

// Topology is the arrangement of Zones and Nodes a Scenario provisions: the
// Zones, and the NodePools every one of them holds in declaration order. The same
// Topology always expands to the same Zone and Node names, because every name is
// derived from positions counted from one rather than generated: a Zone
// zone-<n>, a Node <zone>-node-<n>, counted through the Zone's NodePools.
type Topology struct {
	zones int
	pools []NodePool
}

// NewTopology builds the Topology a Scenario provisions, refusing a Zone count
// it could not provision or past the lab's own limit, a Topology of no
// NodePool, and two NodePools whose Nodes could not be told apart: two under
// one name.
func NewTopology(zones int, pools ...NodePool) (Topology, error) {
	if zones < 1 {
		return Topology{}, fmt.Errorf("a Topology holds at least one Zone, so a count of %d Zones is refused", zones)
	}
	if zones > maxZones {
		return Topology{}, fmt.Errorf("the lab's limit is %d Zones, so a count of %d Zones is refused", maxZones, zones)
	}
	if len(pools) == 0 {
		return Topology{}, errors.New("a Zone holds at least one NodePool, so a Topology of none is refused")
	}
	named := make(map[string]bool, len(pools))
	for _, pool := range pools {
		if named[pool.name] {
			return Topology{}, fmt.Errorf("the Nodes of two NodePools named %q could not be told apart, "+
				"so the second is refused", pool.name)
		}
		named[pool.name] = true
	}

	return Topology{zones: zones, pools: slices.Clone(pools)}, nil
}

// Nodes expands the Topology to every Node it holds, Zone by Zone, and within a
// Zone through its NodePools in declaration order, the index running on from one
// NodePool to the next.
func (t Topology) Nodes() []Node {
	nodes := make([]Node, 0, t.zones*t.nodesPerZone())
	for zone := range t.zones {
		zoneName := zoneNameAt(zone)
		index := 0
		for _, pool := range t.pools {
			for range pool.nodesPerZone {
				nodes = append(nodes, Node{
					Name:     nodeNameAt(zoneName, index),
					Zone:     zoneName,
					NodePool: pool.name,
					Capacity: pool.capacity,
				})
				index++
			}
		}
	}

	return nodes
}

// nodesPerZone is how many Nodes each Zone holds over all its NodePools.
func (t Topology) nodesPerZone() int {
	count := 0
	for _, pool := range t.pools {
		count += pool.nodesPerZone
	}

	return count
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
