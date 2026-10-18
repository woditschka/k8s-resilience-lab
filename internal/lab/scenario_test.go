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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/lab/node"
	"github.com/woditschka/k8s-resilience-lab/internal/lab/pod"
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

	// twoPodsPerNode is the Pod count of a Node capacity a Scenario states
	// outright: below roomyPodsPerNode, so a Topology declared at it differs from
	// one naming Roomy. someCoresPerNode and someGibibytesPerNode are the other
	// two dimensions such a line states, which no test here turns on, and
	// someCPUSpelled and someMemorySpelled are how Kubernetes spells them on the
	// renamed line.
	twoPodsPerNode       = 2
	someCoresPerNode     = 4
	someGibibytesPerNode = 16
	someCPUSpelled       = "4"
	someMemorySpelled    = "16Gi"

	// roomForOnePodPerNode is a Node capacity a Workload can fill, and
	// podsBeyondEveryNode a Workload of one Pod more than a Topology at that
	// capacity holds: onePodLeftPending is what no Node then has room for, whatever
	// the scheduler tries. noPodPending is the count such a Placement does not
	// hold, which is what a Then it does not meet is written with.
	roomForOnePodPerNode = 1
	onePodLeftPending    = 1
	podsBeyondEveryNode  = podsOverEveryNode + onePodLeftPending
	noPodPending         = 0

	// noNodeDeclared is how many Nodes a Placement holds where no Topology was
	// declared above the Wait that reached it.
	noNodeDeclared = 0

	// theMinimumKept is the Required Capacity the Workloads below declare: one Pod
	// fewer than the Workload runs, so one Node stopping leaves exactly the minimum
	// and the declaration is one the domain admits.
	theMinimumKept = podsOverEveryNode - onePodLeftBehind

	// aMinimumNoWorkloadCouldKeep is a Required Capacity above the Pods the Workload
	// declares, so the Given declaring one is a Step that cannot be carried out.
	aMinimumNoWorkloadCouldKeep = podsOverEveryNode + 1

	// podsShortOfTheMinimum is a count of Pods Running below the minimum those
	// Workloads declare, which is what a moment missed is counted at.
	podsShortOfTheMinimum = theMinimumKept - 1

	// noNodesPerZone and noPods are each a declaration the internal/scenario
	// constructor refuses, so the Given declaring one is a Step that cannot be
	// carried out. A specification setting nothing is the third, and it is the
	// zero value of the type rather than a count.
	noNodesPerZone = 0
	noPods         = 0

	// moreNodesThanTheTopologyHolds and moreZonesThanTheTopologyHolds are Failures
	// that Topology cannot satisfy, so the When asking for one is a Step that cannot
	// be carried out.
	moreNodesThanTheTopologyHolds = twoZones*twoNodesPerZone + 1
	moreZonesThanTheTopologyHolds = twoZones + 1

	// theWhenThatNeverRan is what a Then reading the Nodes a Failure stopped names
	// when no When ran before it, which is what a reader has to add.
	theWhenThatNeverRan = "no When"

	// oneNodeFailing is a Failure the declared Topology can satisfy, which is what
	// a When reading the Placement before it has to be to get as far as that read.
	oneNodeFailing = 1

	// theMinimumNeverDeclared is what the Then judging a Required Capacity names
	// when the Workload declared none, which is the other way it has nothing to
	// judge.
	theMinimumNeverDeclared = "declares no Required Capacity"

	// noMomentNamed is how a Finding naming neither of the two moments is read, so
	// one that named none is visible in the comparison rather than missing from it.
	noMomentNamed = "no moment"

	// noSpecificationAtAll is how a Workload line reads when it declares the zero
	// specification: it names nothing, which is the Given's own refusal.
	noSpecificationAtAll = ""

	// onePodLeftBehind is the Pod the control plane has not moved off the Node that
	// stopped responding, and podLeftOnTheFailedNode what it is called there.
	onePodLeftBehind       = 1
	podLeftOnTheFailedNode = string(someWorkloadName) + "-left-behind"

	// podLeftPending is what the Pod no Node had room for is called in a Placement
	// that left it on none.
	podLeftPending = string(someWorkloadName) + "-left-pending"

	// aFinelyStatedWait is a time a Wait held for, stated more finely than a report
	// states it, which is what the rounding below is read on.
	aFinelyStatedWait = 6234 * time.Millisecond

	// aTimeAWaitStates is a time a Wait of the tests below states it must be
	// reached in, and aWaitPastTheTimeItStates a time such a Wait took past it.
	aTimeAWaitStates         = 15
	aWaitPastTheTimeItStates = 25500 * time.Millisecond

	// aWaitOfNoMeasuredTime is what a Wait with nothing to wait for took, which is
	// why a line reads whether such a Wait was reached off the Step rather than off
	// this time (docs/prd.md REQ-SIM-006 edge case 2).
	aWaitOfNoMeasuredTime = 0

	// theWaitsOwnStep is where the one Wait of a Scenario recorded nothing else
	// stands in that Scenario's report.
	theWaitsOwnStep = 0

	// theWaitingTimeInSeconds is the lab's own waiting time in the whole seconds a
	// Wait states a time in, which is the longest time a Wait may state.
	theWaitingTimeInSeconds = int(scenario.WaitingTime / time.Second)

	// theSecondsAboveTheLabsWaitingTime and noSecondsAtAll are the two times no
	// Wait may state: one second more than the lab waits for a state of the
	// cluster, and no time at all.
	theSecondsAboveTheLabsWaitingTime = theWaitingTimeInSeconds + 1
	noSecondsAtAll                    = 0

	// someWorkloadName is the name a Workload's Pods are labelled with. A Then
	// counts Pods per domain and reads no meaning into the name, so it stands for
	// any. It is named here rather than taken from the Scenarios' place, which
	// internal/lab does not import.
	someWorkloadName WorkloadName = "some-workload"
)

// aWaitStating is the time a Wait of that many whole seconds is held to, built
// the way the Step surface builds one.
func aWaitStating(t *testing.T, seconds int) scenario.StatedWait {
	t.Helper()

	stated, err := scenario.NewStatedWait(seconds)
	if err != nil {
		t.Fatalf("build the time a Wait of %d seconds is held to: %v", seconds, err)
	}

	return stated
}

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

// aScenarioAtAWaitFor is a Scenario whose Wait for that state of the cluster has
// been recorded and not yet judged, which is the world a Wait's times are
// recorded and judged in.
func aScenarioAtAWaitFor(state string) *Scenario {
	waiting := aScenario()
	waiting.record(WaitStep, state)

	return waiting
}

// aScenarioAfterFailingWithNoWaitSince is a Scenario whose When stopped this Node
// responding and which has not Waited since, which is the world a Then standing
// straight after a Failure judges in.
func aScenarioAfterFailingWithNoWaitSince(placed scenario.Placement, stopped string) *Scenario {
	return &Scenario{observed: placed, failed: []string{stopped}}
}

// aScenarioAfterFailingWhichKept is that Scenario once its When has also derived
// what the Workload kept Running at the moment the Failure was injected, which is
// the earlier of the two moments a Required Capacity is judged at.
func aScenarioAfterFailingWhichKept(placed scenario.Placement, kept int, stopped ...string) *Scenario {
	failing := aScenarioAfterFailing(placed, stopped...)
	failing.runningAtTheFailure = kept

	return failing
}

// aScenarioAfterFailingAZone is a Scenario whose When stopped this Zone whole and
// which has settled at placed, which is the world a Then reading the Zones that
// failed judges in.
func aScenarioAfterFailingAZone(placed scenario.Placement, stopped string) *Scenario {
	return &Scenario{observed: placed, settled: true, failedZones: []string{stopped}}
}

// The Named Node capacity this suite defines under a name of its own, as a
// Scenario's place defines Standard: the lab defines none, so a test needing one
// defines it. Its Pod count leaves room for more than one Pod per Node, and its
// cpu and memory are spelled as Kubernetes spells them on the line naming it.
const roomyName node.CapacityName = "Roomy"

const (
	roomyCoresPerNode     = 16
	roomyGibibytesPerNode = 64
	roomyPodsPerNode      = 10
)

// roomyNodes is that Named Node capacity as a NodePool names it.
func roomyNodes() node.Capacity {
	return node.Named(roomyName, node.Capacity{CPU: roomyCoresPerNode, Memory: roomyGibibytesPerNode * node.Gi, Pods: roomyPodsPerNode})
}

// aTopologyOf is the Topology of the stated shape, its Nodes holding the
// capacity Roomy stands for, which theGeneralNodePool names.
func aTopologyOf(t *testing.T, zones, nodesPerZone int) scenario.Topology {
	t.Helper()

	return aTopologyHolding(t, zones, nodesPerZone, theCapacityRoomyStandsFor(t))
}

// theCapacityRoomyStandsFor is the domain's capacity of the values Roomy's
// definition gives.
func theCapacityRoomyStandsFor(t *testing.T) scenario.NodeCapacity {
	t.Helper()

	capacity, err := scenario.NewNodeCapacity(roomyPodsPerNode, coresOf(roomyCoresPerNode), gibibytesOf(roomyGibibytesPerNode))
	if err != nil {
		t.Fatalf("build the capacity Roomy stands for: %v", err)
	}

	return capacity
}

