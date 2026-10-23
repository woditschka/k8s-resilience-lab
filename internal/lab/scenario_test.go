// These tests reach no cluster and no engine: a Given is judged by the
// internal/scenario value it constructs, and a Then by a Placement the test
// built, so no stand-in for the Simulated Cluster exists.
package lab

import (
	"fmt"
	"slices"
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
	// More than one Node per Zone makes an even Placement a decision rather
	// than the only one available.
	twoZones        = 2
	twoNodesPerZone = 2

	podsOverEveryNode = twoZones * twoNodesPerZone

	podsPerZoneWhenEven = podsOverEveryNode / twoZones
	podsPerNodeWhenEven = 1

	// twoPodsPerNode is below roomyPodsPerNode, so a Topology declared at it
	// differs from one naming Roomy. No test here turns on the cpu and memory.
	twoPodsPerNode       = 2
	someCoresPerNode     = 4
	someGibibytesPerNode = 16
	someCPUSpelled       = "4"
	someMemorySpelled    = "16Gi"

	roomForOnePodPerNode = 1
	onePodLeftPending    = 1
	podsBeyondEveryNode  = podsOverEveryNode + onePodLeftPending
	noPodPending         = 0

	noNodeDeclared = 0

	// A Topology of two Zones of two Nodes, so one Node holds one Pod of an
	// even spread and one Zone two.
	floorSurvivingOneNode = podsOverEveryNode - podsPerNodeWhenEven
	floorSurvivingOneZone = podsOverEveryNode - podsPerZoneWhenEven

	oneLost = 1

	// A loss reads as a report line states it, after the count it is written
	// with.
	oneNodeLost   = "1 Node"
	oneZoneLost   = "1 Zone"
	everyZoneLost = "2 Zones"

	noNodesPerZone = 0
	noPods         = 0
	noReplicas     = 0

	moreNodesThanTheTopologyHolds = twoZones*twoNodesPerZone + 1
	moreZonesThanTheTopologyHolds = twoZones + 1

	theWhenThatNeverRan = "no When"

	oneNodeFailing = 1

	theToleranceNeverDeclared = "no Deployment declares one"

	noSpecificationAtAll = ""

	onePodLeftBehind       = 1
	podLeftOnTheFailedNode = string(someDeploymentName) + "-left-behind"

	podLeftPending = string(someDeploymentName) + "-left-pending"

	// aFinelyStatedWait is stated more finely than a report states it, so the
	// rounding is read on it.
	aFinelyStatedWait = 6234 * time.Millisecond

	aTimeAWaitStates         = 15
	aWaitPastTheTimeItStates = 25500 * time.Millisecond

	// A Wait with nothing to wait for takes no measured time, so whether it was
	// reached is read off the Step rather than off this time.
	aWaitOfNoMeasuredTime = 0

	theWaitsOwnStep = 0

	theWaitingTimeInSeconds = int(scenario.WaitingTime / time.Second)

	theSecondsAboveTheLabsWaitingTime = theWaitingTimeInSeconds + 1
	noSecondsAtAll                    = 0

	// someDeploymentName is named here rather than taken from the Scenarios'
	// place, which internal/lab does not import.
	someDeploymentName DeploymentName = "some-deployment"
)

func aWaitStating(t *testing.T, seconds int) scenario.StatedWait {
	t.Helper()

	stated, err := scenario.NewStatedWait(seconds)
	if err != nil {
		t.Fatalf("build the time a Wait of %d seconds is held to: %v", seconds, err)
	}

	return stated
}

func aScenario() *Scenario {
	return &Scenario{}
}

func aScenarioOver(placed scenario.Placement) *Scenario {
	return &Scenario{observed: placed, settled: true}
}

func aScenarioAfterFailing(placed scenario.Placement, stopped ...string) *Scenario {
	return &Scenario{observed: placed, settled: true, failed: stopped}
}

func aScenarioAtAWaitFor(state string) *Scenario {
	waiting := aScenario()
	waiting.record(WaitStep, state)

	return waiting
}

func aScenarioAfterFailingWithNoWaitSince(placed scenario.Placement, stopped string) *Scenario {
	return &Scenario{observed: placed, failed: []string{stopped}}
}

// The copy stands for the Timeline a recording delivered, so a Then judging
// every Floor reads it without a cluster.
func aScenarioRecording(stopped []string, recorded ...scenario.Entry) *Scenario {
	return &Scenario{stoppedPerWhen: [][]string{stopped}, recorded: &timelineCopy{copied: recorded}}
}

func aScenarioAfterFailingAZone(placed scenario.Placement, stopped string) *Scenario {
	return &Scenario{observed: placed, settled: true, failedZones: []string{stopped}}
}

// The lab defines no Named Node capacity, so this suite defines its own. Its
// Pod count leaves room for more than one Pod per Node.
const roomyName node.CapacityName = "Roomy"

const (
	roomyCoresPerNode     = 16
	roomyGibibytesPerNode = 64
	roomyPodsPerNode      = 10
)

func roomyNodes() node.Capacity {
	return node.Named(roomyName, node.Capacity{CPU: roomyCoresPerNode, Memory: roomyGibibytesPerNode * node.Gi, Pods: roomyPodsPerNode})
}

func aTopologyOf(t *testing.T, zones, nodesPerZone int) scenario.Topology {
	t.Helper()

	return aTopologyHolding(t, zones, nodesPerZone, theCapacityRoomyStandsFor(t))
}

func theCapacityRoomyStandsFor(t *testing.T) scenario.NodeCapacity {
	t.Helper()

	capacity, err := scenario.NewNodeCapacity(roomyPodsPerNode, coresOf(roomyCoresPerNode), gibibytesOf(roomyGibibytesPerNode))
	if err != nil {
		t.Fatalf("build the capacity Roomy stands for: %v", err)
	}

	return capacity
}

func aTopologyAt(t *testing.T, zones, nodesPerZone, podsPerNode int) scenario.Topology {
	t.Helper()

	capacity, err := scenario.NewNodeCapacity(podsPerNode, coresOf(someCoresPerNode), gibibytesOf(someGibibytesPerNode))
	if err != nil {
		t.Fatalf("build a Node capacity of %d Pods: %v", podsPerNode, err)
	}

	return aTopologyHolding(t, zones, nodesPerZone, capacity)
}

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

func aDeploymentDeclaringResilientPlacementOf(t *testing.T, pods int) scenario.Deployment {
	t.Helper()

	specification, err := resilientPlacement().Declared()
	if err != nil {
		t.Fatalf("define Resilient placement: %v", err)
	}
	deployment, err := scenario.NewDeployment(string(someDeploymentName), pods, specification)
	if err != nil {
		t.Fatalf("build a Deployment of %d replicas declaring Resilient placement: %v", pods, err)
	}

	return deployment
}

func aDeploymentSurvivingTheLossOf(t *testing.T, pods int, lost FailureTolerance, against scenario.Topology) scenario.Deployment {
	t.Helper()

	return aDeploymentNamedSurvivingTheLossOf(t, someDeploymentName, pods, lost, against)
}

func aDeploymentNamedSurvivingTheLossOf(t *testing.T, name DeploymentName, pods int, lost FailureTolerance,
	against scenario.Topology,
) scenario.Deployment {
	t.Helper()

	specification, err := resilientPlacement().Declared()
	if err != nil {
		t.Fatalf("define Resilient placement: %v", err)
	}
	declared, err := scenario.NewDeployment(string(name), pods, specification)
	if err != nil {
		t.Fatalf("build a Deployment %s of %d replicas declaring Resilient placement: %v", name, pods, err)
	}
	deployment, err := declared.WithFailureTolerance(lost.tolerance, against)
	if err != nil {
		t.Fatalf("declare a Failure tolerance of %s: %v", lost.tolerance.Reads(), err)
	}

	return deployment
}

func onePodPerNodeOf(topology scenario.Topology) scenario.Placement {
	nodes := topology.Nodes()
	placed := make([]scenario.Pod, 0, len(nodes))
	for index, node := range nodes {
		placed = append(placed, scenario.Pod{
			Name:  fmt.Sprintf("%s-%d", someDeploymentName, index),
			Node:  node.Name,
			Phase: scenario.PodRunning,
		})
	}

	return scenario.NewPlacement(nodes, placed)
}

func onePodLeftPendingBesideOnePerNodeOf(topology scenario.Topology) scenario.Placement {
	placed := onePodPerNodeOf(topology).Pods()
	pending := scenario.Pod{Name: podLeftPending, Phase: scenario.PodPending}

	return scenario.NewPlacement(topology.Nodes(), append(placed, pending))
}

// The first Node the Topology derives is the pick internal/scenario makes.
func theNodeThatStopped(topology scenario.Topology) string {
	return topology.Nodes()[0].Name
}

// The stopped Node is no Ready Node, so the observation is read against the
// rest.
func everyPodBackOnTheRespondingNodesOf(topology scenario.Topology, stopped string) scenario.Placement {
	responding := theNodesStillRespondingIn(topology, stopped)

	return scenario.NewPlacement(responding, podsRunningOver(responding, podsOverEveryNode))
}

// The left-behind Pod is counted under that Node's name, because the report
// states what was observed.
func onePodLeftOnTheFailedNodeOf(topology scenario.Topology, stopped string) scenario.Placement {
	responding := theNodesStillRespondingIn(topology, stopped)
	moved := podsRunningOver(responding, podsOverEveryNode-onePodLeftBehind)
	leftBehind := scenario.Pod{Name: podLeftOnTheFailedNode, Node: stopped, Phase: scenario.PodRunning}

	return scenario.NewPlacement(responding, append(moved, leftBehind))
}

// The first Zone the Topology derives is the pick internal/scenario makes.
func theZoneThatStopped(topology scenario.Topology) string {
	return topology.Zones()[0]
}

// The stopped Zone's Nodes are no Ready Nodes, so that Zone is in the
// observation no longer.
func everyPodBackOnTheRespondingZonesOf(topology scenario.Topology, stopped string) scenario.Placement {
	responding := theNodesOutside(topology, stopped)

	return scenario.NewPlacement(responding, podsRunningOver(responding, podsOverEveryNode))
}

