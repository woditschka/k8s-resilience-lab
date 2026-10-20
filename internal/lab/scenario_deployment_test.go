// These tests cross to a real Simulated Cluster: a Wait is where a Scenario's
// declarations reach the control plane, and what the control plane makes of
// them is its own judgement, which no stand-in could report.
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

// Each container on the engine says which Scenario it serves.
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

// The control plane is polled rather than watched, so a Wait with nothing to
// wait for is reached at the first poll rather than instantly. A tenth of the
// lab's waiting time is well above one poll and well short of waiting for a
// state to arrive.
const atOnce = scenario.WaitingTime / 10

const (
	// A Deployment restarts its Pods always, so the API server refuses any
	// other restart policy there. The lab's vocabulary names no restart policy,
	// which makes full access the one way to reach that field.
	theRefusedRestartPolicy = corev1.RestartPolicyNever

	theDeployingCauseNames = "deploy a Workload"
)

func aFullAccessTheControlPlaneRefuses() pod.Customization {
	return pod.FullAccess(func(spec *corev1.PodSpec) { spec.RestartPolicy = theRefusedRestartPolicy })
}

// aScenarioOnAClusterFor fails only the calling test where no engine answers,
// so every test that needs none still runs.
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

// How long a Wait held varies from run to run, so a comparison pins only that
// the line carries a time at all.
type reportedStep struct {
	Kind       StepKind
	Name       string
	Waited     bool
	HeldTo     time.Duration
	TimeStated bool
	Reached    bool
}

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

func reportedAsWaiting(step Step) reportedStep {
	return reportedStep{Kind: step.Kind, Name: step.Name, Waited: true, HeldTo: scenario.WaitingTime, Reached: true}
}

func reportedAtOnce(step Step) reportedStep {
	return reportedStep{Kind: step.Kind, Name: step.Name}
}

// Written with its Given or as a Step of its own, a Wait reports the same lines
// and reaches the same Placement.
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

// A Wait for every Pod Ready would be outlasted by the lab's waiting time in
// the same world, which is why this Wait is the one a Scenario of a Workload
// with no room writes.
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

// How long a Wait held varies from run to run, so it is asserted against a
// bound and not a time.
func expectReachedAtOnce(t *testing.T, recorded []Step) {
	t.Helper()

	for _, step := range recorded {
		if step.Kind == WaitStep && step.Waited >= atOnce {
			t.Errorf("a Wait with nothing to wait for is reached within %s, but reads: %s", atOnce, step)
		}
	}
}

// A Given reaches no cluster, so the Wait below it is the Step that applied the
// declaration and the Step the ending names. Full access is how a Scenario can
// write such a value; the rule is every Customization's.
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

// The Workload declares no spread and one Pod per gpu Node, so room alone
// decides where its Pods land.
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

func podsOn(placed scenario.Placement, nodes []string) []scenario.Pod {
	return slices.DeleteFunc(placed.Pods(), func(sitting scenario.Pod) bool { return !slices.Contains(nodes, sitting.Node) })
}

func namesOfWhensIn(steps []Step) []string {
	var names []string
	for _, step := range steps {
		if step.Kind == WhenStep {
			names = append(names, step.Name)
		}
	}

	return names
}

const (
	gpuTaintKey     = "nvidia.com/gpu"
	gpuTaintPresent = "present"
)

func theTaintedGpuNodePool() node.DeclaredPool {
	return theGpuNodePool().Taints(node.Taint{Key: gpuTaintKey, Value: gpuTaintPresent, Effect: node.NoSchedule})
}

func toleratingTheGpuTaint() pod.Toleration {
	return pod.Toleration{Key: gpuTaintKey, Operator: pod.Equal, Value: gpuTaintPresent, Effect: pod.NoSchedule}
}

// One Pod of aboveRoomyRequest fits per gpu Node, so room alone decides where
// such Pods land.
func aboveRoomyRequest() pod.Request {
	return pod.Request{CPU: roomyCoresPerNode + 1, Memory: (roomyGibibytesPerNode + 1) * pod.Gi}
}

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
