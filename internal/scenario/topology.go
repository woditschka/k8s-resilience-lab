// Package scenario holds a run's description and its observed outcome as
// immutable values: the Topology a Scenario provisions, the Deployment it
// deploys, and the Placement it observes. It names no Kubernetes type; the
// mapping to the control plane's vocabulary lives in internal/cluster.
package scenario

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
)

// maxZones is a limit of the lab's own, standing until what should bound a Zone
// count is decided.
const maxZones = 26

// NodeCapacity is the room a Node has, which a Deployment can exceed by declaring
// more Pods, or asking for more cpu or memory, than its Topology has room for.
type NodeCapacity struct {
	pods   int
	cpu    CPU
	memory Memory
}

// NewNodeCapacity refuses each dimension at none: a Node always has room for a
// Pod that asks for something.
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

// NamedNodeCapacity lets a NodePool name a capacity instead of stating it. The
// package defines no name of its own.
type NamedNodeCapacity struct {
	name     string
	capacity NodeCapacity
}

// NewNamedNodeCapacity refuses the zero capacity no constructor builds, since
// it is a definition stating none.
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

func (n NamedNodeCapacity) Name() string {
	return n.name
}

func (n NamedNodeCapacity) Capacity() NodeCapacity {
	return n.capacity
}

func (c NodeCapacity) Pods() int {
	return c.pods
}

func (c NodeCapacity) CPU() CPU {
	return c.cpu
}

func (c NodeCapacity) Memory() Memory {
	return c.memory
}

// Equal is the equality go-cmp compares through, since the type's fields are
// unexported.
func (c NodeCapacity) Equal(other NodeCapacity) bool {
	return c == other
}

// Node carries no invariant of its own, since a Topology derives every Node it
// holds.
type Node struct {
	Name     string
	Zone     string
	NodePool string
	Capacity NodeCapacity
	Labels   NodeLabels
	Taints   NodeTaints
}

// Equal compares the labels and taints a Node carries by value.
func (n Node) Equal(other Node) bool {
	return n.Name == other.Name && n.Zone == other.Zone && n.NodePool == other.NodePool &&
		n.Capacity == other.Capacity && n.Labels.Equal(other.Labels) && n.Taints.Equal(other.Taints)
}

// A NodePool name and a stated label value must meet this syntax because a Node
// carries each as a label value. The domain names no Kubernetes package, so the
// rule is held here and pinned to the client library's validation by a test in
// internal/cluster.
var labelValueSyntax = regexp.MustCompile(`^[A-Za-z0-9]([-A-Za-z0-9_.]*[A-Za-z0-9])?$`)

const maxLabelValueLength = 63

// NodePool is a group of identical Nodes in every Zone, sharing one name and
// one Node capacity, as the cloud providers' node pools do.
type NodePool struct {
	name         string
	nodesPerZone int
	capacity     NodeCapacity
	labels       NodeLabels
	taints       NodeTaints
}

// NewNodePool refuses the zero capacity no constructor builds, since it is a
// NodePool stating none.
func NewNodePool(name string, nodesPerZone int, capacity NodeCapacity) (NodePool, error) {
	if nodesPerZone < 1 {
		return NodePool{}, fmt.Errorf("a Zone holds at least one Node, so a count of %d Nodes per Zone is refused",
			nodesPerZone)
	}
	if name == "" {
		return NodePool{}, errors.New("a NodePool states its name, so a NodePool without one is refused")
	}
	if !isLabelValue(name) {
		return NodePool{}, fmt.Errorf("a NodePool name is a Kubernetes label value: at most %d letters, digits, "+
			"'-', '_' or '.', opening and closing on a letter or digit, so the name %q is refused",
			maxLabelValueLength, name)
	}
	if capacity == (NodeCapacity{}) {
		return NodePool{}, errors.New("a NodePool states its Node capacity, so a NodePool without one is refused")
	}

	return NodePool{name: name, nodesPerZone: nodesPerZone, capacity: capacity}, nil
}

func (p NodePool) Labelled(added NodeLabels) (NodePool, error) {
	labels, err := p.labels.Adding(added)
	if err != nil {
		return NodePool{}, err
	}
	labelled := p
	labelled.labels = labels

	return labelled, nil
}

func (p NodePool) Labels() NodeLabels {
	return p.labels
}

func (p NodePool) Tainted(added NodeTaints) (NodePool, error) {
	taints, err := p.taints.Adding(added)
	if err != nil {
		return NodePool{}, err
	}
	tainted := p
	tainted.taints = taints

	return tainted, nil
}

func (p NodePool) Taints() NodeTaints {
	return p.taints
}

// Equal is the equality go-cmp compares through, since the type's fields are
// unexported.
func (p NodePool) Equal(other NodePool) bool {
	return p.name == other.name && p.nodesPerZone == other.nodesPerZone && p.capacity == other.capacity &&
		p.labels.Equal(other.labels) && p.taints.Equal(other.taints)
}

// Topology derives every name from positions counted from one rather than
// generating it, so the same Topology always expands to the same names: a Zone
// zone-<n>, a Node <zone>-node-<n>, counted through the Zone's NodePools.
type Topology struct {
	zones int
	pools []NodePool
}

// NewTopology refuses two NodePools under one name, because their Nodes could
// not be told apart.
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

// Nodes runs the Node index on from one NodePool to the next within a Zone.
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
					Labels:   pool.labels,
					Taints:   pool.taints,
				})
				index++
			}
		}
	}

	return nodes
}

func (t Topology) nodesPerZone() int {
	count := 0
	for _, pool := range t.pools {
		count += pool.nodesPerZone
	}

	return count
}

func (t Topology) Zones() []string {
	zones := make([]string, 0, t.zones)
	for zone := range t.zones {
		zones = append(zones, zoneNameAt(zone))
	}

	return zones
}

// LabelValues holds no value at all for a key no NodePool states.
func (t Topology) LabelValues(key string) []string {
	var values []string
	for _, pool := range t.pools {
		if value, held := pool.labels.Value(key); held && !slices.Contains(values, value) {
			values = append(values, value)
		}
	}

	return values
}

func (t Topology) HoldsNodePool(name string) bool {
	return slices.ContainsFunc(t.pools, func(pool NodePool) bool { return pool.name == name })
}

func zoneNameAt(index int) string {
	return fmt.Sprintf("zone-%d", index+1)
}

func nodeNameAt(zoneName string, index int) string {
	return fmt.Sprintf("%s-node-%d", zoneName, index+1)
}