// A Zone's Nodes stop one after another, so an observation taken between them
// reads against one of them still.
func onePodLeftInTheFailedZoneOf(topology scenario.Topology, stopped string) scenario.Placement {
	responding := theNodesOutside(topology, stopped)
	moved := podsRunningOver(responding, podsOverEveryNode-onePodLeftBehind)
	stillReady := theFirstNodeIn(topology, stopped)
	leftBehind := scenario.Pod{Name: podLeftOnTheFailedNode, Node: stillReady.Name, Phase: scenario.PodRunning}

	return scenario.NewPlacement(append(responding, stillReady), append(moved, leftBehind))
}

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

// A Node that stopped responding leaves the observation at once.
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

// A Node takes a second Pod only once every Node holds one.
func podsRunningOver(nodes []scenario.Node, pods int) []scenario.Pod {
	placed := make([]scenario.Pod, 0, pods)
	for at := range pods {
		placed = append(placed, scenario.Pod{
			Name:  fmt.Sprintf("%s-%d", someDeploymentName, at),
			Node:  nodes[at%len(nodes)].Name,
			Phase: scenario.PodRunning,
		})
	}

	return placed
}

func everyPodOnOneNodeOf(topology scenario.Topology) scenario.Placement {
	nodes := topology.Nodes()
	crowded := onePodPerNodeOf(topology).Pods()
	for index := range crowded {
		crowded[index].Node = nodes[0].Name
	}

	return scenario.NewPlacement(nodes, crowded)
}

// A Step's name is what a report shows and what a Finding points at, so this
// suite states it.
func topologyStepOf(zones, nodesPerZone int) Step {
	return Step{
		Kind: GivenStep,
		Name: fmt.Sprintf("a Topology of %d Zones of a NodePool %s of %d %s Nodes holding %d cpu, %dGi memory and %d Pods each",
			zones, generalNodePool, nodesPerZone, roomyName, roomyCoresPerNode, roomyGibibytesPerNode, roomyPodsPerNode),
	}
}

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

// One template, so the lines cannot drift apart here.
func topologyStepHoldingCapacity(zones, nodesPerZone int, capacity string) Step {
	return Step{
		Kind: GivenStep,
		Name: fmt.Sprintf("a Topology of %d Zones of a NodePool %s of %d Nodes holding %s each",
			zones, generalNodePool, nodesPerZone, capacity),
	}
}

func deploymentStepOf(pods int, declaring string) Step {
	return deploymentStepNamedOf(someDeploymentName, pods, declaring)
}

func deploymentStepNamedOf(name DeploymentName, pods int, declaring string) Step {
	return Step{
		Kind: GivenStep,
		Name: fmt.Sprintf("a Deployment %s of %d replicas declaring %s", name, pods, declaring),
	}
}

func resilientPlacementDeploymentStepOf(pods int) Step {
	return resilientPlacementDeploymentStepNamed(someDeploymentName, pods)
}

func resilientPlacementDeploymentStepNamed(name DeploymentName, pods int) Step {
	return carrying(deploymentStepNamedOf(name, pods, resilientPlacementReads()), resilientPlacementCarries()...)
}

func resilientPlacementReads() string {
	return fmt.Sprintf("%s, each Pod starting in %s to %s", resilientPlacementNamed,
		someShortestStartupMillis*time.Millisecond, someLongestStartupMillis*time.Millisecond)
}

func carrying(step Step, settings ...string) Step {
	step.Settings = settings

	return step
}

// A toleration is reported at the lab's waiting time, the one time the domain
// holds it at.
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

const aFullAccessLine = "a full access to the Kubernetes Pod specification applies"

func theLeavingLines() []string {
	return []string{aTolerationLine(scenario.TaintNodeNotReady), aTolerationLine(scenario.TaintNodeUnreachable)}
}

func resilientPlacementCarries() []string {
	return append(theLeavingLines(), aSpreadLine(scenario.ZoneLabel, anEvenSkew), aSpreadLine(scenario.HostnameLabel, anEvenSkew))
}

// Naming the Failure tolerance renames the line rather than adding one.
func deploymentStepSurvivingTheLossOf(pods int, lost string) Step {
	return namedDeploymentStepSurvivingTheLossOf(someDeploymentName, pods, lost)
}

func namedDeploymentStepSurvivingTheLossOf(name DeploymentName, pods int, lost string) Step {
	surviving := resilientPlacementDeploymentStepNamed(name, pods)
	surviving.Name = fmt.Sprintf("%s, surviving the loss of %s", surviving.Name, lost)

	return surviving
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

// Once a When resolves what it stops it renames its line, which only a run
// against a cluster reaches.
func nodesFailStepOf(nodes int) Step {
	return Step{Kind: WhenStep, Name: fmt.Sprintf("%d Nodes fail", nodes)}
}

func oneNodeFailsStep() Step {
	return Step{Kind: WhenStep, Name: fmt.Sprintf("%d Node fails", oneNodeFailing)}
}

func zonesFailStepOf(zones int) Step {
	return Step{Kind: WhenStep, Name: fmt.Sprintf("%d Zones fail", zones)}
}

func allNodesReadyStep() Step {
	return Step{Kind: WaitStep, Name: allNodesReady}
}

func allPodsReadyStep() Step {
	return Step{Kind: WaitStep, Name: allPodsReady}
}

func settledPlacementStep() Step {
	return Step{Kind: WaitStep, Name: theSettledPlacement}
}

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

// enoughPodsStayedReadyStep is also the line a Scenario with nothing to judge
// ends at, recorded before any reading.
func enoughPodsStayedReadyStep(readings ...string) Step {
	return Step{Kind: ThenStep, Name: "enough Pods stayed Ready from the first Failure", Readings: readings}
}

// The mismatch text is internal/expect's and is not this suite's to pin.
type judgedFinding struct {
	Step        Step
	Mismatching bool
}

func asJudged(findings []Finding) []judgedFinding {
	judged := make([]judgedFinding, 0, len(findings))
	for _, finding := range findings {
		judged = append(judged, judgedFinding{Step: finding.Step, Mismatching: finding.Mismatch != ""})
	}

	return judged
}

// A reader of a restore time must never read one time for the other.
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

func TestAStepThatIsNotAWait_ShouldReadAsItsKindAndNameAlone(t *testing.T) {
	declared := topologyStepOf(twoZones, twoNodesPerZone)

	expect.Expect(t, declared.String()).To(expect.Equal("Given " + declared.Name))
}

// The time it took is left off, because what a reader acts on is the time the
// Scenario called and missed.
func TestAWaitNotReachedInTheTimeItStates_ShouldEndTheScenarioNamingThatTimeAlone(t *testing.T) {
	ending := aScenarioAtAWaitFor(allPodsReady)

	ending.reached(aWaitStating(t, aTimeAWaitStates), aWaitPastTheTimeItStates)

	ended := ending.outcome()
	expect.Expect(t, ended.EndedAt.String()).
		To(expect.Equal("Wait for " + allPodsReady + ", not reached in the " + secondsRead(aTimeAWaitStates) + " stated"))
	causeNames(t, ended, theWaitReachedTooLate)
}

// The refusal stands before the Wait applies anything, so these Scenarios reach
// no cluster, and each board lists the declared Nodes with no Pod on them.
func TestAWaitStatingATimeWithItsGivenAndOneOnItsOwn_ShouldEachBeHeldToTheTimeItStates(t *testing.T) {
	everyDeclaredNodeAndNoPod := scenario.NewPlacementBoard(aTopologyOf(t, twoZones, twoNodesPerZone), scenario.Placement{}, scenario.PlacementBoard{}).Lines()
	var noNodeLine []string
	waits := map[string]struct {
		write func(*Scenario)
		named string
		step  Step
		board []string
	}{
		"chained onto the Topology it declares": {
			write: func(ending *Scenario) {
				ending.Given().Zones(twoZones, theGeneralNodePool()).
					WaitFor().AllNodesReadyWithinSeconds(theSecondsAboveTheLabsWaitingTime)
			},
			named: secondsRead(theSecondsAboveTheLabsWaitingTime),
			step:  allNodesReadyStep(),
			board: everyDeclaredNodeAndNoPod,
		},
		"chained onto the Deployment it declares": {
			write: func(ending *Scenario) {
				ending.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement()).
					WaitFor().AllPodsReadyWithinSeconds(noSecondsAtAll)
			},
			named: strconv.Itoa(noSecondsAtAll),
			step:  allPodsReadyStep(),
			board: noNodeLine,
		},
		"a Wait Step of its own for the Nodes": {
			write: func(ending *Scenario) {
				ending.WaitFor().AllNodesReadyWithinSeconds(noSecondsAtAll)
			},
			named: strconv.Itoa(noSecondsAtAll),
			step:  allNodesReadyStep(),
			board: noNodeLine,
		},
		"a Wait Step of its own for the Pods": {
			write: func(ending *Scenario) {
				ending.WaitFor().AllPodsReadyWithinSeconds(theSecondsAboveTheLabsWaitingTime)
			},
			named: secondsRead(theSecondsAboveTheLabsWaitingTime),
			step:  allPodsReadyStep(),
			board: noNodeLine,
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
			expect.Expect(t, ended.Steps[len(ended.Steps)-1].Board.Lines()).To(expect.Equal(wait.board))
		})
	}
}

func secondsRead(seconds int) string {
	return (time.Duration(seconds) * time.Second).String()
}

// What a Given declared is applied by the Wait after it, so these lines run
// against a Scenario holding no Simulated Cluster at all.
func TestAGivenThatIsNotAskedToWait_ShouldDeclareWithoutReachingTheCluster(t *testing.T) {
	declaring := aScenario()

	declaring.Given().Zones(twoZones, theGeneralNodePool())
	declaring.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())

	declared := declaring.outcome()
	expect.Expect(t, declaring.topology.Nodes()).To(expect.Equal(aTopologyOf(t, twoZones, twoNodesPerZone).Nodes()))
	expect.Expect(t, declaring.deployment).To(expect.Equal(aDeploymentDeclaringResilientPlacementOf(t, podsOverEveryNode)))
	expect.Expect(t, declared.Steps).To(expect.Equal([]Step{
		topologyStepOf(twoZones, twoNodesPerZone),
		resilientPlacementDeploymentStepOf(podsOverEveryNode),
	}))
}

