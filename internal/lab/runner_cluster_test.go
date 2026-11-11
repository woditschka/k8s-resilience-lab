//go:build !unit

// These tests cross to a real Simulated Cluster. One judges the order in which
// a panicking Scenario lets go of the cluster, its recording and the
// recording's file: no stand-in has a recording to outlive its file. The others
// judge what an Interactive run holds, observes and removes.
package lab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
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
	theScenarioEndedBeforeItReports = "a-scenario-ended-before-it-reports"
	thePanickingScenarioOnACluster  = "a-scenario-that-panics-on-a-cluster"
	whatTheClusterScenarioSaid      = "a Step's own code panicked with the cluster recording"
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
		return running.runNamed(t.Context(), thePanickingScenarioOnACluster, noDeschedulerImage, func(steps *Scenario) {
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
	theHeldScenario                    = "a-scenario-held-before-its-first-when"
	theScenarioStoppedPartWay          = "a-scenario-stopped-while-its-steps-run"
	theScenarioEndedAtARefusal         = "a-scenario-held-as-a-refused-step-left-it"
	theScenarioFiledWhenStoppedPartWay = "a-scenario-filed-when-stopped-while-its-steps-run"
	theScenarioWhoseTimelineIsNotFiled = "a-scenario-whose-held-timeline-is-not-filed"
	aFileInTheFoldersPlace             = "not-a-folder"
	theStopEndsTheSteps                = true
	noTimelineReportedAtTheHold        = ""
	anyFileMode                        = 0o600
	anObserverIntervalForATest         = 200 * time.Millisecond
	theNodesOfTwoZonesOfTwoEach        = twoZones * twoNodesPerZone
)

func aRunnerOnThisMachine(t *testing.T) *Runner {
	t.Helper()

	return aRunnerFiledIn(t, timeline.NewFolder(t.TempDir()))
}

func aRunnerFiledIn(t *testing.T, folder timeline.Folder) *Runner {
	t.Helper()

	engine, err := cluster.DetectEngine(t.Context(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("no container engine answers on this machine, so the Simulated Cluster cannot be driven: %v", err)
	}
	t.Cleanup(engine.Close)

	return NewRunner(engine, cluster.DefaultKubernetesVersion(), folder)
}

// noDeschedulerImage is what a run declaring no Descheduler policy hands its
// Scenario.
var noDeschedulerImage = deschedulerImage{}

// aRunOnThisMachineOf carries out the named Scenario as a run of it alone does,
// obtaining the Descheduler image first where it declares a policy.
func aRunOnThisMachineOf(t *testing.T, name string, steps func(*Scenario)) (Outcome, error) {
	t.Helper()

	return aRunOf(t, aRunnerOnThisMachine(t), name, steps)
}

func aRunOf(t *testing.T, running *Runner, name string, steps func(*Scenario)) (Outcome, error) {
	t.Helper()

	image, err := running.obtainTheDeschedulerOf(t.Context(), []func(*Scenario){steps})
	if err != nil {
		t.Fatalf("obtain the Descheduler image of the Scenario %s: %v", name, err)
	}

	return running.runNamed(t.Context(), name, image, steps)
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

func anInjector() *Injector {
	return NewInjector()
}

func ignoringEachLineFollowed(string) {}

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

	held, err := running.holdNamed(holding, theHeldScenario, noDeschedulerImage, func(steps *Scenario) {
		provisioned = steps.provisioned
		steps.Given().Zones(twoZones, theGeneralNodePool())
		steps.WaitFor().AllNodesReady()
		steps.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())
		steps.WaitFor().AllPodsReady()
		declared = steps.topology

		steps.When().NodesFail(oneNodeFailing)
		steps.WaitFor().AllPodsReady()
	}, observer, anInjector(), func(Outcome) {
		readyWhileHeld = theReadyNodesOf(t, provisioned)
		boardWhileHeld = theBoardObservedOnceHeld(boards)
		stopTheLab()
	}, ignoringEachLineFollowed)
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

	_, err := running.holdNamed(holding, theScenarioStoppedPartWay, noDeschedulerImage, func(steps *Scenario) {
		provisioned = steps.provisioned
		steps.Given().Zones(twoZones, theGeneralNodePool())
		stopTheLab()
		steps.WaitFor().AllNodesReady()
	}, anObserverDiscardingEachBoard(), anInjector(), func(Outcome) {}, ignoringEachLineFollowed)
	if err != nil {
		t.Fatalf("carry out the Scenario %s until the lab stopped: %v", theScenarioStoppedPartWay, err)
	}

	expectRemoved(t, provisioned)
}

func TestAnInteractiveRunStoppedWhileTheStepsRun_ShouldFileItsTimelineAsAScenarioEndingPartWay(t *testing.T) {
	t.Parallel()
	running := aRunnerOnThisMachine(t)
	holding, stopTheLab := context.WithCancel(t.Context())
	var atTheHold Outcome

	stopped, err := running.holdNamed(holding, theScenarioFiledWhenStoppedPartWay, noDeschedulerImage, func(steps *Scenario) {
		steps.Given().Zones(twoZones, theGeneralNodePool())
		stopTheLab()
		steps.WaitFor().AllNodesReady()
	}, anObserverDiscardingEachBoard(), anInjector(), func(reported Outcome) { atTheHold = reported }, ignoringEachLineFollowed)
	if err != nil {
		t.Fatalf("carry out the Scenario %s until the lab stopped: %v", theScenarioFiledWhenStoppedPartWay, err)
	}

	expect.Expect(t, stopped.Ended()).To(expect.Equal(theStopEndsTheSteps))
	expect.Expect(t, atTheHold.FiledAt).To(expect.Equal(noTimelineReportedAtTheHold))
	expect.Expect(t, theTimelineFiledAt(t, stopped, stopped.Steps[0].String())).
		To(expect.Equal(filedAtTheStop{Entries: len(stopped.Timeline.Entries()), HoldsTheStep: theStepFiled}))
}

// A file where the Timeline folder should be leaves no folder to file it in.
func aTimelineFolderTheLabCannotCreate(t *testing.T) timeline.Folder {
	t.Helper()

	root := filepath.Join(t.TempDir(), aFileInTheFoldersPlace)
	if err := os.WriteFile(root, nil, anyFileMode); err != nil {
		t.Fatalf("put a file where the Timeline folder goes: %v", err)
	}

	return timeline.NewFolder(root)
}

func TestAnInteractiveRunWhoseTimelineCannotBeWrittenAtTheStop_ShouldNameIt(t *testing.T) {
	t.Parallel()
	folder := aTimelineFolderTheLabCannotCreate(t)
	running := aRunnerFiledIn(t, folder)
	holding, stopTheLab := context.WithCancel(t.Context())
	var atTheHold Outcome

	stopped, err := running.holdNamed(holding, theScenarioWhoseTimelineIsNotFiled, noDeschedulerImage,
		func(steps *Scenario) {
			steps.Given().Zones(twoZones, theGeneralNodePool())
			steps.WaitFor().AllNodesReady()
		}, anObserverDiscardingEachBoard(), anInjector(), func(reported Outcome) {
			atTheHold = reported
			stopTheLab()
		}, ignoringEachLineFollowed)
	if err != nil {
		t.Fatalf("carry out the Scenario %s: %v", theScenarioWhoseTimelineIsNotFiled, err)
	}

	expect.Expect(t, atTheHold.NotFiled).To(expect.Equal[error](nil))
	if stopped.NotFiled == nil {
		t.Fatalf("the Timeline could not be written at the stop, and the Outcome named nothing")
	}
	if !strings.Contains(stopped.NotFiled.Error(), folder.Path()) {
		t.Errorf("the Timeline could not be written, and the Outcome named %q instead of the folder %s", stopped.NotFiled, folder.Path())
	}
}

func TestAnInteractiveRunWhoseStepCannotBeCarriedOut_ShouldHoldTheClusterAsThatStepLeftIt(t *testing.T) {
	t.Parallel()
	running := aRunnerOnThisMachine(t)
	holding, stopTheLab := context.WithCancel(t.Context())
	var provisioned *cluster.SimulatedCluster
	var readyWhileHeld []scenario.Node

	held, err := running.holdNamed(holding, theScenarioEndedAtARefusal, noDeschedulerImage, func(steps *Scenario) {
		provisioned = steps.provisioned
		steps.Given().Zones(twoZones, theGeneralNodePool())
		steps.WaitFor().AllNodesReady()
		steps.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), aSpecificationTheConstructorRefuses())
	}, anObserverDiscardingEachBoard(), anInjector(), func(Outcome) {
		readyWhileHeld = theReadyNodesOf(t, provisioned)
		stopTheLab()
	}, ignoringEachLineFollowed)
	if err != nil {
		t.Fatalf("carry out the Scenario %s: %v", theScenarioEndedAtARefusal, err)
	}

	expect.Expect(t, held.EndedAt.Kind).To(expect.Equal(GivenStep))
	expect.Expect(t, readyWhileHeld).To(expect.HaveLen[scenario.Node](theNodesOfTwoZonesOfTwoEach))
}

