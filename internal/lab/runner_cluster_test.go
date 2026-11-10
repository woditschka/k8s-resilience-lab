// These tests cross to a real Simulated Cluster. One judges the order in which
// a panicking Scenario lets go of the cluster, its recording and the
// recording's file: no stand-in has a recording to outlive its file. The others
// judge what an Interactive run holds, observes and removes.
package lab

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/cluster"
	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/lab/descheduler"
	"github.com/woditschka/k8s-resilience-lab/internal/lab/node"
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

const (
	theScenarioMakingNodesRespondAgain = "a-scenario-making-its-stopped-nodes-respond-again"
	theScenarioMakingZonesRespondAgain = "a-scenario-making-its-stopped-zones-respond-again"
)

// whensAndCause is what a run of a Scenario reports of its Whens, and why it
// ended where it did, empty for one that carried out every Step.
type whensAndCause struct {
	Whens []string
	Cause string
}

// aRunOfAZoneFailureFollowedBy stops one Zone, so more than one Node is stopped
// by one When, and then waits for every Node Ready, which only a Node that
// responds again reaches.
func aRunOfAZoneFailureFollowedBy(t *testing.T, name string, responding func(When)) whensAndCause {
	t.Helper()

	ran, err := aRunnerOnThisMachine(t).runNamed(t.Context(), name, func(steps *Scenario) {
		steps.Given().Zones(twoZones, theGeneralNodePool())
		steps.WaitFor().AllNodesReady()
		steps.When().ZonesFail(oneLost)
		responding(steps.When())
		steps.WaitFor().AllNodesReady()
	})
	if err != nil {
		t.Fatalf("carry out the Scenario %s: %v", name, err)
	}

	var whens []string
	for _, step := range ran.Steps {
		if step.Kind == WhenStep {
			whens = append(whens, step.Name)
		}
	}

	return whensAndCause{Whens: whens, Cause: causeOf(ran)}
}

func theFirstZoneFailing(topology scenario.Topology) string {
	return zonesFailing(oneLost) + ", " + topology.Zones()[0]
}

func TestAWhenMakingNodesRespondAgain_ShouldMakeEveryStoppedNodeRespondAgainAndReportWhich(t *testing.T) {
	t.Parallel()
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	stopped := nodeNamesOf(theNodesOutside(declared, declared.Zones()[1]))

	ran := aRunOfAZoneFailureFollowedBy(t, theScenarioMakingNodesRespondAgain, When.NodesRespondAgain)

	expect.Expect(t, ran).To(expect.Equal(whensAndCause{Whens: []string{
		theFirstZoneFailing(declared),
		theStoppedNodesRespondingAgain + ", " + strings.Join(stopped, ", "),
	}}))
}

func TestAWhenMakingZonesRespondAgain_ShouldMakeEveryNodeOfEveryStoppedZoneRespondAgainAndReportWhich(t *testing.T) {
	t.Parallel()
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)

	ran := aRunOfAZoneFailureFollowedBy(t, theScenarioMakingZonesRespondAgain, When.ZonesRespondAgain)

	expect.Expect(t, ran).To(expect.Equal(whensAndCause{Whens: []string{
		theFirstZoneFailing(declared),
		theStoppedZonesRespondingAgain + ", " + declared.Zones()[0],
	}}))
}

const (
	theScenarioLosingPods = "a-scenario-losing-pods-of-its-deployment"
	twoPodsLost           = 2
)

func TestAWhenLosingPodsOfADeployment_ShouldLoseThatManyOfItsPlacedPodsInOneStepAndReportEach(t *testing.T) {
	t.Parallel()
	ran, err := aRunnerOnThisMachine(t).runNamed(t.Context(), theScenarioLosingPods, func(steps *Scenario) {
		steps.Given().Zones(twoZones, theGeneralNodePool())
		steps.WaitFor().AllNodesReady()
		steps.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())
		steps.WaitFor().AllPodsReady()
		steps.When().PodsFail(someDeploymentName, twoPodsLost)
		steps.WaitFor().AllPodsReady()
	})
	if err != nil {
		t.Fatalf("carry out the Scenario %s: %v", theScenarioLosingPods, err)
	}

	lost := strings.Split(strings.TrimPrefix(theLastWhenOf(ran).Name, podsFailing(twoPodsLost, someDeploymentName)+", "), ", ")
	board := ran.Steps[len(ran.Steps)-1].Board.Lines()
	expect.Expect(t, causeOf(ran)).To(expect.Equal(""))
	expect.Expect(t, len(lost)).To(expect.Equal(twoPodsLost))
	expect.Expect(t, len(board) > 0).To(expect.Equal(true))
	expect.Expect(t, theNamesHeldBy(board, lost)).To(expect.Equal([]string{}))
}