// The domain's Deployment holds the specification's settings alone, so the full
// accesses must be kept beside it.
func TestAGivenDeploymentCustomizedWithFullAccess_ShouldKeepItBesideTheDeploymentItDeclared(t *testing.T) {
	declaring := aScenario()
	customized := resilientPlacement().Customize(aFullAccess())

	declaring.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), customized)

	expect.Expect(t, placesOfTheFullAccesses(declaring.accesses)).
		To(expect.Equal(placesOfTheFullAccesses(customized.Accesses())))
}

func TestAGivenDeploymentDeclaringAFailureTolerance_ShouldSetItsFloorFromTheTopologyDeclaredAboveIt(t *testing.T) {
	const twoNodesLost = 2
	cases := map[string]struct {
		lost  FailureTolerance
		reads string
	}{
		"one Zone":  {lost: Zones(oneLost), reads: oneZoneLost},
		"two Nodes": {lost: Nodes(twoNodesLost), reads: "2 Nodes"},
	}

	for named, one := range cases {
		t.Run(named, func(t *testing.T) {
			declaring := aScenario()
			declaring.Given().Zones(twoZones, theGeneralNodePool())

			declaring.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement()).
				SurvivesLosing(one.lost)

			declared := declaring.outcome()
			expect.Expect(t, declaring.deployment).To(expect.Equal(
				aDeploymentSurvivingTheLossOf(t, podsOverEveryNode, one.lost, aTopologyOf(t, twoZones, twoNodesPerZone))))
			expect.Expect(t, declared.Steps).To(expect.Equal([]Step{
				topologyStepOf(twoZones, twoNodesPerZone),
				deploymentStepSurvivingTheLossOf(podsOverEveryNode, one.reads),
			}))
		})
	}
}

// The losses the domain refuses are its own suite's; this is the path through
// the Given Step.
func TestALossADeploymentCannotDeclare_ShouldEndTheScenarioNamingThatGiven(t *testing.T) {
	ending := aScenarioOver(onePodPerNodeOf(aTopologyOf(t, twoZones, twoNodesPerZone)))
	ending.Given().Zones(twoZones, theGeneralNodePool())
	ending.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement()).
		SurvivesLosing(Zones(twoZones))

	ending.Then().AllPodsRunning()

	ended := ending.outcome()
	refused := deploymentStepSurvivingTheLossOf(podsOverEveryNode, everyZoneLost)
	expect.Expect(t, ended.EndedAt).To(expect.Equal(refused))
	expect.Expect(t, ended.Steps).To(expect.Equal([]Step{topologyStepOf(twoZones, twoNodesPerZone), refused}))
	expect.Expect(t, asJudged(ended.Findings)).To(expect.Equal([]judgedFinding{}))
	causeNames(t, ended, "a Failure tolerance of "+everyZoneLost+" is refused")
}

// The times the domain refuses are its own suite's; this is the path through
// the Given Step.
func TestARefusedStartupTime_ShouldEndTheScenarioAtTheGivenDeclaringItsDeployment(t *testing.T) {
	const aShortestAboveItsLongestMillis, itsLongestMillis = 900, 400
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	ending := aScenarioOver(onePodPerNodeOf(declared))
	ending.Given().Zones(twoZones, theGeneralNodePool())
	ending.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(),
		resilientPlacement().Customize(pod.StartupMillisBetween(aShortestAboveItsLongestMillis, itsLongestMillis)))

	ending.Then().AllPodsRunning()

	ended := ending.outcome()
	refused := deploymentStepOf(podsOverEveryNode, string(resilientPlacementNamed))
	expect.Expect(t, ended.EndedAt).To(expect.Equal(refused))
	expect.Expect(t, ended.Steps).To(expect.Equal([]Step{topologyStepOf(twoZones, twoNodesPerZone), refused}))
	causeNames(t, ended, fmt.Sprintf("shortest Startup time of %s", aShortestAboveItsLongestMillis*time.Millisecond))
}

// A definition leaving out its Startup time does not compile, so the one way
// left to write none is the zero Startup time, refused as a definition argument
// and as a Customization.
func TestAPodSpecificationDefinedWithoutAStartupTime_ShouldBeRefusedWhereItIsDefined(t *testing.T) {
	written := map[string]pod.Specification{
		"defined with none":    pod.Predefined(resilientPlacementNamed, pod.StartupTime{}),
		"customized with none": resilientPlacement().Customize(pod.StartupTime{}),
	}

	for how, declares := range written {
		t.Run(how, func(t *testing.T) {
			declared := aTopologyOf(t, twoZones, twoNodesPerZone)
			ending := aScenarioOver(onePodPerNodeOf(declared))
			ending.Given().Zones(twoZones, theGeneralNodePool())

			ending.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), declares)

			ended := ending.outcome()
			expect.Expect(t, ended.EndedAt).To(expect.Equal(deploymentStepOf(podsOverEveryNode, string(resilientPlacementNamed))))
			causeNames(t, ended, "names no Startup time")
		})
	}
}

// The refusals themselves are the domain's suite's.
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
			ending.Given().Zones(twoZones, theGeneralNodePool())

			ending.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), taken.declares)

			ended := ending.outcome()
			expect.Expect(t, ended.EndedAt).To(expect.Equal(deploymentStepOf(podsOverEveryNode, taken.named)))
			causeNames(t, ended, taken.cause)
		})
	}
}

// Every field of a setting is required and its zero value is the field left
// out. A Node capacity leaving out a dimension is refused by the domain at the
// Topology Given.
func TestASettingMissingAValueKubernetesRequires_ShouldBeRefusedWhereItIsStated(t *testing.T) {
	const noneSpelled = "0"
	theDeploymentLineRefused := deploymentStepOf(podsOverEveryNode, string(resilientPlacementNamed))

	refused := map[string]struct {
		declare func(*Scenario)
		steps   []Step
		cause   string
	}{
		"a toleration leaving out its key": {
			declare: declaringADeploymentCustomizedWith(leavingOut(aToleration(), func(left *pod.Toleration) { left.Key = "" })),
			steps:   []Step{topologyStepOf(twoZones, twoNodesPerZone), theDeploymentLineRefused},
			cause:   "pod.Toleration leaves out key",
		},
		"a toleration leaving out its operator": {
			declare: declaringADeploymentCustomizedWith(leavingOut(aToleration(), func(left *pod.Toleration) { left.Operator = "" })),
			steps:   []Step{topologyStepOf(twoZones, twoNodesPerZone), theDeploymentLineRefused},
			cause:   "pod.Toleration leaves out operator",
		},
		"a toleration leaving out its effect": {
			declare: declaringADeploymentCustomizedWith(leavingOut(aToleration(), func(left *pod.Toleration) { left.Effect = "" })),
			steps:   []Step{topologyStepOf(twoZones, twoNodesPerZone), theDeploymentLineRefused},
			cause:   "pod.Toleration leaves out effect",
		},
		"a toleration leaving out its seconds": {
			declare: declaringADeploymentCustomizedWith(leavingOut(aToleration(), func(left *pod.Toleration) { left.TolerationSeconds = 0 })),
			steps:   []Step{topologyStepOf(twoZones, twoNodesPerZone), theDeploymentLineRefused},
			cause:   "pod.Toleration leaves out tolerationSeconds",
		},
		"a spread leaving out its topology key": {
			declare: declaringADeploymentCustomizedWith(leavingOut(aSpread(), func(left *pod.TopologySpreadConstraint) { left.TopologyKey = "" })),
			steps:   []Step{topologyStepOf(twoZones, twoNodesPerZone), theDeploymentLineRefused},
			cause:   "pod.TopologySpreadConstraint leaves out topologyKey",
		},
		"a spread leaving out its skew": {
			declare: declaringADeploymentCustomizedWith(leavingOut(aSpread(), func(left *pod.TopologySpreadConstraint) { left.MaxSkew = 0 })),
			steps:   []Step{topologyStepOf(twoZones, twoNodesPerZone), theDeploymentLineRefused},
			cause:   "pod.TopologySpreadConstraint leaves out maxSkew",
		},
		"a spread leaving out what happens to a Pod no Node can take": {
			declare: declaringADeploymentCustomizedWith(leavingOut(aSpread(), func(left *pod.TopologySpreadConstraint) { left.WhenUnsatisfiable = "" })),
			steps:   []Step{topologyStepOf(twoZones, twoNodesPerZone), theDeploymentLineRefused},
			cause:   "pod.TopologySpreadConstraint leaves out whenUnsatisfiable",
		},
		"a spread leaving out which Nodes' taints count": {
			declare: declaringADeploymentCustomizedWith(leavingOut(aSpread(), func(left *pod.TopologySpreadConstraint) { left.NodeTaintsPolicy = "" })),
			steps:   []Step{topologyStepOf(twoZones, twoNodesPerZone), theDeploymentLineRefused},
			cause:   "pod.TopologySpreadConstraint leaves out nodeTaintsPolicy",
		},
		"a request leaving out its cpu": {
			declare: declaringADeploymentCustomizedWith(leavingOut(aRequest(), func(left *pod.Request) { left.CPU = 0 })),
			steps:   []Step{topologyStepOf(twoZones, twoNodesPerZone), theDeploymentLineRefused},
			cause:   "pod.Request leaves out cpu",
		},
		"a request leaving out its memory": {
			declare: declaringADeploymentCustomizedWith(leavingOut(aRequest(), func(left *pod.Request) { left.Memory = 0 })),
			steps:   []Step{topologyStepOf(twoZones, twoNodesPerZone), theDeploymentLineRefused},
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

// What each field holds is no matter to the refusals above.
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

// Setting a field to its zero value is how a keyed literal leaving it out
// reads.
func leavingOut[S any](stated S, out func(*S)) S {
	out(&stated)

	return stated
}

func declaringADeploymentCustomizedWith(setting pod.Setting) func(*Scenario) {
	return func(declaring *Scenario) {
		declaring.Given().Zones(twoZones, theGeneralNodePool())
		declaring.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement().Customize(setting))
	}
}

func declaringATopologyOf(capacity node.Capacity) func(*Scenario) {
	return func(declaring *Scenario) {
		declaring.Given().Zones(twoZones, node.Pool(generalNodePool, node.PerZone(twoNodesPerZone), capacity))
	}
}

// The line reads under Kubernetes' own names: the kind the lab creates and the
// field its count sets.
func TestADeploymentsGivenLine_ShouldNameItsKindAndItsReplicas(t *testing.T) {
	const oneReplica = 1

	written := map[string]struct {
		replicas int
		reads    string
	}{
		"one replica":      {replicas: oneReplica, reads: fmt.Sprintf("a Deployment %s of %d replica", someDeploymentName, oneReplica)},
		"several replicas": {replicas: podsOverEveryNode, reads: fmt.Sprintf("a Deployment %s of %d replicas", someDeploymentName, podsOverEveryNode)},
	}

	for how, taken := range written {
		t.Run(how, func(t *testing.T) {
			declaring := aScenario()

			declaring.Given().Deployment(someDeploymentName, taken.replicas, askingForNothing(), resilientPlacement())

			expect.Expect(t, declaring.outcome().Steps).To(expect.Equal([]Step{carrying(
				Step{Kind: GivenStep, Name: taken.reads + " declaring " + resilientPlacementReads()},
				resilientPlacementCarries()...)}))
		})
	}
}

func TestADeploymentsGivenLine_ShouldNameTheStartupTimeItsPodsTake(t *testing.T) {
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

			declaring.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), taken.declares)

			expect.Expect(t, declaring.outcome().Steps).
				To(expect.Equal([]Step{carrying(deploymentStepOf(podsOverEveryNode, taken.reads), resilientPlacementCarries()...)}))
		})
	}
}

