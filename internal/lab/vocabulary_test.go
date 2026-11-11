// The tests needing a container engine and those needing none share these
// names, so this file carries neither kind's build constraint.

package lab

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/cluster"
	"github.com/woditschka/k8s-resilience-lab/internal/lab/node"
	"github.com/woditschka/k8s-resilience-lab/internal/lab/pod"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// observed carries a board as it reads, so a comparison needs no access to its
// fields.
type observed struct {
	Lines       []string
	NotObserved string
}

func deschedulerPassStepUnder(name string) Step {
	step := deschedulerPassStepNamingNoPolicy()
	step.Name += " under " + name

	return step
}

// theStopNamedFor opens the sentence a Step left undone is named with.
func theStopNamedFor(name string) string {
	return "the Scenario " + name + " ended at "
}

func theScenariosThatWentWrongIn(carried []Carried) []string {
	var wentWrong []string
	for _, each := range carried {
		if each.Err != nil {
			wentWrong = append(wentWrong, each.Outcome.Name)
		}
	}

	return wentWrong
}

func theFailureOf(carried []Carried, named string) string {
	for _, each := range carried {
		if each.Outcome.Name == named && each.Err != nil {
			return each.Err.Error()
		}
	}

	return ""
}

func theKindsOf(steps []Step) []StepKind {
	kinds := make([]StepKind, 0, len(steps))
	for _, step := range steps {
		kinds = append(kinds, step.Kind)
	}

	return kinds
}

const (
	gpuTaintKey     = "nvidia.com/gpu"
	gpuTaintPresent = "present"
)

const (
	// More than one Node per Zone makes an even Placement a decision rather
	// than the only one available.
	twoZones        = 2
	twoNodesPerZone = 2

	podsOverEveryNode = twoZones * twoNodesPerZone

	podsPerZoneWhenEven = podsOverEveryNode / twoZones
	podsPerNodeWhenEven = 1

	// twoPodsPerNode is below roomyPodsPerNode, so a Topology declared at it
	// differs from one naming Roomy. No test here turns on the cpu and memory.
	twoPodsPerNode       = 2
	someCoresPerNode     = 4
	someGibibytesPerNode = 16
	someCPUSpelled       = "4"
	someMemorySpelled    = "16Gi"

	roomForOnePodPerNode = 1
	onePodLeftPending    = 1
	podsBeyondEveryNode  = podsOverEveryNode + onePodLeftPending
	noPodPending         = 0

	noNodeDeclared = 0

	// A Topology of two Zones of two Nodes, so one Node holds one Pod of an
	// even spread and one Zone two.
	floorSurvivingOneNode = podsOverEveryNode - podsPerNodeWhenEven
	floorSurvivingOneZone = podsOverEveryNode - podsPerZoneWhenEven

	oneLost = 1

	// A loss reads as a report line states it, after the count it is written
	// with.
	oneNodeLost   = "1 Node"
	oneZoneLost   = "1 Zone"
	everyZoneLost = "2 Zones"

	noNodesPerZone = 0
	noPods         = 0
	noReplicas     = 0

	moreNodesThanTheTopologyHolds = twoZones*twoNodesPerZone + 1
	moreZonesThanTheTopologyHolds = twoZones + 1

	theWhenThatNeverRan = "no When"

	oneNodeFailing = 1

	theToleranceNeverDeclared = "no Deployment declares one"

	noSpecificationAtAll = ""

	onePodLeftBehind       = 1
	podLeftOnTheFailedNode = string(someDeploymentName) + "-left-behind"

	podLeftPending = string(someDeploymentName) + "-left-pending"

	// aFinelyStatedWait is stated more finely than a report states it, so the
	// rounding is read on it.
	aFinelyStatedWait = 6234 * time.Millisecond

	aTimeAWaitStates         = 15
	aWaitPastTheTimeItStates = 25500 * time.Millisecond

	// A Wait with nothing to wait for takes no measured time, so whether it was
	// reached is read off the Step rather than off this time.
	aWaitOfNoMeasuredTime = 0

	theWaitsOwnStep = 0

	theWaitingTimeInSeconds = int(scenario.WaitingTime / time.Second)

	theSecondsAboveTheLabsWaitingTime = theWaitingTimeInSeconds + 1
	noSecondsAtAll                    = 0

	// someDeploymentName is named here rather than taken from the Scenarios'
	// place, which internal/lab does not import.
	someDeploymentName DeploymentName = "some-deployment"
)

// The lab defines no Named Node capacity, so this suite defines its own. Its
// Pod count leaves room for more than one Pod per Node.
const roomyName node.CapacityName = "Roomy"

const (
	roomyCoresPerNode     = 16
	roomyGibibytesPerNode = 64
	roomyPodsPerNode      = 10
)

