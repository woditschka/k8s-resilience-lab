// The one test in this file crosses to a real Simulated Cluster, which every
// other test of the Step surface does not: whether the control plane refuses a
// declared value is the control plane's own judgement, and a Scenario holds the
// concrete *cluster.SimulatedCluster it was provisioned with, so no stand-in could
// report that refusal (docs/testing-principles.md § Mocking Policy). It sits in
// the package because the Step a refused value ends a Scenario at is the Step
// surface's own rule.
package lab

import (
	"context"
	"log/slog"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/woditschka/k8s-resilience-lab/internal/cluster"
	"github.com/woditschka/k8s-resilience-lab/internal/expect"
)

const (
	// theRefusedSettingScenario names the Scenario this file's Simulated Cluster is
	// provisioned for, so the container on the engine says which Scenario it serves.
	theRefusedSettingScenario = "a-refused-setting"

	// theRefusedRestartPolicy is a value the control plane refuses on a Deployment's
	// Pod template: a Deployment restarts its Pods always, so the API server admits
	// no other policy there and refuses the Deployment itself. The lab's own
	// vocabulary names no restart policy, which makes full access the one way a
	// Scenario reaches that field.
	theRefusedRestartPolicy = corev1.RestartPolicyNever

	// theDeployingStepNames is what the cause of such an ending opens with, which is
	// what tells a reader the Workload never reached the cluster.
	theDeployingStepNames = "deploy the Workload"
)

// aFullAccessTheControlPlaneRefuses is full access naming that policy, written the
// way a Scenario writes one.
func aFullAccessTheControlPlaneRefuses() Customization {
	return FullAccess(func(pod *corev1.PodSpec) { pod.RestartPolicy = theRefusedRestartPolicy })
}

// aScenarioOnAProvisionedCluster is a Scenario over a Simulated Cluster of its
// own, the way the Runner builds one: provisioned for this test alone and removed
// when it ends, however it ends. On a machine where no engine answers the calling
// test fails here naming why, and every test that needs none still runs
// (docs/testing-principles.md § Mocking Policy).
func aScenarioOnAProvisionedCluster(t *testing.T) *Scenario {
	t.Helper()

	engine, err := cluster.DetectEngine(t.Context(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("no container engine answers on this machine, so the Simulated Cluster cannot be driven: %v", err)
	}
	t.Cleanup(engine.Close)

	provisioned, err := cluster.Provision(
		t.Context(), engine, cluster.DefaultKubernetesVersion(), theRefusedSettingScenario)
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

// Edge case 8: a value the control plane refuses ends the Scenario at the Step
// that deployed its Workload, naming that Step and what was refused. The Workload
// reaches the cluster at the first Step needing an observed Placement, so that
// Then is the Step the ending names, and no Then after it is carried out or judged.
// Full access is how a Scenario can write such a value; the rule is every
// Customization's.
func TestFullAccessNamingASettingTheControlPlaneRefuses_ShouldEndTheScenarioAtTheStepThatDeployedTheWorkload(
	t *testing.T,
) {
	ending := aScenarioOnAProvisionedCluster(t)

	ending.Given().Topology(Zones(twoZones), NodesPerZone(twoNodesPerZone))
	ending.Given().Workload(someWorkloadName, Pods(podsOverEveryNode),
		ResilientPlacement().Customize(aFullAccessTheControlPlaneRefuses()))
	ending.Then().AllPodsRunning()
	ending.Then().PodsPerZone(podsPerZoneWhenEven)

	ended := ending.outcome()
	expect.Expect(t, ended.EndedAt).To(expect.Equal(allPodsRunningStep()))
	expect.Expect(t, ended.Steps).To(expect.Equal([]Step{
		topologyStepOf(twoZones, twoNodesPerZone),
		resilientPlacementWorkloadStepOf(podsOverEveryNode),
		allPodsRunningStep(),
	}))
	expect.Expect(t, asJudged(ended.Findings)).To(expect.Equal([]judgedFinding{}))
	causeNames(t, ended, theDeployingStepNames)
	causeNames(t, ended, string(theRefusedRestartPolicy))
}
