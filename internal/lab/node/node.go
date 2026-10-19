// Package node holds what a Node is, as a Scenario writes it: the NodePool a
// Topology line declares, the capacity every Node of it holds, and the
// Kubernetes names of the labels a Node carries, the taints a NodePool states,
// and the taints it is given when it stops responding. A Scenario reaches each
// through the package name, so the qualifier reads as the subject: node.Pool,
// node.Capacity, node.Zone, per
// docs/adr/2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md.
//
// It imports neither internal/lab nor the pod package: internal/lab reads a
// declared NodePool through its exported read-only methods.
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
	// Zone is the label a Zone is a failure domain under, and Hostname the one a
	// Node is a single host under.
	Zone     = scenario.ZoneLabel
	Hostname = scenario.HostnameLabel

	// NotReady and Unreachable are the two taints the control plane gives a Node
	// it has stopped hearing from.
	NotReady    = scenario.TaintNodeNotReady
	Unreachable = scenario.TaintNodeUnreachable

	// NoSchedule is the one effect a NodePool's taint takes: it keeps every Pod
	// not tolerating it off the Node.
	NoSchedule = scenario.NoSchedule

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

// Labels are the labels a NodePool states its Nodes carry beside the lab's own,
// as Kubernetes writes a Node's labels: a map from key to value, per
// docs/adr/2026-10-18-a-nodepool-states-its-own-node-labels-beside-the-labs.md.
type Labels map[string]string

// Taint is one taint a NodePool states its Nodes carry, as Kubernetes writes a
// Node's taint: its key, its value and the effect it has on a Pod not
// tolerating it. Every field is required, and node.NoSchedule is the one effect
// a NodePool states, per
// docs/adr/2026-10-19-a-nodepool-states-noschedule-taints-and-a-later-workload-deploys-beside-the-earlier.md.
type Taint struct {
	Key    string
	Value  string
	Effect scenario.TaintEffect
}

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
	labels   Labels
	taints   []Taint

	declared scenario.NodePool
	refused  error
}

// Labels states labels this NodePool's Nodes carry beside the lab's own, and may
// be written more than once. A label the domain refuses, or a key stated in an
// earlier call, is carried to the Topology line as this NodePool's refusal, the
// first refusal standing as a refused capacity does. The labels stated are kept
// beside the value, so the line names what was written, refused or not.
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

// Taints states taints this NodePool's Nodes carry apart from their labels, and
// may be written more than once. A taint the domain refuses, or a key stated in
// an earlier call, is carried to the Topology line as this NodePool's refusal,
// the first refusal standing as a refused label does. The taints stated are kept
// beside the value, so the line names what was written, refused or not.
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

// Declared is the domain's NodePool this one declares, or the first refusal the
// domain gave it, which the Topology line taking it ends the Scenario with.
func (p DeclaredPool) Declared() (scenario.NodePool, error) {
	return p.declared, p.refused
}

// Reads names the NodePool by its name, its Node count per Zone, the name of the
// Named Node capacity its Nodes hold where it names one, and the three
// dimensions every one of them holds, cpu, memory and Pod count, the order every
// reported capacity names (docs/system-design.md). The labels it states follow
// as key=value in key order, then the taints as key=value:Effect in key order; a
// NodePool stating none names none.
func (p DeclaredPool) Reads() string {
	one, many := "Node", "Nodes"
	if p.capacity.name != "" {
		one, many = string(p.capacity.name)+" "+one, string(p.capacity.name)+" "+many
	}

	return fmt.Sprintf("a NodePool %s of %s holding %s cpu, %s memory and %s each%s%s", p.name,
		counted(int(p.perZone), one, many), p.capacity.cpu(), p.capacity.Memory,
		counted(p.capacity.Pods, "Pod", "Pods"), p.labelsRead(), p.taintsRead())
}

// labelsRead is how the labels this NodePool states read after its capacity.
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

// taintsRead is how the taints this NodePool states read after its labels.
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

// byKey orders two taints by their keys.
func byKey(a, b Taint) int {
	return strings.Compare(a.Key, b.Key)
}

// counted is a count beside the noun it counts, singular for one.
func counted(count int, singular, plural string) string {
	if count == 1 {
		return "1 " + singular
	}

	return strconv.Itoa(count) + " " + plural
}
