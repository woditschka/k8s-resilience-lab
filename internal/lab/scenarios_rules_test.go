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
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
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

// theStepSurface is the one package a Scenario source imports: the lab's own
// Step surface, this package.
const theStepSurface = "github.com/woditschka/k8s-resilience-lab/internal/lab"

// theStepSurfaceName is the name a Scenario source reaches that surface under,
// which is the one prefix every name it writes carries, and theScenarioParameter
// is what a Scenario function takes and its every Step is called on, as a source
// writes it. A source reaching the surface under another name is reported as
// holding no Scenario, which is the same finding by a different route.
const (
	theStepSurfaceName   = "lab"
	theScenarioParameter = "*" + theStepSurfaceName + ".Scenario"
)

// theLabsOwnPackageDir is where this package's own sources sit under the module
// root, which the names it offers Scenarios are read off.
const theLabsOwnPackageDir = "internal/lab"

// thePodSettingsPackage is the one Kubernetes package a Scenario may import
// beside the Step surface: the one naming the Pod settings a Scenario reaches
// through full access, where the lab's own vocabulary names none. It is the single
// stated exception to the one-vocabulary rule, carried by the depguard rule
// scenarios-hold-steps-alone and by the import rule below together.
// thePodSettingsName is the name the lab's own sources reach it under.
const (
	thePodSettingsPackage = "k8s.io/api/core/v1"
	thePodSettingsName    = "corev1"
)

// The imports a scratch Scenario source is written with, each as a source writes
// it: the Step surface, the Pod settings package, and a Kubernetes package that is
// neither.
const (
	theStepSurfaceImport      = `"` + theStepSurface + `"`
	thePodSettingsImport      = thePodSettingsName + ` "` + thePodSettingsPackage + `"`
	theKubernetesClientImport = `"k8s.io/client-go/kubernetes"`
)

// theScenarioSourceShape is a Scenario source holding its imports and one Workload
// line, which the shapes the Workload-line, name and import rules admit are read
// on. It is written as a Scenario of the lab is written, so a rule reaches its
// lines the same way, and theSourceFileMode is the mode such a scratch source is
// written under.
const theScenarioSourceShape = `package scenarios

import (
%s)

const Checkout lab.WorkloadName = "checkout"

func AWorkload_ShouldBeDeclared(s *lab.Scenario) {
	%s
}
`

const theSourceFileMode = 0o600

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

// theWorkloadStep is how a Workload's Given line names itself and theCustomize
// call the one call a Scenario may write on the specification it declares. The
// line takes three arguments — the Workload name, the Pod count and one of the
// lab's predefined Pod specifications — and the specification is that one call,
// optionally customized once. A setting of the Scenario's own would be a further
// argument or a call of its own, and neither is admitted, per
// docs/adr/2026-10-05-workload-declares-a-predefined-pod-specification.md.
//
// The shape is read off the syntax tree rather than the text of the line,
// because a Customization wraps over several lines once gofmt has had it.
const (
	theWorkloadStep = "Workload"
	theCustomize    = "Customize"

	theWorkloadLineArguments = 3
	theSpecificationArgument = 2
)

// theWorkloadsOwnCalls is the closed set a Workload line may end at one call of,
// written on the Workload Step itself: the Required Capacity the Workload
// declares, per docs/adr/2026-10-06-required-capacity-judged-at-two-moments.md. A
// second of them, and any other call written there, stays refused.
var theWorkloadsOwnCalls = []string{"RequiredCapacity"}

func TestEveryScenarioSource_ShouldHoldOnlyGivenWhenAndThenSteps(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := declarationsThatDoNotBelong(filesOfThePlace(t, place))

	if len(offenders) > 0 {
		t.Errorf("a Scenario source holds one exported Scenario function named %s whose every line is a Given, When or Then Step, and the two files beside them declare the Workload names and the list alone, but:\n%s",
			theScenarioNameShape,
			strings.Join(offenders, "\n"))
	}
}

func TestEveryScenarioOfTheLab_ShouldDeclareOnePredefinedPodSpecificationPerWorkload(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := workloadLinesDeclaringNoPredefinedSpecification(filesOfThePlace(t, place))

	if len(offenders) > 0 {
		t.Errorf("every Workload line of a Scenario declares one of the lab's predefined Pod specifications and no setting of its own, but:\n%s",
			strings.Join(offenders, "\n"))
	}
}

