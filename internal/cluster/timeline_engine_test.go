//go:build !unit

package cluster

import (
	"cmp"
	"maps"
	"slices"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// One Node with room for a single Pod and two Pods, so the control plane leaves
// one Pending whatever it does.
const (
	theOneZone     = 1
	theOneNodeInIt = 1
	roomForOnePod  = 1
	twoPods        = 2

	theOneReplicaSet = 1
)

const (
	podScheduled        = "Scheduled"
	podSchedulingFailed = "FailedScheduling"
)

func aTopologyOfOneNodeHoldingOnePod(t *testing.T) scenario.Topology {
	t.Helper()

	capacity := aCapacityOf(t, roomForOnePod, someCPUPerNode, someMemoryPerNode)
	topology, err := topologyOfTheGeneralNodePool(theOneZone, theOneNodeInIt, capacity)
	if err != nil {
		t.Fatalf("build a Topology of one Node: %v", err)
	}

	return topology
}

// The recording starts before the first Node is created and stops after the
// Placement settles, the sequence the runner follows.
func aRecordedScenario(t *testing.T, topology scenario.Topology, deployment scenario.Deployment) (scenario.Timeline, []scenario.Entry, scenario.Placement) {
	t.Helper()

	provisioned, _ := aProvisionedCluster(t)
	delivered := &entriesDelivered{}
	if err := provisioned.Record(t.Context(), delivered); err != nil {
		t.Fatalf("record the Timeline of %s: %v", provisioned.Name(), err)
	}

	deployInto(t, provisioned, topology, deployment)
	settled, err := provisioned.AwaitSettled(t.Context(), deployment)
	if err != nil {
		t.Fatalf("await the Placement settling: %v", err)
	}

	return provisioned.StopRecording(), delivered.entries, settled
}

func TestAScenarioWithATopologyAndADeployment_ShouldRecordEveryNodePodAndDeploymentChange(t *testing.T) {
	t.Parallel()
	topology, deployment := anEvenSpreadTopology(t), aDeploymentDeclaring(t, ninePods, resilientPlacement(t))

	recorded, delivered, settled := aRecordedScenario(t, topology, deployment)

	expect.Expect(t, namesRecorded(recorded, scenario.NodeObject)).To(expect.Equal(nodesOf(topology)))
	expect.Expect(t, namesRecorded(recorded, scenario.PodObject)).
		To(expect.Equal(everyPodNameOf(topology, podsOf(settled))))
	expect.Expect(t, namesRecorded(recorded, scenario.DeploymentObject)).
		To(expect.Equal([]string{deployedDeploymentName}))
	expect.Expect(t, namesRecorded(recorded, scenario.ReplicaSetObject)).To(expect.HaveLen[string](theOneReplicaSet))
	expect.Expect(t, delivered).To(expect.Equal(recorded.Entries()))
}

func TestARecordedTimeline_ShouldHoldItsEntriesInTheOrderTheLabReceivedThem(t *testing.T) {
	t.Parallel()
	recorded, _, _ := aRecordedScenario(t, anEvenSpreadTopology(t), aDeploymentDeclaring(t, ninePods, resilientPlacement(t)))

	expect.Expect(t, slices.IsSortedFunc(recorded.Entries(), byReceipt)).To(expect.Equal(true))
}

func TestAPlacedPodAndAPendingPod_ShouldCarryTheReasonTheControlPlaneGave(t *testing.T) {
	t.Parallel()
	deployment := aDeploymentDeclaring(t, twoPods, resilientPlacement(t))

	recorded, _, settled := aRecordedScenario(t, aTopologyOfOneNodeHoldingOnePod(t), deployment)

	placed, pending := settled.PodsInPhase(scenario.PodRunning), settled.PodsInPhase(scenario.PodPending)
	expect.Expect(t, placed).To(expect.HaveLen[scenario.Pod](roomForOnePod))
	expect.Expect(t, pending).To(expect.HaveLen[scenario.Pod](twoPods - roomForOnePod))
	expectReported(t, recorded, placed[0].Name, podScheduled)
	expectReported(t, recorded, pending[0].Name, podSchedulingFailed)
}

func TestAClusterAlreadyRecording_ShouldRefuseASecondRecord(t *testing.T) {
	t.Parallel()
	provisioned, _ := aProvisionedCluster(t)
	delivered := &entriesDelivered{}
	if err := provisioned.Record(t.Context(), delivered); err != nil {
		t.Fatalf("record the Timeline of %s: %v", provisioned.Name(), err)
	}
	t.Cleanup(func() { provisioned.StopRecording() })

	refused := provisioned.Record(t.Context(), delivered)

	if refused == nil {
		t.Fatalf("%s took a second record of its Timeline rather than refusing one", provisioned.Name())
	}
	expectNaming(t, refused.Error(), provisioned.Name())
}

func expectReported(t *testing.T, recorded scenario.Timeline, pod, reason string) {
	t.Helper()

	reported := reasonsReportedFor(recorded, pod)
	if !slices.Contains(reported, reason) {
		t.Errorf("the Timeline carries the reason %s for the Pod %s, but reports %v", reason, pod, reported)
	}
}

func reasonsReportedFor(recorded scenario.Timeline, pod string) []string {
	var reported []string
	for _, entry := range recorded.Entries() {
		if entry.Change == scenario.ChangeReported && entry.Name == pod {
			reported = append(reported, entry.Reason)
		}
	}

	return reported
}

func namesRecorded(recorded scenario.Timeline, kind scenario.ObjectKind) []string {
	named := make(map[string]bool)
	for _, entry := range recorded.Entries() {
		if entry.Kind == kind {
			named[entry.Name] = true
		}
	}

	return slices.Sorted(maps.Keys(named))
}

func nodesOf(topology scenario.Topology) []string {
	declared := topology.Nodes()
	names := make([]string, 0, len(declared))
	for _, node := range declared {
		names = append(names, node.Name)
	}

	return slices.Sorted(slices.Values(names))
}

func podsOf(settled scenario.Placement) []string {
	observed := settled.Pods()
	names := make([]string, 0, len(observed))
	for _, pod := range observed {
		names = append(names, pod.Name)
	}

	return names
}

func everyPodNameOf(topology scenario.Topology, deploymentPods []string) []string {
	named := make([]string, 0, len(deploymentPods))
	for _, node := range topology.Nodes() {
		named = append(named, probePodNamePrefix+node.Name)
	}

	return slices.Sorted(slices.Values(append(named, deploymentPods...)))
}

func byReceipt(one, next scenario.Entry) int {
	return cmp.Compare(one.Elapsed, next.Elapsed)
}
