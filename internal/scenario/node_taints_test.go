package scenario_test

import (
	"strings"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// The taint a test states where it turns on a stated taint: a key and value a
// dedicated NodePool of gpu Nodes carries. someTaint is the other key a test states.
const (
	gpuKey     = "nvidia.com/gpu"
	gpuPresent = "present"
)

// taintsOf is the Node taints stated, which a test building taints the domain
// admits states.
func taintsOf(t *testing.T, stated ...scenario.Taint) scenario.NodeTaints {
	t.Helper()

	taints, err := scenario.NewNodeTaints(stated...)
	if err != nil {
		t.Fatalf("build the Node taints %v: %v", stated, err)
	}

	return taints
}

// A NodePool's taints are read in key order whatever order they were stated in,
// so a run reports them alike every time.
func TestNodeTaints_ShouldBeReadInKeyOrder(t *testing.T) {
	taints := taintsOf(t,
		scenario.Taint{Key: gpuKey, Value: gpuPresent, Effect: scenario.NoSchedule},
		scenario.Taint{Key: someTaint, Value: someValue, Effect: scenario.NoSchedule},
	)

	expect.Expect(t, taints.Taints()).To(expect.Equal([]scenario.Taint{
		{Key: someTaint, Value: someValue, Effect: scenario.NoSchedule},
		{Key: gpuKey, Value: gpuPresent, Effect: scenario.NoSchedule},
	}))
}

// A NodePool states NoSchedule taints alone, every field of each, under a key and
// value Kubernetes holds, and under no key the control plane sets on a Node it
// stopped hearing from. One key is stated once.
func TestNodeTaints_ShouldRefuseATaintTheLabCannotHold(t *testing.T) {
	const noValue = ""
	gpu := scenario.Taint{Key: gpuKey, Value: gpuPresent, Effect: scenario.NoSchedule}
	with := func(change func(*scenario.Taint)) scenario.Taint {
		changed := gpu
		change(&changed)

		return changed
	}
	tests := map[string]struct {
		stated  []scenario.Taint
		refused string
	}{
		"a NoSchedule taint":              {stated: []scenario.Taint{gpu}},
		"two taints of different keys":    {stated: []scenario.Taint{gpu, with(func(t *scenario.Taint) { t.Key = someTaint })}},
		"no taint at all":                 {stated: nil},
		"a taint leaving out its key":     {stated: []scenario.Taint{with(func(t *scenario.Taint) { t.Key = noValue })}, refused: "leaves out its key"},
		"a taint leaving out its value":   {stated: []scenario.Taint{with(func(t *scenario.Taint) { t.Value = noValue })}, refused: "leaves out its value"},
		"a taint leaving out its effect":  {stated: []scenario.Taint{with(func(t *scenario.Taint) { t.Effect = noValue })}, refused: "leaves out its effect"},
		"a PreferNoSchedule taint":        {stated: []scenario.Taint{with(func(t *scenario.Taint) { t.Effect = "PreferNoSchedule" })}, refused: "PreferNoSchedule"},
		"a NoExecute taint":               {stated: []scenario.Taint{with(func(t *scenario.Taint) { t.Effect = scenario.NoExecute })}, refused: "NoExecute"},
		"a key with a space":              {stated: []scenario.Taint{with(func(t *scenario.Taint) { t.Key = "nvidia gpu" })}, refused: "nvidia gpu"},
		"a key of two prefixes":           {stated: []scenario.Taint{with(func(t *scenario.Taint) { t.Key = "a/b/c" })}, refused: "a/b/c"},
		"a value opening on a dash":       {stated: []scenario.Taint{with(func(t *scenario.Taint) { t.Value = "-present" })}, refused: "-present"},
		"a value that is too long":        {stated: []scenario.Taint{with(func(t *scenario.Taint) { t.Value = strings.Repeat("v", 64) })}, refused: strings.Repeat("v", 64)},
		"one key stated twice":            {stated: []scenario.Taint{gpu, with(func(t *scenario.Taint) { t.Value = someValue })}, refused: gpuKey},
		"the not-ready taint":             {stated: []scenario.Taint{with(func(t *scenario.Taint) { t.Key = scenario.TaintNodeNotReady })}, refused: scenario.TaintNodeNotReady},
		"the unreachable taint":           {stated: []scenario.Taint{with(func(t *scenario.Taint) { t.Key = scenario.TaintNodeUnreachable })}, refused: scenario.TaintNodeUnreachable},
		"a key under the control plane's": {stated: []scenario.Taint{with(func(t *scenario.Taint) { t.Key = "node.kubernetes.io/out-of-service" })}, refused: "node.kubernetes.io/out-of-service"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := scenario.NewNodeTaints(test.stated...)

			expect.Expect(t, refusalNaming(err, test.refused)).To(expect.Equal(test.refused != ""))
		})
	}
}

