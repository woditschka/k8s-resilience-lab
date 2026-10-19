package node_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/lab/node"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// Every name the node package offers a Scenario is the domain's own spelling,
// which internal/scenario's mapping test pins to the client library's constant,
// so a Scenario writing node.Zone writes the label Kubernetes reads.
func TestTheNodeNames_ShouldBeTheDomainsOwnSpelling(t *testing.T) {
	expect.Expect(t, []string{node.Zone, node.Hostname, node.NotReady, node.Unreachable}).To(expect.Equal([]string{
		scenario.ZoneLabel, scenario.HostnameLabel, scenario.TaintNodeNotReady, scenario.TaintNodeUnreachable,
	}))
	expect.Expect(t, time.Duration(node.UnresponsiveWait)*time.Second).To(expect.Equal(scenario.UnresponsiveNodeWait))
	expect.Expect(t, node.Gi.String()).To(expect.Equal("1Gi"))
}

// A Node capacity's fields are the resource names Kubernetes reports a Node's
// room under (docs/adr/2026-10-18-pod-and-node-settings-are-keyed-literals-refused-at-their-given.md).
func TestANodeCapacitysFields_ShouldBeTheResourceNamesKubernetesReportsANodesRoomUnder(t *testing.T) {
	expect.Expect(t, exportedFieldsOf(reflect.TypeFor[node.Capacity]())).To(expect.Equal([]string{
		string(corev1.ResourceCPU), string(corev1.ResourceMemory), string(corev1.ResourcePods),
	}))
}

// exportedFieldsOf is every field a keyed literal of that type writes, spelled
// as Kubernetes spells a resource name.
func exportedFieldsOf(written reflect.Type) []string {
	var fields []string
	for field := range written.Fields() {
		if field.IsExported() {
			fields = append(fields, strings.ToLower(field.Name))
		}
	}

	return fields
}

// A NodePool states its name, its Node count per Zone and a capacity of all three
// dimensions, and the Topology line taking it ends the Scenario with the first
// refusal the domain gave it.
func TestANodePool_ShouldDeclareTheNodePoolItStatesOrTheFirstRefusalOfTheDomain(t *testing.T) {
	complete := node.Capacity{CPU: 4, Memory: 16 * node.Gi, Pods: 10}
	tests := map[string]struct {
		pool    node.DeclaredPool
		refused bool
	}{
		"a complete capacity":         {pool: node.Pool("general", 2, complete)},
		"a Named capacity":            {pool: node.Pool("general", 2, node.Named("medium", complete))},
		"no capacity":                 {pool: node.Pool("general", 2, node.Capacity{}), refused: true},
		"a capacity without cpu":      {pool: node.Pool("general", 2, node.Capacity{Memory: 16 * node.Gi, Pods: 10}), refused: true},
		"a capacity without memory":   {pool: node.Pool("general", 2, node.Capacity{CPU: 4, Pods: 10}), refused: true},
		"a capacity without Pods":     {pool: node.Pool("general", 2, node.Capacity{CPU: 4, Memory: 16 * node.Gi}), refused: true},
		"a Named capacity of no name": {pool: node.Pool("general", 2, node.Named("", complete)), refused: true},
		"no Node per Zone":            {pool: node.Pool("general", 0, complete), refused: true},
		"no name":                     {pool: node.Pool("", 2, complete), refused: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := test.pool.Declared()

			expect.Expect(t, err != nil).To(expect.Equal(test.refused))
		})
	}
}

func TestANodePoolDeclared_ShouldBeTheDomainsNodePoolOfTheNameTheCountAndTheCapacityStated(t *testing.T) {
	capacity, err := scenario.NewNodeCapacity(10, 4*1000, 16*node.Gi)
	expect.Expect(t, err).To(expect.Equal[error](nil))
	want, err := scenario.NewNodePool("general", 2, capacity)
	expect.Expect(t, err).To(expect.Equal[error](nil))

	declared, err := node.Pool("general", 2, node.Named("medium", node.Capacity{CPU: 4, Memory: 16 * node.Gi, Pods: 10})).Declared()

	expect.Expect(t, err).To(expect.Equal[error](nil))
	expect.Expect(t, declared).To(expect.Equal(want))
}

