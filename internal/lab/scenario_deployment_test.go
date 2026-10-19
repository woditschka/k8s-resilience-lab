// The tests in this file cross to a real Simulated Cluster, which every other
// test of the Step surface does not: a Wait is where the declarations of a
// Scenario reach the control plane, and what the control plane makes of them is
// its own judgement, so a Scenario holds the concrete *cluster.SimulatedCluster
// it was provisioned with and no stand-in could report either
// (docs/testing-principles.md § Mocking Policy). They sit in the package because
// the Step a Wait reports, and the Step a refused value ends a Scenario at, are
// the Step surface's own rules.
package lab

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/woditschka/k8s-resilience-lab/internal/cluster"
	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/lab/node"
	"github.com/woditschka/k8s-resilience-lab/internal/lab/pod"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// The Scenarios the Simulated Clusters of this file are provisioned for, so each
// container on the engine says which Scenario it serves.
const (
	theRefusedSettingScenario  = "a-refused-setting"
	theWaitingScenario         = "a-wait-for-the-cluster"
	theNothingPendingScenario  = "a-wait-with-nothing-pending"
	theSettlingScenario        = "a-wait-for-the-settled-placement"
	theNothingDeclaredScenario = "a-wait-with-nothing-declared"
	theNodePoolsScenario       = "a-topology-of-node-pools"
	theGpuRequestScenario      = "a-request-only-gpu-nodes-hold"
	theLabelFailureScenario    = "a-failure-by-label"
	theNextWorkloadScenario    = "a-next-workload"
)

// atOnce is the time a Wait with nothing to wait for is reached inside. The
// control plane is polled rather than watched, so such a Wait is reached at the
// poll that finds nothing pending rather than instantly: a tenth of the lab's own
// waiting time is well above one poll and well short of waiting for a state to
// arrive.
const atOnce = scenario.WaitingTime / 10

const (
	// theRefusedRestartPolicy is a value the control plane refuses on a Deployment's
	// Pod template: a Deployment restarts its Pods always, so the API server admits
	// no other policy there and refuses the Deployment itself. The lab's own
	// vocabulary names no restart policy, which makes full access the one way a
	// Scenario reaches that field.
	theRefusedRestartPolicy = corev1.RestartPolicyNever

	// theDeployingCauseNames is what the cause of such an ending opens with, which
	// is what tells a reader the Wait named by the ending is where the Workload was
	// deployed.
	theDeployingCauseNames = "deploy a Workload"
)

// aFullAccessTheControlPlaneRefuses is full access naming that policy, written the
// way a Scenario writes one.
func aFullAccessTheControlPlaneRefuses() pod.Customization {
	return pod.FullAccess(func(spec *corev1.PodSpec) { spec.RestartPolicy = theRefusedRestartPolicy })
}

