// These tests cross to a real Simulated Cluster. One judges the order in which
// a panicking Scenario lets go of the cluster, its recording and the
// recording's file: no stand-in has a recording to outlive its file. The others
// judge what an Interactive run holds, observes and removes.
package lab

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/cluster"
	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/lab/pod"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
	"github.com/woditschka/k8s-resilience-lab/internal/timeline"
)

const (
	thePanickingScenarioOnACluster = "a-scenario-that-panics-on-a-cluster"
	whatTheClusterScenarioSaid     = "a Step's own code panicked with the cluster recording"
)

func TestAScenarioThatPanicsOnARealCluster_ShouldLeaveNoClusterAndNoSecondPanic(t *testing.T) {
	t.Parallel()
	engine, err := cluster.DetectEngine(t.Context(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("no container engine answers on this machine, so the Simulated Cluster cannot be driven: %v", err)
	}
	t.Cleanup(engine.Close)
	running := NewRunner(engine, cluster.DefaultKubernetesVersion(), timeline.NewFolder(t.TempDir()))
	var provisioned *cluster.SimulatedCluster

	// The Scenario panics at once, so the change is still on its way to the
	// recording when the panic unwinds, which is what a file closed too early
	// would meet.
	carried := carryOutAtOnce([]string{thePanickingScenarioOnACluster}, func(int) (Outcome, error) {
		return running.runNamed(t.Context(), thePanickingScenarioOnACluster, func(steps *Scenario) {
			provisioned = steps.provisioned
			declared := aTopologyOf(t, twoZones, twoNodesPerZone)
			if err := provisioned.ApplyTopology(steps.ctx, declared); err != nil {
				t.Errorf("apply the Topology before the panic: %v", err)
			}
			if err := provisioned.FailNode(steps.ctx, declared.Nodes()[0].Name); err != nil {
				t.Errorf("stop a Node responding before the panic: %v", err)
			}
			panic(whatTheClusterScenarioSaid)
		})
	})

	expect.Expect(t, theScenariosThatWentWrongIn(carried)).To(expect.Equal([]string{thePanickingScenarioOnACluster}))
	expect.Expect(t, strings.Contains(theFailureOf(carried, thePanickingScenarioOnACluster), whatTheClusterScenarioSaid)).
		To(expect.Equal(true))
	if _, err := provisioned.ReadyNodes(t.Context()); err == nil {
		t.Errorf("the panicking Scenario's Simulated Cluster %s still answers, but its container is removed", provisioned.Name())
	}
}

const (
	theHeldScenario             = "a-scenario-held-before-its-first-when"
	theScenarioStoppedPartWay   = "a-scenario-stopped-while-its-steps-run"
	theScenarioEndedAtARefusal  = "a-scenario-held-as-a-refused-step-left-it"
	anObserverIntervalForATest  = 200 * time.Millisecond
	theNodesOfTwoZonesOfTwoEach = twoZones * twoNodesPerZone
)

func aRunnerOnThisMachine(t *testing.T) *Runner {
	t.Helper()

	engine, err := cluster.DetectEngine(t.Context(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("no container engine answers on this machine, so the Simulated Cluster cannot be driven: %v", err)
	}
	t.Cleanup(engine.Close)

	return NewRunner(engine, cluster.DefaultKubernetesVersion(), timeline.NewFolder(t.TempDir()))
}

// observed carries a board as it reads, so a comparison needs no access to its
// fields.
type observed struct {
	Lines       []string
	NotObserved string
}

// anObserverHandingOverEachBoard blocks the observer until the test takes the
// board, or the run lets go of the cluster.
func anObserverHandingOverEachBoard(holding context.Context) (Observer, <-chan observed) {
	boards := make(chan observed)

	return Observer{Every: anObserverIntervalForATest, Observed: func(board scenario.PlacementBoard, err error) {
		handed := observed{Lines: board.Lines()}
		if err != nil {
			handed.NotObserved = err.Error()
		}
		select {
		case boards <- handed:
		case <-holding.Done():
		}
	}}, boards
}

func anInjectorReportingNothing() *Injector {
	return NewInjector(func(Step) {})
}

func anObserverDiscardingEachBoard() Observer {
	return Observer{Every: anObserverIntervalForATest, Observed: func(scenario.PlacementBoard, error) {}}
}

// The first board taken may have waited since the Steps ran, and the second is
// marked against it; the third is marked against a board of the held cluster.
func theBoardObservedOnceHeld(boards <-chan observed) observed {
	<-boards
	<-boards

	return <-boards
}

// A board marked against itself has no newcomer, as one observed twice of a
// cluster that has stopped changing.
func theBoardOfAClusterNoLongerChanging(topology scenario.Topology, placed scenario.Placement) observed {
	once := scenario.NewPlacementBoard(topology, placed, scenario.PlacementBoard{})

	return observed{Lines: scenario.NewPlacementBoard(topology, placed, once).Lines()}
}

// A specification naming nothing is refused by the Deployment's constructor, so
// the Given declaring it cannot be carried out.
func aSpecificationTheConstructorRefuses() pod.Specification {
	return pod.Specification{}
}

func expectRemoved(t *testing.T, provisioned *cluster.SimulatedCluster) {
	t.Helper()

	if _, err := provisioned.ReadyNodes(context.WithoutCancel(t.Context())); err == nil {
		t.Errorf("the Simulated Cluster %s still answers, but its container is removed", provisioned.Name())
	}
}

func theReadyNodesOf(t *testing.T, provisioned *cluster.SimulatedCluster) []scenario.Node {
	t.Helper()

	ready, err := provisioned.ReadyNodes(context.WithoutCancel(t.Context()))
	if err != nil {
		t.Fatalf("read the Ready Nodes of the held Simulated Cluster %s: %v", provisioned.Name(), err)
	}

	return ready
}

// The board observed while held shows the Deployment's Pods alone, so no Pod the
// lab placed while provisioning the Topology stands on it.
func TestAnInteractiveRunNamingAScenario_ShouldHoldItsClusterAsTheStepsBeforeItsFirstWhenLeftIt(t *testing.T) {
	t.Parallel()
	running := aRunnerOnThisMachine(t)
	holding, stopTheLab := context.WithCancel(t.Context())
	observer, boards := anObserverHandingOverEachBoard(holding)
	var provisioned *cluster.SimulatedCluster
	var declared scenario.Topology
	var readyWhileHeld []scenario.Node
	var boardWhileHeld observed

	held, err := running.holdNamed(holding, theHeldScenario, func(steps *Scenario) {
		provisioned = steps.provisioned
		steps.Given().Zones(twoZones, theGeneralNodePool())
		steps.WaitFor().AllNodesReady()
		steps.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())
		steps.WaitFor().AllPodsReady()
		declared = steps.topology

		steps.When().NodesFail(oneNodeFailing)
		steps.WaitFor().AllPodsReady()
	}, observer, anInjectorReportingNothing(), func(Outcome) {
		readyWhileHeld = theReadyNodesOf(t, provisioned)
		boardWhileHeld = theBoardObservedOnceHeld(boards)
		stopTheLab()
	})
	if err != nil {
		t.Fatalf("carry out the held Scenario %s: %v", theHeldScenario, err)
	}

	expect.Expect(t, theKindsOf(held.Steps)).To(expect.Equal([]StepKind{GivenStep, WaitStep, GivenStep, WaitStep}))
	expect.Expect(t, readyWhileHeld).To(expect.HaveLen[scenario.Node](theNodesOfTwoZonesOfTwoEach))
	expect.Expect(t, boardWhileHeld).To(expect.Equal(theBoardOfAClusterNoLongerChanging(declared, held.Placement)))
	expectRemoved(t, provisioned)
}

func TestStoppingAnInteractiveRun_ShouldRemoveItsSimulatedCluster(t *testing.T) {
	t.Parallel()
	running := aRunnerOnThisMachine(t)
	holding, stopTheLab := context.WithCancel(t.Context())
	var provisioned *cluster.SimulatedCluster

	_, err := running.holdNamed(holding, theScenarioStoppedPartWay, func(steps *Scenario) {
		provisioned = steps.provisioned
		steps.Given().Zones(twoZones, theGeneralNodePool())
		stopTheLab()
		steps.WaitFor().AllNodesReady()
	}, anObserverDiscardingEachBoard(), anInjectorReportingNothing(), func(Outcome) {})
	if err != nil {
		t.Fatalf("carry out the Scenario %s until the lab stopped: %v", theScenarioStoppedPartWay, err)
	}

	expectRemoved(t, provisioned)
}

func TestAnInteractiveRunWhoseStepCannotBeCarriedOut_ShouldHoldTheClusterAsThatStepLeftIt(t *testing.T) {
	t.Parallel()
	running := aRunnerOnThisMachine(t)
	holding, stopTheLab := context.WithCancel(t.Context())
	var provisioned *cluster.SimulatedCluster
	var readyWhileHeld []scenario.Node

	held, err := running.holdNamed(holding, theScenarioEndedAtARefusal, func(steps *Scenario) {
		provisioned = steps.provisioned
		steps.Given().Zones(twoZones, theGeneralNodePool())
		steps.WaitFor().AllNodesReady()
		steps.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), aSpecificationTheConstructorRefuses())
	}, anObserverDiscardingEachBoard(), anInjectorReportingNothing(), func(Outcome) {
		readyWhileHeld = theReadyNodesOf(t, provisioned)
		stopTheLab()
	})
	if err != nil {
		t.Fatalf("carry out the Scenario %s: %v", theScenarioEndedAtARefusal, err)
	}

	expect.Expect(t, held.EndedAt.Kind).To(expect.Equal(GivenStep))
	expect.Expect(t, readyWhileHeld).To(expect.HaveLen[scenario.Node](theNodesOfTwoZonesOfTwoEach))
}