func TestANodePoolRead_ShouldNameItsCountItsCapacityAndTheNameOfTheCapacityWhereItHasOne(t *testing.T) {
	capacity := node.Capacity{CPU: 4, Memory: 16 * node.Gi, Pods: 1}
	tests := map[string]struct {
		pool  node.DeclaredPool
		reads string
	}{
		"a stated capacity": {
			pool:  node.Pool("general", 2, capacity),
			reads: "a NodePool general of 2 Nodes holding 4 cpu, 16Gi memory and 1 Pod each",
		},
		"a Named capacity": {
			pool:  node.Pool("general", 1, node.Named("medium", capacity)),
			reads: "a NodePool general of 1 medium Node holding 4 cpu, 16Gi memory and 1 Pod each",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			expect.Expect(t, test.pool.Reads()).To(expect.Equal(test.reads))
		})
	}
}

// The label a test of a labelled NodePool states, the two values it states under
// it, and a key and value no test turns on.
const (
	capacityType = "capacity-type"
	spot         = "spot"
	onDemand     = "on-demand"
	someKey      = "some-key"
	someValue    = "some-value"
)

// aPool is the named default for a NodePool a labelled test adds its labels to.
func aPool() node.DeclaredPool {
	return node.Pool("spot", 1, node.Capacity{CPU: 4, Memory: 16 * node.Gi, Pods: 10})
}

// A NodePool states its own labels alone and each once, under keys and values
// Kubernetes holds; each refusal is carried to the Topology line, which ends the
// Scenario with it, and names what was refused (docs/prd.md REQ-SIM-017 edge
// cases 1 and 2). A refused NodePool stays refused whatever is labelled after.
func TestARefusedNodeLabel_ShouldBeRefusedWhereItIsStated(t *testing.T) {
	tests := map[string]struct {
		pool    node.DeclaredPool
		refused string
	}{
		"the Zone label": {pool: aPool().Labels(node.Labels{node.Zone: someValue}), refused: node.Zone},
		"one key in two calls": {
			pool:    aPool().Labels(node.Labels{capacityType: spot}).Labels(node.Labels{capacityType: onDemand}),
			refused: capacityType,
		},
		"a label after a refused capacity": {
			pool:    node.Pool("spot", 1, node.Capacity{}).Labels(node.Labels{capacityType: spot}),
			refused: "Node capacity",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := test.pool.Declared()

			expect.Expect(t, err != nil && strings.Contains(err.Error(), test.refused)).To(expect.Equal(true))
		})
	}
}

// A NodePool labelled in two calls declares the domain's NodePool holding both.
func TestANodePoolLabelledInTwoCalls_ShouldDeclareTheDomainsNodePoolHoldingBothLabels(t *testing.T) {
	both, err := scenario.NewNodeLabels(map[string]string{capacityType: spot, someKey: someValue})
	expect.Expect(t, err).To(expect.Equal[error](nil))
	unlabelled, err := aPool().Declared()
	expect.Expect(t, err).To(expect.Equal[error](nil))
	want, err := unlabelled.Labelled(both)
	expect.Expect(t, err).To(expect.Equal[error](nil))

	declared, err := aPool().Labels(node.Labels{capacityType: spot}).Labels(node.Labels{someKey: someValue}).Declared()

	expect.Expect(t, err).To(expect.Equal[error](nil))
	expect.Expect(t, declared).To(expect.Equal(want))
}

// A NodePool's line reports its stated labels after its capacity, in key order
// whichever call stated them, and a NodePool stating none reads as before.
func TestALabelledNodePoolRead_ShouldNameItsLabelsAfterItsCapacityInKeyOrder(t *testing.T) {
	labelled := aPool().Labels(node.Labels{someKey: someValue}).Labels(node.Labels{capacityType: spot})

	expect.Expect(t, labelled.Reads()).To(expect.Equal(
		"a NodePool spot of 1 Node holding 4 cpu, 16Gi memory and 10 Pods each, labelled capacity-type=spot, some-key=some-value"))
}