func roomyNodes() node.Capacity {
	return node.Named(roomyName, node.Capacity{CPU: roomyCoresPerNode, Memory: roomyGibibytesPerNode * node.Gi, Pods: roomyPodsPerNode})
}

func aTopologyOf(t *testing.T, zones, nodesPerZone int) scenario.Topology {
	t.Helper()

	return aTopologyHolding(t, zones, nodesPerZone, theCapacityRoomyStandsFor(t))
}

func theCapacityRoomyStandsFor(t *testing.T) scenario.NodeCapacity {
	t.Helper()

	capacity, err := scenario.NewNodeCapacity(roomyPodsPerNode, coresOf(roomyCoresPerNode), gibibytesOf(roomyGibibytesPerNode))
	if err != nil {
		t.Fatalf("build the capacity Roomy stands for: %v", err)
	}

	return capacity
}

func aTopologyHolding(t *testing.T, zones, nodesPerZone int, capacity scenario.NodeCapacity) scenario.Topology {
	t.Helper()

	pool, err := scenario.NewNodePool(string(generalNodePool), nodesPerZone, capacity)
	if err != nil {
		t.Fatalf("build a NodePool of %d Nodes per Zone: %v", nodesPerZone, err)
	}
	topology, err := scenario.NewTopology(zones, pool)
	if err != nil {
		t.Fatalf("build a Topology of %d Zones of %d Nodes: %v", zones, nodesPerZone, err)
	}

	return topology
}

func aDeploymentNamedSurvivingTheLossOf(t *testing.T, name DeploymentName, pods int, lost FailureTolerance,
	against scenario.Topology,
) scenario.Deployment {
	t.Helper()

	specification, err := resilientPlacement().Declared()
	if err != nil {
		t.Fatalf("define Resilient placement: %v", err)
	}
	declared, err := scenario.NewDeployment(string(name), pods, specification)
	if err != nil {
		t.Fatalf("build a Deployment %s of %d replicas declaring Resilient placement: %v", name, pods, err)
	}
	deployment, err := declared.WithFailureTolerance(lost.tolerance, against)
	if err != nil {
		t.Fatalf("declare a Failure tolerance of %s: %v", lost.tolerance.Reads(), err)
	}

	return deployment
}

// The first Node the Topology derives is the pick internal/scenario makes.
func theNodeThatStopped(topology scenario.Topology) string {
	return topology.Nodes()[0].Name
}

func theNodesOutside(topology scenario.Topology, zone string) []scenario.Node {
	declared := topology.Nodes()
	outside := make([]scenario.Node, 0, len(declared))
	for _, node := range declared {
		if node.Zone != zone {
			outside = append(outside, node)
		}
	}

	return outside
}

// A Step's name is what a report shows and what a Finding points at, so this
// suite states it.
func topologyStepOf(zones, nodesPerZone int) Step {
	return Step{
		Kind: GivenStep,
		Name: fmt.Sprintf("a Topology of %d Zones of a NodePool %s of %d %s Nodes holding %d cpu, %dGi memory and %d Pods each",
			zones, generalNodePool, nodesPerZone, roomyName, roomyCoresPerNode, roomyGibibytesPerNode, roomyPodsPerNode),
	}
}

func topologyStepHolding(zones, nodesPerZone int, podsSpelled string) Step {
	return topologyStepHoldingCapacity(zones, nodesPerZone,
		fmt.Sprintf("%s cpu, %s memory and %s", someCPUSpelled, someMemorySpelled, podsSpelled))
}

// One template, so the lines cannot drift apart here.
func topologyStepHoldingCapacity(zones, nodesPerZone int, capacity string) Step {
	return Step{
		Kind: GivenStep,
		Name: fmt.Sprintf("a Topology of %d Zones of a NodePool %s of %d Nodes holding %s each",
			zones, generalNodePool, nodesPerZone, capacity),
	}
}

func deploymentStepNamedOf(name DeploymentName, pods int, declaring string) Step {
	return Step{
		Kind: GivenStep,
		Name: fmt.Sprintf("a Deployment %s of %d replicas declaring %s", name, pods, declaring),
	}
}

func resilientPlacementDeploymentStepOf(pods int) Step {
	return resilientPlacementDeploymentStepNamed(someDeploymentName, pods)
}

func resilientPlacementDeploymentStepNamed(name DeploymentName, pods int) Step {
	return carrying(deploymentStepNamedOf(name, pods, resilientPlacementReads()), resilientPlacementCarries()...)
}

func resilientPlacementReads() string {
	return fmt.Sprintf("%s, each Pod starting in %s to %s", resilientPlacementNamed,
		someShortestStartupMillis*time.Millisecond, someLongestStartupMillis*time.Millisecond)
}