// The name of the Step stays as it was, so a Finding naming it stays short.
func TestADeploymentsGivenLine_ShouldReportEverySettingItsPodsCarryUnderItsKubernetesName(t *testing.T) {
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

			declaring.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), taken.declares)

			expect.Expect(t, declaring.outcome().Steps[0].Settings).To(expect.Equal(taken.carries))
		})
	}
}

func TestACustomizedDeploymentsGivenLine_ShouldReportTheSettingsAsCustomized(t *testing.T) {
	declaring := aScenario()

	declaring.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(),
		resilientPlacement().Customize(aZoneSpreadAt(aWiderSkew), aRequestOf(oneCorePerPod, twoGibibytesPerPod), aFullAccess()))

	expect.Expect(t, declaring.outcome().Steps[0].Settings).To(expect.Equal(append(theLeavingLines(),
		aSpreadLine(scenario.ZoneLabel, aWiderSkew), aSpreadLine(scenario.HostnameLabel, anEvenSkew),
		aRequestLine(oneCorePerPod, twoGibibytesPerPod), aFullAccessLine)))
}

func TestADeploymentLinesRequest_ShouldReportBeneathTheSpecificationsOwnSettings(t *testing.T) {
	declaring := aScenario()

	declaring.Given().Deployment(someDeploymentName, podsOverEveryNode,
		pod.Settings(aRequestOf(oneCorePerPod, twoGibibytesPerPod)), resilientPlacement())

	expect.Expect(t, declaring.outcome().Steps[0].Settings).To(expect.Equal(append(resilientPlacementCarries(),
		aRequestLine(oneCorePerPod, twoGibibytesPerPod))))
}

func TestADeploymentLinesRequest_ShouldStandOverARequestTheSpecificationIsCustomizedWith(t *testing.T) {
	declaring := aScenario()

	declaring.Given().Deployment(someDeploymentName, podsOverEveryNode,
		pod.Settings(aRequestOf(oneCorePerPod, twoGibibytesPerPod)),
		resilientPlacement().Customize(aRequestOf(overriddenCoresPerPod, overriddenGibibytesPerPod)))

	expect.Expect(t, declaring.outcome().Steps[0].Settings).To(expect.Equal(append(resilientPlacementCarries(),
		aRequestLine(oneCorePerPod, twoGibibytesPerPod))))
}

func TestANodePoolNamingACapacityItsScenarioDefines_ShouldProvisionNodesWithThatDefinitionsCapacity(t *testing.T) {
	declaring := aScenario()

	declaring.Given().Zones(twoZones, node.Pool(generalNodePool, node.PerZone(twoNodesPerZone), roomyNodes()))

	expect.Expect(t, declaring.topology.Nodes()).To(expect.Equal(aTopologyOf(t, twoZones, twoNodesPerZone).Nodes()))
}

func TestAGivenTopologyNamingANamedNodeCapacity_ShouldReportTheNameAndWhatItStandsFor(t *testing.T) {
	declaring := aScenario()

	declaring.Given().Zones(twoZones, node.Pool(generalNodePool, node.PerZone(twoNodesPerZone), roomyNodes()))

	expect.Expect(t, declaring.outcome().Steps).To(expect.Equal([]Step{topologyStepOf(twoZones, twoNodesPerZone)}))
}

func TestAGivenTopologyNamingItsNodeCapacity_ShouldDeclareThatCapacityAndRenameItsLine(t *testing.T) {
	declaring := aScenario()

	declaring.Given().Zones(twoZones, node.Pool(generalNodePool, node.PerZone(twoNodesPerZone),
		node.Capacity{CPU: someCoresPerNode, Memory: someGibibytesPerNode * node.Gi, Pods: twoPodsPerNode}))

	declared := declaring.outcome()
	expect.Expect(t, declaring.topology.Nodes()).
		To(expect.Equal(aTopologyAt(t, twoZones, twoNodesPerZone, twoPodsPerNode).Nodes()))
	expect.Expect(t, declared.Steps).
		To(expect.Equal([]Step{topologyStepHoldingOf(twoZones, twoNodesPerZone, twoPodsPerNode)}))
}

// The gpu NodePool's cpu and memory are double Roomy's, so its Nodes differ in
// both; no test here turns on its Pod count.
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

func theGpuNodePool() node.DeclaredPool {
	return node.Pool(gpuNodePool, node.PerZone(oneGpuNodePerZone),
		node.Capacity{CPU: gpuCoresPerNode, Memory: gpuGibibytesNode * node.Gi, Pods: someGpuPodsPerNode})
}

func theGeneralNodePool() node.DeclaredPool {
	return node.Pool(generalNodePool, node.PerZone(twoNodesPerZone), roomyNodes())
}

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

func topologyStepOfNodePools(zones int) Step {
	return Step{Kind: GivenStep, Name: fmt.Sprintf(
		"a Topology of %d Zones of a NodePool %s of %d %s Nodes holding %d cpu, %dGi memory and %d Pods each; "+
			"a NodePool %s of %d Node holding %d cpu, %dGi memory and %d Pods each",
		zones, generalNodePool, twoNodesPerZone, roomyName, roomyCoresPerNode, roomyGibibytesPerNode, roomyPodsPerNode,
		gpuNodePool, oneGpuNodePerZone, gpuCoresPerNode, gpuGibibytesNode, someGpuPodsPerNode)}
}

func TestARunOfNodePools_ShouldReportEachNodePoolByNameCountAndCapacity(t *testing.T) {
	declaring := aScenario()

	declaring.Given().Zones(twoZones, theGeneralNodePool(), theGpuNodePool())

	expect.Expect(t, declaring.outcome().Steps).To(expect.Equal([]Step{topologyStepOfNodePools(twoZones)}))
	expect.Expect(t, declaring.topology.Nodes()).To(expect.Equal(aTopologyOfNodePools(t, twoZones).Nodes()))
}

