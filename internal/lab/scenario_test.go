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
	"strings"
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

	// noNodesPerZone and noPods are each a declaration the internal/scenario
	// constructor refuses, so the Given declaring one is a Step that cannot be
	// carried out. A specification setting nothing is the third, and it is the
	// zero value of the type rather than a count.
	noNodesPerZone = 0
	noPods         = 0

	// moreNodesThanTheTopologyHolds is a Failure that Topology cannot satisfy, so
	// the When asking for it is a Step that cannot be carried out.
	moreNodesThanTheTopologyHolds = twoZones*twoNodesPerZone + 1

	// theWhenThatNeverRan is what a Then reading the Nodes a Failure stopped names
	// when no When ran before it, which is what a reader has to add.
	theWhenThatNeverRan = "no When"

	// noSpecificationAtAll is how a Workload line reads when it declares the zero
	// specification: it names nothing, which is the Given's own refusal.
	noSpecificationAtAll = ""

	// onePodLeftBehind is the Pod the control plane has not moved off the Node that
	// stopped responding, and podLeftOnTheFailedNode what it is called there.
	onePodLeftBehind       = 1
	podLeftOnTheFailedNode = string(someWorkloadName) + "-left-behind"

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

// aScenarioAfterFailing is a Scenario whose When stopped these Nodes responding
// and which has settled at placed since, which is the world a Then reading the
// Failure judges in.
func aScenarioAfterFailing(placed scenario.Placement, stopped ...string) *Scenario {
	return &Scenario{observed: placed, settled: true, failed: stopped}
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

// aWorkloadDeclaringResilientPlacementOf is the Workload a Given declares when
// it names that specification for pods Pods.
func aWorkloadDeclaringResilientPlacementOf(t *testing.T, pods int) scenario.Workload {
	t.Helper()

	workload, err := scenario.NewWorkload(string(someWorkloadName), pods, ResilientPlacement().declared)
	if err != nil {
		t.Fatalf("build a Workload of %d Pods declaring Resilient placement: %v", pods, err)
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

// theNodeThatStopped is the Node a When of one Node stops responding: the first
// the Topology derives, which is the pick internal/scenario makes.
func theNodeThatStopped(topology scenario.Topology) string {
	return topology.Nodes()[0].Name
}

// everyPodBackOnTheRespondingNodesOf is the Placement a Failure settles at: every
// Pod the Workload declared Running on a Node still responding, and none on the
// Node that stopped. That Node is no Ready Node, so the observation is read
// against the rest.
func everyPodBackOnTheRespondingNodesOf(topology scenario.Topology, stopped string) scenario.Placement {
	responding := theNodesStillRespondingIn(topology, stopped)

	return scenario.NewPlacement(responding, podsRunningOver(responding, podsOverEveryNode))
}

// onePodLeftOnTheFailedNodeOf is the same Placement observed while the control
// plane has not moved one Pod off the Node that stopped: the Pod is counted under
// that Node's name, because the report states what was observed.
func onePodLeftOnTheFailedNodeOf(topology scenario.Topology, stopped string) scenario.Placement {
	responding := theNodesStillRespondingIn(topology, stopped)
	moved := podsRunningOver(responding, podsOverEveryNode-onePodLeftBehind)
	leftBehind := scenario.Pod{Name: podLeftOnTheFailedNode, Node: stopped, Phase: scenario.PodRunning}

	return scenario.NewPlacement(responding, append(moved, leftBehind))
}

// theNodesStillRespondingIn are the Nodes an observation is read against once
// stopped has left it, which a Node that stopped responding does at once.
func theNodesStillRespondingIn(topology scenario.Topology, stopped string) []scenario.Node {
	declared := topology.Nodes()
	responding := make([]scenario.Node, 0, len(declared))
	for _, node := range declared {
		if node.Name != stopped {
			responding = append(responding, node)
		}
	}

	return responding
}

// podsRunningOver is pods Running Pods dealt over these Nodes in turn, so a Node
// takes a second Pod only once every Node holds one.
func podsRunningOver(nodes []scenario.Node, pods int) []scenario.Pod {
	placed := make([]scenario.Pod, 0, pods)
	for at := range pods {
		placed = append(placed, scenario.Pod{
			Name:  fmt.Sprintf("%s-%d", someWorkloadName, at),
			Node:  nodes[at%len(nodes)].Name,
			Phase: scenario.PodRunning,
		})
	}

	return placed
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

// workloadStepOf is the Step a Workload line records: its Pods, and the intent of
// the specification it declares. resilientPlacementWorkloadStepOf is that line
// for the specification the Scenarios in this file declare.
func workloadStepOf(pods int, declaring string) Step {
	return Step{
		Kind: GivenStep,
		Name: fmt.Sprintf("a Workload %s of %d Pods declaring %s", someWorkloadName, pods, declaring),
	}
}

func resilientPlacementWorkloadStepOf(pods int) Step {
	return workloadStepOf(pods, ResilientPlacement().name())
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

// nodesFailStepOf is the Step a When records before it resolves the Nodes it
// stops; once it has them, it renames that line to carry their names.
func nodesFailStepOf(nodes int) Step {
	return Step{Kind: WhenStep, Name: fmt.Sprintf("%d Nodes fail", nodes)}
}

func noPodsOnTheFailedNodesStep() Step {
	return Step{Kind: ThenStep, Name: "no Pod on the Nodes that failed"}
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
	declaring.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), ResilientPlacement())

	declared := declaring.outcome()
	expect.Expect(t, declaring.topology.Nodes()).To(expect.Equal(aTopologyOf(t, twoZones, twoNodesPerZone).Nodes()))
	expect.Expect(t, declaring.workload).To(expect.Equal(aWorkloadDeclaringResilientPlacementOf(t, podsOverEveryNode)))
	expect.Expect(t, declared.Steps).To(expect.Equal([]Step{
		topologyStepOf(twoZones, twoNodesPerZone),
		resilientPlacementWorkloadStepOf(podsOverEveryNode),
	}))
}

// A Workload line customized with full access leaves the Scenario holding those
// full accesses, which is how they reach the Simulated Cluster: the Workload the
// domain constructs holds the specification's settings alone.
func TestAGivenWorkloadCustomizedWithFullAccess_ShouldKeepItBesideTheWorkloadItDeclared(t *testing.T) {
	declaring := aScenario()
	customized := ResilientPlacement().Customize(aFullAccess())

	declaring.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), customized)

	expect.Expect(t, placesOfTheFullAccesses(declaring.accesses)).
		To(expect.Equal(placesOfTheFullAccesses(customized.accesses)))
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
	judging.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), ResilientPlacement())

	judging.Then().AllPodsRunning()
	judging.Then().PodsPerZone(podsPerZoneWhenEven)
	judging.Then().PodsPerNode(podsPerNodeWhenEven)

	judged := judging.outcome()
	expect.Expect(t, asJudged(judged.Findings)).To(expect.Equal([]judgedFinding{}))
	expect.Expect(t, judged.Steps).To(expect.Equal([]Step{
		topologyStepOf(twoZones, twoNodesPerZone),
		resilientPlacementWorkloadStepOf(podsOverEveryNode),
		allPodsRunningStep(),
		podsPerZoneStepOf(podsPerZoneWhenEven),
		podsPerNodeStepOf(podsPerNodeWhenEven),
	}))
}

