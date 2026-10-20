// This test crosses to a real Simulated Cluster, because it judges the order in
// which a panicking Scenario lets go of the cluster, its recording and the
// recording's file: no stand-in has a recording to outlive its file.
package lab

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/cluster"
	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/timeline"
)

const (
	thePanickingScenarioOnACluster = "a-scenario-that-panics-on-a-cluster"
	whatTheClusterScenarioSaid     = "a Step's own code panicked with the cluster recording"
)

func TestAScenarioThatPanicsOnARealCluster_ShouldLeaveNoClusterAndNoSecondPanic(t *testing.T) {
	t.Parallel()
	engine, err := cluster.DetectEngine(t.Context(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("no container engine answers on this machine, so the Simulated Cluster cannot be driven: %v", err)
	}
	t.Cleanup(engine.Close)
	running := NewRunner(engine, cluster.DefaultKubernetesVersion(), timeline.NewFolder(t.TempDir()))
	var provisioned *cluster.SimulatedCluster

	// The Scenario panics at once, so the change is still on its way to the
	// recording when the panic unwinds, which is what a file closed too early
	// would meet.
	carried := carryOutAtOnce([]string{thePanickingScenarioOnACluster}, func(int) (Outcome, error) {
		return running.runNamed(t.Context(), thePanickingScenarioOnACluster, func(steps *Scenario) {
			provisioned = steps.provisioned
			declared := aTopologyOf(t, twoZones, twoNodesPerZone)
			if err := provisioned.ApplyTopology(steps.ctx, declared); err != nil {
				t.Errorf("apply the Topology before the panic: %v", err)
			}
			if err := provisioned.FailNode(steps.ctx, declared.Nodes()[0].Name); err != nil {
				t.Errorf("stop a Node responding before the panic: %v", err)
			}
			panic(whatTheClusterScenarioSaid)
		})
	})

	expect.Expect(t, theScenariosThatWentWrongIn(carried)).To(expect.Equal([]string{thePanickingScenarioOnACluster}))
	expect.Expect(t, strings.Contains(theFailureOf(carried, thePanickingScenarioOnACluster), whatTheClusterScenarioSaid)).
		To(expect.Equal(true))
	if _, err := provisioned.ReadyNodes(t.Context()); err == nil {
		t.Errorf("the panicking Scenario's Simulated Cluster %s still answers, but its container is removed", provisioned.Name())
	}
}
