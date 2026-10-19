// The tests in this file drive the name a Scenario runs under, and how a run
// carries out its Scenarios at once. The name comes from the condition half of
// the Scenario's function's name, so no run input reaches it and no Scenario
// carries a name of its own. They reach no engine: a function naming no
// condition, or two naming one, stops the run before a Simulated Cluster is
// provisioned, and carrying out Scenarios at once is driven with functions in
// memory (docs/system-design.md, lab.Runner).
package lab

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/cluster"
	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/timeline"
)

// someScenariosPackage is the package path the Go runtime writes before a
// function's own name. Only the name after it bears on the Scenario name, so the
// path stands for any.
const someScenariosPackage = "github.com/woditschka/k8s-resilience-lab/scenarios."

// The functions below are Scenarios a refusal is driven with. Each is declared
// at the top level, because a closure's runtime name is itself one naming no
// condition.
const (
	theFunctionNamingNoCondition = "aFunctionNamingNoCondition"
	theFirstOfTheTwins           = "ATwinCondition_ShouldReachOneOutcome"
	theSecondOfTheTwins          = "ATwinCondition_ShouldReachAnotherOutcome"
)

func aFunctionNamingNoCondition(*Scenario) {}

func ATwinCondition_ShouldReachOneOutcome(*Scenario) {}

func ATwinCondition_ShouldReachAnotherOutcome(*Scenario) {}

// aRunnerReachingNoEngine is the Runner a refusal is driven through. A Scenario
// name that cannot be derived stops the run before anything is provisioned, so
// the engine this runner holds is never dialed and the Timeline folder it files
// into is never created; both stand for any.
func aRunnerReachingNoEngine(t *testing.T) *Runner {
	t.Helper()

	return NewRunner(cluster.Endpoint{}, cluster.DefaultKubernetesVersion(), timeline.NewFolder(t.TempDir()))
}

func TestAScenarioFunction_ShouldRunUnderTheConditionItsNameStatesWithHyphens(t *testing.T) {
	scenarioNamesPerFunction := map[string]struct {
		function     string
		scenarioName string
	}{
		"a condition opening with a one-letter word": {
			function:     "ANodeThatStopsResponding_ShouldLeaveThePodsOnTheNodesStillResponding",
			scenarioName: "a-node-that-stops-responding",
		},
		"a one-letter word inside the condition": {
			function:     "NoRoomAfterAZoneStopsResponding_ShouldSettleAtSixPodsRunningAndThreePending",
			scenarioName: "no-room-after-a-zone-stops-responding",
		},
		"a run of capitals before a word": {
			function:     "GeneralPodsBesideTaintedGPUNodes_ShouldStayOffThem",
			scenarioName: "general-pods-beside-tainted-gpu-nodes",
		},
		"a run of capitals ending the condition": {function: "PodsOnGPU_ShouldRun", scenarioName: "pods-on-gpu"},
		"a digit ending a word":                  {function: "Zone3StopsResponding_ShouldHold", scenarioName: "zone3-stops-responding"},
		"a condition of one word":                {function: "Eviction_ShouldRestoreThePods", scenarioName: "eviction"},
	}

	for named, expected := range scenarioNamesPerFunction {
		t.Run(named, func(t *testing.T) {
			derived, err := scenarioNameOf(someScenariosPackage + expected.function)
			if err != nil {
				t.Fatalf("derive the Scenario name of %s: %v", expected.function, err)
			}

			expect.Expect(t, derived).To(expect.Equal(expected.scenarioName))
		})
	}
}