// The NodePools the domain refuses are its own suite's; this is the path
// through the Given Step. A NodePool omitting its capacity does not compile, so
// the zero capacity is the one written here.
func TestARefusedNodePoolDeclaration_ShouldBeRefusedWhereItIsStated(t *testing.T) {
	const noNodePerZone, noName, noCapacityName, noPod = 0, "", node.CapacityName(""), 0
	var noNodeCapacity node.Capacity

	refused := map[string]struct {
		declare func(Given)
		cause   string
	}{
		"nothing held in a Zone": {
			declare: func(given Given) { given.Zones(twoZones) },
			cause:   "a Topology of none is refused",
		},
		"a NodePool stating no name": {
			declare: func(given Given) {
				given.Zones(twoZones, node.Pool(noName, node.PerZone(twoNodesPerZone), roomyNodes()))
			},
			cause: "a NodePool states its name",
		},
		"two NodePools under one name": {
			declare: func(given Given) { given.Zones(twoZones, theGeneralNodePool(), theGeneralNodePool()) },
			cause:   "two NodePools named \"general\" could not be told apart",
		},
		"a NodePool stating no Node capacity": {
			declare: func(given Given) {
				given.Zones(twoZones, node.Pool(generalNodePool, node.PerZone(twoNodesPerZone), noNodeCapacity))
			},
			cause: "a NodePool states its Node capacity",
		},
		"a Named Node capacity defined without a name": {
			declare: func(given Given) {
				given.Zones(twoZones, node.Pool(generalNodePool, node.PerZone(twoNodesPerZone),
					node.Named(noCapacityName, node.Capacity{CPU: roomyCoresPerNode, Memory: roomyGibibytesPerNode * node.Gi, Pods: roomyPodsPerNode})))
			},
			cause: "a Named Node capacity states its name",
		},
		"a Named Node capacity of a value the domain refuses": {
			declare: func(given Given) {
				given.Zones(twoZones, node.Pool(generalNodePool, node.PerZone(twoNodesPerZone),
					node.Named(roomyName, node.Capacity{CPU: roomyCoresPerNode, Memory: roomyGibibytesPerNode * node.Gi, Pods: noPod})))
			},
			cause: fmt.Sprintf("a capacity of %d Pods is refused", noPod),
		},
		"a NodePool the domain refuses": {
			declare: func(given Given) {
				given.Zones(twoZones, node.Pool(generalNodePool, node.PerZone(noNodePerZone), roomyNodes()))
			},
			cause: fmt.Sprintf("a count of %d Nodes per Zone is refused", noNodePerZone),
		},
		"a NodePool stating a label the lab owns": {
			declare: func(given Given) {
				given.Zones(twoZones, theGeneralNodePool().Labels(node.Labels{node.Zone: spotCapacity}))
			},
			cause: node.Zone,
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

const (
	capacityTypeLabel    = "capacity-type"
	onDemandCapacity     = "on-demand"
	spotCapacity         = "spot"
	oneNodePerZoneOfEach = 1
)

func theOnDemandNodePool() node.DeclaredPool {
	return node.Pool(onDemandCapacity, node.PerZone(oneNodePerZoneOfEach), roomyNodes()).
		Labels(node.Labels{capacityTypeLabel: onDemandCapacity})
}

func theSpotNodePool() node.DeclaredPool {
	return node.Pool(spotCapacity, node.PerZone(oneNodePerZoneOfEach), roomyNodes()).
		Labels(node.Labels{capacityTypeLabel: spotCapacity})
}

func aTopologyOfLabelledNodePools(t *testing.T, zones int) scenario.Topology {
	t.Helper()

	topology, err := scenario.NewTopology(zones,
		aLabelledNodePool(t, onDemandCapacity), aLabelledNodePool(t, spotCapacity))
	if err != nil {
		t.Fatalf("build a Topology of %d Zones of two labelled NodePools: %v", zones, err)
	}

	return topology
}

func aLabelledNodePool(t *testing.T, capacityType string) scenario.NodePool {
	t.Helper()

	labels, err := scenario.NewNodeLabels(map[string]string{capacityTypeLabel: capacityType})
	if err != nil {
		t.Fatalf("build the label %s=%s: %v", capacityTypeLabel, capacityType, err)
	}
	pool, err := scenario.NewNodePool(capacityType, oneNodePerZoneOfEach, theCapacityRoomyStandsFor(t))
	if err != nil {
		t.Fatalf("build the NodePool %s: %v", capacityType, err)
	}
	labelled, err := pool.Labelled(labels)
	if err != nil {
		t.Fatalf("label the NodePool %s: %v", capacityType, err)
	}

	return labelled
}

func topologyStepOfLabelledNodePools(zones int) Step {
	labelledPool := "a NodePool %s of %d %s Node holding %d cpu, %dGi memory and %d Pods each, labelled %s=%s"

	return Step{Kind: GivenStep, Name: fmt.Sprintf("a Topology of %d Zones of "+labelledPool+"; "+labelledPool,
		zones,
		onDemandCapacity, oneNodePerZoneOfEach, roomyName, roomyCoresPerNode, roomyGibibytesPerNode, roomyPodsPerNode,
		capacityTypeLabel, onDemandCapacity,
		spotCapacity, oneNodePerZoneOfEach, roomyName, roomyCoresPerNode, roomyGibibytesPerNode, roomyPodsPerNode,
		capacityTypeLabel, spotCapacity)}
}

func TestAGivenTopologyStatingLabels_ShouldReportEachNodePoolsLabelsAfterItsCapacity(t *testing.T) {
	declaring := aScenario()

	declaring.Given().Zones(twoZones, theOnDemandNodePool(), theSpotNodePool())

	expect.Expect(t, declaring.outcome().Steps).To(expect.Equal([]Step{topologyStepOfLabelledNodePools(twoZones)}))
	expect.Expect(t, declaring.topology.Nodes()).To(expect.Equal(aTopologyOfLabelledNodePools(t, twoZones).Nodes()))
}

const podsPerCapacityTypeWhenEven = twoZones * oneNodePerZoneOfEach

func podsPerValueOfStep(pods int, key string) Step {
	return Step{Kind: ThenStep, Name: fmt.Sprintf("%d Pods per value of %s", pods, key)}
}

// A value whose Nodes hold no Pod counts as none, so a Placement leaving one
// empty is a Finding.
func TestAThenCountingPodsPerLabelValue_ShouldJudgeEveryValueTheNodePoolsState(t *testing.T) {
	atAnotherCount := podsPerCapacityTypeWhenEven + 1
	tests := map[string]struct {
		placed   func(scenario.Topology) scenario.Placement
		judgedAt int
		findings []judgedFinding
	}{
		"an even Placement at its count": {
			placed: onePodPerNodeOf, judgedAt: podsPerCapacityTypeWhenEven, findings: []judgedFinding{},
		},
		"an even Placement at another count": {
			placed: onePodPerNodeOf, judgedAt: atAnotherCount,
			findings: []judgedFinding{{Step: podsPerValueOfStep(atAnotherCount, capacityTypeLabel), Mismatching: true}},
		},
		"a Placement leaving one value empty": {
			placed: everyPodOnOneNodeOf, judgedAt: podsPerCapacityTypeWhenEven,
			findings: []judgedFinding{{Step: podsPerValueOfStep(podsPerCapacityTypeWhenEven, capacityTypeLabel), Mismatching: true}},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			judging := aScenarioOver(test.placed(aTopologyOfLabelledNodePools(t, twoZones)))
			judging.Given().Zones(twoZones, theOnDemandNodePool(), theSpotNodePool())
			judging.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())

			judging.Then().PodsPerValueOf(capacityTypeLabel, test.judgedAt)
			judging.Then().AllPodsRunning()

			judged := judging.outcome()
			expect.Expect(t, asJudged(judged.Findings)).To(expect.Equal(test.findings))
			expect.Expect(t, judged.Steps).To(expect.Equal([]Step{
				topologyStepOfLabelledNodePools(twoZones),
				resilientPlacementDeploymentStepOf(podsOverEveryNode),
				podsPerValueOfStep(test.judgedAt, capacityTypeLabel),
				allPodsRunningStep(),
			}))
		})
	}
}

func TestAThenStatingALabelNoNodePoolStates_ShouldEndTheScenarioAtThatThen(t *testing.T) {
	const aLabelNoNodePoolStates = "rack"
	ending := aScenarioOver(onePodPerNodeOf(aTopologyOfLabelledNodePools(t, twoZones)))
	ending.Given().Zones(twoZones, theOnDemandNodePool(), theSpotNodePool())

	ending.Then().PodsPerValueOf(aLabelNoNodePoolStates, podsPerCapacityTypeWhenEven)
	ending.Then().AllPodsRunning()

	ended := ending.outcome()
	expect.Expect(t, ended.EndedAt).To(expect.Equal(podsPerValueOfStep(podsPerCapacityTypeWhenEven, aLabelNoNodePoolStates)))
	expect.Expect(t, ended.Steps).To(expect.Equal([]Step{
		topologyStepOfLabelledNodePools(twoZones),
		podsPerValueOfStep(podsPerCapacityTypeWhenEven, aLabelNoNodePoolStates),
	}))
	expect.Expect(t, asJudged(ended.Findings)).To(expect.Equal([]judgedFinding{}))
	causeNames(t, ended, fmt.Sprintf("%q", aLabelNoNodePoolStates))
}

func TestThenSteps_ShouldAssertThePlacementOutcome(t *testing.T) {
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	judging := aScenarioOver(onePodPerNodeOf(declared))
	judging.Given().Zones(twoZones, theGeneralNodePool())
	judging.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())

	judging.Then().AllPodsRunning()
	judging.Then().PodsPerZone(podsPerZoneWhenEven)
	judging.Then().PodsPerNode(podsPerNodeWhenEven)

	judged := judging.outcome()
	expect.Expect(t, asJudged(judged.Findings)).To(expect.Equal([]judgedFinding{}))
	expect.Expect(t, judged.Steps).To(expect.Equal([]Step{
		topologyStepOf(twoZones, twoNodesPerZone),
		resilientPlacementDeploymentStepOf(podsOverEveryNode),
		allPodsRunningStep(),
		podsPerZoneStepOf(podsPerZoneWhenEven),
		podsPerNodeStepOf(podsPerNodeWhenEven),
	}))
}

func TestThenThePlacementDoesNotMeet_ShouldReportAFindingAndRunTheLaterSteps(t *testing.T) {
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	judging := aScenarioOver(everyPodOnOneNodeOf(declared))
	judging.Given().Zones(twoZones, theGeneralNodePool())
	judging.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())

	judging.Then().PodsPerZone(podsPerZoneWhenEven)
	judging.Then().AllPodsRunning()

	judged := judging.outcome()
	expect.Expect(t, asJudged(judged.Findings)).To(expect.Equal([]judgedFinding{
		{Step: podsPerZoneStepOf(podsPerZoneWhenEven), Mismatching: true},
	}))
	expect.Expect(t, judged.Steps).To(expect.Equal([]Step{
		topologyStepOf(twoZones, twoNodesPerZone),
		resilientPlacementDeploymentStepOf(podsOverEveryNode),
		podsPerZoneStepOf(podsPerZoneWhenEven),
		allPodsRunningStep(),
	}))
}

func TestAThenCountingThePodsLeftPending_ShouldReportAFindingOnAnotherCount(t *testing.T) {
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	judging := aScenarioOver(onePodLeftPendingBesideOnePerNodeOf(declared))
	judging.Given().Zones(twoZones, theGeneralNodePool())
	judging.Given().Deployment(someDeploymentName, podsBeyondEveryNode, askingForNothing(), resilientPlacement())

	judging.Then().PodsPending(onePodLeftPending)
	judging.Then().PodsPending(noPodPending)

	judged := judging.outcome()
	expect.Expect(t, asJudged(judged.Findings)).To(expect.Equal([]judgedFinding{
		{Step: podsPendingStepOf(noPodPending), Mismatching: true},
	}))
	expect.Expect(t, judged.Steps).To(expect.Equal([]Step{
		topologyStepOf(twoZones, twoNodesPerZone),
		resilientPlacementDeploymentStepOf(podsBeyondEveryNode),
		onePodPendingStep(),
		podsPendingStepOf(noPodPending),
	}))
}