// heldForClicks is a held run the Live view's clicks reach through its
// Injector, with what its Steps declared.
type heldForClicks struct {
	injector    *Injector
	provisioned *cluster.SimulatedCluster
	topology    scenario.Topology
	deployment  scenario.Deployment
	boards      <-chan scenario.PlacementBoard
	// whens is written by the Injector while the test's own click runs, so the
	// test reads it once that click returns.
	whens []string
}

// aHeldRunTakingClicks returns once the held run's Outcome is reported, so a
// click from the test is one the held run takes. The cluster is removed when
// the test ends.
func aHeldRunTakingClicks(t *testing.T, name string) *heldForClicks {
	t.Helper()

	running := aRunnerOnThisMachine(t)
	holding, stopTheLab := context.WithCancel(t.Context())
	held := &heldForClicks{}
	held.injector = NewInjector(func(when Step) { held.whens = append(held.whens, when.String()) })
	observer, boards := anObserverHandingOverEachPlacementBoard(holding)
	held.boards = boards
	reported, ended := make(chan struct{}), make(chan error, 1)
	go func() {
		_, err := running.holdNamed(holding, name, func(steps *Scenario) {
			held.provisioned = steps.provisioned
			steps.Given().Zones(twoZones, theGeneralNodePool())
			steps.WaitFor().AllNodesReady()
			steps.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())
			steps.WaitFor().AllPodsReady()
			held.topology, held.deployment = steps.topology, steps.deployment
		}, observer, held.injector, func(Outcome) { close(reported) })
		ended <- err
	}()
	t.Cleanup(func() {
		stopTheLab()
		if err := <-ended; err != nil {
			t.Errorf("carry out the held Scenario %s: %v", name, err)
		}
	})

	select {
	case <-reported:
	case err := <-ended:
		t.Fatalf("the held Scenario %s ended before it reported: %v", name, err)
	}

	return held
}