// aTopologyAt is the Topology of the stated shape with every Node holding
// podsPerNode Pods, the cpu and memory being the ones a line here states, which
// is the capacity a Given names outright.
func aTopologyAt(t *testing.T, zones, nodesPerZone, podsPerNode int) scenario.Topology {
	t.Helper()

	capacity, err := scenario.NewNodeCapacity(podsPerNode, coresOf(someCoresPerNode), gibibytesOf(someGibibytesPerNode))
	if err != nil {
		t.Fatalf("build a Node capacity of %d Pods: %v", podsPerNode, err)
	}

	return aTopologyHolding(t, zones, nodesPerZone, capacity)
}

// aTopologyHolding is the Topology of the stated shape whose every Node holds the
// capacity named.
func aTopologyHolding(t *testing.T, zones, nodesPerZone int, capacity scenario.NodeCapacity) scenario.Topology {
	t.Helper()

	pool, err := scenario.NewNodePool(string(generalNodePool), nodesPerZone, capacity)
	if err != nil {
		t.Fatalf("build a NodePool of %d Nodes per Zone: %v", nodesPerZone, err)
	}
	topology, err := scenario.NewTopology(zones, pool)
	if err != nil {
		t.Fatalf("build a Topology of %d Zones of %d Nodes: %v", zones, nodesPerZone, err)
	}

	return topology
}

// aWorkloadDeclaringResilientPlacementOf is the Workload a Given declares when
// it names that specification for pods Pods.
func aWorkloadDeclaringResilientPlacementOf(t *testing.T, pods int) scenario.Workload {
	t.Helper()

	specification, err := resilientPlacement().Declared()
	if err != nil {
		t.Fatalf("define Resilient placement: %v", err)
	}
	workload, err := scenario.NewWorkload(string(someWorkloadName), pods, specification)
	if err != nil {
		t.Fatalf("build a Workload of %d Pods declaring Resilient placement: %v", pods, err)
	}

	return workload
}

