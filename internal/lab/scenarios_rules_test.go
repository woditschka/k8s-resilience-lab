// The two rules in this file govern the place the lab's Scenarios sit in: that
// every declaration there is a Scenario, a Workload name, a Named Node capacity
// or the list, each in the file admitted for it; and that the place holds every Scenario under a name it
// can run, a README naming them, no test file and nothing of the machinery. They
// live with the machinery that defines what a Step is rather than in the
// prominent place itself, so the place stays Scenarios alone
// (docs/architecture-principles.md § Module Boundaries).
package lab

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"strconv"
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

// theNamedNodeCapacities is the file of the place where the lab's own Scenarios
// take the Named Node capacities they name from, the lab defining none. In it,
// every declaration is an exported function taking nothing whose body is one
// return of a call to the Step surface's theNamedNodeCapacityDefinition, the name
// written first as a string literal, so the file holds definitions and no
// machinery, and each name has the one definition a Scenario reaches it by.
const (
	theNamedNodeCapacities         = "nodes.go"
	theNamedNodeCapacityDefinition = "NamedNodeCapacity"
	theReturn                      = "return "
)

// theDomainsName and theDomainsNamedNodeCapacity are the package and the
// constructor the Step surface's definition builds a Named Node capacity
// through, and theLabsSourceRoots the directories under the module root whose
// sources are the lab itself rather than its Scenarios. No source there defines
// a Named Node capacity: only the Step surface's own definition calls the
// domain's constructor.
const (
	theDomainsName              = "scenario"
	theDomainsNamedNodeCapacity = "NewNamedNodeCapacity"
)

var theLabsSourceRoots = []string{"internal", "cmd"}

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

// theScenarioSource and theLabsSource are the names a scratch Scenario source
// and a scratch source of the lab itself are written under.
const (
	theScenarioSource = "shape.go"
	theLabsSource     = "source.go"
)

// theNamedNodeCapacitiesShape is a file of Named Node capacities holding its
// definitions, and theStandardNodesDefinition the one definition the lab's own
// place holds, which theTopologyLine names.
const (
	theNamedNodeCapacitiesShape = `package scenarios

import "github.com/woditschka/k8s-resilience-lab/internal/lab"

%s
`
	theStandardNodesDefinition = `func StandardNodes() lab.DeclaredNodeCapacity {
	return lab.NamedNodeCapacity("Standard", lab.CPU(16), lab.MemoryGi(64), lab.Pods(10))
}`
)

// theScenarioNameShape is how a Scenario's function is named, and
// theOutcomePrefix opens the half of it that states the outcome: the shape
// docs/testing-principles.md § Test Naming gives a test name without its Test
// prefix, so a Scenario reads as what it asserts wherever it is named.
const (
	theScenarioNameShape = "<Condition>_Should<Outcome>"
	theOutcomePrefix     = "Should"
)

// theStepKinds are the four kinds a line of a Scenario may be.
var theStepKinds = []StepKind{GivenStep, WhenStep, WaitStep, ThenStep}

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

// theWaits are the Waits a Scenario writes, under the names the Step surface
// offers them: every Node of the declared Topology Ready, every Pod of the
// declared Workload Ready, and the Placement of that Workload Settled, each
// written with the time it must be reached in or without one.
// theWaitsStatingATime are the three that state it. theWaitOpener is the call
// every way of writing a Wait is written through — on the Scenario itself, and on
// the Given whose declaration the Wait fits — and it is the one opener that is not
// its kind's own word, per
// docs/adr/2026-10-07-a-scenario-writes-its-own-wait.md.
var (
	theWaitsStatingATime = []string{
		"AllNodesReadyWithinSeconds", "AllPodsReadyWithinSeconds", "PlacementSettledWithinSeconds",
	}
	theWaits = slices.Concat(
		[]string{"AllNodesReady", "AllPodsReady", "PlacementSettled"}, theWaitsStatingATime,
	)
)

const theWaitOpener = "WaitFor"