// Stopping a Node is the Simulated Cluster's, so these tests judge the Then
// over a Placement the test built.
func TestAPlacementHoldingNoPodOnTheNodesThatFailed_ShouldRecordNoFinding(t *testing.T) {
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	stopped := theNodeThatStopped(declared)
	judging := aScenarioAfterFailing(everyPodBackOnTheRespondingNodesOf(declared, stopped), stopped)
	judging.Given().Zones(twoZones, theGeneralNodePool())
	judging.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())

	judging.Then().NoPodsOnTheFailedNodes()
	judging.Then().AllPodsRunning()

	judged := judging.outcome()
	expect.Expect(t, asJudged(judged.Findings)).To(expect.Equal([]judgedFinding{}))
	expect.Expect(t, judged.Steps).To(expect.Equal([]Step{
		topologyStepOf(twoZones, twoNodesPerZone),
		resilientPlacementDeploymentStepOf(podsOverEveryNode),
		noPodsOnTheFailedNodesStep(),
		allPodsRunningStep(),
	}))
}

func TestAPlacementStillHoldingAPodOnANodeThatFailed_ShouldReportAFindingAndRunTheLaterSteps(t *testing.T) {
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	stopped := theNodeThatStopped(declared)
	judging := aScenarioAfterFailing(onePodLeftOnTheFailedNodeOf(declared, stopped), stopped)
	judging.Given().Zones(twoZones, theGeneralNodePool())
	judging.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())

	judging.Then().NoPodsOnTheFailedNodes()
	judging.Then().AllPodsRunning()

	judged := judging.outcome()
	expect.Expect(t, asJudged(judged.Findings)).To(expect.Equal([]judgedFinding{
		{Step: noPodsOnTheFailedNodesStep(), Mismatching: true},
	}))
	expect.Expect(t, judged.Steps).To(expect.Equal([]Step{
		topologyStepOf(twoZones, twoNodesPerZone),
		resilientPlacementDeploymentStepOf(podsOverEveryNode),
		noPodsOnTheFailedNodesStep(),
		allPodsRunningStep(),
	}))
}

// A Zone still holding a Node in the observation is where a Pod left behind is
// still counted.
func TestAPlacementHoldingNoPodInTheZonesThatFailed_ShouldRecordNoFinding(t *testing.T) {
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	stopped := theZoneThatStopped(declared)
	judging := aScenarioAfterFailingAZone(everyPodBackOnTheRespondingZonesOf(declared, stopped), stopped)
	judging.Given().Zones(twoZones, theGeneralNodePool())
	judging.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())

	judging.Then().NoPodsInTheFailedZones()
	judging.Then().AllPodsRunning()

	judged := judging.outcome()
	expect.Expect(t, asJudged(judged.Findings)).To(expect.Equal([]judgedFinding{}))
	expect.Expect(t, judged.Steps).To(expect.Equal([]Step{
		topologyStepOf(twoZones, twoNodesPerZone),
		resilientPlacementDeploymentStepOf(podsOverEveryNode),
		noPodsInTheFailedZonesStep(),
		allPodsRunningStep(),
	}))
}

func TestAPlacementStillHoldingAPodInAZoneThatFailed_ShouldReportAFindingAndRunTheLaterSteps(t *testing.T) {
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	stopped := theZoneThatStopped(declared)
	judging := aScenarioAfterFailingAZone(onePodLeftInTheFailedZoneOf(declared, stopped), stopped)
	judging.Given().Zones(twoZones, theGeneralNodePool())
	judging.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())

	judging.Then().NoPodsInTheFailedZones()
	judging.Then().AllPodsRunning()

	judged := judging.outcome()
	expect.Expect(t, asJudged(judged.Findings)).To(expect.Equal([]judgedFinding{
		{Step: noPodsInTheFailedZonesStep(), Mismatching: true},
	}))
	expect.Expect(t, judged.Steps).To(expect.Equal([]Step{
		topologyStepOf(twoZones, twoNodesPerZone),
		resilientPlacementDeploymentStepOf(podsOverEveryNode),
		noPodsInTheFailedZonesStep(),
		allPodsRunningStep(),
	}))
}

func theNodesOneZoneFailingStops(t *testing.T, topology scenario.Topology) []string {
	t.Helper()

	failure, err := scenario.NewZoneFailure(topology, oneLost)
	if err != nil {
		t.Fatalf("build a Failure stopping %d Zone: %v", oneLost, err)
	}

	return failure.NodeNames()
}

func onePodRunningPerNodeRecordedOf(topology scenario.Topology, deployment DeploymentName) []scenario.Entry {
	nodes := topology.Nodes()
	recorded := make([]scenario.Entry, 0, len(nodes))
	for index, node := range nodes {
		recorded = append(recorded, scenario.Entry{
			Change: scenario.ChangeModified, Kind: scenario.PodObject, Name: fmt.Sprintf("%s-%d", deployment, index),
			Reason: string(scenario.PodRunning), Detail: node.Name,
		})
	}

	return recorded
}

func aWhenNoted() scenario.Entry {
	return scenario.Entry{Change: scenario.ChangeNoted, Kind: scenario.StepObject, Name: scenario.WhenStepNamed}
}

// theReadingsOf asks internal/scenario, whose suite owns how a Floor is read,
// so this suite asserts only that the Then consulted it.
func theReadingsOf(t *testing.T, recorded []scenario.Entry, stopped []string, judged ...scenario.Deployment) []scenario.FloorReading {
	t.Helper()

	readings, err := scenario.NewTimeline(recorded).
		Floors(judged, []string{string(someDeploymentName), string(anotherDeploymentName)}, [][]string{stopped})
	if err != nil {
		t.Fatalf("read the Floors over the recorded Timeline: %v", err)
	}

	return readings
}

func linesOf(readings []scenario.FloorReading) []string {
	lines := make([]string, 0, len(readings))
	for _, reading := range readings {
		lines = append(lines, reading.Line())
	}

	return lines
}

// The second Deployment declares no Failure tolerance, so it is not judged.
func TestADeploymentKeepingItsFloorThroughTheFailure_ShouldReportItKeptAndRecordNoFinding(t *testing.T) {
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	stopped := []string{theNodeThatStopped(declared)}
	recorded := slices.Concat(onePodRunningPerNodeRecordedOf(declared, someDeploymentName),
		onePodRunningPerNodeRecordedOf(declared, anotherDeploymentName), []scenario.Entry{aWhenNoted()})
	judging := aScenarioRecording(stopped, recorded...)
	judging.Given().Zones(twoZones, theGeneralNodePool())
	judging.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement()).
		SurvivesLosing(Nodes(oneLost))
	judging.Given().Deployment(anotherDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())

	judging.Then().EnoughPodsStayedReady()

	judged := judging.outcome()
	surviving := aDeploymentSurvivingTheLossOf(t, podsOverEveryNode, Nodes(oneLost), declared)
	expect.Expect(t, surviving.Floor()).To(expect.Equal(floorSurvivingOneNode))
	expect.Expect(t, asJudged(judged.Findings)).To(expect.Equal([]judgedFinding{}))
	expect.Expect(t, judged.Steps).To(expect.Equal([]Step{
		topologyStepOf(twoZones, twoNodesPerZone),
		deploymentStepSurvivingTheLossOf(podsOverEveryNode, oneNodeLost),
		resilientPlacementDeploymentStepNamed(anotherDeploymentName, podsOverEveryNode),
		enoughPodsStayedReadyStep(linesOf(theReadingsOf(t, recorded, stopped, surviving))...),
	}))
}

// A Zone holds two Pods of each Deployment: one Deployment's Floor admits that
// loss and the other's does not.
func TestOneOfTwoDeploymentsFallingBelowItsFloor_ShouldFailTheWholeThenNamingThatDeployment(t *testing.T) {
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	stopped := theNodesOneZoneFailingStops(t, declared)
	recorded := slices.Concat(onePodRunningPerNodeRecordedOf(declared, someDeploymentName),
		onePodRunningPerNodeRecordedOf(declared, anotherDeploymentName), []scenario.Entry{aWhenNoted()})
	judging := aScenarioRecording(stopped, recorded...)
	judging.Given().Zones(twoZones, theGeneralNodePool())
	judging.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement()).
		SurvivesLosing(Nodes(oneLost))
	judging.Given().Deployment(anotherDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement()).
		SurvivesLosing(Zones(oneLost))
	judging.observed, judging.settled = onePodPerNodeOf(declared), true

	judging.Then().EnoughPodsStayedReady()
	judging.Then().AllPodsRunning()

	judged := judging.outcome()
	falling := aDeploymentSurvivingTheLossOf(t, podsOverEveryNode, Nodes(oneLost), declared)
	keeping := aDeploymentNamedSurvivingTheLossOf(t, anotherDeploymentName, podsOverEveryNode, Zones(oneLost), declared)
	readings := theReadingsOf(t, recorded, stopped, falling, keeping)
	judgedAt := enoughPodsStayedReadyStep(linesOf(readings)...)
	expect.Expect(t, keeping.Floor()).To(expect.Equal(floorSurvivingOneZone))
	expect.Expect(t, judged.Findings).To(expect.Equal([]Finding{{Step: judgedAt, Mismatch: readings[0].Fall()}}))
	expect.Expect(t, judged.Steps).To(expect.Equal([]Step{
		topologyStepOf(twoZones, twoNodesPerZone),
		deploymentStepSurvivingTheLossOf(podsOverEveryNode, oneNodeLost),
		namedDeploymentStepSurvivingTheLossOf(anotherDeploymentName, podsOverEveryNode, oneZoneLost),
		judgedAt,
		allPodsRunningStep(),
	}))
	expect.Expect(t, causeOf(judged)).To(expect.Equal(""))
}

