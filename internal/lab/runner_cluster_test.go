// These tests cross to a real Simulated Cluster. One judges the order in which
// a panicking Scenario lets go of the cluster, its recording and the
// recording's file: no stand-in has a recording to outlive its file. The others
// judge what an Interactive run holds, observes and removes.
package lab

import (
	"context"
	"log/slog"
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
	once := scenario.NewPlacementBoard(topology, placed.Pods(), scenario.PlacementBoard{})

	return observed{Lines: scenario.NewPlacementBoard(topology, placed.Pods(), once).Lines()}
}

// A specification naming nothing is refused by the Workload's constructor, so
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

// The board observed while held shows the Workload's Pods alone, so no Pod the
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
		steps.Given().Topology(Zones(twoZones), theGeneralNodePool())
		steps.WaitFor().AllNodesReady()
		steps.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), resilientPlacement())
		steps.WaitFor().AllPodsReady()
		declared = steps.topology

		steps.When().NodesFail(Nodes(oneNodeFailing))
		steps.WaitFor().AllPodsReady()
	}, observer, func(Outcome) {
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
		steps.Given().Topology(Zones(twoZones), theGeneralNodePool())
		stopTheLab()
		steps.WaitFor().AllNodesReady()
	}, anObserverDiscardingEachBoard(), func(Outcome) {})
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
		steps.Given().Topology(Zones(twoZones), theGeneralNodePool())
		steps.WaitFor().AllNodesReady()
		steps.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), aSpecificationTheConstructorRefuses())
	}, anObserverDiscardingEachBoard(), func(Outcome) {
		readyWhileHeld = theReadyNodesOf(t, provisioned)
		stopTheLab()
	})
	if err != nil {
		t.Fatalf("carry out the Scenario %s: %v", theScenarioEndedAtARefusal, err)
	}

	expect.Expect(t, held.EndedAt.Kind).To(expect.Equal(GivenStep))
	expect.Expect(t, readyWhileHeld).To(expect.HaveLen[scenario.Node](theNodesOfTwoZonesOfTwoEach))
}
