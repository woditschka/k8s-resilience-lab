// The two rules in this file govern the place the lab's Scenarios sit in: that
// every declaration there is a Scenario, a Workload name or the list, each in the
// file admitted for it; and that the place holds every Scenario under a name it
// can run, a README naming them, no test file and nothing of the machinery. They
// live with the machinery that defines what a Step is rather than in the
// prominent place itself, so the place stays Scenarios alone
// (docs/architecture-principles.md § Module Boundaries).
package lab

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/woditschka/k8s-resilience-lab/internal/modulerules"
)

// ownPackageDir is where go test runs this suite: the package's own source
// directory, which the module root is located from.
const ownPackageDir = "."

// scenariosDirectory is the one place the lab's Scenarios sit, under the module
// root: prominent at the top level, and holding nothing of the machinery this
// package is.
const scenariosDirectory = "scenarios"

// theReadme names every Scenario of that place and the outcome each asserts,
// and theTestFileSuffix marks a file the place does not hold.
const (
	theReadme         = "README.md"
	theTestFileSuffix = "_test.go"
)

// theWorkloadNames is the one file of the place that is not a Scenario and not
// the list: where the lab's own Scenarios take the names of the Workloads they
// deploy from. In it, every declaration is an exported constant of the Step
// surface's Workload-name type, so the file holds names and no machinery.
const (
	theWorkloadNames    = "workloads.go"
	theWorkloadNameType = "lab.WorkloadName"
)

// theList is the other file of the place that is not a Scenario: it holds the
// one exported function naming every Scenario of the place, taking nothing,
// which is what a run iterates. onlyOnce is how often that function names each
// of them: naming none leaves a Scenario unrun, and naming one twice runs it
// twice.
const (
	theList         = "scenarios.go"
	theListFunction = "All"
	onlyOnce        = 1
)

// theLabsOwnPackages is the prefix of an import a Scenario source would reach the
// lab's own code through, and theStepSurface is the one of them it may name.
const (
	theLabsOwnPackages = "github.com/woditschka/k8s-resilience-lab/"
	theStepSurface     = theLabsOwnPackages + "internal/lab"
)

// theKubernetesPackages are the client packages a Scenario never names: a
// Scenario declares in the lab's own vocabulary, and the mapping to the control
// plane's lives in internal/cluster.
const theKubernetesPackages = "k8s.io"

// theScenarioParameter is what a Scenario function takes, and the value its every
// Step is called on, as a Scenario source writes it. A source reaching the Step
// surface under another name is reported as holding no Scenario, which is the
// same finding by a different route.
const theScenarioParameter = "*lab.Scenario"

// theScenarioNameShape is how a Scenario's function is named, and
// theOutcomePrefix opens the half of it that states the outcome: the shape
// docs/testing-principles.md § Test Naming gives a test name without its Test
// prefix, so a Scenario reads as what it asserts wherever it is named.
const (
	theScenarioNameShape = "<Condition>_Should<Outcome>"
	theOutcomePrefix     = "Should"
)

// theStepKinds are the three kinds a line of a Scenario may be.
var theStepKinds = []StepKind{GivenStep, WhenStep, ThenStep}

func TestEveryScenarioSource_ShouldHoldOnlyGivenWhenAndThenSteps(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := declarationsThatDoNotBelong(filesOfThePlace(t, place))

	if len(offenders) > 0 {
		t.Errorf("a Scenario source holds one exported Scenario function named %s whose every line is a Given, When or Then Step, and the two files beside them declare the Workload names and the list alone, but:\n%s",
			theScenarioNameShape,
			strings.Join(offenders, "\n"))
	}
}

func TestTheScenariosPlace_ShouldHoldEveryScenarioAndAReadmeNamingThem(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := whatThePlaceIsMissing(t, place, filesOfThePlace(t, place))

	if len(offenders) > 0 {
		t.Errorf("one place holds every Scenario of the lab, a README naming them, and nothing of the machinery, but:\n%s",
			strings.Join(offenders, "\n"))
	}
}

// theScenariosPlace is where the lab's Scenarios sit, located from the module
// file rather than from where go test happens to run.
func theScenariosPlace(t *testing.T) string {
	t.Helper()

	root, err := modulerules.Root(t.Context(), ownPackageDir)
	if err != nil {
		t.Fatalf("locate the root of the module: %v", err)
	}

	return filepath.Join(root, scenariosDirectory)
}

// filesOfThePlace is every Go file of that place the lab ships: the Scenario
// sources and the file of Workload names beside them. The place holds no test
// file, which the place rule below reports rather than scans.
func filesOfThePlace(t *testing.T, place string) []modulerules.GoFile {
	t.Helper()

	sources, err := modulerules.ScanGoFiles(place, modulerules.ProductionGoFilesOnly)
	if err != nil {
		t.Fatalf("read the Scenario sources under %s: %v", place, err)
	}

	return sources
}

