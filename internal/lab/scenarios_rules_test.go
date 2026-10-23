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

// In theDeploymentNames every declaration is an exported constant of the
// Deployment-name type, so the file holds names and no machinery.
const (
	theDeploymentNames    = "deployments.go"
	theDeploymentNameType = "lab.DeploymentName"
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
	theStandardPod                = "StandardPod"
	theGPUPod                     = "GPUPod"
	theRequestSetting             = "Request"
	theStandardCPU                = "4"
	theStandardMemory             = "16"
	theGPUCPU                     = "24"
	theGPUMemory                  = "96"
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
	theNamedSettings  = []string{theNamedSetting, theStandardPod, theGPUPod, theEvenSpread}
	theNamedRequests  = []string{theStandardPod, theGPUPod}
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

const Checkout lab.DeploymentName = "checkout"

func ADeployment_ShouldBeDeclared(s *lab.Scenario) {
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
	theStandardNodeDefinition = `func StandardNode() node.Capacity {
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
	theStandardPodDefinition = `func StandardPod() pod.Customization {
	return pod.Settings(pod.Request{CPU: 4, Memory: 16 * pod.Gi})
}`
	theGPUPodDefinition = `func GPUPod() pod.Customization {
	return pod.Settings(pod.Request{CPU: 24, Memory: 96 * pod.Gi})
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

// A Deployment line takes the Deployment name, the Pod count, the Request its Pods
// ask for and one predefined Pod specification, customized at most once. The
// Request is a Named setting of one Request or a pod.Request of the line's own,
// and the Customize states none, so the line names one Request alone. A setting
// of the Scenario's own would be a further argument or call, and neither is
// admitted.
//
// The shape is read off the syntax tree rather than the text, because a
// Customization wraps over several lines once gofmt has had it.
const (
	theDeploymentStep = "Deployment"
	theCustomize      = "Customize"

	theDeploymentLineArguments = 4
	theRequestArgument         = 2
	theSpecificationArgument   = 3
)

// A Named setting stands for one Pod's Request and a Named Node capacity for
// one Node's room, so no source of the Scenarios defines or names these plurals.
var thePluralNames = []string{"StandardPods", "GPUPods", "StandardNodes", "GPUNodes"}

// A second call of theDeploymentsOwnCalls, or any other call written there, stays
// refused.
var theDeploymentsOwnCalls = []string{"SurvivesLosing"}

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
		t.Errorf("%s holds the list and exported Scenario functions named %s whose every line is a Given, When, Wait or Then Step, and the files beside it declare the Deployment names, the Node label keys, the taints, the Named Node capacities and the predefined Pod specifications alone, but:\n%s",
			theList,
			theScenarioNameShape,
			strings.Join(offenders, "\n"))
	}
}

func TestEveryScenarioDeploymentLine_ShouldNameStandardPodOrGPUPodOrItsOwnRequestAndOnlyOne(t *testing.T) {
	place := theScenariosPlace(t)

	files := filesOfThePlace(t, place)

	offenders := slices.Concat(deploymentLinesNotOfTheAdmittedShape(files), namesLeftInThePlural(files))

	if len(offenders) > 0 {
		t.Errorf("every Deployment line of a Scenario names %s(), %s() or a pod.%s of its own, then one predefined Pod specification of %s stating no Request of its own in Customize, and no source of the Scenarios names %s, but:\n%s",
			theStandardPod, theGPUPod, theRequestSetting, thePodSpecifications,
			strings.Join(thePluralNames, ", "), strings.Join(offenders, "\n"))
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

func TestTheDeploymentNamesRule_ShouldAdmitDeploymentNamesAndReportAnythingElse(t *testing.T) {
	written := map[string]struct {
		declarations string
		admitted     bool
	}{
		"a Deployment name":      {declarations: `const Checkout lab.DeploymentName = "checkout"`, admitted: true},
		"an untyped constant":    {declarations: `const Checkout = "checkout"`},
		"a type of its own":      {declarations: `type Count int`},
		"an alias of a count":    {declarations: `type Count = int`},
		"an alias of a lab name": {declarations: `type Name = lab.DeploymentName`},
		"a function":             {declarations: `func Checkout() lab.DeploymentName { return "checkout" }`},
	}

	for shape, taken := range written {
		t.Run(shape, func(t *testing.T) {
			place := aPlaceHolding(t, map[string]string{theDeploymentNames: "package scenarios\n\n" +
				"import \"github.com/woditschka/k8s-resilience-lab/internal/lab\"\n\n" + taken.declarations + "\n"})

			offenders := declarationsThatDoNotBelong(place)

			expect.Expect(t, len(offenders) == 0).To(expect.Equal(taken.admitted))
		})
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
			line: `s.Given().Zones(3, node.Pool("general", node.PerZone(3), ` +
				`node.Capacity{CPU: 4, Memory: 16 * node.Gi, Pods: 10}))`,
			admitted: true,
		},
		"a NodePool named otherwise": {
			line:     `s.Given().Zones(3, node.Pool("gpu", node.PerZone(3), StandardNode()))`,
			admitted: false,
		},
		"two NodePools, each named after what it holds": {
			line: `s.Given().Zones(3, node.Pool("on-demand", node.PerZone(1), StandardNode()), ` +
				`node.Pool("spot", node.PerZone(1), StandardNode()))`,
			admitted: true,
		},
		"a labelled NodePool named general": {
			line: `s.Given().Zones(3, node.Pool("general", node.PerZone(3), StandardNode()).` +
				`Labels(node.Labels{"capacity-type": "spot"}))`,
			admitted: true,
		},
		"no NodePool": {
			line:     `s.Given().Zones(3)`,
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
			line: `s.Given().Zones(3, node.Pool("general", node.PerZone(3), ` +
				`node.Capacity{CPU: 4, Memory: 16 * node.Gi, Pods: 10}))`,
			admitted: true,
		},
		"a NodePool stating its capacity as a value of another type": {
			line: `s.Given().Zones(3, node.Pool("general", node.PerZone(3), ` +
				`pod.Request{CPU: 4, Memory: 16 * pod.Gi}))`,
			admitted: false,
		},
		"a NodePool naming a capacity of the lab's own": {
			line:     `s.Given().Zones(3, node.Pool("general", node.PerZone(3), node.StandardNode()))`,
			admitted: false,
		},
		"a NodePool naming a capacity the place does not define": {
			line: `s.Given().Zones(3, node.Pool("general", node.PerZone(3), ` +
				`aCapacityOfItsOwn()))`,
			admitted: false,
		},
		"a NodePool defining its capacity on its own line": {
			line: `s.Given().Zones(3, node.Pool("general", node.PerZone(3), ` +
				`node.Named("Standard", node.Capacity{CPU: 16, Memory: 64 * node.Gi, Pods: 10})))`,
			admitted: false,
		},
		"a NodePool writing no capacity": {
			line:     `s.Given().Zones(3, node.Pool("general", node.PerZone(3)))`,
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
		theStandardDefinition       = theStandardNodeDefinition
		aSecondDefinitionOfStandard = `func LargerStandardNode() node.Capacity {
	return node.Named("Standard", node.Capacity{CPU: 32, Memory: 128 * node.Gi, Pods: 20})
}`
		aDefinitionTakingItsValues = `func SizedNode(pods int) node.Capacity {
	return node.Named("Sized", node.Capacity{CPU: 16, Memory: 64 * node.Gi, Pods: pods})
}`
		aDefinitionReturningAStatedCapacity = `func StatedNode() node.Capacity {
	return node.Capacity{CPU: 16, Memory: 64 * node.Gi, Pods: 10}
}`
		aDefinitionUnderAComputedName = `func ComputedNode() node.Capacity {
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
			line: `s.Given().Zones(3, node.Pool("general", node.PerZone(3), ` +
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

func StandardNode() Capacity {
	return Named("Standard", Capacity{CPU: 16, Memory: 64 * Gi, Pods: 10})
}
`,
		},
		"the node package reaching the domain's constructor elsewhere": {
			source: `package node

func StandardNode() scenario.NamedNodeCapacity {
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

func standardNode() node.Capacity {
	return node.Named("Standard", node.Capacity{CPU: 16, Memory: 64 * node.Gi, Pods: 10})
}
`,
		},
		"the command defining a capacity of its own": {
			source: `package main

func standardNode() node.Capacity {
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

// A Request stands on the Deployment line, so a reader of the Scenario sees what
// its Pods ask for without opening a definition.
func TestEveryPredefinedPodSpecification_ShouldHoldNoRequest(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := podSpecificationsHoldingARequest(filesOfThePlace(t, place))

	if len(offenders) > 0 {
		t.Errorf("no definition of %s writes a pod.%s or names %s() or %s(), but:\n%s",
			thePodSpecifications, theRequestSetting, theStandardPod, theGPUPod, strings.Join(offenders, "\n"))
	}
}

func TestThePredefinedSpecificationRequestRule_ShouldAdmitADefinitionWithoutARequestAndReportOneHoldingAny(t *testing.T) {
	const (
		aDefinitionNamingStandardPod = `func SteadyPlacement() pod.Specification {
	return pod.Predefined("Steady placement", pod.StartupMillisBetween(500, 1000),
		` + theNamedSettingWritten + `,
		StandardPod(),
	)
}`
		aDefinitionNamingGPUPod = `func HungryPlacement() pod.Specification {
	return pod.Predefined("Hungry placement", pod.StartupMillisBetween(500, 1000),
		` + theNamedSettingWritten + `,
		GPUPod(),
	)
}`
		aDefinitionStatingARequest = `func TightPlacement() pod.Specification {
	return pod.Predefined("Tight placement", pod.StartupMillisBetween(500, 1000),
		` + theNamedSettingWritten + `,
		pod.Request{CPU: 3, Memory: 12 * pod.Gi},
	)
}`
		aDefinitionGroupingARequest = `func GroupedPlacement() pod.Specification {
	return pod.Predefined("Grouped placement", pod.StartupMillisBetween(500, 1000),
		` + theNamedSettingWritten + `,
		pod.Settings(pod.Request{CPU: 3, Memory: 12 * pod.Gi}),
	)
}`
	)

	written := map[string]struct {
		source   string
		admitted bool
	}{
		"a definition holding no Request": {
			source:   aPodSpecificationsSourceOf(theResilientPlacementDefinition),
			admitted: true,
		},
		"a definition naming Standard Pods": {
			source: aPodSpecificationsSourceOf(aDefinitionNamingStandardPod),
		},
		"a definition naming GPU Pods": {
			source: aPodSpecificationsSourceOf(aDefinitionNamingGPUPod),
		},
		"a definition stating a Request": {
			source: aPodSpecificationsSourceOf(aDefinitionStatingARequest),
		},
		"a definition grouping a Request": {
			source: aPodSpecificationsSourceOf(aDefinitionGroupingARequest),
		},
	}

	for shape, taken := range written {
		t.Run(shape, func(t *testing.T) {
			place := aPlaceHolding(t, map[string]string{thePodSpecifications: taken.source})

			offenders := podSpecificationsHoldingARequest(place)

			expect.Expect(t, len(offenders) == 0).To(expect.Equal(taken.admitted))
		})
	}
}

func TestStandardPod_ShouldHoldOneRequestOfFourCpuAndSixteenGiAndNothingElse(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := namedRequestsNotHolding(filesOfThePlace(t, place), theStandardPod, theStandardCPU, theStandardMemory)

	if len(offenders) > 0 {
		t.Errorf("%s holds one exported function %s taking nothing, whose body is one return of pod.%s of one pod.%s{CPU: %s, Memory: %s * pod.Gi}, but:\n%s",
			thePodSpecifications, theStandardPod, theGroupingMeans, theRequestSetting,
			theStandardCPU, theStandardMemory, strings.Join(offenders, "\n"))
	}
}

func TestGPUPod_ShouldHoldOneRequestOfTwentyFourCpuAndNinetySixGiAndNothingElse(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := namedRequestsNotHolding(filesOfThePlace(t, place), theGPUPod, theGPUCPU, theGPUMemory)

	if len(offenders) > 0 {
		t.Errorf("%s holds one exported function %s taking nothing, whose body is one return of pod.%s of one pod.%s{CPU: %s, Memory: %s * pod.Gi}, but:\n%s",
			thePodSpecifications, theGPUPod, theGroupingMeans, theRequestSetting,
			theGPUCPU, theGPUMemory, strings.Join(offenders, "\n"))
	}
}

// The shape of the group is the Named setting rule's to read; this reads the
// one Request's values.
func namedRequestsNotHolding(files []modulerules.GoFile, name, cpu, memory string) []string {
	var offenders []string
	held := 0
	for _, file := range files {
		if filepath.Base(file.Path) != thePodSpecifications {
			continue
		}
		for _, declared := range file.Declarations {
			if declared.Kind != modulerules.FuncDeclaration || declared.Name != name {
				continue
			}
			held++
			group, why := theGroupOf(declared)
			if group == nil {
				offenders = append(offenders, fmt.Sprintf("  %s, in %s: %s", file.Path, declared.Name, why))

				continue
			}
			if len(group.Args) != onlyOnce || !isTheRequestOf(group.Args[0], cpu, memory) {
				offenders = append(offenders, fmt.Sprintf("  %s, in %s: does not hold exactly pod.%s{CPU: %s, Memory: %s * pod.Gi}",
					file.Path, declared.Name, theRequestSetting, cpu, memory))
			}
		}
	}
	if held != onlyOnce {
		offenders = append(offenders, fmt.Sprintf("  %s defines %s %d times", thePodSpecifications, name, held))
	}

	return offenders
}

func podSpecificationsHoldingARequest(files []modulerules.GoFile) []string {
	var offenders []string
	for _, file := range files {
		if filepath.Base(file.Path) != thePodSpecifications {
			continue
		}
		for _, declared := range file.Declarations {
			name, call, isADefinition := theNameDefinedBy(declared, thePodName, thePodSpecificationDefinition)
			if isADefinition && slices.ContainsFunc(call.Args, holdsARequest) {
				offenders = append(offenders, fmt.Sprintf("  %s, in %s: %q holds a Request",
					file.Path, declared.Name, name))
			}
		}
	}

	return offenders
}

// A Request is held when it is written anywhere inside the setting, a group
// included.
func holdsARequest(setting ast.Expr) bool {
	holds := false
	ast.Inspect(setting, func(visited ast.Node) bool {
		if written, isAnExpression := visited.(ast.Expr); isAnExpression &&
			(isALiteralOf(written, thePodName, theRequestSetting) || namesANamedRequest(written)) {
			holds = true
		}

		return !holds
	})

	return holds
}

func namesANamedRequest(setting ast.Expr) bool {
	named, isACall := setting.(*ast.CallExpr)
	if !isACall {
		return false
	}
	function, isAName := named.Fun.(*ast.Ident)

	return isAName && slices.Contains(theNamedRequests, function.Name) && len(named.Args) == 0
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
			line:        theDeploymentLine,
			admitted:    true,
		},
		"two definitions under one name": {
			definitions: []string{theResilientPlacementDefinition, aSecondDefinitionOfResilientPlacement},
			line:        theDeploymentLine,
		},
		"a definition taking a value": {
			definitions: []string{theResilientPlacementDefinition, aDefinitionTakingItsValues},
			line:        theDeploymentLine,
		},
		"a definition under a name no literal writes": {
			definitions: []string{theResilientPlacementDefinition, aDefinitionUnderAComputedName},
			line:        theDeploymentLine,
		},
		"a variable in place of a function": {
			definitions: []string{theResilientPlacementDefinition, aVariableOfASpecification},
			line:        theDeploymentLine,
		},
		"a definition not naming the Named setting": {
			definitions: []string{theResilientPlacementDefinition, aDefinitionNamingNoNamedSetting},
			line:        theDeploymentLine,
		},
		"a definition writing a toleration of the not-ready taint besides it": {
			definitions: []string{theResilientPlacementDefinition, aDefinitionWritingTheNotReadyToleration},
			line:        theDeploymentLine,
		},
		"a definition writing a toleration of the unreachable taint besides it": {
			definitions: []string{theResilientPlacementDefinition, aDefinitionWritingTheUnreachableToleration},
			line:        theDeploymentLine,
		},
		"a definition naming its Startup time by no Startup form": {
			definitions: []string{theResilientPlacementDefinition, aDefinitionNamingAZeroStartupTime},
			line:        theDeploymentLine,
		},
		"a Scenario defining one on its own line": {
			definitions: []string{theResilientPlacementDefinition},
			line: `s.Given().Deployment(Checkout, 9, StandardPod(), pod.Predefined("Inline placement", ` +
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
func isANamedSetting(declared modulerules.Declaration) bool {
	return declared.Kind == modulerules.FuncDeclaration && slices.Contains(theNamedSettings, declared.Name)
}

// A Deployment line writing a definition inline is an offender too.
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

func TestTheNamedSettingRule_ShouldAdmitTheGroupOfTheTwoTolerationsAndTheGroupOfOneRequestAndReportAnythingElse(t *testing.T) {
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
		aStandardPodLeavingOutTheMemory = `func StandardPod() pod.Customization {
	return pod.Settings(pod.Request{CPU: 4})
}`
		aStandardPodWithAToleration = `func StandardPod() pod.Customization {
	return pod.Settings(
		pod.Request{CPU: 4, Memory: 16 * pod.Gi},
		pod.Toleration{Key: node.NotReady, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: node.UnresponsiveWait},
	)
}`
		aStandardPodWrittenByPosition = `func StandardPod() pod.Customization {
	return pod.Settings(pod.Request{4, 16 * pod.Gi})
}`
		aStandardPodNotGrouping = `func StandardPod() pod.Customization {
	return pod.StartupSeconds(1)
}`
		aStandardPodTakingAValue = `func StandardPod(cpu int) pod.Customization {
	return pod.Settings(pod.Request{CPU: cpu, Memory: 16 * pod.Gi})
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
		"the group of one Request besides it": {
			source:   aPodSpecificationsSourceOf(theNamedSettingDefinition, theStandardPodDefinition, theResilientPlacementDefinition),
			admitted: true,
		},
		"both groups of one Request besides it": {
			source: aPodSpecificationsSourceOf(
				theNamedSettingDefinition, theStandardPodDefinition, theGPUPodDefinition, theResilientPlacementDefinition),
			admitted: true,
		},
		"a group of one Request leaving out the memory": {
			source: aPodSpecificationsSourceOf(theNamedSettingDefinition, aStandardPodLeavingOutTheMemory, theResilientPlacementDefinition),
		},
		"a group of the Request with a toleration": {
			source: aPodSpecificationsSourceOf(theNamedSettingDefinition, aStandardPodWithAToleration, theResilientPlacementDefinition),
		},
		"a group of a Request written by position": {
			source: aPodSpecificationsSourceOf(theNamedSettingDefinition, aStandardPodWrittenByPosition, theResilientPlacementDefinition),
		},
		"a Standard Pods setting that groups nothing": {
			source: aPodSpecificationsSourceOf(theNamedSettingDefinition, aStandardPodNotGrouping, theResilientPlacementDefinition),
		},
		"a Standard Pods setting taking a value": {
			source: aPodSpecificationsSourceOf(theNamedSettingDefinition, aStandardPodTakingAValue, theResilientPlacementDefinition),
		},
		"Standard Pods defined twice": {
			source: aPodSpecificationsSourceOf(
				theNamedSettingDefinition, theStandardPodDefinition, theStandardPodDefinition, theResilientPlacementDefinition),
		},
		"GPU Pods defined twice": {
			source: aPodSpecificationsSourceOf(
				theNamedSettingDefinition, theGPUPodDefinition, theGPUPodDefinition, theResilientPlacementDefinition),
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
	held := make(map[string]int)
	for _, file := range files {
		for _, declared := range file.Declarations {
			if why, isGrouped := theGroupedSettings[declared.Name]; isGrouped &&
				filepath.Base(file.Path) == thePodSpecifications && declared.Kind == modulerules.FuncDeclaration {
				held[declared.Name]++
				if reason := why(declared); reason != "" {
					offenders = append(offenders, fmt.Sprintf("  %s, in %s: %s", file.Path, declared.Name, reason))
				}

				continue
			}
			if slices.Contains(declared.Names, theNameOf(thePodName, theGroupingMeans)) {
				offenders = append(offenders, fmt.Sprintf("  %s, in %s: calls pod.%s besides %s, %s and %s",
					file.Path, declared.Name, theGroupingMeans, theNamedSetting, theStandardPod, theGPUPod))
			}
		}
	}
	if held[theNamedSetting] != onlyOnce {
		offenders = append(offenders, fmt.Sprintf("  %s defines %s %d times", thePodSpecifications, theNamedSetting, held[theNamedSetting]))
	}
	for _, named := range theNamedRequests {
		if held[named] > onlyOnce {
			offenders = append(offenders, fmt.Sprintf("  %s defines %s %d times", thePodSpecifications, named, held[named]))
		}
	}

	return offenders
}

// theGroupedSettings are the Named settings that return one pod.Settings call,
// each with the reading of what that group must hold.
var theGroupedSettings = map[string]func(modulerules.Declaration) string{
	theNamedSetting: whyNotTheNamedSetting,
	theStandardPod:  whyNotAGroupOfOneRequest,
	theGPUPod:       whyNotAGroupOfOneRequest,
}

func theGroupOf(declared modulerules.Declaration) (*ast.CallExpr, string) {
	if !declared.Exported || len(declared.Parameters) != 0 || len(declared.Statements) != onlyOnce {
		return nil, "is not an exported function taking nothing whose body is one statement"
	}
	returned, isAReturn := strings.CutPrefix(declared.Statements[0].Written, theReturn)
	if !isAReturn {
		return nil, "does not return its group"
	}
	parsed, err := parser.ParseExpr(returned)
	if err != nil {
		return nil, "does not return an expression"
	}
	group, isACall := parsed.(*ast.CallExpr)
	if !isACall || !callsThe(group, thePodName, theGroupingMeans) {
		return nil, fmt.Sprintf("does not return one pod.%s call", theGroupingMeans)
	}

	return group, ""
}

func whyNotTheNamedSetting(declared modulerules.Declaration) string {
	group, why := theGroupOf(declared)
	if group == nil {
		return why
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

func whyNotAGroupOfOneRequest(declared modulerules.Declaration) string {
	group, why := theGroupOf(declared)
	if group == nil {
		return why
	}
	if len(group.Args) != onlyOnce || !isAStatedRequest(group.Args[0]) {
		return fmt.Sprintf("does not group exactly one pod.%s keyed by both its CPU and its Memory", theRequestSetting)
	}

	return ""
}

func isAStatedRequest(setting ast.Expr) bool {
	fields, isARequest := keyedFieldsOf(setting, thePodName, theRequestSetting)
	_, statesTheCPU := fields["CPU"]
	_, statesTheMemory := fields["Memory"]

	return isARequest && len(fields) == 2 && statesTheCPU && statesTheMemory
}

func isTheRequestOf(setting ast.Expr, cpu, memory string) bool {
	fields, isARequest := keyedFieldsOf(setting, thePodName, theRequestSetting)
	if !isARequest || len(fields) != 2 {
		return false
	}
	gibibytes, isAProduct := fields["Memory"].(*ast.BinaryExpr)

	return isTheInteger(fields["CPU"], cpu) &&
		isAProduct && gibibytes.Op == token.MUL && namesThe(gibibytes.Y, thePodName, "Gi") && isTheInteger(gibibytes.X, memory)
}

func isTheInteger(written ast.Expr, value string) bool {
	literal, isALiteral := written.(*ast.BasicLit)

	return isALiteral && literal.Kind == token.INT && literal.Value == value
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
func TestTheDeploymentLineRule_ShouldAdmitOneRequestAndOneCustomizedSpecificationAndReportAnythingElse(t *testing.T) {
	written := map[string]struct {
		line     string
		admitted bool
	}{
		"one predefined specification": {
			line:     `s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement())`,
			admitted: true,
		},
		"GPU Pods as the Request": {
			line:     `s.Given().Deployment(Checkout, 9, GPUPod(), ResilientPlacement())`,
			admitted: true,
		},
		"a Request of the line's own": {
			line:     `s.Given().Deployment(Checkout, 9, pod.Request{CPU: 3, Memory: 12 * pod.Gi}, ResilientPlacement())`,
			admitted: true,
		},
		"no Request at all": {
			line:     `s.Given().Deployment(Checkout, 9, ResilientPlacement())`,
			admitted: false,
		},
		"two Requests on the line": {
			line: `s.Given().Deployment(Checkout, 9, StandardPod(), pod.Request{CPU: 3, Memory: 12 * pod.Gi},
		ResilientPlacement())`,
			admitted: false,
		},
		"a Request leaving out its memory": {
			line:     `s.Given().Deployment(Checkout, 9, pod.Request{CPU: 3}, ResilientPlacement())`,
			admitted: false,
		},
		"a Request written by position": {
			line:     `s.Given().Deployment(Checkout, 9, pod.Request{3, 12 * pod.Gi}, ResilientPlacement())`,
			admitted: false,
		},
		"a group of the Scenario's own as the Request": {
			line: `s.Given().Deployment(Checkout, 9, pod.Settings(pod.Request{CPU: 3, Memory: 12 * pod.Gi}),
		ResilientPlacement())`,
			admitted: false,
		},
		"a Request stated in Customize": {
			line: `s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement().Customize(
		pod.Request{CPU: 3, Memory: 12 * pod.Gi},
	))`,
			admitted: false,
		},
		"a Named Request named in Customize": {
			line:     `s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement().Customize(GPUPod()))`,
			admitted: false,
		},
		"a specification it customizes": {
			line: `s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement().Customize(
		pod.TopologySpreadConstraint{TopologyKey: node.Zone, MaxSkew: 2, WhenUnsatisfiable: pod.DoNotSchedule, NodeTaintsPolicy: pod.Honor},
	))`,
			admitted: true,
		},
		"a Customize naming no setting": {
			line:     `s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement().Customize())`,
			admitted: true,
		},
		"no specification at all": {
			line:     `s.Given().Deployment(Checkout, 9)`,
			admitted: false,
		},
		"a setting written beside the specification": {
			line: `s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement(),
		pod.TopologySpreadConstraint{TopologyKey: node.Zone, MaxSkew: 2, WhenUnsatisfiable: pod.DoNotSchedule, NodeTaintsPolicy: pod.Honor})`,
			admitted: false,
		},
		"a second Customize written on the first": {
			line:     `s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement().Customize().Customize())`,
			admitted: false,
		},
		"a Failure tolerance written after the Deployment Step": {
			line:     `s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement()).SurvivesLosing(lab.Zones(1))`,
			admitted: true,
		},
		"a second Failure tolerance written on the first": {
			line: `s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement()).` +
				`SurvivesLosing(lab.Zones(1)).SurvivesLosing(lab.Nodes(1))`,
			admitted: false,
		},
		"a Wait chained onto the Deployment Step": {
			line:     `s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement()).WaitFor().AllPodsReady()`,
			admitted: true,
		},
		"a Wait chained onto the Failure tolerance": {
			line: `s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement()).` +
				`SurvivesLosing(lab.Zones(1)).WaitFor().AllPodsReady()`,
			admitted: true,
		},
		"a Wait written on the Deployment Step with no WaitFor opening it": {
			line:     `s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement()).AllPodsReady()`,
			admitted: false,
		},
		"a Customize written after the Deployment Step": {
			line:     `s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement()).Customize()`,
			admitted: false,
		},
		"a call written after the Deployment Step": {
			line:     `s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement()).Pool("general", node.PerZone(1), StandardNode())`,
			admitted: false,
		},
		"a specification of the Scenario's own": {
			line:     `s.Given().Deployment(Checkout, 9, StandardPod(), aSpecificationOfItsOwn())`,
			admitted: false,
		},
		"a specification named on the Step surface": {
			line:     `s.Given().Deployment(Checkout, 9, StandardPod(), lab.ResilientPlacement())`,
			admitted: false,
		},
		"a specification defined on the line itself": {
			line: `s.Given().Deployment(Checkout, 9, StandardPod(), ` +
				`pod.Predefined("Inline placement", pod.StartupMillisBetween(500, 1000)))`,
			admitted: false,
		},
	}

	for shape, taken := range written {
		t.Run(shape, func(t *testing.T) {
			offenders := deploymentLinesNotOfTheAdmittedShape(aScenarioWhoseDeploymentLineIs(t, taken.line))

			expect.Expect(t, len(offenders) == 0).To(expect.Equal(taken.admitted))
		})
	}
}

func TestThePluralNameRule_ShouldAdmitTheSingularNamesAndReportAPluralOneDefinedOrNamed(t *testing.T) {
	const (
		theSingularLines = `s.Given().Zones(3, node.Pool("general", node.PerZone(3), StandardNode()))
	s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement())`
		aPluralNodeCapacityDefinition = `func GPUNodes() node.Capacity {
	return node.Named("GPU", node.Capacity{CPU: 32, Memory: 128 * node.Gi, Pods: 10})
}`
		aPluralNamedSettingDefinition = `func StandardPods() pod.Customization {
	return pod.Settings(pod.Request{CPU: 4, Memory: 16 * pod.Gi})
}`
	)

	written := map[string]struct {
		sources  map[string]string
		admitted bool
	}{
		"the singular names": {
			sources:  map[string]string{theScenarioSource: aScenarioSourceWhoseStepsAre(theSingularLines)},
			admitted: true,
		},
		"a Named Node capacity named in the plural": {
			sources: map[string]string{theScenarioSource: aScenarioSourceWhoseStepsAre(
				`s.Given().Zones(3, node.Pool("general", node.PerZone(3), StandardNodes()))`)},
		},
		"a Named setting named in the plural": {
			sources: map[string]string{theScenarioSource: aScenarioSourceWhoseStepsAre(
				`s.Given().Deployment(Checkout, 9, GPUPods(), ResilientPlacement())`)},
		},
		"a Named Node capacity defined in the plural": {
			sources: map[string]string{theNamedNodeCapacities: aNamedNodeCapacitiesSourceOf(aPluralNodeCapacityDefinition)},
		},
		"a Named setting defined in the plural": {
			sources: map[string]string{thePodSpecifications: aPodSpecificationsSourceOf(aPluralNamedSettingDefinition)},
		},
	}

	for shape, taken := range written {
		t.Run(shape, func(t *testing.T) {
			offenders := namesLeftInThePlural(aPlaceHolding(t, taken.sources))

			expect.Expect(t, len(offenders) == 0).To(expect.Equal(taken.admitted))
		})
	}
}

const (
	theDeploymentLine = `s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement())`
	theFullAccessLine = `s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement().Customize(
		pod.FullAccess(func(spec *` + thePodSettingsName + `.PodSpec) {
			spec.PriorityClassName = "critical"
		}),
	))`
)

func aScenarioWhoseDeploymentLineIs(t *testing.T, line string) []modulerules.GoFile {
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
		thePodSpecifications: aPodSpecificationsSourceOf(theNamedSettingDefinition, theStandardPodDefinition,
			theGPUPodDefinition, theEvenSpreadDefinition, theResilientPlacementDefinition),
	})
}

func aScenarioBesideTheNamedNodeCapacities(t *testing.T, steps ...string) []modulerules.GoFile {
	t.Helper()

	return aPlaceHolding(t, map[string]string{
		theScenarioSource:      aScenarioSourceWhoseStepsAre(steps...),
		theNamedNodeCapacities: aNamedNodeCapacitiesSourceOf(theStandardNodeDefinition),
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
				theDeploymentLine, theWaitForEveryPodLineStatingATime, theThenLine,
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
			steps:    []string{theTopologyWaitingLine, theDeploymentWaitingLine, theThenLine},
			admitted: true,
		},
		"a Wait of its own before the first Then": {
			steps:    []string{theTopologyLine, theDeploymentLine, theWaitForEveryPodLine, theThenLine},
			admitted: true,
		},
		"no Wait before the first Then": {
			steps:    []string{theTopologyLine, theDeploymentLine, theThenLine},
			admitted: false,
		},
		"two Waits in a row": {
			steps: []string{
				theTopologyLine, theDeploymentLine,
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
			steps:    []string{theTopologyLine, theDeploymentLine, theFailureLine, theWaitForEveryPodLine, theThenLine},
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
	theTopologyLine          = `s.Given().Zones(3, node.Pool("general", node.PerZone(3), node.Capacity{CPU: 4, Memory: 16 * node.Gi, Pods: 10}))`
	theTopologyWaitingLine   = theTopologyLine + `.WaitFor().AllNodesReady()`
	theDeploymentWaitingLine = theDeploymentLine + `.WaitFor().AllPodsReady()`
	theWaitForEveryNodeLine  = `s.WaitFor().AllNodesReady()`
	theWaitForEveryPodLine   = `s.WaitFor().AllPodsReady()`

	theWaitForEveryPodLineStatingATime = `s.WaitFor().AllPodsReadyWithinSeconds(30)`
	theFailureLine                     = `s.When().NodesFail(1)`
	theThenLine                        = `s.Then().PodsPerZone(3)`
)

func aWorldDeclaredAndAwaited(followedBy ...string) []string {
	opened := []string{theTopologyLine, theWaitForEveryNodeLine, theDeploymentLine, theWaitForEveryPodLine}

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
			steps: []string{theTopologyWaitingLine, theDeploymentLine, theWaitForEveryPodLine, theThenLine},
		},
		"a Wait written on the Deployment it waits for": {
			steps: []string{theTopologyLine, theWaitForEveryNodeLine, theDeploymentWaitingLine, theThenLine},
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

// A name the lab offers carries its package, and a name the Scenarios define
// reads bare. One written unqualified, as a dot import would write it, is
// neither a subject's name nor one the Scenarios define.
func TestEveryScenarioStep_ShouldQualifyWhatTheLabOffersAndWriteBareWhatTheScenariosDefine(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := namesUsedThatTheLabDoesNotOffer(t, filesOfThePlace(t, place))

	if len(offenders) > 0 {
		t.Errorf("every name a Scenario uses is one lab, pod or node exports under its own qualifier, or one the Scenarios define, but:\n%s",
			strings.Join(offenders, "\n"))
	}
}

// Read on scratch sources, since the lab's own place names only what it is
// offered.
func TestTheQualifierRule_ShouldReportANameNeitherItsSubjectNorThePlaceDeclares(t *testing.T) {
	written := map[string]struct {
		line     string
		admitted bool
	}{
		"the names each subject's package exports": {
			line: `s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement().Customize(` +
				`pod.Request{CPU: 1, Memory: 1 * pod.Gi}))`,
			admitted: true,
		},
		"a specification the Step surface no longer exports": {
			line: `s.Given().Deployment(Checkout, 9, StandardPod(), lab.MaximumAvailability())`,
		},
		"a name of the Scenario's own": {
			line: `s.Given().Deployment(Checkout, 9, StandardPod(), aSpecificationOfItsOwn())`,
		},
		"a Pod setting under the Scenario's package": {
			line: `s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement().Customize(` +
				`lab.Request{CPU: 1, Memory: 1 * pod.Gi}))`,
		},
		"a Node's taint under the pod package": {
			line: `s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement().Customize(` +
				`pod.Toleration{Key: pod.NotReady, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: 300}))`,
		},
		"a function marking the position of a value": {
			line: `s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement().Customize(` +
				`pod.Toleration{Key: pod.Key(node.NotReady), Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: 300}))`,
		},
		"a Pod setting written unqualified, as a dot import writes it": {
			line: `s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement().Customize(` +
				`Request{CPU: 1, Memory: 1 * Gi}))`,
		},
		"a NodePool the node package offers, qualified": {
			line:     `s.Given().Zones(3, node.Pool("general", node.PerZone(3), node.Capacity{CPU: 4, Memory: 16 * node.Gi, Pods: 10}))`,
			admitted: true,
		},
		"a NodePool the node package offers, written bare": {
			line: `s.Given().Zones(3, Pool("general", PerZone(3), Capacity{CPU: 4, Memory: 16 * Gi, Pods: 10}))`,
		},
		"a name the Scenarios define, written bare": {
			line:     `s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement())`,
			admitted: true,
		},
		"a name the Scenarios define, qualified by a package that does not define it": {
			line: `s.Given().Deployment(Checkout, 9, pod.StandardPod(), ResilientPlacement())`,
		},
		"a Deployment name the Scenarios define, qualified": {
			line: `s.Given().Deployment(lab.Checkout, 9, StandardPod(), ResilientPlacement())`,
		},
	}

	for named, taken := range written {
		t.Run(named, func(t *testing.T) {
			offenders := namesUsedThatTheLabDoesNotOffer(t, aScenarioWhoseDeploymentLineIs(t, taken.line))

			expect.Expect(t, len(offenders) == 0).To(expect.Equal(taken.admitted))
		})
	}
}

// Read on scratch sources, since no shipped Scenario uses full access. The
// placeholder the Scenario's own function literal declares is no name it
// writes.
func TestTheQualifierRule_ShouldAdmitTheKubernetesPodSettingsFullAccessNames(t *testing.T) {
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

// Every count reads bare under the Step naming its subject, so a typed count
// the Step surface declares fails here, and a Deployment's replicas are the int
// its second parameter names.
func TestADeploymentLine_ShouldStateItsReplicasAsAPlainCount(t *testing.T) {
	root, err := modulerules.Root(t.Context(), ownPackageDir)
	if err != nil {
		t.Fatalf("locate the root of the module: %v", err)
	}
	sources, err := modulerules.ScanGoFiles(filepath.Join(root, theLabsOwnPackageDir), modulerules.ProductionGoFilesOnly)
	if err != nil {
		t.Fatalf("read the sources of the Step surface: %v", err)
	}

	expect.Expect(t, typedCountsOfTheStepSurface(sources)).To(expect.HaveLen[string](0))
	expect.Expect(t, deploymentReplicasParametersOfTheStepSurface(sources)).To(expect.Equal([]modulerules.Parameter{{Name: "replicas", Type: "int"}}))
}

func deploymentReplicasParametersOfTheStepSurface(sources []modulerules.GoFile) []modulerules.Parameter {
	var replicas []modulerules.Parameter
	for _, source := range sources {
		if source.Package != theStepSurfaceName {
			continue
		}
		for _, declared := range source.Declarations {
			if declared.Kind == modulerules.FuncDeclaration && declared.Name == "Deployment" && len(declared.Parameters) > 1 {
				replicas = append(replicas, declared.Parameters[1])
			}
		}
	}

	return replicas
}

func typedCountsOfTheStepSurface(sources []modulerules.GoFile) []string {
	counts := []string{}
	for _, source := range sources {
		if source.Package != theStepSurfaceName {
			continue
		}
		for _, declared := range source.Declarations {
			if declared.Kind == modulerules.TypeDeclaration && declared.Exported &&
				slices.Equal(declared.Names, []modulerules.UsedName{{Name: "int"}}) {
				counts = append(counts, declared.Name)
			}
		}
	}

	return counts
}

// Inline Customizations are read on scratch sources, since no shipped Scenario
// writes one on a Deployment line.
func TestEveryInlineCustomization_ShouldBeWrittenQualified(t *testing.T) {
	written := map[string]struct {
		line     string
		admitted bool
	}{
		"a Startup time": {
			line:     `s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement().Customize(pod.StartupSeconds(3)))`,
			admitted: true,
		},
		"a toleration": {
			line: `s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement().Customize(` +
				`pod.Toleration{Key: node.NotReady, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: 300}))`,
			admitted: true,
		},
		"a full access": {line: theFullAccessLine, admitted: true},
		"a Startup time written bare": {
			line: `s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement().Customize(StartupSeconds(3)))`,
		},
		"a toleration written bare": {
			line: `s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement().Customize(` +
				`Toleration{Key: node.NotReady, Operator: Exists, Effect: NoExecute, TolerationSeconds: 300}))`,
		},
		"a full access written bare": {
			line: `s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement().Customize(FullAccess(nil)))`,
		},
		"a Customization the place does not define": {
			line: `s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement().Customize(aCustomizationOfItsOwn()))`,
		},
	}

	for named, taken := range written {
		t.Run(named, func(t *testing.T) {
			offenders := namesUsedThatTheLabDoesNotOffer(t, aScenarioAlsoImporting(t, thePodSettingsImport, taken.line))

			expect.Expect(t, len(offenders) == 0).To(expect.Equal(taken.admitted))
		})
	}
}

// The place defines its own names and re-exports none of the lab's, so a Scenario
// line always shows where a name comes from.
func TestTheScenarios_ShouldHoldNoAliasOfALabName(t *testing.T) {
	place := theScenariosPlace(t)

	offenders := aliasesOfALabName(filesOfThePlace(t, place))

	if len(offenders) > 0 {
		t.Errorf("the Scenarios hold no alias of a name lab, pod or node define, but:\n%s", strings.Join(offenders, "\n"))
	}
}

func TestTheAliasRule_ShouldReportEveryShapeOfAnAliasAndAdmitTheShippedDefinitions(t *testing.T) {
	const theImports = "import (\n\t\"github.com/woditschka/k8s-resilience-lab/internal/lab\"\n" +
		"\t\"github.com/woditschka/k8s-resilience-lab/internal/lab/node\"\n" +
		"\t\"github.com/woditschka/k8s-resilience-lab/internal/lab/pod\"\n)\n\n"
	written := map[string]struct {
		declarations string
		admitted     bool
	}{
		"a Named Node capacity defined over a literal": {
			declarations: `func StandardNode() node.NamedCapacity { return node.Named("standard", node.Capacity{CPU: 4}) }`,
			admitted:     true,
		},
		"an Even spread returning a keyed literal": {
			declarations: `func EvenSpreadOver(subject string) pod.TopologySpread { return pod.TopologySpread{Key: subject} }`,
			admitted:     true,
		},
		"a typed Deployment name": {
			declarations: `const Checkout lab.DeploymentName = "checkout"`,
			admitted:     true,
		},
		"a type alias of a lab name":  {declarations: `type Name = lab.DeploymentName`},
		"a type alias of a pod name":  {declarations: `type Spec = pod.Specification`},
		"a type alias of a node name": {declarations: `type Capacity = node.Capacity`},
		"a constant standing for a pod name": {
			declarations: `const NotReady = pod.NoExecute`,
		},
		"a variable standing for a node name": {
			declarations: `var Pool = node.Pool`,
		},
		"a function only forwarding to a node name": {
			declarations: `func Named(name string, capacity node.Capacity) node.NamedCapacity { return node.Named(name, capacity) }`,
		},
		"a function only forwarding its variadic parameter": {
			declarations: `func Settings(settings ...pod.Setting) pod.Customization { return pod.Settings(settings...) }`,
		},
		"a function only forwarding to a name without a parameter": {
			declarations: `func Hostname() string { return node.Hostname() }`,
		},
	}

	for shape, taken := range written {
		t.Run(shape, func(t *testing.T) {
			place := aPlaceHolding(t, map[string]string{thePodSpecifications: "package scenarios\n\n" + theImports + taken.declarations + "\n"})

			offenders := aliasesOfALabName(place)

			expect.Expect(t, len(offenders) == 0).To(expect.Equal(taken.admitted))
		})
	}
}

func aliasesOfALabName(files []modulerules.GoFile) []string {
	var offenders []string
	for _, source := range files {
		parsed, err := parser.ParseFile(token.NewFileSet(), source.Path, nil, parser.SkipObjectResolution)
		if err != nil {
			offenders = append(offenders, fmt.Sprintf("  %s cannot be read: %v", source.Path, err))
			continue
		}
		for _, declared := range parsed.Decls {
			offenders = append(offenders, aliasesIn(source.Path, declared)...)
		}
	}

	return offenders
}

func aliasesIn(path string, declared ast.Decl) []string {
	var offenders []string
	switch shape := declared.(type) {
	case *ast.GenDecl:
		for _, spec := range shape.Specs {
			switch held := spec.(type) {
			case *ast.TypeSpec:
				if held.Assign.IsValid() && isASubjectsName(held.Type) {
					offenders = append(offenders, fmt.Sprintf("  %s declares the type %s, an alias of a name lab, pod or node define", path, held.Name.Name))
				}
			case *ast.ValueSpec:
				for at, value := range held.Values {
					if isASubjectsName(value) && at < len(held.Names) {
						offenders = append(offenders, fmt.Sprintf("  %s declares %s, which only stands for a name lab, pod or node define",
							path, held.Names[at].Name))
					}
				}
			}
		}
	case *ast.FuncDecl:
		if forwardsToASubject(shape) {
			offenders = append(offenders, fmt.Sprintf("  %s declares the function %s, which only forwards to a name lab, pod or node define",
				path, shape.Name.Name))
		}
	}

	return offenders
}

func isASubjectsName(written ast.Expr) bool {
	selected, isSelected := written.(*ast.SelectorExpr)
	if !isSelected {
		return false
	}
	qualifier, isAName := selected.X.(*ast.Ident)

	return isAName && slices.Contains(theSubjects, qualifier.Name)
}

func forwardsToASubject(declared *ast.FuncDecl) bool {
	if declared.Body == nil || len(declared.Body.List) != 1 {
		return false
	}
	returned, isAReturn := declared.Body.List[0].(*ast.ReturnStmt)
	if !isAReturn || len(returned.Results) != 1 {
		return false
	}
	call, isACall := returned.Results[0].(*ast.CallExpr)
	if !isACall || !isASubjectsName(call.Fun) {
		return false
	}

	var own []string
	for _, field := range declared.Type.Params.List {
		for _, name := range field.Names {
			own = append(own, name.Name)
		}
	}
	passed := make([]string, 0, len(call.Args))
	for _, argument := range call.Args {
		name, isAName := argument.(*ast.Ident)
		if !isAName {
			return false
		}
		passed = append(passed, name.Name)
	}

	return slices.Equal(own, passed)
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
		case theDeploymentNames:
			offenders = append(offenders, declarationsThatAreNotDeploymentNames(file)...)
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

func declarationsThatAreNotDeploymentNames(source modulerules.GoFile) []string {
	var offenders []string
	for _, declared := range source.Declarations {
		isAName := declared.Kind == modulerules.ConstDeclaration && declared.Exported && declared.Type == theDeploymentNameType
		if isAName {
			continue
		}
		offenders = append(offenders, fmt.Sprintf("  %s declares the %s %s, which is not an exported %s constant",
			source.Path, declared.Kind, declared.Name, theDeploymentNameType))
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

// A Scenario declaring no Deployment line is reported too, since it would meet
// the shape rule by having nothing to check.
func deploymentLinesNotOfTheAdmittedShape(files []modulerules.GoFile) []string {
	defined := thePodSpecificationsDeclaredIn(files)

	var offenders []string
	for _, source := range files {
		for _, declared := range source.Declarations {
			if !isAScenario(declared) {
				continue
			}
			offenders = append(offenders, deploymentLinesOfScenarioNotOfTheAdmittedShape(source.Path, declared, defined)...)
		}
	}

	return offenders
}

func deploymentLinesOfScenarioNotOfTheAdmittedShape(
	path string, declared modulerules.Declaration, defined map[string]bool,
) []string {
	var offenders []string
	deploying := 0
	for _, line := range declared.Statements {
		if !namesTheDeploymentStep(line) {
			continue
		}
		deploying++
		if why := whyNotOneRequestAndOnePredefinedSpecification(line, defined); why != "" {
			offenders = append(offenders, fmt.Sprintf("  %s, in %s: %s %s", path, declared.Name, line.Written, why))
		}
	}
	if deploying == 0 {
		offenders = append(offenders, fmt.Sprintf("  %s, in %s: no Deployment is declared at all", path, declared.Name))
	}

	return offenders
}

func namesLeftInThePlural(files []modulerules.GoFile) []string {
	var offenders []string
	for _, file := range files {
		for _, declared := range file.Declarations {
			if slices.Contains(thePluralNames, declared.Name) {
				offenders = append(offenders, fmt.Sprintf("  %s: defines %s", file.Path, declared.Name))
			}
			for _, used := range declared.Names {
				if used.Qualifier == "" && slices.Contains(thePluralNames, used.Name) {
					offenders = append(offenders, fmt.Sprintf("  %s, in %s: names %s", file.Path, declared.Name, used.Name))
				}
			}
		}
	}

	return offenders
}

// A line calling something after the Deployment Step deploys one too, and is a
// line the shape rule reports.
func namesTheDeploymentStep(line modulerules.Statement) bool {
	for called := &line; called != nil; called = called.On {
		if called.Call && called.Name == theDeploymentStep {
			return true
		}
	}

	return false
}

func whyNotOneRequestAndOnePredefinedSpecification(line modulerules.Statement, defined map[string]bool) string {
	deploying, ends := theDeploymentStepTheLineEndsAt(line)
	if !ends || len(deploying.Arguments) != theDeploymentLineArguments {
		return "is not one Deployment Step of a Request and a predefined Pod specification"
	}
	if !isOneRequest(deploying.Arguments[theRequestArgument]) {
		return fmt.Sprintf("names no %s(), %s() or pod.%s keyed by both its CPU and its Memory",
			theStandardPod, theGPUPod, theRequestSetting)
	}

	declared := deploying.Arguments[theSpecificationArgument]
	if declared.Call && declared.Name == theCustomize && declared.On != nil {
		if slices.ContainsFunc(declared.Arguments, customizesARequest) {
			return "states a Request in Customize beside the line's own"
		}
		declared = *declared.On
	}
	if !isAPredefinedSpecification(declared, defined) {
		return "declares no predefined Pod specification"
	}

	return ""
}

func isOneRequest(asks modulerules.Statement) bool {
	if asks.Call && asks.On == nil && len(asks.Arguments) == 0 && slices.Contains(theNamedRequests, asks.Name) {
		return true
	}
	written, err := parser.ParseExpr(asks.Written)

	return err == nil && isAStatedRequest(written)
}

// A setting that does not parse on its own is no Request the rule can read, and
// the line's other rules report it.
func customizesARequest(setting modulerules.Statement) bool {
	written, err := parser.ParseExpr(setting.Written)

	return err == nil && holdsARequest(written)
}

// A Wait chained onto the line is read past first: it is a Step of its own and
// no part of what the Deployment declares.
func theDeploymentStepTheLineEndsAt(line modulerules.Statement) (modulerules.Statement, bool) {
	line = whatTheChainedWaitWasWrittenOn(line)

	if line.Name == theDeploymentStep {
		return line, true
	}
	if slices.Contains(theDeploymentsOwnCalls, line.Name) && line.On != nil && line.On.Name == theDeploymentStep {
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
			for _, source := range aScenarioSourceThatImports(t, taken.imported, theDeploymentLine) {
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
			line: `s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement().Customize(` +
				`pod.Request{CPU: 3, Memory: 12 * pod.Gi}))`,
			admitted: true,
		},
		"a keyed Node capacity": {
			line: `s.Given().Zones(3, node.Pool("general", node.PerZone(3), ` +
				`node.Capacity{CPU: 4, Memory: 16 * node.Gi, Pods: 10}))`,
			admitted: true,
		},
		"a Request by position": {
			line: `s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement().Customize(` +
				`pod.Request{3, 12 * pod.Gi}))`,
		},
		"a Node capacity by position": {
			line: `s.Given().Zones(3, node.Pool("general", node.PerZone(3), ` +
				`node.Capacity{4, 16 * node.Gi, 10}))`,
		},
		"a toleration by position": {
			line: `s.Given().Deployment(Checkout, 9, StandardPod(), ResilientPlacement().Customize(` +
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
