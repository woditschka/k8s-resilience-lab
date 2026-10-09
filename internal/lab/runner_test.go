// The tests in this file drive the name a Scenario runs under, and how a run
// carries out its Scenarios at once. The name comes from the file the
// Scenario's function is defined in, so no run input reaches it and no Scenario
// carries a name of its own. They reach no engine: a name that cannot be derived
// stops the run before a Simulated Cluster is provisioned, and carrying out
// Scenarios at once is driven with functions in memory, because a Scenario
// defined beside a test has no run name (docs/system-design.md, lab.Runner).
package lab

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/cluster"
	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/timeline"
)

const (
	// someScenariosPlace is where the Go runtime would record a Scenario source.
	// Only the file's own name bears on the run name, so the directory stands for
	// any.
	someScenariosPlace = "/home/engineer/k8s-resilience-lab/scenarios"

	// theScenarioOfATestFile and theTestFileHoldingIt are what a run that can
	// derive no name names: the function below, and this file it is defined in,
	// which is a test file and so no Scenario source.
	theScenarioOfATestFile = "aScenarioDefinedBesideThisTest"
	theTestFileHoldingIt   = "runner_test.go"
)

// aScenarioDefinedBesideThisTest is a function over the Step surface that no
// Scenario source holds: the Scenarios' place holds no test file, so the lab can
// locate no Scenario file for it and derives no run name.
func aScenarioDefinedBesideThisTest(*Scenario) {}

// aRunnerReachingNoEngine is the Runner a derivation is driven through. A run
// name that cannot be derived stops the run before anything is provisioned, so
// the engine this runner holds is never dialed and the Timeline folder it files
// into is never created; both stand for any.
func aRunnerReachingNoEngine(t *testing.T) *Runner {
	t.Helper()

	return NewRunner(cluster.Endpoint{}, cluster.DefaultKubernetesVersion(), timeline.NewFolder(t.TempDir()))
}

func TestAScenarioFile_ShouldGiveARunNameOfItsStemWithHyphens(t *testing.T) {
	runNamesPerFile := map[string]struct {
		file    string
		runName string
	}{
		"a file of two words": {file: "resilient_placement.go", runName: "resilient-placement"},
		"a file of one word":  {file: "eviction.go", runName: "eviction"},
	}

	for named, expected := range runNamesPerFile {
		t.Run(named, func(t *testing.T) {
			derived, err := runNameOf(filepath.Join(someScenariosPlace, expected.file))
			if err != nil {
				t.Fatalf("derive the run name of %s: %v", expected.file, err)
			}

			expect.Expect(t, derived).To(expect.Equal(expected.runName))
		})
	}
}

func TestAFileThatIsNoScenarioSource_ShouldYieldNoRunNameRatherThanAGuess(t *testing.T) {
	filesOfNoScenario := map[string]string{
		"a test file":             filepath.Join(someScenariosPlace, "resilient_placement_test.go"),
		"a file that is not Go":   filepath.Join(someScenariosPlace, theReadme),
		"a directory and no file": someScenariosPlace + string(filepath.Separator),
	}

	for named, file := range filesOfNoScenario {
		t.Run(named, func(t *testing.T) {
			derived, err := runNameOf(file)

			if err == nil {
				t.Errorf("%s yields no run name, but %s gave %q", named, file, derived)
			}
		})
	}
}

func TestAFunctionOfNoScenarioFile_ShouldStopTheRunNamingTheFunctionAndTheFile(t *testing.T) {
	running := aRunnerReachingNoEngine(t)

	reported, err := running.Run(t.Context(), aScenarioDefinedBesideThisTest)

	if err == nil {
		t.Fatalf("a function the lab can locate no Scenario file for stops the run, but it reported: %+v", reported)
	}
	if !strings.Contains(err.Error(), theScenarioOfATestFile) {
		t.Errorf("the run names the function it could derive no name for, but reads: %v", err)
	}
	if !strings.Contains(err.Error(), theTestFileHoldingIt) {
		t.Errorf("the run names the file it could derive no name from, but reads: %v", err)
	}
}

func TestARunListingAFunctionOfNoScenarioFile_ShouldStopBeforeCarryingOutAnyScenario(t *testing.T) {
	running := aRunnerReachingNoEngine(t)

	carried, err := running.RunAtOnce(t.Context(), []func(*Scenario){aScenarioDefinedBesideThisTest})

	if err == nil {
		t.Fatalf("a function the lab can locate no Scenario file for stops the run, but it carried out: %+v", carried)
	}
	if !strings.Contains(err.Error(), theScenarioOfATestFile) {
		t.Errorf("the run names the function it could derive no name for, but reads: %v", err)
	}
}

// threeScenariosOfARun are the run names the Scenarios carried out at once below
// run under. Which names they are bears on nothing; that there are several, each
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

// theNamesReportedIn are the run names of what a run carried out, in the order
// it handed them back.
func theNamesReportedIn(carried []Carried) []string {
	named := make([]string, 0, len(carried))
	for _, each := range carried {
		named = append(named, each.Outcome.Name)
	}

	return named
}

// theScenariosThatWentWrongIn are the run names of the Scenarios the lab could
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