// aScenarioOnAClusterFor is a Scenario over a Simulated Cluster of its own, the
// way the Runner builds one: provisioned under that Scenario's name for this test
// alone and removed when it ends, however it ends. On a machine where no engine
// answers the calling test fails here naming why, and every test that needs none
// still runs (docs/testing-principles.md § Mocking Policy).
func aScenarioOnAClusterFor(t *testing.T, scenarioName string) *Scenario {
	t.Helper()

	engine, err := cluster.DetectEngine(t.Context(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("no container engine answers on this machine, so the Simulated Cluster cannot be driven: %v", err)
	}
	t.Cleanup(engine.Close)

	provisioned, err := cluster.Provision(t.Context(), engine, cluster.DefaultKubernetesVersion(), scenarioName)
	if err != nil {
		t.Fatalf("provision a Simulated Cluster on the engine at %s: %v", engine.Socket(), err)
	}
	t.Cleanup(func() {
		if err := provisioned.Close(context.WithoutCancel(t.Context())); err != nil {
			t.Errorf("remove the Simulated Cluster %s: %v", provisioned.Name(), err)
		}
	})

	return newScenario(t.Context(), provisioned)
}

// reportedStep is one Step as the tests here compare it: which kind it is and the
// name it reports, whether it carries a time it took, the time it was held to,
// whether the Scenario stated that time, and whether the Wait was reached. How
// long a Wait held varies from run to run, so what a comparison pins is that the
// line carries a time at all; the other three are the Scenario's own and are
// compared outright.
type reportedStep struct {
	Kind       StepKind
	Name       string
	Waited     bool
	HeldTo     time.Duration
	TimeStated bool
	Reached    bool
}

// asReported is every Step a Scenario recorded, read that way.
func asReported(steps []Step) []reportedStep {
	reported := make([]reportedStep, 0, len(steps))
	for _, step := range steps {
		reported = append(reported, reportedStep{
			Kind:       step.Kind,
			Name:       step.Name,
			Waited:     step.Waited > noWait,
			HeldTo:     step.HeldTo,
			TimeStated: step.TimeStated,
			Reached:    step.Reached,
		})
	}

	return reported
}

// reportedAsWaiting and reportedAtOnce are one expected Step read that way: a
// Wait that was reached, stating no time of its own and so held to the lab's own
// waiting time and marked as stating none, and a Step that waited for nothing.
func reportedAsWaiting(step Step) reportedStep {
	return reportedStep{Kind: step.Kind, Name: step.Name, Waited: true, HeldTo: scenario.WaitingTime, Reached: true}
}

func reportedAtOnce(step Step) reportedStep {
	return reportedStep{Kind: step.Kind, Name: step.Name}
}

// Both ways of writing a Wait are one Wait: it applies whatever the Givens above
// it declared, holds until the control plane reports every Node and then every Pod
// Ready, and reports a Wait line of its own with the time it held for. Written
// with the Given that declared it or as a Step of its own, the lines reported and
// the Placement reached are the same, and the Thens after it judge a Placement
// that has stopped changing (docs/prd.md REQ-SIM-006).
func TestAWaitWrittenWithItsGivenAndOneWrittenOnItsOwn_ShouldReportTheSameWaitLineAndPlacement(t *testing.T) {
	t.Parallel()
	forms := map[string]func(*Scenario){
		"with the Given that declared it": func(waiting *Scenario) {
			waiting.Given().Topology(Zones(twoZones), theGeneralNodePool()).WaitFor().AllNodesReady()
			waiting.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), resilientPlacement()).
				WaitFor().AllPodsReady()
		},
		"as a Step of its own": func(waiting *Scenario) {
			waiting.Given().Topology(Zones(twoZones), theGeneralNodePool())
			waiting.WaitFor().AllNodesReady()
			waiting.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), resilientPlacement())
			waiting.WaitFor().AllPodsReady()
		},
	}

	for written, form := range forms {
		t.Run(written, func(t *testing.T) {
			declared := aTopologyOf(t, twoZones, twoNodesPerZone)
			waiting := aScenarioOnAClusterFor(t, theWaitingScenario)

			form(waiting)
			waiting.Then().AllPodsRunning()
			waiting.Then().PodsPerNode(podsPerNodeWhenEven)

			waited := waiting.outcome()
			expect.Expect(t, causeOf(waited)).To(expect.Equal(""))
			expect.Expect(t, asReported(waited.Steps)).To(expect.Equal([]reportedStep{
				reportedAtOnce(topologyStepOf(twoZones, twoNodesPerZone)),
				reportedAsWaiting(allNodesReadyStep()),
				reportedAtOnce(resilientPlacementWorkloadStepOf(podsOverEveryNode)),
				reportedAsWaiting(allPodsReadyStep()),
				reportedAtOnce(allPodsRunningStep()),
				reportedAtOnce(podsPerNodeStepOf(podsPerNodeWhenEven)),
			}))
			expect.Expect(t, asJudged(waited.Findings)).To(expect.Equal([]judgedFinding{}))
			expect.Expect(t, waited.Placement.PodsNotReady()).To(expect.HaveLen[scenario.Pod](noPod))
			expect.Expect(t, waited.Placement.PodsPerNode()).
				To(expect.Equal(evenlyOver(nodesOf(declared), podsPerNodeWhenEven)))
		})
	}
}