func TestThenThePlacementDoesNotMeet_ShouldReportAFindingAndRunTheLaterSteps(t *testing.T) {
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	judging := aScenarioOver(everyPodOnOneNodeOf(declared))
	judging.Given().Topology(Zones(twoZones), NodesPerZone(twoNodesPerZone))
	judging.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), ResilientPlacement())

	judging.Then().PodsPerZone(podsPerZoneWhenEven)
	judging.Then().AllPodsRunning()

	judged := judging.outcome()
	expect.Expect(t, asJudged(judged.Findings)).To(expect.Equal([]judgedFinding{
		{Step: podsPerZoneStepOf(podsPerZoneWhenEven), Mismatching: true},
	}))
	expect.Expect(t, judged.Steps).To(expect.Equal([]Step{
		topologyStepOf(twoZones, twoNodesPerZone),
		resilientPlacementWorkloadStepOf(podsOverEveryNode),
		podsPerZoneStepOf(podsPerZoneWhenEven),
		allPodsRunningStep(),
	}))
}

// The two tests below judge the Then that reads a Failure where every sibling Then
// is judged: over a Placement the test built, with no cluster. Stopping a Node
// responding is the Simulated Cluster's, and this Then's own decision is a count
// read off the Placement the Scenario settled at.
func TestAPlacementHoldingNoPodOnTheNodesThatFailed_ShouldRecordNoFinding(t *testing.T) {
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	stopped := theNodeThatStopped(declared)
	judging := aScenarioAfterFailing(everyPodBackOnTheRespondingNodesOf(declared, stopped), stopped)
	judging.Given().Topology(Zones(twoZones), NodesPerZone(twoNodesPerZone))
	judging.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), ResilientPlacement())

	judging.Then().NoPodsOnTheFailedNodes()
	judging.Then().AllPodsRunning()

	judged := judging.outcome()
	expect.Expect(t, asJudged(judged.Findings)).To(expect.Equal([]judgedFinding{}))
	expect.Expect(t, judged.Steps).To(expect.Equal([]Step{
		topologyStepOf(twoZones, twoNodesPerZone),
		resilientPlacementWorkloadStepOf(podsOverEveryNode),
		noPodsOnTheFailedNodesStep(),
		allPodsRunningStep(),
	}))
}

