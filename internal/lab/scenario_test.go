// The tests in this file drive the Step surface a Scenario's lines run against.
// They reach no cluster and no engine: a Given is judged by the immutable
// internal/scenario value it constructs, and a Then by a Placement the test
// built, so the surface is driven over real values alone and no stand-in for the
// Simulated Cluster exists (docs/testing-principles.md § Mocking Policy). They
// sit in the package because a Scenario's declarations and its recorded Steps are
// what the Runner beside them reads.
package lab

import (
	"fmt"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

const (
	// twoZones and twoNodesPerZone shape a Topology small enough for a reader to
	// hold every derived name at once, and still hold more than one Node per Zone,
	// which is what makes an even Placement a decision rather than the only one
	// available.
	twoZones        = 2
	twoNodesPerZone = 2

	// podsOverEveryNode is the Workload the Thens below judge: one Pod for each
	// Node of that Topology.
	podsOverEveryNode = twoZones * twoNodesPerZone

	// podsPerZoneWhenEven and podsPerNodeWhenEven are the shares an even Placement
	// of that Workload gives each domain.
	podsPerZoneWhenEven = podsOverEveryNode / twoZones
	podsPerNodeWhenEven = 1

	// twoPodsPerNode is a Node capacity a Scenario names outright: below the
	// default, so a Topology declared at it differs from one that named none.
	twoPodsPerNode = 2

	// noNodesPerZone, noPods and noSuchSpreadDomain are each a declaration the
	// internal/scenario constructor refuses, so the Given declaring one is a Step
	// that cannot be carried out.
	noNodesPerZone                           = 0
	noPods                                   = 0
	noSuchSpreadDomain scenario.SpreadDomain = "Racks"

	// someWorkloadName is the name a Workload's Pods are labelled with. A Then
	// counts Pods per domain and reads no meaning into the name, so it stands for
	// any. It is named here rather than taken from the Scenarios' place, which
	// internal/lab does not import.
	someWorkloadName WorkloadName = "some-workload"
)

// aScenario is a Scenario whose Steps declare alone. No Placement has been
// observed and none is needed: the Givens below reach no cluster.
func aScenario() *Scenario {
	return &Scenario{}
}

// aScenarioOver is a Scenario that has already settled at placed, which is the
// world every Then judges in.
func aScenarioOver(placed scenario.Placement) *Scenario {
	return &Scenario{observed: placed, settled: true}
}

// aTopologyOf is the Topology of the stated shape, at the capacity a Given
// declares when a Scenario names none.
func aTopologyOf(t *testing.T, zones, nodesPerZone int) scenario.Topology {
	t.Helper()

	return aTopologyAt(t, zones, nodesPerZone, scenario.DefaultNodeCapacity)
}

// aTopologyAt is the Topology of the stated shape with every Node holding
// podsPerNode Pods, which is the capacity a Given names outright.
func aTopologyAt(t *testing.T, zones, nodesPerZone, podsPerNode int) scenario.Topology {
	t.Helper()

	capacity, err := scenario.NewNodeCapacity(podsPerNode)
	if err != nil {
		t.Fatalf("build a Node capacity of %d Pods: %v", podsPerNode, err)
	}
	topology, err := scenario.NewTopology(zones, nodesPerZone, capacity)
	if err != nil {
		t.Fatalf("build a Topology of %d Zones of %d Nodes: %v", zones, nodesPerZone, err)
	}

	return topology
}

// anEvenlySpreadWorkloadOf is the Workload a Given declares when it spreads pods
// evenly over the Zones and over the Nodes.
func anEvenlySpreadWorkloadOf(t *testing.T, pods int) scenario.Workload {
	t.Helper()

	overZones, err := scenario.NewSpreadRule(OverZones, evenSkew)
	if err != nil {
		t.Fatalf("build a spread rule over the Zones: %v", err)
	}
	overNodes, err := scenario.NewSpreadRule(OverNodes, evenSkew)
	if err != nil {
		t.Fatalf("build a spread rule over the Nodes: %v", err)
	}
	workload, err := scenario.NewWorkload(string(someWorkloadName), pods, overZones, overNodes)
	if err != nil {
		t.Fatalf("build a Workload of %d Pods spread evenly: %v", pods, err)
	}

	return workload
}

// onePodPerNodeOf is a Placement of one Running Pod on every Node of topology,
// which is the even Placement the Thens below are met by.
func onePodPerNodeOf(topology scenario.Topology) scenario.Placement {
	nodes := topology.Nodes()
	placed := make([]scenario.Pod, 0, len(nodes))
	for index, node := range nodes {
		placed = append(placed, scenario.Pod{
			Name:  fmt.Sprintf("%s-%d", someWorkloadName, index),
			Node:  node.Name,
			Phase: scenario.PodRunning,
		})
	}

	return scenario.NewPlacement(nodes, placed)
}

// everyPodOnOneNodeOf is a Placement of the same Pods crowded onto the first Node
// of topology, which no even Then is met by.
func everyPodOnOneNodeOf(topology scenario.Topology) scenario.Placement {
	nodes := topology.Nodes()
	crowded := onePodPerNodeOf(topology).Pods()
	for index := range crowded {
		crowded[index].Node = nodes[0].Name
	}

	return scenario.NewPlacement(nodes, crowded)
}

// topologyStepOf, workloadStepOf and the Then Steps below are the Steps the
// Scenarios in this file record. A Step's name is what a report shows and what a
// Finding points at, so this suite states it.
func topologyStepOf(zones, nodesPerZone int) Step {
	return Step{Kind: GivenStep, Name: fmt.Sprintf("a Topology of %d Zones of %d Nodes", zones, nodesPerZone)}
}

// topologyStepHoldingOf is the Step a Topology line records once it names the
// capacity its Nodes hold, which renames the line rather than adding one.
func topologyStepHoldingOf(zones, nodesPerZone, podsPerNode int) Step {
	return Step{
		Kind: GivenStep,
		Name: fmt.Sprintf("a Topology of %d Zones of %d Nodes holding %d Pods each", zones, nodesPerZone, podsPerNode),
	}
}

func workloadStepOf(pods int) Step {
	return Step{Kind: GivenStep, Name: fmt.Sprintf("a Workload %s of %d Pods", someWorkloadName, pods)}
}

// spreadWorkloadStepOf is the Step a Workload line records once it spreads its
// Pods over one domain alone.
func spreadWorkloadStepOf(pods int, domain scenario.SpreadDomain) Step {
	return Step{
		Kind: GivenStep,
		Name: fmt.Sprintf("a Workload %s of %d Pods spread evenly over the %s", someWorkloadName, pods, domain),
	}
}

func evenlySpreadWorkloadStepOf(pods int) Step {
	return Step{
		Kind: GivenStep,
		Name: fmt.Sprintf("a Workload %s of %d Pods spread evenly over the Zones and over the Nodes", someWorkloadName, pods),
	}
}

func allPodsRunningStep() Step {
	return Step{Kind: ThenStep, Name: "every Pod Running"}
}

func podsPerZoneStepOf(pods int) Step {
	return Step{Kind: ThenStep, Name: fmt.Sprintf("%d Pods per Zone", pods)}
}

func podsPerNodeStepOf(pods int) Step {
	return Step{Kind: ThenStep, Name: fmt.Sprintf("%d Pod per Node", pods)}
}

// judgedFinding is one Finding as this suite reads it: the Step it names, and
// whether it carried the mismatch. The mismatch text is what internal/expect
// reported and is not this suite's to pin.
type judgedFinding struct {
	Step        Step
	Mismatching bool
}

// asJudged is every Finding reported, read that way.
func asJudged(findings []Finding) []judgedFinding {
	judged := make([]judgedFinding, 0, len(findings))
	for _, finding := range findings {
		judged = append(judged, judgedFinding{Step: finding.Step, Mismatching: finding.Mismatch != ""})
	}

	return judged
}

// kindsOf is which of the three kinds each Step recorded was, in Step order.
func kindsOf(steps []Step) []StepKind {
	kinds := make([]StepKind, 0, len(steps))
	for _, step := range steps {
		kinds = append(kinds, step.Kind)
	}

	return kinds
}

func TestGivenSteps_ShouldDeclareTheTopologyAndTheWorkload(t *testing.T) {
	declaring := aScenario()

	declaring.Given().Topology(Zones(twoZones), NodesPerZone(twoNodesPerZone))
	declaring.Given().Workload(someWorkloadName, Pods(podsOverEveryNode)).SpreadEvenly(OverZones, OverNodes)

	declared := declaring.outcome()
	expect.Expect(t, declaring.topology.Nodes()).To(expect.Equal(aTopologyOf(t, twoZones, twoNodesPerZone).Nodes()))
	expect.Expect(t, declaring.workload).To(expect.Equal(anEvenlySpreadWorkloadOf(t, podsOverEveryNode)))
	expect.Expect(t, declared.Steps).To(expect.Equal([]Step{
		topologyStepOf(twoZones, twoNodesPerZone),
		evenlySpreadWorkloadStepOf(podsOverEveryNode),
	}))
}

func TestAGivenTopologyNamingItsNodeCapacity_ShouldDeclareThatCapacityAndRenameItsLine(t *testing.T) {
	declaring := aScenario()

	declaring.Given().Topology(Zones(twoZones), NodesPerZone(twoNodesPerZone)).NodeCapacity(Pods(twoPodsPerNode))

	declared := declaring.outcome()
	expect.Expect(t, declaring.topology.Nodes()).
		To(expect.Equal(aTopologyAt(t, twoZones, twoNodesPerZone, twoPodsPerNode).Nodes()))
	expect.Expect(t, declared.Steps).
		To(expect.Equal([]Step{topologyStepHoldingOf(twoZones, twoNodesPerZone, twoPodsPerNode)}))
}

func TestThenSteps_ShouldAssertThePlacementOutcome(t *testing.T) {
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	judging := aScenarioOver(onePodPerNodeOf(declared))
	judging.Given().Topology(Zones(twoZones), NodesPerZone(twoNodesPerZone))
	judging.Given().Workload(someWorkloadName, Pods(podsOverEveryNode)).SpreadEvenly(OverZones, OverNodes)

	judging.Then().AllPodsRunning()
	judging.Then().PodsPerZone(podsPerZoneWhenEven)
	judging.Then().PodsPerNode(podsPerNodeWhenEven)

	judged := judging.outcome()
	expect.Expect(t, asJudged(judged.Findings)).To(expect.Equal([]judgedFinding{}))
	expect.Expect(t, judged.Steps).To(expect.Equal([]Step{
		topologyStepOf(twoZones, twoNodesPerZone),
		evenlySpreadWorkloadStepOf(podsOverEveryNode),
		allPodsRunningStep(),
		podsPerZoneStepOf(podsPerZoneWhenEven),
		podsPerNodeStepOf(podsPerNodeWhenEven),
	}))
}

func TestThenThePlacementDoesNotMeet_ShouldReportAFindingAndRunTheLaterSteps(t *testing.T) {
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	judging := aScenarioOver(everyPodOnOneNodeOf(declared))
	judging.Given().Topology(Zones(twoZones), NodesPerZone(twoNodesPerZone))
	judging.Given().Workload(someWorkloadName, Pods(podsOverEveryNode)).SpreadEvenly(OverZones, OverNodes)

	judging.Then().PodsPerZone(podsPerZoneWhenEven)
	judging.Then().AllPodsRunning()

	judged := judging.outcome()
	expect.Expect(t, asJudged(judged.Findings)).To(expect.Equal([]judgedFinding{
		{Step: podsPerZoneStepOf(podsPerZoneWhenEven), Mismatching: true},
	}))
	expect.Expect(t, judged.Steps).To(expect.Equal([]Step{
		topologyStepOf(twoZones, twoNodesPerZone),
		evenlySpreadWorkloadStepOf(podsOverEveryNode),
		podsPerZoneStepOf(podsPerZoneWhenEven),
		allPodsRunningStep(),
	}))
}

func TestStepThatCannotBeCarriedOut_ShouldEndTheScenarioAndNameThatStep(t *testing.T) {
	// Each case declares the same world up to the one Given its constructor
	// refuses, and names the Steps recorded up to and including that line: a
	// Scenario that has ended records none of the lines after it.
	cases := []struct {
		name    string
		declare func(*Scenario)
		endedAt Step
		steps   []Step
		cause   string
	}{
		{
			name: "a Topology no Zone could be provisioned with",
			declare: func(ending *Scenario) {
				ending.Given().Topology(Zones(twoZones), NodesPerZone(noNodesPerZone))
				ending.Given().Workload(someWorkloadName, Pods(podsOverEveryNode)).SpreadEvenly(OverZones, OverNodes)
			},
			endedAt: topologyStepOf(twoZones, noNodesPerZone),
			steps:   []Step{topologyStepOf(twoZones, noNodesPerZone)},
			cause:   fmt.Sprintf("a Zone holds at least one Node, so a count of %d Nodes per Zone is refused", noNodesPerZone),
		},
		{
			name: "a Topology whose Nodes hold no Pod",
			declare: func(ending *Scenario) {
				ending.Given().Topology(Zones(twoZones), NodesPerZone(twoNodesPerZone)).NodeCapacity(Pods(noPods))
				ending.Given().Workload(someWorkloadName, Pods(podsOverEveryNode)).SpreadEvenly(OverZones, OverNodes)
			},
			endedAt: topologyStepHoldingOf(twoZones, twoNodesPerZone, noPods),
			steps:   []Step{topologyStepHoldingOf(twoZones, twoNodesPerZone, noPods)},
			cause:   fmt.Sprintf("a Node holds at least one Pod, so a capacity of %d Pods is refused", noPods),
		},
		{
			name: "a Workload of no Pods",
			declare: func(ending *Scenario) {
				ending.Given().Topology(Zones(twoZones), NodesPerZone(twoNodesPerZone))
				ending.Given().Workload(someWorkloadName, Pods(noPods))
			},
			endedAt: workloadStepOf(noPods),
			steps:   []Step{topologyStepOf(twoZones, twoNodesPerZone), workloadStepOf(noPods)},
			cause:   fmt.Sprintf("a Workload declares at least one Pod, so a count of %d Pods is refused", noPods),
		},
		{
			name: "a Workload spread over a domain no rule names",
			declare: func(ending *Scenario) {
				ending.Given().Topology(Zones(twoZones), NodesPerZone(twoNodesPerZone))
				ending.Given().Workload(someWorkloadName, Pods(podsOverEveryNode)).SpreadEvenly(noSuchSpreadDomain)
			},
			endedAt: spreadWorkloadStepOf(podsOverEveryNode, noSuchSpreadDomain),
			steps: []Step{
				topologyStepOf(twoZones, twoNodesPerZone),
				spreadWorkloadStepOf(podsOverEveryNode, noSuchSpreadDomain),
			},
			cause: fmt.Sprintf("a spread rule names [%s %s], so the domain %q is refused",
				OverZones, OverNodes, noSuchSpreadDomain),
		},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			declared := aTopologyOf(t, twoZones, twoNodesPerZone)
			ending := aScenarioOver(onePodPerNodeOf(declared))

			one.declare(ending)
			ending.Then().AllPodsRunning()

			ended := ending.outcome()
			expect.Expect(t, ended.EndedAt).To(expect.Equal(one.endedAt))
			expect.Expect(t, ended.Steps).To(expect.Equal(one.steps))
			expect.Expect(t, asJudged(ended.Findings)).To(expect.Equal([]judgedFinding{}))
			expect.Expect(t, causeOf(ended)).To(expect.Equal(one.cause))
		})
	}
}