// The shapes a Workload line takes, read as a rule over the syntax tree rather
// than over the text of the line: one predefined specification, customized or
// not, and nothing besides. No Scenario of the lab writes a Customization yet, so
// the shipped sources exercise the first of these shapes alone and the rule's own
// decisions are read here.
func TestTheWorkloadLineRule_ShouldAdmitOneCustomizedSpecificationAndReportAnythingElse(t *testing.T) {
	written := map[string]struct {
		line     string
		admitted bool
	}{
		"one predefined specification": {
			line:     `s.Given().Workload(Checkout, lab.Pods(9), lab.ResilientPlacement())`,
			admitted: true,
		},
		"a specification it customizes": {
			line: `s.Given().Workload(Checkout, lab.Pods(9), lab.ResilientPlacement().Customize(
		lab.TopologySpread(lab.TopologyKey(lab.ZoneLabel), lab.MaxSkew(2),
			lab.WhenUnsatisfiable(lab.DoNotSchedule), lab.NodeTaintsPolicy(lab.Honor)),
	))`,
			admitted: true,
		},
		"a Customize naming no setting": {
			line:     `s.Given().Workload(Checkout, lab.Pods(9), lab.ResilientPlacement().Customize())`,
			admitted: true,
		},
		"no specification at all": {
			line:     `s.Given().Workload(Checkout, lab.Pods(9))`,
			admitted: false,
		},
		"a setting written beside the specification": {
			line: `s.Given().Workload(Checkout, lab.Pods(9), lab.ResilientPlacement(),
		lab.TopologySpread(lab.TopologyKey(lab.ZoneLabel), lab.MaxSkew(2),
			lab.WhenUnsatisfiable(lab.DoNotSchedule), lab.NodeTaintsPolicy(lab.Honor)))`,
			admitted: false,
		},
		"a second Customize written on the first": {
			line:     `s.Given().Workload(Checkout, lab.Pods(9), lab.ResilientPlacement().Customize().Customize())`,
			admitted: false,
		},
		"a Required Capacity written after the Workload Step": {
			line:     `s.Given().Workload(Checkout, lab.Pods(9), lab.ResilientPlacement()).RequiredCapacity(lab.Pods(6))`,
			admitted: true,
		},
		"a second Required Capacity written on the first": {
			line: `s.Given().Workload(Checkout, lab.Pods(9), lab.ResilientPlacement()).` +
				`RequiredCapacity(lab.Pods(6)).RequiredCapacity(lab.Pods(3))`,
			admitted: false,
		},
		"a Customize written after the Workload Step": {
			line:     `s.Given().Workload(Checkout, lab.Pods(9), lab.ResilientPlacement()).Customize()`,
			admitted: false,
		},
		"a call written after the Workload Step": {
			line:     `s.Given().Workload(Checkout, lab.Pods(9), lab.ResilientPlacement()).NodeCapacity(lab.Pods(2))`,
			admitted: false,
		},
		"a specification of the Scenario's own": {
			line:     `s.Given().Workload(Checkout, lab.Pods(9), aSpecificationOfItsOwn())`,
			admitted: false,
		},
	}

	for shape, taken := range written {
		t.Run(shape, func(t *testing.T) {
			offenders := workloadLinesDeclaringNoPredefinedSpecification(aScenarioWhoseWorkloadLineIs(t, taken.line))

			expect.Expect(t, len(offenders) == 0).To(expect.Equal(taken.admitted))
		})
	}
}

// theWorkloadLine is a Workload line every rule here admits, which a rule read on
// something other than the line itself is written with. theFullAccessLine is that
// line reaching a Pod setting the lab's own vocabulary does not name, written the
// way a Scenario writes full access.
const (
	theWorkloadLine   = `s.Given().Workload(Checkout, lab.Pods(9), lab.ResilientPlacement())`
	theFullAccessLine = `s.Given().Workload(Checkout, lab.Pods(9), lab.ResilientPlacement().Customize(
		lab.FullAccess(func(pod *` + thePodSettingsName + `.PodSpec) {
			pod.PriorityClassName = "critical"
		}),
	))`
)

// aScenarioWhoseWorkloadLineIs is a Scenario source holding that one Workload
// line and importing the Step surface alone, which is what a Scenario naming the
// lab's own vocabulary needs.
func aScenarioWhoseWorkloadLineIs(t *testing.T, line string) []modulerules.GoFile {
	t.Helper()

	return aScenarioSourceThatImports(t, []string{theStepSurfaceImport}, line)
}

