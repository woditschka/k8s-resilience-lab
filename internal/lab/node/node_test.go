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
	expect.Expect(t, declared == want).To(expect.Equal(true))
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