// scenarioSourcesIn is every file of the place that is a Scenario's own source,
// which is every one of them but the two admitted beside them by name.
func scenarioSourcesIn(files []modulerules.GoFile) []modulerules.GoFile {
	var sources []modulerules.GoFile
	for _, file := range files {
		if isAScenarioSource(file.Path) {
			sources = append(sources, file)
		}
	}

	return sources
}

// isAScenarioSource reports whether a file of the place holds a Scenario, which
// every file of it does but the Workload names and the list.
func isAScenarioSource(path string) bool {
	switch filepath.Base(path) {
	case theWorkloadNames, theList:
		return false
	default:
		return true
	}
}

// declarationsThatDoNotBelong names every declaration of the place that its own
// file does not admit, and every line of a Scenario that is not a Step. One
// predicate per file admitted by name, so a further shape is a deliberate edit
// rather than a side effect.
func declarationsThatDoNotBelong(files []modulerules.GoFile) []string {
	var offenders []string
	for _, file := range files {
		switch filepath.Base(file.Path) {
		case theWorkloadNames:
			offenders = append(offenders, declarationsThatAreNotWorkloadNames(file)...)
		case theList:
			offenders = append(offenders, declarationsThatAreNotTheList(file)...)
		default:
			offenders = append(offenders, declarationsThatAreNotAScenario(file)...)
		}
	}

	return offenders
}

// declarationsThatAreNotAScenario names every declaration of a Scenario source
// that is not a Scenario, and every line of a Scenario that is not a Step.
func declarationsThatAreNotAScenario(source modulerules.GoFile) []string {
	var offenders []string
	for _, declared := range source.Declarations {
		if !isAScenario(declared) {
			offenders = append(offenders, fmt.Sprintf("  %s declares the %s %s, which is not a Scenario named %s",
				source.Path, declared.Kind, declared.Name, theScenarioNameShape))

			continue
		}
		offenders = append(offenders, linesOfScenarioThatAreNotSteps(source.Path, declared)...)
	}

	return offenders
}

// declarationsThatAreNotWorkloadNames names every declaration of the file of
// Workload names that is not one: the file holds the names the lab's Scenarios
// deploy under and nothing else.
func declarationsThatAreNotWorkloadNames(source modulerules.GoFile) []string {
	var offenders []string
	for _, declared := range source.Declarations {
		if declared.Kind != modulerules.ConstDeclaration || !declared.Exported || declared.Type != theWorkloadNameType {
			offenders = append(offenders, fmt.Sprintf("  %s declares the %s %s, which is not an exported %s constant",
				source.Path, declared.Kind, declared.Name, theWorkloadNameType))
		}
	}

	return offenders
}

// isAScenario reports whether a declaration is a Scenario: an exported function
// over the Step surface, named for the condition it runs under and the outcome
// it asserts, and nothing else.
func isAScenario(declared modulerules.Declaration) bool {
	return declared.Kind == modulerules.FuncDeclaration && declared.Exported &&
		len(declared.Parameters) == 1 && declared.Parameters[0].Type == theScenarioParameter &&
		isNamedAsAScenario(declared.Name)
}

// isNamedAsAScenario reports whether a name is the Scenario-name shape: a
// condition, one underscore, then Should and the outcome asserted.
func isNamedAsAScenario(name string) bool {
	condition, outcome, underscored := strings.Cut(name, "_")

	return underscored && condition != "" && !strings.Contains(outcome, "_") &&
		strings.HasPrefix(outcome, theOutcomePrefix) && outcome != theOutcomePrefix
}

// linesOfScenarioThatAreNotSteps names every line of a Scenario that is not a
// Given, When or Then Step called on the Scenario the function took.
func linesOfScenarioThatAreNotSteps(path string, declared modulerules.Declaration) []string {
	var offenders []string
	for _, line := range declared.Statements {
		if !isAStep(line, declared.Parameters[0].Name) {
			offenders = append(offenders, fmt.Sprintf("  %s, in %s: %s is not a Given, When or Then Step",
				path, declared.Name, line))
		}
	}

	return offenders
}

func isAStep(line, scenarioParameter string) bool {
	return slices.ContainsFunc(theStepKinds, func(kind StepKind) bool {
		return strings.HasPrefix(line, fmt.Sprintf("%s.%s().", scenarioParameter, kind))
	})
}

// declarationsThatAreNotTheList names every declaration of the list file that is
// not the list itself: the file holds the one exported function naming the
// Scenarios and nothing else.
func declarationsThatAreNotTheList(source modulerules.GoFile) []string {
	var offenders []string
	for _, declared := range source.Declarations {
		if !isTheList(declared) {
			offenders = append(offenders, fmt.Sprintf("  %s declares the %s %s, which is not the exported %s naming the Scenarios",
				source.Path, declared.Kind, declared.Name, theListFunction))
		}
	}

	return offenders
}

// isTheList reports whether a declaration is the list: the exported function of
// that name, taking nothing, so a run reaches every Scenario through it alone.
func isTheList(declared modulerules.Declaration) bool {
	return declared.Kind == modulerules.FuncDeclaration && declared.Exported &&
		declared.Name == theListFunction && len(declared.Parameters) == 0
}