// aWorkloadKeepingOf is that Workload once it declares the fewest Pods it must
// keep Running through a Failure.
func aWorkloadKeepingOf(t *testing.T, pods, minimum int) scenario.Workload {
	t.Helper()

	workload, err := aWorkloadDeclaringResilientPlacementOf(t, pods).WithRequiredCapacity(minimum)
	if err != nil {
		t.Fatalf("declare a Required Capacity of %d Pods: %v", minimum, err)
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

// onePodLeftPendingBesideOnePerNodeOf is the Placement a Workload of one Pod more
// than its Nodes hold settles at: one Pod Running on each Node of topology, and
// the Pod no Node had room for left Pending on none of them.
func onePodLeftPendingBesideOnePerNodeOf(topology scenario.Topology) scenario.Placement {
	placed := onePodPerNodeOf(topology).Pods()
	pending := scenario.Pod{Name: podLeftPending, Phase: scenario.PodPending}

	return scenario.NewPlacement(topology.Nodes(), append(placed, pending))
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

// podsRunningShortOfTheMinimumOn is the Placement a Failure settled at while the
// control plane has yet to place every Pod again: fewer Pods Running on the Nodes
// still responding than the Workload's Required Capacity names.
func podsRunningShortOfTheMinimumOn(topology scenario.Topology, stopped string) scenario.Placement {
	responding := theNodesStillRespondingIn(topology, stopped)

	return scenario.NewPlacement(responding, podsRunningOver(responding, podsShortOfTheMinimum))
}

// theZoneThatStopped is the Zone a When of one Zone stops responding: the first
// the Topology derives, which is the pick internal/scenario makes.
func theZoneThatStopped(topology scenario.Topology) string {
	return topology.Zones()[0]
}

// everyPodBackOnTheRespondingZonesOf is the Placement a Zone Failure settles at:
// every Pod the Workload declared Running in a Zone still responding. The stopped
// Zone's Nodes are no Ready Nodes, so the observation is read against the rest and
// that Zone is in it no longer.
func everyPodBackOnTheRespondingZonesOf(topology scenario.Topology, stopped string) scenario.Placement {
	responding := theNodesOutside(topology, stopped)

	return scenario.NewPlacement(responding, podsRunningOver(responding, podsOverEveryNode))
}

// onePodLeftInTheFailedZoneOf is the same Placement observed while one Node of the
// stopped Zone is still Ready and still holds a Pod: a Zone's Nodes stop one after
// another, so an observation taken between them reads against one of them still.
func onePodLeftInTheFailedZoneOf(topology scenario.Topology, stopped string) scenario.Placement {
	responding := theNodesOutside(topology, stopped)
	moved := podsRunningOver(responding, podsOverEveryNode-onePodLeftBehind)
	stillReady := theFirstNodeIn(topology, stopped)
	leftBehind := scenario.Pod{Name: podLeftOnTheFailedNode, Node: stillReady.Name, Phase: scenario.PodRunning}

	return scenario.NewPlacement(append(responding, stillReady), append(moved, leftBehind))
}

// theNodesOutside are the Nodes of every Zone but this one, and theFirstNodeIn the
// first Node the Topology derives in it.
func theNodesOutside(topology scenario.Topology, zone string) []scenario.Node {
	declared := topology.Nodes()
	outside := make([]scenario.Node, 0, len(declared))
	for _, node := range declared {
		if node.Zone != zone {
			outside = append(outside, node)
		}
	}

	return outside
}

func theFirstNodeIn(topology scenario.Topology, zone string) scenario.Node {
	for _, node := range topology.Nodes() {
		if node.Zone == zone {
			return node
		}
	}

	return scenario.Node{}
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
//
// topologyStepOf is the line of a NodePool naming Roomy, which it reads by that
// name and the values Roomy stands for.
func topologyStepOf(zones, nodesPerZone int) Step {
	return Step{
		Kind: GivenStep,
		Name: fmt.Sprintf("a Topology of %d Zones of a NodePool %s of %d %s Nodes holding %d cpu, %dGi memory and %d Pods each",
			zones, generalNodePool, nodesPerZone, roomyName, roomyCoresPerNode, roomyGibibytesPerNode, roomyPodsPerNode),
	}
}

// topologyStepHoldingOf is the Step a Topology line records once its NodePool
// states the capacity its Nodes hold, and topologyStepHoldingOnePodOf is that
// line at a capacity of one Pod, which reads in the singular as the counted Steps
// below do.
func topologyStepHoldingOf(zones, nodesPerZone, podsPerNode int) Step {
	return topologyStepHolding(zones, nodesPerZone, fmt.Sprintf("%d Pods", podsPerNode))
}

func topologyStepHoldingOnePodOf(zones, nodesPerZone int) Step {
	return topologyStepHolding(zones, nodesPerZone, fmt.Sprintf("%d Pod", roomForOnePodPerNode))
}

func topologyStepHolding(zones, nodesPerZone int, podsSpelled string) Step {
	return topologyStepHoldingCapacity(zones, nodesPerZone,
		fmt.Sprintf("%s cpu, %s memory and %s", someCPUSpelled, someMemorySpelled, podsSpelled))
}

// topologyStepHoldingCapacity is how any such line reads, whichever capacity it
// states: one template, so the lines cannot drift apart here.
func topologyStepHoldingCapacity(zones, nodesPerZone int, capacity string) Step {
	return Step{
		Kind: GivenStep,
		Name: fmt.Sprintf("a Topology of %d Zones of a NodePool %s of %d Nodes holding %s each",
			zones, generalNodePool, nodesPerZone, capacity),
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
	return carrying(workloadStepOf(pods, resilientPlacementReads()), resilientPlacementCarries()...)
}

// resilientPlacementReads is how Resilient placement is named on the Given line
// declaring it: its name, then the Startup time its Pods take.
func resilientPlacementReads() string {
	return fmt.Sprintf("%s, each Pod starting in %s to %s", resilientPlacementNamed,
		someShortestStartupMillis*time.Millisecond, someLongestStartupMillis*time.Millisecond)
}

// carrying is that Step reporting the settings its Workload's Pods carry, one
// line each.
func carrying(step Step, settings ...string) Step {
	step.Settings = settings

	return step
}

// aTolerationLine, aSpreadLine and aRequestLine are how a Given reports one
// setting its Workload's Pods carry: as a manifest writes it, the Pod-spec field
// it fills, then each field's Kubernetes API name and value. A toleration is
// reported at the lab's waiting time, the one time the domain holds it at.
func aTolerationLine(taint string) string {
	return fmt.Sprintf("tolerations: {key: %s, operator: Exists, effect: NoExecute, tolerationSeconds: %d}",
		taint, theLabsWaitInSeconds())
}

func aSpreadLine(topologyKey string, maxSkew int) string {
	return fmt.Sprintf(
		"topologySpreadConstraints: {topologyKey: %s, maxSkew: %d, whenUnsatisfiable: DoNotSchedule, nodeTaintsPolicy: Honor}",
		topologyKey, maxSkew)
}

func aRequestLine(cores, gibibytes int) string {
	return fmt.Sprintf("resources.requests: {cpu: %d, memory: %dGi}", cores, gibibytes)
}

// aFullAccessLine is how a Given reports a full access its Workload's Pods carry.
const aFullAccessLine = "a full access to the Kubernetes Pod specification applies"

// theLeavingLines are the two settings the Named setting for leaving an
// unresponsive Node stands for, and resilientPlacementCarries every setting
// Resilient placement's Pods carry, in the order the domain holds them.
func theLeavingLines() []string {
	return []string{aTolerationLine(scenario.TaintNodeNotReady), aTolerationLine(scenario.TaintNodeUnreachable)}
}

func resilientPlacementCarries() []string {
	return append(theLeavingLines(), aSpreadLine(scenario.ZoneLabel, anEvenSkew), aSpreadLine(scenario.HostnameLabel, anEvenSkew))
}

// workloadStepKeepingOf is that line once it names the fewest Pods the Workload
// must keep Running, which renames the line rather than adding one.
func workloadStepKeepingOf(pods, minimum int) Step {
	keeping := resilientPlacementWorkloadStepOf(pods)
	keeping.Name = fmt.Sprintf("%s, keeping at least %d Pods Running", keeping.Name, minimum)

	return keeping
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

// nodesFailStepOf and zonesFailStepOf are the Steps a When records before it
// resolves what it stops; once it has them, it renames that line to carry their
// names, which only a run against a cluster reaches.
func nodesFailStepOf(nodes int) Step {
	return Step{Kind: WhenStep, Name: fmt.Sprintf("%d Nodes fail", nodes)}
}

// oneNodeFailsStep is the Step a When of one Node records, which reads in the
// singular where the counted Steps above read in the plural.
func oneNodeFailsStep() Step {
	return Step{Kind: WhenStep, Name: fmt.Sprintf("%d Node fails", oneNodeFailing)}
}

func zonesFailStepOf(zones int) Step {
	return Step{Kind: WhenStep, Name: fmt.Sprintf("%d Zones fail", zones)}
}

// allNodesReadyStep, allPodsReadyStep and settledPlacementStep are the Steps the
// three Waits record: a Wait line of their own, whichever way the Scenario wrote
// them.
func allNodesReadyStep() Step {
	return Step{Kind: WaitStep, Name: allNodesReady}
}

func allPodsReadyStep() Step {
	return Step{Kind: WaitStep, Name: allPodsReady}
}

func settledPlacementStep() Step {
	return Step{Kind: WaitStep, Name: theSettledPlacement}
}

// podsPendingStepOf is the Step the Then counting the Pods left Pending records,
// and onePodPendingStep that line where one Pod is left, which reads in the
// singular as the counted Steps above do.
func podsPendingStepOf(pods int) Step {
	return Step{Kind: ThenStep, Name: fmt.Sprintf("%d Pods Pending", pods)}
}

func onePodPendingStep() Step {
	return Step{Kind: ThenStep, Name: fmt.Sprintf("%d Pod Pending", onePodLeftPending)}
}

func noPodsOnTheFailedNodesStep() Step {
	return Step{Kind: ThenStep, Name: "no Pod on the Nodes that failed"}
}

func noPodsInTheFailedZonesStep() Step {
	return Step{Kind: ThenStep, Name: "no Pod in the Zones that failed"}
}

// requiredCapacityStep is the Step the Then judging a Required Capacity records
// before it has the counts, which is the line a Scenario with nothing to judge
// ends at. requiredCapacityStepOf is that line once it states the minimum and both
// counts, which it is renamed to before it judges them.
func requiredCapacityStep() Step {
	return Step{Kind: ThenStep, Name: "the Required Capacity held"}
}

func requiredCapacityStepOf(minimum, atTheFailure, onceSettled int) Step {
	return Step{
		Kind: ThenStep,
		Name: fmt.Sprintf("%s: a minimum of %d Pods, %d at the moment of the Failure and %d once Settled",
			requiredCapacityStep().Name, minimum, atTheFailure, onceSettled),
	}
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

// momentsNamedIn is the moment each Finding names, of the two a Required Capacity
// is judged at: one judgement reports both moments under one Step, so what tells
// its Findings apart is the moment each of them names.
func momentsNamedIn(findings []Finding) []string {
	named := make([]string, 0, len(findings))
	for _, finding := range findings {
		named = append(named, momentNamedBy(finding))
	}

	return named
}

func momentNamedBy(finding Finding) string {
	for _, moment := range []string{atTheMomentOfTheFailure, onceThePlacementHadSettled} {
		if strings.Contains(finding.Mismatch, moment) {
			return moment
		}
	}

	return noMomentNamed
}

// A Wait's line names the time it took where it was reached, and the time it was
// held to where it was not, so a reader of a restore time never reads one for the
// other. A stated time is marked as stated and the lab's own waiting time is not,
// and the time it took is stated to the tenth of a second such a reader acts on.
// The three Wait kinds read the same way, each line being composed from the Step's
// own values (docs/prd.md REQ-SIM-006).
func TestAWaitsLine_ShouldTellTheTimeItTookFromTheTimeItWasHeldTo(t *testing.T) {
	statingATime := aWaitStating(t, aTimeAWaitStates)
	statingNone := scenario.NewWaitStatingNoTime()

	lines := map[string]struct {
		state  string
		record func(*Scenario)
		reads  string
	}{
		"stating no time, reached": {
			state:  allNodesReady,
			record: func(waiting *Scenario) { waiting.reached(statingNone, aFinelyStatedWait) },
			reads:  "Wait for " + allNodesReady + ", reached in 6.2s",
		},
		"stating a time, reached inside it": {
			state:  allPodsReady,
			record: func(waiting *Scenario) { waiting.reached(statingATime, aFinelyStatedWait) },
			reads:  "Wait for " + allPodsReady + ", reached in 6.2s of the " + secondsRead(aTimeAWaitStates) + " stated",
		},
		"stating no time, not reached": {
			state:  theSettledPlacement,
			record: func(waiting *Scenario) { waiting.notReached(statingNone) },
			reads:  "Wait for " + theSettledPlacement + ", not reached in " + scenario.WaitingTime.String(),
		},
		"with nothing to wait for": {
			state:  allPodsReady,
			record: func(waiting *Scenario) { waiting.reached(statingNone, aWaitOfNoMeasuredTime) },
			reads:  "Wait for " + allPodsReady + ", reached in 0s",
		},
	}

	for written, line := range lines {
		t.Run(written, func(t *testing.T) {
			waiting := aScenarioAtAWaitFor(line.state)

			line.record(waiting)

			expect.Expect(t, waiting.outcome().Steps[theWaitsOwnStep].String()).To(expect.Equal(line.reads))
		})
	}
}

// A Step of any other kind waited for nothing and reads as its kind and the name
// it reports alone, which is what the time a Wait was held to tells apart: a Wait
// with nothing to wait for was still held to a time, and reports being reached in
// no measured time rather than reading as a Given does.
func TestAStepThatIsNotAWait_ShouldReadAsItsKindAndNameAlone(t *testing.T) {
	declared := topologyStepOf(twoZones, twoNodesPerZone)

	expect.Expect(t, declared.String()).To(expect.Equal("Given " + declared.Name))
}

// A Wait reached later than the time it states ends the Scenario at that Wait,
// whose line names that time alone: the time it took is left off, because what a
// reader acts on there is the time the Scenario called and missed. The judgement
// takes the time measured rather than reading a clock, which is what lets it run
// over no cluster (docs/prd.md REQ-SIM-006).
func TestAWaitNotReachedInTheTimeItStates_ShouldEndTheScenarioNamingThatTimeAlone(t *testing.T) {
	ending := aScenarioAtAWaitFor(allPodsReady)

	ending.reached(aWaitStating(t, aTimeAWaitStates), aWaitPastTheTimeItStates)

	ended := ending.outcome()
	expect.Expect(t, ended.EndedAt.String()).
		To(expect.Equal("Wait for " + allPodsReady + ", not reached in the " + secondsRead(aTimeAWaitStates) + " stated"))
	causeNames(t, ended, theWaitReachedTooLate)
}

// Each form of writing a Wait states a time of its own: all four carry the
// seconds written on their line into the domain, which refuses a time the lab
// cannot hold a Wait to and names the time that Wait stated. The refusal stands
// before the Wait applies anything, so these Scenarios reach no cluster.
func TestAWaitStatingATimeWithItsGivenAndOneOnItsOwn_ShouldEachBeHeldToTheTimeItStates(t *testing.T) {
	waits := map[string]struct {
		write func(*Scenario)
		named string
		step  Step
	}{
		"chained onto the Topology it declares": {
			write: func(ending *Scenario) {
				ending.Given().Topology(Zones(twoZones), theGeneralNodePool()).
					WaitFor().AllNodesReadyWithinSeconds(theSecondsAboveTheLabsWaitingTime)
			},
			named: secondsRead(theSecondsAboveTheLabsWaitingTime),
			step:  allNodesReadyStep(),
		},
		"chained onto the Workload it declares": {
			write: func(ending *Scenario) {
				ending.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), resilientPlacement()).
					WaitFor().AllPodsReadyWithinSeconds(noSecondsAtAll)
			},
			named: strconv.Itoa(noSecondsAtAll),
			step:  allPodsReadyStep(),
		},
		"a Wait Step of its own for the Nodes": {
			write: func(ending *Scenario) {
				ending.WaitFor().AllNodesReadyWithinSeconds(noSecondsAtAll)
			},
			named: strconv.Itoa(noSecondsAtAll),
			step:  allNodesReadyStep(),
		},
		"a Wait Step of its own for the Pods": {
			write: func(ending *Scenario) {
				ending.WaitFor().AllPodsReadyWithinSeconds(theSecondsAboveTheLabsWaitingTime)
			},
			named: secondsRead(theSecondsAboveTheLabsWaitingTime),
			step:  allPodsReadyStep(),
		},
	}

	for written, wait := range waits {
		t.Run(written, func(t *testing.T) {
			ending := aScenario()

			wait.write(ending)

			ended := ending.outcome()
			expect.Expect(t, ended.EndedAt).To(expect.Equal(wait.step))
			causeNames(t, ended, wait.named)
			causeNames(t, ended, scenario.WaitingTime.String())
		})
	}
}

// secondsRead is how a refusal states a time of that many seconds, which is the
// duration those seconds are resolved to rather than the count written.
func secondsRead(seconds int) string {
	return (time.Duration(seconds) * time.Second).String()
}

// A Given declares the value its line names and reaches no cluster: what it
// declared is applied by the Wait a Scenario writes after it, so these two lines
// run against a Scenario holding no Simulated Cluster at all
// (docs/adr/2026-10-07-a-scenario-writes-its-own-wait.md).
func TestAGivenThatIsNotAskedToWait_ShouldDeclareWithoutReachingTheCluster(t *testing.T) {
	declaring := aScenario()

	declaring.Given().Topology(Zones(twoZones), theGeneralNodePool())
	declaring.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), resilientPlacement())

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
	customized := resilientPlacement().Customize(aFullAccess())

	declaring.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), customized)

	expect.Expect(t, placesOfTheFullAccesses(declaring.accesses)).
		To(expect.Equal(placesOfTheFullAccesses(customized.Accesses())))
}

// A Workload line declaring a Required Capacity carries the minimum into the
// Workload the domain holds, and renames its own line to it (docs/prd.md
// REQ-SIM-010).
func TestAGivenWorkloadDeclaringARequiredCapacity_ShouldDeclareThatMinimumBesideItsPods(t *testing.T) {
	declaring := aScenario()

	declaring.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), resilientPlacement()).
		RequiredCapacity(Pods(theMinimumKept))

	declared := declaring.outcome()
	expect.Expect(t, declaring.workload).To(expect.Equal(aWorkloadKeepingOf(t, podsOverEveryNode, theMinimumKept)))
	expect.Expect(t, declared.Steps).
		To(expect.Equal([]Step{workloadStepKeepingOf(podsOverEveryNode, theMinimumKept)}))
}