func TestEveryScenarioSource_ShouldHoldOnlyGivenWhenWaitAndThenSteps(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := declarationsThatDoNotBelong(filesOfThePlace(t, place))

	if len(offenders) > 0 {
		t.Errorf("a Scenario source holds one exported Scenario function named %s whose every line is a Given, When, Wait or Then Step, and the three files beside them declare the Workload names, the Named Node capacities and the list alone, but:\n%s",
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

// Every Scenario of the lab runs on one NodePool, and each names it general, so
// the lab's reports read alike and a Scenario about NodePools is the one that
// departs from it (docs/prd.md REQ-SIM-013).
func TestEveryScenarioOfTheLab_ShouldNameItsOneNodePoolGeneral(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := scenariosNotNamingTheirOneNodePoolGeneral(filesOfThePlace(t, place))

	if len(offenders) > 0 {
		t.Errorf("every Scenario of the lab declares one NodePool and names it %s, but:\n%s",
			theLabsNodePoolName, strings.Join(offenders, "\n"))
	}
}

// The rule's own decisions are read here, since the shipped sources all follow it.
func TestTheNodePoolNameRule_ShouldAdmitOneNodePoolNamedGeneralAndReportAnythingElse(t *testing.T) {
	written := map[string]struct {
		line     string
		admitted bool
	}{
		"one NodePool named general": {
			line:     theTopologyLine,
			admitted: true,
		},
		"a NodePool stating a capacity": {
			line: `s.Given().Topology(lab.Zones(3), lab.NodePool("general", lab.NodesPerZone(3), ` +
				`lab.NodeCapacity(lab.CPU(4), lab.MemoryGi(16), lab.Pods(10))))`,
			admitted: true,
		},
		"a NodePool named otherwise": {
			line:     `s.Given().Topology(lab.Zones(3), lab.NodePool("gpu", lab.NodesPerZone(3), lab.StandardNodes()))`,
			admitted: false,
		},
		"two NodePools": {
			line: `s.Given().Topology(lab.Zones(3), lab.NodePool("general", lab.NodesPerZone(3), lab.StandardNodes()), ` +
				`lab.NodePool("general", lab.NodesPerZone(1), lab.StandardNodes()))`,
			admitted: false,
		},
		"no NodePool": {
			line:     `s.Given().Topology(lab.Zones(3))`,
			admitted: false,
		},
	}

	for shape, taken := range written {
		t.Run(shape, func(t *testing.T) {
			offenders := scenariosNotNamingTheirOneNodePoolGeneral(aScenarioWhoseStepsAre(t, taken.line))

			expect.Expect(t, len(offenders) == 0).To(expect.Equal(taken.admitted))
		})
	}
}

// theNodePoolStep is how a NodePool's declaration names itself, and
// theLabsNodePoolName is the name the lab's own Scenarios give their one.
const (
	theNodePoolStep     = "NodePool"
	theLabsNodePoolName = `"general"`
)

// scenariosNotNamingTheirOneNodePoolGeneral names every Scenario of the place that
// declares no NodePool, more than one, or one under another name. The NodePool
// call is found wherever in a line it is written, since it is an argument of the
// Topology line.
func scenariosNotNamingTheirOneNodePoolGeneral(files []modulerules.GoFile) []string {
	var offenders []string
	for _, source := range files {
		for _, declared := range source.Declarations {
			if !isAScenario(declared) {
				continue
			}
			var pools []modulerules.Statement
			for _, line := range declared.Statements {
				pools = append(pools, nodePoolCallsIn(line)...)
			}
			if len(pools) != onlyOnce || len(pools[0].Arguments) == 0 || pools[0].Arguments[0].Written != theLabsNodePoolName {
				offenders = append(offenders, fmt.Sprintf("  %s, in %s: declares %d NodePools, and not one named %s",
					source.Path, declared.Name, len(pools), theLabsNodePoolName))
			}
		}
	}

	return offenders
}

// nodePoolCallsIn is every call to the NodePool declaration inside one expression:
// on the expression it is called on, and in the arguments of every call.
func nodePoolCallsIn(expression modulerules.Statement) []modulerules.Statement {
	var found []modulerules.Statement
	if expression.Call && expression.Name == theNodePoolStep {
		found = append(found, expression)
	}
	if expression.On != nil {
		found = append(found, nodePoolCallsIn(*expression.On)...)
	}
	for _, argument := range expression.Arguments {
		found = append(found, nodePoolCallsIn(argument)...)
	}

	return found
}

// Every NodePool of the lab's Scenarios states its Node capacity or names one the
// place defines, so a reader of the Scenario sees every value its Nodes hold or
// a name whose one definition sits beside it.
func TestEveryScenarioOfTheLab_ShouldStateOrNameEachNodePoolsCapacity(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := nodePoolsNeitherStatingNorNamingTheirCapacity(filesOfThePlace(t, place))

	if len(offenders) > 0 {
		t.Errorf("every NodePool of the lab's Scenarios writes lab.%s(...) or a function %s declares as its capacity, but:\n%s",
			theStatedCapacity, theNamedNodeCapacities, strings.Join(offenders, "\n"))
	}
}

// The rule's own decisions are read here, since the shipped sources write only
// the shapes it admits.
func TestTheNodeCapacityRule_ShouldAdmitAStatedOrNamedCapacityAndReportAnythingElse(t *testing.T) {
	written := map[string]struct {
		line     string
		admitted bool
	}{
		"a NodePool naming a capacity the place defines": {
			line:     theTopologyLine,
			admitted: true,
		},
		"a NodePool stating its capacity": {
			line: `s.Given().Topology(lab.Zones(3), lab.NodePool("general", lab.NodesPerZone(3), ` +
				`lab.NodeCapacity(lab.CPU(4), lab.MemoryGi(16), lab.Pods(10))))`,
			admitted: true,
		},
		"a NodePool naming a capacity of the lab's own": {
			line:     `s.Given().Topology(lab.Zones(3), lab.NodePool("general", lab.NodesPerZone(3), lab.StandardNodes()))`,
			admitted: false,
		},
		"a NodePool naming a capacity the place does not define": {
			line: `s.Given().Topology(lab.Zones(3), lab.NodePool("general", lab.NodesPerZone(3), ` +
				`aCapacityOfItsOwn()))`,
			admitted: false,
		},
		"a NodePool defining its capacity on its own line": {
			line: `s.Given().Topology(lab.Zones(3), lab.NodePool("general", lab.NodesPerZone(3), ` +
				`lab.NamedNodeCapacity("Standard", lab.CPU(16), lab.MemoryGi(64), lab.Pods(10))))`,
			admitted: false,
		},
		"a NodePool writing no capacity": {
			line:     `s.Given().Topology(lab.Zones(3), lab.NodePool("general", lab.NodesPerZone(3)))`,
			admitted: false,
		},
	}

	for shape, taken := range written {
		t.Run(shape, func(t *testing.T) {
			offenders := nodePoolsNeitherStatingNorNamingTheirCapacity(aScenarioBesideTheNamedNodeCapacities(t, taken.line))

			expect.Expect(t, len(offenders) == 0).To(expect.Equal(taken.admitted))
		})
	}
}

// theStatedCapacity is the call the Step surface offers a NodePool stating its
// capacity by, and theCapacityArgument is where the NodePool call takes its
// capacity: after its name and its Node count.
const (
	theStatedCapacity   = "NodeCapacity"
	theCapacityArgument = 2
)

// nodePoolsNeitherStatingNorNamingTheirCapacity names every NodePool of a Scenario
// of the place whose capacity argument is neither a call to lab.NodeCapacity nor
// a call of a function the place's file of Named Node capacities declares, or
// which writes no capacity argument at all.
func nodePoolsNeitherStatingNorNamingTheirCapacity(files []modulerules.GoFile) []string {
	named := theNamedNodeCapacitiesDeclaredIn(files)

	var offenders []string
	for _, source := range files {
		for _, declared := range source.Declarations {
			if !isAScenario(declared) {
				continue
			}
			for _, line := range declared.Statements {
				for _, pool := range nodePoolCallsIn(line) {
					if !statesOrNamesItsCapacity(pool, named) {
						offenders = append(offenders, fmt.Sprintf("  %s, in %s: %s", source.Path, declared.Name, pool.Written))
					}
				}
			}
		}
	}

	return offenders
}

// statesOrNamesItsCapacity reports whether a NodePool call takes its capacity as
// the call the Step surface offers for stating one, or as an unqualified call of
// one of the Named Node capacities the place defines.
func statesOrNamesItsCapacity(pool modulerules.Statement, named map[string]bool) bool {
	if len(pool.Arguments) <= theCapacityArgument {
		return false
	}
	capacity := pool.Arguments[theCapacityArgument]
	if !capacity.Call {
		return false
	}
	if capacity.On == nil {
		return named[capacity.Name]
	}

	return capacity.On.Name == theStepSurfaceName && capacity.Name == theStatedCapacity
}

// theNamedNodeCapacitiesDeclaredIn are the names the place's file of Named Node
// capacities declares, which a NodePool names its capacity by.
func theNamedNodeCapacitiesDeclaredIn(files []modulerules.GoFile) map[string]bool {
	named := make(map[string]bool)
	for _, file := range files {
		if filepath.Base(file.Path) != theNamedNodeCapacities {
			continue
		}
		for _, declared := range file.Declarations {
			named[declared.Name] = true
		}
	}

	return named
}

// Each Named Node capacity the lab's Scenarios name is defined once, in the
// place's file of them, and nowhere else: no Scenario defines one on its own
// line, and no source of the lab itself defines one, the Step surface's own
// definition being the one call of the domain's constructor (docs/prd.md
// REQ-SIM-015).
func TestEveryNamedNodeCapacityOfTheScenarios_ShouldBeDefinedOnceBesideThem(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := slices.Concat(
		namedNodeCapacitiesNotDefinedOnceBesideTheScenarios(filesOfThePlace(t, place)),
		callsDefiningANamedNodeCapacityOfTheLabsOwn(theLabsOwnSources(t)))

	if len(offenders) > 0 {
		t.Errorf("each Named Node capacity is defined once in %s, by an exported function taking nothing that returns one lab.%s call, and the lab defines none, but:\n%s",
			theNamedNodeCapacities, theNamedNodeCapacityDefinition, strings.Join(offenders, "\n"))
	}
}

// The rule's own decisions over the place are read here, since the shipped file
// writes only the shape it admits.
func TestTheNamedNodeCapacitiesRule_ShouldAdmitOneDefinitionPerNameAndReportAnythingElse(t *testing.T) {
	const (
		theStandardDefinition = `func StandardNodes() lab.DeclaredNodeCapacity {
	return lab.NamedNodeCapacity("Standard", lab.CPU(16), lab.MemoryGi(64), lab.Pods(10))
}`
		aSecondDefinitionOfStandard = `func MoreStandardNodes() lab.DeclaredNodeCapacity {
	return lab.NamedNodeCapacity("Standard", lab.CPU(32), lab.MemoryGi(128), lab.Pods(20))
}`
		aDefinitionTakingItsValues = `func SizedNodes(pods lab.Pods) lab.DeclaredNodeCapacity {
	return lab.NamedNodeCapacity("Sized", lab.CPU(16), lab.MemoryGi(64), pods)
}`
		aDefinitionReturningAStatedCapacity = `func StatedNodes() lab.DeclaredNodeCapacity {
	return lab.NodeCapacity(lab.CPU(16), lab.MemoryGi(64), lab.Pods(10))
}`
		aDefinitionUnderAComputedName = `func ComputedNodes() lab.DeclaredNodeCapacity {
	return lab.NamedNodeCapacity(lab.NodeCapacityName("Comp"+"uted"), lab.CPU(16), lab.MemoryGi(64), lab.Pods(10))
}`
		aVariableOfACapacity = `var LargeNodes = lab.NamedNodeCapacity("Large", lab.CPU(32), lab.MemoryGi(128), lab.Pods(10))`
	)

	written := map[string]struct {
		definitions []string
		line        string
		admitted    bool
	}{
		"one definition per name, named by a Scenario": {
			definitions: []string{theStandardDefinition},
			line:        theTopologyLine,
			admitted:    true,
		},
		"two definitions under one name": {
			definitions: []string{theStandardDefinition, aSecondDefinitionOfStandard},
			line:        theTopologyLine,
		},
		"a definition taking a value": {
			definitions: []string{aDefinitionTakingItsValues},
			line:        theTopologyLine,
		},
		"a definition returning a stated capacity": {
			definitions: []string{aDefinitionReturningAStatedCapacity},
			line:        theTopologyLine,
		},
		"a definition under a name no literal writes": {
			definitions: []string{aDefinitionUnderAComputedName},
			line:        theTopologyLine,
		},
		"a variable in place of a function": {
			definitions: []string{aVariableOfACapacity},
			line:        theTopologyLine,
		},
		"a Scenario defining one on its own line": {
			definitions: []string{theStandardDefinition},
			line: `s.Given().Topology(lab.Zones(3), lab.NodePool("general", lab.NodesPerZone(3), ` +
				`lab.NamedNodeCapacity("Large", lab.CPU(32), lab.MemoryGi(128), lab.Pods(10))))`,
		},
	}

	for shape, taken := range written {
		t.Run(shape, func(t *testing.T) {
			place := aPlaceHolding(t, map[string]string{
				theScenarioSource:      aScenarioSourceWhoseStepsAre(taken.line),
				theNamedNodeCapacities: aNamedNodeCapacitiesSourceOf(taken.definitions...),
			})

			offenders := namedNodeCapacitiesNotDefinedOnceBesideTheScenarios(place)

			expect.Expect(t, len(offenders) == 0).To(expect.Equal(taken.admitted))
		})
	}
}

// The rule's own decisions over the lab's sources are read here, since the
// shipped sources hold only the Step surface's own definition.
func TestTheLabDefinesNoNamedNodeCapacityRule_ShouldReportEveryDefinitionButTheStepSurfacesOwn(t *testing.T) {
	written := map[string]struct {
		source   string
		admitted bool
	}{
		"the Step surface's own definition": {
			source: `package lab

func NamedNodeCapacity(name NodeCapacityName, cpu scenario.CPU, memory scenario.Memory, pods Pods) DeclaredNodeCapacity {
	named, err := scenario.NewNamedNodeCapacity(string(name), NodeCapacity(cpu, memory, pods).declared)
	return DeclaredNodeCapacity{declared: named.Capacity(), refused: err}
}
`,
			admitted: true,
		},
		"the Step surface defining a capacity of its own": {
			source: `package lab

func StandardNodes() DeclaredNodeCapacity {
	return NamedNodeCapacity("Standard", CPU(16), MemoryGi(64), Pods(10))
}
`,
		},
		"the Step surface reaching the domain's constructor elsewhere": {
			source: `package lab

func StandardNodes() scenario.NamedNodeCapacity {
	named, _ := scenario.NewNamedNodeCapacity("Standard", scenario.NodeCapacity{})
	return named
}
`,
		},
		"the domain defining a capacity of its own": {
			source: `package scenario

func StandardNodeCapacity() NamedNodeCapacity {
	named, _ := NewNamedNodeCapacity("Standard", NodeCapacity{})
	return named
}
`,
		},
		"the command defining a capacity of its own": {
			source: `package main

func standardNodes() lab.DeclaredNodeCapacity {
	return lab.NamedNodeCapacity("Standard", lab.CPU(16), lab.MemoryGi(64), lab.Pods(10))
}
`,
		},
	}

	for shape, taken := range written {
		t.Run(shape, func(t *testing.T) {
			sources := aPlaceHolding(t, map[string]string{theLabsSource: taken.source})

			offenders := callsDefiningANamedNodeCapacityOfTheLabsOwn(sources)

			expect.Expect(t, len(offenders) == 0).To(expect.Equal(taken.admitted))
		})
	}
}

// namedNodeCapacitiesNotDefinedOnceBesideTheScenarios names every declaration of
// the place's file of Named Node capacities that is not one definition, every
// name defined more than once there, and every other file of the place calling
// the Step surface's definition itself.
func namedNodeCapacitiesNotDefinedOnceBesideTheScenarios(files []modulerules.GoFile) []string {
	var offenders []string
	defined := make(map[string]int)
	for _, file := range files {
		if filepath.Base(file.Path) != theNamedNodeCapacities {
			offenders = append(offenders, definitionsOutsideTheNamedNodeCapacities(file)...)

			continue
		}
		offenders = append(offenders, declarationsThatAreNotNamedNodeCapacities(file)...)
		for _, declared := range file.Declarations {
			if name, isADefinition := theNameDefinedBy(declared); isADefinition {
				defined[name]++
			}
		}
	}
	for name, definitions := range defined {
		if definitions > onlyOnce {
			offenders = append(offenders, fmt.Sprintf("  %s defines the Named Node capacity %q %d times",
				theNamedNodeCapacities, name, definitions))
		}
	}

	return offenders
}

// declarationsThatAreNotNamedNodeCapacities names every declaration of the file
// of Named Node capacities that is not one definition: the file holds the
// capacities the lab's Scenarios name and nothing else.
func declarationsThatAreNotNamedNodeCapacities(source modulerules.GoFile) []string {
	var offenders []string
	for _, declared := range source.Declarations {
		if _, isADefinition := theNameDefinedBy(declared); !isADefinition {
			offenders = append(offenders, fmt.Sprintf("  %s declares the %s %s, which is not an exported function "+
				"taking nothing that returns one lab.%s call under a name it writes",
				source.Path, declared.Kind, declared.Name, theNamedNodeCapacityDefinition))
		}
	}

	return offenders
}

// definitionsOutsideTheNamedNodeCapacities names every declaration of another
// file of the place that calls the Step surface's definition itself.
func definitionsOutsideTheNamedNodeCapacities(source modulerules.GoFile) []string {
	var offenders []string
	for _, declared := range source.Declarations {
		if slices.Contains(declared.Names, theStepSurfacesDefinition) {
			offenders = append(offenders, fmt.Sprintf("  %s, in %s: defines a Named Node capacity outside %s",
				source.Path, declared.Name, theNamedNodeCapacities))
		}
	}

	return offenders
}

// theStepSurfacesDefinition is the definition as a source of the place names it.
var theStepSurfacesDefinition = modulerules.UsedName{
	Qualifier: theStepSurfaceName, Name: theNamedNodeCapacityDefinition,
}

// theNameDefinedBy is the name a declaration of the file of Named Node
// capacities defines, and whether it is one definition at all: an exported
// function taking nothing whose one statement returns a call of the Step
// surface's definition, its first argument the name as a string literal.
func theNameDefinedBy(declared modulerules.Declaration) (string, bool) {
	if declared.Kind != modulerules.FuncDeclaration || !declared.Exported ||
		len(declared.Parameters) != 0 || len(declared.Statements) != onlyOnce {
		return "", false
	}
	returned, isAReturn := strings.CutPrefix(declared.Statements[0].Written, theReturn)
	if !isAReturn {
		return "", false
	}
	parsed, err := parser.ParseExpr(returned)
	if err != nil {
		return "", false
	}
	call, isACall := parsed.(*ast.CallExpr)
	if !isACall || len(call.Args) == 0 || !callsTheStepSurfacesDefinition(call) {
		return "", false
	}
	literal, isALiteral := call.Args[0].(*ast.BasicLit)
	if !isALiteral || literal.Kind != token.STRING {
		return "", false
	}
	name, err := strconv.Unquote(literal.Value)

	return name, err == nil
}

// callsTheStepSurfacesDefinition reports whether call is lab.NamedNodeCapacity.
func callsTheStepSurfacesDefinition(call *ast.CallExpr) bool {
	selected, isSelected := call.Fun.(*ast.SelectorExpr)
	if !isSelected || selected.Sel.Name != theNamedNodeCapacityDefinition {
		return false
	}
	qualifier, isAName := selected.X.(*ast.Ident)

	return isAName && qualifier.Name == theStepSurfaceName
}

// callsDefiningANamedNodeCapacityOfTheLabsOwn names every declaration of the
// lab's own sources that defines a Named Node capacity: one calling the Step
// surface's definition, from inside the Step surface or out of it, and one
// calling the domain's constructor but the Step surface's definition itself.
func callsDefiningANamedNodeCapacityOfTheLabsOwn(files []modulerules.GoFile) []string {
	var offenders []string
	for _, file := range files {
		for _, declared := range file.Declarations {
			for _, used := range declared.Names {
				if definesANamedNodeCapacity(file.Package, declared, used) {
					offenders = append(offenders, fmt.Sprintf("  %s, in %s: %s defines a Named Node capacity of the lab's own",
						file.Path, declared.Name, writtenAs(used)))
				}
			}
		}
	}

	return offenders
}

// definesANamedNodeCapacity reports whether one name a declaration of package
// uses is a call defining a Named Node capacity there.
func definesANamedNodeCapacity(declaredIn string, declared modulerules.Declaration, used modulerules.UsedName) bool {
	switch used.Name {
	case theNamedNodeCapacityDefinition:
		return used.Qualifier == theStepSurfaceName || (used.Qualifier == "" && declaredIn == theStepSurfaceName)
	case theDomainsNamedNodeCapacity:
		isTheStepSurfacesDefinition := declaredIn == theStepSurfaceName &&
			declared.Name == theNamedNodeCapacityDefinition && used.Qualifier == theDomainsName

		return !isTheStepSurfacesDefinition && (used.Qualifier == theDomainsName || declaredIn == theDomainsName)
	default:
		return false
	}
}

// theLabsOwnSources are the production sources of the lab itself, every one under
// theLabsSourceRoots, which is where a Named Node capacity of the lab's own would
// be defined.
func theLabsOwnSources(t *testing.T) []modulerules.GoFile {
	t.Helper()

	root, err := modulerules.Root(t.Context(), ownPackageDir)
	if err != nil {
		t.Fatalf("locate the root of the module: %v", err)
	}

	var sources []modulerules.GoFile
	for _, under := range theLabsSourceRoots {
		scanned, err := modulerules.ScanGoFiles(filepath.Join(root, under), modulerules.ProductionGoFilesOnly)
		if err != nil {
			t.Fatalf("read the lab's own sources under %s: %v", under, err)
		}
		sources = append(sources, scanned...)
	}

	return sources
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
		"a Wait chained onto the Workload Step": {
			line:     `s.Given().Workload(Checkout, lab.Pods(9), lab.ResilientPlacement()).WaitFor().AllPodsReady()`,
			admitted: true,
		},
		"a Wait chained onto the Required Capacity": {
			line: `s.Given().Workload(Checkout, lab.Pods(9), lab.ResilientPlacement()).` +
				`RequiredCapacity(lab.Pods(6)).WaitFor().AllPodsReady()`,
			admitted: true,
		},
		"a Wait written on the Workload Step with no WaitFor opening it": {
			line:     `s.Given().Workload(Checkout, lab.Pods(9), lab.ResilientPlacement()).AllPodsReady()`,
			admitted: false,
		},
		"a Customize written after the Workload Step": {
			line:     `s.Given().Workload(Checkout, lab.Pods(9), lab.ResilientPlacement()).Customize()`,
			admitted: false,
		},
		"a call written after the Workload Step": {
			line:     `s.Given().Workload(Checkout, lab.Pods(9), lab.ResilientPlacement()).NodeCapacity(lab.CPU(4), lab.MemoryGi(16), lab.Pods(2))`,
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
// that Workload line, scanned the way the rules scan the lab's own place.
func aScenarioSourceThatImports(t *testing.T, imported []string, line string) []modulerules.GoFile {
	t.Helper()

	return aPlaceHolding(t, map[string]string{
		theScenarioSource: fmt.Sprintf(theScenarioSourceShape, anImportBlockOf(imported), line),
	})
}

// aScenarioBesideTheNamedNodeCapacities is a Scenario source holding these Steps
// beside a file of Named Node capacities defining StandardNodes, as the lab's own
// place holds them.
func aScenarioBesideTheNamedNodeCapacities(t *testing.T, steps ...string) []modulerules.GoFile {
	t.Helper()

	return aPlaceHolding(t, map[string]string{
		theScenarioSource:      aScenarioSourceWhoseStepsAre(steps...),
		theNamedNodeCapacities: aNamedNodeCapacitiesSourceOf(theStandardNodesDefinition),
	})
}

// aScenarioSourceWhoseStepsAre is the text of a Scenario source holding these
// Steps, one per line, importing the Step surface alone.
func aScenarioSourceWhoseStepsAre(steps ...string) string {
	return fmt.Sprintf(theScenarioSourceShape, anImportBlockOf([]string{theStepSurfaceImport}), strings.Join(steps, "\n\t"))
}

// aNamedNodeCapacitiesSourceOf is the text of a file of Named Node capacities
// holding these definitions, importing the Step surface alone.
func aNamedNodeCapacitiesSourceOf(definitions ...string) string {
	return fmt.Sprintf(theNamedNodeCapacitiesShape, strings.Join(definitions, "\n\n"))
}

// aPlaceHolding is the files named, each holding the source given, scanned the
// way the rules scan the lab's own place. They are written to a directory of
// their own and parsed, because what a rule reads is a parsed source and not a
// string.
func aPlaceHolding(t *testing.T, sources map[string]string) []modulerules.GoFile {
	t.Helper()

	place := t.TempDir()
	for name, source := range sources {
		if err := os.WriteFile(filepath.Join(place, name), []byte(source), theSourceFileMode); err != nil {
			t.Fatalf("write the source %s the rule is read on: %v", name, err)
		}
	}

	scanned, err := modulerules.ScanGoFiles(place, modulerules.ProductionGoFilesOnly)
	if err != nil {
		t.Fatalf("scan the sources under %s: %v", place, err)
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

func TestEveryScenarioOfTheLab_ShouldWaitBeforeItsFirstThenAndAfterEveryFailure(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := stepsReadingAPlacementNoWaitReached(filesOfThePlace(t, place))

	if len(offenders) > 0 {
		t.Errorf("every Then and every Failure of a Scenario has a Wait between it and the Given or the Failure before it, but:\n%s",
			strings.Join(offenders, "\n"))
	}
}

// The Wait after a Failure is where the lab's own subject is measured — how long
// the control plane took to place the Pods it could place again — so every
// Failure Scenario states the time that Wait must be reached in and a run judges
// the restore rather than only reporting it. The Waits above the first Failure
// wait for the world the Failure is injected into and state none (docs/prd.md
// REQ-SIM-006).
func TestEveryFailureScenario_ShouldStateATimeOnTheWaitAfterItsFailure(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := waitsStatingATimeTheirPlaceDoesNotAskFor(filesOfThePlace(t, place))

	if len(offenders) > 0 {
		t.Errorf("the Wait after a Failure states the time it must be reached in, and a Wait above the first Failure states none, but:\n%s",
			strings.Join(offenders, "\n"))
	}
}

// Every line of a Scenario is one Step, so a reader scans the Step kinds in one
// column and inserts, removes or reorders a line on its own. A Wait written on the
// Given before it reads as an expression instead and takes that line's Step kind
// off the column, which is why the lab's own Scenarios write none
// (docs/adr/2026-10-07-the-labs-scenarios-write-one-step-per-line.md).
func TestEveryScenarioOfTheLab_ShouldWriteEachWaitAsAStepOfItsOwn(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := waitsWrittenOnTheGivenBeforeThem(filesOfThePlace(t, place))

	if len(offenders) > 0 {
		t.Errorf("every Wait of a Scenario is a Step of its own, opened on the Scenario, but:\n%s",
			strings.Join(offenders, "\n"))
	}
}

// The rule's own decisions are read here because every Scenario of the lab states
// the time its restore Wait must be reached in, so the place holds no Wait for the
// rule to report.
func TestTheStatedTimeRule_ShouldReportEveryWaitStatingATimeItsPlaceDoesNotAskFor(t *testing.T) {
	written := map[string]struct {
		steps    []string
		admitted bool
	}{
		"a Wait stating a time after the Failure": {
			steps: aWorldDeclaredAndAwaited(
				theThenLine, theFailureLine, theWaitForEveryPodLineStatingATime, theThenLine,
			),
			admitted: true,
		},
		"a Wait stating no time after the Failure": {
			steps: aWorldDeclaredAndAwaited(
				theThenLine, theFailureLine, theWaitForEveryPodLine, theThenLine,
			),
			admitted: false,
		},
		"a Wait stating a time before any Failure": {
			steps: []string{
				theTopologyLine, theWaitForEveryNodeLine,
				theWorkloadLine, theWaitForEveryPodLineStatingATime, theThenLine,
			},
			admitted: false,
		},
	}

	for shape, taken := range written {
		t.Run(shape, func(t *testing.T) {
			offenders := waitsStatingATimeTheirPlaceDoesNotAskFor(aScenarioWhoseStepsAre(t, taken.steps...))

			expect.Expect(t, len(offenders) == 0).To(expect.Equal(taken.admitted))
		})
	}
}

// waitsStatingATimeTheirPlaceDoesNotAskFor names every Wait of the place standing
// after a Failure that states no time it must be reached in, and every Wait above
// the first Failure that states one.
func waitsStatingATimeTheirPlaceDoesNotAskFor(files []modulerules.GoFile) []string {
	var offenders []string
	for _, source := range files {
		for _, declared := range source.Declarations {
			if !isAScenario(declared) {
				continue
			}
			offenders = append(offenders, waitsOfScenarioStatingTheWrongTime(source.Path, declared)...)
		}
	}

	return offenders
}

// waitsOfScenarioStatingTheWrongTime walks one Scenario in the order it is
// written: every Wait below the first Failure is a restore the lab judges against
// a time, and every Wait above it provisions the world that Failure is injected
// into.
func waitsOfScenarioStatingTheWrongTime(path string, declared modulerules.Declaration) []string {
	parameter := declared.Parameters[0].Name

	var offenders []string
	restoring := false
	for _, line := range declared.Statements {
		switch {
		case isAFailure(line, parameter):
			restoring = true
		case isAWait(line):
			offenders = append(offenders, whereAWaitStatesTheWrongTime(restoring, path, declared, line)...)
		}
	}

	return offenders
}

// whereAWaitStatesTheWrongTime is the one offender such a Wait names, and nothing
// where what it states is what its place in the Scenario asks for.
func whereAWaitStatesTheWrongTime(
	restoring bool,
	path string,
	declared modulerules.Declaration,
	line modulerules.Statement,
) []string {
	if statesATime(line) == restoring {
		return nil
	}

	named := "stands before any Failure and states a time it must be reached in"
	if restoring {
		named = "stands after a Failure and states no time it must be reached in"
	}

	return []string{fmt.Sprintf("  %s, in %s: %s %s", path, declared.Name, line.Written, named)}
}

// statesATime reports whether a Wait line states the time it must be reached in,
// which is the name that Wait is written under.
func statesATime(line modulerules.Statement) bool {
	return slices.Contains(theWaitsStatingATime, line.Name)
}

// The Steps a Scenario is read as writing its Waits in order, each form of a Wait
// and each Step that reads what one reached. The rule's own decisions are read
// here because every Scenario of the lab Waits everywhere, so the place holds no
// Step for the rule to report.
func TestTheWaitOrderingRule_ShouldReportEveryThenAndFailureNoWaitStandsBefore(t *testing.T) {
	written := map[string]struct {
		steps    []string
		admitted bool
	}{
		"a Wait chained onto each Given": {
			steps:    []string{theTopologyWaitingLine, theWorkloadWaitingLine, theThenLine},
			admitted: true,
		},
		"a Wait of its own before the first Then": {
			steps:    []string{theTopologyLine, theWorkloadLine, theWaitForEveryPodLine, theThenLine},
			admitted: true,
		},
		"no Wait before the first Then": {
			steps:    []string{theTopologyLine, theWorkloadLine, theThenLine},
			admitted: false,
		},
		"two Waits in a row": {
			steps: []string{
				theTopologyLine, theWorkloadLine,
				theWaitForEveryNodeLine, theWaitForEveryPodLine, theThenLine,
			},
			admitted: true,
		},
		"a Wait between the Failure and the Then after it": {
			steps: aWorldDeclaredAndAwaited(
				theThenLine, theFailureLine, theWaitForEveryPodLine, theThenLine,
			),
			admitted: true,
		},
		"no Wait between the Failure and the Then after it": {
			steps:    aWorldDeclaredAndAwaited(theThenLine, theFailureLine, theThenLine),
			admitted: false,
		},
		"no Wait before the Failure": {
			steps:    []string{theTopologyLine, theWorkloadLine, theFailureLine, theWaitForEveryPodLine, theThenLine},
			admitted: false,
		},
	}

	for shape, taken := range written {
		t.Run(shape, func(t *testing.T) {
			offenders := stepsReadingAPlacementNoWaitReached(aScenarioWhoseStepsAre(t, taken.steps...))

			expect.Expect(t, len(offenders) == 0).To(expect.Equal(taken.admitted))
		})
	}
}

// The Step lines the rules above are read on: each Given on its own and with the
// Wait that fits it chained onto its line, each Wait written as a Step of its own,
// the Failure that discards the Placement, and a Then that judges one.
const (
	theTopologyLine         = `s.Given().Topology(lab.Zones(3), lab.NodePool("general", lab.NodesPerZone(3), StandardNodes()))`
	theTopologyWaitingLine  = theTopologyLine + `.WaitFor().AllNodesReady()`
	theWorkloadWaitingLine  = theWorkloadLine + `.WaitFor().AllPodsReady()`
	theWaitForEveryNodeLine = `s.WaitFor().AllNodesReady()`
	theWaitForEveryPodLine  = `s.WaitFor().AllPodsReady()`

	// The Wait for every Pod stating the time it must be reached in, which is how a
	// Wait after a Failure is written.
	theWaitForEveryPodLineStatingATime = `s.WaitFor().AllPodsReadyWithinSeconds(30)`
	theFailureLine                     = `s.When().NodesFail(lab.Nodes(1))`
	theThenLine                        = `s.Then().PodsPerZone(3)`
)

// aWorldDeclaredAndAwaited are the Steps a Scenario of the lab opens with, one
// Step per line: each Given with the Wait that applies it standing under it,
// followed by the Steps the rule under test is read on.
func aWorldDeclaredAndAwaited(followedBy ...string) []string {
	opened := []string{theTopologyLine, theWaitForEveryNodeLine, theWorkloadLine, theWaitForEveryPodLine}

	return append(opened, followedBy...)
}

// aScenarioWhoseStepsAre is a Scenario source holding these Steps, one per line
// as a Scenario writes them, importing the Step surface alone.
func aScenarioWhoseStepsAre(t *testing.T, steps ...string) []modulerules.GoFile {
	t.Helper()

	return aScenarioSourceThatImports(t, []string{theStepSurfaceImport}, strings.Join(steps, "\n\t"))
}

// stepsReadingAPlacementNoWaitReached names every Step of the place that reads the
// Placement a Wait reached where none stands before it: a Then, or a Failure When,
// with no Wait since the Givens or since the Failure before it. Such a Step ends
// the Scenario when it runs, so the rule reads the order off the source instead of
// leaving it to a run (docs/adr/2026-10-07-a-scenario-writes-its-own-wait.md).
func stepsReadingAPlacementNoWaitReached(files []modulerules.GoFile) []string {
	var offenders []string
	for _, source := range files {
		for _, declared := range source.Declarations {
			if !isAScenario(declared) {
				continue
			}
			offenders = append(offenders, stepsOfScenarioNoWaitStandsBefore(source.Path, declared)...)
		}
	}

	return offenders
}

// stepsOfScenarioNoWaitStandsBefore walks one Scenario in the order it is written:
// a Wait reaches a Placement, a Then judges the one reached last, and a Failure
// reads it to count the Pods kept Running and then discards it, so what a Wait
// reached holds for the Thens after it and no further.
func stepsOfScenarioNoWaitStandsBefore(path string, declared modulerules.Declaration) []string {
	parameter := declared.Parameters[0].Name

	var offenders []string
	waited := false
	for _, line := range declared.Statements {
		switch {
		case isAWait(line):
			waited = true
		case isAFailure(line, parameter):
			offenders = append(offenders, whereNoWaitStands(waited, path, declared, line)...)
			waited = false
		case isAThen(line, parameter):
			offenders = append(offenders, whereNoWaitStands(waited, path, declared, line)...)
		}
	}

	return offenders
}

// whereNoWaitStands is the one offender a Step reading a Placement names where no
// Wait stands before it, and nothing where one does.
func whereNoWaitStands(
	waited bool,
	path string,
	declared modulerules.Declaration,
	line modulerules.Statement,
) []string {
	if waited {
		return nil
	}

	return []string{fmt.Sprintf("  %s, in %s: %s reads the Placement a Wait reached, and %s",
		path, declared.Name, line.Written, theWaitThatNeverStood)}
}

// isAWait reports whether a line of a Scenario is a Wait: a line whose outermost
// call is one of the two Waits, which both ways of writing one end at — the
// WaitFor of a Given's own line, and a Wait Step of the Scenario's.
func isAWait(line modulerules.Statement) bool {
	return line.Call && slices.Contains(theWaits, line.Name)
}

// isAFailure reports whether a line is a Failure: a When Step, which is the one
// thing a When does to the cluster now that a Wait opens on its own call.
func isAFailure(line modulerules.Statement, scenarioParameter string) bool {
	kind, opened := theStepKindOf(line, scenarioParameter)

	return opened && kind == WhenStep
}

func isAThen(line modulerules.Statement, scenarioParameter string) bool {
	kind, opened := theStepKindOf(line, scenarioParameter)

	return opened && kind == ThenStep
}

// The two forms of writing a Wait the layout rule is read on: a Wait opened on the
// Scenario, and the same Wait written on the Given whose declaration it fits. The
// chained form stays offered to an engineer writing a Scenario, so what the rule
// reports is read here rather than on the lab's own place, which writes it nowhere.
func TestTheWaitLayoutRule_ShouldReportEveryWaitWrittenOnTheGivenBeforeIt(t *testing.T) {
	written := map[string]struct {
		steps    []string
		admitted bool
	}{
		"each Wait a Step of its own": {
			steps:    aWorldDeclaredAndAwaited(theThenLine),
			admitted: true,
		},
		"a Wait written on the Topology it waits for": {
			steps: []string{theTopologyWaitingLine, theWorkloadLine, theWaitForEveryPodLine, theThenLine},
		},
		"a Wait written on the Workload it waits for": {
			steps: []string{theTopologyLine, theWaitForEveryNodeLine, theWorkloadWaitingLine, theThenLine},
		},
	}

	for shape, taken := range written {
		t.Run(shape, func(t *testing.T) {
			offenders := waitsWrittenOnTheGivenBeforeThem(aScenarioWhoseStepsAre(t, taken.steps...))

			expect.Expect(t, len(offenders) == 0).To(expect.Equal(taken.admitted))
		})
	}
}

// waitsWrittenOnTheGivenBeforeThem names every Wait of the place that is no Step of
// its own: one opened on the Given whose declaration it fits rather than on the
// Scenario.
func waitsWrittenOnTheGivenBeforeThem(files []modulerules.GoFile) []string {
	var offenders []string
	for _, source := range files {
		for _, declared := range source.Declarations {
			if !isAScenario(declared) {
				continue
			}
			offenders = append(offenders, waitsOfScenarioRidingTheirGiven(source.Path, declared)...)
		}
	}

	return offenders
}

// waitsOfScenarioRidingTheirGiven names every Wait of one Scenario whose line is of
// another kind, which is a Wait riding the Given it was written on: that line is
// the Given's Step, and the Wait is no Step of the Scenario's own.
func waitsOfScenarioRidingTheirGiven(path string, declared modulerules.Declaration) []string {
	parameter := declared.Parameters[0].Name

	var offenders []string
	for _, line := range declared.Statements {
		if !isAWait(line) || isAWaitStep(line, parameter) {
			continue
		}
		offenders = append(offenders, fmt.Sprintf("  %s, in %s: %s is written on the Given before it rather than as a Step of its own",
			path, declared.Name, line.Written))
	}

	return offenders
}

// isAWaitStep reports whether a line is a Wait Step: a Wait opened on the Scenario
// itself, which is the kind the line is then read as.
func isAWaitStep(line modulerules.Statement, scenarioParameter string) bool {
	kind, opened := theStepKindOf(line, scenarioParameter)

	return opened && kind == WaitStep
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
	case theWorkloadNames, theNamedNodeCapacities, theList:
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
		case theNamedNodeCapacities:
			offenders = append(offenders, declarationsThatAreNotNamedNodeCapacities(file)...)
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
// whether the line ends at that Step at all. The Wait a Scenario chains onto the
// line is read past first: it is a Step of its own and no part of what the Workload
// declares, so the shape below is read on the declaration it was written after.
func theWorkloadStepTheLineEndsAt(line modulerules.Statement) (modulerules.Statement, bool) {
	line = whatTheChainedWaitWasWrittenOn(line)

	if line.Name == theWorkloadStep {
		return line, true
	}
	if slices.Contains(theWorkloadsOwnCalls, line.Name) && line.On != nil && line.On.Name == theWorkloadStep {
		return *line.On, true
	}

	return modulerules.Statement{}, false
}

// whatTheChainedWaitWasWrittenOn is the declaration a line ends at once the Wait
// chained onto it through its WaitFor is read past, and the line itself where it
// ends at no such Wait. A Wait named directly on a declaration is not one: the
// WaitFor is what opens it, so such a line keeps the shape it was written in and is
// reported.
func whatTheChainedWaitWasWrittenOn(line modulerules.Statement) modulerules.Statement {
	if !isAWait(line) || line.On == nil || !line.On.Call || line.On.Name != theWaitOpener || line.On.On == nil {
		return line
	}

	return *line.On.On
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
// Given, When, Wait or Then Step called on the Scenario the function took.
func linesOfScenarioThatAreNotSteps(path string, declared modulerules.Declaration) []string {
	var offenders []string
	for _, line := range declared.Statements {
		if !isAStep(line, declared.Parameters[0].Name) {
			offenders = append(offenders, fmt.Sprintf("  %s, in %s: %s is not a Given, When, Wait or Then Step",
				path, declared.Name, line.Written))
		}
	}

	return offenders
}

// isAStep reports whether a line of a Scenario is one Step: a call on a Given,
// When, WaitFor or Then taken from the Scenario the function was handed.
func isAStep(line modulerules.Statement, scenarioParameter string) bool {
	_, opened := theStepKindOf(line, scenarioParameter)

	return opened
}

// theStepKindOf is the kind of Step a line of a Scenario is, read off the opener it
// was written through, and reports whether the line is a Step at all. The Wait a
// Given's own line chains is opened on that declaration and not on the Scenario, so
// it is the Given the kind is read off here and the Wait's own Step the Scenario
// records — which is why the ordering rule reads a Wait off its outermost call
// instead.
func theStepKindOf(line modulerules.Statement, scenarioParameter string) (StepKind, bool) {
	for called := &line; called != nil; called = called.On {
		if kind, opened := opensTheStep(called, scenarioParameter); opened {
			return kind, true
		}
	}

	return "", false
}

// opensTheStep is the kind of Step a call written on the Scenario opens, and
// reports whether it opens one at all.
func opensTheStep(called *modulerules.Statement, scenarioParameter string) (StepKind, bool) {
	if !called.Call || called.On == nil || called.On.Name != scenarioParameter {
		return "", false
	}

	opened := slices.IndexFunc(theStepKinds, func(kind StepKind) bool {
		return called.Name == theOpenerOf(kind)
	})
	if opened < 0 {
		return "", false
	}

	return theStepKinds[opened], true
}

// theOpenerOf is the call on the Scenario a Step of this kind is written through:
// the kind's own word for three of the four, and WaitFor for a Wait, so that one
// call carries both ways of writing a Wait.
func theOpenerOf(kind StepKind) string {
	if kind == WaitStep {
		return theWaitOpener
	}

	return string(kind)
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