func TestAHeldRunUnderTestEndingBeforeItReports_ShouldFailTheTestNamingTheRunsErrorAndBeSafeToStopTwice(t *testing.T) {
	t.Parallel()
	running := aRunnerOnThisMachine(t)
	holding, stopHolding := context.WithCancel(t.Context())
	stopHolding()

	held, err := startAHeldRun(holding, stopHolding, running, theScenarioEndedBeforeItReports, func(*Scenario) {}, ignoringEachLineFollowed)
	_, firstStop := held.stopTheLab()
	_, secondStop := held.stopTheLab()

	expect.Expect(t, errors.Is(err, context.Canceled)).To(expect.Equal(true))
	expect.Expect(t, strings.Contains(fmt.Sprint(err), theScenarioEndedBeforeItReports)).To(expect.Equal(true))
	expect.Expect(t, errors.Is(firstStop, context.Canceled)).To(expect.Equal(true))
	expect.Expect(t, errors.Is(secondStop, context.Canceled)).To(expect.Equal(true))
}

// heldForClicks is a held run the Live view's clicks reach through its
// Injector, with what its Steps declared.
type heldForClicks struct {
	injector    *Injector
	provisioned *cluster.SimulatedCluster
	topology    scenario.Topology
	deployment  scenario.Deployment
	// descheduling runs the passes of a Descheduler declared on an interval.
	descheduling *deschedulerInterval
	boards       <-chan scenario.PlacementBoard
	// followed is written by the held run's reader of its Timeline, which may
	// still be handing a line to its terminal once the held run has ended.
	followedLock sync.Mutex
	followed     []string
	terminal     func(string)
	// reportedAtTheHold is written before the held run's report returns, which
	// is before startAHeldRun does.
	reportedAtTheHold Outcome
	// stopTheLab may be called by the test and again by its cleanup, so it
	// stops the held run once and hands back its Outcome each time.
	stopTheLab func() (Outcome, error)
}

// aHeldRunTakingClicks returns once the held run's Outcome is reported, so a
// click from the test is one the held run takes. The cluster is removed when
// the test ends.
func aHeldRunTakingClicks(t *testing.T, name string) *heldForClicks {
	t.Helper()

	return aHeldRunDeclaring(t, name, func(*Scenario) {})
}

// aHeldRunDeschedulingOnAnInterval runs a pass every second under a policy
// restoring the hostname spread, from its Pods' Wait until the lab stops.
func aHeldRunDeschedulingOnAnInterval(t *testing.T, name string) *heldForClicks {
	t.Helper()

	return aHeldRunDeclaring(t, name, func(steps *Scenario) {
		steps.Given().Descheduler(aPolicyRestoringTheSpread()).EverySeconds(aPassEverySecond)
	})
}

// aHeldRunDeclaring declares beside the Deployment whatever further Givens the
// test needs, before the Wait applying them.
func aHeldRunDeclaring(t *testing.T, name string, furtherGivens func(*Scenario)) *heldForClicks {
	t.Helper()

	return aHeldRunCarryingOut(t, name, aDeploymentOverTwoZonesDeclaring(furtherGivens))
}

// aHeldRunFollowedOn is a held run taking clicks whose lines are handed to the
// terminal given.
func aHeldRunFollowedOn(t *testing.T, name string, terminal func(string)) *heldForClicks {
	t.Helper()

	return aHeldRunPrintingTo(t, name, aDeploymentOverTwoZonesDeclaring(func(*Scenario) {}), terminal)
}

func aDeploymentOverTwoZonesDeclaring(furtherGivens func(*Scenario)) func(*Scenario) {
	return func(steps *Scenario) {
		steps.Given().Zones(twoZones, theGeneralNodePool())
		steps.WaitFor().AllNodesReady()
		steps.Given().Deployment(someDeploymentName, podsOverEveryNode, askingForNothing(), resilientPlacement())
		furtherGivens(steps)
		steps.WaitFor().AllPodsReady()
	}
}

// aHeldRunCarryingOut holds the cluster once the Steps given are carried out,
// keeping what they declared.
func aHeldRunCarryingOut(t *testing.T, name string, steps func(*Scenario)) *heldForClicks {
	t.Helper()

	return aHeldRunPrintingTo(t, name, steps, ignoringEachLineFollowed)
}