const theScenarioLosingPodsBeforeANodeFails = "a-scenario-losing-pods-before-a-node-fails"

// A Pod Failure stops no Node, so the Floor of the Deployment declaring a
// Failure tolerance is read against the Node the later When stops, from that
// When's note on, never from the Pod Failure's. It runs on a cluster since
// only a real loss carries the When out.
func TestAWhenLosingPodsBeforeANodeFailure_ShouldLeaveTheFloorJudgedAgainstTheNodeTheLaterWhenStops(t *testing.T) {
	t.Parallel()
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	ran, err := aRunnerOnThisMachine(t).runNamed(t.Context(), theScenarioLosingPodsBeforeANodeFails, func(steps *Scenario) {
		steps.Given().Zones(twoZones, theGeneralNodePool())
		steps.WaitFor().AllNodesReady()
		steps.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())
		steps.Given().Deployment(anotherDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement()).
			SurvivesLosing(Nodes(oneLost))
		steps.WaitFor().AllPodsReady()
		steps.When().PodsFail(someDeploymentName, oneLost)
		steps.WaitFor().AllPodsReady()
		steps.When().NodesFail(oneLost)
		steps.WaitFor().AllPodsReady()
		steps.Then().EnoughPodsStayedReady()
	})
	if err != nil {
		t.Fatalf("carry out the Scenario %s: %v", theScenarioLosingPodsBeforeANodeFails, err)
	}

	judged := aDeploymentNamedSurvivingTheLossOf(t, anotherDeploymentName, podsOverEveryNode, Nodes(oneLost), declared)
	readings, err := ran.Timeline.Floors([]scenario.Deployment{judged},
		[]string{string(someDeploymentName), string(anotherDeploymentName)},
		[]scenario.WhenNodes{{}, {Stopped: []string{theNodeThatStopped(declared)}}})
	if err != nil {
		t.Fatalf("read the Floors over the recorded Timeline: %v", err)
	}
	expect.Expect(t, causeOf(ran)).To(expect.Equal(""))
	expect.Expect(t, ran.Steps[len(ran.Steps)-1]).To(expect.Equal(enoughPodsStayedReadyStep(linesOf(readings)...)))
}

func theNamesHeldBy(lines, names []string) []string {
	held := []string{}
	for _, name := range names {
		if strings.Contains(strings.Join(lines, "\n"), name) {
			held = append(held, name)
		}
	}

	return held
}

func theLastWhenOf(ran Outcome) Step {
	var last Step
	for _, step := range ran.Steps {
		if step.Kind == WhenStep {
			last = step
		}
	}

	return last
}

const (
	theScenarioRunningADeschedulerPass = "a-scenario-running-a-descheduler-pass"
	theScenarioWhosePassEvictsNothing  = "a-scenario-whose-descheduler-pass-evicts-nothing"
	theScenarioWhosePassCannotRun      = "a-scenario-whose-descheduler-pass-cannot-run"
	thePolicyRestoringTheSpread        = "even-spread-restored"
	aPolicyNameThePassRefuses          = "never-scheduled"
	onePodDescheduled                  = 1
	aPassThatExitedNonZero             = "exited with status"
	anActionTheDeschedulerRefuses      = descheduler.UnsatisfiableConstraintAction("NeverSchedule")
)

func aPolicyRestoringTheSpread() descheduler.Policy {
	return aPolicyBalancingTheSpreadUnder(thePolicyRestoringTheSpread, descheduler.DoNotSchedule)
}

// The Descheduler validates the constraint actions it is given, so a pass under
// this policy exits non-zero before it evicts anything.
func aPolicyThePassRefuses() descheduler.Policy {
	return aPolicyBalancingTheSpreadUnder(aPolicyNameThePassRefuses, anActionTheDeschedulerRefuses)
}