func TestAFloorJudgementWithNothingToJudge_ShouldEndTheScenarioNamingThatThen(t *testing.T) {
	// The Then is recorded before it states any reading.
	cases := map[string]struct {
		declare  func(*Scenario)
		judging  *Scenario
		declared Step
		cause    string
	}{
		"a Floor no Failure came before": {
			judging: &Scenario{},
			declare: func(ending *Scenario) {
				ending.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement()).
					SurvivesLosing(Nodes(oneLost))
			},
			declared: deploymentStepSurvivingTheLossOf(podsOverEveryNode, oneNodeLost),
			cause:    theWhenThatNeverRan,
		},
		"a Failure no Deployment declared a Failure tolerance for": {
			judging: aScenarioRecording([]string{theNodeThatStopped(aTopologyOf(t, twoZones, twoNodesPerZone))}, aWhenNoted()),
			declare: func(ending *Scenario) {
				ending.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())
			},
			declared: resilientPlacementDeploymentStepOf(podsOverEveryNode),
			cause:    theToleranceNeverDeclared,
		},
		"a Failure the Timeline holds no note of": {
			judging: aScenarioRecording([]string{theNodeThatStopped(aTopologyOf(t, twoZones, twoNodesPerZone))}),
			declare: func(ending *Scenario) {
				ending.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement()).
					SurvivesLosing(Nodes(oneLost))
			},
			declared: deploymentStepSurvivingTheLossOf(podsOverEveryNode, oneNodeLost),
			cause:    "no When's note",
		},
	}

	for named, one := range cases {
		t.Run(named, func(t *testing.T) {
			ending := one.judging
			ending.Given().Zones(twoZones, theGeneralNodePool())
			one.declare(ending)

			ending.Then().EnoughPodsStayedReady()
			ending.Then().AllPodsRunning()

			ended := ending.outcome()
			expect.Expect(t, ended.EndedAt).To(expect.Equal(enoughPodsStayedReadyStep()))
			expect.Expect(t, ended.Steps).To(expect.Equal([]Step{
				topologyStepOf(twoZones, twoNodesPerZone), one.declared, enoughPodsStayedReadyStep(),
			}))
			expect.Expect(t, asJudged(ended.Findings)).To(expect.Equal([]judgedFinding{}))
			causeNames(t, ended, one.cause)
		})
	}
}

func TestStepThatCannotBeCarriedOut_ShouldEndTheScenarioAndNameThatStep(t *testing.T) {
	// A Scenario that has ended records none of the lines after it.
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
				ending.Given().Zones(twoZones, node.Pool(generalNodePool, node.PerZone(noNodesPerZone), roomyNodes()))
				ending.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())
			},
			endedAt: topologyStepOf(twoZones, noNodesPerZone),
			steps:   []Step{topologyStepOf(twoZones, noNodesPerZone)},
			cause:   fmt.Sprintf("a Zone holds at least one Node, so a count of %d Nodes per Zone is refused", noNodesPerZone),
		},
		{
			name: "a Topology whose Nodes hold no Pod",
			declare: func(ending *Scenario) {
				ending.Given().Zones(twoZones, node.Pool(generalNodePool, node.PerZone(twoNodesPerZone),
					node.Capacity{CPU: someCoresPerNode, Memory: someGibibytesPerNode * node.Gi, Pods: noPods}))
				ending.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())
			},
			endedAt: topologyStepHoldingOf(twoZones, twoNodesPerZone, noPods),
			steps:   []Step{topologyStepHoldingOf(twoZones, twoNodesPerZone, noPods)},
			cause:   fmt.Sprintf("a Node holds at least one Pod, so a capacity of %d Pods is refused", noPods),
		},
		{
			name: "a Deployment of no Pods",
			declare: func(ending *Scenario) {
				ending.Given().Zones(twoZones, theGeneralNodePool())
				ending.Given().Deployment(someDeploymentName, noReplicas, askingForNothing(), resilientPlacement())
			},
			endedAt: resilientPlacementDeploymentStepOf(noReplicas),
			steps: []Step{
				topologyStepOf(twoZones, twoNodesPerZone),
				resilientPlacementDeploymentStepOf(noReplicas),
			},
			cause: fmt.Sprintf("a Deployment declares at least one replica, so a count of %d replicas is refused", noReplicas),
		},
		{
			name: "a Deployment declaring a specification that sets nothing",
			declare: func(ending *Scenario) {
				ending.Given().Zones(twoZones, theGeneralNodePool())
				ending.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), pod.Specification{})
			},
			endedAt: deploymentStepOf(podsOverEveryNode, noSpecificationAtAll),
			steps: []Step{
				topologyStepOf(twoZones, twoNodesPerZone),
				deploymentStepOf(podsOverEveryNode, noSpecificationAtAll),
			},
			cause: "a Deployment's Pods are deployed with the specification it declares, " +
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
			ending.Given().Zones(twoZones, theGeneralNodePool())
			ending.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())

			ending.Then().PodsPerZone(podsPerZoneWhenEven)
			ending.Then().AllPodsRunning()

			ended := ending.outcome()
			expect.Expect(t, ended.EndedAt).To(expect.Equal(podsPerZoneStepOf(podsPerZoneWhenEven)))
			expect.Expect(t, ended.Steps).To(expect.Equal([]Step{
				topologyStepOf(twoZones, twoNodesPerZone),
				resilientPlacementDeploymentStepOf(podsOverEveryNode),
				podsPerZoneStepOf(podsPerZoneWhenEven),
			}))
			expect.Expect(t, asJudged(ended.Findings)).To(expect.Equal([]judgedFinding{}))
			causeNames(t, ended, theWaitThatNeverStood)
		})
	}
}

// The Scenario ends at the When rather than stopping Nodes in a world it has
// not observed.
func TestAFailureBeforeTheScenarioHasWaited_ShouldEndTheScenarioNamingThatWhen(t *testing.T) {
	ending := aScenario()
	ending.Given().Zones(twoZones, theGeneralNodePool())
	ending.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())

	ending.When().NodesFail(oneNodeFailing)
	ending.Then().AllPodsRunning()

	ended := ending.outcome()
	expect.Expect(t, ended.EndedAt).To(expect.Equal(oneNodeFailsStep()))
	expect.Expect(t, ended.Steps).To(expect.Equal([]Step{
		topologyStepOf(twoZones, twoNodesPerZone),
		resilientPlacementDeploymentStepOf(podsOverEveryNode),
		oneNodeFailsStep(),
	}))
	expect.Expect(t, asJudged(ended.Findings)).To(expect.Equal([]judgedFinding{}))
	causeNames(t, ended, theWaitThatNeverStood)
}

// The When here is the refused-count one, which resolves no Node and so reaches
// no cluster.
func TestAThenStandingBeforeAWhen_ShouldJudgeThePlacementTheScenarioStartsFrom(t *testing.T) {
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	judging := aScenarioOver(onePodPerNodeOf(declared))
	judging.Given().Zones(twoZones, theGeneralNodePool())
	judging.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())

	judging.Then().AllPodsRunning()
	judging.Then().PodsPerZone(podsPerZoneWhenEven)
	judging.When().NodesFail(moreNodesThanTheTopologyHolds)

	judged := judging.outcome()
	expect.Expect(t, asJudged(judged.Findings)).To(expect.Equal([]judgedFinding{}))
	expect.Expect(t, judged.Steps).To(expect.Equal([]Step{
		topologyStepOf(twoZones, twoNodesPerZone),
		resilientPlacementDeploymentStepOf(podsOverEveryNode),
		allPodsRunningStep(),
		podsPerZoneStepOf(podsPerZoneWhenEven),
		nodesFailStepOf(moreNodesThanTheTopologyHolds),
	}))
}

func TestAWhenAskingForMoreFailingNodesThanTheTopologyHolds_ShouldEndTheScenarioNamingThatStep(t *testing.T) {
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	ending := aScenarioOver(onePodPerNodeOf(declared))
	ending.Given().Zones(twoZones, theGeneralNodePool())
	ending.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())

	ending.When().NodesFail(moreNodesThanTheTopologyHolds)
	ending.Then().AllPodsRunning()

	ended := ending.outcome()
	expect.Expect(t, ended.EndedAt).To(expect.Equal(nodesFailStepOf(moreNodesThanTheTopologyHolds)))
	expect.Expect(t, ended.Steps).To(expect.Equal([]Step{
		topologyStepOf(twoZones, twoNodesPerZone),
		resilientPlacementDeploymentStepOf(podsOverEveryNode),
		nodesFailStepOf(moreNodesThanTheTopologyHolds),
	}))
	expect.Expect(t, asJudged(ended.Findings)).To(expect.Equal([]judgedFinding{}))
	causeNames(t, ended, fmt.Sprintf("a Failure of %d Nodes is refused", moreNodesThanTheTopologyHolds))
}

func TestAWhenThatFailsMoreZonesThanTheTopologyHolds_ShouldEndTheScenarioAtThatStep(t *testing.T) {
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	ending := aScenarioOver(onePodPerNodeOf(declared))
	ending.Given().Zones(twoZones, theGeneralNodePool())
	ending.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())

	ending.When().ZonesFail(moreZonesThanTheTopologyHolds)
	ending.Then().AllPodsRunning()

	ended := ending.outcome()
	expect.Expect(t, ended.EndedAt).To(expect.Equal(zonesFailStepOf(moreZonesThanTheTopologyHolds)))
	expect.Expect(t, ended.Steps).To(expect.Equal([]Step{
		topologyStepOf(twoZones, twoNodesPerZone),
		resilientPlacementDeploymentStepOf(podsOverEveryNode),
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
			ending.Given().Zones(twoZones, theGeneralNodePool())
			ending.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())

			then.judge(ending)
			ending.Then().AllPodsRunning()

			ended := ending.outcome()
			expect.Expect(t, ended.EndedAt).To(expect.Equal(then.step))
			expect.Expect(t, ended.Steps).To(expect.Equal([]Step{
				topologyStepOf(twoZones, twoNodesPerZone),
				resilientPlacementDeploymentStepOf(podsOverEveryNode),
				then.step,
			}))
			expect.Expect(t, asJudged(ended.Findings)).To(expect.Equal([]judgedFinding{}))
			causeNames(t, ended, theWhenThatNeverRan)
		})
	}
}