// anObserverHandingOverEachPlacementBoard blocks the observer until the test
// takes the board, or the run lets go of the cluster.
func anObserverHandingOverEachPlacementBoard(holding context.Context) (Observer, <-chan scenario.PlacementBoard) {
	boards := make(chan scenario.PlacementBoard)

	return Observer{Every: anObserverIntervalForATest, Observed: func(board scenario.PlacementBoard, _ error) {
		select {
		case boards <- board:
		case <-holding.Done():
		}
	}}, boards
}

// aPodPlacedOn takes boards until one places a Pod on the Node, so the Pod is
// one the Injector's latest board holds: the observer hands a board over only
// once the Injector has it. The board held over the Steps may predate the
// Pods, so no fixed count of boards is enough.
func (h *heldForClicks) aPodPlacedOn(t *testing.T, node string) scenario.BoardPod {
	t.Helper()

	giveUp := time.After(scenario.WaitingTime)
	for {
		select {
		case board := <-h.boards:
			if pod, placed := aPodOnBoardPlacedOn(board, node); placed {
				return pod
			}
		case <-giveUp:
			t.Fatalf("no board of the held Scenario placed a Pod on Node %s within %s", node, scenario.WaitingTime)
		}
	}
}

func aPodOnBoardPlacedOn(board scenario.PlacementBoard, node string) (scenario.BoardPod, bool) {
	for _, zone := range board.Zones() {
		for _, onBoard := range zone.Nodes {
			if onBoard.Name == node && len(onBoard.Pods) > 0 {
				return onBoard.Pods[0], true
			}
		}
	}

	return scenario.BoardPod{}, false
}

