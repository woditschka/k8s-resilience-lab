package scenario_test

import (
	"strings"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// The Node labels a test states where it turns on a stated label: a key outside
// every key the lab owns, the two values a NodePool of on-demand and one of spot
// state under it, and some key and value no test turns on.
const (
	capacityType = "capacity-type"
	onDemand     = "on-demand"
	spot         = "spot"
	someKey      = "some-key"
	someValue    = "some-value"
)

// labelsOf is the Node labels stated as that map, which a test building labels
// the domain admits states.
func labelsOf(t *testing.T, stated map[string]string) scenario.NodeLabels {
	t.Helper()

	labels, err := scenario.NewNodeLabels(stated)
	if err != nil {
		t.Fatalf("build the Node labels %v: %v", stated, err)
	}

	return labels
}

// A NodePool's labels are read in key order whatever order they were stated in,
// so a run reports them alike every time.
func TestNodeLabels_ShouldBeReadInKeyOrder(t *testing.T) {
	labels := labelsOf(t, map[string]string{someKey: someValue, capacityType: spot})

	expect.Expect(t, labels.Pairs()).To(expect.Equal([]scenario.NodeLabel{
		{Key: capacityType, Value: spot},
		{Key: someKey, Value: someValue},
	}))
}

// A NodePool may state its own keys alone: the lab writes the Zone, hostname,
// NodePool and simulator selection labels on every Node, and owns every key under
// its prefix. A key or value Kubernetes cannot hold as a label is refused, an
// empty value being one it holds (docs/prd.md REQ-SIM-017 edge case 1).
func TestNodeLabels_ShouldRefuseAKeyTheLabOwnsAndOneKubernetesCannotHold(t *testing.T) {
	const noValue = ""
	tests := map[string]struct {
		stated  map[string]string
		refused string
	}{
		"a key and a value Kubernetes holds": {stated: map[string]string{capacityType: spot}},
		"a key under a prefix":               {stated: map[string]string{"example.com/" + capacityType: spot}},
		"an empty value":                     {stated: map[string]string{capacityType: noValue}},
		"no label at all":                    {stated: map[string]string{}},
		"the Zone label":                     {stated: map[string]string{scenario.ZoneLabel: someValue}, refused: scenario.ZoneLabel},
		"the hostname label":                 {stated: map[string]string{scenario.HostnameLabel: someValue}, refused: scenario.HostnameLabel},
		"the NodePool label":                 {stated: map[string]string{scenario.NodePoolLabel: someValue}, refused: scenario.NodePoolLabel},
		"the simulator's selection label":    {stated: map[string]string{scenario.SimulatorSelectionLabel: someValue}, refused: scenario.SimulatorSelectionLabel},
		"a key under the lab's prefix":       {stated: map[string]string{"k8s-resilience-lab/rack": someValue}, refused: "k8s-resilience-lab/rack"},
		"an empty key":                       {stated: map[string]string{"": someValue}, refused: `""`},
		"a key of two prefixes":              {stated: map[string]string{"a/b/c": someValue}, refused: "a/b/c"},
		"a key with a space":                 {stated: map[string]string{"capacity type": someValue}, refused: "capacity type"},
		"a key of an upper-case prefix":      {stated: map[string]string{"Example.com/x": someValue}, refused: "Example.com/x"},
		"a key whose name is too long":       {stated: map[string]string{strings.Repeat("k", 64): someValue}, refused: strings.Repeat("k", 64)},
		"a value with a space":               {stated: map[string]string{capacityType: "on demand"}, refused: "on demand"},
		"a value opening on a dash":          {stated: map[string]string{capacityType: "-spot"}, refused: "-spot"},
		"a value that is too long":           {stated: map[string]string{capacityType: strings.Repeat("v", 64)}, refused: strings.Repeat("v", 64)},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := scenario.NewNodeLabels(test.stated)

			expect.Expect(t, refusalNaming(err, test.refused)).To(expect.Equal(test.refused != ""))
		})
	}
}

// refusalNaming reports whether err is a refusal naming what was refused.
func refusalNaming(err error, refused string) bool {
	return err != nil && strings.Contains(err.Error(), refused)
}

// Labels added to labels already stated hold both, and a key stated in both is
// refused naming that key (docs/prd.md REQ-SIM-017 edge case 2).
func TestNodeLabelsAdded_ShouldHoldBothAndRefuseAKeyStatedTwice(t *testing.T) {
	stated := labelsOf(t, map[string]string{capacityType: spot})

	added, err := stated.Adding(labelsOf(t, map[string]string{someKey: someValue}))
	expect.Expect(t, err).To(expect.Equal[error](nil))
	expect.Expect(t, added).To(expect.Equal(labelsOf(t, map[string]string{capacityType: spot, someKey: someValue})))

	_, err = stated.Adding(labelsOf(t, map[string]string{capacityType: onDemand}))
	expect.Expect(t, refusalNaming(err, capacityType)).To(expect.Equal(true))
}

func TestNodeLabels_ShouldReadTheValueOfAKeyTheyHoldAndNoneOfOneTheyDoNot(t *testing.T) {
	labels := labelsOf(t, map[string]string{capacityType: spot})

	value, held := labels.Value(capacityType)
	expect.Expect(t, value).To(expect.Equal(spot))
	expect.Expect(t, held).To(expect.Equal(true))
	_, held = labels.Value(someKey)
	expect.Expect(t, held).To(expect.Equal(false))
}