func causeOf(reported Outcome) string {
	if reported.Cause == nil {
		return ""
	}

	return reported.Cause.Error()
}

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

const (
	aKeyNoNodePoolStates   = "node-class"
	aValueNoNodePoolStates = "reserved"
)

func labelledNodesFailStepOf(label string) Step {
	return Step{Kind: WhenStep, Name: "Nodes labelled " + label + " fail"}
}

func TestAFailureByLabelNamingWhatNoNodePoolStates_ShouldEndTheScenarioAtThatWhen(t *testing.T) {
	labelled := aTopologyOfLabelledNodePools(t, twoZones)
	refusals := map[string]struct {
		scenario *Scenario
		labels   node.Labels
		step     string
		cause    string
	}{
		"a key no NodePool states": {
			scenario: aScenarioOver(onePodPerNodeOf(labelled)),
			labels:   node.Labels{aKeyNoNodePoolStates: spotCapacity},
			step:     aKeyNoNodePoolStates + "=" + spotCapacity,
			cause:    aKeyNoNodePoolStates,
		},
		"a value no NodePool states for the key": {
			scenario: aScenarioOver(onePodPerNodeOf(labelled)),
			labels:   node.Labels{capacityTypeLabel: aValueNoNodePoolStates},
			step:     capacityTypeLabel + "=" + aValueNoNodePoolStates,
			cause:    aValueNoNodePoolStates,
		},
		"a Scenario that has not Waited": {
			scenario: aScenario(),
			labels:   node.Labels{capacityTypeLabel: spotCapacity},
			step:     capacityTypeLabel + "=" + spotCapacity,
			cause:    theWaitThatNeverStood,
		},
	}

	for refused, refusal := range refusals {
		t.Run(refused, func(t *testing.T) {
			ending := refusal.scenario
			ending.Given().Zones(twoZones, theOnDemandNodePool(), theSpotNodePool())

			ending.When().NodesLabelledFail(refusal.labels)
			ending.Then().AllPodsRunning()

			ended := ending.outcome()
			failing := labelledNodesFailStepOf(refusal.step)
			expect.Expect(t, ended.EndedAt).To(expect.Equal(failing))
			expect.Expect(t, ended.Steps).To(expect.Equal([]Step{topologyStepOfLabelledNodePools(twoZones), failing}))
			expect.Expect(t, asJudged(ended.Findings)).To(expect.Equal([]judgedFinding{}))
			causeNames(t, ended, refusal.cause)
		})
	}
}

func nodesOfNodePool(topology scenario.Topology, nodePool string) []scenario.Node {
	var pooled []scenario.Node
	for _, candidate := range topology.Nodes() {
		if candidate.NodePool == nodePool {
			pooled = append(pooled, candidate)
		}
	}

	return pooled
}

func namesOf(nodes []scenario.Node) []string {
	names := make([]string, 0, len(nodes))
	for _, named := range nodes {
		names = append(names, named.Name)
	}

	return names
}

func podsOnNodePoolStep(pods int, pool node.PoolName) Step {
	return Step{Kind: ThenStep, Name: fmt.Sprintf("%d Pods on the NodePool %s", pods, pool)}
}

func TestAThenStatingANodePoolAndAPodCount_ShouldRecordAFindingUnlessItsNodesHoldThatCount(t *testing.T) {
	const podsOnOneNodePoolWhenEven, noPod = twoZones * oneNodePerZoneOfEach, 0
	tests := map[string]struct {
		placed   func(scenario.Topology) scenario.Placement
		judgedAt int
		findings []judgedFinding
	}{
		"an even Placement at its count": {
			placed: onePodPerNodeOf, judgedAt: podsOnOneNodePoolWhenEven, findings: []judgedFinding{},
		},
		"an even Placement at another count": {
			placed: onePodPerNodeOf, judgedAt: noPod,
			findings: []judgedFinding{{Step: podsOnNodePoolStep(noPod, spotCapacity), Mismatching: true}},
		},
		"a Placement leaving the NodePool empty at none": {
			placed: everyPodOnOneNodeOf, judgedAt: noPod, findings: []judgedFinding{},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			judging := aScenarioOver(test.placed(aTopologyOfLabelledNodePools(t, twoZones)))
			judging.Given().Zones(twoZones, theOnDemandNodePool(), theSpotNodePool())
			judging.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())

			judging.Then().PodsOnNodePool(spotCapacity, test.judgedAt)
			judging.Then().AllPodsRunning()

			judged := judging.outcome()
			expect.Expect(t, asJudged(judged.Findings)).To(expect.Equal(test.findings))
			expect.Expect(t, judged.Steps).To(expect.Equal([]Step{
				topologyStepOfLabelledNodePools(twoZones),
				resilientPlacementDeploymentStepOf(podsOverEveryNode),
				podsOnNodePoolStep(test.judgedAt, spotCapacity),
				allPodsRunningStep(),
			}))
		})
	}
}

func TestAThenNamingANodePoolTheTopologyLacks_ShouldEndTheScenarioAtThatThen(t *testing.T) {
	const aNodePoolTheTopologyLacks, noPod = node.PoolName("gpu"), 0
	ending := aScenarioOver(onePodPerNodeOf(aTopologyOfLabelledNodePools(t, twoZones)))
	ending.Given().Zones(twoZones, theOnDemandNodePool(), theSpotNodePool())

	ending.Then().PodsOnNodePool(aNodePoolTheTopologyLacks, noPod)
	ending.Then().AllPodsRunning()

	ended := ending.outcome()
	expect.Expect(t, ended.EndedAt).To(expect.Equal(podsOnNodePoolStep(noPod, aNodePoolTheTopologyLacks)))
	expect.Expect(t, ended.Steps).To(expect.Equal([]Step{
		topologyStepOfLabelledNodePools(twoZones),
		podsOnNodePoolStep(noPod, aNodePoolTheTopologyLacks),
	}))
	expect.Expect(t, asJudged(ended.Findings)).To(expect.Equal([]judgedFinding{}))
	causeNames(t, ended, fmt.Sprintf("%q", aNodePoolTheTopologyLacks))
}

const anotherDeploymentName DeploymentName = "another-deployment"

// The Placement observed before belongs to the earlier Deployment, so a Then
// before the next Wait has none of the newest to judge.
func TestALaterDeploymentGiven_ShouldBeTheDeploymentEveryLaterStepReads(t *testing.T) {
	const fewerPods = podsOverEveryNode - 1
	declaring := aScenarioOver(onePodPerNodeOf(aTopologyOf(t, twoZones, twoNodesPerZone)))
	declaring.Given().Zones(twoZones, theGeneralNodePool())
	declaring.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())

	declaring.Given().Deployment(anotherDeploymentName, fewerPods, askingForNothing(), resilientPlacement())
	declaring.Then().AllPodsRunning()

	declared := declaring.outcome()
	expect.Expect(t, declaring.deployment.Name()).To(expect.Equal(string(anotherDeploymentName)))
	expect.Expect(t, declaring.deployment.Replicas()).To(expect.Equal(fewerPods))
	expect.Expect(t, declared.EndedAt).To(expect.Equal(allPodsRunningStep()))
	causeNames(t, declared, theWaitThatNeverStood)
}

// A repeated name would deploy a second Deployment whose Pods the first one's
// selector reads.
func TestADeploymentNameAlreadyDeclared_ShouldEndTheScenarioAtThatGiven(t *testing.T) {
	declaring := aScenario()
	declaring.Given().Zones(twoZones, theGeneralNodePool())
	declaring.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())

	declaring.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())
	declaring.Then().AllPodsRunning()

	ended := declaring.outcome()
	expect.Expect(t, ended.EndedAt).To(expect.Equal(resilientPlacementDeploymentStepOf(podsOverEveryNode)))
	expect.Expect(t, ended.Steps).To(expect.HaveLen[Step](3))
	causeNames(t, ended, fmt.Sprintf("%q", someDeploymentName))
}

func TestATaintTheLabCannotHold_ShouldBeRefusedWhereItIsStated(t *testing.T) {
	tests := map[string]struct {
		taint   node.Taint
		refused string
	}{
		"a taint of another effect": {taint: node.Taint{Key: gpuTaintKey, Value: gpuTaintPresent, Effect: scenario.NoExecute}, refused: string(scenario.NoExecute)},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ending := aScenario()

			ending.Given().Zones(twoZones, theGeneralNodePool(), theGpuNodePool().Taints(test.taint))
			ending.WaitFor().AllNodesReady()

			ended := ending.outcome()
			expect.Expect(t, ended.Steps).To(expect.HaveLen[Step](1))
			expect.Expect(t, ended.EndedAt.Kind).To(expect.Equal(GivenStep))
			causeNames(t, ended, test.refused)
		})
	}
}

func TestATolerationOfANoScheduleTaint_ShouldBeReportedBeneathItsDeploymentsGivenLineWithNoWaitingTime(t *testing.T) {
	tolerating := pod.Toleration{Key: gpuTaintKey, Operator: pod.Equal, Value: gpuTaintPresent, Effect: pod.NoSchedule}

	declaring := aScenario()
	declaring.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), defaultPlacement().Customize(tolerating))

	expect.Expect(t, slices.Contains(declaring.outcome().Steps[0].Settings,
		"tolerations: {key: nvidia.com/gpu, operator: Equal, value: present, effect: NoSchedule}")).To(expect.Equal(true))
	expect.Expect(t, causeOf(declaring.outcome())).To(expect.Equal(""))
}

func TestANoScheduleTolerationStatingAWaitingTime_ShouldEndTheScenarioAtThatGiven(t *testing.T) {
	const someWait = 30
	tolerating := pod.Toleration{Key: gpuTaintKey, Operator: pod.Equal, Value: gpuTaintPresent, Effect: pod.NoSchedule}
	waiting := tolerating
	waiting.TolerationSeconds = someWait

	refusing := aScenario()
	refusing.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), defaultPlacement().Customize(waiting))

	expect.Expect(t, refusing.outcome().EndedAt.Kind).To(expect.Equal(GivenStep))
	causeNames(t, refusing.outcome(), "pod.Toleration states tolerationSeconds")
}