// aScenarioAlsoImporting is a Scenario source importing that one path beside the
// Step surface, which is what a Scenario reaching a Pod setting through full
// access needs.
func aScenarioAlsoImporting(t *testing.T, imported, line string) []modulerules.GoFile {
	t.Helper()

	return aScenarioSourceThatImports(t, []string{theStepSurfaceImport, imported}, line)
}

// aScenarioSourceThatImports is a Scenario source written with those imports and
// that Workload line, scanned the way the rules scan the lab's own place. It is
// written to a directory of its own and parsed, because what a rule reads is a
// parsed source and not a string.
func aScenarioSourceThatImports(t *testing.T, imported []string, line string) []modulerules.GoFile {
	t.Helper()

	place := t.TempDir()
	source := fmt.Sprintf(theScenarioSourceShape, anImportBlockOf(imported), line)
	if err := os.WriteFile(filepath.Join(place, "shape.go"), []byte(source), theSourceFileMode); err != nil {
		t.Fatalf("write the Scenario source the rule is read on: %v", err)
	}

	scanned, err := modulerules.ScanGoFiles(place, modulerules.ProductionGoFilesOnly)
	if err != nil {
		t.Fatalf("scan the Scenario source under %s: %v", place, err)
	}

	return scanned
}

// anImportBlockOf is the body of a source's import block, one import per line as
// gofmt writes them.
func anImportBlockOf(imported []string) string {
	var block strings.Builder
	for _, written := range imported {
		fmt.Fprintf(&block, "\t%s\n", written)
	}

	return block.String()
}

func TestEveryScenarioSource_ShouldNameOnlyWhatTheLabOffersScenarios(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := namesUsedThatTheLabDoesNotOffer(t, filesOfThePlace(t, place))

	if len(offenders) > 0 {
		t.Errorf("every name a Scenario uses is one internal/lab offers it, so a Scenario reads as one vocabulary under one prefix, but:\n%s",
			strings.Join(offenders, "\n"))
	}
}

// What the name rule reports is read here rather than on the lab's own place,
// which names only what it offers: a name the Step surface does not export, and
// one neither it nor the place declares at all.
func TestTheNameRule_ShouldReportANameNeitherTheStepSurfaceNorThePlaceDeclares(t *testing.T) {
	written := map[string]struct {
		line     string
		admitted bool
	}{
		"the names the Step surface exports": {
			line:     `s.Given().Workload(Checkout, lab.Pods(9), lab.ResilientPlacement())`,
			admitted: true,
		},
		"a specification the Step surface no longer exports": {
			line: `s.Given().Workload(Checkout, lab.Pods(9), lab.MaximumAvailability())`,
		},
		"a name of the Scenario's own": {
			line: `s.Given().Workload(Checkout, lab.Pods(9), aSpecificationOfItsOwn())`,
		},
	}

	for named, taken := range written {
		t.Run(named, func(t *testing.T) {
			offenders := namesUsedThatTheLabDoesNotOffer(t, aScenarioWhoseWorkloadLineIs(t, taken.line))

			expect.Expect(t, len(offenders) == 0).To(expect.Equal(taken.admitted))
		})
	}
}

// The names a Scenario reaching full access writes are read here rather than on
// the lab's own place, which holds no Scenario using full access: the Pod settings
// it names are the one thing a Scenario writes that internal/lab does not offer
// it, and the placeholder the Scenario's own function literal declares is no name
// it writes at all.
func TestTheNameRule_ShouldAdmitTheKubernetesPodSettingsFullAccessNames(t *testing.T) {
	reaching := aScenarioAlsoImporting(t, thePodSettingsImport, theFullAccessLine)

	offenders := namesUsedThatTheLabDoesNotOffer(t, reaching)

	expect.Expect(t, offenders).To(expect.HaveLen[string](0))
}

// namesUsedThatTheLabDoesNotOffer names every name a file of the place uses that
// is neither one internal/lab exports nor one the place declares itself. A name
// taken from a value, such as a Step called on the Scenario, is the compiler's own
// to check and admitted here.
func namesUsedThatTheLabDoesNotOffer(t *testing.T, files []modulerules.GoFile) []string {
	t.Helper()

	offered := theNamesTheStepSurfaceOffers(t)
	ownedByThePlace := theNamesDeclaredIn(files)

	var offenders []string
	for _, file := range files {
		for _, declared := range file.Declarations {
			for _, used := range declared.Names {
				if isOffered(used, offered, ownedByThePlace, declared) {
					continue
				}
				offenders = append(offenders, fmt.Sprintf("  %s, in %s: %s is no name internal/lab offers Scenarios",
					file.Path, declared.Name, writtenAs(used)))
			}
		}
	}

	return offenders
}

