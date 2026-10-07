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
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/woditschka/k8s-resilience-lab/internal/cluster"
	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// The Scenarios the Simulated Clusters of this file are provisioned for, so each
// container on the engine says which Scenario it serves.
const (
	theRefusedSettingScenario = "a-refused-setting"
	theWaitingScenario        = "a-wait-for-the-cluster"
	theNothingPendingScenario = "a-wait-with-nothing-pending"
)

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
func aFullAccessTheControlPlaneRefuses() Customization {
	return FullAccess(func(pod *corev1.PodSpec) { pod.RestartPolicy = theRefusedRestartPolicy })
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

// reportedStep is one Step as the tests here compare it: the line it reports,
// whether it stated a time it held for, and the time it was held to. How long a
// Wait held varies from run to run, so what a comparison pins is that the line
// carries a time at all; the time it was held to is the Scenario's own statement
// and is compared outright.
type reportedStep struct {
	Kind   StepKind
	Name   string
	Waited bool
	Within time.Duration
}

// asReported is every Step a Scenario recorded, read that way.
func asReported(steps []Step) []reportedStep {
	reported := make([]reportedStep, 0, len(steps))
	for _, step := range steps {
		reported = append(reported, reportedStep{
			Kind:   step.Kind,
			Name:   step.Name,
			Waited: step.Waited > noWait,
			Within: step.Within,
		})
	}

	return reported
}

// reportedAsWaiting and reportedAtOnce are one expected Step read that way: a
// Wait stating no time of its own, which is held to the lab's own waiting time,
// and a Step that waited for nothing.
func reportedAsWaiting(step Step) reportedStep {
	return reportedStep{Kind: step.Kind, Name: step.Name, Waited: true, Within: scenario.WaitingTime}
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
	forms := map[string]func(*Scenario){
		"with the Given that declared it": func(waiting *Scenario) {
			waiting.Given().Topology(Zones(twoZones), NodesPerZone(twoNodesPerZone)).WaitFor().AllNodesReady()
			waiting.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), ResilientPlacement()).
				WaitFor().AllPodsReady()
		},
		"as a Step of its own": func(waiting *Scenario) {
			waiting.Given().Topology(Zones(twoZones), NodesPerZone(twoNodesPerZone))
			waiting.WaitFor().AllNodesReady()
			waiting.Given().Workload(someWorkloadName, Pods(podsOverEveryNode), ResilientPlacement())
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
	declared := aTopologyOf(t, twoZones, twoNodesPerZone)
	waiting := aScenarioOnAClusterFor(t, theNothingPendingScenario)

	waiting.Given().Topology(Zones(twoZones), NodesPerZone(twoNodesPerZone)).WaitFor().AllNodesReady()
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

// Edge case 8: a value the control plane refuses ends the Scenario at the Wait
// that deployed its Workload, naming that Step and what was refused. A Given
// declares and reaches no cluster, so the Wait below it is the Step that applied
// the declaration and the Step the ending names, with the declaration in its
// cause; no Step after it is carried out or judged. Full access is how a Scenario
// can write such a value; the rule is every Customization's.
func TestFullAccessNamingASettingTheControlPlaneRefuses_ShouldEndTheScenarioAtTheWaitThatDeployedTheWorkload(
	t *testing.T,
) {
	ending := aScenarioOnAClusterFor(t, theRefusedSettingScenario)

	ending.Given().Topology(Zones(twoZones), NodesPerZone(twoNodesPerZone))
	ending.Given().Workload(someWorkloadName, Pods(podsOverEveryNode),
		ResilientPlacement().Customize(aFullAccessTheControlPlaneRefuses()))
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