func TestAPlacementStillHoldingAPodOnANodeThatFailed_ShouldReportAFindingAndRunTheLaterSteps(t *testing.T) {
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	stopped := theNodeThatStopped(declared)
	judging := aScenarioAfterFailing(onePodLeftOnTheFailedNodeOf(declared, stopped), stopped)
	judging.Given().Topology(Zones(twoZones), NodesPerZone(twoNodesPerZone))
	judging.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), ResilientPlacement())

	judging.Then().NoPodsOnTheFailedNodes()
	judging.Then().AllPodsRunning()

	judged := judging.outcome()
	expect.Expect(t, asJudged(judged.Findings)).To(expect.Equal([]judgedFinding{
		{Step: noPodsOnTheFailedNodesStep(), Mismatching: true},
	}))
	expect.Expect(t, judged.Steps).To(expect.Equal([]Step{
		topologyStepOf(twoZones, twoNodesPerZone),
		resilientPlacementWorkloadStepOf(podsOverEveryNode),
		noPodsOnTheFailedNodesStep(),
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
				ending.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), ResilientPlacement())
			},
			endedAt: topologyStepOf(twoZones, noNodesPerZone),
			steps:   []Step{topologyStepOf(twoZones, noNodesPerZone)},
			cause:   fmt.Sprintf("a Zone holds at least one Node, so a count of %d Nodes per Zone is refused", noNodesPerZone),
		},
		{
			name: "a Topology whose Nodes hold no Pod",
			declare: func(ending *Scenario) {
				ending.Given().Topology(Zones(twoZones), NodesPerZone(twoNodesPerZone)).NodeCapacity(Pods(noPods))
				ending.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), ResilientPlacement())
			},
			endedAt: topologyStepHoldingOf(twoZones, twoNodesPerZone, noPods),
			steps:   []Step{topologyStepHoldingOf(twoZones, twoNodesPerZone, noPods)},
			cause:   fmt.Sprintf("a Node holds at least one Pod, so a capacity of %d Pods is refused", noPods),
		},
		{
			name: "a Workload of no Pods",
			declare: func(ending *Scenario) {
				ending.Given().Topology(Zones(twoZones), NodesPerZone(twoNodesPerZone))
				ending.Given().Workload(someWorkloadName, Pods(noPods), ResilientPlacement())
			},
			endedAt: resilientPlacementWorkloadStepOf(noPods),
			steps: []Step{
				topologyStepOf(twoZones, twoNodesPerZone),
				resilientPlacementWorkloadStepOf(noPods),
			},
			cause: fmt.Sprintf("a Workload declares at least one Pod, so a count of %d Pods is refused", noPods),
		},
		{
			name: "a Workload declaring a specification that sets nothing",
			declare: func(ending *Scenario) {
				ending.Given().Topology(Zones(twoZones), NodesPerZone(twoNodesPerZone))
				ending.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), PodSpecification{})
			},
			endedAt: workloadStepOf(podsOverEveryNode, noSpecificationAtAll),
			steps: []Step{
				topologyStepOf(twoZones, twoNodesPerZone),
				workloadStepOf(podsOverEveryNode, noSpecificationAtAll),
			},
			cause: "a Workload's Pods are deployed with the specification it declares, " +
				"so a specification holding no setting is refused",
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
	running.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), ResilientPlacement())
	running.Then().AllPodsRunning()

	reported := running.outcome()
	expect.Expect(t, kindsOf(reported.Steps)).To(expect.Equal([]StepKind{GivenStep, GivenStep, ThenStep}))
	expect.Expect(t, reported.Placement).To(expect.Equal(settled))
}

