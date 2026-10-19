// The two rules in this file govern the place the lab's Scenarios sit in: that
// every declaration there is a Scenario, a Workload name, a Named Node capacity,
// a predefined Pod specification, a Named setting or the list, each in the file
// admitted for it; and that the place holds every Scenario under a name it can
// run, a README naming them, no test file and nothing of the machinery. They
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

// theNodeLabelKeys is the file of the place where the lab's own Scenarios take
// the keys of the Node labels their NodePools state, and their Thens and spreads
// count over, from. In it, every declaration is an exported untyped string
// constant, so the file holds keys and no machinery, per
// docs/adr/2026-10-18-a-nodepool-states-its-own-node-labels-beside-the-labs.md.
const theNodeLabelKeys = "labels.go"

// theTaints is the file of the place where the lab's own Scenarios take the keys
// and values of the taints their NodePools state, and their tolerations name,
// from. It holds exported untyped string constants alone, as theNodeLabelKeys
// does, per
// docs/adr/2026-10-19-a-nodepool-states-noschedule-taints-and-a-later-workload-deploys-beside-the-earlier.md.
const theTaints = "taints.go"

// theNamedNodeCapacities is the file of the place where the lab's own Scenarios
// take the Named Node capacities they name from, the lab defining none. In it,
// every declaration is an exported function taking nothing whose body is one
// return of a call to the node package's theNamedNodeCapacityDefinition, the name
// written first as a string literal, so the file holds definitions and no
// machinery, and each name has the one definition a Scenario reaches it by.
const (
	theNamedNodeCapacities         = "nodes.go"
	theNamedNodeCapacityDefinition = "Named"
	theReturn                      = "return "
)

// theDomainsName and theDomainsNamedNodeCapacity are the package and the
// constructor the node package builds a Named Node capacity through, and
// theLabsSourceRoots the directories under the module root whose sources are the
// lab itself rather than its Scenarios. No source there defines a Named Node
// capacity: only theNamedCapacitysDomainValue, the node package's one
// declaration building the domain value of a capacity a Scenario named, calls
// the domain's constructor.
const (
	theDomainsName               = "scenario"
	theDomainsNamedNodeCapacity  = "NewNamedNodeCapacity"
	theNamedCapacitysDomainValue = "declared"
)

var theLabsSourceRoots = []string{"internal", "cmd"}

// thePodSpecifications is the file of the place where the lab's own Scenarios
// take the predefined Pod specifications they declare from, the lab defining
// none. In it, every declaration is an exported function taking nothing whose
// body is one return of a call to the pod package's thePodSpecificationDefinition,
// the name written first as a string literal and the Startup time second, or one
// of theNamedSettings, so the file holds definitions and no machinery.
// theDomainsPodSpecification is the constructor that definition builds the
// domain value through, which no other source of the lab calls.
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

// theEvenSpread is the Named setting for one spread the Scenarios define beside
// their specifications, theSpreadSetting the setting it holds, and theSpreadFields
// how many fields that setting writes. theResilientPlacement is the definition
// naming it once over each of theSpreadSubjects, the node package's labels for
// a Zone and for a Node.
const (
	theEvenSpread          = "EvenSpreadOver"
	theSpreadSetting       = "TopologySpreadConstraint"
	theSpreadFields        = 4
	theEvenSkew            = "1"
	theResilientPlacement  = "ResilientPlacement"
	theSpreadSubjectOfZone = "Zone"
	theSpreadSubjectOfNode = "Hostname"
)