// A Wait applies what is declared and not yet applied, so a Wait with nothing to
// wait for reports at once rather than being refused: the second of two Waits in a
// row finds nothing pending, and a Wait for every Pod before any Workload is
// declared waits for no Pod (docs/prd.md REQ-SIM-006 edge cases 1 and 2).
func TestAWaitWithNothingToWaitFor_ShouldReportAtOnceRatherThanBeRefused(t *testing.T) {
	t.Parallel()
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	waiting := aScenarioOnAClusterFor(t, theNothingPendingScenario)

	waiting.Given().Topology(Zones(twoZones), theGeneralNodePool()).WaitFor().AllNodesReady()
	waiting.WaitFor().AllPodsReady()
	waiting.WaitFor().AllPodsReady()

	waited := waiting.outcome()
	expect.Expect(t, causeOf(waited)).To(expect.Equal(""))
	expect.Expect(t, asReported(waited.Steps)).To(expect.Equal([]reportedStep{
		reportedAtOnce(topologyStepOf(twoZones, twoNodesPerZone)),
		reportedAsWaiting(allNodesReadyStep()),
		reportedAsWaiting(allPodsReadyStep()),
		reportedAsWaiting(allPodsReadyStep()),
	}))
	expect.Expect(t, waited.Placement.Pods()).To(expect.HaveLen[scenario.Pod](noPod))
	expect.Expect(t, waited.Placement.PodsPerNode()).To(expect.Equal(evenlyOver(nodesOf(declared), noPod)))
}

// A Wait for the Settled Placement holds for the Placement alone: it is reached
// once the control plane has accounted for every Pod the Workload declares,
// placed or not, so a Workload of one Pod more than its Nodes have room for
// reaches it with that Pod left Pending. A Wait for every Pod Ready would be
// outlasted by the lab's own waiting time in the same world, which is why this
// Wait is the one a Scenario of a Workload with no room writes (docs/prd.md
// REQ-SIM-011).
func TestAWaitForASettledPlacement_ShouldBeReachedWhilePodsStayPending(t *testing.T) {
	t.Parallel()
	waiting := aScenarioOnAClusterFor(t, theSettlingScenario)

	waiting.Given().Topology(Zones(twoZones), node.Pool(generalNodePool, node.PerZone(twoNodesPerZone),
		node.Capacity{CPU: someCoresPerNode, Memory: someGibibytesPerNode * node.Gi, Pods: roomForOnePodPerNode}))
	waiting.WaitFor().AllNodesReady()
	waiting.Given().Workload(someWorkloadName, Pods(podsBeyondEveryNode), resilientPlacement())
	waiting.WaitFor().PlacementSettled()

	settled := waiting.outcome()
	expect.Expect(t, causeOf(settled)).To(expect.Equal(""))
	expect.Expect(t, asReported(settled.Steps)).To(expect.Equal([]reportedStep{
		reportedAtOnce(topologyStepHoldingOnePodOf(twoZones, twoNodesPerZone)),
		reportedAsWaiting(allNodesReadyStep()),
		reportedAtOnce(resilientPlacementWorkloadStepOf(podsBeyondEveryNode)),
		reportedAsWaiting(settledPlacementStep()),
	}))
	expect.Expect(t, asJudged(settled.Findings)).To(expect.Equal([]judgedFinding{}))
	expect.Expect(t, settled.Placement.PodsInPhase(scenario.PodPending)).
		To(expect.HaveLen[scenario.Pod](onePodLeftPending))
	expect.Expect(t, settled.Placement.PodsInPhase(scenario.PodRunning)).
		To(expect.HaveLen[scenario.Pod](podsOverEveryNode))
}