func TestAFunctionNameStatingNoCondition_ShouldYieldNoScenarioNameButNameTheFunction(t *testing.T) {
	functionsNamingNoCondition := map[string]string{
		"a name without the outcome":         "AWorkloadPlacedForResilience",
		"an empty condition":                 "_ShouldPlaceThreePodsPerZone",
		"a closure inside a Scenario":        "AWorkload_ShouldBePlaced.func1",
		"a method value":                     "(*Catalog).AWorkload_ShouldBePlaced-fm",
		"an underscore inside the condition": "A_Workload_ShouldBePlaced",
		"a letter beyond ASCII":              "AWorkloadÜberall_ShouldBePlaced",
	}

	for named, function := range functionsNamingNoCondition {
		t.Run(named, func(t *testing.T) {
			derived, err := scenarioNameOf(someScenariosPackage + function)

			if err == nil {
				t.Fatalf("%s names no condition, but gave the Scenario name %q", function, derived)
			}
			expect.Expect(t, strings.Contains(err.Error(), function)).To(expect.Equal(true))
		})
	}
}

func TestAScenarioFunctionNamingNoCondition_ShouldStopTheRunNamingTheFunction(t *testing.T) {
	running := aRunnerReachingNoEngine(t)

	reported, err := running.Run(t.Context(), aFunctionNamingNoCondition)

	if err == nil {
		t.Fatalf("a function naming no condition stops the run, but it reported: %+v", reported)
	}
	expect.Expect(t, strings.Contains(err.Error(), theFunctionNamingNoCondition)).To(expect.Equal(true))
}

func TestARunListingAFunctionNamingNoCondition_ShouldStopBeforeCarryingOutAnyScenario(t *testing.T) {
	running := aRunnerReachingNoEngine(t)

	carried, err := running.RunAtOnce(t.Context(), []func(*Scenario){aFunctionNamingNoCondition})

	if err == nil {
		t.Fatalf("a function naming no condition stops the run, but it carried out: %+v", carried)
	}
	expect.Expect(t, strings.Contains(err.Error(), theFunctionNamingNoCondition)).To(expect.Equal(true))
}

func TestTwoScenariosNamingTheSameCondition_ShouldStopTheRunBeforeCarryingOutAnyScenario(t *testing.T) {
	running := aRunnerReachingNoEngine(t)

	carried, err := running.RunAtOnce(t.Context(),
		[]func(*Scenario){ATwinCondition_ShouldReachOneOutcome, ATwinCondition_ShouldReachAnotherOutcome})

	if err == nil {
		t.Fatalf("two Scenarios naming one condition stop the run, but it carried out: %+v", carried)
	}
	expect.Expect(t, strings.Contains(err.Error(), theFirstOfTheTwins)).To(expect.Equal(true))
	expect.Expect(t, strings.Contains(err.Error(), theSecondOfTheTwins)).To(expect.Equal(true))
}

// threeScenariosOfARun are the Scenario names the Scenarios carried out at once
// below run under. Which names they are bears on nothing; that there are several, each
// distinct, is what a run carrying them out at once is judged by.
var threeScenariosOfARun = []string{"some-scenario", "another-scenario", "a-third-scenario"}

// longerThanCarryingThemOutTakes bounds how long a Scenario below waits for the
// others. Carried out at once they meet within milliseconds; carried out one
// after another they never meet, and the deadline is what turns that into a
// failing test rather than a hung one.
const longerThanCarryingThemOutTakes = 10 * time.Second

// noScenarioWentWrong is how many Scenarios a run that carried out every one to
// its end names as having gone wrong.
const noScenarioWentWrong = 0

func TestARunOfEveryScenario_ShouldStartEachScenarioWithoutWaitingForAnotherToEnd(t *testing.T) {
	t.Parallel()
	eachWaitingForTheOthers := aMeetingOfEveryScenario(t, len(threeScenariosOfARun))

	carried := carryOutAtOnce(threeScenariosOfARun, eachWaitingForTheOthers)

	expect.Expect(t, theScenariosThatWentWrongIn(carried)).To(expect.HaveLen[string](noScenarioWentWrong))
}

func TestARunOfEveryScenario_ShouldHandBackEachOutcomeInDeclaredOrderWhateverOrderTheyFinishedIn(t *testing.T) {
	t.Parallel()
	lastDeclaredFinishingFirst := aRunFinishingInReverse(t, threeScenariosOfARun)

	carried := carryOutAtOnce(threeScenariosOfARun, lastDeclaredFinishingFirst)

	expect.Expect(t, theNamesReportedIn(carried)).To(expect.Equal(threeScenariosOfARun))
}

