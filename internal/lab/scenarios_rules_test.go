// These rules govern the Scenarios' place. They live with the machinery rather
// than in the place itself, so the place stays Scenarios alone.
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

const ownPackageDir = "."

const scenariosDirectory = "scenarios"

const (
	theReadme         = "README.md"
	theTestFileSuffix = "_test.go"
)

// In theWorkloadNames every declaration is an exported constant of the
// Workload-name type, so the file holds names and no machinery.
const (
	theWorkloadNames    = "workloads.go"
	theWorkloadNameType = "lab.WorkloadName"
)

// In theNodeLabelKeys every declaration is an exported untyped string constant,
// so the file holds keys and no machinery.
const theNodeLabelKeys = "labels.go"

// theTaints holds exported untyped string constants alone, as theNodeLabelKeys
// does.
const theTaints = "taints.go"

// In theNamedNodeCapacities every declaration is an exported function taking
// nothing that returns one call of theNamedNodeCapacityDefinition, the name
// first as a string literal, so each name has the one definition a Scenario
// reaches it by.
const (
	theNamedNodeCapacities         = "nodes.go"
	theNamedNodeCapacityDefinition = "Named"
	theReturn                      = "return "
)

// No source under theLabsSourceRoots defines a Named Node capacity: only
// theNamedCapacitysDomainValue calls the domain's constructor.
const (
	theDomainsName               = "scenario"
	theDomainsNamedNodeCapacity  = "NewNamedNodeCapacity"
	theNamedCapacitysDomainValue = "declared"
)

var theLabsSourceRoots = []string{"internal", "cmd"}

// In thePodSpecifications every declaration is an exported function taking
// nothing that returns one call of thePodSpecificationDefinition, the name
// first as a string literal and the Startup time second, or one of
// theNamedSettings. No other source of the lab calls
// theDomainsPodSpecification.
const (
	thePodSpecifications          = "pods.go"
	thePodSpecificationDefinition = "Predefined"
	theDomainsPodSpecification    = "NewPodSpecification"
	theStartupTimeArgument        = 1
	theFirstSettingArgument       = 2
	theNamedSetting               = "LeaveUnresponsiveNodes"
	theGroupingMeans              = "Settings"
	theTolerationSetting          = "Toleration"
	theTolerationsKey             = "Key"
	theTolerationFields           = 4
)

const (
	theEvenSpread          = "EvenSpreadOver"
	theSpreadSetting       = "TopologySpreadConstraint"
	theSpreadFields        = 4
	theEvenSkew            = "1"
	theResilientPlacement  = "ResilientPlacement"
	theSpreadSubjectOfZone = "Zone"
	theSpreadSubjectOfNode = "Hostname"
)

// theLabsTaints are the two taints the Named setting tolerates at the lab's
// waiting time, and that no definition writes a toleration of itself.
var (
	theNamedSettings  = []string{theNamedSetting, theEvenSpread}
	theSpreadSubjects = []string{theSpreadSubjectOfZone, theSpreadSubjectOfNode}
	theLabsTaints     = []string{"NotReady", "Unreachable"}
	theStartupForms   = []string{"StartupSeconds", "StartupSecondsBetween", "StartupMillis", "StartupMillisBetween"}
)

// The list names each Scenario onlyOnce: naming none leaves a Scenario unrun,
// and naming one twice runs it twice.
const (
	theList         = "scenarios.go"
	theListFunction = "All"
	onlyOnce        = 1
)

const (
	theStepSurface = "github.com/woditschka/k8s-resilience-lab/internal/lab"
	thePodPackage  = theStepSurface + "/pod"
	theNodePackage = theStepSurface + "/node"
)

// A source reaching the surface under another name is reported as holding no
// Scenario, which is the same finding by a different route.
const (
	theStepSurfaceName   = "lab"
	thePodName           = "pod"
	theNodeName          = "node"
	theScenarioParameter = "*" + theStepSurfaceName + ".Scenario"
)

var theSubjects = []string{theStepSurfaceName, thePodName, theNodeName}

const theLabsOwnPackageDir = "internal/lab"

// thePodSettingsPackage is the single exception to the one-vocabulary rule, for
// a Scenario reaching a Pod setting through full access. The depguard rule
// scenarios-hold-steps-alone and the import rule below carry it together.
const (
	thePodSettingsPackage = "k8s.io/api/core/v1"
	thePodSettingsName    = "corev1"
)

const (
	theStepSurfaceImport      = `"` + theStepSurface + `"`
	thePodImport              = `"` + thePodPackage + `"`
	theNodeImport             = `"` + theNodePackage + `"`
	thePodSettingsImport      = thePodSettingsName + ` "` + thePodSettingsPackage + `"`
	theKubernetesClientImport = `"k8s.io/client-go/kubernetes"`
	theDomainImport           = `"github.com/woditschka/k8s-resilience-lab/internal/scenario"`
)

var theScenarioImports = []string{theStepSurfaceImport, theNodeImport, thePodImport}

// theScenarioSourceShape is written as a Scenario of the lab is written, so a
// rule reaches its lines the same way.
const theScenarioSourceShape = `package scenarios

import (
%s)

const Checkout lab.WorkloadName = "checkout"

func AWorkload_ShouldBeDeclared(s *lab.Scenario) {
	%s
}
`

const theSourceFileMode = 0o600

const (
	theScenarioSource = "shape.go"
	theLabsSource     = "source.go"
)

const (
	aFileOfDefinitionsShape = `package scenarios

import (
	"github.com/woditschka/k8s-resilience-lab/internal/lab/node"
	"github.com/woditschka/k8s-resilience-lab/internal/lab/pod"
)

%s
`
	theStandardNodesDefinition = `func StandardNodes() node.Capacity {
	return node.Named("Standard", node.Capacity{CPU: 16, Memory: 64 * node.Gi, Pods: 10})
}`
)

const (
	theNamedSettingWritten          = `LeaveUnresponsiveNodes()`
	theResilientPlacementDefinition = `func ResilientPlacement() pod.Specification {
	return pod.Predefined("Resilient placement", pod.StartupMillisBetween(500, 1000),
		` + theNamedSettingWritten + `,
		EvenSpreadOver(node.Zone),
		EvenSpreadOver(node.Hostname),
	)
}`
)

const (
	theNamedSettingDefinition = `func LeaveUnresponsiveNodes() pod.Customization {
	return pod.Settings(
		pod.Toleration{Key: node.NotReady, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: node.UnresponsiveWait},
		pod.Toleration{Key: node.Unreachable, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: node.UnresponsiveWait},
	)
}`
	theEvenSpreadDefinition = `func EvenSpreadOver(topologyKey string) pod.TopologySpreadConstraint {
	return pod.TopologySpreadConstraint{
		TopologyKey: topologyKey, MaxSkew: 1, WhenUnsatisfiable: pod.DoNotSchedule, NodeTaintsPolicy: pod.Honor,
	}
}`
)

// A Scenario is named the way a test is, without its Test prefix, so it reads
// as what it asserts wherever it is named.
const (
	theScenarioNameShape = "<Condition>_Should<Outcome>"
	theOutcomePrefix     = "Should"
)

var theStepKinds = []StepKind{GivenStep, WhenStep, WaitStep, ThenStep}

// A Workload line takes the Workload name, the Pod count and one predefined Pod
// specification, customized at most once. A setting of the Scenario's own would
// be a further argument or call, and neither is admitted.
//
// The shape is read off the syntax tree rather than the text, because a
// Customization wraps over several lines once gofmt has had it.
const (
	theWorkloadStep = "Workload"
	theCustomize    = "Customize"

	theWorkloadLineArguments = 3
	theSpecificationArgument = 2
)

// A second call of theWorkloadsOwnCalls, or any other call written there, stays
// refused.
var theWorkloadsOwnCalls = []string{"RequiredCapacity"}

// theWaitOpener is the one opener that is not its kind's own word, and every
// way of writing a Wait goes through it.
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
		t.Errorf("%s holds the list and exported Scenario functions named %s whose every line is a Given, When, Wait or Then Step, and the files beside it declare the Workload names, the Node label keys, the taints, the Named Node capacities and the predefined Pod specifications alone, but:\n%s",
			theList,
			theScenarioNameShape,
			strings.Join(offenders, "\n"))
	}
}

func TestEveryScenarioOfTheLab_ShouldDeclareOnePredefinedPodSpecificationPerWorkload(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := workloadLinesDeclaringNoPredefinedSpecification(filesOfThePlace(t, place))

	if len(offenders) > 0 {
		t.Errorf("every Workload line of a Scenario declares one predefined Pod specification of %s and no setting of its own, but:\n%s",
			thePodSpecifications,
			strings.Join(offenders, "\n"))
	}
}

// The rule tests below read scratch sources, since the shipped sources follow
// each rule and write only the shapes it admits. A test that reads them for a
// reason of its own states it.

func TestTheNodeLabelKeysRule_ShouldAdmitExportedUntypedStringConstantsAndReportAnythingElse(t *testing.T) {
	written := map[string]struct {
		declarations string
		admitted     bool
	}{
		"an exported untyped string constant": {declarations: `const CapacityType = "capacity-type"`, admitted: true},
		"a group of them": {
			declarations: "const (\n\tCapacityType = \"capacity-type\"\n\tRack = \"rack\"\n)", admitted: true,
		},
		"a typed constant":       {declarations: `const CapacityType string = "capacity-type"`},
		"an unexported constant": {declarations: `const capacityType = "capacity-type"`},
		"a constant of a number": {declarations: `const Racks = 3`},
		"a constant of no value": {declarations: "const (\n\tCapacityType = \"capacity-type\"\n\tRack\n)"},
		"a variable":             {declarations: `var CapacityType = "capacity-type"`},
		"a function":             {declarations: `func CapacityType() string { return "capacity-type" }`},
		"a computed constant":    {declarations: `const CapacityType = "capacity" + "-type"`},
	}

	for _, file := range []string{theNodeLabelKeys, theTaints} {
		for shape, taken := range written {
			t.Run(file+" "+shape, func(t *testing.T) {
				place := aPlaceHolding(t, map[string]string{file: "package scenarios\n\n" + taken.declarations + "\n"})

				offenders := declarationsThatDoNotBelong(place)

				expect.Expect(t, len(offenders) == 0).To(expect.Equal(taken.admitted))
			})
		}
	}
}

// A single NodePool is named general so the lab's reports read alike; a
// Scenario declaring several names each after what it holds.
func TestEveryScenarioOfTheLab_ShouldDeclareANodePoolAndNameItGeneralWhenItDeclaresOnlyOne(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := scenariosDeclaringNoNodePoolOrOneNotNamedGeneral(filesOfThePlace(t, place))

	if len(offenders) > 0 {
		t.Errorf("every Scenario of the lab declares a NodePool, and names its one NodePool %s, but:\n%s",
			theLabsNodePoolName, strings.Join(offenders, "\n"))
	}
}

func TestTheNodePoolNameRule_ShouldAdmitSeveralNodePoolsOrOneNamedGeneralAndReportNoneOrOneNamedOtherwise(t *testing.T) {
	written := map[string]struct {
		line     string
		admitted bool
	}{
		"one NodePool named general": {
			line:     theTopologyLine,
			admitted: true,
		},
		"a NodePool stating a capacity": {
			line: `s.Given().Topology(lab.Zones(3), node.Pool("general", node.PerZone(3), ` +
				`node.Capacity{CPU: 4, Memory: 16 * node.Gi, Pods: 10}))`,
			admitted: true,
		},
		"a NodePool named otherwise": {
			line:     `s.Given().Topology(lab.Zones(3), node.Pool("gpu", node.PerZone(3), StandardNodes()))`,
			admitted: false,
		},
		"two NodePools, each named after what it holds": {
			line: `s.Given().Topology(lab.Zones(3), node.Pool("on-demand", node.PerZone(1), StandardNodes()), ` +
				`node.Pool("spot", node.PerZone(1), StandardNodes()))`,
			admitted: true,
		},
		"a labelled NodePool named general": {
			line: `s.Given().Topology(lab.Zones(3), node.Pool("general", node.PerZone(3), StandardNodes()).` +
				`Labels(node.Labels{"capacity-type": "spot"}))`,
			admitted: true,
		},
		"no NodePool": {
			line:     `s.Given().Topology(lab.Zones(3))`,
			admitted: false,
		},
	}

	for shape, taken := range written {
		t.Run(shape, func(t *testing.T) {
			offenders := scenariosDeclaringNoNodePoolOrOneNotNamedGeneral(aScenarioWhoseStepsAre(t, taken.line))

			expect.Expect(t, len(offenders) == 0).To(expect.Equal(taken.admitted))
		})
	}
}