// A minimum above the Pods the Workload runs is refused where it is declared,
// which is docs/prd.md REQ-SIM-010 edge case 1. The minima the domain refuses are
// its own suite's, this one being the path through the Given Step.
func TestARequiredCapacityNoWorkloadCouldKeep_ShouldEndTheScenarioNamingThatGiven(t *testing.T) {
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	ending := aScenarioOver(onePodPerNodeOf(declared))
	ending.Given().Topology(Zones(twoZones), theGeneralNodePool())
	ending.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), resilientPlacement()).
		RequiredCapacity(Pods(aMinimumNoWorkloadCouldKeep))

	ending.Then().AllPodsRunning()

	ended := ending.outcome()
	refused := workloadStepKeepingOf(podsOverEveryNode, aMinimumNoWorkloadCouldKeep)
	expect.Expect(t, ended.EndedAt).To(expect.Equal(refused))
	expect.Expect(t, ended.Steps).To(expect.Equal([]Step{topologyStepOf(twoZones, twoNodesPerZone), refused}))
	expect.Expect(t, asJudged(ended.Findings)).To(expect.Equal([]judgedFinding{}))
	causeNames(t, ended, fmt.Sprintf("could never keep %d Pods Running", aMinimumNoWorkloadCouldKeep))
}

// A Startup time the domain refuses is refused where it is stated, so the
// Scenario ends at the Given declaring that Workload, which is docs/prd.md
// REQ-SIM-012 edge case 2. The times the domain refuses are its own suite's, this
// one being the path through the Given Step.
func TestARefusedStartupTime_ShouldEndTheScenarioAtTheGivenDeclaringItsWorkload(t *testing.T) {
	const aShortestAboveItsLongestMillis, itsLongestMillis = 900, 400
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	ending := aScenarioOver(onePodPerNodeOf(declared))
	ending.Given().Topology(Zones(twoZones), theGeneralNodePool())
	ending.Given().Workload(someWorkloadName, Pods(podsOverEveryNode),
		resilientPlacement().Customize(pod.StartupMillisBetween(aShortestAboveItsLongestMillis, itsLongestMillis)))

	ending.Then().AllPodsRunning()

	ended := ending.outcome()
	refused := workloadStepOf(podsOverEveryNode, string(resilientPlacementNamed))
	expect.Expect(t, ended.EndedAt).To(expect.Equal(refused))
	expect.Expect(t, ended.Steps).To(expect.Equal([]Step{topologyStepOf(twoZones, twoNodesPerZone), refused}))
	causeNames(t, ended, fmt.Sprintf("shortest Startup time of %s", aShortestAboveItsLongestMillis*time.Millisecond))
}

// Edge case 1: a definition names its Startup time as its second argument, so one
// leaving it out does not compile. The one way left to write none is the zero
// Startup time no form builds, which is refused, and so is that zero time written
// as a pod.Customization; either ends the Scenario at the Given declaring a Workload
// of it (docs/prd.md REQ-SIM-015).
func TestAPodSpecificationDefinedWithoutAStartupTime_ShouldBeRefusedWhereItIsDefined(t *testing.T) {
	written := map[string]pod.Specification{
		"defined with none":    pod.Predefined(resilientPlacementNamed, pod.StartupTime{}),
		"customized with none": resilientPlacement().Customize(pod.StartupTime{}),
	}

	for how, declares := range written {
		t.Run(how, func(t *testing.T) {
			declared := aTopologyOf(t, twoZones, twoNodesPerZone)
			ending := aScenarioOver(onePodPerNodeOf(declared))
			ending.Given().Topology(Zones(twoZones), theGeneralNodePool())

			ending.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), declares)

			ended := ending.outcome()
			expect.Expect(t, ended.EndedAt).To(expect.Equal(workloadStepOf(podsOverEveryNode, string(resilientPlacementNamed))))
			causeNames(t, ended, "names no Startup time")
		})
	}
}

// A definition the domain refuses, under no name or naming a Startup time no Pod
// can take, ends the Scenario at the Given declaring a Workload of it, as a
// refused pod.Customization does. The refusals themselves are the domain's suite's.
func TestAPodSpecificationTheDomainRefuses_ShouldEndTheScenarioAtTheGivenDeclaringIt(t *testing.T) {
	const noName, aShortestAboveItsLongestMillis, itsLongestMillis = "", 900, 400

	written := map[string]struct {
		declares pod.Specification
		named    string
		cause    string
	}{
		"under no name": {
			declares: pod.Predefined(noName, someStartupTime()),
			named:    noName,
			cause:    "under no name",
		},
		"naming a Startup time no Pod can take": {
			declares: pod.Predefined(resilientPlacementNamed,
				pod.StartupMillisBetween(aShortestAboveItsLongestMillis, itsLongestMillis)),
			named: string(resilientPlacementNamed),
			cause: fmt.Sprintf("shortest Startup time of %s", aShortestAboveItsLongestMillis*time.Millisecond),
		},
	}

	for how, taken := range written {
		t.Run(how, func(t *testing.T) {
			declared := aTopologyOf(t, twoZones, twoNodesPerZone)
			ending := aScenarioOver(onePodPerNodeOf(declared))
			ending.Given().Topology(Zones(twoZones), theGeneralNodePool())

			ending.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), taken.declares)

			ended := ending.outcome()
			expect.Expect(t, ended.EndedAt).To(expect.Equal(workloadStepOf(podsOverEveryNode, taken.named)))
			causeNames(t, ended, taken.cause)
		})
	}
}