func expectInjected(t *testing.T, injected error) {
	t.Helper()

	if injected != nil {
		t.Fatalf("inject the Failure clicked: %v", injected)
	}
}

func (h *heldForClicks) theFirstNode() string {
	return h.topology.Nodes()[0].Name
}

func (h *heldForClicks) theFirstZone() string {
	return h.topology.Zones()[0]
}

func (h *heldForClicks) theNodesOfZone(zone string) []string {
	var names []string
	for _, node := range h.topology.Nodes() {
		if node.Zone == zone {
			names = append(names, node.Name)
		}
	}

	return names
}

func (h *heldForClicks) everyNodeBut(stopped ...string) []string {
	var names []string
	for _, node := range h.topology.Nodes() {
		if !slices.Contains(stopped, node.Name) {
			names = append(names, node.Name)
		}
	}

	return names
}

func (h *heldForClicks) theReadyNodeNames(t *testing.T) []string {
	t.Helper()

	return nodeNamesOf(theReadyNodesOf(t, h.provisioned))
}

// thePodNamesOnceReplaced waits for every Pod the Deployment declares to be
// Ready again, so a lost Pod's replacement is among them.
func (h *heldForClicks) thePodNamesOnceReplaced(t *testing.T) []string {
	t.Helper()

	replaced, err := h.provisioned.AwaitPodsReady(t.Context(), h.deployment)
	if err != nil {
		t.Fatalf("await the Deployment %s replacing its lost Pod: %v", h.deployment.Name(), err)
	}
	names := make([]string, 0, len(replaced.Pods()))
	for _, pod := range replaced.Pods() {
		names = append(names, pod.Name)
	}

	return names
}

func nodeNamesOf(nodes []scenario.Node) []string {
	names := make([]string, 0, len(nodes))
	for _, node := range nodes {
		names = append(names, node.Name)
	}

	return names
}

// lostAndReplaced is what a Pod Failure leaves: the lost Pod gone, and as many
// Pods as declared.
type lostAndReplaced struct {
	Gone bool
	Pods int
}

func whatTheLossLeft(lost string, pods []string) lostAndReplaced {
	return lostAndReplaced{Gone: !slices.Contains(pods, lost), Pods: len(pods)}
}

const (
	theScenarioClickedOnANode          = "a-scenario-clicked-on-a-node"
	theScenarioClickedOnAZone          = "a-scenario-clicked-on-a-zone"
	theScenarioClickedOnAPod           = "a-scenario-clicked-on-a-pod"
	theScenarioReportingItsClicks      = "a-scenario-reporting-its-clicks"
	theScenarioClickedOnNothing        = "a-scenario-clicked-on-nothing"
	theScenarioClickedWhileItsStepsRun = "a-scenario-clicked-while-its-steps-run"
	theScenarioClickedOnAPartlyStopped = "a-scenario-clicked-on-a-partly-stopped-zone"
	theScenarioClickedOnAPodOfAStopped = "a-scenario-clicked-on-a-pod-of-a-stopped-node"
	theScenarioClickedOnAStoppedNode   = "a-scenario-clicked-on-a-stopped-node"
	theScenarioClickedOnAStoppedZone   = "a-scenario-clicked-on-a-stopped-zone"
	theScenarioClickedOnANodeOfAZone   = "a-scenario-clicked-on-a-node-of-a-stopped-zone"
	theScenarioClickedOnARestoredNode  = "a-scenario-clicked-on-a-restored-node"
	aNameNoNodeZoneOrPodCarries        = "zone-9-node-9"
	noWhenReported                     = 0
	theLostPodGone                     = true
)

func TestAClickOnANodesBoxInTheLiveView_ShouldStopThatNodeRespondingAsANodeFailureDoes(t *testing.T) {
	t.Parallel()
	held := aHeldRunTakingClicks(t, theScenarioClickedOnANode)
	clicked := held.theFirstNode()

	expectInjected(t, held.injector.ClickNode(clicked))

	expect.Expect(t, held.theReadyNodeNames(t)).To(expect.Equal(held.everyNodeBut(clicked)))
}

func TestAClickOnAZonesBoxInTheLiveView_ShouldStopEveryNodeOfThatZoneRespondingAtOnce(t *testing.T) {
	t.Parallel()
	held := aHeldRunTakingClicks(t, theScenarioClickedOnAZone)
	clicked := held.theFirstZone()

	expectInjected(t, held.injector.ClickZone(clicked))

	expect.Expect(t, held.theReadyNodeNames(t)).To(expect.Equal(held.everyNodeBut(held.theNodesOfZone(clicked)...)))
}