// That Wait standing before any Workload is declared has nothing to wait for, so
// it is reached at once, as the Wait for every Pod in the same place is: the
// Placement it reaches holds no Pod, and no Node either where no Topology has
// been declared above it (docs/prd.md REQ-SIM-011).
func TestAWaitForASettledPlacementWithNothingDeclared_ShouldBeReachedAtOnce(t *testing.T) {
	t.Parallel()
	waiting := aScenarioOnAClusterFor(t, theNothingDeclaredScenario)

	waiting.WaitFor().PlacementSettled()

	reached := waiting.outcome()
	expect.Expect(t, causeOf(reached)).To(expect.Equal(""))
	expect.Expect(t, asReported(reached.Steps)).To(expect.Equal([]reportedStep{
		reportedAsWaiting(settledPlacementStep()),
	}))
	expectReachedAtOnce(t, reached.Steps)
	expect.Expect(t, reached.Placement.Pods()).To(expect.HaveLen[scenario.Pod](noPod))
	expect.Expect(t, reached.Placement.Nodes()).To(expect.HaveLen[scenario.Node](noNodeDeclared))
}

// expectReachedAtOnce asserts every Wait among these Steps was reached at once,
// which is what tells a Wait with nothing to wait for apart from one that held
// until the cluster changed. How long a Wait held varies from run to run, so what
// it is asserted against is a bound and not a time.
func expectReachedAtOnce(t *testing.T, recorded []Step) {
	t.Helper()

	for _, step := range recorded {
		if step.Kind == WaitStep && step.Waited >= atOnce {
			t.Errorf("a Wait with nothing to wait for is reached within %s, but reads: %s", atOnce, step)
		}
	}
}

// Edge case 5: a value the control plane refuses ends the Scenario at the Wait
// that deployed its Workload, naming that Step and what was refused. A Given
// declares and reaches no cluster, so the Wait below it is the Step that applied
// the declaration and the Step the ending names, with the declaration in its
// cause; no Step after it is carried out or judged. Full access is how a Scenario
// can write such a value; the rule is every pod.Customization's.
func TestFullAccessNamingASettingTheControlPlaneRefuses_ShouldEndTheScenarioAtTheWaitThatDeployedTheWorkload(
	t *testing.T,
) {
	t.Parallel()
	ending := aScenarioOnAClusterFor(t, theRefusedSettingScenario)

	ending.Given().Topology(Zones(twoZones), theGeneralNodePool())
	ending.Given().Workload(someWorkloadName, Pods(podsOverEveryNode),
		resilientPlacement().Customize(aFullAccessTheControlPlaneRefuses()))
	ending.WaitFor().AllPodsReady()
	ending.Then().PodsPerZone(podsPerZoneWhenEven)

	ended := ending.outcome()
	expect.Expect(t, ended.EndedAt).To(expect.Equal(allPodsReadyStep()))
	expect.Expect(t, asReported(ended.Steps)).To(expect.Equal([]reportedStep{
		reportedAtOnce(topologyStepOf(twoZones, twoNodesPerZone)),
		reportedAtOnce(resilientPlacementWorkloadStepOf(podsOverEveryNode)),
		reportedAtOnce(allPodsReadyStep()),
	}))
	expect.Expect(t, asJudged(ended.Findings)).To(expect.Equal([]judgedFinding{}))
	causeNames(t, ended, theDeployingCauseNames)
	causeNames(t, ended, string(theRefusedRestartPolicy))
}

// A Topology of NodePools reaches the cluster as every Zone holding each
// NodePool's Nodes Ready at that NodePool's capacity, numbered from one through
// the NodePools in declaration order, and a Node of the gpu NodePool carrying its
// label: the Nodes read back from the control plane equal those the Topology
// declared (docs/prd.md REQ-SIM-013).
func TestATopologyOfNodePools_ShouldProvisionEveryZoneWithEachNodePoolsNodesAndCapacity(t *testing.T) {
	t.Parallel()
	const threeZones = 3
	provisioning := aScenarioOnAClusterFor(t, theNodePoolsScenario)

	provisioning.Given().Topology(Zones(threeZones), theGeneralNodePool(), theGpuNodePool())
	provisioning.WaitFor().AllNodesReady()

	ready := provisioning.outcome()
	expect.Expect(t, causeOf(ready)).To(expect.Equal(""))
	expect.Expect(t, ready.Placement.Nodes()).To(expect.Equal(aTopologyOfNodePools(t, threeZones).Nodes()))
}