// Edge case 3: every field of a setting the lab offers is required, and its zero
// value is the field left out. A Pod setting leaving one out is refused where it
// is stated, so the Scenario ends at the Given declaring its Workload, naming the
// setting and the field under its Kubernetes name; a Node capacity leaving out a
// dimension is refused by the domain at the Topology Given declaring it
// (docs/prd.md REQ-SIM-016 and
// docs/adr/2026-10-18-pod-and-node-settings-are-keyed-literals-refused-at-their-given.md).
func TestASettingMissingAValueKubernetesRequires_ShouldBeRefusedWhereItIsStated(t *testing.T) {
	const noneSpelled = "0"
	theWorkloadLineRefused := workloadStepOf(podsOverEveryNode, string(resilientPlacementNamed))

	refused := map[string]struct {
		declare func(*Scenario)
		steps   []Step
		cause   string
	}{
		"a toleration leaving out its key": {
			declare: declaringAWorkloadCustomizedWith(leavingOut(aToleration(), func(left *pod.Toleration) { left.Key = "" })),
			steps:   []Step{topologyStepOf(twoZones, twoNodesPerZone), theWorkloadLineRefused},
			cause:   "pod.Toleration leaves out key",
		},
		"a toleration leaving out its operator": {
			declare: declaringAWorkloadCustomizedWith(leavingOut(aToleration(), func(left *pod.Toleration) { left.Operator = "" })),
			steps:   []Step{topologyStepOf(twoZones, twoNodesPerZone), theWorkloadLineRefused},
			cause:   "pod.Toleration leaves out operator",
		},
		"a toleration leaving out its effect": {
			declare: declaringAWorkloadCustomizedWith(leavingOut(aToleration(), func(left *pod.Toleration) { left.Effect = "" })),
			steps:   []Step{topologyStepOf(twoZones, twoNodesPerZone), theWorkloadLineRefused},
			cause:   "pod.Toleration leaves out effect",
		},
		"a toleration leaving out its seconds": {
			declare: declaringAWorkloadCustomizedWith(leavingOut(aToleration(), func(left *pod.Toleration) { left.TolerationSeconds = 0 })),
			steps:   []Step{topologyStepOf(twoZones, twoNodesPerZone), theWorkloadLineRefused},
			cause:   "pod.Toleration leaves out tolerationSeconds",
		},
		"a spread leaving out its topology key": {
			declare: declaringAWorkloadCustomizedWith(leavingOut(aSpread(), func(left *pod.TopologySpreadConstraint) { left.TopologyKey = "" })),
			steps:   []Step{topologyStepOf(twoZones, twoNodesPerZone), theWorkloadLineRefused},
			cause:   "pod.TopologySpreadConstraint leaves out topologyKey",
		},
		"a spread leaving out its skew": {
			declare: declaringAWorkloadCustomizedWith(leavingOut(aSpread(), func(left *pod.TopologySpreadConstraint) { left.MaxSkew = 0 })),
			steps:   []Step{topologyStepOf(twoZones, twoNodesPerZone), theWorkloadLineRefused},
			cause:   "pod.TopologySpreadConstraint leaves out maxSkew",
		},
		"a spread leaving out what happens to a Pod no Node can take": {
			declare: declaringAWorkloadCustomizedWith(leavingOut(aSpread(), func(left *pod.TopologySpreadConstraint) { left.WhenUnsatisfiable = "" })),
			steps:   []Step{topologyStepOf(twoZones, twoNodesPerZone), theWorkloadLineRefused},
			cause:   "pod.TopologySpreadConstraint leaves out whenUnsatisfiable",
		},
		"a spread leaving out which Nodes' taints count": {
			declare: declaringAWorkloadCustomizedWith(leavingOut(aSpread(), func(left *pod.TopologySpreadConstraint) { left.NodeTaintsPolicy = "" })),
			steps:   []Step{topologyStepOf(twoZones, twoNodesPerZone), theWorkloadLineRefused},
			cause:   "pod.TopologySpreadConstraint leaves out nodeTaintsPolicy",
		},
		"a request leaving out its cpu": {
			declare: declaringAWorkloadCustomizedWith(leavingOut(aRequest(), func(left *pod.Request) { left.CPU = 0 })),
			steps:   []Step{topologyStepOf(twoZones, twoNodesPerZone), theWorkloadLineRefused},
			cause:   "pod.Request leaves out cpu",
		},
		"a request leaving out its memory": {
			declare: declaringAWorkloadCustomizedWith(leavingOut(aRequest(), func(left *pod.Request) { left.Memory = 0 })),
			steps:   []Step{topologyStepOf(twoZones, twoNodesPerZone), theWorkloadLineRefused},
			cause:   "pod.Request leaves out memory",
		},
		"a Node capacity leaving out its cpu": {
			declare: declaringATopologyOf(leavingOut(aNodeCapacity(), func(left *node.Capacity) { left.CPU = 0 })),
			steps: []Step{topologyStepHoldingCapacity(twoZones, twoNodesPerZone,
				fmt.Sprintf("%s cpu, %s memory and %d Pods", noneSpelled, someMemorySpelled, twoPodsPerNode))},
			cause: fmt.Sprintf("a capacity of %s cpu is refused", noneSpelled),
		},
		"a Node capacity leaving out its memory": {
			declare: declaringATopologyOf(leavingOut(aNodeCapacity(), func(left *node.Capacity) { left.Memory = 0 })),
			steps: []Step{topologyStepHoldingCapacity(twoZones, twoNodesPerZone,
				fmt.Sprintf("%s cpu, %s memory and %d Pods", someCPUSpelled, noneSpelled, twoPodsPerNode))},
			cause: fmt.Sprintf("a capacity of %s memory is refused", noneSpelled),
		},
		"a Node capacity leaving out its Pods": {
			declare: declaringATopologyOf(leavingOut(aNodeCapacity(), func(left *node.Capacity) { left.Pods = 0 })),
			steps:   []Step{topologyStepHoldingOf(twoZones, twoNodesPerZone, noPods)},
			cause:   fmt.Sprintf("a capacity of %d Pods is refused", noPods),
		},
	}

	for stated, setting := range refused {
		t.Run(stated, func(t *testing.T) {
			ending := aScenario()

			setting.declare(ending)
			ending.Then().AllPodsRunning()

			ended := ending.outcome()
			expect.Expect(t, ended.Steps).To(expect.Equal(setting.steps))
			expect.Expect(t, ended.EndedAt).To(expect.Equal(setting.steps[len(setting.steps)-1]))
			causeNames(t, ended, setting.cause)
		})
	}
}

// aToleration, aSpread, aRequest and aNodeCapacity are each setting the lab
// offers with every field stated, which the refusals above leave one field out
// of; what each field holds is no matter to them.
func aToleration() pod.Toleration {
	return pod.Toleration{Key: node.NotReady, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: theLabsWaitInSeconds()}
}

func aSpread() pod.TopologySpreadConstraint {
	return pod.TopologySpreadConstraint{
		TopologyKey: node.Zone, MaxSkew: aWiderSkew, WhenUnsatisfiable: pod.DoNotSchedule, NodeTaintsPolicy: pod.Honor,
	}
}

func aRequest() pod.Request {
	return pod.Request{CPU: oneCorePerPod, Memory: twoGibibytesPerPod * pod.Gi}
}

func aNodeCapacity() node.Capacity {
	return node.Capacity{CPU: someCoresPerNode, Memory: someGibibytesPerNode * node.Gi, Pods: twoPodsPerNode}
}

// leavingOut is a setting with the one field out names set to its zero value,
// which is how a keyed literal leaving it out reads.
func leavingOut[S any](stated S, out func(*S)) S {
	out(&stated)

	return stated
}

// declaringAWorkloadCustomizedWith declares the Topology every Workload line of
// this file runs on, then a Workload of Resilient placement customized with the
// setting, and declaringATopologyOf declares a Topology of Nodes holding the
// capacity.
func declaringAWorkloadCustomizedWith(setting pod.Setting) func(*Scenario) {
	return func(declaring *Scenario) {
		declaring.Given().Topology(Zones(twoZones), theGeneralNodePool())
		declaring.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), resilientPlacement().Customize(setting))
	}
}

func declaringATopologyOf(capacity node.Capacity) func(*Scenario) {
	return func(declaring *Scenario) {
		declaring.Given().Topology(Zones(twoZones), node.Pool(generalNodePool, node.PerZone(twoNodesPerZone), capacity))
	}
}

// A Workload's Given line names the Startup time its Pods take after the name of
// the specification it declares: a range as its shortest and its longest, and one
// time every Pod takes once (docs/prd.md REQ-SIM-012).
func TestAWorkloadsGivenLine_ShouldNameTheStartupTimeItsPodsTake(t *testing.T) {
	const oneStartupMillis = 2000

	written := map[string]struct {
		declares pod.Specification
		reads    string
	}{
		"a shortest and a longest": {
			declares: resilientPlacement(),
			reads:    resilientPlacementReads(),
		},
		"one time every Pod takes": {
			declares: resilientPlacement().Customize(pod.StartupMillis(oneStartupMillis)),
			reads:    fmt.Sprintf("%s, each Pod starting in %s", resilientPlacementNamed, oneStartupMillis*time.Millisecond),
		},
	}

	for how, taken := range written {
		t.Run(how, func(t *testing.T) {
			declaring := aScenario()

			declaring.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), taken.declares)

			expect.Expect(t, declaring.outcome().Steps).
				To(expect.Equal([]Step{carrying(workloadStepOf(podsOverEveryNode, taken.reads), resilientPlacementCarries()...)}))
		})
	}
}

// A Workload's Given reports every setting its Pods carry under its Kubernetes
// name, in the order the domain holds them: a Named setting as the settings it
// stands for, a pod.Customization as it customized them, and each full access as one
// line after the settings (docs/prd.md REQ-SIM-016). The name of the Step stays
// as it was, so a Finding naming it stays short.
func TestAWorkloadsGivenLine_ShouldReportEverySettingItsPodsCarryUnderItsKubernetesName(t *testing.T) {
	const aWaitOfItsOwnSeconds = 300

	written := map[string]struct {
		declares pod.Specification
		carries  []string
	}{
		"a predefined specification": {
			declares: resilientPlacement(),
			carries:  resilientPlacementCarries(),
		},
		"Default placement, holding the two tolerations alone": {
			declares: defaultPlacement(),
			carries:  theLeavingLines(),
		},
		"a customization writing another time for one of the two tolerations": {
			declares: resilientPlacement().Customize(
				pod.Toleration{Key: node.NotReady, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: aWaitOfItsOwnSeconds}),
			carries: resilientPlacementCarries(),
		},
	}

	for how, taken := range written {
		t.Run(how, func(t *testing.T) {
			declaring := aScenario()

			declaring.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), taken.declares)

			expect.Expect(t, declaring.outcome().Steps[0].Settings).To(expect.Equal(taken.carries))
		})
	}
}

// A Workload with a pod.Customization reports the settings as customized: the zone
// spread replaced in place, the request appended and each full access as one
// line after the settings (docs/prd.md REQ-SIM-016).
func TestACustomizedWorkloadsGivenLine_ShouldReportTheSettingsAsCustomized(t *testing.T) {
	declaring := aScenario()

	declaring.Given().Workload(someWorkloadName, Pods(podsOverEveryNode),
		resilientPlacement().Customize(aZoneSpreadAt(aWiderSkew), aRequestOf(oneCorePerPod, twoGibibytesPerPod), aFullAccess()))

	expect.Expect(t, declaring.outcome().Steps[0].Settings).To(expect.Equal(append(theLeavingLines(),
		aSpreadLine(scenario.ZoneLabel, aWiderSkew), aSpreadLine(scenario.HostnameLabel, anEvenSkew),
		aRequestLine(oneCorePerPod, twoGibibytesPerPod), aFullAccessLine)))
}