func aPolicyBalancingTheSpreadUnder(name string, action descheduler.UnsatisfiableConstraintAction) descheduler.Policy {
	return descheduler.Policy{Profiles: []descheduler.Profile{{
		Name: name,
		PluginConfig: []descheduler.PluginConfig{{
			Name: descheduler.RemovePodsViolatingTopologySpreadConstraint,
			Args: descheduler.RemovePodsViolatingTopologySpreadConstraintArgs{
				Constraints: []descheduler.UnsatisfiableConstraintAction{action},
			},
		}},
		Plugins: descheduler.Plugins{
			Balance: descheduler.PluginSet{Enabled: []descheduler.PluginName{descheduler.RemovePodsViolatingTopologySpreadConstraint}},
		},
	}}}
}

func deschedulerPassStepUnder(name string) Step {
	step := deschedulerPassStepNamingNoPolicy()
	step.Name += " under " + name

	return step
}

func thePodsDescheduledBeforeTheStepAfter(recorded scenario.Timeline, noted Step) []string {
	descheduled := []string{}
	passed := false
	for _, entry := range recorded.Entries() {
		switch {
		case entry.Kind == scenario.StepObject && passed:
			return descheduled
		case entry.Kind == scenario.StepObject:
			passed = entry.Detail == noted.String()
		case passed && entry.Reason == scenario.EvictedByTheDescheduler:
			descheduled = append(descheduled, entry.Name)
		}
	}

	return descheduled
}

// A Node that stops responding and responds again leaves it empty and another
// Node of its Zone holding two, past the hostname spread, so the pass evicts
// one Pod. Its eviction recorded before the next Step's note shows the next
// Step started once the pass ended.
func TestAWhenRunningADeschedulerPass_ShouldRunOnePassUnderTheDeclaredPolicyBeforeTheNextStep(t *testing.T) {
	t.Parallel()
	ran, err := aRunnerOnThisMachine(t).runNamed(t.Context(), theScenarioRunningADeschedulerPass, func(steps *Scenario) {
		steps.Given().Zones(twoZones, theGeneralNodePool())
		steps.WaitFor().AllNodesReady()
		steps.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())
		steps.Given().Descheduler(aPolicyRestoringTheSpread())
		steps.WaitFor().AllPodsReady()
		steps.When().NodesFail(oneLost)
		steps.WaitFor().AllPodsReady()
		steps.When().NodesRespondAgain()
		steps.WaitFor().AllNodesReady()
		steps.WaitFor().PlacementSettled()
		steps.When().DeschedulerRunsOnce()
		steps.WaitFor().AllPodsReady()
	})
	if err != nil {
		t.Fatalf("carry out the Scenario %s: %v", theScenarioRunningADeschedulerPass, err)
	}

	pass := deschedulerPassStepUnder(thePolicyRestoringTheSpread)
	descheduled := thePodsDescheduledBeforeTheStepAfter(ran.Timeline, pass)
	expect.Expect(t, causeOf(ran)).To(expect.Equal(""))
	expect.Expect(t, theLastWhenOf(ran)).To(expect.Equal(pass))
	expect.Expect(t, len(descheduled)).To(expect.Equal(onePodDescheduled))
}

func TestADeschedulerPassThatCannotRunOrDoesNotEnd_ShouldEndTheScenarioAtThatWhenNamingWhy(t *testing.T) {
	t.Parallel()
	ran, err := aRunnerOnThisMachine(t).runNamed(t.Context(), theScenarioWhosePassCannotRun, func(steps *Scenario) {
		steps.Given().Zones(twoZones, theGeneralNodePool())
		steps.WaitFor().AllNodesReady()
		steps.Given().Descheduler(aPolicyThePassRefuses())
		steps.When().DeschedulerRunsOnce()
		steps.WaitFor().AllNodesReady()
	})
	if err != nil {
		t.Fatalf("carry out the Scenario %s: %v", theScenarioWhosePassCannotRun, err)
	}

	expect.Expect(t, ran.EndedAt).To(expect.Equal(deschedulerPassStepUnder(aPolicyNameThePassRefuses)))
	causeNames(t, ran, aPassThatExitedNonZero)
}