func TestScenarioWithoutAFailure_ShouldCarryNoWhenStep(t *testing.T) {
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	settled := onePodPerNodeOf(declared)
	running := aScenarioOver(settled)

	running.Given().Topology(Zones(twoZones), NodesPerZone(twoNodesPerZone))
	running.Given().Workload(someWorkloadName, Pods(podsOverEveryNode)).SpreadEvenly(OverZones, OverNodes)
	running.Then().AllPodsRunning()

	reported := running.outcome()
	expect.Expect(t, kindsOf(reported.Steps)).To(expect.Equal([]StepKind{GivenStep, GivenStep, ThenStep}))
	expect.Expect(t, reported.Placement).To(expect.Equal(settled))
}

// causeOf is why a Scenario ended, as a Scenario that ran to its last Step
// reports no cause at all.
func causeOf(reported Outcome) string {
	if reported.Cause == nil {
		return ""
	}

	return reported.Cause.Error()
}

func TestAnEvenExpectation_ShouldGiveEveryDomainTheSameShare(t *testing.T) {
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	nodes := declared.Nodes()
	firstZone, lastZone := nodes[0].Zone, nodes[len(nodes)-1].Zone

	expected := evenlyOver(zonesOf(declared), podsPerZoneWhenEven)

	expect.Expect(t, expected).To(expect.Equal(map[string]int{
		firstZone: podsPerZoneWhenEven,
		lastZone:  podsPerZoneWhenEven,
	}))
}

func TestEachZoneOfATopology_ShouldBeNamedOnceInZoneOrder(t *testing.T) {
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	nodes := declared.Nodes()
	firstZone, lastZone := nodes[0].Zone, nodes[len(nodes)-1].Zone

	named := zonesOf(declared)

	expect.Expect(t, named).To(expect.Equal([]string{firstZone, lastZone}))
}

func TestEveryNodeOfATopology_ShouldBeNamedInTopologyOrder(t *testing.T) {
	const oneZone = 1
	declared := aTopologyOf(t, oneZone, twoNodesPerZone)
	nodes := declared.Nodes()

	named := nodesOf(declared)

	expect.Expect(t, named).To(expect.Equal([]string{nodes[0].Name, nodes[1].Name}))
}