// A NodePool naming a Named Node capacity holds the cpu, memory and Pod count
// its definition gives, whatever name the definition gives it (docs/prd.md
// REQ-SIM-015), and its line names it beside the values it stands for.
func TestANodePoolNamingACapacityItsScenarioDefines_ShouldProvisionNodesWithThatDefinitionsCapacity(t *testing.T) {
	declaring := aScenario()

	declaring.Given().Topology(Zones(twoZones), node.Pool(generalNodePool, node.PerZone(twoNodesPerZone), roomyNodes()))

	expect.Expect(t, declaring.topology.Nodes()).To(expect.Equal(aTopologyOf(t, twoZones, twoNodesPerZone).Nodes()))
}

func TestAGivenTopologyNamingANamedNodeCapacity_ShouldReportTheNameAndWhatItStandsFor(t *testing.T) {
	declaring := aScenario()

	declaring.Given().Topology(Zones(twoZones), node.Pool(generalNodePool, node.PerZone(twoNodesPerZone), roomyNodes()))

	expect.Expect(t, declaring.outcome().Steps).To(expect.Equal([]Step{topologyStepOf(twoZones, twoNodesPerZone)}))
}

func TestAGivenTopologyNamingItsNodeCapacity_ShouldDeclareThatCapacityAndRenameItsLine(t *testing.T) {
	declaring := aScenario()

	declaring.Given().Topology(Zones(twoZones), node.Pool(generalNodePool, node.PerZone(twoNodesPerZone),
		node.Capacity{CPU: someCoresPerNode, Memory: someGibibytesPerNode * node.Gi, Pods: twoPodsPerNode}))

	declared := declaring.outcome()
	expect.Expect(t, declaring.topology.Nodes()).
		To(expect.Equal(aTopologyAt(t, twoZones, twoNodesPerZone, twoPodsPerNode).Nodes()))
	expect.Expect(t, declared.Steps).
		To(expect.Equal([]Step{topologyStepHoldingOf(twoZones, twoNodesPerZone, twoPodsPerNode)}))
}

// The NodePools the Topology lines below declare: one named general naming
// Roomy, and one named gpu of fewer, larger Nodes, in the shape of
// docs/prd.md REQ-SIM-013's own example. The gpu NodePool's cpu and memory are
// double what Roomy holds, so its Nodes differ from the other's in both, and
// its Pod count is a value no test here turns on.
const (
	generalNodePool node.PoolName = "general"
	gpuNodePool     node.PoolName = "gpu"
)

const (
	oneGpuNodePerZone  = 1
	gpuCoresPerNode    = 32
	gpuGibibytesNode   = 128
	someGpuPodsPerNode = 10
)

// theGpuNodePool and theGeneralNodePool are those two NodePools as a Topology
// line declares them.
func theGpuNodePool() node.DeclaredPool {
	return node.Pool(gpuNodePool, node.PerZone(oneGpuNodePerZone),
		node.Capacity{CPU: gpuCoresPerNode, Memory: gpuGibibytesNode * node.Gi, Pods: someGpuPodsPerNode})
}

func theGeneralNodePool() node.DeclaredPool {
	return node.Pool(generalNodePool, node.PerZone(twoNodesPerZone), roomyNodes())
}

// aTopologyOfNodePools is the Topology of zones Zones each holding the two
// NodePools above, built by the domain's own constructors, which is what a line
// declaring them declares.
func aTopologyOfNodePools(t *testing.T, zones int) scenario.Topology {
	t.Helper()

	gpuCapacity, err := scenario.NewNodeCapacity(someGpuPodsPerNode, coresOf(gpuCoresPerNode), gibibytesOf(gpuGibibytesNode))
	if err != nil {
		t.Fatalf("build the gpu NodePool's capacity: %v", err)
	}
	general, err := scenario.NewNodePool(string(generalNodePool), twoNodesPerZone, theCapacityRoomyStandsFor(t))
	if err != nil {
		t.Fatalf("build the general NodePool: %v", err)
	}
	gpu, err := scenario.NewNodePool(string(gpuNodePool), oneGpuNodePerZone, gpuCapacity)
	if err != nil {
		t.Fatalf("build the gpu NodePool: %v", err)
	}
	topology, err := scenario.NewTopology(zones, general, gpu)
	if err != nil {
		t.Fatalf("build a Topology of %d Zones of two NodePools: %v", zones, err)
	}

	return topology
}

// topologyStepOfNodePools is the Step a Topology line of those two NodePools
// records: each NodePool in declaration order, by its name, its Node count per
// Zone, and the capacity it names or states, a stated one by its cpu, memory and
// Pod count.
func topologyStepOfNodePools(zones int) Step {
	return Step{Kind: GivenStep, Name: fmt.Sprintf(
		"a Topology of %d Zones of a NodePool %s of %d %s Nodes holding %d cpu, %dGi memory and %d Pods each; "+
			"a NodePool %s of %d Node holding %d cpu, %dGi memory and %d Pods each",
		zones, generalNodePool, twoNodesPerZone, roomyName, roomyCoresPerNode, roomyGibibytesPerNode, roomyPodsPerNode,
		gpuNodePool, oneGpuNodePerZone, gpuCoresPerNode, gpuGibibytesNode, someGpuPodsPerNode)}
}

// A run reports each NodePool on the Topology line that declared it, by the
// capacity it names or states.
func TestARunOfNodePools_ShouldReportEachNodePoolByNameCountAndCapacity(t *testing.T) {
	declaring := aScenario()

	declaring.Given().Topology(Zones(twoZones), theGeneralNodePool(), theGpuNodePool())

	expect.Expect(t, declaring.outcome().Steps).To(expect.Equal([]Step{topologyStepOfNodePools(twoZones)}))
	expect.Expect(t, declaring.topology.Nodes()).To(expect.Equal(aTopologyOfNodePools(t, twoZones).Nodes()))
}

// What a Topology line holds is NodePools alone, each stating its own name and
// capacity, so a line holding none is refused there and the Scenario ends at it. A
// NodePool the domain refuses is carried to the same line; the NodePools the
// domain refuses are its own suite's, this one being the path through the Given
// Step (docs/prd.md REQ-SIM-013 edge cases 1 to 5). A NodePool omitting its
// capacity does not compile, so the zero capacity is the one written here.
func TestARefusedNodePoolDeclaration_ShouldBeRefusedWhereItIsStated(t *testing.T) {
	const noNodePerZone, noName, noCapacityName, noPod = 0, "", node.CapacityName(""), 0
	var noNodeCapacity node.Capacity

	refused := map[string]struct {
		declare func(Given)
		cause   string
	}{
		"nothing held in a Zone": {
			declare: func(given Given) { given.Topology(Zones(twoZones)) },
			cause:   "a Topology of none is refused",
		},
		"a NodePool stating no name": {
			declare: func(given Given) {
				given.Topology(Zones(twoZones), node.Pool(noName, node.PerZone(twoNodesPerZone), roomyNodes()))
			},
			cause: "a NodePool states its name",
		},
		"two NodePools under one name": {
			declare: func(given Given) { given.Topology(Zones(twoZones), theGeneralNodePool(), theGeneralNodePool()) },
			cause:   "two NodePools named \"general\" could not be told apart",
		},
		"a NodePool stating no Node capacity": {
			declare: func(given Given) {
				given.Topology(Zones(twoZones), node.Pool(generalNodePool, node.PerZone(twoNodesPerZone), noNodeCapacity))
			},
			cause: "a NodePool states its Node capacity",
		},
		"a Named Node capacity defined without a name": {
			declare: func(given Given) {
				given.Topology(Zones(twoZones), node.Pool(generalNodePool, node.PerZone(twoNodesPerZone),
					node.Named(noCapacityName, node.Capacity{CPU: roomyCoresPerNode, Memory: roomyGibibytesPerNode * node.Gi, Pods: roomyPodsPerNode})))
			},
			cause: "a Named Node capacity states its name",
		},
		"a Named Node capacity of a value the domain refuses": {
			declare: func(given Given) {
				given.Topology(Zones(twoZones), node.Pool(generalNodePool, node.PerZone(twoNodesPerZone),
					node.Named(roomyName, node.Capacity{CPU: roomyCoresPerNode, Memory: roomyGibibytesPerNode * node.Gi, Pods: noPod})))
			},
			cause: fmt.Sprintf("a capacity of %d Pods is refused", noPod),
		},
		"a NodePool the domain refuses": {
			declare: func(given Given) {
				given.Topology(Zones(twoZones), node.Pool(generalNodePool, node.PerZone(noNodePerZone), roomyNodes()))
			},
			cause: fmt.Sprintf("a count of %d Nodes per Zone is refused", noNodePerZone),
		},
	}

	for line, declaration := range refused {
		t.Run(line, func(t *testing.T) {
			ending := aScenario()

			declaration.declare(ending.Given())
			ending.Then().AllPodsRunning()

			ended := ending.outcome()
			expect.Expect(t, ended.Steps).To(expect.Equal([]Step{ended.EndedAt}))
			expect.Expect(t, ended.EndedAt.Kind).To(expect.Equal(GivenStep))
			causeNames(t, ended, declaration.cause)
		})
	}
}