// isOffered reports whether one name a declaration uses is one a Scenario may
// write: a name of the Step surface, a name the place declares, a parameter of
// the declaration using it, a name Go itself predeclares, or a name taken from a
// value, whose own type the compiler checks.
func isOffered(
	used modulerules.UsedName,
	offered, ownedByThePlace map[string]bool,
	declared modulerules.Declaration,
) bool {
	if used.Qualifier == theStepSurfaceName {
		return offered[used.Name]
	}
	if used.Qualifier != "" {
		return true
	}

	return ownedByThePlace[used.Name] || isAParameterOf(declared, used.Name) ||
		types.Universe.Lookup(used.Name) != nil
}

// theNamesTheStepSurfaceOffers are the names this package exports, read off its
// own sources, which is what a Scenario may write under the one prefix it uses.
func theNamesTheStepSurfaceOffers(t *testing.T) map[string]bool {
	t.Helper()

	root, err := modulerules.Root(t.Context(), ownPackageDir)
	if err != nil {
		t.Fatalf("locate the root of the module: %v", err)
	}
	sources, err := modulerules.ScanGoFiles(filepath.Join(root, theLabsOwnPackageDir), modulerules.ProductionGoFilesOnly)
	if err != nil {
		t.Fatalf("read the sources of the Step surface: %v", err)
	}

	offered := make(map[string]bool)
	for _, source := range sources {
		for _, declared := range source.Declarations {
			if declared.Exported {
				offered[declared.Name] = true
			}
		}
	}

	return offered
}

// theNamesDeclaredIn are the names the place declares itself: its Scenarios, the
// Workload names they deploy and the list of them.
func theNamesDeclaredIn(files []modulerules.GoFile) map[string]bool {
	declared := make(map[string]bool)
	for _, file := range files {
		for _, held := range file.Declarations {
			declared[held.Name] = true
		}
	}

	return declared
}

func isAParameterOf(declared modulerules.Declaration, name string) bool {
	return slices.ContainsFunc(declared.Parameters, func(parameter modulerules.Parameter) bool {
		return parameter.Name == name
	})
}

// writtenAs is one used name as its source writes it, which is how a report of it
// reads back.
func writtenAs(used modulerules.UsedName) string {
	if used.Qualifier == "" {
		return used.Name
	}

	return used.Qualifier + "." + used.Name
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

// workloadLinesDeclaringNoPredefinedSpecification names every Workload line of
// the place that is not that shape, and every Scenario holding no Workload line
// at all: a Scenario deploys a Workload, so one declaring none would meet the
// shape rule by having nothing to check.
func workloadLinesDeclaringNoPredefinedSpecification(files []modulerules.GoFile) []string {
	var offenders []string
	for _, source := range files {
		for _, declared := range source.Declarations {
			if !isAScenario(declared) {
				continue
			}
			offenders = append(offenders, workloadLinesOfScenarioThatDeclareNoSpecification(source.Path, declared)...)
		}
	}

	return offenders
}

func workloadLinesOfScenarioThatDeclareNoSpecification(path string, declared modulerules.Declaration) []string {
	var offenders []string
	deploying := 0
	for _, line := range declared.Statements {
		if !namesTheWorkloadStep(line) {
			continue
		}
		deploying++
		if !declaresOnePredefinedSpecification(line) {
			offenders = append(offenders, fmt.Sprintf("  %s, in %s: %s declares no predefined Pod specification",
				path, declared.Name, line.Written))
		}
	}
	if deploying == 0 {
		offenders = append(offenders, fmt.Sprintf("  %s, in %s: no Workload is declared at all", path, declared.Name))
	}

	return offenders
}

// namesTheWorkloadStep reports whether a line of a Scenario deploys a Workload,
// wherever in the line that Step is called: a line calling something after it
// deploys one too, and is a line the rule below reports.
func namesTheWorkloadStep(line modulerules.Statement) bool {
	for called := &line; called != nil; called = called.On {
		if called.Call && called.Name == theWorkloadStep {
			return true
		}
	}

	return false
}

// declaresOnePredefinedSpecification reports whether a Workload line declares one
// of the lab's predefined Pod specifications and nothing besides: the line ends at
// the Workload Step, or at one call of the Workload's own closed set written on
// it, and that Step takes the Workload name, the Pod count and that specification,
// itself one predefined call that zero or one Customize is written on.
func declaresOnePredefinedSpecification(line modulerules.Statement) bool {
	deploying, ends := theWorkloadStepTheLineEndsAt(line)
	if !ends || len(deploying.Arguments) != theWorkloadLineArguments {
		return false
	}

	declared := deploying.Arguments[theSpecificationArgument]
	if declared.Call && declared.Name == theCustomize && declared.On != nil {
		declared = *declared.On
	}

	return isAPredefinedSpecification(declared)
}

// theWorkloadStepTheLineEndsAt is the Workload Step a line ends at, directly or
// through one call of the Workload's own closed set written on it, and reports
// whether the line ends at that Step at all.
func theWorkloadStepTheLineEndsAt(line modulerules.Statement) (modulerules.Statement, bool) {
	if line.Name == theWorkloadStep {
		return line, true
	}
	if slices.Contains(theWorkloadsOwnCalls, line.Name) && line.On != nil && line.On.Name == theWorkloadStep {
		return *line.On, true
	}

	return modulerules.Statement{}, false
}

// isAPredefinedSpecification reports whether an expression declares one of the
// lab's predefined specifications: a call taking nothing, named on the Step
// surface and not on a value of the Scenario's own.
func isAPredefinedSpecification(declared modulerules.Statement) bool {
	return declared.Call && len(declared.Arguments) == 0 &&
		declared.On != nil && !declared.On.Call && declared.On.Name == theStepSurfaceName
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
				path, declared.Name, line.Written))
		}
	}

	return offenders
}