func carrying(step Step, settings ...string) Step {
	step.Settings = settings

	return step
}

// A toleration is reported at the lab's waiting time, the one time the domain
// holds it at.
func aTolerationLine(taint string) string {
	return fmt.Sprintf("tolerations: {key: %s, operator: Exists, effect: NoExecute, tolerationSeconds: %d}",
		taint, theLabsWaitInSeconds())
}

func aSpreadLine(topologyKey string, maxSkew int) string {
	return fmt.Sprintf(
		"topologySpreadConstraints: {topologyKey: %s, maxSkew: %d, whenUnsatisfiable: DoNotSchedule, nodeTaintsPolicy: Honor}",
		topologyKey, maxSkew)
}

func theLeavingLines() []string {
	return []string{aTolerationLine(scenario.TaintNodeNotReady), aTolerationLine(scenario.TaintNodeUnreachable)}
}

func resilientPlacementCarries() []string {
	return append(theLeavingLines(), aSpreadLine(scenario.ZoneLabel, anEvenSkew), aSpreadLine(scenario.HostnameLabel, anEvenSkew))
}

func allPodsRunningStep() Step {
	return Step{Kind: ThenStep, Name: "every Pod Running"}
}

func podsPerNodeStepOf(pods int) Step {
	return Step{Kind: ThenStep, Name: fmt.Sprintf("%d Pod per Node", pods)}
}

func allNodesReadyStep() Step {
	return Step{Kind: WaitStep, Name: allNodesReady}
}

func allPodsReadyStep() Step {
	return Step{Kind: WaitStep, Name: allPodsReady}
}

// enoughPodsStayedReadyStep is also the line a Scenario with nothing to judge
// ends at, recorded before any reading.
func enoughPodsStayedReadyStep(readings ...string) Step {
	return Step{Kind: ThenStep, Name: "enough Pods stayed Ready from the first Failure", Readings: readings}
}

// The mismatch text is internal/expect's and is not this suite's to pin.
type judgedFinding struct {
	Step        Step
	Mismatching bool
}

func asJudged(findings []Finding) []judgedFinding {
	judged := make([]judgedFinding, 0, len(findings))
	for _, finding := range findings {
		judged = append(judged, judgedFinding{Step: finding.Step, Mismatching: finding.Mismatch != ""})
	}

	return judged
}

// The gpu NodePool's cpu and memory are double Roomy's, so its Nodes differ in
// both; no test here turns on its Pod count.
const (
	generalNodePool node.PoolName = "general"
	gpuNodePool     node.PoolName = "gpu"
)

const (
	oneGpuNodePerZone  = 1
	gpuCoresPerNode    = 32
	gpuGibibytesNode   = 128
	someGpuPodsPerNode = 10
)

func theGpuNodePool() node.DeclaredPool {
	return node.Pool(gpuNodePool, node.PerZone(oneGpuNodePerZone),
		node.Capacity{CPU: gpuCoresPerNode, Memory: gpuGibibytesNode * node.Gi, Pods: someGpuPodsPerNode})
}

func theGeneralNodePool() node.DeclaredPool {
	return node.Pool(generalNodePool, node.PerZone(twoNodesPerZone), roomyNodes())
}

func aTopologyOfNodePools(t *testing.T, zones int) scenario.Topology {
	t.Helper()

	gpuCapacity, err := scenario.NewNodeCapacity(someGpuPodsPerNode, coresOf(gpuCoresPerNode), gibibytesOf(gpuGibibytesNode))
	if err != nil {
		t.Fatalf("build the gpu NodePool's capacity: %v", err)
	}
	general, err := scenario.NewNodePool(string(generalNodePool), twoNodesPerZone, theCapacityRoomyStandsFor(t))
	if err != nil {
		t.Fatalf("build the general NodePool: %v", err)
	}
	gpu, err := scenario.NewNodePool(string(gpuNodePool), oneGpuNodePerZone, gpuCapacity)
	if err != nil {
		t.Fatalf("build the gpu NodePool: %v", err)
	}
	topology, err := scenario.NewTopology(zones, general, gpu)
	if err != nil {
		t.Fatalf("build a Topology of %d Zones of two NodePools: %v", zones, err)
	}

	return topology
}

const (
	capacityTypeLabel    = "capacity-type"
	onDemandCapacity     = "on-demand"
	spotCapacity         = "spot"
	oneNodePerZoneOfEach = 1
)

func theSpotNodePool() node.DeclaredPool {
	return node.Pool(spotCapacity, node.PerZone(oneNodePerZoneOfEach), roomyNodes()).
		Labels(node.Labels{capacityTypeLabel: spotCapacity})
}