// Taints added to taints already stated hold both, and a key stated in both is
// refused naming that key.
func TestNodeTaintsAdded_ShouldHoldBothAndRefuseAKeyStatedTwice(t *testing.T) {
	gpu := scenario.Taint{Key: gpuKey, Value: gpuPresent, Effect: scenario.NoSchedule}
	other := scenario.Taint{Key: someTaint, Value: someValue, Effect: scenario.NoSchedule}
	stated := taintsOf(t, gpu)

	added, err := stated.Adding(taintsOf(t, other))
	expect.Expect(t, err).To(expect.Equal[error](nil))
	expect.Expect(t, added).To(expect.Equal(taintsOf(t, gpu, other)))

	_, err = stated.Adding(taintsOf(t, scenario.Taint{Key: gpuKey, Value: someValue, Effect: scenario.NoSchedule}))
	expect.Expect(t, refusalNaming(err, gpuKey)).To(expect.Equal(true))
}

// A NodePool's Nodes carry its taints and the other NodePools' Nodes none, and
// the Topology names the NodePools it holds.
func TestATopologyOfATaintedNodePool_ShouldTaintItsNodesAlone(t *testing.T) {
	const oneZone, oneNodePerZone, gpu = 1, 1, "gpu"
	gpuTaints := taintsOf(t, scenario.Taint{Key: gpuKey, Value: gpuPresent, Effect: scenario.NoSchedule})
	tainted, err := aNodePool(t, gpu, oneNodePerZone, aCapacity(t)).Tainted(gpuTaints)
	if err != nil {
		t.Fatalf("taint the NodePool %s: %v", gpu, err)
	}

	topology, err := scenario.NewTopology(oneZone, aNodePool(t, theGeneralNodePool, oneNodePerZone, aCapacity(t)), tainted)

	expect.Expect(t, err).To(expect.Equal[error](nil))
	expect.Expect(t, topology.Nodes()).To(expect.Equal([]scenario.Node{
		{Name: "zone-1-node-1", Zone: "zone-1", NodePool: theGeneralNodePool, Capacity: aCapacity(t)},
		{Name: "zone-1-node-2", Zone: "zone-1", NodePool: gpu, Capacity: aCapacity(t), Taints: gpuTaints},
	}))
	expect.Expect(t, topology.HoldsNodePool(gpu)).To(expect.Equal(true))
	expect.Expect(t, topology.HoldsNodePool(someValue)).To(expect.Equal(false))
}

// A NodePool tainted twice holds both taints, and refuses a key it already holds.
func TestANodePoolTaintedTwice_ShouldHoldBothAndRefuseAKeyItAlreadyHolds(t *testing.T) {
	const oneNodePerZone = 1
	gpu := scenario.Taint{Key: gpuKey, Value: gpuPresent, Effect: scenario.NoSchedule}
	other := scenario.Taint{Key: someTaint, Value: someValue, Effect: scenario.NoSchedule}
	pool, err := aNodePool(t, theGeneralNodePool, oneNodePerZone, aCapacity(t)).Tainted(taintsOf(t, gpu))
	if err != nil {
		t.Fatalf("taint the NodePool: %v", err)
	}

	twice, err := pool.Tainted(taintsOf(t, other))
	expect.Expect(t, err).To(expect.Equal[error](nil))
	expect.Expect(t, twice.Taints()).To(expect.Equal(taintsOf(t, gpu, other)))

	_, err = pool.Tainted(taintsOf(t, gpu))
	expect.Expect(t, refusalNaming(err, gpuKey)).To(expect.Equal(true))
}

// Two Nodes or two NodePools alike in all else are equal exactly when their
// taints are, so comparing Placements or Topologies notices a taint.
func TestTwoValuesAlikeButForTheirTaints_ShouldBeEqualOnlyWhenTheirTaintsAre(t *testing.T) {
	const oneNodePerZone = 1
	gpuTaints := taintsOf(t, scenario.Taint{Key: gpuKey, Value: gpuPresent, Effect: scenario.NoSchedule})
	untainted := aNodePool(t, theGeneralNodePool, oneNodePerZone, aCapacity(t))
	tainted, err := untainted.Tainted(gpuTaints)
	if err != nil {
		t.Fatalf("taint the NodePool: %v", err)
	}
	node := scenario.Node{Name: "zone-1-node-1", Zone: "zone-1", NodePool: theGeneralNodePool, Capacity: aCapacity(t)}
	taintedNode := node
	taintedNode.Taints = gpuTaints

	expect.Expect(t, []bool{
		untainted.Equal(tainted), tainted.Equal(tainted), node.Equal(taintedNode), taintedNode.Equal(taintedNode),
	}).To(expect.Equal([]bool{false, true, false, true}))
}