// One Pod on every Node holds the hostname spread, so the pass finds no Pod to
// evict, and the Wait after it narrates no change.
func TestADeschedulerPassEvictingNothing_ShouldLeaveThePlacementAsItStands(t *testing.T) {
	t.Parallel()
	ran, err := aRunnerOnThisMachine(t).runNamed(t.Context(), theScenarioWhosePassEvictsNothing, func(steps *Scenario) {
		steps.Given().Zones(twoZones, theGeneralNodePool())
		steps.WaitFor().AllNodesReady()
		steps.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())
		steps.Given().Descheduler(aPolicyRestoringTheSpread())
		steps.WaitFor().AllPodsReady()
		steps.WaitFor().PlacementSettled()
		steps.When().DeschedulerRunsOnce()
		steps.WaitFor().AllPodsReady()
	})
	if err != nil {
		t.Fatalf("carry out the Scenario %s: %v", theScenarioWhosePassEvictsNothing, err)
	}

	expect.Expect(t, causeOf(ran)).To(expect.Equal(""))
	expect.Expect(t, ran.Steps[len(ran.Steps)-1].Narrative).To(expect.Equal(scenario.WaitNarrative{}))
}

const (
	theScenarioRunningPassesOnAnInterval  = "a-scenario-running-descheduler-passes-on-an-interval"
	theScenarioSettlingBeforeItsFirstPass = "a-scenario-settling-before-its-first-descheduler-pass"
	theScenarioWhoseIntervalCannotRun     = "a-scenario-whose-descheduler-interval-cannot-run"
	theScenarioEndingWithItsInterval      = "a-scenario-ending-with-its-descheduler-interval"
	theScenarioEndedAtAStepByItsInterval  = "a-scenario-ended-at-a-step-with-its-descheduler-interval"
	aPassEverySecond                      = 1
	// The Settled Wait is held to less than the interval, so reaching it in
	// time shows it never waited on a pass.
	anIntervalLongerThanTheSettledWait = 60
	aSettledWaitShorterThanTheInterval = 30
)

// A pass begun after a Wait for one evicting nothing is what ends that Wait,
// so the second such Wait reached shows a further pass ran on the interval.
func TestADeschedulerDeclaredWithAnInterval_ShouldRunAPassOnEachIntervalUntilTheScenarioEnds(t *testing.T) {
	t.Parallel()
	ran, err := aRunnerOnThisMachine(t).runNamed(t.Context(), theScenarioRunningPassesOnAnInterval, func(steps *Scenario) {
		steps.Given().Zones(twoZones, theGeneralNodePool())
		steps.WaitFor().AllNodesReady()
		steps.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())
		steps.Given().Descheduler(aPolicyRestoringTheSpread()).EverySeconds(aPassEverySecond)
		steps.WaitFor().AllPodsReady()
		steps.WaitFor().DeschedulerPassEvictingNothing()
		steps.WaitFor().DeschedulerPassEvictingNothing()
	})
	if err != nil {
		t.Fatalf("carry out the Scenario %s: %v", theScenarioRunningPassesOnAnInterval, err)
	}

	expect.Expect(t, causeOf(ran)).To(expect.Equal(""))
	expect.Expect(t, theWaitsReachedIn(ran)).To(expect.Equal([]string{
		allNodesReady, allPodsReady, aDeschedulerPassEvictingNothing, aDeschedulerPassEvictingNothing,
	}))
}

func theWaitsReachedIn(ran Outcome) []string {
	var reached []string
	for _, step := range ran.Steps {
		if step.Kind == WaitStep && step.Reached {
			reached = append(reached, step.Name)
		}
	}

	return reached
}

func TestAWaitForAPlacementSettledUnderADeschedulerOnAnInterval_ShouldObserveThePlacementAlone(t *testing.T) {
	t.Parallel()
	ran, err := aRunnerOnThisMachine(t).runNamed(t.Context(), theScenarioSettlingBeforeItsFirstPass, func(steps *Scenario) {
		steps.Given().Zones(twoZones, theGeneralNodePool())
		steps.WaitFor().AllNodesReady()
		steps.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())
		steps.Given().Descheduler(aPolicyRestoringTheSpread()).EverySeconds(anIntervalLongerThanTheSettledWait)
		steps.WaitFor().PlacementSettledWithinSeconds(aSettledWaitShorterThanTheInterval)
	})
	if err != nil {
		t.Fatalf("carry out the Scenario %s: %v", theScenarioSettlingBeforeItsFirstPass, err)
	}

	expect.Expect(t, causeOf(ran)).To(expect.Equal(""))
	expect.Expect(t, theWaitsReachedIn(ran)).To(expect.Equal([]string{allNodesReady, theSettledPlacement}))
}