// The taint a test states on a dedicated NodePool, and the effect it takes.
const (
	gpuKey     = "nvidia.com/gpu"
	gpuPresent = "present"
)

// aGPUTaint is the named default for the one taint a tainted test states.
func aGPUTaint() node.Taint {
	return node.Taint{Key: gpuKey, Value: gpuPresent, Effect: node.NoSchedule}
}

// A taint is Kubernetes' own type under its own name, its fields Kubernetes'
// names, and its one effect the domain's spelling, so a Scenario's literal reads
// as the manifest it becomes.
func TestATaint_ShouldBeTheKubernetesTypeOfItsNameUnderTheDomainsSpelling(t *testing.T) {
	kubernetes := reflect.TypeFor[corev1.Taint]()
	var missing []string
	for field := range reflect.TypeFor[node.Taint]().Fields() {
		if _, held := kubernetes.FieldByName(field.Name); !held {
			missing = append(missing, field.Name)
		}
	}

	expect.Expect(t, reflect.TypeFor[node.Taint]().Name()).To(expect.Equal(kubernetes.Name()))
	expect.Expect(t, missing).To(expect.HaveLen[string](0))
	expect.Expect(t, node.NoSchedule).To(expect.Equal(scenario.NoSchedule))
}

// A NodePool tainted in two calls declares the domain's NodePool holding both
// taints.
func TestANodePoolTaintedInTwoCalls_ShouldDeclareTheDomainsNodePoolHoldingBothTaints(t *testing.T) {
	other := node.Taint{Key: someKey, Value: someValue, Effect: node.NoSchedule}
	both, err := scenario.NewNodeTaints(scenario.Taint(aGPUTaint()), scenario.Taint(other))
	expect.Expect(t, err).To(expect.Equal[error](nil))
	untainted, err := aPool().Declared()
	expect.Expect(t, err).To(expect.Equal[error](nil))
	want, err := untainted.Tainted(both)
	expect.Expect(t, err).To(expect.Equal[error](nil))

	declared, err := aPool().Taints(aGPUTaint()).Taints(other).Declared()

	expect.Expect(t, err).To(expect.Equal[error](nil))
	expect.Expect(t, declared).To(expect.Equal(want))
}

// A taint the domain refuses, or a key stated in an earlier call, is carried to
// the Topology line, the first refusal standing.
func TestARefusedTaint_ShouldBeRefusedWhereItIsStated(t *testing.T) {
	const noExecute = scenario.NoExecute
	tests := map[string]struct {
		pool    node.DeclaredPool
		refused string
	}{
		"a taint of another effect": {
			pool:    aPool().Taints(node.Taint{Key: gpuKey, Value: gpuPresent, Effect: noExecute}),
			refused: string(noExecute),
		},
		"one key in two calls": {
			pool:    aPool().Taints(aGPUTaint()).Taints(node.Taint{Key: gpuKey, Value: someValue, Effect: node.NoSchedule}),
			refused: gpuKey,
		},
		"a taint after a refused label": {
			pool:    aPool().Labels(node.Labels{node.Zone: someValue}).Taints(aGPUTaint()),
			refused: node.Zone,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := test.pool.Declared()

			expect.Expect(t, err != nil && strings.Contains(err.Error(), test.refused)).To(expect.Equal(true))
		})
	}
}

// A NodePool's line reports its stated taints after its labels, as kubectl
// writes one, in key order whichever call stated them.
func TestATaintedNodePoolRead_ShouldNameItsTaintsAfterItsLabelsInKeyOrder(t *testing.T) {
	tainted := aPool().Taints(aGPUTaint()).Labels(node.Labels{capacityType: spot}).
		Taints(node.Taint{Key: someKey, Value: someValue, Effect: node.NoSchedule})

	expect.Expect(t, tainted.Reads()).To(expect.Equal(
		"a NodePool spot of 1 Node holding 4 cpu, 16Gi memory and 10 Pods each, labelled capacity-type=spot, " +
			"tainted nvidia.com/gpu=present:NoSchedule, some-key=some-value:NoSchedule"))
}