// A Scenario's lines are carried out in the order they stand in, so a Then before
// a When judges the Placement the Scenario starts from and the When after it
// injects the Failure into that same settled world. The When here is the
// refused-count one, which resolves no Node and so reaches no cluster.
func TestAThenStandingBeforeAWhen_ShouldJudgeThePlacementTheScenarioStartsFrom(t *testing.T) {
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	judging := aScenarioOver(onePodPerNodeOf(declared))
	judging.Given().Topology(Zones(twoZones), NodesPerZone(twoNodesPerZone))
	judging.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), ResilientPlacement())

	judging.Then().AllPodsRunning()
	judging.Then().PodsPerZone(podsPerZoneWhenEven)
	judging.When().NodesFail(Nodes(moreNodesThanTheTopologyHolds))

	judged := judging.outcome()
	expect.Expect(t, asJudged(judged.Findings)).To(expect.Equal([]judgedFinding{}))
	expect.Expect(t, judged.Steps).To(expect.Equal([]Step{
		topologyStepOf(twoZones, twoNodesPerZone),
		resilientPlacementWorkloadStepOf(podsOverEveryNode),
		allPodsRunningStep(),
		podsPerZoneStepOf(podsPerZoneWhenEven),
		nodesFailStepOf(moreNodesThanTheTopologyHolds),
	}))
}

func TestAWhenAskingForMoreFailingNodesThanTheTopologyHolds_ShouldEndTheScenarioNamingThatStep(t *testing.T) {
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	ending := aScenarioOver(onePodPerNodeOf(declared))
	ending.Given().Topology(Zones(twoZones), NodesPerZone(twoNodesPerZone))
	ending.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), ResilientPlacement())

	ending.When().NodesFail(Nodes(moreNodesThanTheTopologyHolds))
	ending.Then().AllPodsRunning()

	ended := ending.outcome()
	expect.Expect(t, ended.EndedAt).To(expect.Equal(nodesFailStepOf(moreNodesThanTheTopologyHolds)))
	expect.Expect(t, ended.Steps).To(expect.Equal([]Step{
		topologyStepOf(twoZones, twoNodesPerZone),
		resilientPlacementWorkloadStepOf(podsOverEveryNode),
		nodesFailStepOf(moreNodesThanTheTopologyHolds),
	}))
	expect.Expect(t, asJudged(ended.Findings)).To(expect.Equal([]judgedFinding{}))
	causeNames(t, ended, fmt.Sprintf("a Failure of %d Nodes is refused", moreNodesThanTheTopologyHolds))
}

func TestAThenReadingAFailureWithNoWhenBeforeIt_ShouldEndTheScenarioNamingThatThen(t *testing.T) {
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	ending := aScenarioOver(onePodPerNodeOf(declared))
	ending.Given().Topology(Zones(twoZones), NodesPerZone(twoNodesPerZone))
	ending.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), ResilientPlacement())

	ending.Then().NoPodsOnTheFailedNodes()
	ending.Then().AllPodsRunning()

	ended := ending.outcome()
	expect.Expect(t, ended.EndedAt).To(expect.Equal(noPodsOnTheFailedNodesStep()))
	expect.Expect(t, ended.Steps).To(expect.Equal([]Step{
		topologyStepOf(twoZones, twoNodesPerZone),
		resilientPlacementWorkloadStepOf(podsOverEveryNode),
		noPodsOnTheFailedNodesStep(),
	}))
	expect.Expect(t, asJudged(ended.Findings)).To(expect.Equal([]judgedFinding{}))
	causeNames(t, ended, theWhenThatNeverRan)
}

// causeOf is why a Scenario ended, as a Scenario that ran to its last Step
// reports no cause at all.
func causeOf(reported Outcome) string {
	if reported.Cause == nil {
		return ""
	}

	return reported.Cause.Error()
}

// causeNames asserts a Scenario ended for a reason naming what a reader has to
// act on, the way the suites of the refusing constructors read their own
// refusals.
func causeNames(t *testing.T, ended Outcome, named string) {
	t.Helper()

	if !strings.Contains(causeOf(ended), named) {
		t.Errorf("the Scenario ended for a reason naming %q, but it reads: %s", named, causeOf(ended))
	}
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