func TestAScenarioThatFailsPartWayAmongOthers_ShouldLeaveTheOthersToReachTheirOwnOutcome(t *testing.T) {
	t.Parallel()
	thePanickingScenario := threeScenariosOfARun[1]

	carried := carryOutAtOnce(threeScenariosOfARun, oneOfThemPanicking(threeScenariosOfARun, thePanickingScenario))

	expect.Expect(t, theNamesReportedIn(carried)).To(expect.Equal(threeScenariosOfARun))
	expect.Expect(t, theScenariosThatWentWrongIn(carried)).To(expect.Equal([]string{thePanickingScenario}))
	expect.Expect(t, strings.Contains(theFailureOf(carried, thePanickingScenario), whatThePanicSaid)).To(expect.Equal(true))
}

// whatThePanicSaid is the value a Scenario panics with above, which the run
// names so a reader can find what panicked.
const whatThePanicSaid = "a Step's own code panicked"

// oneOfThemPanicking carries out the Scenario named panicking by panicking, and
// every other Scenario by reporting an Outcome under its own name.
func oneOfThemPanicking(named []string, panicking string) func(int) (Outcome, error) {
	return func(declared int) (Outcome, error) {
		if named[declared] == panicking {
			panic(whatThePanicSaid)
		}

		return Outcome{Name: named[declared]}, nil
	}
}

// aMeetingOfEveryScenario carries out a Scenario by waiting until every one of
// the run has started, so it ends only when they all run at once. One that waits
// past the deadline ends naming why.
func aMeetingOfEveryScenario(t *testing.T, scenarios int) func(int) (Outcome, error) {
	t.Helper()

	ctx, stop := context.WithTimeout(t.Context(), longerThanCarryingThemOutTakes)
	t.Cleanup(stop)

	var started sync.WaitGroup
	started.Add(scenarios)
	everyOneStarted := make(chan struct{})
	go func() {
		started.Wait()
		close(everyOneStarted)
	}()

	return func(int) (Outcome, error) {
		started.Done()
		select {
		case <-everyOneStarted:
			return Outcome{}, nil
		case <-ctx.Done():
			return Outcome{}, context.Cause(ctx)
		}
	}
}

// aRunFinishingInReverse carries out each Scenario by waiting for the one
// declared after it to finish, so the last declared finishes first and the first
// declared last.
func aRunFinishingInReverse(t *testing.T, named []string) func(int) (Outcome, error) {
	t.Helper()

	ctx, stop := context.WithTimeout(t.Context(), longerThanCarryingThemOutTakes)
	t.Cleanup(stop)

	finished := make([]chan struct{}, len(named)+1)
	for declared := range finished {
		finished[declared] = make(chan struct{})
	}
	close(finished[len(named)])

	return func(declared int) (Outcome, error) {
		defer close(finished[declared])
		select {
		case <-finished[declared+1]:
			return Outcome{Name: named[declared]}, nil
		case <-ctx.Done():
			return Outcome{Name: named[declared]}, context.Cause(ctx)
		}
	}
}

// theNamesReportedIn are the Scenario names of what a run carried out, in the order
// it handed them back.
func theNamesReportedIn(carried []Carried) []string {
	named := make([]string, 0, len(carried))
	for _, each := range carried {
		named = append(named, each.Outcome.Name)
	}

	return named
}

// theScenariosThatWentWrongIn are the Scenario names of the Scenarios the lab could
// not carry out to their end.
func theScenariosThatWentWrongIn(carried []Carried) []string {
	var wentWrong []string
	for _, each := range carried {
		if each.Err != nil {
			wentWrong = append(wentWrong, each.Outcome.Name)
		}
	}

	return wentWrong
}

// theFailureOf is what the run names as having gone wrong for the Scenario
// named, and empty where nothing did.
func theFailureOf(carried []Carried, named string) string {
	for _, each := range carried {
		if each.Outcome.Name == named && each.Err != nil {
			return each.Err.Error()
		}
	}

	return ""
}