// The scheduler places a Pod by the room each Node has, so a Request only the gpu
// NodePool's Nodes have room for runs its Pods on those Nodes alone. The Workload
// declares no spread and one Pod per gpu Node, so room alone decides where they
// land (docs/prd.md REQ-SIM-013).
func TestARequestOnlyOneNodePoolHasRoomFor_ShouldPlacePodsOnThatNodePoolAlone(t *testing.T) {
	t.Parallel()
	const onePodPerGpuNode = 1
	podsTheGpuNodesHold := twoZones * oneGpuNodePerZone * onePodPerGpuNode
	aboveRoomy := pod.Request{CPU: roomyCoresPerNode + 1, Memory: (roomyGibibytesPerNode + 1) * pod.Gi}
	placing := aScenarioOnAClusterFor(t, theGpuRequestScenario)

	placing.Given().Topology(Zones(twoZones), theGeneralNodePool(), theGpuNodePool())
	placing.Given().Workload(someWorkloadName, Pods(podsTheGpuNodesHold), defaultPlacement().Customize(aboveRoomy))
	placing.WaitFor().PlacementSettled()

	settled := placing.outcome()
	expect.Expect(t, causeOf(settled)).To(expect.Equal(""))
	expect.Expect(t, settled.Placement.PodsInPhase(scenario.PodRunning)).
		To(expect.HaveLen[scenario.Pod](podsTheGpuNodesHold))
	expect.Expect(t, settled.Placement.PodsPerNode()).
		To(expect.Equal(onEveryNodeOf(aTopologyOfNodePools(t, twoZones), string(gpuNodePool), onePodPerGpuNode)))
}

// onEveryNodeOf is the Pods per Node a Placement holds where every Node of the
// NodePool named holds pods Pods and every other Node none.
func onEveryNodeOf(topology scenario.Topology, nodePool string, pods int) map[string]int {
	counts := make(map[string]int)
	for _, node := range topology.Nodes() {
		counts[node.Name] = 0
		if node.NodePool == nodePool {
			counts[node.Name] = pods
		}
	}

	return counts
}

// A Failure by label stops every Node the control plane reports carrying the
// value, in every Zone, in one Step: the Step reports the Nodes it resolved, the
// Pods are moved to the Nodes still responding, and none stays on a Node that
// stopped (docs/prd.md REQ-SIM-018).
func TestAFailureByLabel_ShouldStopEveryNodeCarryingThatValueInOneStep(t *testing.T) {
	t.Parallel()
	const (
		threeZones         = 3
		twoOnDemandPerZone = 2
		ninePods           = 9
	)
	stopping := aScenarioOnAClusterFor(t, theLabelFailureScenario)
	stopping.Given().Topology(Zones(threeZones),
		node.Pool(onDemandCapacity, node.PerZone(twoOnDemandPerZone), roomyNodes()).
			Labels(node.Labels{capacityTypeLabel: onDemandCapacity}),
		theSpotNodePool())
	stopping.WaitFor().AllNodesReady()
	stopping.Given().Workload(someWorkloadName, Pods(ninePods), resilientPlacement())
	stopping.WaitFor().AllPodsReady()
	spotNodes := namesOf(nodesOfNodePool(stopping.topology, spotCapacity))

	stopping.When().NodesLabelledFail(node.Labels{capacityTypeLabel: spotCapacity})
	stopping.WaitFor().AllPodsReady()

	stopped := stopping.outcome()
	expect.Expect(t, causeOf(stopped)).To(expect.Equal(""))
	expect.Expect(t, stopping.failed).To(expect.Equal(spotNodes))
	expect.Expect(t, namesOfWhensIn(stopped.Steps)).To(expect.Equal([]string{
		"Nodes labelled " + capacityTypeLabel + "=" + spotCapacity + " fail, " + strings.Join(spotNodes, ", "),
	}))
	expect.Expect(t, stopped.Placement.PodsInPhase(scenario.PodRunning)).To(expect.HaveLen[scenario.Pod](ninePods))
	expect.Expect(t, podsOn(stopped.Placement, spotNodes)).To(expect.HaveLen[scenario.Pod](0))
}