func TestAClickOnAPlacedPodsBoxInTheLiveView_ShouldLoseThatPodAndItsDeploymentShouldReplaceIt(t *testing.T) {
	t.Parallel()
	held := aHeldRunTakingClicks(t, theScenarioClickedOnAPod)
	clicked := held.aPodPlacedOn(t, held.theFirstNode())

	expectInjected(t, held.injector.LosePod(clicked.Name))

	expect.Expect(t, whatTheLossLeft(clicked.Name, held.thePodNamesOnceReplaced(t))).
		To(expect.Equal(lostAndReplaced{Gone: theLostPodGone, Pods: podsOverEveryNode}))
}

// The Pod is lost first, since a Node Failure soon has the control plane move
// it away.
func TestAFailureClickedInTheLiveView_ShouldReportAWhenToTheTerminalNamingThePodNodeOrZone(t *testing.T) {
	t.Parallel()
	held := aHeldRunTakingClicks(t, theScenarioReportingItsClicks)
	stoppedZone := held.theFirstZone()
	stoppedNode := held.everyNodeBut(held.theNodesOfZone(stoppedZone)...)[0]
	lostPod := held.aPodPlacedOn(t, held.theFirstNode())

	expectInjected(t, held.injector.LosePod(lostPod.Name))
	expectInjected(t, held.injector.ClickNode(stoppedNode))
	expectInjected(t, held.injector.ClickZone(stoppedZone))

	expect.Expect(t, held.whens).To(expect.Equal([]string{
		"When Pod " + lostPod.BoardName() + " is lost",
		"When Node " + stoppedNode + " stops responding",
		"When Zone " + stoppedZone + " stops responding",
	}))
}

// A Pending Pod's box carries no Failure, and the board refuses a Failure of a
// Pod it holds unplaced, so what reaches the held run from anywhere else on the
// page is a name resolving to nothing.
func TestAClickOnAPendingPodOrAnyOtherPartOfTheLiveView_ShouldChangeNothingAndReportNothing(t *testing.T) {
	t.Parallel()
	held := aHeldRunTakingClicks(t, theScenarioClickedOnNothing)

	expectInjected(t, held.injector.ClickNode(aNameNoNodeZoneOrPodCarries))
	expectInjected(t, held.injector.ClickZone(aNameNoNodeZoneOrPodCarries))
	expectInjected(t, held.injector.LosePod(aNameNoNodeZoneOrPodCarries))

	expect.Expect(t, held.theReadyNodeNames(t)).To(expect.Equal(held.everyNodeBut()))
	expect.Expect(t, held.whens).To(expect.HaveLen[string](noWhenReported))
}

func TestAClickInTheLiveViewWhileTheStepsStillRun_ShouldChangeNothing(t *testing.T) {
	t.Parallel()
	running := aRunnerOnThisMachine(t)
	holding, stopTheLab := context.WithCancel(t.Context())
	var whens []string
	injector := NewInjector(func(when Step) { whens = append(whens, when.String()) })
	var provisioned *cluster.SimulatedCluster
	var declared scenario.Topology
	var readyAfterTheClick []scenario.Node

	_, err := running.holdNamed(holding, theScenarioClickedWhileItsStepsRun, func(steps *Scenario) {
		provisioned = steps.provisioned
		steps.Given().Zones(twoZones, theGeneralNodePool())
		steps.WaitFor().AllNodesReady()
		declared = steps.topology
		if err := injector.ClickNode(declared.Nodes()[0].Name); err != nil {
			t.Errorf("click a Node while the Steps run: %v", err)
		}
		readyAfterTheClick = theReadyNodesOf(t, provisioned)
	}, anObserverDiscardingEachBoard(), injector, func(Outcome) { stopTheLab() })
	if err != nil {
		t.Fatalf("carry out the Scenario %s: %v", theScenarioClickedWhileItsStepsRun, err)
	}

	expect.Expect(t, readyAfterTheClick).To(expect.HaveLen[scenario.Node](theNodesOfTwoZonesOfTwoEach))
	expect.Expect(t, whens).To(expect.HaveLen[string](noWhenReported))
}