func TestThenSteps_ShouldAssertThePlacementOutcome(t *testing.T) {
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	judging := aScenarioOver(onePodPerNodeOf(declared))
	judging.Given().Topology(Zones(twoZones), theGeneralNodePool())
	judging.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), resilientPlacement())

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
	judging.Given().Topology(Zones(twoZones), theGeneralNodePool())
	judging.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), resilientPlacement())

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

// A Then counting the Pods left Pending judges what no Node had room for: the
// count the Placement holds is met, and another count is a Finding under the Step
// naming that count while the later Steps still run (docs/prd.md REQ-SIM-011).
func TestAThenCountingThePodsLeftPending_ShouldReportAFindingOnAnotherCount(t *testing.T) {
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	judging := aScenarioOver(onePodLeftPendingBesideOnePerNodeOf(declared))
	judging.Given().Topology(Zones(twoZones), theGeneralNodePool())
	judging.Given().Workload(someWorkloadName, Pods(podsBeyondEveryNode), resilientPlacement())

	judging.Then().PodsPending(onePodLeftPending)
	judging.Then().PodsPending(noPodPending)

	judged := judging.outcome()
	expect.Expect(t, asJudged(judged.Findings)).To(expect.Equal([]judgedFinding{
		{Step: podsPendingStepOf(noPodPending), Mismatching: true},
	}))
	expect.Expect(t, judged.Steps).To(expect.Equal([]Step{
		topologyStepOf(twoZones, twoNodesPerZone),
		resilientPlacementWorkloadStepOf(podsBeyondEveryNode),
		onePodPendingStep(),
		podsPendingStepOf(noPodPending),
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
	judging.Given().Topology(Zones(twoZones), theGeneralNodePool())
	judging.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), resilientPlacement())

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
	judging.Given().Topology(Zones(twoZones), theGeneralNodePool())
	judging.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), resilientPlacement())

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

// The Then that reads a Zone Failure is judged the same way over the Zone axis: a
// Zone whose Nodes have all left the observation holds no Pod, and one still
// holding a Node of that Zone is where a Pod left behind is still counted.
func TestAPlacementHoldingNoPodInTheZonesThatFailed_ShouldRecordNoFinding(t *testing.T) {
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	stopped := theZoneThatStopped(declared)
	judging := aScenarioAfterFailingAZone(everyPodBackOnTheRespondingZonesOf(declared, stopped), stopped)
	judging.Given().Topology(Zones(twoZones), theGeneralNodePool())
	judging.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), resilientPlacement())

	judging.Then().NoPodsInTheFailedZones()
	judging.Then().AllPodsRunning()

	judged := judging.outcome()
	expect.Expect(t, asJudged(judged.Findings)).To(expect.Equal([]judgedFinding{}))
	expect.Expect(t, judged.Steps).To(expect.Equal([]Step{
		topologyStepOf(twoZones, twoNodesPerZone),
		resilientPlacementWorkloadStepOf(podsOverEveryNode),
		noPodsInTheFailedZonesStep(),
		allPodsRunningStep(),
	}))
}

func TestAPlacementStillHoldingAPodInAZoneThatFailed_ShouldReportAFindingAndRunTheLaterSteps(t *testing.T) {
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	stopped := theZoneThatStopped(declared)
	judging := aScenarioAfterFailingAZone(onePodLeftInTheFailedZoneOf(declared, stopped), stopped)
	judging.Given().Topology(Zones(twoZones), theGeneralNodePool())
	judging.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), resilientPlacement())

	judging.Then().NoPodsInTheFailedZones()
	judging.Then().AllPodsRunning()

	judged := judging.outcome()
	expect.Expect(t, asJudged(judged.Findings)).To(expect.Equal([]judgedFinding{
		{Step: noPodsInTheFailedZonesStep(), Mismatching: true},
	}))
	expect.Expect(t, judged.Steps).To(expect.Equal([]Step{
		topologyStepOf(twoZones, twoNodesPerZone),
		resilientPlacementWorkloadStepOf(podsOverEveryNode),
		noPodsInTheFailedZonesStep(),
		allPodsRunningStep(),
	}))
}

// The count at the moment of the Failure is a derivation and not an observation,
// so it is driven here over the Placement the When discards rather than through
// the When, which stops Nodes responding against a cluster
// (docs/adr/2026-10-06-required-capacity-judged-at-two-moments.md). Which Pods a
// Placement reports Running apart from a Node is internal/scenario's own suite's.
func TestAPodOnANodeThatStoppedResponding_ShouldNotCountAsRunningAtTheMomentOfTheFailure(t *testing.T) {
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	stopped := theNodeThatStopped(declared)
	beforeTheFailure := onePodLeftOnTheFailedNodeOf(declared, stopped)
	failing := aScenarioAfterFailing(beforeTheFailure, stopped)

	kept := failing.podsKeptRunning(beforeTheFailure)

	expect.Expect(t, kept).To(expect.Equal(podsOverEveryNode - onePodLeftBehind))
}

// A Required Capacity held through a Failure states what it counted at each
// moment beside the minimum declared, met or not (docs/prd.md REQ-SIM-010).
func TestACapacityThatHoldsThroughAFailure_ShouldStateThePodsCountedAtEachMomentAndRecordNoFinding(t *testing.T) {
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	stopped := theNodeThatStopped(declared)
	judging := aScenarioAfterFailingWhichKept(
		everyPodBackOnTheRespondingNodesOf(declared, stopped), theMinimumKept, stopped)
	judging.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), resilientPlacement()).
		RequiredCapacity(Pods(theMinimumKept))

	judging.Then().RequiredCapacityHeld()
	judging.Then().AllPodsRunning()

	judged := judging.outcome()
	expect.Expect(t, asJudged(judged.Findings)).To(expect.Equal([]judgedFinding{}))
	expect.Expect(t, judged.Steps).To(expect.Equal([]Step{
		workloadStepKeepingOf(podsOverEveryNode, theMinimumKept),
		requiredCapacityStepOf(theMinimumKept, theMinimumKept, podsOverEveryNode),
		allPodsRunningStep(),
	}))
}

func TestACapacityMissedAtOneMoment_ShouldReportAFindingNamingThatMoment(t *testing.T) {
	// Each case misses the minimum at one of the two moments and meets it at the
	// other, so the Finding it reports is the one naming the moment missed.
	cases := []struct {
		name             string
		keptAtTheFailure int
		settledAt        func(scenario.Topology, string) scenario.Placement
		keptOnceSettled  int
		moment           string
	}{
		{
			name:             "the Failure cost more Pods than the minimum leaves",
			keptAtTheFailure: podsShortOfTheMinimum,
			settledAt:        everyPodBackOnTheRespondingNodesOf,
			keptOnceSettled:  podsOverEveryNode,
			moment:           atTheMomentOfTheFailure,
		},
		{
			name:             "the Placement settled below the minimum",
			keptAtTheFailure: theMinimumKept,
			settledAt:        podsRunningShortOfTheMinimumOn,
			keptOnceSettled:  podsShortOfTheMinimum,
			moment:           onceThePlacementHadSettled,
		},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			declared := aTopologyOf(t, twoZones, twoNodesPerZone)
			stopped := theNodeThatStopped(declared)
			judging := aScenarioAfterFailingWhichKept(one.settledAt(declared, stopped), one.keptAtTheFailure, stopped)
			judging.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), resilientPlacement()).
				RequiredCapacity(Pods(theMinimumKept))

			judging.Then().RequiredCapacityHeld()
			judging.Then().NoPodsOnTheFailedNodes()

			judged := judging.outcome()
			missed := requiredCapacityStepOf(theMinimumKept, one.keptAtTheFailure, one.keptOnceSettled)
			expect.Expect(t, asJudged(judged.Findings)).
				To(expect.Equal([]judgedFinding{{Step: missed, Mismatching: true}}))
			expect.Expect(t, momentsNamedIn(judged.Findings)).To(expect.Equal([]string{one.moment}))
			expect.Expect(t, judged.Steps).To(expect.Equal([]Step{
				workloadStepKeepingOf(podsOverEveryNode, theMinimumKept),
				missed,
				noPodsOnTheFailedNodesStep(),
			}))
		})
	}
}

