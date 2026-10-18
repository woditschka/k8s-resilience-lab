// Package node holds what a Node is, as a Scenario writes it: the NodePool a
// Topology line declares, the capacity every Node of it holds, and the
// Kubernetes names of the labels a Node carries and the taints it is given when
// it stops responding. A Scenario reaches each through the package name, so the
// qualifier reads as the subject: node.Pool, node.Capacity, node.Zone, per
// docs/adr/2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md.
//
// It imports neither internal/lab nor the pod package: internal/lab reads a
// declared NodePool through its exported read-only methods.
package node

import (
	"fmt"
	"strconv"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

const (
	// Zone is the label a Zone is a failure domain under, and Hostname the one a
	// Node is a single host under.
	Zone     = scenario.ZoneLabel
	Hostname = scenario.HostnameLabel

	// NotReady and Unreachable are the two taints the control plane gives a Node
	// it has stopped hearing from.
	NotReady    = scenario.TaintNodeNotReady
	Unreachable = scenario.TaintNodeUnreachable

	// UnresponsiveWait is the lab's waiting time before a Pod leaves a Node that
	// stopped responding, in the whole seconds a toleration's tolerationSeconds
	// holds it in.
	UnresponsiveWait = int64(scenario.UnresponsiveNodeWait / time.Second)

	// Gi is one gibibyte, the unit a Node capacity's Memory is written in, as a
	// manifest writes 64Gi: Memory: 64 * node.Gi.
	Gi = scenario.Memory(mebibytesPerGibibyte)
)

// The lab's own quantities hold a cpu in millicores and a memory in mebibytes.
const (
	millicoresPerCore    = 1000
	mebibytesPerGibibyte = 1024
)

// PoolName is the name a NodePool's Nodes carry as a label.
type PoolName string

// PerZone is how many Nodes a NodePool holds in each Zone of a Topology.
type PerZone int

// CapacityName is the name a Scenario defines a Named Node capacity under, which
// a NodePool's line reports its Nodes by.
type CapacityName string

// Capacity is the room every Node of a NodePool holds, under the resource names
// Kubernetes reports it by: CPU in whole cores, Memory in node.Gi, and the Pods
// it has room for. Every field is required, so no value deciding a Placement is
// the lab's own: a capacity leaving out a dimension is refused by the domain at
// the Topology line taking it, and the zero capacity as stating none, per
// docs/adr/2026-10-18-pod-and-node-settings-are-keyed-literals-refused-at-their-given.md.
//
// The name it was defined under is unexported, so only Named gives one.
type Capacity struct {
	CPU    int
	Memory scenario.Memory
	Pods   int

	named bool
	name  CapacityName
}

// Named defines a Named Node capacity: the capacity a NodePool's Nodes hold under
// a name a Scenario gives it. The lab defines no name of its own: a Scenario's
// place defines each one once, per
// docs/adr/2026-10-09-every-nodepool-states-or-names-its-node-capacity.md.
func Named(name CapacityName, capacity Capacity) Capacity {
	named := capacity
	named.named, named.name = true, name

	return named
}

// cpu is the capacity's cpu as the lab's own quantity holds it.
func (c Capacity) cpu() scenario.CPU {
	return scenario.CPU(c.CPU * millicoresPerCore)
}

// declared is the domain's capacity of this one, or the first refusal the domain
// gave it. The zero capacity states none, which the NodePool taking it refuses.
func (c Capacity) declared() (scenario.NodeCapacity, error) {
	if c == (Capacity{}) {
		return scenario.NodeCapacity{}, nil
	}
	stated, err := scenario.NewNodeCapacity(c.Pods, c.cpu(), c.Memory)
	if err != nil || !c.named {
		return stated, err
	}
	named, err := scenario.NewNamedNodeCapacity(string(c.name), stated)
	if err != nil {
		return scenario.NodeCapacity{}, err
	}

	return named.Capacity(), nil
}

// Pool declares one NodePool every Zone of a Topology holds: perZone identical
// Nodes under name, which they carry as a label, each holding the capacity the
// NodePool states or names, per
// docs/adr/2026-10-09-every-nodepool-states-or-names-its-node-capacity.md. It is
// written as an argument of the Topology line, which ends the Scenario with any
// refusal the NodePool carries.
func Pool(name PoolName, perZone PerZone, capacity Capacity) DeclaredPool {
	declared := DeclaredPool{name: name, perZone: perZone, capacity: capacity}
	stated, err := capacity.declared()
	if err != nil {
		declared.refused = err

		return declared
	}
	declared.declared, declared.refused = scenario.NewNodePool(string(name), int(perZone), stated)

	return declared
}

// DeclaredPool is one NodePool as a Topology line states it, complete as it
// stands (docs/architecture-principles.md § Construction and update).
//
// It carries the immutable domain value, or the first refusal the domain gave
// it: the Topology line is where the refusal ends the Scenario, as a refused Pod
// specification is carried to its Workload line. The stated name and capacity
// are kept beside the value so the line names what was written, refused or not.
type DeclaredPool struct {
	perZone  PerZone
	name     PoolName
	capacity Capacity

	declared scenario.NodePool
	refused  error
}

// Declared is the domain's NodePool this one declares, or the first refusal the
// domain gave it, which the Topology line taking it ends the Scenario with.
func (p DeclaredPool) Declared() (scenario.NodePool, error) {
	return p.declared, p.refused
}

// Reads names the NodePool by its name, its Node count per Zone, the name of the
// Named Node capacity its Nodes hold where it names one, and the three
// dimensions every one of them holds, cpu, memory and Pod count, the order every
// reported capacity names (docs/system-design.md).
func (p DeclaredPool) Reads() string {
	one, many := "Node", "Nodes"
	if p.capacity.name != "" {
		one, many = string(p.capacity.name)+" "+one, string(p.capacity.name)+" "+many
	}

	return fmt.Sprintf("a NodePool %s of %s holding %s cpu, %s memory and %s each", p.name,
		counted(int(p.perZone), one, many), p.capacity.cpu(), p.capacity.Memory,
		counted(p.capacity.Pods, "Pod", "Pods"))
}

// counted is a count beside the noun it counts, singular for one.
func counted(count int, singular, plural string) string {
	if count == 1 {
		return "1 " + singular
	}

	return strconv.Itoa(count) + " " + plural
}