// A NodePool's Nodes carry its labels, and the Topology names the values its
// NodePools state for a key once each, in declaration order, a NodePool stating
// no value of that key adding none.
func TestATopologyOfLabelledNodePools_ShouldLabelTheirNodesAndNameEachValueStatedOnce(t *testing.T) {
	const oneZone, oneNodePerZone = 1, 1
	onDemandLabels := labelsOf(t, map[string]string{capacityType: onDemand})
	spotLabels := labelsOf(t, map[string]string{capacityType: spot})
	topology, err := scenario.NewTopology(oneZone,
		labelledNodePool(t, onDemand, onDemandLabels),
		labelledNodePool(t, "more-on-demand", onDemandLabels),
		labelledNodePool(t, spot, spotLabels),
		aNodePool(t, theGeneralNodePool, oneNodePerZone, aCapacity(t)),
	)
	if err != nil {
		t.Fatalf("build a Topology of labelled NodePools: %v", err)
	}

	expect.Expect(t, topology.LabelValues(capacityType)).To(expect.Equal([]string{onDemand, spot}))
	expect.Expect(t, topology.LabelValues(someKey)).To(expect.HaveLen[string](0))
	expect.Expect(t, topology.Nodes()).To(expect.Equal([]scenario.Node{
		{Name: "zone-1-node-1", Zone: "zone-1", NodePool: onDemand, Capacity: aCapacity(t), Labels: onDemandLabels},
		{Name: "zone-1-node-2", Zone: "zone-1", NodePool: "more-on-demand", Capacity: aCapacity(t), Labels: onDemandLabels},
		{Name: "zone-1-node-3", Zone: "zone-1", NodePool: spot, Capacity: aCapacity(t), Labels: spotLabels},
		{Name: "zone-1-node-4", Zone: "zone-1", NodePool: theGeneralNodePool, Capacity: aCapacity(t)},
	}))
}

// A NodePool labelled twice holds both labels, and refuses a key it already holds.
func TestANodePoolLabelledTwice_ShouldHoldBothAndRefuseAKeyItAlreadyHolds(t *testing.T) {
	const oneNodePerZone = 1
	pool := labelledNodePool(t, spot, labelsOf(t, map[string]string{capacityType: spot}))

	twice, err := pool.Labelled(labelsOf(t, map[string]string{someKey: someValue}))
	expect.Expect(t, err).To(expect.Equal[error](nil))
	expect.Expect(t, twice.Labels()).To(expect.Equal(labelsOf(t, map[string]string{capacityType: spot, someKey: someValue})))

	_, err = pool.Labelled(labelsOf(t, map[string]string{capacityType: onDemand}))
	expect.Expect(t, refusalNaming(err, capacityType)).To(expect.Equal(true))
	expect.Expect(t, aNodePool(t, spot, oneNodePerZone, aCapacity(t)).Labels()).To(expect.Equal(scenario.NodeLabels{}))
}

// labelledNodePool is a NodePool of one Node per Zone under name, carrying the
// labels named.
func labelledNodePool(t *testing.T, name string, labels scenario.NodeLabels) scenario.NodePool {
	t.Helper()

	const oneNodePerZone = 1
	labelled, err := aNodePool(t, name, oneNodePerZone, aCapacity(t)).Labelled(labels)
	if err != nil {
		t.Fatalf("label the NodePool %s: %v", name, err)
	}

	return labelled
}

// Two Nodes, two NodePools or two sets of Node labels alike in all else are equal
// exactly when their labels are, so comparing Placements or Topologies notices a
// label the control plane added or dropped (docs/prd.md REQ-SIM-017).
func TestTwoValuesAlikeButForTheirNodeLabels_ShouldBeEqualOnlyWhenTheirLabelsAre(t *testing.T) {
	capacity := aCapacity(t)
	nodes := func(left, right scenario.NodeLabels) bool {
		node := scenario.Node{Name: "zone-1-node-1", Zone: "zone-1", NodePool: spot, Capacity: capacity}
		leftNode, rightNode := node, node
		leftNode.Labels, rightNode.Labels = left, right

		return leftNode.Equal(rightNode)
	}
	nodePools := func(left, right scenario.NodePool) bool {
		return left.Equal(right)
	}
	pool := aNodePool(t, spot, 1, capacity)
	labelled := func(labels scenario.NodeLabels) scenario.NodePool {
		t.Helper()

		labelledPool, err := pool.Labelled(labels)
		if err != nil {
			t.Fatalf("label the NodePool with %v: %v", labels.Pairs(), err)
		}

		return labelledPool
	}
	setsOfLabels := func(left, right scenario.NodeLabels) bool {
		return left.Equal(right)
	}
	spotLabels := labelsOf(t, map[string]string{capacityType: spot})
	onDemandLabels := labelsOf(t, map[string]string{capacityType: onDemand})
	spotLabelsAgain := labelsOf(t, map[string]string{capacityType: spot})

	tests := map[string]struct {
		equal bool
		want  bool
	}{
		"two Nodes differing in their labels alone": {
			equal: nodes(spotLabels, onDemandLabels),
			want:  false,
		},
		"two Nodes of the same labels": {
			equal: nodes(spotLabels, spotLabelsAgain),
			want:  true,
		},
		"two NodePools differing in their labels alone": {
			equal: nodePools(labelled(spotLabels), labelled(onDemandLabels)),
			want:  false,
		},
		"two NodePools of the same labels": {
			equal: nodePools(labelled(spotLabels), labelled(spotLabelsAgain)),
			want:  true,
		},
		"two sets of Node labels differing in a value": {
			equal: setsOfLabels(spotLabels, onDemandLabels),
			want:  false,
		},
		"two sets of the same Node labels": {
			equal: setsOfLabels(spotLabels, spotLabelsAgain),
			want:  true,
		},
	}

	for compared, taken := range tests {
		t.Run(compared, func(t *testing.T) {
			expect.Expect(t, taken.equal).To(expect.Equal(taken.want))
		})
	}
}