func TestADeschedulerOnAnIntervalThatCannotStartOrStops_ShouldEndTheScenarioNamingWhy(t *testing.T) {
	t.Parallel()
	ran, err := aRunnerOnThisMachine(t).runNamed(t.Context(), theScenarioWhoseIntervalCannotRun, func(steps *Scenario) {
		steps.Given().Zones(twoZones, theGeneralNodePool())
		steps.WaitFor().AllNodesReady()
		steps.Given().Descheduler(aPolicyThePassRefuses()).EverySeconds(aPassEverySecond)
		steps.WaitFor().AllNodesReady()
		steps.WaitFor().DeschedulerPassEvictingNothing()
	})
	if err != nil {
		t.Fatalf("carry out the Scenario %s: %v", theScenarioWhoseIntervalCannotRun, err)
	}

	expect.Expect(t, ran.EndedAt.Name).To(expect.Equal(aDeschedulerPassEvictingNothing))
	causeNames(t, ran, aPassThatExitedNonZero)
}

// The engine is asked for every container named after the Scenario's cluster,
// its pass containers included, so the test names no container of its own.
func theContainersNamedAfter(t *testing.T, engine cluster.Endpoint, clusterName string) []string {
	t.Helper()

	asked := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", engine.Socket())
		},
	}}
	defer asked.CloseIdleConnections()
	filters, err := json.Marshal(map[string][]string{"name": {clusterName}})
	if err != nil {
		t.Fatalf("compose the engine's filter for %s: %v", clusterName, err)
	}
	request, err := http.NewRequestWithContext(context.WithoutCancel(t.Context()), http.MethodGet,
		"http://engine/containers/json?"+url.Values{"all": {"true"}, "filters": {string(filters)}}.Encode(), nil)
	if err != nil {
		t.Fatalf("build the engine's listing of %s: %v", clusterName, err)
	}
	answer, err := asked.Do(request)
	if err != nil {
		t.Fatalf("list the containers named after %s: %v", clusterName, err)
	}
	defer func() { _ = answer.Body.Close() }()

	var listed []struct{ Names []string }
	if err := json.NewDecoder(answer.Body).Decode(&listed); err != nil {
		t.Fatalf("read the containers named after %s: %v", clusterName, err)
	}
	named := []string{}
	for _, container := range listed {
		named = append(named, container.Names...)
	}

	return named
}

// Each ending follows a Wait a pass ended, so a pass on the interval runs or is
// about to as the Scenario ends.
func TestAScenarioWhoseDeschedulerRanOnAnInterval_ShouldLeaveNothingOfItsDeschedulerWhenItEnds(t *testing.T) {
	t.Parallel()
	endings := map[string]struct {
		name   string
		ending func(*Scenario)
	}{
		"every Step carried out": {name: theScenarioEndingWithItsInterval, ending: func(*Scenario) {}},
		"a Step ending it":       {name: theScenarioEndedAtAStepByItsInterval, ending: func(steps *Scenario) { steps.When().DeschedulerRunsOnce() }},
	}

	for ended, by := range endings {
		t.Run(ended, func(t *testing.T) {
			t.Parallel()
			engine, err := cluster.DetectEngine(t.Context(), slog.New(slog.DiscardHandler))
			if err != nil {
				t.Fatalf("no container engine answers on this machine, so the Simulated Cluster cannot be driven: %v", err)
			}
			t.Cleanup(engine.Close)

			ran, err := NewRunner(engine, cluster.DefaultKubernetesVersion(), timeline.NewFolder(t.TempDir())).
				runNamed(t.Context(), by.name, func(steps *Scenario) {
					steps.Given().Zones(twoZones, theGeneralNodePool())
					steps.WaitFor().AllNodesReady()
					steps.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())
					steps.Given().Descheduler(aPolicyRestoringTheSpread()).EverySeconds(aPassEverySecond)
					steps.WaitFor().DeschedulerPassEvictingNothing()
					by.ending(steps)
				})
			if err != nil {
				t.Fatalf("carry out the Scenario %s: %v", by.name, err)
			}

			expect.Expect(t, theContainersNamedAfter(t, engine, ran.Cluster)).To(expect.Equal([]string{}))
		})
	}
}