func TestAClickOnAZoneWithOnlySomeNodesStopped_ShouldStopTheOthersAndNameTheZone(t *testing.T) {
	t.Parallel()
	held := aHeldRunTakingClicks(t, theScenarioClickedOnAPartlyStopped)
	zone := held.theFirstZone()
	stoppedFirst := held.theNodesOfZone(zone)[0]
	expectInjected(t, held.injector.ClickNode(stoppedFirst))

	expectInjected(t, held.injector.ClickZone(zone))

	expect.Expect(t, held.theReadyNodeNames(t)).To(expect.Equal(held.everyNodeBut(held.theNodesOfZone(zone)...)))
	expect.Expect(t, held.whens).To(expect.Equal([]string{
		"When Node " + stoppedFirst + " stops responding",
		"When Zone " + zone + " stops responding",
	}))
}

func TestAClickOnAPlacedPodOfAStoppedNode_ShouldLoseItAllTheSame(t *testing.T) {
	t.Parallel()
	held := aHeldRunTakingClicks(t, theScenarioClickedOnAPodOfAStopped)
	stopped := held.theFirstNode()
	clicked := held.aPodPlacedOn(t, stopped)
	expectInjected(t, held.injector.ClickNode(stopped))

	expectInjected(t, held.injector.LosePod(clicked.Name))

	expect.Expect(t, whatTheLossLeft(clicked.Name, held.thePodNamesOnceReplaced(t))).
		To(expect.Equal(lostAndReplaced{Gone: theLostPodGone, Pods: podsOverEveryNode}))
}

func TestAClickOnAStoppedNodesBoxInTheLiveView_ShouldMakeThatNodeRespondAgainAndReportAWhen(t *testing.T) {
	t.Parallel()
	held := aHeldRunTakingClicks(t, theScenarioClickedOnAStoppedNode)
	clicked := held.theFirstNode()
	expectInjected(t, held.injector.ClickNode(clicked))

	expectInjected(t, held.injector.ClickNode(clicked))

	expect.Expect(t, held.theReadyNodeNames(t)).To(expect.Equal(held.everyNodeBut()))
	expect.Expect(t, held.whens).To(expect.Equal([]string{
		"When Node " + clicked + " stops responding",
		"When Node " + clicked + " responds again",
	}))
}

func TestAClickOnAZoneWhoseEveryNodeStopped_ShouldMakeEveryNodeOfItRespondAgainAndNameTheZone(t *testing.T) {
	t.Parallel()
	held := aHeldRunTakingClicks(t, theScenarioClickedOnAStoppedZone)
	clicked := held.theFirstZone()
	expectInjected(t, held.injector.ClickZone(clicked))

	expectInjected(t, held.injector.ClickZone(clicked))

	expect.Expect(t, held.theReadyNodeNames(t)).To(expect.Equal(held.everyNodeBut()))
	expect.Expect(t, held.whens).To(expect.Equal([]string{
		"When Zone " + clicked + " stops responding",
		"When Zone " + clicked + " responds again",
	}))
}

func TestAClickOnOneNodeOfAStoppedZone_ShouldMakeThatNodeAloneRespondAgain(t *testing.T) {
	t.Parallel()
	held := aHeldRunTakingClicks(t, theScenarioClickedOnANodeOfAZone)
	zone := held.theFirstZone()
	clicked, stillStopped := held.theNodesOfZone(zone)[0], held.theNodesOfZone(zone)[1:]
	expectInjected(t, held.injector.ClickZone(zone))

	expectInjected(t, held.injector.ClickNode(clicked))

	expect.Expect(t, held.theReadyNodeNames(t)).To(expect.Equal(held.everyNodeBut(stillStopped...)))
	expect.Expect(t, held.whens).To(expect.Equal([]string{
		"When Zone " + zone + " stops responding",
		"When Node " + clicked + " responds again",
	}))
}

func TestAClickOnANodeThatRespondsAgain_ShouldStopItRespondingOnceMore(t *testing.T) {
	t.Parallel()
	held := aHeldRunTakingClicks(t, theScenarioClickedOnARestoredNode)
	clicked := held.theFirstNode()
	expectInjected(t, held.injector.ClickNode(clicked))
	expectInjected(t, held.injector.ClickNode(clicked))

	expectInjected(t, held.injector.ClickNode(clicked))

	expect.Expect(t, held.theReadyNodeNames(t)).To(expect.Equal(held.everyNodeBut(clicked)))
	expect.Expect(t, held.whens).To(expect.Equal([]string{
		"When Node " + clicked + " stops responding",
		"When Node " + clicked + " responds again",
		"When Node " + clicked + " stops responding",
	}))
}