// isAStep reports whether a line of a Scenario is one Step: a call on a Given,
// When or Then taken from the Scenario the function was handed.
func isAStep(line modulerules.Statement, scenarioParameter string) bool {
	for called := &line; called != nil; called = called.On {
		if opensTheStep(called, scenarioParameter) {
			return true
		}
	}

	return false
}

func opensTheStep(called *modulerules.Statement, scenarioParameter string) bool {
	return called.Call && called.On != nil && called.On.Name == scenarioParameter &&
		slices.ContainsFunc(theStepKinds, func(kind StepKind) bool {
			return called.Name == string(kind)
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
		for _, identifier := range identifiersIn(line.Written) {
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

// What the import rule admits is read here rather than on the lab's own place,
// where every source imports the Step surface alone: no Scenario of the lab
// reaches full access, so the one admitted exception is exercised on a source of
// this test's own.
func TestTheImportRule_ShouldAdmitThePodSettingsPackageAndNoOtherKubernetesPackage(t *testing.T) {
	written := map[string]struct {
		imported []string
		admitted bool
	}{
		"the Step surface alone": {
			imported: []string{theStepSurfaceImport},
			admitted: true,
		},
		"the Pod settings full access reaches, beside it": {
			imported: []string{theStepSurfaceImport, thePodSettingsImport},
			admitted: true,
		},
		"another Kubernetes package beside it": {
			imported: []string{theStepSurfaceImport, theKubernetesClientImport},
			admitted: false,
		},
	}

	for imports, taken := range written {
		t.Run(imports, func(t *testing.T) {
			var offenders []string
			for _, source := range aScenarioSourceThatImports(t, taken.imported, theWorkloadLine) {
				offenders = append(offenders, machineryImportedBy(source)...)
			}

			expect.Expect(t, len(offenders) == 0).To(expect.Equal(taken.admitted))
		})
	}
}

// machineryImportedBy names every import of a Scenario source that is neither the
// Step surface nor the Pod settings package: another package of the lab, the
// Kubernetes client, or anything else. The Step surface alone is what makes every
// name a Scenario writes one the lab offers it, which the name rule above reads on
// that footing; the Pod settings package is the single stated exception, for a
// Scenario reaching a setting through full access.
func machineryImportedBy(source modulerules.GoFile) []string {
	var offenders []string
	for _, imported := range source.Imports {
		if imported == theStepSurface || imported == thePodSettingsPackage {
			continue
		}
		offenders = append(offenders, fmt.Sprintf("  %s imports %s, which is neither the Step surface nor the Pod settings full access reaches",
			source.Path, imported))
	}

	return offenders
}