func linesOf(readings []scenario.FloorReading) []string {
	lines := make([]string, 0, len(readings))
	for _, reading := range readings {
		lines = append(lines, reading.Line())
	}

	return lines
}

func causeOf(reported Outcome) string {
	if reported.Cause == nil {
		return ""
	}

	return reported.Cause.Error()
}

func causeNames(t *testing.T, ended Outcome, named string) {
	t.Helper()

	if !strings.Contains(causeOf(ended), named) {
		t.Errorf("the Scenario ended for a reason naming %q, but it reads: %s", named, causeOf(ended))
	}
}

const anotherDeploymentName DeploymentName = "another-deployment"

func theDeschedulerPairedByDefault(t *testing.T) cluster.DeschedulerVersion {
	t.Helper()

	paired, err := cluster.DefaultKubernetesVersion().PairedDescheduler()
	if err != nil {
		t.Fatalf("pair a Descheduler with the default Kubernetes version: %v", err)
	}

	return paired
}

func deschedulerPolicyStepNaming(name string) Step {
	return Step{Kind: GivenStep, Name: "a Descheduler policy " + name}
}

func deschedulerPolicyStepOf(name string, paired cluster.DeschedulerVersion) Step {
	named := deschedulerPolicyStepNaming(name)
	named.Name += " for Descheduler " + paired.String()

	return named
}

func deschedulerPassStepNamingNoPolicy() Step {
	return Step{Kind: WhenStep, Name: "one Descheduler pass"}
}

// No test turns on the interval but those naming or refusing it.
const (
	aPassEveryFiveSeconds = 5
	anIntervalOfNoSeconds = 0
)

func deschedulerOnAnIntervalStepOf(name string, paired cluster.DeschedulerVersion, seconds int) Step {
	step := deschedulerPolicyStepOf(name, paired)
	step.Name += fmt.Sprintf(", a pass every %d seconds", seconds)

	return step
}

const (
	defaultPlacementNamed   pod.SpecificationName = "Default placement"
	resilientPlacementNamed pod.SpecificationName = "Resilient placement"
)

// aWiderSkew differs from the definition's, so a customized specification is
// read back as the Scenario's value and not the definition's.
const (
	anEvenSkew = 1
	aWiderSkew = 2
)

// No test of this package turns on the Startup time.
const (
	someShortestStartupMillis = 500
	someLongestStartupMillis  = 1000
)

func someStartupTime() pod.StartupTime {
	return pod.StartupMillisBetween(someShortestStartupMillis, someLongestStartupMillis)
}

// The lab defines no specification and a test of internal/lab cannot import the
// Scenarios' place, so the tests define their own.
func defaultPlacement() pod.Specification {
	return pod.Predefined(defaultPlacementNamed, someStartupTime(), leavingAnUnresponsiveNode())
}

func resilientPlacement() pod.Specification {
	return pod.Predefined(resilientPlacementNamed, someStartupTime(),
		leavingAnUnresponsiveNode(),
		aZoneSpreadAt(anEvenSkew),
		pod.TopologySpreadConstraint{
			TopologyKey: node.Hostname, MaxSkew: anEvenSkew, WhenUnsatisfiable: pod.DoNotSchedule, NodeTaintsPolicy: pod.Honor,
		},
	)
}

// The lab defines no Named setting and the test cannot import the Scenarios'
// place, so the two tolerations are written directly.
func leavingAnUnresponsiveNode() pod.Customization {
	return pod.Settings(
		pod.Toleration{Key: node.NotReady, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: node.UnresponsiveWait},
		pod.Toleration{Key: node.Unreachable, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: node.UnresponsiveWait},
	)
}

func theLabsWaitInSeconds() int64 {
	return int64(scenario.UnresponsiveNodeWait.Seconds())
}

func aZoneSpreadAt(maxSkew int) pod.Setting {
	return pod.TopologySpreadConstraint{
		TopologyKey: node.Zone, MaxSkew: maxSkew, WhenUnsatisfiable: pod.DoNotSchedule, NodeTaintsPolicy: pod.Honor,
	}
}

// askingForNothing is the Deployment line's Request where the test asserts none,
// so its Pods ask for no cpu and no memory.
func askingForNothing() pod.Customization {
	return pod.Settings()
}

func aRequestOf(cores, gibibytes int) pod.Setting {
	return pod.Request{CPU: cores, Memory: scenario.Memory(gibibytes) * pod.Gi}
}

const millicoresPerCore = 1000

func coresOf(cores int) scenario.CPU {
	return scenario.CPU(cores * millicoresPerCore)
}

func gibibytesOf(gibibytes int) scenario.Memory {
	return scenario.Memory(gibibytes) * node.Gi
}
