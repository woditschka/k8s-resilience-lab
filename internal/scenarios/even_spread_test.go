// The tests in this file drive the Even spread Scenario's own declarations and
// expectations, which are decisions over immutable scenario values and reach no
// cluster. The Scenario's cluster-driving steps are driven by the engine-backed
// tests of cmd/lab, per docs/testing-principles.md § Seams. They sit in the
// package rather than beside it because the declarations are the Scenario's own
// and no other package calls them.
package scenarios

import (
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

const (
	// twoZones and twoNodesPerZone shape a Topology small enough for a reader to
	// hold every derived name at once, and still hold more than one Node per Zone,
	// which is what makes naming each Zone once a decision rather than a copy.
	twoZones        = 2
	twoNodesPerZone = 2

	// oneZone and oneNodePerZone shape the smallest Topology there is.
	oneZone        = 1
	oneNodePerZone = 1

	// podsPerDomain is the share an even expectation gives each domain.
	podsPerDomain = 3

	// someDomainName and anotherDomainName are the domains an even expectation is
	// spread over. It counts Pods per name and reads no meaning into the name, so
	// these stand for any two.
	someDomainName    = "some-domain"
	anotherDomainName = "another-domain"
)

// aTopology is the Topology of the stated shape, with a capacity no test below
// turns on.
func aTopology(t *testing.T, zones, nodesPerZone int) scenario.Topology {
	t.Helper()

	capacity, err := scenario.NewNodeCapacity(evenSpreadNodeCapacity)
	if err != nil {
		t.Fatalf("build a Node capacity of %d Pods: %v", evenSpreadNodeCapacity, err)
	}
	topology, err := scenario.NewTopology(zones, nodesPerZone, capacity)
	if err != nil {
		t.Fatalf("build a Topology of %d Zones of %d Nodes: %v", zones, nodesPerZone, err)
	}

	return topology
}

// spreadRule is the rule over domain at maxSkew, which is what the expected
// Workload below is built from.
func spreadRule(t *testing.T, domain scenario.SpreadDomain, maxSkew int) scenario.SpreadRule {
	t.Helper()

	rule, err := scenario.NewSpreadRule(domain, maxSkew)
	if err != nil {
		t.Fatalf("build a spread rule over the %s at a Skew of %d: %v", domain, maxSkew, err)
	}

	return rule
}

func TestTheEvenSpreadWorkload_ShouldDeclareItsPodsEvenlyOverZonesAndNodes(t *testing.T) {
	nodes := len(aTopology(t, evenSpreadZones, evenSpreadNodesPerZone).Nodes())

	declared, err := evenlySpreadWorkload(nodes)
	if err != nil {
		t.Fatalf("declare the Even spread Workload over %d Nodes: %v", nodes, err)
	}

	expected, err := scenario.NewWorkload(evenSpreadWorkloadName, nodes,
		spreadRule(t, scenario.SpreadOverZones, evenSkew),
		spreadRule(t, scenario.SpreadOverNodes, evenSkew),
	)
	if err != nil {
		t.Fatalf("build the Workload the Scenario is expected to declare: %v", err)
	}
	expect.Expect(t, declared).To(expect.Equal(expected))
}

func TestAWorkloadOfNoPods_ShouldBeRefusedBeforeAnyClusterIsReached(t *testing.T) {
	const noNodes = 0

	_, err := evenlySpreadWorkload(noNodes)

	if err == nil {
		t.Fatal("a Workload of no Pods was declared, and the Scenario would deploy it")
	}
}

func TestEachZoneOfATopology_ShouldBeNamedOnceInZoneOrder(t *testing.T) {
	topology := aTopology(t, twoZones, twoNodesPerZone)
	nodes := topology.Nodes()
	firstZone, lastZone := nodes[0].Zone, nodes[len(nodes)-1].Zone

	named := zonesOf(topology)

	expect.Expect(t, named).To(expect.Equal([]string{firstZone, lastZone}))
}

func TestEveryNodeOfATopology_ShouldBeNamedInTopologyOrder(t *testing.T) {
	topology := aTopology(t, oneZone, twoNodesPerZone)
	nodes := topology.Nodes()

	named := nodesOf(topology)

	expect.Expect(t, named).To(expect.Equal([]string{nodes[0].Name, nodes[1].Name}))
}

func TestAnEvenExpectation_ShouldGiveEveryDomainTheSameShare(t *testing.T) {
	expected := evenlyOver([]string{someDomainName, anotherDomainName}, podsPerDomain)

	expect.Expect(t, expected).To(expect.Equal(map[string]int{
		someDomainName:    podsPerDomain,
		anotherDomainName: podsPerDomain,
	}))
}

func TestTheSmallestTopology_ShouldExpectEveryPodInItsOneZoneOnItsOneNode(t *testing.T) {
	topology := aTopology(t, oneZone, oneNodePerZone)
	onlyNode := topology.Nodes()[0]

	expect.Expect(t, evenlyOver(zonesOf(topology), podsPerDomain)).To(expect.Equal(map[string]int{onlyNode.Zone: podsPerDomain}))
	expect.Expect(t, evenlyOver(nodesOf(topology), podsPerDomain)).To(expect.Equal(map[string]int{onlyNode.Name: podsPerDomain}))
}