func TestAFailureToleranceWithNothingToJudge_ShouldEndTheScenarioNamingThatThen(t *testing.T) {
	// A Required Capacity is judged through a Failure and against a minimum, so a
	// Scenario missing either has nothing to judge and ends at the Then, which is
	// recorded before it states any count.
	cases := []struct {
		name     string
		declare  func(scenario.Topology, string) *Scenario
		declared Step
		cause    string
	}{
		{
			name: "a minimum no Failure came before",
			declare: func(declared scenario.Topology, _ string) *Scenario {
				ending := aScenarioOver(onePodPerNodeOf(declared))
				ending.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), resilientPlacement()).
					RequiredCapacity(Pods(theMinimumKept))

				return ending
			},
			declared: workloadStepKeepingOf(podsOverEveryNode, theMinimumKept),
			cause:    theWhenThatNeverRan,
		},
		{
			name: "a Failure the Workload declared no minimum for",
			declare: func(declared scenario.Topology, stopped string) *Scenario {
				ending := aScenarioAfterFailing(everyPodBackOnTheRespondingNodesOf(declared, stopped), stopped)
				ending.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), resilientPlacement())

				return ending
			},
			declared: resilientPlacementWorkloadStepOf(podsOverEveryNode),
			cause:    theMinimumNeverDeclared,
		},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			declared := aTopologyOf(t, twoZones, twoNodesPerZone)
			ending := one.declare(declared, theNodeThatStopped(declared))

			ending.Then().RequiredCapacityHeld()
			ending.Then().AllPodsRunning()

			ended := ending.outcome()
			expect.Expect(t, ended.EndedAt).To(expect.Equal(requiredCapacityStep()))
			expect.Expect(t, ended.Steps).To(expect.Equal([]Step{one.declared, requiredCapacityStep()}))
			expect.Expect(t, asJudged(ended.Findings)).To(expect.Equal([]judgedFinding{}))
			causeNames(t, ended, one.cause)
		})
	}
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
				ending.Given().Topology(Zones(twoZones), node.Pool(generalNodePool, node.PerZone(noNodesPerZone), roomyNodes()))
				ending.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), resilientPlacement())
			},
			endedAt: topologyStepOf(twoZones, noNodesPerZone),
			steps:   []Step{topologyStepOf(twoZones, noNodesPerZone)},
			cause:   fmt.Sprintf("a Zone holds at least one Node, so a count of %d Nodes per Zone is refused", noNodesPerZone),
		},
		{
			name: "a Topology whose Nodes hold no Pod",
			declare: func(ending *Scenario) {
				ending.Given().Topology(Zones(twoZones), node.Pool(generalNodePool, node.PerZone(twoNodesPerZone),
					node.Capacity{CPU: someCoresPerNode, Memory: someGibibytesPerNode * node.Gi, Pods: noPods}))
				ending.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), resilientPlacement())
			},
			endedAt: topologyStepHoldingOf(twoZones, twoNodesPerZone, noPods),
			steps:   []Step{topologyStepHoldingOf(twoZones, twoNodesPerZone, noPods)},
			cause:   fmt.Sprintf("a Node holds at least one Pod, so a capacity of %d Pods is refused", noPods),
		},
		{
			name: "a Workload of no Pods",
			declare: func(ending *Scenario) {
				ending.Given().Topology(Zones(twoZones), theGeneralNodePool())
				ending.Given().Workload(someWorkloadName, Pods(noPods), resilientPlacement())
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
				ending.Given().Topology(Zones(twoZones), theGeneralNodePool())
				ending.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), pod.Specification{})
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

// A Then judges the Placement a Wait reached, so a Then with no Wait between it
// and the Givens, or between it and the Failure that discarded the last
// Placement, has nothing settled to judge and the Scenario ends there
// (docs/prd.md REQ-SIM-006).
func TestAThenWithNoWaitBeforeIt_ShouldEndTheScenarioNamingThatThen(t *testing.T) {
	cases := map[string]func(scenario.Topology) *Scenario{
		"no Wait since the Givens": func(_ scenario.Topology) *Scenario {
			return aScenario()
		},
		"no Wait since the Failure": func(declared scenario.Topology) *Scenario {
			stopped := theNodeThatStopped(declared)

			return aScenarioAfterFailingWithNoWaitSince(
				everyPodBackOnTheRespondingNodesOf(declared, stopped), stopped)
		},
	}

	for since, declare := range cases {
		t.Run(since, func(t *testing.T) {
			declared := aTopologyOf(t, twoZones, twoNodesPerZone)
			ending := declare(declared)
			ending.Given().Topology(Zones(twoZones), theGeneralNodePool())
			ending.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), resilientPlacement())

			ending.Then().PodsPerZone(podsPerZoneWhenEven)
			ending.Then().AllPodsRunning()

			ended := ending.outcome()
			expect.Expect(t, ended.EndedAt).To(expect.Equal(podsPerZoneStepOf(podsPerZoneWhenEven)))
			expect.Expect(t, ended.Steps).To(expect.Equal([]Step{
				topologyStepOf(twoZones, twoNodesPerZone),
				resilientPlacementWorkloadStepOf(podsOverEveryNode),
				podsPerZoneStepOf(podsPerZoneWhenEven),
			}))
			expect.Expect(t, asJudged(ended.Findings)).To(expect.Equal([]judgedFinding{}))
			causeNames(t, ended, theWaitThatNeverStood)
		})
	}
}

// A Failure counts what the Workload kept Running at the moment it is injected,
// off the Placement the last Wait reached, so a Scenario that has not Waited once
// ends at that When rather than stopping Nodes in a world it has not observed.
func TestAFailureBeforeTheScenarioHasWaited_ShouldEndTheScenarioNamingThatWhen(t *testing.T) {
	ending := aScenario()
	ending.Given().Topology(Zones(twoZones), theGeneralNodePool())
	ending.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), resilientPlacement())

	ending.When().NodesFail(Nodes(oneNodeFailing))
	ending.Then().AllPodsRunning()

	ended := ending.outcome()
	expect.Expect(t, ended.EndedAt).To(expect.Equal(oneNodeFailsStep()))
	expect.Expect(t, ended.Steps).To(expect.Equal([]Step{
		topologyStepOf(twoZones, twoNodesPerZone),
		resilientPlacementWorkloadStepOf(podsOverEveryNode),
		oneNodeFailsStep(),
	}))
	expect.Expect(t, asJudged(ended.Findings)).To(expect.Equal([]judgedFinding{}))
	causeNames(t, ended, theWaitThatNeverStood)
}

// A Scenario's lines are carried out in the order they stand in, so a Then before
// a When judges the Placement the Scenario starts from and the When after it
// injects the Failure into that same settled world. The When here is the
// refused-count one, which resolves no Node and so reaches no cluster.
func TestAThenStandingBeforeAWhen_ShouldJudgeThePlacementTheScenarioStartsFrom(t *testing.T) {
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	judging := aScenarioOver(onePodPerNodeOf(declared))
	judging.Given().Topology(Zones(twoZones), theGeneralNodePool())
	judging.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), resilientPlacement())

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
	ending.Given().Topology(Zones(twoZones), theGeneralNodePool())
	ending.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), resilientPlacement())

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

func TestAWhenThatFailsMoreZonesThanTheTopologyHolds_ShouldEndTheScenarioAtThatStep(t *testing.T) {
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	ending := aScenarioOver(onePodPerNodeOf(declared))
	ending.Given().Topology(Zones(twoZones), theGeneralNodePool())
	ending.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), resilientPlacement())

	ending.When().ZonesFail(Zones(moreZonesThanTheTopologyHolds))
	ending.Then().AllPodsRunning()

	ended := ending.outcome()
	expect.Expect(t, ended.EndedAt).To(expect.Equal(zonesFailStepOf(moreZonesThanTheTopologyHolds)))
	expect.Expect(t, ended.Steps).To(expect.Equal([]Step{
		topologyStepOf(twoZones, twoNodesPerZone),
		resilientPlacementWorkloadStepOf(podsOverEveryNode),
		zonesFailStepOf(moreZonesThanTheTopologyHolds),
	}))
	expect.Expect(t, asJudged(ended.Findings)).To(expect.Equal([]judgedFinding{}))
	causeNames(t, ended, fmt.Sprintf("a Failure of %d Zones is refused", moreZonesThanTheTopologyHolds))
}

func TestAThenReadingAFailureWithNoWhenBeforeIt_ShouldEndTheScenarioNamingThatThen(t *testing.T) {
	thens := map[string]struct {
		judge func(*Scenario)
		step  Step
	}{
		"the Nodes that failed": {
			judge: func(ending *Scenario) { ending.Then().NoPodsOnTheFailedNodes() },
			step:  noPodsOnTheFailedNodesStep(),
		},
		"the Zones that failed": {
			judge: func(ending *Scenario) { ending.Then().NoPodsInTheFailedZones() },
			step:  noPodsInTheFailedZonesStep(),
		},
	}

	for read, then := range thens {
		t.Run(read, func(t *testing.T) {
			declared := aTopologyOf(t, twoZones, twoNodesPerZone)
			ending := aScenarioOver(onePodPerNodeOf(declared))
			ending.Given().Topology(Zones(twoZones), theGeneralNodePool())
			ending.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), resilientPlacement())

			then.judge(ending)
			ending.Then().AllPodsRunning()

			ended := ending.outcome()
			expect.Expect(t, ended.EndedAt).To(expect.Equal(then.step))
			expect.Expect(t, ended.Steps).To(expect.Equal([]Step{
				topologyStepOf(twoZones, twoNodesPerZone),
				resilientPlacementWorkloadStepOf(podsOverEveryNode),
				then.step,
			}))
			expect.Expect(t, asJudged(ended.Findings)).To(expect.Equal([]judgedFinding{}))
			causeNames(t, ended, theWhenThatNeverRan)
		})
	}
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

	expected := evenlyOver(declared.Zones(), podsPerZoneWhenEven)

	expect.Expect(t, expected).To(expect.Equal(map[string]int{
		firstZone: podsPerZoneWhenEven,
		lastZone:  podsPerZoneWhenEven,
	}))
}

func TestEveryNodeOfATopology_ShouldBeNamedInTopologyOrder(t *testing.T) {
	const oneZone = 1
	declared := aTopologyOf(t, oneZone, twoNodesPerZone)
	nodes := declared.Nodes()

	named := nodesOf(declared)

	expect.Expect(t, named).To(expect.Equal([]string{nodes[0].Name, nodes[1].Name}))
}