const (
	theTwinOfAZoneThatRespondsAgain = "a-zone-that-responds-again-under-a-descheduler"
	oneEvictionPerPass              = 1
)

// The Scenario's own function lives in the scenarios package, which imports this
// one, so its Steps and policy are written out here as it writes them.
func aPolicyLikeEvenSpreadRestored() descheduler.Policy {
	return descheduler.Policy{
		MaxNoOfPodsToEvictTotal: oneEvictionPerPass,
		Profiles: []descheduler.Profile{{
			Name: thePolicyRestoringTheSpread,
			PluginConfig: []descheduler.PluginConfig{
				{Name: descheduler.DefaultEvictor, Args: descheduler.DefaultEvictorArgs{NodeFit: true}},
				{
					Name: descheduler.RemovePodsViolatingTopologySpreadConstraint,
					Args: descheduler.RemovePodsViolatingTopologySpreadConstraintArgs{
						Constraints: []descheduler.UnsatisfiableConstraintAction{descheduler.DoNotSchedule},
					},
				},
			},
			Plugins: descheduler.Plugins{
				Balance: descheduler.PluginSet{Enabled: []descheduler.PluginName{descheduler.RemovePodsViolatingTopologySpreadConstraint}},
			},
		}},
	}
}

func TestAZoneThatRespondsAgainUnderADescheduler_ShouldMovePodsBackUntilEveryZoneHoldsThreeAndEveryNodeOneWithTheFloorHeld(t *testing.T) {
	const (
		threeZones        = 3
		threeNodesPerZone = 3
		ninePods          = threeZones * threeNodesPerZone
		threePodsPerZone  = ninePods / threeZones
		onePodPerNode     = 1
		aRecoveryWait     = 30
	)
	t.Parallel()
	ran, err := aRunnerOnThisMachine(t).runNamed(t.Context(), theTwinOfAZoneThatRespondsAgain, func(steps *Scenario) {
		steps.Given().Zones(threeZones, node.Pool(generalNodePool, node.PerZone(threeNodesPerZone), roomyNodes()))
		steps.WaitFor().AllNodesReady()
		steps.Given().Deployment(someDeploymentName, ninePods, askingForNothing(), resilientPlacement()).SurvivesLosing(Zones(oneLost))
		steps.Given().Descheduler(aPolicyLikeEvenSpreadRestored()).EverySeconds(aPassEveryFiveSeconds)
		steps.WaitFor().AllPodsReady()
		steps.When().ZonesFail(oneLost)
		steps.WaitFor().AllPodsReadyWithinSeconds(aRecoveryWait)
		steps.When().ZonesRespondAgain()
		steps.WaitFor().AllNodesReadyWithinSeconds(aRecoveryWait)
		steps.WaitFor().DeschedulerPassEvictingNothing()
		steps.WaitFor().PlacementSettledWithinSeconds(aRecoveryWait)
		steps.Then().AllPodsRunning()
		steps.Then().PodsPerZone(threePodsPerZone)
		steps.Then().PodsPerNode(onePodPerNode)
		steps.Then().EnoughPodsStayedReady()
	})
	if err != nil {
		t.Fatalf("carry out the Scenario %s: %v", theTwinOfAZoneThatRespondsAgain, err)
	}

	expect.Expect(t, causeOf(ran)).To(expect.Equal(""))
	expect.Expect(t, ran.Findings).To(expect.Equal([]Finding(nil)))
	expect.Expect(t, len(theEvictionsByTheDeschedulerOn(ran.Timeline)) > 0).To(expect.Equal(true))
}

func theEvictionsByTheDeschedulerOn(recorded scenario.Timeline) []string {
	var evicted []string
	for _, entry := range recorded.Entries() {
		if entry.Reason == scenario.EvictedByTheDescheduler {
			evicted = append(evicted, entry.Name)
		}
	}

	return evicted
}