// whatThePlaceIsMissing names what the Scenarios' place does not hold: a
// Scenario, its README, a README entry for a Scenario it holds, the list naming
// each Scenario once, or a Scenario source reaching past the Step surface.
func whatThePlaceIsMissing(t *testing.T, place string, files []modulerules.GoFile) []string {
	t.Helper()

	held := scenarioSourcesIn(files)

	var missing []string
	if len(held) == 0 {
		missing = append(missing, fmt.Sprintf("  %s holds no Scenario", place))
	}

	missing = append(missing, testFilesIn(t, place)...)
	missing = append(missing, scenariosTheListDoesNotNameOnce(place, files, held)...)

	named := readTheReadme(t, place)
	for _, source := range held {
		if !strings.Contains(named, filepath.Base(source.Path)) {
			missing = append(missing, fmt.Sprintf("  %s names no Scenario of %s", theReadme, filepath.Base(source.Path)))
		}
		if _, err := runNameOf(source.Path); err != nil {
			missing = append(missing, fmt.Sprintf("  %s holds a Scenario that would run under no name: %v", place, err))
		}
	}
	for _, file := range files {
		missing = append(missing, machineryImportedBy(file)...)
	}

	return missing
}

// scenariosTheListDoesNotNameOnce names every Scenario of the place its list
// does not name exactly once, and the list itself where the place holds none: a
// Scenario the list does not name is one no run reaches.
func scenariosTheListDoesNotNameOnce(place string, files, held []modulerules.GoFile) []string {
	listed, found := theListIn(files)
	if !found {
		return []string{fmt.Sprintf("  %s holds no %s declaring the exported %s", place, theList, theListFunction)}
	}

	var missing []string
	for _, source := range held {
		for _, declared := range source.Declarations {
			if named := timesNamed(listed, declared.Name); isAScenario(declared) && named != onlyOnce {
				missing = append(missing, fmt.Sprintf("  %s names the Scenario %s %d times, not once",
					theList, declared.Name, named))
			}
		}
	}

	return missing
}

// theListIn is the list of the place's Scenarios, where the place holds the file
// declaring it.
func theListIn(files []modulerules.GoFile) (modulerules.Declaration, bool) {
	for _, file := range files {
		if filepath.Base(file.Path) != theList {
			continue
		}
		for _, declared := range file.Declarations {
			if isTheList(declared) {
				return declared, true
			}
		}
	}

	return modulerules.Declaration{}, false
}

// timesNamed is how often the list names name, read off the identifiers of its
// own lines as the source writes them, so a name inside a longer one is not
// mistaken for the Scenario.
func timesNamed(listed modulerules.Declaration, name string) int {
	named := 0
	for _, line := range listed.Statements {
		for _, identifier := range identifiersIn(line) {
			if identifier == name {
				named++
			}
		}
	}

	return named
}

// identifiersIn is every identifier one line of the list names, the line split
// at every character an identifier cannot hold.
func identifiersIn(line string) []string {
	return strings.FieldsFunc(line, func(character rune) bool {
		return !unicode.IsLetter(character) && !unicode.IsDigit(character) && character != '_'
	})
}

// testFilesIn names every test file of the place, which holds what the lab ships
// alone: a Scenario is itself the check, and a test beside it could only repeat
// its own Then Steps, so the lab's Scenarios run in the command's own suite
// instead (docs/adr/2026-10-03-scenarios-as-go-functions.md).
func testFilesIn(t *testing.T, place string) []string {
	t.Helper()

	held, err := os.ReadDir(place)
	if err != nil {
		t.Fatalf("read the files of %s: %v", place, err)
	}

	var tests []string
	for _, file := range held {
		if strings.HasSuffix(file.Name(), theTestFileSuffix) {
			tests = append(tests, fmt.Sprintf("  %s holds the test file %s", place, file.Name()))
		}
	}

	return tests
}

// readTheReadme is what the README of the Scenarios' place says, which is what
// names each Scenario to an engineer who opens the place.
func readTheReadme(t *testing.T, place string) string {
	t.Helper()

	named, err := os.ReadFile(filepath.Join(place, theReadme))
	if err != nil {
		t.Errorf("read the README naming every Scenario: %v", err)
	}

	return string(named)
}

// machineryImportedBy names every import of a Scenario source that reaches past
// the Step surface: another package of the lab, or the Kubernetes client.
func machineryImportedBy(source modulerules.GoFile) []string {
	var offenders []string
	for _, imported := range source.Imports {
		reachesTheLab := strings.HasPrefix(imported, theLabsOwnPackages) && imported != theStepSurface
		if reachesTheLab || strings.HasPrefix(imported, theKubernetesPackages) {
			offenders = append(offenders, fmt.Sprintf("  %s imports %s, which is not the Step surface", source.Path, imported))
		}
	}

	return offenders
}