// podsOn are the Pods of the Placement sitting on one of these Nodes.
func podsOn(placed scenario.Placement, nodes []string) []scenario.Pod {
	return slices.DeleteFunc(placed.Pods(), func(sitting scenario.Pod) bool { return !slices.Contains(nodes, sitting.Node) })
}

// namesOfWhensIn are the names of the When Steps among these, as they read.
func namesOfWhensIn(steps []Step) []string {
	var names []string
	for _, step := range steps {
		if step.Kind == WhenStep {
			names = append(names, step.Name)
		}
	}

	return names
}

// The taint the gpu NodePool of the tests below states, as a dedicated NodePool
// of gpu Nodes is tainted.
const (
	gpuTaintKey     = "nvidia.com/gpu"
	gpuTaintPresent = "present"
)

// theTaintedGpuNodePool is the gpu NodePool tainted NoSchedule, which keeps off
// it every Pod not tolerating the taint.
func theTaintedGpuNodePool() node.DeclaredPool {
	return theGpuNodePool().Taints(node.Taint{Key: gpuTaintKey, Value: gpuTaintPresent, Effect: node.NoSchedule})
}

// toleratingTheGpuTaint is the toleration of that taint, naming its value and no
// waiting time.
func toleratingTheGpuTaint() pod.Toleration {
	return pod.Toleration{Key: gpuTaintKey, Operator: pod.Equal, Value: gpuTaintPresent, Effect: pod.NoSchedule}
}

// aboveRoomyRequest is a Request only a gpu Node has room for, one Pod of it per
// gpu Node, so room alone decides where such Pods land.
func aboveRoomyRequest() pod.Request {
	return pod.Request{CPU: roomyCoresPerNode + 1, Memory: (roomyGibibytesPerNode + 1) * pod.Gi}
}

// A later Workload is deployed by the next Wait beside the earlier, which stays
// Running where it was placed, and every Step after it judges the later one
// alone: the Pods tolerating the taint and asking for more than a general Node
// holds are Running on the gpu Nodes, and none on a general Node.
func TestALaterWorkload_ShouldBeDeployedBesideTheEarlierAndJudgedAlone(t *testing.T) {
	t.Parallel()
	gpuNodes := twoZones * oneGpuNodePerZone
	generalPods := twoZones * twoNodesPerZone
	const noPod = 0
	deploying := aScenarioOnAClusterFor(t, theNextWorkloadScenario)
	deploying.Given().Topology(Zones(twoZones), theGeneralNodePool(), theTaintedGpuNodePool())
	deploying.WaitFor().AllNodesReady()
	deploying.Given().Workload(someWorkloadName, Pods(generalPods), resilientPlacement())
	deploying.WaitFor().AllPodsReady()
	earlier := deploying.workload

	deploying.Given().Workload(anotherWorkloadName, Pods(gpuNodes),
		defaultPlacement().Customize(aboveRoomyRequest(), toleratingTheGpuTaint()))
	deploying.WaitFor().AllPodsReady()
	deploying.Then().AllPodsRunning()
	deploying.Then().PodsOnNodePool(gpuNodePool, gpuNodes)
	deploying.Then().PodsOnNodePool(generalNodePool, noPod)

	judged := deploying.outcome()
	expect.Expect(t, causeOf(judged)).To(expect.Equal(""))
	expect.Expect(t, asJudged(judged.Findings)).To(expect.Equal([]judgedFinding{}))
	expect.Expect(t, judged.Placement.Pods()).To(expect.HaveLen[scenario.Pod](gpuNodes))
	stillPlaced, err := deploying.provisioned.Observe(t.Context(), earlier)
	if err != nil {
		t.Fatalf("observe the earlier Workload: %v", err)
	}
	expect.Expect(t, stillPlaced.PodsPerNodePool()).
		To(expect.Equal(map[string]int{string(generalNodePool): generalPods, string(gpuNodePool): noPod}))
}
