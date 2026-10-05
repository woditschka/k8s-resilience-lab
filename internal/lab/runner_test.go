// The tests in this file drive the name a Scenario runs under. It comes from the
// file the Scenario's function is defined in, so no run input reaches it and no
// Scenario carries a name of its own. They reach no engine: a name that cannot be
// derived stops the run before a Simulated Cluster is provisioned, which is the
// one path through the Runner that needs none.
package lab

import (
	"path/filepath"
	"strings"
	"testing"

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
		"a file of two words": {file: "even_spread.go", runName: "even-spread"},
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
		"a test file":             filepath.Join(someScenariosPlace, "even_spread_test.go"),
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