func aHeldRunPrintingTo(t *testing.T, name string, steps func(*Scenario), terminal func(string)) *heldForClicks {
	t.Helper()

	holding, stopHolding := context.WithCancel(t.Context())
	held, err := startAHeldRun(holding, stopHolding, aRunnerOnThisMachine(t), name, steps, terminal)
	t.Cleanup(func() {
		// The early end is reported once, by the Fatal below.
		if _, stopErr := held.stopTheLab(); stopErr != nil && err == nil {
			t.Errorf("carry out the held Scenario %s: %v", name, stopErr)
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	return held
}

// startAHeldRun returns once the held run reports its Outcome, or with the
// error the run ended with when it ends first. The heldForClicks it returns
// is never nil, so the caller can stop the lab either way.
func startAHeldRun(holding context.Context, stopHolding context.CancelFunc, running *Runner, name string,
	steps func(*Scenario), terminal func(string),
) (*heldForClicks, error) {
	held := &heldForClicks{injector: NewInjector(), terminal: terminal, stopTheLab: func() (Outcome, error) { return Outcome{}, nil }}
	image, err := running.obtainTheDeschedulerOf(holding, []func(*Scenario){steps})
	if err != nil {
		return held, fmt.Errorf("obtain the Descheduler image of the held Scenario %s: %w", name, err)
	}
	observer, boards := anObserverHandingOverEachPlacementBoard(holding)
	held.boards = boards
	reported, ended := make(chan struct{}), make(chan struct{})
	var stopped Carried
	go func() {
		defer close(ended)
		outcome, err := running.holdNamed(holding, name, image, func(declaring *Scenario) {
			held.provisioned = declaring.provisioned
			steps(declaring)
			held.topology, held.deployment, held.descheduling = declaring.topology, declaring.deployment, declaring.descheduling
		}, observer, held.injector, func(atTheHold Outcome) {
			held.reportedAtTheHold = atTheHold
			close(reported)
		}, held.follow)
		stopped = Carried{Outcome: outcome, Err: err}
	}()
	held.stopTheLab = func() (Outcome, error) {
		stopHolding()
		<-ended

		return stopped.Outcome, stopped.Err
	}

	select {
	case <-reported:
		return held, nil
	case <-ended:
		return held, fmt.Errorf("the held Scenario %s ended before it reported: %w", name, stopped.Err)
	}
}

// theOutcomeOnceStopped is what the held run hands back once the engineer stops
// the lab.
func (h *heldForClicks) theOutcomeOnceStopped(t *testing.T) Outcome {
	t.Helper()

	stopped, err := h.stopTheLab()
	if err != nil {
		t.Fatalf("stop the held Scenario: %v", err)
	}

	return stopped
}

// follow keeps the line before the terminal takes it, so a line a stalled
// terminal holds is kept all the same.
func (h *heldForClicks) follow(line string) {
	h.followedLock.Lock()
	h.followed = append(h.followed, line)
	h.followedLock.Unlock()

	h.terminal(line)
}

// theLinesFollowed stops the held run, so its reader has handed over every line
// it was handed before the stop.
func (h *heldForClicks) theLinesFollowed(t *testing.T) []string {
	t.Helper()

	h.theOutcomeOnceStopped(t)
	h.followedLock.Lock()
	defer h.followedLock.Unlock()

	return slices.Clone(h.followed)
}

// aStalledTerminal takes its first line and returns from it only once the test
// has ended, as a terminal stopped with Ctrl-S or an unread pager does.
type aStalledTerminal struct {
	reached  chan struct{}
	first    sync.Once
	released chan struct{}
	returned atomic.Bool
}

func aTerminalThatStopsTakingLines(t *testing.T) *aStalledTerminal {
	t.Helper()

	stalled := &aStalledTerminal{reached: make(chan struct{}), released: make(chan struct{})}
	t.Cleanup(func() { close(stalled.released) })

	return stalled
}

func (s *aStalledTerminal) take(string) {
	s.first.Do(func() { close(s.reached) })
	<-s.released
	s.returned.Store(true)
}

func (s *aStalledTerminal) awaitItsFirstLine(t *testing.T) {
	t.Helper()

	select {
	case <-s.reached:
	case <-time.After(scenario.WaitingTime):
		t.Fatalf("the held run handed its terminal no line within %s", scenario.WaitingTime)
	}
}

// theWhensFollowed are the lines a click's When gives, the only ones naming a
// Step.
func (h *heldForClicks) theWhensFollowed(t *testing.T) []string {
	t.Helper()

	whens := []string{}
	for _, line := range h.theLinesFollowed(t) {
		if strings.HasPrefix(line, string(WhenStep)+" ") {
			whens = append(whens, line)
		}
	}

	return whens
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

func (h *heldForClicks) awaitEveryNodeReady(t *testing.T) {
	t.Helper()

	if _, err := h.provisioned.AwaitNodesReady(t.Context(), h.topology, h.deployment); err != nil {
		t.Fatalf("await every Node of the held Scenario Ready: %v", err)
	}
}

// awaitAPassEvictingNothing ends at the first pass begun after the call that
// evicts nothing, so a pass the cluster gave cause to evict has run before it.
func (h *heldForClicks) awaitAPassEvictingNothing(t *testing.T) {
	t.Helper()

	waiting, cancel := context.WithTimeout(t.Context(), scenario.WaitingTime)
	defer cancel()
	if err := h.descheduling.awaitAPassEvictingNothingAfter(waiting, h.descheduling.passesBegun()); err != nil {
		t.Fatalf("await a Descheduler pass of the held Scenario evicting nothing: %v", err)
	}
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
	noLineFollowed                     = 0
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

	expect.Expect(t, held.theWhensFollowed(t)).To(expect.Equal([]string{
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
	expect.Expect(t, held.theWhensFollowed(t)).To(expect.HaveLen[string](noWhenReported))
}

func TestAClickInTheLiveViewWhileTheStepsStillRun_ShouldChangeNothing(t *testing.T) {
	t.Parallel()
	running := aRunnerOnThisMachine(t)
	holding, stopTheLab := context.WithCancel(t.Context())
	var followed []string
	injector := NewInjector()
	var provisioned *cluster.SimulatedCluster
	var declared scenario.Topology
	var readyAfterTheClick []scenario.Node

	stopped, err := running.holdNamed(holding, theScenarioClickedWhileItsStepsRun, noDeschedulerImage, func(steps *Scenario) {
		provisioned = steps.provisioned
		steps.Given().Zones(twoZones, theGeneralNodePool())
		steps.WaitFor().AllNodesReady()
		declared = steps.topology
		if err := injector.ClickNode(declared.Nodes()[0].Name); err != nil {
			t.Errorf("click a Node while the Steps run: %v", err)
		}
		readyAfterTheClick = theReadyNodesOf(t, provisioned)
	}, anObserverDiscardingEachBoard(), injector, func(Outcome) { stopTheLab() }, func(line string) { followed = append(followed, line) })
	if err != nil {
		t.Fatalf("carry out the Scenario %s: %v", theScenarioClickedWhileItsStepsRun, err)
	}

	expect.Expect(t, readyAfterTheClick).To(expect.HaveLen[scenario.Node](theNodesOfTwoZonesOfTwoEach))
	expect.Expect(t, followed).To(expect.HaveLen[string](noLineFollowed))
	expect.Expect(t, theWhensNotedIn(stopped.Timeline)).To(expect.HaveLen[string](noWhenReported))
}

func TestAClickOnAZoneWithOnlySomeNodesStopped_ShouldStopTheOthersAndNameTheZone(t *testing.T) {
	t.Parallel()
	held := aHeldRunTakingClicks(t, theScenarioClickedOnAPartlyStopped)
	zone := held.theFirstZone()
	stoppedFirst := held.theNodesOfZone(zone)[0]
	expectInjected(t, held.injector.ClickNode(stoppedFirst))

	expectInjected(t, held.injector.ClickZone(zone))

	expect.Expect(t, held.theReadyNodeNames(t)).To(expect.Equal(held.everyNodeBut(held.theNodesOfZone(zone)...)))
	expect.Expect(t, held.theWhensFollowed(t)).To(expect.Equal([]string{
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
	expect.Expect(t, held.theWhensFollowed(t)).To(expect.Equal([]string{
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
	expect.Expect(t, held.theWhensFollowed(t)).To(expect.Equal([]string{
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
	expect.Expect(t, held.theWhensFollowed(t)).To(expect.Equal([]string{
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
	expect.Expect(t, held.theWhensFollowed(t)).To(expect.Equal([]string{
		"When Node " + clicked + " stops responding",
		"When Node " + clicked + " responds again",
		"When Node " + clicked + " stops responding",
	}))
}

const (
	theScenarioJoiningAClickToItsTimeline  = "a-scenario-joining-a-click-to-its-timeline"
	theScenarioJoiningNoClickToItsTimeline = "a-scenario-joining-no-click-to-its-timeline"
	theScenarioRecordingWhatAClickChanged  = "a-scenario-recording-what-a-click-changed"
	theScenarioFilingItsTimelineAtTheStop  = "a-scenario-filing-its-timeline-at-the-stop"
	thePodScheduled                        = "Scheduled"
	theScenarioDeschedulingWhileHeld       = "a-scenario-descheduling-while-held"
	thePassEvicted                         = true
	theStepFiled                           = true
)

// The Steps carried out hold no When, so every When the Timeline holds is a
// click's.
func theWhensNotedIn(recorded scenario.Timeline) []string {
	noted := []string{}
	for _, entry := range recorded.Entries() {
		if entry.Kind == scenario.StepObject && entry.Name == string(WhenStep) {
			noted = append(noted, entry.Detail)
		}
	}

	return noted
}

// theReadinessRecordedAfter reads the Node's readiness from the entries after
// the When's note alone, so a readiness recorded there followed the note.
func theReadinessRecordedAfter(recorded scenario.Timeline, when, node string) []string {
	readiness := []string{}
	noted := false
	for _, entry := range recorded.Entries() {
		if entry.Kind == scenario.StepObject && entry.Detail == when {
			noted = true
		}
		if noted && entry.Kind == scenario.NodeObject && entry.Name == node {
			readiness = append(readiness, entry.Reason)
		}
	}

	return readiness
}

// The API server reports the clicked Node NotReady once the click returns, but
// the record receives that change on its watch stream later. The Wait a
// Scenario writes after its When, here for the Pods the Node held, lets the
// record receive it before the lab stops, as it does for a Scenario's Then.
func TestAClickOnAHeldCluster_ShouldJoinTheTimelineAsAWhenAtItsMomentWordedAsTheTerminalsWhen(t *testing.T) {
	t.Parallel()
	held := aHeldRunTakingClicks(t, theScenarioJoiningAClickToItsTimeline)
	clicked := held.theFirstNode()

	expectInjected(t, held.injector.ClickNode(clicked))
	held.thePodNamesOnceReplaced(t)

	recorded := held.theOutcomeOnceStopped(t).Timeline
	expect.Expect(t, theWhensNotedIn(recorded)).To(expect.Equal(held.theWhensFollowed(t)))
	expect.Expect(t, slices.Contains(theReadinessRecordedAfter(recorded, held.theWhensFollowed(t)[0], clicked), string(scenario.NodeNotReady))).
		To(expect.Equal(true))
}

// A Pending Pod's box carries no Failure, so a name resolving to nothing stands
// for every click that changes nothing.
func TestAClickChangingNothing_ShouldAddNoWhenToTheTimeline(t *testing.T) {
	t.Parallel()
	held := aHeldRunTakingClicks(t, theScenarioJoiningNoClickToItsTimeline)

	expectInjected(t, held.injector.ClickNode(aNameNoNodeZoneOrPodCarries))
	expectInjected(t, held.injector.ClickZone(aNameNoNodeZoneOrPodCarries))
	expectInjected(t, held.injector.LosePod(aNameNoNodeZoneOrPodCarries))

	expect.Expect(t, theWhensNotedIn(held.theOutcomeOnceStopped(t).Timeline)).To(expect.HaveLen[string](noWhenReported))
}

// theScheduledAfter names each Pod the control plane reported Scheduled after
// the When's note.
func theScheduledAfter(recorded scenario.Timeline, when string) []string {
	scheduled := []string{}
	noted := false
	for _, entry := range recorded.Entries() {
		if entry.Kind == scenario.StepObject && entry.Detail == when {
			noted = true
		}
		if noted && entry.Kind == scenario.PodObject && entry.Change == scenario.ChangeReported && entry.Reason == thePodScheduled {
			scheduled = append(scheduled, entry.Name)
		}
	}

	return scheduled
}

func theNewcomers(before, after []string) []string {
	newcomers := []string{}
	for _, name := range after {
		if !slices.Contains(before, name) {
			newcomers = append(newcomers, name)
		}
	}

	return newcomers
}

// The lost Pod's replacement is scheduled by the control plane alone, so its
// reported reason is the control plane's.
func TestChangesAfterAClickOnAHeldCluster_ShouldBeRecordedInTheTimelineWithTheControlPlanesReason(t *testing.T) {
	t.Parallel()
	held := aHeldRunTakingClicks(t, theScenarioRecordingWhatAClickChanged)
	clicked := held.aPodPlacedOn(t, held.theFirstNode())
	placedBefore := held.thePodNamesOnceReplaced(t)

	expectInjected(t, held.injector.LosePod(clicked.Name))

	replacements := theNewcomers(placedBefore, held.thePodNamesOnceReplaced(t))
	expect.Expect(t, theScheduledAfter(held.theOutcomeOnceStopped(t).Timeline, held.theWhensFollowed(t)[0])).To(expect.Equal(replacements))
}

// filedAtTheStop is what the file a held run filed holds: its entry count, and
// whether a Step's line is among them.
type filedAtTheStop struct {
	Entries      int
	HoldsTheStep bool
}

// theTimelineFiledAt reads the file back, its first line being the header.
func theTimelineFiledAt(t *testing.T, stopped Outcome, step string) filedAtTheStop {
	t.Helper()

	filed, err := os.ReadFile(stopped.FiledAt)
	if err != nil {
		t.Fatalf("read the Timeline filed at %s: %v", stopped.FiledAt, err)
	}
	lines := strings.Split(strings.TrimSuffix(string(filed), "\n"), "\n")

	return filedAtTheStop{Entries: len(lines[1:]), HoldsTheStep: strings.Contains(string(filed), step)}
}

func TestAnInteractiveRunStopped_ShouldFileItsTimelineWithEveryEntryUpToTheStop(t *testing.T) {
	t.Parallel()
	held := aHeldRunTakingClicks(t, theScenarioFilingItsTimelineAtTheStop)
	expectInjected(t, held.injector.ClickNode(held.theFirstNode()))

	stopped := held.theOutcomeOnceStopped(t)

	expect.Expect(t, stopped.NotFiled).To(expect.Equal[error](nil))
	expect.Expect(t, theTimelineFiledAt(t, stopped, held.theWhensFollowed(t)[0])).
		To(expect.Equal(filedAtTheStop{Entries: len(stopped.Timeline.Entries()), HoldsTheStep: theStepFiled}))
}

const (
	theScenarioOnAStalledTerminal      = "a-scenario-on-a-stalled-terminal"
	theScenarioClickedOnAStalledOne    = "a-scenario-clicked-on-a-stalled-terminal"
	theTerminalStillTakingItsFirstLine = false
	theTimelineFiled                   = true
	thePendingClickReturned            = true
)

// stoppedOnAStalledTerminal is what a held run stopped on a stalled terminal
// leaves: whether the terminal had returned from its line by then, and the
// Timeline it filed.
type stoppedOnAStalledTerminal struct {
	TerminalReturned bool
	Filed            bool
	FiledAtTheStop   filedAtTheStop
}

// The click's When is the terminal's first line, since a cluster settled before
// the hold has nothing named before a click. The Outcome once stopped carries
// no error, so the cluster's removal succeeded.
func TestAHeldClusterWhoseTerminalStopsTakingLines_ShouldFileItsTimelineAndBeRemovedWhenTheLabStops(t *testing.T) {
	t.Parallel()
	terminal := aTerminalThatStopsTakingLines(t)
	held := aHeldRunFollowedOn(t, theScenarioOnAStalledTerminal, terminal.take)
	clicked := held.theFirstNode()
	expectInjected(t, held.injector.ClickNode(clicked))
	terminal.awaitItsFirstLine(t)

	stopped := held.theOutcomeOnceStopped(t)

	expect.Expect(t, stoppedOnAStalledTerminal{
		TerminalReturned: terminal.returned.Load(),
		Filed:            stopped.NotFiled == nil,
		FiledAtTheStop:   theTimelineFiledAt(t, stopped, "When Node "+clicked+" stops responding"),
	}).To(expect.Equal(stoppedOnAStalledTerminal{
		TerminalReturned: theTerminalStillTakingItsFirstLine,
		Filed:            theTimelineFiled,
		FiledAtTheStop:   filedAtTheStop{Entries: len(stopped.Timeline.Entries()), HoldsTheStep: theStepFiled},
	}))
	expectRemoved(t, held.provisioned)
}

// The second click's When cannot be handed on while the terminal holds the
// first, so the click waits in the Timeline as the lab stops. The stop waits
// for a click under way, so a click that returned did so before Hold did. Its
// error is the stopped run's own and is not asserted.
func TestAClickPendingOnAStalledTerminalWhenTheLabStops_ShouldReturnAndLetTheLabStop(t *testing.T) {
	t.Parallel()
	terminal := aTerminalThatStopsTakingLines(t)
	held := aHeldRunFollowedOn(t, theScenarioClickedOnAStalledOne, terminal.take)
	clicked := held.theFirstNode()
	expectInjected(t, held.injector.ClickNode(clicked))
	terminal.awaitItsFirstLine(t)
	pending := make(chan struct{})
	go func() {
		defer close(pending)
		_ = held.injector.ClickNode(clicked)
	}()

	held.theOutcomeOnceStopped(t)

	expect.Expect(t, isClosed(pending)).To(expect.Equal(thePendingClickReturned))
}

func isClosed(signal <-chan struct{}) bool {
	select {
	case <-signal:
		return true
	default:
		return false
	}
}

// thePodsDescheduledAfter names each Pod the Timeline reports evicted by the
// Descheduler after the When's note.
func thePodsDescheduledAfter(recorded scenario.Timeline, when string) []string {
	descheduled := []string{}
	noted := false
	for _, entry := range recorded.Entries() {
		if entry.Kind == scenario.StepObject && entry.Detail == when {
			noted = true
		}
		if noted && entry.Reason == scenario.EvictedByTheDescheduler {
			descheduled = append(descheduled, entry.Name)
		}
	}

	return descheduled
}

// evictionsWhileHeld is what a held Timeline says of a Node Failure's Pod and of
// the Pods a pass evicted once the Node responded again.
type evictionsWhileHeld struct {
	DescheduledOnceRespondingAgain bool
	FailuresPodDescheduled         bool
}

// A Node that stops responding and responds again leaves it empty and another
// Node holding two, past the hostname spread, so a pass on the interval evicts
// a Pod the Failure did not lose. The Descheduler evicts from Ready Nodes
// alone, so the Pod the Failure lost is never its.
func TestAPassEvictingPodsOnAHeldCluster_ShouldLeaveEntriesSetApartFromAFailuresEvictions(t *testing.T) {
	t.Parallel()
	held := aHeldRunDeschedulingOnAnInterval(t, theScenarioDeschedulingWhileHeld)
	stopped := held.theFirstNode()
	lostToTheFailure := held.aPodPlacedOn(t, stopped)
	expectInjected(t, held.injector.ClickNode(stopped))
	held.thePodNamesOnceReplaced(t)

	expectInjected(t, held.injector.ClickNode(stopped))
	held.awaitEveryNodeReady(t)
	held.awaitAPassEvictingNothing(t)

	recorded := held.theOutcomeOnceStopped(t).Timeline
	expect.Expect(t, evictionsWhileHeld{
		DescheduledOnceRespondingAgain: len(thePodsDescheduledAfter(recorded, held.theWhensFollowed(t)[1])) > 0,
		FailuresPodDescheduled:         slices.Contains(theEvictionsByTheDeschedulerOn(recorded), lostToTheFailure.Name),
	}).To(expect.Equal(evictionsWhileHeld{DescheduledOnceRespondingAgain: thePassEvicted}))
}

const (
	theScenarioFollowingItsMoments    = "a-scenario-following-its-moments"
	theScenarioFollowedWithNoInterval = "a-scenario-followed-with-no-descheduler-interval"
	theDeschedulersEviction           = " by the Descheduler"
	aMomentOpensAfter                 = "  "
	theMomentsFollowedHaveEntries     = true
	theNodeNamedNotReady              = true
	noEvictionByTheDescheduler        = 0
	noLineBeforeTheClick              = 0
)

// toTheMillisecond is the elapsed time a narrative's line and an entry's line
// open with.
var toTheMillisecond = regexp.MustCompile(`^\+[0-9]+\.[0-9]{3}s$`)

// theMomentsFollowed are the lines no click's When gives.
func (h *heldForClicks) theMomentsFollowed(t *testing.T) []string {
	t.Helper()

	moments := []string{}
	for _, line := range h.theLinesFollowed(t) {
		if !strings.HasPrefix(line, string(WhenStep)+" ") {
			moments = append(moments, line)
		}
	}

	return moments
}

// theMomentsWithNoEntry names each moment whose opening time is not to the
// millisecond, or opens no entry of the Timeline filed.
func theMomentsWithNoEntry(moments []string, filed scenario.Timeline) []string {
	entryAt := map[string]bool{}
	for _, entry := range filed.Entries() {
		at, _, _ := strings.Cut(entry.Line(), aMomentOpensAfter)
		entryAt[at] = true
	}
	unfiled := []string{}
	for _, moment := range moments {
		at, _, _ := strings.Cut(moment, aMomentOpensAfter)
		if !toTheMillisecond.MatchString(at) || !entryAt[at] {
			unfiled = append(unfiled, moment)
		}
	}

	return unfiled
}

// momentsFiled is whether any moment was followed, and those the filed
// Timeline holds no entry for.
type momentsFiled struct {
	Followed    bool
	WithNoEntry []string
}

func TestAMomentLineFollowed_ShouldOpenWithItsElapsedTimeToTheMillisecondAndHaveAnEntryInTheFiledTimeline(t *testing.T) {
	t.Parallel()
	held := aHeldRunTakingClicks(t, theScenarioFollowingItsMoments)
	expectInjected(t, held.injector.ClickNode(held.theFirstNode()))
	held.thePodNamesOnceReplaced(t)

	moments := held.theMomentsFollowed(t)

	expect.Expect(t, momentsFiled{
		Followed: len(moments) > 0, WithNoEntry: theMomentsWithNoEntry(moments, held.theOutcomeOnceStopped(t).Timeline),
	}).To(expect.Equal(momentsFiled{Followed: theMomentsFollowedHaveEntries, WithNoEntry: []string{}}))
}

func theLinesSaying(lines []string, said func(string) bool) []string {
	saying := []string{}
	for _, line := range lines {
		if said(line) {
			saying = append(saying, line)
		}
	}

	return saying
}

// clicksAndMoments is what the terminal of a held cluster named: the lines
// before the first click, the clicks, whether the Node clicked was named
// NotReady, and the Descheduler's evictions.
type clicksAndMoments struct {
	BeforeTheClick   int
	Whens            []string
	NodeNotReady     bool
	ByTheDescheduler int
}

// The cluster settled before the hold, so nothing is named before the click.
func TestAHeldClusterWithNoDeschedulerInterval_ShouldNameClicksAndTheMomentsAfterThemOnly(t *testing.T) {
	t.Parallel()
	held := aHeldRunTakingClicks(t, theScenarioFollowedWithNoInterval)
	clicked := held.theFirstNode()
	expectInjected(t, held.injector.ClickNode(clicked))
	held.thePodNamesOnceReplaced(t)

	lines := held.theLinesFollowed(t)

	theClick := "When Node " + clicked + " stops responding"
	expect.Expect(t, clicksAndMoments{
		BeforeTheClick: slices.Index(lines, theClick),
		Whens:          held.theWhensFollowed(t),
		NodeNotReady: len(theLinesSaying(lines, func(line string) bool {
			return strings.HasSuffix(line, aMomentOpensAfter+"Node "+clicked+" "+string(scenario.NodeNotReady))
		})) > 0,
		ByTheDescheduler: len(theLinesSaying(lines, func(line string) bool { return strings.HasSuffix(line, theDeschedulersEviction) })),
	}).To(expect.Equal(clicksAndMoments{
		BeforeTheClick: noLineBeforeTheClick, Whens: []string{theClick}, NodeNotReady: theNodeNamedNotReady,
		ByTheDescheduler: noEvictionByTheDescheduler,
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

	ran, err := aRunOnThisMachineOf(t, name, func(steps *Scenario) {
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
	ran, err := aRunOnThisMachineOf(t, theScenarioLosingPods, func(steps *Scenario) {
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
	ran, err := aRunOnThisMachineOf(t, theScenarioLosingPodsBeforeANodeFails, func(steps *Scenario) {
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
	ran, err := aRunOnThisMachineOf(t, theScenarioRunningADeschedulerPass, func(steps *Scenario) {
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
	ran, err := aRunOnThisMachineOf(t, theScenarioWhosePassCannotRun, func(steps *Scenario) {
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
	ran, err := aRunOnThisMachineOf(t, theScenarioWhosePassEvictsNothing, func(steps *Scenario) {
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
	ran, err := aRunOnThisMachineOf(t, theScenarioRunningPassesOnAnInterval, func(steps *Scenario) {
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
	ran, err := aRunOnThisMachineOf(t, theScenarioSettlingBeforeItsFirstPass, func(steps *Scenario) {
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
	ran, err := aRunOnThisMachineOf(t, theScenarioWhoseIntervalCannotRun, func(steps *Scenario) {
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

			ran, err := aRunOf(t, NewRunner(engine, cluster.DefaultKubernetesVersion(), timeline.NewFolder(t.TempDir())),
				by.name, func(steps *Scenario) {
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
	theScenarioWhoseIntervalStopsWhileHeld    = "a-scenario-whose-descheduler-interval-stops-while-held"
	theScenarioClickedOnceItsIntervalStopped  = "a-scenario-clicked-once-its-descheduler-interval-stopped"
	theScenarioEndedByItsIntervalWhenStopped  = "a-scenario-ended-by-its-descheduler-interval-when-stopped"
	theScenarioWhoseIntervalRunsWhileHeld     = "a-scenario-whose-descheduler-interval-runs-while-held"
	theFailedPassAlone                        = 1
	noStopNamed                               = 0
	theHeldScenarioHeldAtTheStop              = true
	theCauseNamesWhyTheIntervalStopped        = true
	theTimelineFiledOnceTheIntervalStopped    = true
	theGivenNamedBeforeWhyTheIntervalStopped  = true
	whyTheIntervalStoppedNamedOnTheTerminal   = true
	noCauseReportedAtTheHold                  = ""
	theTimeForTheFirstPassOnceHeldToFail      = anIntervalLongerThanTheNodesWait*time.Second + scenario.WaitingTime
	anIntervalLongerThanTheNodesWait          = 15
	aNodesWaitShorterThanTheIntervalInSeconds = 10
)

// The Nodes' Wait applies the interval and is held to less than it, so the hold
// begins before the first pass, which the policy has fail. A Wait that overran
// would end the Scenario at it, and the Outcome reported at the hold would name
// why.
func aHeldRunWhoseIntervalStopsOnceHeld(t *testing.T, name string, terminal func(string)) *heldForClicks {
	t.Helper()

	return aHeldRunPrintingTo(t, name, func(steps *Scenario) {
		steps.Given().Zones(twoZones, theGeneralNodePool())
		steps.WaitFor().AllNodesReady()
		steps.Given().Descheduler(aPolicyThePassRefuses()).EverySeconds(anIntervalLongerThanTheNodesWait)
		steps.WaitFor().AllNodesReadyWithinSeconds(aNodesWaitShorterThanTheIntervalInSeconds)
	}, terminal)
}

func theGivenDeclaringTheFailingInterval(t *testing.T) Step {
	t.Helper()

	return deschedulerOnAnIntervalStepOf(aPolicyNameThePassRefuses, theDeschedulerPairedByDefault(t), anIntervalLongerThanTheNodesWait)
}

func namingTheStopOf(name string) func(string) bool {
	return func(line string) bool { return strings.HasPrefix(line, theStopNamedFor(name)) }
}

// aTerminalAwaitingAStop hands the test the first line naming the held Scenario
// ended, as the terminal takes it, so the test reads it while the lab still
// holds the cluster.
type aTerminalAwaitingAStop struct {
	namesTheStop func(string) bool
	first        sync.Once
	named        chan string
}

func aTerminalAwaitingAStopOf(name string) *aTerminalAwaitingAStop {
	// One slot holds the one line handed over, so the terminal never waits on
	// the test.
	return &aTerminalAwaitingAStop{namesTheStop: namingTheStopOf(name), named: make(chan string, 1)}
}

func (a *aTerminalAwaitingAStop) take(line string) {
	if a.namesTheStop(line) {
		a.first.Do(func() { a.named <- line })
	}
}

func (a *aTerminalAwaitingAStop) awaitTheStopNamed(t *testing.T) string {
	t.Helper()

	select {
	case line := <-a.named:
		return line
	case <-time.After(theTimeForTheFirstPassOnceHeldToFail):
		t.Fatalf("the held run named no stop of its Descheduler interval within %s", theTimeForTheFirstPassOnceHeldToFail)

		return ""
	}
}

// awaitTheIntervalStopping ends at the failure that stopped the held run's
// interval, which no pass evicting nothing comes before under a policy every
// pass refuses.
func (h *heldForClicks) awaitTheIntervalStopping(t *testing.T) {
	t.Helper()

	waiting, cancel := context.WithTimeout(t.Context(), theTimeForTheFirstPassOnceHeldToFail)
	defer cancel()
	if err := h.descheduling.awaitAPassEvictingNothingAfter(waiting, h.descheduling.passesBegun()); err == nil || waiting.Err() != nil {
		t.Fatalf("no pass on the held Scenario's Descheduler interval failed within %s: %v", theTimeForTheFirstPassOnceHeldToFail, err)
	}
}

// namedOnTheTerminal is what a held run's terminal named of its interval's stop,
// beside the cause the Outcome reported at the hold.
type namedOnTheTerminal struct {
	CauseAtTheHold string
	GivenNamed     bool
	WhyNamed       bool
}

func TestADeschedulerIntervalStoppingWhileTheClusterIsHeld_ShouldBeNamedOnTheTerminalWithWhyBeforeTheLabStops(t *testing.T) {
	t.Parallel()
	terminal := aTerminalAwaitingAStopOf(theScenarioWhoseIntervalStopsWhileHeld)
	held := aHeldRunWhoseIntervalStopsOnceHeld(t, theScenarioWhoseIntervalStopsWhileHeld, terminal.take)

	named := terminal.awaitTheStopNamed(t)

	expect.Expect(t, namedOnTheTerminal{
		CauseAtTheHold: causeOf(held.reportedAtTheHold),
		GivenNamed:     strings.HasPrefix(named, theStopNamedFor(theScenarioWhoseIntervalStopsWhileHeld)+theGivenDeclaringTheFailingInterval(t).String()+": "),
		WhyNamed:       strings.Contains(named, aPassThatExitedNonZero),
	}).To(expect.Equal(namedOnTheTerminal{
		CauseAtTheHold: noCauseReportedAtTheHold,
		GivenNamed:     theGivenNamedBeforeWhyTheIntervalStopped,
		WhyNamed:       whyTheIntervalStoppedNamedOnTheTerminal,
	}))
}

// clickedOnceStopped is what a click on a held cluster whose interval stopped
// left: the Nodes still Ready, and the passes the interval began.
type clickedOnceStopped struct {
	CauseAtTheHold string
	ReadyNodes     []string
	PassesBegun    int
}

func TestAHeldClusterWhoseDeschedulerIntervalStopped_ShouldStayHeldAndTakeClicks(t *testing.T) {
	t.Parallel()
	held := aHeldRunWhoseIntervalStopsOnceHeld(t, theScenarioClickedOnceItsIntervalStopped, ignoringEachLineFollowed)
	held.awaitTheIntervalStopping(t)
	clicked := held.theFirstNode()

	expectInjected(t, held.injector.ClickNode(clicked))

	expect.Expect(t, clickedOnceStopped{
		CauseAtTheHold: causeOf(held.reportedAtTheHold),
		ReadyNodes:     held.theReadyNodeNames(t),
		PassesBegun:    held.descheduling.passesBegun(),
	}).To(expect.Equal(clickedOnceStopped{
		CauseAtTheHold: noCauseReportedAtTheHold,
		ReadyNodes:     held.everyNodeBut(clicked),
		PassesBegun:    theFailedPassAlone,
	}))
}

// endedByItsInterval is what a held run whose interval stopped hands back once
// the lab stops.
type endedByItsInterval struct {
	CauseAtTheHold string
	EndedAt        string
	WhyNamed       bool
	Filed          bool
}

func TestAHeldRunWhoseDeschedulerIntervalStopped_ShouldEndAtTheDeclaringGivenNamingWhyWhenTheLabStops(t *testing.T) {
	t.Parallel()
	held := aHeldRunWhoseIntervalStopsOnceHeld(t, theScenarioEndedByItsIntervalWhenStopped, ignoringEachLineFollowed)
	held.awaitTheIntervalStopping(t)

	stopped := held.theOutcomeOnceStopped(t)

	expect.Expect(t, endedByItsInterval{
		CauseAtTheHold: causeOf(held.reportedAtTheHold),
		EndedAt:        stopped.EndedAt.String(),
		WhyNamed:       strings.Contains(causeOf(stopped), aPassThatExitedNonZero),
		Filed:          stopped.NotFiled == nil && stopped.FiledAt != "",
	}).To(expect.Equal(endedByItsInterval{
		CauseAtTheHold: noCauseReportedAtTheHold,
		EndedAt:        theGivenDeclaringTheFailingInterval(t).String(),
		WhyNamed:       theCauseNamesWhyTheIntervalStopped,
		Filed:          theTimelineFiledOnceTheIntervalStopped,
	}))
	expectRemoved(t, held.provisioned)
}

// heldOnAnIntervalWhenStopped is what a held run whose interval kept running
// hands back once the lab stops, and how many lines named a stop.
type heldOnAnIntervalWhenStopped struct {
	Held       bool
	StopsNamed int
}

func TestAHeldRunWhoseDeschedulerIntervalKeepsRunning_ShouldEndHeldWithNoStopNamed(t *testing.T) {
	t.Parallel()
	held := aHeldRunDeschedulingOnAnInterval(t, theScenarioWhoseIntervalRunsWhileHeld)
	held.awaitAPassEvictingNothing(t)

	stopped := held.theOutcomeOnceStopped(t)

	expect.Expect(t, heldOnAnIntervalWhenStopped{
		Held:       stopped.Held(),
		StopsNamed: len(theLinesSaying(held.theLinesFollowed(t), namingTheStopOf(theScenarioWhoseIntervalRunsWhileHeld))),
	}).To(expect.Equal(heldOnAnIntervalWhenStopped{Held: theHeldScenarioHeldAtTheStop, StopsNamed: noStopNamed}))
}

// Each is declared at the top level, because a closure's runtime name names no
// condition, and a run or held run derives the Scenario's name from it.
func ADeclaredPolicysPass_ShouldRunOnTheImageTheRunObtained(steps *Scenario) {
	steps.Given().Zones(twoZones, theGeneralNodePool())
	steps.WaitFor().AllNodesReady()
	steps.Given().Descheduler(aPolicyRestoringTheSpread())
	steps.When().DeschedulerRunsOnce()
	steps.WaitFor().AllNodesReady()
}

func AHeldScenarioDeclaringAPolicy_ShouldBeHeldPastItsDeschedulerGiven(steps *Scenario) {
	steps.Given().Zones(twoZones, theGeneralNodePool())
	steps.Given().Descheduler(aPolicyRestoringTheSpread())
	steps.WaitFor().AllNodesReady()
}

// A run entry that skipped the obtain would end the Scenario at its Descheduler
// Given, which is what makes this fail on a machine already holding the image.
func TestARunCarryingAScenarioDeclaringAPolicy_ShouldObtainTheDeschedulerImageBeforeAnyScenarioStarts(t *testing.T) {
	t.Parallel()

	carried, err := aRunnerOnThisMachine(t).RunAtOnce(t.Context(), []func(*Scenario){ADeclaredPolicysPass_ShouldRunOnTheImageTheRunObtained})

	if err != nil {
		t.Fatalf("carry out a run declaring a Descheduler policy: %v", err)
	}
	expect.Expect(t, theScenariosThatWentWrongIn(carried)).To(expect.Equal[[]string](nil))
	expect.Expect(t, causeOf(carried[0].Outcome)).To(expect.Equal(""))
}

// The lab is stopped once the Outcome is reported, so the held cluster goes at
// once.
func TestAHeldScenarioDeclaringAPolicy_ShouldObtainItsDeschedulerImageBeforeProvisioning(t *testing.T) {
	t.Parallel()
	holding, stopHolding := context.WithCancel(t.Context())
	defer stopHolding()
	var atTheHold Outcome

	_, err := aRunnerOnThisMachine(t).Hold(holding, AHeldScenarioDeclaringAPolicy_ShouldBeHeldPastItsDeschedulerGiven,
		anObserverDiscardingEachBoard(), anInjector(), func(reported Outcome) {
			atTheHold = reported
			stopHolding()
		}, ignoringEachLineFollowed)

	if err != nil {
		t.Fatalf("hold a Scenario declaring a Descheduler policy: %v", err)
	}
	expect.Expect(t, causeOf(atTheHold)).To(expect.Equal(noCauseReportedAtTheHold))
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
	ran, err := aRunOnThisMachineOf(t, theTwinOfAZoneThatRespondsAgain, func(steps *Scenario) {
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

const theHeldTwinOfAZoneThatRespondsAgain = "a-zone-that-responds-again-under-a-descheduler-held"

// theNodesNamedTurning names, in order, each Node a line names turning to the
// readiness.
func theNodesNamedTurning(lines []string, readiness scenario.NodeReadiness) []string {
	turned := []string{}
	for _, line := range lines {
		_, said, _ := strings.Cut(line, aMomentOpensAfter)
		if named, isANode := strings.CutPrefix(said, "Node "); isANode && strings.HasSuffix(named, " "+string(readiness)) {
			turned = append(turned, strings.TrimSuffix(named, " "+string(readiness)))
		}
	}
	slices.Sort(turned)

	return turned
}

func theLinesAfter(lines []string, when string) []string {
	return lines[slices.Index(lines, when)+1:]
}

// zoneFollowed is what the terminal named while the Zone stopped and responded
// again: the Nodes turning each way, whether the Descheduler moved a Pod, and
// whether as many Pods were scheduled after the Zone responded again as the
// Descheduler evicted.
type zoneFollowed struct {
	TurnedNotReady   []string
	TurnedReady      []string
	MovedBack        bool
	MovedAllReplaced bool
}

// The Steps are the Scenario's up to its first When, written out as its twin's
// are; the clicks stand for its two Whens.
func TestAZoneThatRespondsAgainUnderADeschedulerHeld_ShouldNameItsNodesTurningThePodsMovedAndEachReplacementsNode(t *testing.T) {
	const (
		threeZones        = 3
		threeNodesPerZone = 3
		ninePods          = threeZones * threeNodesPerZone
		thePodsMovedBack  = true
		everyMoveReplaced = true
	)
	t.Parallel()
	held := aHeldRunCarryingOut(t, theHeldTwinOfAZoneThatRespondsAgain, func(steps *Scenario) {
		steps.Given().Zones(threeZones, node.Pool(generalNodePool, node.PerZone(threeNodesPerZone), roomyNodes()))
		steps.WaitFor().AllNodesReady()
		steps.Given().Deployment(someDeploymentName, ninePods, askingForNothing(), resilientPlacement()).SurvivesLosing(Zones(oneLost))
		steps.Given().Descheduler(aPolicyLikeEvenSpreadRestored()).EverySeconds(aPassEveryFiveSeconds)
		steps.WaitFor().AllPodsReady()
	})
	zone := held.theFirstZone()
	expectInjected(t, held.injector.ClickZone(zone))
	held.thePodNamesOnceReplaced(t)
	expectInjected(t, held.injector.ClickZone(zone))
	held.awaitEveryNodeReady(t)
	held.awaitAPassEvictingNothing(t)
	held.thePodNamesOnceReplaced(t)

	onceResponding := theLinesAfter(held.theLinesFollowed(t), "When Zone "+zone+" responds again")

	movedBack := theLinesSaying(onceResponding, func(line string) bool { return strings.HasSuffix(line, theDeschedulersEviction) })
	scheduled := theLinesSaying(onceResponding, func(line string) bool { return strings.Contains(line, " scheduled on ") })
	expect.Expect(t, zoneFollowed{
		TurnedNotReady:   theNodesNamedTurning(held.theLinesFollowed(t), scenario.NodeNotReady),
		TurnedReady:      theNodesNamedTurning(onceResponding, scenario.NodeReady),
		MovedBack:        len(movedBack) > 0,
		MovedAllReplaced: len(scheduled) >= len(movedBack),
	}).To(expect.Equal(zoneFollowed{
		TurnedNotReady: held.theNodesOfZone(zone), TurnedReady: held.theNodesOfZone(zone),
		MovedBack: thePodsMovedBack, MovedAllReplaced: everyMoveReplaced,
	}))
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
