// Package node holds what a Node is, as a Scenario writes it: the NodePool a
// Topology line declares, its capacity, and the Kubernetes names of the labels
// and taints a Node carries. The qualifier reads as the subject: node.Pool,
// node.Capacity, node.Zone.
//
// It imports neither internal/lab nor the pod package.
package node

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

const (
	Zone     = scenario.ZoneLabel
	Hostname = scenario.HostnameLabel

	// NotReady and Unreachable are the taints the control plane gives a Node it
	// has stopped hearing from.
	NotReady    = scenario.TaintNodeNotReady
	Unreachable = scenario.TaintNodeUnreachable

	// NoSchedule is the one effect a NodePool's taint may take.
	NoSchedule = scenario.NoSchedule

	// UnresponsiveWait is in whole seconds, the unit of a toleration's
	// tolerationSeconds.
	UnresponsiveWait = int64(scenario.UnresponsiveNodeWait / time.Second)

	// Gi is the unit a capacity's Memory is written in: Memory: 64 * node.Gi.
	Gi = scenario.Memory(mebibytesPerGibibyte)
)

const (
	millicoresPerCore    = 1000
	mebibytesPerGibibyte = 1024
)

// PoolName is also the label value a NodePool's Nodes carry.
type PoolName string

type PerZone int

type CapacityName string

type Labels map[string]string

// Taint requires every field, and node.NoSchedule is the one effect a NodePool
// states.
type Taint struct {
	Key    string
	Value  string
	Effect scenario.TaintEffect
}

// Capacity requires every field, so no value deciding a Placement is the lab's
// own; a capacity leaving out a dimension is refused at the Topology line
// taking it. Only Named gives a Capacity a name.
type Capacity struct {
	CPU    int
	Memory scenario.Memory
	Pods   int

	named bool
	name  CapacityName
}

func Named(name CapacityName, capacity Capacity) Capacity {
	named := capacity
	named.named, named.name = true, name

	return named
}

func (c Capacity) cpu() scenario.CPU {
	return scenario.CPU(c.CPU * millicoresPerCore)
}

// The zero capacity states none, which the NodePool taking it refuses.
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

// Pool declares perZone identical Nodes in every Zone of a Topology. A refusal
// it carries ends the Scenario at the Topology line.
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

// DeclaredPool carries the domain value or its first refusal, which ends the
// Scenario at the Topology line. The stated name and capacity are kept beside
// the value so the line names what was written, refused or not.
type DeclaredPool struct {
	perZone  PerZone
	name     PoolName
	capacity Capacity
	labels   Labels
	taints   []Taint

	declared scenario.NodePool
	refused  error
}

// Labels may be written more than once. A refused label or a key stated in an
// earlier call becomes this NodePool's refusal, the first refusal standing.
func (p DeclaredPool) Labels(stated Labels) DeclaredPool {
	labelled := p
	labelled.labels = Labels{}
	maps.Copy(labelled.labels, p.labels)
	maps.Copy(labelled.labels, stated)
	if p.refused != nil {
		return labelled
	}

	added, err := scenario.NewNodeLabels(stated)
	if err != nil {
		labelled.refused = err

		return labelled
	}
	labelled.declared, labelled.refused = p.declared.Labelled(added)

	return labelled
}

// Taints may be written more than once. A refused taint or a key stated in an
// earlier call becomes this NodePool's refusal, the first refusal standing.
func (p DeclaredPool) Taints(stated ...Taint) DeclaredPool {
	tainted := p
	tainted.taints = slices.Concat(p.taints, stated)
	if p.refused != nil {
		return tainted
	}

	domain := make([]scenario.Taint, 0, len(stated))
	for _, taint := range stated {
		domain = append(domain, scenario.Taint(taint))
	}
	added, err := scenario.NewNodeTaints(domain...)
	if err != nil {
		tainted.refused = err

		return tainted
	}
	tainted.declared, tainted.refused = p.declared.Tainted(added)

	return tainted
}

func (p DeclaredPool) Declared() (scenario.NodePool, error) {
	return p.declared, p.refused
}

// Reads names cpu, memory and Pod count in that order, the order every reported
// capacity uses, then the labels and taints in key order.
func (p DeclaredPool) Reads() string {
	one, many := "Node", "Nodes"
	if p.capacity.name != "" {
		one, many = string(p.capacity.name)+" "+one, string(p.capacity.name)+" "+many
	}

	return fmt.Sprintf("a NodePool %s of %s holding %s cpu, %s memory and %s each%s%s", p.name,
		counted(int(p.perZone), one, many), p.capacity.cpu(), p.capacity.Memory,
		counted(p.capacity.Pods, "Pod", "Pods"), p.labelsRead(), p.taintsRead())
}

func (p DeclaredPool) labelsRead() string {
	if len(p.labels) == 0 {
		return ""
	}

	read := make([]string, 0, len(p.labels))
	for _, key := range slices.Sorted(maps.Keys(p.labels)) {
		read = append(read, key+"="+p.labels[key])
	}

	return ", labelled " + strings.Join(read, ", ")
}

func (p DeclaredPool) taintsRead() string {
	if len(p.taints) == 0 {
		return ""
	}

	read := make([]string, 0, len(p.taints))
	for _, taint := range slices.SortedStableFunc(slices.Values(p.taints), byKey) {
		read = append(read, scenario.Taint(taint).String())
	}

	return ", tainted " + strings.Join(read, ", ")
}

func byKey(a, b Taint) int {
	return strings.Compare(a.Key, b.Key)
}

func counted(count int, singular, plural string) string {
	if count == 1 {
		return "1 " + singular
	}

	return strconv.Itoa(count) + " " + plural
}