const (
	theNodePoolStep     = "Pool"
	theLabsNodePoolName = `"general"`
)

// The NodePool call is found anywhere in a line, since it is an argument of the
// Topology line.
func scenariosDeclaringNoNodePoolOrOneNotNamedGeneral(files []modulerules.GoFile) []string {
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
			if len(pools) == 0 || len(pools) == onlyOnce && !namesTheLabsNodePool(pools[0]) {
				offenders = append(offenders, fmt.Sprintf("  %s, in %s: declares %d NodePools, and not one named %s",
					source.Path, declared.Name, len(pools), theLabsNodePoolName))
			}
		}
	}

	return offenders
}

func namesTheLabsNodePool(pool modulerules.Statement) bool {
	return len(pool.Arguments) > 0 && pool.Arguments[0].Written == theLabsNodePoolName
}

func nodePoolCallsIn(expression modulerules.Statement) []modulerules.Statement {
	var found []modulerules.Statement
	if expression.Call && expression.Name == theNodePoolStep && expression.On != nil && expression.On.Name == theNodeName {
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

// A reader of the Scenario then sees every value its Nodes hold, or a name
// whose one definition sits beside it.
func TestEveryScenarioOfTheLab_ShouldStateOrNameEachNodePoolsCapacity(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := nodePoolsNeitherStatingNorNamingTheirCapacity(filesOfThePlace(t, place))

	if len(offenders) > 0 {
		t.Errorf("every NodePool of the lab's Scenarios writes %s...} or a function %s declares as its capacity, but:\n%s",
			theStatedCapacity, theNamedNodeCapacities, strings.Join(offenders, "\n"))
	}
}

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
			line: `s.Given().Topology(lab.Zones(3), node.Pool("general", node.PerZone(3), ` +
				`node.Capacity{CPU: 4, Memory: 16 * node.Gi, Pods: 10}))`,
			admitted: true,
		},
		"a NodePool stating its capacity as a value of another type": {
			line: `s.Given().Topology(lab.Zones(3), node.Pool("general", node.PerZone(3), ` +
				`pod.Request{CPU: 4, Memory: 16 * pod.Gi}))`,
			admitted: false,
		},
		"a NodePool naming a capacity of the lab's own": {
			line:     `s.Given().Topology(lab.Zones(3), node.Pool("general", node.PerZone(3), node.StandardNodes()))`,
			admitted: false,
		},
		"a NodePool naming a capacity the place does not define": {
			line: `s.Given().Topology(lab.Zones(3), node.Pool("general", node.PerZone(3), ` +
				`aCapacityOfItsOwn()))`,
			admitted: false,
		},
		"a NodePool defining its capacity on its own line": {
			line: `s.Given().Topology(lab.Zones(3), node.Pool("general", node.PerZone(3), ` +
				`node.Named("Standard", node.Capacity{CPU: 16, Memory: 64 * node.Gi, Pods: 10})))`,
			admitted: false,
		},
		"a NodePool writing no capacity": {
			line:     `s.Given().Topology(lab.Zones(3), node.Pool("general", node.PerZone(3)))`,
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

// theCapacityArgument follows the NodePool's name and Node count.
const (
	theStatedCapacity   = theNodeName + ".Capacity{"
	theCapacityArgument = 2
)

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

func statesOrNamesItsCapacity(pool modulerules.Statement, named map[string]bool) bool {
	if len(pool.Arguments) <= theCapacityArgument {
		return false
	}
	capacity := pool.Arguments[theCapacityArgument]
	if strings.HasPrefix(capacity.Written, theStatedCapacity) {
		return true
	}

	return capacity.Call && capacity.On == nil && named[capacity.Name]
}

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

// The node package's own building of a named capacity is the one call of the
// domain's constructor.
func TestEveryNamedNodeCapacityOfTheScenarios_ShouldBeDefinedOnceBesideThem(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := slices.Concat(
		namedNodeCapacitiesNotDefinedOnceBesideTheScenarios(filesOfThePlace(t, place)),
		callsDefiningANamedNodeCapacityOfTheLabsOwn(theLabsOwnSources(t)))

	if len(offenders) > 0 {
		t.Errorf("each Named Node capacity is defined once in %s, by an exported function taking nothing that returns one node.%s call, and the lab defines none, but:\n%s",
			theNamedNodeCapacities, theNamedNodeCapacityDefinition, strings.Join(offenders, "\n"))
	}
}

func TestTheNamedNodeCapacitiesRule_ShouldAdmitOneDefinitionPerNameAndReportAnythingElse(t *testing.T) {
	const (
		theStandardDefinition       = theStandardNodesDefinition
		aSecondDefinitionOfStandard = `func MoreStandardNodes() node.Capacity {
	return node.Named("Standard", node.Capacity{CPU: 32, Memory: 128 * node.Gi, Pods: 20})
}`
		aDefinitionTakingItsValues = `func SizedNodes(pods int) node.Capacity {
	return node.Named("Sized", node.Capacity{CPU: 16, Memory: 64 * node.Gi, Pods: pods})
}`
		aDefinitionReturningAStatedCapacity = `func StatedNodes() node.Capacity {
	return node.Capacity{CPU: 16, Memory: 64 * node.Gi, Pods: 10}
}`
		aDefinitionUnderAComputedName = `func ComputedNodes() node.Capacity {
	return node.Named(node.CapacityName("Comp"+"uted"), node.Capacity{CPU: 16, Memory: 64 * node.Gi, Pods: 10})
}`
		aVariableOfACapacity = `var LargeNodes = node.Named("Large", node.Capacity{CPU: 32, Memory: 128 * node.Gi, Pods: 10})`
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
			line: `s.Given().Topology(lab.Zones(3), node.Pool("general", node.PerZone(3), ` +
				`node.Named("Large", node.Capacity{CPU: 32, Memory: 128 * node.Gi, Pods: 10})))`,
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

// Read on scratch sources, since the shipped sources hold only the node
// package's own building of a named capacity.
func TestTheLabDefinesNoNamedNodeCapacityRule_ShouldReportEveryDefinitionButTheNodePackagesOwn(t *testing.T) {
	written := map[string]struct {
		source   string
		admitted bool
	}{
		"the node package's own building of a named capacity": {
			source: `package node

func (c Capacity) declared() (scenario.NodeCapacity, error) {
	named, err := scenario.NewNamedNodeCapacity(string(c.name), scenario.NodeCapacity{})
	return named.Capacity(), err
}
`,
			admitted: true,
		},
		"the node package defining a capacity of its own": {
			source: `package node

func StandardNodes() Capacity {
	return Named("Standard", Capacity{CPU: 16, Memory: 64 * Gi, Pods: 10})
}
`,
		},
		"the node package reaching the domain's constructor elsewhere": {
			source: `package node

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
		"the Step surface defining a capacity of its own": {
			source: `package lab

func standardNodes() node.Capacity {
	return node.Named("Standard", node.Capacity{CPU: 16, Memory: 64 * node.Gi, Pods: 10})
}
`,
		},
		"the command defining a capacity of its own": {
			source: `package main

func standardNodes() node.Capacity {
	return node.Named("Standard", node.Capacity{CPU: 16, Memory: 64 * node.Gi, Pods: 10})
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
			if name, _, isADefinition := theNameDefinedBy(declared, theNodeName, theNamedNodeCapacityDefinition); isADefinition {
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

func declarationsThatAreNotNamedNodeCapacities(source modulerules.GoFile) []string {
	var offenders []string
	for _, declared := range source.Declarations {
		if _, _, isADefinition := theNameDefinedBy(declared, theNodeName, theNamedNodeCapacityDefinition); !isADefinition {
			offenders = append(offenders, fmt.Sprintf("  %s declares the %s %s, which is not an exported function "+
				"taking nothing that returns one node.%s call under a name it writes",
				source.Path, declared.Kind, declared.Name, theNamedNodeCapacityDefinition))
		}
	}

	return offenders
}

func definitionsOutsideTheNamedNodeCapacities(source modulerules.GoFile) []string {
	var offenders []string
	for _, declared := range source.Declarations {
		if slices.Contains(declared.Names, theNameOf(theNodeName, theNamedNodeCapacityDefinition)) {
			offenders = append(offenders, fmt.Sprintf("  %s, in %s: defines a Named Node capacity outside %s",
				source.Path, declared.Name, theNamedNodeCapacities))
		}
	}

	return offenders
}

func theNameOf(qualifier, name string) modulerules.UsedName {
	return modulerules.UsedName{Qualifier: qualifier, Name: name}
}

// A definition is an exported function taking nothing whose one statement
// returns a call of the qualified definition, its first argument the name as a
// string literal.
func theNameDefinedBy(declared modulerules.Declaration, qualifier, definition string) (string, *ast.CallExpr, bool) {
	if declared.Kind != modulerules.FuncDeclaration || !declared.Exported ||
		len(declared.Parameters) != 0 || len(declared.Statements) != onlyOnce {
		return "", nil, false
	}
	returned, isAReturn := strings.CutPrefix(declared.Statements[0].Written, theReturn)
	if !isAReturn {
		return "", nil, false
	}
	parsed, err := parser.ParseExpr(returned)
	if err != nil {
		return "", nil, false
	}
	call, isACall := parsed.(*ast.CallExpr)
	if !isACall || len(call.Args) == 0 || !callsThe(call, qualifier, definition) {
		return "", nil, false
	}
	literal, isALiteral := call.Args[0].(*ast.BasicLit)
	if !isALiteral || literal.Kind != token.STRING {
		return "", nil, false
	}
	name, err := strconv.Unquote(literal.Value)

	return name, call, err == nil
}

func callsThe(call *ast.CallExpr, qualifier, name string) bool {
	return namesThe(call.Fun, qualifier, name)
}

func namesThe(expression ast.Expr, qualifier, name string) bool {
	selected, isSelected := expression.(*ast.SelectorExpr)
	if !isSelected || selected.Sel.Name != name {
		return false
	}
	written, isAName := selected.X.(*ast.Ident)

	return isAName && written.Name == qualifier
}

func keyedFieldsOf(written ast.Expr, qualifier, name string) (map[string]ast.Expr, bool) {
	literal, isALiteral := written.(*ast.CompositeLit)
	if !isALiteral || !namesThe(literal.Type, qualifier, name) {
		return nil, false
	}

	fields := make(map[string]ast.Expr, len(literal.Elts))
	for _, element := range literal.Elts {
		keyed, isKeyed := element.(*ast.KeyValueExpr)
		if !isKeyed {
			return nil, false
		}
		key, isAName := keyed.Key.(*ast.Ident)
		if !isAName {
			return nil, false
		}
		fields[key.Name] = keyed.Value
	}

	return fields, true
}

func isALiteralOf(written ast.Expr, qualifier, name string) bool {
	literal, isALiteral := written.(*ast.CompositeLit)

	return isALiteral && namesThe(literal.Type, qualifier, name)
}

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

func definesANamedNodeCapacity(declaredIn string, declared modulerules.Declaration, used modulerules.UsedName) bool {
	switch used.Name {
	case theNamedNodeCapacityDefinition:
		return used.Qualifier == theNodeName || (used.Qualifier == "" && declaredIn == theNodeName)
	case theDomainsNamedNodeCapacity:
		isTheNodePackagesOwn := declaredIn == theNodeName &&
			declared.Name == theNamedCapacitysDomainValue && used.Qualifier == theDomainsName

		return !isTheNodePackagesOwn && (used.Qualifier == theDomainsName || declaredIn == theDomainsName)
	default:
		return false
	}
}

// The pod package's own mechanism is the one call of the domain's constructor.
func TestEveryPodSpecificationOfTheScenarios_ShouldBeDefinedOnceBesideThem(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := slices.Concat(
		podSpecificationsNotDefinedOnceBesideTheScenarios(filesOfThePlace(t, place)),
		callsDefiningAPodSpecificationOfTheLabsOwn(theLabsOwnSources(t)))

	if len(offenders) > 0 {
		t.Errorf("each predefined Pod specification is defined once in %s, by an exported function taking nothing that returns one pod.%s call, and the lab defines none, but:\n%s",
			thePodSpecifications, thePodSpecificationDefinition, strings.Join(offenders, "\n"))
	}
}

// A definition names the Named setting rather than writing either toleration,
// so a reader sees its Pods leave an unresponsive Node after the lab's waiting
// time. Leaving the Startup time out does not compile, since it is the
// mechanism's second argument.
func TestEveryPredefinedPodSpecification_ShouldNameTheSettingForLeavingAnUnresponsiveNodeAndWriteNeitherToleration(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := podSpecificationsNotShowingWhatTheyHold(filesOfThePlace(t, place))

	if len(offenders) > 0 {
		t.Errorf("each definition of %s names its Startup time with one of pod.%s, names %s() and writes no pod.%s of the lab's taints, but:\n%s",
			thePodSpecifications, strings.Join(theStartupForms, ", pod."), theNamedSetting, theTolerationSetting,
			strings.Join(offenders, "\n"))
	}
}

func TestThePodSpecificationsRule_ShouldAdmitOneDefinitionPerNameAndReportAnythingElse(t *testing.T) {
	const (
		aSecondDefinitionOfResilientPlacement = `func EvenPlacement() pod.Specification {
	return pod.Predefined("Resilient placement", pod.StartupMillisBetween(500, 1000),
		` + theNamedSettingWritten + `,
	)
}`
		aDefinitionTakingItsValues = `func TimedPlacement(startup pod.StartupTime) pod.Specification {
	return pod.Predefined("Timed placement", startup,
		` + theNamedSettingWritten + `,
	)
}`
		aDefinitionUnderAComputedName = `func ComputedPlacement() pod.Specification {
	return pod.Predefined(pod.SpecificationName("Comp"+"uted"), pod.StartupMillisBetween(500, 1000),
		` + theNamedSettingWritten + `,
	)
}`
		aVariableOfASpecification = `var LoosePlacement = pod.Predefined("Loose placement", pod.StartupMillisBetween(500, 1000),
	` + theNamedSettingWritten + `,
)`
		aDefinitionNamingNoNamedSetting = `func BarePlacement() pod.Specification {
	return pod.Predefined("Bare placement", pod.StartupMillisBetween(500, 1000))
}`
		aDefinitionWritingTheNotReadyToleration = `func PatientPlacement() pod.Specification {
	return pod.Predefined("Patient placement", pod.StartupMillisBetween(500, 1000),
		` + theNamedSettingWritten + `,
		pod.Toleration{Key: node.NotReady, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: 300},
	)
}`
		aDefinitionWritingTheUnreachableToleration = `func StubbornPlacement() pod.Specification {
	return pod.Predefined("Stubborn placement", pod.StartupMillisBetween(500, 1000),
		` + theNamedSettingWritten + `,
		pod.Toleration{Key: node.Unreachable, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: node.UnresponsiveWait},
	)
}`
		aDefinitionNamingAZeroStartupTime = `func TimelessPlacement() pod.Specification {
	return pod.Predefined("Timeless placement", pod.StartupTime{},
		` + theNamedSettingWritten + `,
	)
}`
	)

	written := map[string]struct {
		definitions []string
		line        string
		admitted    bool
	}{
		"one definition per name, declared by a Scenario": {
			definitions: []string{theResilientPlacementDefinition},
			line:        theWorkloadLine,
			admitted:    true,
		},
		"two definitions under one name": {
			definitions: []string{theResilientPlacementDefinition, aSecondDefinitionOfResilientPlacement},
			line:        theWorkloadLine,
		},
		"a definition taking a value": {
			definitions: []string{theResilientPlacementDefinition, aDefinitionTakingItsValues},
			line:        theWorkloadLine,
		},
		"a definition under a name no literal writes": {
			definitions: []string{theResilientPlacementDefinition, aDefinitionUnderAComputedName},
			line:        theWorkloadLine,
		},
		"a variable in place of a function": {
			definitions: []string{theResilientPlacementDefinition, aVariableOfASpecification},
			line:        theWorkloadLine,
		},
		"a definition not naming the Named setting": {
			definitions: []string{theResilientPlacementDefinition, aDefinitionNamingNoNamedSetting},
			line:        theWorkloadLine,
		},
		"a definition writing a toleration of the not-ready taint besides it": {
			definitions: []string{theResilientPlacementDefinition, aDefinitionWritingTheNotReadyToleration},
			line:        theWorkloadLine,
		},
		"a definition writing a toleration of the unreachable taint besides it": {
			definitions: []string{theResilientPlacementDefinition, aDefinitionWritingTheUnreachableToleration},
			line:        theWorkloadLine,
		},
		"a definition naming its Startup time by no Startup form": {
			definitions: []string{theResilientPlacementDefinition, aDefinitionNamingAZeroStartupTime},
			line:        theWorkloadLine,
		},
		"a Scenario defining one on its own line": {
			definitions: []string{theResilientPlacementDefinition},
			line: `s.Given().Workload(Checkout, lab.Pods(9), pod.Predefined("Inline placement", ` +
				`pod.StartupMillisBetween(500, 1000)))`,
		},
	}

	for shape, taken := range written {
		t.Run(shape, func(t *testing.T) {
			place := aPlaceHolding(t, map[string]string{
				theScenarioSource: aScenarioSourceWhoseStepsAre(taken.line),
				thePodSpecifications: aPodSpecificationsSourceOf(
					append([]string{theNamedSettingDefinition, theEvenSpreadDefinition}, taken.definitions...)...),
			})

			offenders := slices.Concat(
				podSpecificationsNotDefinedOnceBesideTheScenarios(place), podSpecificationsNotShowingWhatTheyHold(place))

			expect.Expect(t, len(offenders) == 0).To(expect.Equal(taken.admitted))
		})
	}
}

// Read on scratch sources, since the shipped sources hold only the mechanism's
// own call of the domain's constructor.
func TestTheLabDefinesNoPodSpecificationRule_ShouldReportEveryDefinitionButTheMechanismsOwn(t *testing.T) {
	written := map[string]struct {
		source   string
		admitted bool
	}{
		"the mechanism's own declaration": {
			source: `package pod

func Predefined(name SpecificationName, startup StartupTime, settings ...Customization) Specification {
	declared, err := scenario.NewPodSpecification(string(name), startup.taken)
	return Specification{declared: declared, refused: err}.Customize(settings...)
}
`,
			admitted: true,
		},
		"the domain re-entering its own constructor": {
			source: `package scenario

func (s PodSpecification) Renamed(name string) (PodSpecification, error) {
	return NewPodSpecification(name, s.startupTime, s.settings...)
}
`,
			admitted: true,
		},
		"the pod package defining a specification of its own": {
			source: `package pod

func ResilientPlacement() Specification {
	return Predefined("Resilient placement", StartupMillisBetween(500, 1000))
}
`,
		},
		"the pod package reaching the domain's constructor elsewhere": {
			source: `package pod

func ResilientPlacement() scenario.PodSpecification {
	declared, _ := scenario.NewPodSpecification("Resilient placement", scenario.StartupTime{})
	return declared
}
`,
		},
		"the command defining a specification of its own": {
			source: `package main

func resilientPlacement() pod.Specification {
	return pod.Predefined("Resilient placement", pod.StartupMillisBetween(500, 1000))
}
`,
		},
	}

	for shape, taken := range written {
		t.Run(shape, func(t *testing.T) {
			sources := aPlaceHolding(t, map[string]string{theLabsSource: taken.source})

			offenders := callsDefiningAPodSpecificationOfTheLabsOwn(sources)

			expect.Expect(t, len(offenders) == 0).To(expect.Equal(taken.admitted))
		})
	}
}

func podSpecificationsNotDefinedOnceBesideTheScenarios(files []modulerules.GoFile) []string {
	var offenders []string
	defined := make(map[string]int)
	for _, file := range files {
		if filepath.Base(file.Path) != thePodSpecifications {
			offenders = append(offenders, definitionsOutsideThePodSpecifications(file)...)

			continue
		}
		offenders = append(offenders, declarationsThatAreNotPodSpecifications(file)...)
		for _, declared := range file.Declarations {
			if name, _, isADefinition := theNameDefinedBy(declared, thePodName, thePodSpecificationDefinition); isADefinition {
				defined[name]++
			}
		}
	}
	for name, definitions := range defined {
		if definitions > onlyOnce {
			offenders = append(offenders, fmt.Sprintf("  %s defines the Pod specification %q %d times",
				thePodSpecifications, name, definitions))
		}
	}

	return offenders
}

func declarationsThatAreNotPodSpecifications(source modulerules.GoFile) []string {
	var offenders []string
	for _, declared := range source.Declarations {
		if _, _, isADefinition := theNameDefinedBy(declared, thePodName, thePodSpecificationDefinition); !isADefinition &&
			!isANamedSetting(declared) {
			offenders = append(offenders, fmt.Sprintf("  %s declares the %s %s, which is not an exported function "+
				"taking nothing that returns one pod.%s call under a name it writes",
				source.Path, declared.Kind, declared.Name, thePodSpecificationDefinition))
		}
	}

	return offenders
}

// The shape of these declarations is the Named setting rule's and the Even
// spread rule's to read.
func isTheNamedSetting(declared modulerules.Declaration) bool {
	return declared.Kind == modulerules.FuncDeclaration && declared.Name == theNamedSetting
}

func isANamedSetting(declared modulerules.Declaration) bool {
	return declared.Kind == modulerules.FuncDeclaration && slices.Contains(theNamedSettings, declared.Name)
}

// A Workload line writing a definition inline is an offender too.
func definitionsOutsideThePodSpecifications(source modulerules.GoFile) []string {
	var offenders []string
	for _, declared := range source.Declarations {
		if slices.Contains(declared.Names, theNameOf(thePodName, thePodSpecificationDefinition)) {
			offenders = append(offenders, fmt.Sprintf("  %s, in %s: defines a Pod specification outside %s",
				source.Path, declared.Name, thePodSpecifications))
		}
	}

	return offenders
}

func podSpecificationsNotShowingWhatTheyHold(files []modulerules.GoFile) []string {
	var offenders []string
	for _, file := range files {
		if filepath.Base(file.Path) != thePodSpecifications {
			continue
		}
		for _, declared := range file.Declarations {
			name, call, isADefinition := theNameDefinedBy(declared, thePodName, thePodSpecificationDefinition)
			if !isADefinition {
				continue
			}
			if !namesAStartupForm(call) {
				offenders = append(offenders, fmt.Sprintf("  %s, in %s: %q names its Startup time by no Startup form",
					file.Path, declared.Name, name))
			}
			if !slices.ContainsFunc(call.Args, namesTheNamedSetting) {
				offenders = append(offenders, fmt.Sprintf("  %s, in %s: %q names no %s()",
					file.Path, declared.Name, name, theNamedSetting))
			}
			for _, taint := range theLabsTaints {
				if slices.ContainsFunc(call.Args, func(setting ast.Expr) bool { return tolerates(setting, taint) }) {
					offenders = append(offenders, fmt.Sprintf("  %s, in %s: %q writes a pod.%s of node.%s itself",
						file.Path, declared.Name, name, theTolerationSetting, taint))
				}
			}
		}
	}

	return offenders
}

// The Named setting is one return of pod.Settings over exactly the two
// tolerations of the lab's taints, each matching any value, with the effect
// that moves a Pod off a Node, at the lab's waiting time. No source of the lab
// itself calls pod.Settings.
func TestTheNamedSettingForLeavingAnUnresponsiveNode_ShouldHoldExactlyTheNotReadyAndUnreachableTolerationsAtTheLabsWaitingTime(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := slices.Concat(namedSettingsNotAsDefined(filesOfThePlace(t, place)), callsGroupingSettingsOfTheLabsOwn(theLabsOwnSources(t)))

	if len(offenders) > 0 {
		t.Errorf("%s holds one exported function %s taking nothing, whose body is one return of pod.%s of the pod.%s of node.%s, "+
			"at node.UnresponsiveWait, and nothing else calls pod.%s, but:\n%s",
			thePodSpecifications, theNamedSetting, theGroupingMeans, theTolerationSetting, strings.Join(theLabsTaints, " and node."),
			theGroupingMeans, strings.Join(offenders, "\n"))
	}
}

func TestTheNamedSettingRule_ShouldAdmitTheOneGroupOfTheTwoTolerationsAndReportAnythingElse(t *testing.T) {
	const (
		aGroupWithAThirdSetting = `func LeaveUnresponsiveNodes() pod.Customization {
	return pod.Settings(
		pod.Toleration{Key: node.NotReady, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: node.UnresponsiveWait},
		pod.Toleration{Key: node.Unreachable, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: node.UnresponsiveWait},
		pod.Request{CPU: 1, Memory: 1 * pod.Gi},
	)
}`
		aGroupWithAWrongWait = `func LeaveUnresponsiveNodes() pod.Customization {
	return pod.Settings(
		pod.Toleration{Key: node.NotReady, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: 300},
		pod.Toleration{Key: node.Unreachable, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: node.UnresponsiveWait},
	)
}`
		aGroupOfOneToleration = `func LeaveUnresponsiveNodes() pod.Customization {
	return pod.Settings(
		pod.Toleration{Key: node.NotReady, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: node.UnresponsiveWait},
	)
}`
		aGroupOfTheSameTaintTwice = `func LeaveUnresponsiveNodes() pod.Customization {
	return pod.Settings(
		pod.Toleration{Key: node.NotReady, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: node.UnresponsiveWait},
		pod.Toleration{Key: node.NotReady, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: node.UnresponsiveWait},
	)
}`
		aGroupOfATolerationLeavingOutAField = `func LeaveUnresponsiveNodes() pod.Customization {
	return pod.Settings(
		pod.Toleration{Key: node.NotReady, Operator: pod.Exists, Effect: pod.NoExecute},
		pod.Toleration{Key: node.Unreachable, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: node.UnresponsiveWait},
	)
}`
		aGroupOfATolerationWrittenByPosition = `func LeaveUnresponsiveNodes() pod.Customization {
	return pod.Settings(
		pod.Toleration{node.NotReady, pod.Exists, pod.NoExecute, node.UnresponsiveWait},
		pod.Toleration{Key: node.Unreachable, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: node.UnresponsiveWait},
	)
}`
		aNamedSettingNotGrouping = `func LeaveUnresponsiveNodes() pod.Customization {
	return pod.StartupSeconds(1)
}`
		aNamedSettingTakingAValue = `func LeaveUnresponsiveNodes(wait int64) pod.Customization {
	return pod.Settings(
		pod.Toleration{Key: node.NotReady, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: wait},
		pod.Toleration{Key: node.Unreachable, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: wait},
	)
}`
		aSecondGroup = `func SpreadEvenly() pod.Customization {
	return pod.Settings(EvenSpreadOver(node.Zone))
}`
	)

	written := map[string]struct {
		source   string
		admitted bool
	}{
		"the one group of the two tolerations": {
			source:   aPodSpecificationsSourceOf(theNamedSettingDefinition, theResilientPlacementDefinition),
			admitted: true,
		},
		"no Named setting at all": {
			source: aPodSpecificationsSourceOf(theResilientPlacementDefinition),
		},
		"a group with a third setting": {
			source: aPodSpecificationsSourceOf(aGroupWithAThirdSetting, theResilientPlacementDefinition),
		},
		"a group with a wrong waiting time": {
			source: aPodSpecificationsSourceOf(aGroupWithAWrongWait, theResilientPlacementDefinition),
		},
		"a group of one toleration": {
			source: aPodSpecificationsSourceOf(aGroupOfOneToleration, theResilientPlacementDefinition),
		},
		"a group of one taint twice": {
			source: aPodSpecificationsSourceOf(aGroupOfTheSameTaintTwice, theResilientPlacementDefinition),
		},
		"a group of a toleration leaving out a field": {
			source: aPodSpecificationsSourceOf(aGroupOfATolerationLeavingOutAField, theResilientPlacementDefinition),
		},
		"a group of a toleration written by position": {
			source: aPodSpecificationsSourceOf(aGroupOfATolerationWrittenByPosition, theResilientPlacementDefinition),
		},
		"a Named setting that groups nothing": {
			source: aPodSpecificationsSourceOf(aNamedSettingNotGrouping, theResilientPlacementDefinition),
		},
		"a Named setting taking a value": {
			source: aPodSpecificationsSourceOf(aNamedSettingTakingAValue, theResilientPlacementDefinition),
		},
		"a second group besides it": {
			source: aPodSpecificationsSourceOf(theNamedSettingDefinition, aSecondGroup, theResilientPlacementDefinition),
		},
		"the Named setting defined twice": {
			source: aPodSpecificationsSourceOf(theNamedSettingDefinition, theNamedSettingDefinition, theResilientPlacementDefinition),
		},
	}

	for shape, taken := range written {
		t.Run(shape, func(t *testing.T) {
			place := aPlaceHolding(t, map[string]string{thePodSpecifications: taken.source})

			offenders := namedSettingsNotAsDefined(place)

			expect.Expect(t, len(offenders) == 0).To(expect.Equal(taken.admitted))
		})
	}
}

// Read on scratch sources, since no shipped source calls the grouping means.
func TestTheLabDefinesNoNamedSettingRule_ShouldReportEveryCallOfTheGroupingMeans(t *testing.T) {
	written := map[string]struct {
		source   string
		admitted bool
	}{
		"the mechanism's own declaration": {
			source: `package pod

func Settings(settings ...Setting) Customization {
	return settingGroup{}
}
`,
			admitted: true,
		},
		"a field named Settings read off a value": {
			source: `package lab

func (s Step) all() []string {
	return s.Settings
}
`,
			admitted: true,
		},
		"the pod package grouping settings itself": {
			source: `package pod

func LeaveUnresponsiveNodes() Customization {
	return Settings(Toleration{Key: "node.kubernetes.io/not-ready", Operator: Exists, Effect: NoExecute, TolerationSeconds: 2})
}
`,
		},
		"the Step surface grouping settings": {
			source: `package lab

func leaving() pod.Customization {
	return pod.Settings()
}
`,
		},
		"the command grouping settings": {
			source: `package main

func leaving() pod.Customization {
	return pod.Settings()
}
`,
		},
	}

	for shape, taken := range written {
		t.Run(shape, func(t *testing.T) {
			sources := aPlaceHolding(t, map[string]string{theLabsSource: taken.source})

			offenders := callsGroupingSettingsOfTheLabsOwn(sources)

			expect.Expect(t, len(offenders) == 0).To(expect.Equal(taken.admitted))
		})
	}
}

func namedSettingsNotAsDefined(files []modulerules.GoFile) []string {
	var offenders []string
	held := 0
	for _, file := range files {
		for _, declared := range file.Declarations {
			if filepath.Base(file.Path) == thePodSpecifications && isTheNamedSetting(declared) {
				held++
				if why := whyNotTheNamedSetting(declared); why != "" {
					offenders = append(offenders, fmt.Sprintf("  %s, in %s: %s", file.Path, declared.Name, why))
				}

				continue
			}
			if slices.Contains(declared.Names, theNameOf(thePodName, theGroupingMeans)) {
				offenders = append(offenders, fmt.Sprintf("  %s, in %s: calls pod.%s besides %s",
					file.Path, declared.Name, theGroupingMeans, theNamedSetting))
			}
		}
	}
	if held != onlyOnce {
		offenders = append(offenders, fmt.Sprintf("  %s defines %s %d times", thePodSpecifications, theNamedSetting, held))
	}

	return offenders
}

func whyNotTheNamedSetting(declared modulerules.Declaration) string {
	if !declared.Exported || len(declared.Parameters) != 0 || len(declared.Statements) != onlyOnce {
		return "is not an exported function taking nothing whose body is one statement"
	}
	returned, isAReturn := strings.CutPrefix(declared.Statements[0].Written, theReturn)
	if !isAReturn {
		return "does not return its group"
	}
	parsed, err := parser.ParseExpr(returned)
	if err != nil {
		return "does not return an expression"
	}
	group, isACall := parsed.(*ast.CallExpr)
	if !isACall || !callsThe(group, thePodName, theGroupingMeans) {
		return fmt.Sprintf("does not return one pod.%s call", theGroupingMeans)
	}
	if len(group.Args) != len(theLabsTaints) {
		return fmt.Sprintf("groups %d settings, not the %d tolerations of the lab's taints", len(group.Args), len(theLabsTaints))
	}
	for position, taint := range theLabsTaints {
		if !isTheLabsTolerationOf(group.Args[position], taint) {
			return fmt.Sprintf("does not hold, as setting %d, pod.%s keyed node.%s, pod.Exists, pod.NoExecute at node.UnresponsiveWait",
				position+1, theTolerationSetting, taint)
		}
	}

	return ""
}

// The toleration is a keyed literal of its four fields: the taint as its key,
// matching any value, the effect that moves a Pod off a Node, at the lab's
// waiting time.
func isTheLabsTolerationOf(setting ast.Expr, taint string) bool {
	fields, isAToleration := keyedFieldsOf(setting, thePodName, theTolerationSetting)

	return isAToleration && len(fields) == theTolerationFields &&
		namesThe(fields[theTolerationsKey], theNodeName, taint) &&
		namesThe(fields["Operator"], thePodName, "Exists") &&
		namesThe(fields["Effect"], thePodName, "NoExecute") &&
		namesThe(fields["TolerationSeconds"], theNodeName, "UnresponsiveWait")
}

// The mechanism's declaration calls nothing by that name, so it is no offender.
func callsGroupingSettingsOfTheLabsOwn(files []modulerules.GoFile) []string {
	var offenders []string
	for _, file := range files {
		for _, declared := range file.Declarations {
			for _, used := range declared.Names {
				// A type declaring a field of that name is no call, so inside
				// the pod package only a function's unqualified use counts.
				callsIt := used.Name == theGroupingMeans && (used.Qualifier == thePodName ||
					(used.Qualifier == "" && file.Package == thePodName && declared.Kind == modulerules.FuncDeclaration))
				if callsIt {
					offenders = append(offenders, fmt.Sprintf("  %s, in %s: %s groups settings under a name of the lab's own",
						file.Path, declared.Name, writtenAs(used)))
				}
			}
		}
	}

	return offenders
}

func namesAStartupForm(definition *ast.CallExpr) bool {
	if len(definition.Args) < theFirstSettingArgument {
		return false
	}
	startup, isACall := definition.Args[theStartupTimeArgument].(*ast.CallExpr)

	return isACall && slices.ContainsFunc(theStartupForms, func(form string) bool {
		return callsThe(startup, thePodName, form)
	})
}

func namesTheNamedSetting(setting ast.Expr) bool {
	named, isACall := setting.(*ast.CallExpr)
	if !isACall {
		return false
	}
	function, isAName := named.Fun.(*ast.Ident)

	return isAName && function.Name == theNamedSetting && len(named.Args) == 0
}

func tolerates(setting ast.Expr, taint string) bool {
	fields, isAToleration := keyedFieldsOf(setting, thePodName, theTolerationSetting)

	return isAToleration && namesThe(fields[theTolerationsKey], theNodeName, taint)
}

func thePodSpecificationsDeclaredIn(files []modulerules.GoFile) map[string]bool {
	defined := make(map[string]bool)
	for _, file := range files {
		if filepath.Base(file.Path) != thePodSpecifications {
			continue
		}
		for _, declared := range file.Declarations {
			if _, _, isADefinition := theNameDefinedBy(declared, thePodName, thePodSpecificationDefinition); isADefinition {
				defined[declared.Name] = true
			}
		}
	}

	return defined
}

func callsDefiningAPodSpecificationOfTheLabsOwn(files []modulerules.GoFile) []string {
	var offenders []string
	for _, file := range files {
		for _, declared := range file.Declarations {
			for _, used := range declared.Names {
				if definesAPodSpecification(file.Package, declared, used) {
					offenders = append(offenders, fmt.Sprintf("  %s, in %s: %s defines a Pod specification of the lab's own",
						file.Path, declared.Name, writtenAs(used)))
				}
			}
		}
	}

	return offenders
}

// The domain calling its own constructor, unqualified, is no definition.
func definesAPodSpecification(declaredIn string, declared modulerules.Declaration, used modulerules.UsedName) bool {
	isTheMechanism := declaredIn == thePodName && declared.Name == thePodSpecificationDefinition

	switch used.Name {
	case thePodSpecificationDefinition:
		return !isTheMechanism &&
			(used.Qualifier == thePodName || (used.Qualifier == "" && declaredIn == thePodName))
	case theDomainsPodSpecification:
		return !isTheMechanism && used.Qualifier == theDomainsName
	default:
		return false
	}
}

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

// No Scenario of the lab writes a Customization yet, so the rule's decisions
// are read on scratch sources.
func TestTheWorkloadLineRule_ShouldAdmitOneCustomizedSpecificationAndReportAnythingElse(t *testing.T) {
	written := map[string]struct {
		line     string
		admitted bool
	}{
		"one predefined specification": {
			line:     `s.Given().Workload(Checkout, lab.Pods(9), ResilientPlacement())`,
			admitted: true,
		},
		"a specification it customizes": {
			line: `s.Given().Workload(Checkout, lab.Pods(9), ResilientPlacement().Customize(
		pod.TopologySpreadConstraint{TopologyKey: node.Zone, MaxSkew: 2, WhenUnsatisfiable: pod.DoNotSchedule, NodeTaintsPolicy: pod.Honor},
	))`,
			admitted: true,
		},
		"a Customize naming no setting": {
			line:     `s.Given().Workload(Checkout, lab.Pods(9), ResilientPlacement().Customize())`,
			admitted: true,
		},
		"no specification at all": {
			line:     `s.Given().Workload(Checkout, lab.Pods(9))`,
			admitted: false,
		},
		"a setting written beside the specification": {
			line: `s.Given().Workload(Checkout, lab.Pods(9), ResilientPlacement(),
		pod.TopologySpreadConstraint{TopologyKey: node.Zone, MaxSkew: 2, WhenUnsatisfiable: pod.DoNotSchedule, NodeTaintsPolicy: pod.Honor})`,
			admitted: false,
		},
		"a second Customize written on the first": {
			line:     `s.Given().Workload(Checkout, lab.Pods(9), ResilientPlacement().Customize().Customize())`,
			admitted: false,
		},
		"a Required Capacity written after the Workload Step": {
			line:     `s.Given().Workload(Checkout, lab.Pods(9), ResilientPlacement()).RequiredCapacity(lab.Pods(6))`,
			admitted: true,
		},
		"a second Required Capacity written on the first": {
			line: `s.Given().Workload(Checkout, lab.Pods(9), ResilientPlacement()).` +
				`RequiredCapacity(lab.Pods(6)).RequiredCapacity(lab.Pods(3))`,
			admitted: false,
		},
		"a Wait chained onto the Workload Step": {
			line:     `s.Given().Workload(Checkout, lab.Pods(9), ResilientPlacement()).WaitFor().AllPodsReady()`,
			admitted: true,
		},
		"a Wait chained onto the Required Capacity": {
			line: `s.Given().Workload(Checkout, lab.Pods(9), ResilientPlacement()).` +
				`RequiredCapacity(lab.Pods(6)).WaitFor().AllPodsReady()`,
			admitted: true,
		},
		"a Wait written on the Workload Step with no WaitFor opening it": {
			line:     `s.Given().Workload(Checkout, lab.Pods(9), ResilientPlacement()).AllPodsReady()`,
			admitted: false,
		},
		"a Customize written after the Workload Step": {
			line:     `s.Given().Workload(Checkout, lab.Pods(9), ResilientPlacement()).Customize()`,
			admitted: false,
		},
		"a call written after the Workload Step": {
			line:     `s.Given().Workload(Checkout, lab.Pods(9), ResilientPlacement()).Pool("general", node.PerZone(1), StandardNodes())`,
			admitted: false,
		},
		"a specification of the Scenario's own": {
			line:     `s.Given().Workload(Checkout, lab.Pods(9), aSpecificationOfItsOwn())`,
			admitted: false,
		},
		"a specification named on the Step surface": {
			line:     `s.Given().Workload(Checkout, lab.Pods(9), lab.ResilientPlacement())`,
			admitted: false,
		},
		"a specification defined on the line itself": {
			line: `s.Given().Workload(Checkout, lab.Pods(9), ` +
				`pod.Predefined("Inline placement", pod.StartupMillisBetween(500, 1000)))`,
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

const (
	theWorkloadLine   = `s.Given().Workload(Checkout, lab.Pods(9), ResilientPlacement())`
	theFullAccessLine = `s.Given().Workload(Checkout, lab.Pods(9), ResilientPlacement().Customize(
		pod.FullAccess(func(spec *` + thePodSettingsName + `.PodSpec) {
			spec.PriorityClassName = "critical"
		}),
	))`
)

func aScenarioWhoseWorkloadLineIs(t *testing.T, line string) []modulerules.GoFile {
	t.Helper()

	return aScenarioSourceThatImports(t, theScenarioImports, line)
}

func aScenarioAlsoImporting(t *testing.T, imported, line string) []modulerules.GoFile {
	t.Helper()

	return aScenarioSourceThatImports(t, append(slices.Clone(theScenarioImports), imported), line)
}

func aScenarioSourceThatImports(t *testing.T, imported []string, line string) []modulerules.GoFile {
	t.Helper()

	return aPlaceHolding(t, map[string]string{
		theScenarioSource: fmt.Sprintf(theScenarioSourceShape, anImportBlockOf(imported), line),
		thePodSpecifications: aPodSpecificationsSourceOf(
			theNamedSettingDefinition, theEvenSpreadDefinition, theResilientPlacementDefinition),
	})
}

func aScenarioBesideTheNamedNodeCapacities(t *testing.T, steps ...string) []modulerules.GoFile {
	t.Helper()

	return aPlaceHolding(t, map[string]string{
		theScenarioSource:      aScenarioSourceWhoseStepsAre(steps...),
		theNamedNodeCapacities: aNamedNodeCapacitiesSourceOf(theStandardNodesDefinition),
	})
}

func aScenarioSourceWhoseStepsAre(steps ...string) string {
	return fmt.Sprintf(theScenarioSourceShape, anImportBlockOf(theScenarioImports), strings.Join(steps, "\n\t"))
}

func aNamedNodeCapacitiesSourceOf(definitions ...string) string {
	return fmt.Sprintf(aFileOfDefinitionsShape, strings.Join(definitions, "\n\n"))
}

func aPodSpecificationsSourceOf(definitions ...string) string {
	return fmt.Sprintf(aFileOfDefinitionsShape, strings.Join(definitions, "\n\n"))
}

// The sources are written to disk and parsed, because a rule reads a parsed
// source and not a string.
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

// The Wait after a Failure measures the lab's subject, how long the control
// plane took to place the Pods again, so stating its time lets a run judge the
// restore rather than only report it. The Waits above the first Failure state
// none.
func TestEveryFailureScenario_ShouldStateATimeOnTheWaitAfterItsFailure(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := waitsStatingATimeTheirPlaceDoesNotAskFor(filesOfThePlace(t, place))

	if len(offenders) > 0 {
		t.Errorf("the Wait after a Failure states the time it must be reached in, and a Wait above the first Failure states none, but:\n%s",
			strings.Join(offenders, "\n"))
	}
}

// One Step per line lets a reader scan the Step kinds in one column and move a
// line on its own. A Wait chained on the Given before it takes that line's Step
// kind off the column.
func TestEveryScenarioOfTheLab_ShouldWriteEachWaitAsAStepOfItsOwn(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := waitsWrittenOnTheGivenBeforeThem(filesOfThePlace(t, place))

	if len(offenders) > 0 {
		t.Errorf("every Wait of a Scenario is a Step of its own, opened on the Scenario, but:\n%s",
			strings.Join(offenders, "\n"))
	}
}

// Read on scratch sources, since every shipped Scenario states its restore
// time.
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

// Every Wait below the first Failure is a restore judged against a time; every
// Wait above it provisions the world the Failure is injected into.
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

// The Wait's name is what states whether it takes a time.
func statesATime(line modulerules.Statement) bool {
	return slices.Contains(theWaitsStatingATime, line.Name)
}

// Read on scratch sources, since every shipped Scenario waits everywhere it
// must.
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

const (
	theTopologyLine         = `s.Given().Topology(lab.Zones(3), node.Pool("general", node.PerZone(3), StandardNodes()))`
	theTopologyWaitingLine  = theTopologyLine + `.WaitFor().AllNodesReady()`
	theWorkloadWaitingLine  = theWorkloadLine + `.WaitFor().AllPodsReady()`
	theWaitForEveryNodeLine = `s.WaitFor().AllNodesReady()`
	theWaitForEveryPodLine  = `s.WaitFor().AllPodsReady()`

	theWaitForEveryPodLineStatingATime = `s.WaitFor().AllPodsReadyWithinSeconds(30)`
	theFailureLine                     = `s.When().NodesFail(lab.Nodes(1))`
	theThenLine                        = `s.Then().PodsPerZone(3)`
)

func aWorldDeclaredAndAwaited(followedBy ...string) []string {
	opened := []string{theTopologyLine, theWaitForEveryNodeLine, theWorkloadLine, theWaitForEveryPodLine}

	return append(opened, followedBy...)
}

func aScenarioWhoseStepsAre(t *testing.T, steps ...string) []modulerules.GoFile {
	t.Helper()

	return aScenarioSourceThatImports(t, theScenarioImports, strings.Join(steps, "\n\t"))
}

// Such a Step ends the Scenario when it runs, so the rule reads the order off
// the source instead of leaving it to a run.
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

// A Failure reads the last Placement to count the Pods kept Running and then
// discards it, so what a Wait reached holds for the Thens after it and no
// further.
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

// Both ways of writing a Wait end at one of the Waits as the outermost call.
func isAWait(line modulerules.Statement) bool {
	return line.Call && slices.Contains(theWaits, line.Name)
}

func isAFailure(line modulerules.Statement, scenarioParameter string) bool {
	kind, opened := theStepKindOf(line, scenarioParameter)

	return opened && kind == WhenStep
}

func isAThen(line modulerules.Statement, scenarioParameter string) bool {
	kind, opened := theStepKindOf(line, scenarioParameter)

	return opened && kind == ThenStep
}

// The chained form stays offered to an engineer, so the rule is read on scratch
// sources; the lab's own place writes it nowhere.
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

func isAWaitStep(line modulerules.Statement, scenarioParameter string) bool {
	kind, opened := theStepKindOf(line, scenarioParameter)

	return opened && kind == WaitStep
}

// A name written unqualified, as a dot import would write it, is neither a
// subject's name nor one the Scenarios define.
func TestEveryScenarioSource_ShouldWriteEachNameUnderItsSubjectOrAsOneTheScenariosDefine(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := namesUsedThatTheLabDoesNotOffer(t, filesOfThePlace(t, place))

	if len(offenders) > 0 {
		t.Errorf("every name a Scenario uses is one lab, pod or node exports under its own qualifier, or one the Scenarios define, but:\n%s",
			strings.Join(offenders, "\n"))
	}
}

// Read on scratch sources, since the lab's own place names only what it is
// offered.
func TestTheNameRule_ShouldReportANameNeitherItsSubjectNorThePlaceDeclares(t *testing.T) {
	written := map[string]struct {
		line     string
		admitted bool
	}{
		"the names each subject's package exports": {
			line: `s.Given().Workload(Checkout, lab.Pods(9), ResilientPlacement().Customize(` +
				`pod.Request{CPU: 1, Memory: 1 * pod.Gi}))`,
			admitted: true,
		},
		"a specification the Step surface no longer exports": {
			line: `s.Given().Workload(Checkout, lab.Pods(9), lab.MaximumAvailability())`,
		},
		"a name of the Scenario's own": {
			line: `s.Given().Workload(Checkout, lab.Pods(9), aSpecificationOfItsOwn())`,
		},
		"a Pod setting under the Scenario's package": {
			line: `s.Given().Workload(Checkout, lab.Pods(9), ResilientPlacement().Customize(` +
				`lab.Request{CPU: 1, Memory: 1 * pod.Gi}))`,
		},
		"a Node's taint under the pod package": {
			line: `s.Given().Workload(Checkout, lab.Pods(9), ResilientPlacement().Customize(` +
				`pod.Toleration{Key: pod.NotReady, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: 300}))`,
		},
		"a function marking the position of a value": {
			line: `s.Given().Workload(Checkout, lab.Pods(9), ResilientPlacement().Customize(` +
				`pod.Toleration{Key: pod.Key(node.NotReady), Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: 300}))`,
		},
		"a Pod setting written unqualified, as a dot import writes it": {
			line: `s.Given().Workload(Checkout, lab.Pods(9), ResilientPlacement().Customize(` +
				`Request{CPU: 1, Memory: 1 * Gi}))`,
		},
	}

	for named, taken := range written {
		t.Run(named, func(t *testing.T) {
			offenders := namesUsedThatTheLabDoesNotOffer(t, aScenarioWhoseWorkloadLineIs(t, taken.line))

			expect.Expect(t, len(offenders) == 0).To(expect.Equal(taken.admitted))
		})
	}
}

// Read on scratch sources, since no shipped Scenario uses full access. The
// placeholder the Scenario's own function literal declares is no name it
// writes.
func TestTheNameRule_ShouldAdmitTheKubernetesPodSettingsFullAccessNames(t *testing.T) {
	reaching := aScenarioAlsoImporting(t, thePodSettingsImport, theFullAccessLine)

	offenders := namesUsedThatTheLabDoesNotOffer(t, reaching)

	expect.Expect(t, offenders).To(expect.HaveLen[string](0))
}

// A name taken from a value, such as a Step called on the Scenario, is the
// compiler's to check and admitted here.
func namesUsedThatTheLabDoesNotOffer(t *testing.T, files []modulerules.GoFile) []string {
	t.Helper()

	offered := theNamesEachSubjectOffers(t)
	ownedByThePlace := theNamesDeclaredIn(files)

	var offenders []string
	for _, file := range files {
		for _, declared := range file.Declarations {
			for _, used := range declared.Names {
				if isOffered(used, offered, ownedByThePlace, declared) {
					continue
				}
				offenders = append(offenders, fmt.Sprintf("  %s, in %s: %s is no name lab, pod or node offers Scenarios under it",
					file.Path, declared.Name, writtenAs(used)))
			}
		}
	}

	return offenders
}

func isOffered(
	used modulerules.UsedName,
	offered map[string]map[string]bool,
	ownedByThePlace map[string]bool,
	declared modulerules.Declaration,
) bool {
	if bySubject, isASubject := offered[used.Qualifier]; isASubject {
		return bySubject[used.Name]
	}
	if used.Qualifier != "" {
		return true
	}

	return ownedByThePlace[used.Name] || isAParameterOf(declared, used.Name) ||
		types.Universe.Lookup(used.Name) != nil
}

func theNamesEachSubjectOffers(t *testing.T) map[string]map[string]bool {
	t.Helper()

	root, err := modulerules.Root(t.Context(), ownPackageDir)
	if err != nil {
		t.Fatalf("locate the root of the module: %v", err)
	}
	sources, err := modulerules.ScanGoFiles(filepath.Join(root, theLabsOwnPackageDir), modulerules.ProductionGoFilesOnly)
	if err != nil {
		t.Fatalf("read the sources of the lab's three packages: %v", err)
	}

	offered := make(map[string]map[string]bool, len(theSubjects))
	for _, subject := range theSubjects {
		offered[subject] = make(map[string]bool)
	}
	for _, source := range sources {
		bySubject, isASubject := offered[source.Package]
		if !isASubject {
			continue
		}
		for _, declared := range source.Declarations {
			if declared.Exported {
				bySubject[declared.Name] = true
			}
		}
	}

	return offered
}

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

func TestEveryScenarioOfTheLab_ShouldStandInOneSourceOfTheScenarios(t *testing.T) {
	place := theScenariosPlace(t)
	files := filesOfThePlace(t, place)

	offenders := slices.Concat(scenariosOutsideTheList(files), scenariosOutOfTheListsOrder(files))

	if len(offenders) > 0 {
		t.Errorf("%s holds every Scenario of the lab, in the order %s names them, and no other source holds one, but:\n%s",
			theList, theListFunction, strings.Join(offenders, "\n"))
	}
}

// What these Scenarios assert bears on nothing; where they stand and in which
// order is what the one-source rule judges.
const (
	theFirstScenario  = "SomeCondition_ShouldHold"
	theSecondScenario = "AnotherCondition_ShouldHold"
)

func TestTheOneSourceRule_ShouldAdmitEveryScenarioInTheListsOrderAndReportOneElsewhereOrOutOfOrder(t *testing.T) {
	placed := map[string]struct {
		sources  map[string]string
		admitted bool
	}{
		"every Scenario after the list, in its order": {
			sources: map[string]string{
				theList: theListSourceOf([]string{theFirstScenario, theSecondScenario}, theFirstScenario, theSecondScenario),
			},
			admitted: true,
		},
		"a Scenario in a source of its own": {
			sources: map[string]string{
				theList:           theListSourceOf([]string{theFirstScenario, theSecondScenario}, theFirstScenario),
				theScenarioSource: theListSourceOf(nil, theSecondScenario),
			},
		},
		"the Scenarios in another order than the list's": {
			sources: map[string]string{
				theList: theListSourceOf([]string{theFirstScenario, theSecondScenario}, theSecondScenario, theFirstScenario),
			},
		},
	}

	for shape, taken := range placed {
		t.Run(shape, func(t *testing.T) {
			place := aPlaceHolding(t, taken.sources)

			offenders := slices.Concat(scenariosOutsideTheList(place), scenariosOutOfTheListsOrder(place))

			expect.Expect(t, len(offenders) == 0).To(expect.Equal(taken.admitted))
		})
	}
}

func theListSourceOf(listed []string, declared ...string) string {
	var source strings.Builder
	fmt.Fprintf(&source, "package scenarios\n\nimport %s\n", theStepSurfaceImport)
	if listed != nil {
		fmt.Fprintf(&source, "\nfunc %s() []func(%s) {\n\treturn []func(%s){\n", theListFunction, theScenarioParameter, theScenarioParameter)
		for _, scenario := range listed {
			fmt.Fprintf(&source, "\t\t%s,\n", scenario)
		}
		source.WriteString("\t}\n}\n")
	}
	for _, scenario := range declared {
		fmt.Fprintf(&source, "\nfunc %s(s %s) {}\n", scenario, theScenarioParameter)
	}

	return source.String()
}

// A run reaches the Scenarios through the list, and an engineer reads them in
// its file.
func scenariosOutsideTheList(files []modulerules.GoFile) []string {
	var offenders []string
	for _, file := range files {
		if filepath.Base(file.Path) == theList {
			continue
		}
		for _, declared := range file.Declarations {
			if isAScenario(declared) {
				offenders = append(offenders, fmt.Sprintf("  %s declares the Scenario %s outside %s", file.Path, declared.Name, theList))
			}
		}
	}

	return offenders
}

// The order is checked so the list reads as the index of what follows. A place
// holding no list is reported by the place rule.
func scenariosOutOfTheListsOrder(files []modulerules.GoFile) []string {
	listed, found := theListIn(files)
	if !found {
		return nil
	}

	declared := make([]string, 0, len(files))
	for _, scenario := range theScenariosIn(files) {
		declared = append(declared, scenario.Name)
	}
	named := theScenariosNamedBy(listed, declared)
	if slices.Equal(declared, named) {
		return nil
	}

	return []string{fmt.Sprintf("  %s declares its Scenarios as %v, but %s names them as %v", theList, declared, theListFunction, named)}
}

func theScenariosNamedBy(listed modulerules.Declaration, declared []string) []string {
	var named []string
	for _, line := range listed.Statements {
		for _, identifier := range identifiersIn(line.Written) {
			if slices.Contains(declared, identifier) {
				named = append(named, identifier)
			}
		}
	}

	return named
}

func theReadmeEntryOf(scenarioName string) string {
	return "**" + scenarioName + "**"
}

// The place is located from the module file rather than from where go test
// runs.
func theScenariosPlace(t *testing.T) string {
	t.Helper()

	root, err := modulerules.Root(t.Context(), ownPackageDir)
	if err != nil {
		t.Fatalf("locate the root of the module: %v", err)
	}

	return filepath.Join(root, scenariosDirectory)
}

func filesOfThePlace(t *testing.T, place string) []modulerules.GoFile {
	t.Helper()

	sources, err := modulerules.ScanGoFiles(place, modulerules.ProductionGoFilesOnly)
	if err != nil {
		t.Fatalf("read the Scenario sources under %s: %v", place, err)
	}

	return sources
}

func theScenariosIn(files []modulerules.GoFile) []modulerules.Declaration {
	var scenarios []modulerules.Declaration
	for _, file := range files {
		if filepath.Base(file.Path) != theList {
			continue
		}
		for _, declared := range file.Declarations {
			if isAScenario(declared) {
				scenarios = append(scenarios, declared)
			}
		}
	}

	return scenarios
}

// One predicate per admitted file, so a further shape is a deliberate edit
// rather than a side effect.
func declarationsThatDoNotBelong(files []modulerules.GoFile) []string {
	var offenders []string
	for _, file := range files {
		switch filepath.Base(file.Path) {
		case theWorkloadNames:
			offenders = append(offenders, declarationsThatAreNotWorkloadNames(file)...)
		case theNamedNodeCapacities:
			offenders = append(offenders, declarationsThatAreNotNamedNodeCapacities(file)...)
		case theNodeLabelKeys, theTaints:
			offenders = append(offenders, declarationsThatAreNotNodeLabelKeys(file)...)
		case thePodSpecifications:
			offenders = append(offenders, declarationsThatAreNotPodSpecifications(file)...)
		case theList:
			offenders = append(offenders, declarationsThatAreNeitherTheListNorAScenario(file)...)
		default:
			offenders = append(offenders, declarationsOfAFileThePlaceDoesNotAdmit(file)...)
		}
	}

	return offenders
}

func declarationsThatAreNeitherTheListNorAScenario(source modulerules.GoFile) []string {
	var offenders []string
	for _, declared := range source.Declarations {
		switch {
		case isTheList(declared):
		case isAScenario(declared):
			offenders = append(offenders, linesOfScenarioThatAreNotSteps(source.Path, declared)...)
		default:
			offenders = append(offenders, fmt.Sprintf("  %s declares the %s %s, which is neither the exported %s nor a Scenario named %s",
				source.Path, declared.Kind, declared.Name, theListFunction, theScenarioNameShape))
		}
	}

	return offenders
}

func declarationsOfAFileThePlaceDoesNotAdmit(source modulerules.GoFile) []string {
	offenders := make([]string, 0, len(source.Declarations))
	for _, declared := range source.Declarations {
		offenders = append(offenders, fmt.Sprintf("  %s declares the %s %s, but the place admits no file of that name",
			source.Path, declared.Kind, declared.Name))
	}

	return offenders
}

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

// The scan keeps no constant's value, so the file is read again here.
func declarationsThatAreNotNodeLabelKeys(source modulerules.GoFile) []string {
	parsed, err := parser.ParseFile(token.NewFileSet(), source.Path, nil, parser.SkipObjectResolution)
	if err != nil {
		return []string{fmt.Sprintf("  %s cannot be read: %v", source.Path, err)}
	}

	var offenders []string
	for _, declared := range parsed.Decls {
		if !declaresNodeLabelKeysAlone(declared) {
			offenders = append(offenders, fmt.Sprintf("  %s declares something other than an exported untyped string constant",
				source.Path))
		}
	}

	return offenders
}

func declaresNodeLabelKeysAlone(declared ast.Decl) bool {
	group, isAGroup := declared.(*ast.GenDecl)
	if !isAGroup || group.Tok != token.CONST {
		return false
	}
	for _, spec := range group.Specs {
		constant, isAValue := spec.(*ast.ValueSpec)
		if !isAValue || constant.Type != nil || len(constant.Values) != len(constant.Names) {
			return false
		}
		for at, name := range constant.Names {
			literal, isALiteral := constant.Values[at].(*ast.BasicLit)
			if !name.IsExported() || !isALiteral || literal.Kind != token.STRING {
				return false
			}
		}
	}

	return true
}

// A Scenario declaring no Workload line is reported too, since it would meet
// the shape rule by having nothing to check.
func workloadLinesDeclaringNoPredefinedSpecification(files []modulerules.GoFile) []string {
	defined := thePodSpecificationsDeclaredIn(files)

	var offenders []string
	for _, source := range files {
		for _, declared := range source.Declarations {
			if !isAScenario(declared) {
				continue
			}
			offenders = append(offenders, workloadLinesOfScenarioThatDeclareNoSpecification(source.Path, declared, defined)...)
		}
	}

	return offenders
}

func workloadLinesOfScenarioThatDeclareNoSpecification(
	path string, declared modulerules.Declaration, defined map[string]bool,
) []string {
	var offenders []string
	deploying := 0
	for _, line := range declared.Statements {
		if !namesTheWorkloadStep(line) {
			continue
		}
		deploying++
		if !declaresOnePredefinedSpecification(line, defined) {
			offenders = append(offenders, fmt.Sprintf("  %s, in %s: %s declares no predefined Pod specification",
				path, declared.Name, line.Written))
		}
	}
	if deploying == 0 {
		offenders = append(offenders, fmt.Sprintf("  %s, in %s: no Workload is declared at all", path, declared.Name))
	}

	return offenders
}

// A line calling something after the Workload Step deploys one too, and is a
// line the shape rule reports.
func namesTheWorkloadStep(line modulerules.Statement) bool {
	for called := &line; called != nil; called = called.On {
		if called.Call && called.Name == theWorkloadStep {
			return true
		}
	}

	return false
}

func declaresOnePredefinedSpecification(line modulerules.Statement, defined map[string]bool) bool {
	deploying, ends := theWorkloadStepTheLineEndsAt(line)
	if !ends || len(deploying.Arguments) != theWorkloadLineArguments {
		return false
	}

	declared := deploying.Arguments[theSpecificationArgument]
	if declared.Call && declared.Name == theCustomize && declared.On != nil {
		declared = *declared.On
	}

	return isAPredefinedSpecification(declared, defined)
}

// A Wait chained onto the line is read past first: it is a Step of its own and
// no part of what the Workload declares.
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

// A Wait named directly on a declaration is not read past, since WaitFor is
// what opens a chained Wait, so such a line is reported.
func whatTheChainedWaitWasWrittenOn(line modulerules.Statement) modulerules.Statement {
	if !isAWait(line) || line.On == nil || !line.On.Call || line.On.Name != theWaitOpener || line.On.On == nil {
		return line
	}

	return *line.On.On
}

func isAPredefinedSpecification(declared modulerules.Statement, defined map[string]bool) bool {
	return declared.Call && len(declared.Arguments) == 0 && declared.On == nil && defined[declared.Name]
}

func isAScenario(declared modulerules.Declaration) bool {
	return declared.Kind == modulerules.FuncDeclaration && declared.Exported &&
		len(declared.Parameters) == 1 && declared.Parameters[0].Type == theScenarioParameter &&
		isNamedAsAScenario(declared.Name)
}

func isNamedAsAScenario(name string) bool {
	condition, outcome, underscored := strings.Cut(name, "_")

	return underscored && condition != "" && !strings.Contains(outcome, "_") &&
		strings.HasPrefix(outcome, theOutcomePrefix) && outcome != theOutcomePrefix
}

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

func isAStep(line modulerules.Statement, scenarioParameter string) bool {
	_, opened := theStepKindOf(line, scenarioParameter)

	return opened
}

// A Wait chained on a Given's line is opened on that declaration, so the kind
// read here is the Given's, which is why the ordering rule reads a Wait off its
// outermost call instead.
func theStepKindOf(line modulerules.Statement, scenarioParameter string) (StepKind, bool) {
	for called := &line; called != nil; called = called.On {
		if kind, opened := opensTheStep(called, scenarioParameter); opened {
			return kind, true
		}
	}

	return "", false
}

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

// WaitFor carries both ways of writing a Wait.
func theOpenerOf(kind StepKind) string {
	if kind == WaitStep {
		return theWaitOpener
	}

	return string(kind)
}

func isTheList(declared modulerules.Declaration) bool {
	return declared.Kind == modulerules.FuncDeclaration && declared.Exported &&
		declared.Name == theListFunction && len(declared.Parameters) == 0
}

func whatThePlaceIsMissing(t *testing.T, place string, files []modulerules.GoFile) []string {
	t.Helper()

	held := theScenariosIn(files)

	var missing []string
	if len(held) == 0 {
		missing = append(missing, fmt.Sprintf("  %s holds no Scenario", place))
	}

	missing = append(missing, testFilesIn(t, place)...)
	missing = append(missing, scenariosTheListDoesNotNameOnce(place, files, held)...)

	named := readTheReadme(t, place)
	for _, declared := range held {
		scenarioName, err := scenarioNameOf(declared.Name)
		if err != nil {
			missing = append(missing, fmt.Sprintf("  %s holds a Scenario that would run under no name: %v", place, err))

			continue
		}
		if !strings.Contains(named, theReadmeEntryOf(scenarioName)) {
			missing = append(missing, fmt.Sprintf("  %s names no Scenario %s, the Scenario name of %s", theReadme, scenarioName, declared.Name))
		}
	}
	for _, file := range files {
		missing = append(missing, machineryImportedBy(file)...)
	}

	return missing
}

func scenariosTheListDoesNotNameOnce(place string, files []modulerules.GoFile, held []modulerules.Declaration) []string {
	listed, found := theListIn(files)
	if !found {
		return []string{fmt.Sprintf("  %s holds no %s declaring the exported %s", place, theList, theListFunction)}
	}

	var missing []string
	for _, declared := range held {
		if named := timesNamed(listed, declared.Name); named != onlyOnce {
			missing = append(missing, fmt.Sprintf("  %s names the Scenario %s %d times, not once",
				theList, declared.Name, named))
		}
	}

	return missing
}

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

// The list is read by identifier, so a name inside a longer one is not mistaken
// for the Scenario.
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

func identifiersIn(line string) []string {
	return strings.FieldsFunc(line, func(character rune) bool {
		return !unicode.IsLetter(character) && !unicode.IsDigit(character) && character != '_'
	})
}

// A Scenario is itself the check, so a test beside it could only repeat its
// Then Steps; the Scenarios run in the command's own suite instead.
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

func readTheReadme(t *testing.T, place string) string {
	t.Helper()

	named, err := os.ReadFile(filepath.Join(place, theReadme))
	if err != nil {
		t.Errorf("read the README naming every Scenario: %v", err)
	}

	return string(named)
}

// Read on scratch sources, since no shipped Scenario reaches full access.
func TestTheImportRule_ShouldAdmitTheLabsThreePackagesAndThePodSettingsPackageAlone(t *testing.T) {
	written := map[string]struct {
		imported []string
		admitted bool
	}{
		"the lab's three packages": {
			imported: theScenarioImports,
			admitted: true,
		},
		"the Pod settings full access reaches, beside them": {
			imported: append(slices.Clone(theScenarioImports), thePodSettingsImport),
			admitted: true,
		},
		"another Kubernetes package beside them": {
			imported: append(slices.Clone(theScenarioImports), theKubernetesClientImport),
			admitted: false,
		},
		"the domain package behind them": {
			imported: append(slices.Clone(theScenarioImports), theDomainImport),
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

// The three packages alone are what make every name a Scenario writes one the
// lab offers it; the Pod settings package is the single exception, for full
// access.
func machineryImportedBy(source modulerules.GoFile) []string {
	admitted := []string{theStepSurface, thePodPackage, theNodePackage, thePodSettingsPackage}

	var offenders []string
	for _, imported := range source.Imports {
		if slices.Contains(admitted, imported) {
			continue
		}
		offenders = append(offenders, fmt.Sprintf("  %s imports %s, which is none of lab, pod and node nor the Pod settings full access reaches",
			source.Path, imported))
	}

	return offenders
}

// A Scenario's three subjects then form no cycle and each reads on its own.
func TestThePodAndNodePackages_ShouldImportNeitherTheStepSurfaceNorEachOther(t *testing.T) {
	root, err := modulerules.Root(t.Context(), ownPackageDir)
	if err != nil {
		t.Fatalf("locate the root of the module: %v", err)
	}
	sources, err := modulerules.ScanGoFiles(filepath.Join(root, theLabsOwnPackageDir), modulerules.ProductionGoFilesOnly)
	if err != nil {
		t.Fatalf("read the sources of the lab's three packages: %v", err)
	}

	offenders := backEdgesAmong(sources)

	expect.Expect(t, offenders).To(expect.HaveLen[string](0))
}

func backEdgesAmong(sources []modulerules.GoFile) []string {
	refused := map[string][]string{
		thePodName:  {theStepSurface, theNodePackage},
		theNodeName: {theStepSurface, thePodPackage},
	}

	var offenders []string
	for _, source := range sources {
		for _, imported := range source.Imports {
			if slices.Contains(refused[source.Package], imported) {
				offenders = append(offenders, fmt.Sprintf("  %s imports %s", source.Path, imported))
			}
		}
	}

	return offenders
}

// The Even spread is one return of the topology spread constraint over the key
// it is given, at an even skew, leaving a Pod no Node can take Pending and
// counting only the Nodes whose taints the Pods tolerate.
func TestTheEvenSpread_ShouldHoldOneTopologySpreadOverTheSubjectItIsGiven(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := evenSpreadsNotAsDefined(filesOfThePlace(t, place))

	if len(offenders) > 0 {
		t.Errorf("%s holds one exported function %s(topologyKey string) returning pod.%s{TopologyKey: topologyKey, "+
			"MaxSkew: %s, WhenUnsatisfiable: pod.DoNotSchedule, NodeTaintsPolicy: pod.Honor} and nothing else, but:\n%s",
			thePodSpecifications, theEvenSpread, theSpreadSetting, theEvenSkew, strings.Join(offenders, "\n"))
	}
}

func TestTheEvenSpreadRule_ShouldAdmitTheOneEvenSpreadAndReportAnythingElse(t *testing.T) {
	const (
		aSpreadAtAWiderSkew = `func EvenSpreadOver(topologyKey string) pod.TopologySpreadConstraint {
	return pod.TopologySpreadConstraint{
		TopologyKey: topologyKey, MaxSkew: 2, WhenUnsatisfiable: pod.DoNotSchedule, NodeTaintsPolicy: pod.Honor,
	}
}`
		aSpreadPlacingPastItsSkew = `func EvenSpreadOver(topologyKey string) pod.TopologySpreadConstraint {
	return pod.TopologySpreadConstraint{
		TopologyKey: topologyKey, MaxSkew: 1, WhenUnsatisfiable: pod.ScheduleAnyway, NodeTaintsPolicy: pod.Honor,
	}
}`
		aSpreadCountingEveryNode = `func EvenSpreadOver(topologyKey string) pod.TopologySpreadConstraint {
	return pod.TopologySpreadConstraint{
		TopologyKey: topologyKey, MaxSkew: 1, WhenUnsatisfiable: pod.DoNotSchedule, NodeTaintsPolicy: pod.Ignore,
	}
}`
		aSpreadOverASubjectOfItsOwn = `func EvenSpreadOver(topologyKey string) pod.TopologySpreadConstraint {
	return pod.TopologySpreadConstraint{
		TopologyKey: node.Zone, MaxSkew: 1, WhenUnsatisfiable: pod.DoNotSchedule, NodeTaintsPolicy: pod.Honor,
	}
}`
		aSpreadLeavingOutAField = `func EvenSpreadOver(topologyKey string) pod.TopologySpreadConstraint {
	return pod.TopologySpreadConstraint{TopologyKey: topologyKey, MaxSkew: 1, WhenUnsatisfiable: pod.DoNotSchedule}
}`
		aSpreadWrittenByPosition = `func EvenSpreadOver(topologyKey string) pod.TopologySpreadConstraint {
	return pod.TopologySpreadConstraint{topologyKey, 1, pod.DoNotSchedule, pod.Honor}
}`
		aSpreadTakingASecondValue = `func EvenSpreadOver(topologyKey string, maxSkew int) pod.TopologySpreadConstraint {
	return pod.TopologySpreadConstraint{
		TopologyKey: topologyKey, MaxSkew: maxSkew, WhenUnsatisfiable: pod.DoNotSchedule, NodeTaintsPolicy: pod.Honor,
	}
}`
		aSpreadGroupedWithAnother = `func EvenSpreadOver(topologyKey string) pod.Customization {
	return pod.Settings(pod.TopologySpreadConstraint{
		TopologyKey: topologyKey, MaxSkew: 1, WhenUnsatisfiable: pod.DoNotSchedule, NodeTaintsPolicy: pod.Honor,
	}, pod.Request{CPU: 1, Memory: 1 * pod.Gi})
}`
	)

	written := map[string]struct {
		definitions []string
		admitted    bool
	}{
		"the Even spread":                    {definitions: []string{theEvenSpreadDefinition}, admitted: true},
		"no Even spread at all":              {},
		"a spread at a wider skew":           {definitions: []string{aSpreadAtAWiderSkew}},
		"a spread placing past its skew":     {definitions: []string{aSpreadPlacingPastItsSkew}},
		"a spread counting every Node":       {definitions: []string{aSpreadCountingEveryNode}},
		"a spread over a subject of its own": {definitions: []string{aSpreadOverASubjectOfItsOwn}},
		"a spread leaving out a field":       {definitions: []string{aSpreadLeavingOutAField}},
		"a spread written by position":       {definitions: []string{aSpreadWrittenByPosition}},
		"a spread taking a second value":     {definitions: []string{aSpreadTakingASecondValue}},
		"a spread grouped with another setting": {
			definitions: []string{aSpreadGroupedWithAnother},
		},
		"the Even spread defined twice": {
			definitions: []string{theEvenSpreadDefinition, theEvenSpreadDefinition},
		},
	}

	for shape, taken := range written {
		t.Run(shape, func(t *testing.T) {
			place := aPlaceHolding(t, map[string]string{thePodSpecifications: aPodSpecificationsSourceOf(taken.definitions...)})

			offenders := evenSpreadsNotAsDefined(place)

			expect.Expect(t, len(offenders) == 0).To(expect.Equal(taken.admitted))
		})
	}
}

func TestResilientPlacement_ShouldNameTheEvenSpreadOverZonesAndOverNodes(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := resilientPlacementNotNamingTheEvenSpread(filesOfThePlace(t, place))

	if len(offenders) > 0 {
		t.Errorf("%s defines %s naming %s(node.%s) and %s(node.%s) and writing no pod.%s itself, but:\n%s",
			thePodSpecifications, theResilientPlacement, theEvenSpread, theSpreadSubjectOfZone, theEvenSpread,
			theSpreadSubjectOfNode, theSpreadSetting, strings.Join(offenders, "\n"))
	}
}

func TestTheResilientPlacementRule_ShouldAdmitTheEvenSpreadNamedOverZonesAndNodesAndReportAnythingElse(t *testing.T) {
	const (
		namingTheZoneSpreadAlone = `func ResilientPlacement() pod.Specification {
	return pod.Predefined("Resilient placement", pod.StartupMillisBetween(500, 1000),
		` + theNamedSettingWritten + `,
		EvenSpreadOver(node.Zone),
	)
}`
		writingTheZoneSpreadItself = `func ResilientPlacement() pod.Specification {
	return pod.Predefined("Resilient placement", pod.StartupMillisBetween(500, 1000),
		` + theNamedSettingWritten + `,
		pod.TopologySpreadConstraint{TopologyKey: node.Zone, MaxSkew: 1, WhenUnsatisfiable: pod.DoNotSchedule, NodeTaintsPolicy: pod.Honor},
		EvenSpreadOver(node.Hostname),
	)
}`
		writingASpreadBesideBoth = `func ResilientPlacement() pod.Specification {
	return pod.Predefined("Resilient placement", pod.StartupMillisBetween(500, 1000),
		` + theNamedSettingWritten + `,
		EvenSpreadOver(node.Zone),
		EvenSpreadOver(node.Hostname),
		pod.TopologySpreadConstraint{TopologyKey: node.Zone, MaxSkew: 2, WhenUnsatisfiable: pod.DoNotSchedule, NodeTaintsPolicy: pod.Honor},
	)
}`
		namingTheSpreadOverAKeyOfItsOwn = `func ResilientPlacement() pod.Specification {
	return pod.Predefined("Resilient placement", pod.StartupMillisBetween(500, 1000),
		` + theNamedSettingWritten + `,
		EvenSpreadOver("topology.kubernetes.io/zone"),
		EvenSpreadOver(node.Hostname),
	)
}`
	)

	written := map[string]struct {
		definitions []string
		admitted    bool
	}{
		"the Even spread over Zones and over Nodes": {
			definitions: []string{theResilientPlacementDefinition},
			admitted:    true,
		},
		"no Resilient placement at all":         {},
		"the Even spread over Zones alone":      {definitions: []string{namingTheZoneSpreadAlone}},
		"the Zone spread written itself":        {definitions: []string{writingTheZoneSpreadItself}},
		"a spread written beside both":          {definitions: []string{writingASpreadBesideBoth}},
		"the Even spread over a key of its own": {definitions: []string{namingTheSpreadOverAKeyOfItsOwn}},
	}

	for shape, taken := range written {
		t.Run(shape, func(t *testing.T) {
			place := aPlaceHolding(t, map[string]string{
				thePodSpecifications: aPodSpecificationsSourceOf(
					append([]string{theNamedSettingDefinition, theEvenSpreadDefinition}, taken.definitions...)...),
			})

			offenders := resilientPlacementNotNamingTheEvenSpread(place)

			expect.Expect(t, len(offenders) == 0).To(expect.Equal(taken.admitted))
		})
	}
}

func evenSpreadsNotAsDefined(files []modulerules.GoFile) []string {
	var offenders []string
	held := 0
	for _, file := range files {
		if filepath.Base(file.Path) != thePodSpecifications {
			continue
		}
		for _, declared := range file.Declarations {
			if declared.Kind != modulerules.FuncDeclaration || declared.Name != theEvenSpread {
				continue
			}
			held++
			if why := whyNotTheEvenSpread(declared); why != "" {
				offenders = append(offenders, fmt.Sprintf("  %s, in %s: %s", file.Path, declared.Name, why))
			}
		}
	}
	if held != onlyOnce {
		offenders = append(offenders, fmt.Sprintf("  %s defines %s %d times", thePodSpecifications, theEvenSpread, held))
	}

	return offenders
}

func whyNotTheEvenSpread(declared modulerules.Declaration) string {
	if !declared.Exported || len(declared.Parameters) != onlyOnce || len(declared.Statements) != onlyOnce {
		return "is not an exported function of one value whose body is one statement"
	}
	returned, isAReturn := strings.CutPrefix(declared.Statements[0].Written, theReturn)
	if !isAReturn {
		return "does not return its spread"
	}
	parsed, err := parser.ParseExpr(returned)
	if err != nil {
		return "does not return an expression"
	}
	fields, isASpread := keyedFieldsOf(parsed, thePodName, theSpreadSetting)
	if !isASpread || len(fields) != theSpreadFields {
		return fmt.Sprintf("does not return one pod.%s with a key beside each of its %d values", theSpreadSetting, theSpreadFields)
	}
	overTheKeyGiven, isAName := fields["TopologyKey"].(*ast.Ident)
	evenSkew, isALiteral := fields["MaxSkew"].(*ast.BasicLit)
	if !isAName || overTheKeyGiven.Name != declared.Parameters[0].Name ||
		!isALiteral || evenSkew.Value != theEvenSkew ||
		!namesThe(fields["WhenUnsatisfiable"], thePodName, "DoNotSchedule") ||
		!namesThe(fields["NodeTaintsPolicy"], thePodName, "Honor") {
		return fmt.Sprintf("does not spread over the key it is given at MaxSkew %s, pod.DoNotSchedule and pod.Honor", theEvenSkew)
	}

	return ""
}

func resilientPlacementNotNamingTheEvenSpread(files []modulerules.GoFile) []string {
	var offenders []string
	for _, file := range files {
		if filepath.Base(file.Path) != thePodSpecifications {
			continue
		}
		for _, declared := range file.Declarations {
			_, call, isADefinition := theNameDefinedBy(declared, thePodName, thePodSpecificationDefinition)
			if !isADefinition || declared.Name != theResilientPlacement {
				continue
			}

			return whereResilientPlacementDepartsFromTheEvenSpread(file.Path, call.Args)
		}
	}

	return append(offenders, fmt.Sprintf("  %s defines no %s", thePodSpecifications, theResilientPlacement))
}

func whereResilientPlacementDepartsFromTheEvenSpread(path string, settings []ast.Expr) []string {
	var offenders []string
	for _, subject := range theSpreadSubjects {
		if !slices.ContainsFunc(settings, func(setting ast.Expr) bool { return namesTheEvenSpreadOver(setting, subject) }) {
			offenders = append(offenders, fmt.Sprintf("  %s, in %s: names no %s(node.%s)",
				path, theResilientPlacement, theEvenSpread, subject))
		}
	}
	for _, setting := range settings {
		if isALiteralOf(setting, thePodName, theSpreadSetting) {
			offenders = append(offenders, fmt.Sprintf("  %s, in %s: writes a pod.%s itself",
				path, theResilientPlacement, theSpreadSetting))
		}
	}

	return offenders
}

func namesTheEvenSpreadOver(setting ast.Expr, subject string) bool {
	named, isACall := setting.(*ast.CallExpr)
	if !isACall || len(named.Args) != onlyOnce {
		return false
	}
	function, isAName := named.Fun.(*ast.Ident)

	return isAName && function.Name == theEvenSpread && namesThe(named.Args[0], theNodeName, subject)
}

// A keyed literal means no value is read by its position.
func TestEveryScenarioSource_ShouldWriteEachValueOfASettingBesideItsKubernetesName(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := settingsWrittenByPosition(t, filesOfThePlace(t, place))

	if len(offenders) > 0 {
		t.Errorf("every literal of a pod or node type a Scenario writes keys each value by its field's name, but:\n%s",
			strings.Join(offenders, "\n"))
	}
}

func TestTheKeyedSettingRule_ShouldAdmitAKeyedLiteralAndReportOneWrittenByPosition(t *testing.T) {
	written := map[string]struct {
		line     string
		admitted bool
	}{
		"a keyed Request": {
			line: `s.Given().Workload(Checkout, lab.Pods(9), ResilientPlacement().Customize(` +
				`pod.Request{CPU: 3, Memory: 12 * pod.Gi}))`,
			admitted: true,
		},
		"a keyed Node capacity": {
			line: `s.Given().Topology(lab.Zones(3), node.Pool("general", node.PerZone(3), ` +
				`node.Capacity{CPU: 4, Memory: 16 * node.Gi, Pods: 10}))`,
			admitted: true,
		},
		"a Request by position": {
			line: `s.Given().Workload(Checkout, lab.Pods(9), ResilientPlacement().Customize(` +
				`pod.Request{3, 12 * pod.Gi}))`,
		},
		"a Node capacity by position": {
			line: `s.Given().Topology(lab.Zones(3), node.Pool("general", node.PerZone(3), ` +
				`node.Capacity{4, 16 * node.Gi, 10}))`,
		},
		"a toleration by position": {
			line: `s.Given().Workload(Checkout, lab.Pods(9), ResilientPlacement().Customize(` +
				`pod.Toleration{node.NotReady, pod.Exists, pod.NoExecute, 300}))`,
		},
	}

	for shape, taken := range written {
		t.Run(shape, func(t *testing.T) {
			offenders := settingsWrittenByPosition(t, aScenarioWhoseStepsAre(t, taken.line))

			expect.Expect(t, len(offenders) == 0).To(expect.Equal(taken.admitted))
		})
	}
}

// The files are parsed again here, since a literal is no name a declaration
// uses.
func settingsWrittenByPosition(t *testing.T, files []modulerules.GoFile) []string {
	t.Helper()

	positions := token.NewFileSet()
	var offenders []string
	for _, file := range files {
		parsed, err := parser.ParseFile(positions, file.Path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", file.Path, err)
		}
		ast.Inspect(parsed, func(visited ast.Node) bool {
			literal, isALiteral := visited.(*ast.CompositeLit)
			if isALiteral && isATypeOfAPodOrANode(literal.Type) && !keysEveryValue(literal) {
				offenders = append(offenders, fmt.Sprintf("  %s: writes a value of a setting without its name",
					positions.Position(literal.Pos())))
			}

			return true
		})
	}

	return offenders
}

func isATypeOfAPodOrANode(written ast.Expr) bool {
	selected, isSelected := written.(*ast.SelectorExpr)
	if !isSelected {
		return false
	}
	qualifier, isAName := selected.X.(*ast.Ident)

	return isAName && (qualifier.Name == thePodName || qualifier.Name == theNodeName)
}

func keysEveryValue(literal *ast.CompositeLit) bool {
	return !slices.ContainsFunc(literal.Elts, func(element ast.Expr) bool {
		_, isKeyed := element.(*ast.KeyValueExpr)

		return !isKeyed
	})
}