// theNamedSettings are the functions the file of Pod specifications holds beside
// them; theLabsTaints are the two taints, as the node package names them, that
// the Named setting every definition names tolerates by the lab's waiting time,
// and that no definition writes a toleration of itself; theStartupForms are the
// four calls a definition names its Startup time with.
var (
	theNamedSettings  = []string{theNamedSetting, theEvenSpread}
	theSpreadSubjects = []string{theSpreadSubjectOfZone, theSpreadSubjectOfNode}
	theLabsTaints     = []string{"NotReady", "Unreachable"}
	theStartupForms   = []string{"StartupSeconds", "StartupSecondsBetween", "StartupMillis", "StartupMillisBetween"}
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

// theStepSurface is the package a Scenario's Steps are written against: the lab's
// own Step surface, this package. thePodPackage and theNodePackage are the two
// beneath it a Scenario imports beside it, holding what a Pod carries and what a
// Node is, per docs/adr/2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md.
const (
	theStepSurface = "github.com/woditschka/k8s-resilience-lab/internal/lab"
	thePodPackage  = theStepSurface + "/pod"
	theNodePackage = theStepSurface + "/node"
)

// theStepSurfaceName, thePodName and theNodeName are the names a Scenario source
// reaches those three packages under, the subjects every name it writes reads
// by, and theScenarioParameter is what a Scenario function takes and its every
// Step is called on, as a source writes it. A source reaching the surface under
// another name is reported as holding no Scenario, which is the same finding by
// a different route.
const (
	theStepSurfaceName   = "lab"
	thePodName           = "pod"
	theNodeName          = "node"
	theScenarioParameter = "*" + theStepSurfaceName + ".Scenario"
)

// theSubjects are the names of the three packages a Scenario reaches the lab
// through.
var theSubjects = []string{theStepSurfaceName, thePodName, theNodeName}

// theLabsOwnPackageDir is where the three packages' own sources sit under the
// module root, which the names they offer Scenarios are read off.
const theLabsOwnPackageDir = "internal/lab"

// thePodSettingsPackage is the one Kubernetes package a Scenario may import
// beside the lab's three: the one naming the Pod settings a Scenario reaches
// through full access, where the lab names none. It is the single
// stated exception to the one-vocabulary rule, carried by the depguard rule
// scenarios-hold-steps-alone and by the import rule below together.
// thePodSettingsName is the name the lab's own sources reach it under.
const (
	thePodSettingsPackage = "k8s.io/api/core/v1"
	thePodSettingsName    = "corev1"
)

// The imports a scratch Scenario source is written with, each as a source writes
// it: the lab's three packages, the Pod settings package, a Kubernetes package
// that is neither, and the domain package the lab's packages keep behind them.
const (
	theStepSurfaceImport      = `"` + theStepSurface + `"`
	thePodImport              = `"` + thePodPackage + `"`
	theNodeImport             = `"` + theNodePackage + `"`
	thePodSettingsImport      = thePodSettingsName + ` "` + thePodSettingsPackage + `"`
	theKubernetesClientImport = `"k8s.io/client-go/kubernetes"`
	theDomainImport           = `"github.com/woditschka/k8s-resilience-lab/internal/scenario"`
)

// theScenarioImports are the imports a Scenario of the lab writes.
var theScenarioImports = []string{theStepSurfaceImport, theNodeImport, thePodImport}

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

// aFileOfDefinitionsShape is a file of Named Node capacities or of predefined
// Pod specifications holding its definitions, and theStandardNodesDefinition the
// one Named Node capacity the lab's own place holds, which theTopologyLine names.
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

// theResilientPlacementDefinition is the one definition of a predefined Pod
// specification the scratch places below hold, which every Workload line written
// there declares, naming the Even spread over Zones and over Nodes, and
// theNamedSettingWritten the Named setting every definition names for its Pods
// leaving an unresponsive Node.
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

// theNamedSettingDefinition and theEvenSpreadDefinition are the two functions of
// the place's file of Pod specifications that are no specification: the Named
// setting every definition names, one pod.Settings call of the two tolerations,
// and the Even spread Resilient placement names over Zones and over Nodes.
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
		t.Errorf("a Scenario source holds one exported Scenario function named %s whose every line is a Given, When, Wait or Then Step, and the four files beside them declare the Workload names, the Named Node capacities, the predefined Pod specifications and the list alone, but:\n%s",
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

// The rule's own decisions over the files of Node label keys and of taints are
// read here, since the shipped files follow it.
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

// Every Scenario of the lab running on one NodePool names it general, so the
// lab's reports read alike; a Scenario about NodePools, declaring several, is
// the one that departs from it and names each after what it holds (docs/prd.md
// REQ-SIM-013, REQ-SIM-017).
func TestEveryScenarioOfTheLab_ShouldDeclareANodePoolAndNameItGeneralWhenItDeclaresOnlyOne(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := scenariosDeclaringNoNodePoolOrOneNotNamedGeneral(filesOfThePlace(t, place))

	if len(offenders) > 0 {
		t.Errorf("every Scenario of the lab declares a NodePool, and names its one NodePool %s, but:\n%s",
			theLabsNodePoolName, strings.Join(offenders, "\n"))
	}
}

// The rule's own decisions are read here, since the shipped sources all follow it.
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

// theNodePoolStep is how a NodePool's declaration names itself under the node
// package, and theLabsNodePoolName is the name the lab's own Scenarios give their
// one.
const (
	theNodePoolStep     = "Pool"
	theLabsNodePoolName = `"general"`
)

// scenariosDeclaringNoNodePoolOrOneNotNamedGeneral names every Scenario of the
// place that declares no NodePool, or one NodePool under another name. The
// NodePool call is found wherever in a line it is written, since it is an
// argument of the Topology line.
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

// namesTheLabsNodePool reports whether one NodePool call names its NodePool
// general.
func namesTheLabsNodePool(pool modulerules.Statement) bool {
	return len(pool.Arguments) > 0 && pool.Arguments[0].Written == theLabsNodePoolName
}

// nodePoolCallsIn is every call to the NodePool declaration, node.Pool, inside one
// expression: on the expression it is called on, and in the arguments of every
// call.
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

// Every NodePool of the lab's Scenarios states its Node capacity or names one the
// place defines, so a reader of the Scenario sees every value its Nodes hold or
// a name whose one definition sits beside it.
func TestEveryScenarioOfTheLab_ShouldStateOrNameEachNodePoolsCapacity(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := nodePoolsNeitherStatingNorNamingTheirCapacity(filesOfThePlace(t, place))

	if len(offenders) > 0 {
		t.Errorf("every NodePool of the lab's Scenarios writes %s...} or a function %s declares as its capacity, but:\n%s",
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

// theStatedCapacity opens the literal the node package offers a NodePool
// stating its capacity by, and theCapacityArgument is where the NodePool call
// takes its capacity: after its name and its Node count.
const (
	theStatedCapacity   = theNodeName + ".Capacity{"
	theCapacityArgument = 2
)

// nodePoolsNeitherStatingNorNamingTheirCapacity names every NodePool of a Scenario
// of the place whose capacity argument is neither a node.Capacity literal nor a
// call of a function the place's file of Named Node capacities declares, or
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
// the literal the node package offers for stating one, or as an unqualified call
// of one of the Named Node capacities the place defines.
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
// line, and no source of the lab itself defines one, the node package's own
// building of a named capacity being the one call of the domain's constructor
// (docs/prd.md REQ-SIM-015).
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

// The rule's own decisions over the place are read here, since the shipped file
// writes only the shape it admits.
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

// The rule's own decisions over the lab's sources are read here, since the
// shipped sources hold only the node package's own building of a named capacity.
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

// namedNodeCapacitiesNotDefinedOnceBesideTheScenarios names every declaration of
// the place's file of Named Node capacities that is not one definition, every
// name defined more than once there, and every other file of the place calling
// the node package's definition itself.
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

// declarationsThatAreNotNamedNodeCapacities names every declaration of the file
// of Named Node capacities that is not one definition: the file holds the
// capacities the lab's Scenarios name and nothing else.
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

// definitionsOutsideTheNamedNodeCapacities names every declaration of another
// file of the place that calls the node package's definition itself.
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

// theNameOf is a name one of the lab's packages offers, as a source of the place
// writes it.
func theNameOf(qualifier, name string) modulerules.UsedName {
	return modulerules.UsedName{Qualifier: qualifier, Name: name}
}

// theNameDefinedBy is the name a declaration of a file of definitions defines,
// the call defining it, and whether it is one definition at all: an exported
// function taking nothing whose one statement returns a call of the definition
// the package qualified offers, its first argument the name as a string literal.
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

// callsThe reports whether call is the function of that name the package
// qualified offers, as a source of the place writes it: <qualifier>.<name>.
func callsThe(call *ast.CallExpr, qualifier, name string) bool {
	return namesThe(call.Fun, qualifier, name)
}

// namesThe reports whether an expression is the name of that name the package
// qualified offers, <qualifier>.<name>, qualified by nothing else.
func namesThe(expression ast.Expr, qualifier, name string) bool {
	selected, isSelected := expression.(*ast.SelectorExpr)
	if !isSelected || selected.Sel.Name != name {
		return false
	}
	written, isAName := selected.X.(*ast.Ident)

	return isAName && written.Name == qualifier
}

// keyedFieldsOf is every field a literal of the type the package qualified
// offers under that name sets, by its key, and whether written is such a literal
// with a key beside every value.
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

// isALiteralOf reports whether written is a literal of the type the package
// qualified offers under that name, keyed or not.
func isALiteralOf(written ast.Expr, qualifier, name string) bool {
	literal, isALiteral := written.(*ast.CompositeLit)

	return isALiteral && namesThe(literal.Type, qualifier, name)
}

// callsDefiningANamedNodeCapacityOfTheLabsOwn names every declaration of the
// lab's own sources that defines a Named Node capacity: one calling the node
// package's definition, from inside that package or out of it, and one calling
// the domain's constructor but the node package's building of a named capacity.
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
		return used.Qualifier == theNodeName || (used.Qualifier == "" && declaredIn == theNodeName)
	case theDomainsNamedNodeCapacity:
		isTheNodePackagesOwn := declaredIn == theNodeName &&
			declared.Name == theNamedCapacitysDomainValue && used.Qualifier == theDomainsName

		return !isTheNodePackagesOwn && (used.Qualifier == theDomainsName || declaredIn == theDomainsName)
	default:
		return false
	}
}

// Each predefined Pod specification the lab's Scenarios declare is defined
// once, in the place's file of them, and nowhere else: no Scenario defines one on
// its own line, and no source of the lab itself defines one, the pod package's
// own mechanism being the one call of the domain's constructor (docs/prd.md
// REQ-SIM-015, edge case 3 included).
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

// Every definition shows the whole of what it holds: the Startup time its Pods
// take, named by one of the four Startup forms, and the Named setting standing
// for both tolerations the lab holds, in place of writing either itself, so a
// reader of the definition sees that its Pods leave an unresponsive Node after the
// lab's waiting time (docs/prd.md REQ-SIM-009, REQ-SIM-015 and REQ-SIM-016).
// Leaving the Startup time out does not compile, since it is the mechanism's
// second argument.
func TestEveryPredefinedPodSpecification_ShouldNameTheSettingForLeavingAnUnresponsiveNodeAndWriteNeitherToleration(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := podSpecificationsNotShowingWhatTheyHold(filesOfThePlace(t, place))

	if len(offenders) > 0 {
		t.Errorf("each definition of %s names its Startup time with one of pod.%s, names %s() and writes no pod.%s of the lab's taints, but:\n%s",
			thePodSpecifications, strings.Join(theStartupForms, ", pod."), theNamedSetting, theTolerationSetting,
			strings.Join(offenders, "\n"))
	}
}

// The rule's own decisions over the place are read here, since the shipped file
// writes only the shape it admits.
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

// The rule's own decisions over the lab's sources are read here, since the
// shipped sources hold only the mechanism's own call of the domain's constructor.
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

// podSpecificationsNotDefinedOnceBesideTheScenarios names every declaration of
// the place's file of predefined Pod specifications that is not one definition,
// every name defined more than once there, and every other file of the place
// calling the pod package's mechanism itself.
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

// declarationsThatAreNotPodSpecifications names every declaration of the file
// of predefined Pod specifications that is not one definition: the file holds
// the specifications the lab's Scenarios declare and nothing else.
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

// isTheNamedSetting reports whether a declaration is the Named setting for
// leaving an unresponsive Node, and isANamedSetting whether it is one of the
// functions the file of Pod specifications holds beside them, by its name. Their
// shape is the Named setting rule's and the Even spread rule's to read.
func isTheNamedSetting(declared modulerules.Declaration) bool {
	return declared.Kind == modulerules.FuncDeclaration && declared.Name == theNamedSetting
}

func isANamedSetting(declared modulerules.Declaration) bool {
	return declared.Kind == modulerules.FuncDeclaration && slices.Contains(theNamedSettings, declared.Name)
}

// definitionsOutsideThePodSpecifications names every declaration of another
// file of the place that calls the pod package's mechanism itself, a Workload
// line writing one inline included.
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

// podSpecificationsNotShowingWhatTheyHold names every definition of the place's
// file of predefined Pod specifications that does not name its Startup time by a
// Startup form, does not name the Named setting for leaving an unresponsive Node,
// or writes a toleration of one of the lab's taints itself.
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

// The Named setting stands beside the specifications that name it, as one
// exported function taking nothing whose body is one return of pod.Settings over
// exactly the two tolerations of the lab's taints, each keyed by its taint,
// matching any value, with the effect that moves a Pod off a Node, at the lab's
// waiting time, and nothing else. The lab defines no group of its own, and no
// source of it calls pod.Settings (docs/prd.md REQ-SIM-016).
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

// The rule's own decisions over the Named setting are read here, since the
// shipped file writes only the shape it admits.
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

// The rule's own decisions over the lab's sources are read here, since no shipped
// source calls the grouping means.
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

// namedSettingsNotAsDefined names every way the place's file of Pod
// specifications fails to hold the Named setting as defined: not exactly one,
// not one return of pod.Settings over exactly the two tolerations of the lab's
// taints at the lab's waiting time, or another declaration of the place calling
// pod.Settings.
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

// whyNotTheNamedSetting is why a declaration named LeaveUnresponsiveNodes is not
// the Named setting, or the empty string where it is.
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

// isTheLabsTolerationOf reports whether a setting is the toleration of that
// taint the Named setting holds: a keyed literal of its four fields, keyed by
// the taint, matching any value, with the effect that moves a Pod off a Node, at
// the lab's waiting time.
func isTheLabsTolerationOf(setting ast.Expr, taint string) bool {
	fields, isAToleration := keyedFieldsOf(setting, thePodName, theTolerationSetting)

	return isAToleration && len(fields) == theTolerationFields &&
		namesThe(fields[theTolerationsKey], theNodeName, taint) &&
		namesThe(fields["Operator"], thePodName, "Exists") &&
		namesThe(fields["Effect"], thePodName, "NoExecute") &&
		namesThe(fields["TolerationSeconds"], theNodeName, "UnresponsiveWait")
}

// callsGroupingSettingsOfTheLabsOwn names every declaration of the lab's own
// sources that calls the grouping means: from outside the pod package as
// pod.Settings, and from inside it unqualified. The mechanism's declaration calls
// nothing by that name, so it is no offender.
func callsGroupingSettingsOfTheLabsOwn(files []modulerules.GoFile) []string {
	var offenders []string
	for _, file := range files {
		for _, declared := range file.Declarations {
			for _, used := range declared.Names {
				// A type declaring a field of that name is no call, so inside the pod
				// package only a function's unqualified use counts.
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

// namesAStartupForm reports whether a definition's Startup time argument is a
// call of one of the four Startup forms.
func namesAStartupForm(definition *ast.CallExpr) bool {
	if len(definition.Args) < theFirstSettingArgument {
		return false
	}
	startup, isACall := definition.Args[theStartupTimeArgument].(*ast.CallExpr)

	return isACall && slices.ContainsFunc(theStartupForms, func(form string) bool {
		return callsThe(startup, thePodName, form)
	})
}

// namesTheNamedSetting reports whether a setting a definition writes is
// LeaveUnresponsiveNodes(), the Named setting of the same place taking nothing.
func namesTheNamedSetting(setting ast.Expr) bool {
	named, isACall := setting.(*ast.CallExpr)
	if !isACall {
		return false
	}
	function, isAName := named.Fun.(*ast.Ident)

	return isAName && function.Name == theNamedSetting && len(named.Args) == 0
}

// tolerates reports whether a setting a definition writes is a pod.Toleration of
// that taint, Key: node.<taint>, whatever time it writes.
func tolerates(setting ast.Expr, taint string) bool {
	fields, isAToleration := keyedFieldsOf(setting, thePodName, theTolerationSetting)

	return isAToleration && namesThe(fields[theTolerationsKey], theNodeName, taint)
}

// thePodSpecificationsDeclaredIn are the names the place's file of predefined Pod
// specifications declares, which a Workload line declares its specification by.
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

// callsDefiningAPodSpecificationOfTheLabsOwn names every declaration of the lab's
// own sources that defines a Pod specification: one calling the pod package's
// mechanism, from inside that package or out of it, and one calling the domain's
// constructor from outside the domain but in the mechanism itself.
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

// definesAPodSpecification reports whether one name a declaration of package
// uses is a call defining a Pod specification there. The domain calling its own
// constructor, unqualified, is the domain's rule re-entered and no definition.
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

// theLabsOwnSources are the production sources of the lab itself, every one under
// theLabsSourceRoots, the pod and node packages included, which is where a Named
// Node capacity, a specification or a Named setting of the lab's own would be
// defined.
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

// theWorkloadLine is a Workload line every rule here admits, which a rule read on
// something other than the line itself is written with. theFullAccessLine is that
// line reaching a Pod setting the lab does not name, written the way a Scenario
// writes full access.
const (
	theWorkloadLine   = `s.Given().Workload(Checkout, lab.Pods(9), ResilientPlacement())`
	theFullAccessLine = `s.Given().Workload(Checkout, lab.Pods(9), ResilientPlacement().Customize(
		pod.FullAccess(func(spec *` + thePodSettingsName + `.PodSpec) {
			spec.PriorityClassName = "critical"
		}),
	))`
)

// aScenarioWhoseWorkloadLineIs is a Scenario source holding that one Workload
// line and importing the lab's three packages, which is what a Scenario of the
// lab needs, beside a file of predefined Pod specifications defining
// ResilientPlacement, as the lab's own place holds them.
func aScenarioWhoseWorkloadLineIs(t *testing.T, line string) []modulerules.GoFile {
	t.Helper()

	return aScenarioSourceThatImports(t, theScenarioImports, line)
}

// aScenarioAlsoImporting is a Scenario source importing that one path beside the
// lab's three packages, which is what a Scenario reaching a Pod setting through
// full access needs.
func aScenarioAlsoImporting(t *testing.T, imported, line string) []modulerules.GoFile {
	t.Helper()

	return aScenarioSourceThatImports(t, append(slices.Clone(theScenarioImports), imported), line)
}

// aScenarioSourceThatImports is a Scenario source written with those imports and
// that Workload line, scanned the way the rules scan the lab's own place.
func aScenarioSourceThatImports(t *testing.T, imported []string, line string) []modulerules.GoFile {
	t.Helper()

	return aPlaceHolding(t, map[string]string{
		theScenarioSource: fmt.Sprintf(theScenarioSourceShape, anImportBlockOf(imported), line),
		thePodSpecifications: aPodSpecificationsSourceOf(
			theNamedSettingDefinition, theEvenSpreadDefinition, theResilientPlacementDefinition),
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
// Steps, one per line, importing the lab's three packages.
func aScenarioSourceWhoseStepsAre(steps ...string) string {
	return fmt.Sprintf(theScenarioSourceShape, anImportBlockOf(theScenarioImports), strings.Join(steps, "\n\t"))
}

// aNamedNodeCapacitiesSourceOf is the text of a file of Named Node capacities
// holding these definitions, importing the pod and node packages.
func aNamedNodeCapacitiesSourceOf(definitions ...string) string {
	return fmt.Sprintf(aFileOfDefinitionsShape, strings.Join(definitions, "\n\n"))
}

// aPodSpecificationsSourceOf is the text of a file of predefined Pod
// specifications holding these definitions, importing the pod and node packages.
func aPodSpecificationsSourceOf(definitions ...string) string {
	return fmt.Sprintf(aFileOfDefinitionsShape, strings.Join(definitions, "\n\n"))
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
	theTopologyLine         = `s.Given().Topology(lab.Zones(3), node.Pool("general", node.PerZone(3), StandardNodes()))`
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
// as a Scenario writes them, importing the lab's three packages.
func aScenarioWhoseStepsAre(t *testing.T, steps ...string) []modulerules.GoFile {
	t.Helper()

	return aScenarioSourceThatImports(t, theScenarioImports, strings.Join(steps, "\n\t"))
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

// Every name a Scenario writes reads by its subject: the Scenario's own under
// lab, what a Pod carries under pod, what a Node is under node, each one its
// package exports, or a name the Scenarios define themselves. A name written
// unqualified, as a dot import would write it, is none of these (docs/prd.md
// REQ-SIM-016).
func TestEveryScenarioSource_ShouldWriteEachNameUnderItsSubjectOrAsOneTheScenariosDefine(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := namesUsedThatTheLabDoesNotOffer(t, filesOfThePlace(t, place))

	if len(offenders) > 0 {
		t.Errorf("every name a Scenario uses is one lab, pod or node exports under its own qualifier, or one the Scenarios define, but:\n%s",
			strings.Join(offenders, "\n"))
	}
}

// What the name rule reports is read here rather than on the lab's own place,
// which names only what it offers: a name its package does not export, a name
// under another subject's package, a name written unqualified, and one neither
// the lab nor the place declares at all.
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

// The names a Scenario reaching full access writes are read here rather than on
// the lab's own place, which holds no Scenario using full access: the Pod settings
// it names are the one thing a Scenario writes that the lab's packages do not
// offer it, and the placeholder the Scenario's own function literal declares is
// no name it writes at all.
func TestTheNameRule_ShouldAdmitTheKubernetesPodSettingsFullAccessNames(t *testing.T) {
	reaching := aScenarioAlsoImporting(t, thePodSettingsImport, theFullAccessLine)

	offenders := namesUsedThatTheLabDoesNotOffer(t, reaching)

	expect.Expect(t, offenders).To(expect.HaveLen[string](0))
}

// namesUsedThatTheLabDoesNotOffer names every name a file of the place uses that
// is neither one lab, pod or node exports under its own qualifier nor one the
// place declares itself. A name taken from a value, such as a Step called on the
// Scenario, is the compiler's own to check and admitted here.
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

// isOffered reports whether one name a declaration uses is one a Scenario may
// write: a name its subject's package exports, under that package's qualifier, a
// name the place declares, a parameter of the declaration using it, a name Go
// itself predeclares, or a name taken from a value, whose own type the compiler
// checks.
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

// theNamesEachSubjectOffers are the names lab, pod and node each export, read off
// their own sources and keyed by the package's name, which is what a Scenario may
// write under that qualifier.
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
	case theWorkloadNames, theNamedNodeCapacities, theNodeLabelKeys, theTaints, theList:
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
		case theNodeLabelKeys, theTaints:
			offenders = append(offenders, declarationsThatAreNotNodeLabelKeys(file)...)
		case thePodSpecifications:
			offenders = append(offenders, declarationsThatAreNotPodSpecifications(file)...)
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

// declarationsThatAreNotNodeLabelKeys names every declaration of the file of
// Node label keys that is not an exported constant of a string literal written
// without a type: the file holds the keys the lab's Scenarios state and nothing
// else. The scan keeps no constant's value, so the file is read again here.
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

// declaresNodeLabelKeysAlone reports whether one declaration is a group of
// exported constants each holding a string literal and naming no type.
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

// workloadLinesDeclaringNoPredefinedSpecification names every Workload line of
// the place that is not that shape, and every Scenario holding no Workload line
// at all: a Scenario deploys a Workload, so one declaring none would meet the
// shape rule by having nothing to check.
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
// of the predefined Pod specifications the place defines and nothing besides: the
// line ends at the Workload Step, or at one call of the Workload's own closed set
// written on it, and that Step takes the Workload name, the Pod count and that
// specification, itself one predefined call that zero or one Customize is
// written on.
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
// predefined specifications the place defines: an unqualified call taking
// nothing, of a function the place's file of them declares.
func isAPredefinedSpecification(declared modulerules.Statement, defined map[string]bool) bool {
	return declared.Call && len(declared.Arguments) == 0 && declared.On == nil && defined[declared.Name]
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
// where every source imports the lab's packages alone: no Scenario of the lab
// reaches full access, so the one admitted exception is exercised on a source of
// this test's own.
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

// machineryImportedBy names every import of a Scenario source that is none of the
// lab's three packages nor the Pod settings package: another package of the lab,
// the Kubernetes client, or anything else. The three packages alone are what
// make every name a Scenario writes one the lab offers it, which the name rule
// above reads on that footing; the Pod settings package is the single stated
// exception, for a Scenario reaching a setting through full access.
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

// The pod and node packages sit beneath the Step surface: it imports both, and
// neither imports it or the other, so a Scenario's three subjects form no cycle
// and each reads on its own (docs/adr/2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md).
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

// backEdgesAmong names every import of a pod or node source that reaches the
// Step surface or the other of the two.
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

// The Even spread stands beside the predefined Pod specifications as one exported
// function taking the topology key, whose body is one return of the topology
// spread constraint over that key, under its Kubernetes names, at an even skew,
// leaving a Pod no Node can take Pending, counting only the Nodes whose taints
// the Pods tolerate, and nothing else (docs/prd.md REQ-SIM-016).
func TestTheEvenSpread_ShouldHoldOneTopologySpreadOverTheSubjectItIsGiven(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := evenSpreadsNotAsDefined(filesOfThePlace(t, place))

	if len(offenders) > 0 {
		t.Errorf("%s holds one exported function %s(topologyKey string) returning pod.%s{TopologyKey: topologyKey, "+
			"MaxSkew: %s, WhenUnsatisfiable: pod.DoNotSchedule, NodeTaintsPolicy: pod.Honor} and nothing else, but:\n%s",
			thePodSpecifications, theEvenSpread, theSpreadSetting, theEvenSkew, strings.Join(offenders, "\n"))
	}
}

// The rule's own decisions over the Even spread are read here, since the shipped
// file writes only the shape it admits.
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

// Resilient placement names the Even spread over Zones and over Nodes, and writes
// no topology spread constraint itself, so its definition reads as the name, the
// Startup time, the Named setting for leaving an unresponsive Node, and the two
// Even spreads (docs/prd.md REQ-SIM-016).
func TestResilientPlacement_ShouldNameTheEvenSpreadOverZonesAndOverNodes(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := resilientPlacementNotNamingTheEvenSpread(filesOfThePlace(t, place))

	if len(offenders) > 0 {
		t.Errorf("%s defines %s naming %s(node.%s) and %s(node.%s) and writing no pod.%s itself, but:\n%s",
			thePodSpecifications, theResilientPlacement, theEvenSpread, theSpreadSubjectOfZone, theEvenSpread,
			theSpreadSubjectOfNode, theSpreadSetting, strings.Join(offenders, "\n"))
	}
}

// The rule's own decisions over Resilient placement are read here, since the
// shipped file writes only the shape it admits.
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

// evenSpreadsNotAsDefined names every way the place's file of Pod specifications
// fails to hold the Even spread as defined: not exactly one, or one that is not an
// exported function of the topology key alone returning the even topology spread
// constraint over it.
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

// whyNotTheEvenSpread is why a declaration named EvenSpreadOver is not the Even
// spread, or the empty string where it is.
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

// resilientPlacementNotNamingTheEvenSpread names every way the place's
// definition of Resilient placement fails to name the Even spread over Zones and
// over Nodes alone: no such definition, a subject it names no Even spread over,
// or a topology spread constraint it writes itself.
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

// whereResilientPlacementDepartsFromTheEvenSpread names every subject the
// settings of Resilient placement name no Even spread over, and every topology
// spread constraint they write themselves.
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

// namesTheEvenSpreadOver reports whether a setting is EvenSpreadOver(node.<subject>),
// the Even spread of the same place over that label.
func namesTheEvenSpreadOver(setting ast.Expr, subject string) bool {
	named, isACall := setting.(*ast.CallExpr)
	if !isACall || len(named.Args) != onlyOnce {
		return false
	}
	function, isAName := named.Fun.(*ast.Ident)

	return isAName && function.Name == theEvenSpread && namesThe(named.Args[0], theNodeName, subject)
}

// Every value of a setting a Scenario writes stands beside its Kubernetes name: a
// literal of a type the pod or node package offers writes a key before each
// value, so no value is read by its position (docs/prd.md REQ-SIM-016).
func TestEveryScenarioSource_ShouldWriteEachValueOfASettingBesideItsKubernetesName(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := settingsWrittenByPosition(t, filesOfThePlace(t, place))

	if len(offenders) > 0 {
		t.Errorf("every literal of a pod or node type a Scenario writes keys each value by its field's name, but:\n%s",
			strings.Join(offenders, "\n"))
	}
}

// The rule's own decisions are read here, since the shipped sources write only the
// keyed form.
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

// settingsWrittenByPosition names every literal of a type the pod or node package
// offers that a file of the place writes with a value standing without its key.
// The files are parsed again here, since a literal is no name a declaration uses.
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

// isATypeOfAPodOrANode reports whether a literal's type is one the pod or the
// node package offers.
func isATypeOfAPodOrANode(written ast.Expr) bool {
	selected, isSelected := written.(*ast.SelectorExpr)
	if !isSelected {
		return false
	}
	qualifier, isAName := selected.X.(*ast.Ident)

	return isAName && (qualifier.Name == thePodName || qualifier.Name == theNodeName)
}

// keysEveryValue reports whether every value of a literal stands beside its key.
func keysEveryValue(literal *ast.CompositeLit) bool {
	return !slices.ContainsFunc(literal.Elts, func(element ast.Expr) bool {
		_, isKeyed := element.(*ast.KeyValueExpr)

		return !isKeyed
	})
}
