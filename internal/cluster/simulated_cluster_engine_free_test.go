//go:build !integration

package cluster

import (
	"fmt"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

func TestADeclaredNodeHoldingNoRunningProbePod_ShouldBeNamedAsStillUnheld(t *testing.T) {
	const oneZone, twoNodesPerZone = 1, 2
	topology, err := topologyOfTheGeneralNodePool(oneZone, twoNodesPerZone, aCapacity(t))
	if err != nil {
		t.Fatalf("build a Topology of %d Nodes: %v", twoNodesPerZone, err)
	}
	declared := topology.Nodes()
	theOneNodeTheSchedulerHolds := declared[1].Name

	unheld := nodesMissingFrom(declared, []string{theOneNodeTheSchedulerHolds})

	expect.Expect(t, unheld).To(expect.Equal([]string{declared[0].Name}))
}

// What an expired Wait reports is read from the last observation alone, so it
// is asserted here rather than by provoking a Wait that runs its whole budget
// out.
func TestAWaitForEveryPodTheWaitingTimeOutlasts_ShouldNameThePodsThatWereNotReady(t *testing.T) {
	observed := aPlacementWithOnePodNotReadyYet(t)

	reported := podsDidNotBecomeReady(aDeploymentDeclaring(t, ninePods, resilientPlacement(t)), observed)

	expect.Expect(t, reported.Error()).To(expect.Equal(fmt.Sprintf(
		"the Pods of the Deployment %s were not all Ready within %s; it last held %d of %d Pods Ready, and these were not: %s",
		deployedDeploymentName, scenario.WaitingTime, ninePods-onePodNotReadyYet, ninePods, observed.PodsNotReady()[0].Name)))
}

func TestAWaitForEveryNodeTheWaitingTimeOutlasts_ShouldNameTheNodesThatWereNotReady(t *testing.T) {
	topology := anEvenSpreadTopology(t)
	observed := aPlacementShortOfOneReadyNode(t)

	reported := nodesDidNotBecomeReady(topology, observed)

	expect.Expect(t, reported.Error()).To(expect.Equal(fmt.Sprintf(
		"the Nodes of the Topology were not all Ready within %s; these were not: %s",
		scenario.WaitingTime, theFirstNodeOf(topology))))
}

// What an expired settle wait reports is read from the observation alone, and
// no Placement the lab can bring about keeps a wait open, so the report is
// asserted here rather than by running the budget out.
func TestAPlacementThatDoesNotSettleAfterTheFailure_ShouldReportTheObservedPlacement(t *testing.T) {
	observed := aPlacementStillHoldingAPodOnTheFailedNode(t)

	reported := didNotSettle(aDeploymentDeclaring(t, ninePods, resilientPlacement(t)), observed)

	expect.Expect(t, reported.Error()).To(expect.Equal(fmt.Sprintf(
		"the Placement of the Deployment %s did not settle within %s; it last held %d of %d Pods over %d Ready Nodes, %d of them on a Node it does not hold and %d still starting",
		deployedDeploymentName, scenario.WaitingTime, ninePods, ninePods, nodesStillResponding, onePodLeftBehind, noPodStillStarting)))
}

// Each row meets every clause but the one it names, so the row says which
// clause decided it.
func TestAnObservedPlacement_ShouldCountAsSettledOnlyWhenItAccountsForEveryDeclaredPodAndAgreesWithTheOneBeforeIt(t *testing.T) {
	accountedFor := aPlacementOfEveryDeclaredPod(t)
	stillHoldingAPodOnTheFailedNode := aPlacementStillHoldingAPodOnTheFailedNode(t)
	shortOfOnePod := aPlacementShortOfOnePod(t)
	withOnePodStillStarting := aPlacementWithOnePodStillStarting(t)

	observations := map[string]struct {
		previous, current scenario.Placement
		settled           bool
	}{
		"two agreeing observations of every declared Pod on a Node they hold": {
			previous: accountedFor, current: accountedFor, settled: true,
		},
		"an observation still holding a Pod on the Node that stopped responding": {
			previous: stillHoldingAPodOnTheFailedNode, current: stillHoldingAPodOnTheFailedNode, settled: false,
		},
		"an observation holding fewer Pods than the Deployment declares": {
			previous: shortOfOnePod, current: shortOfOnePod, settled: false,
		},
		"two observations that do not agree": {
			previous: shortOfOnePod, current: accountedFor, settled: false,
		},
		"an observation holding a placed Pod still inside its Startup time": {
			previous: withOnePodStillStarting, current: withOnePodStillStarting, settled: false,
		},
	}

	for observation, taken := range observations {
		t.Run(observation, func(t *testing.T) {
			settled := hasSettled(taken.previous, taken.current, aDeploymentDeclaring(t, ninePods, resilientPlacement(t)))

			expect.Expect(t, settled).To(expect.Equal(taken.settled))
		})
	}
}

func aPlacementOfEveryDeclaredPod(t *testing.T) scenario.Placement {
	t.Helper()

	nodes := anEvenSpreadTopology(t).Nodes()

	return scenario.NewPlacement(nodes, onePodOnEachOf(nodes))
}

func aPlacementShortOfOnePod(t *testing.T) scenario.Placement {
	t.Helper()

	nodes := anEvenSpreadTopology(t).Nodes()
	created := onePodOnEachOf(nodes)

	return scenario.NewPlacement(nodes, created[:len(created)-onePodNotYetCreated])
}

func aPlacementWithOnePodNotReadyYet(t *testing.T) scenario.Placement {
	t.Helper()

	nodes := anEvenSpreadTopology(t).Nodes()
	placed := onePodOnEachOf(nodes)
	placed[0].Ready = false

	return scenario.NewPlacement(nodes, placed)
}

func aPlacementWithOnePodStillStarting(t *testing.T) scenario.Placement {
	t.Helper()

	nodes := anEvenSpreadTopology(t).Nodes()
	placed := onePodOnEachOf(nodes)
	placed[0].Phase, placed[0].Ready = scenario.PodPending, false

	return scenario.NewPlacement(nodes, placed)
}

// A Node that is not Ready is in no observed Placement.
func aPlacementShortOfOneReadyNode(t *testing.T) scenario.Placement {
	t.Helper()

	nodes := anEvenSpreadTopology(t).Nodes()
	stillResponding := nodes[1:]

	return scenario.NewPlacement(stillResponding, onePodOnEachOf(stillResponding))
}

// The Pod on the failed Node is the clause that holds the settle wait open.
func aPlacementStillHoldingAPodOnTheFailedNode(t *testing.T) scenario.Placement {
	t.Helper()

	topology := anEvenSpreadTopology(t)
	stopped, stillResponding := theFirstNodeOf(topology), topology.Nodes()[1:]
	leftBehind := scenario.Pod{Name: podLeftOnTheFailedNode, Node: stopped, Phase: scenario.PodRunning}

	return scenario.NewPlacement(stillResponding, append([]scenario.Pod{leftBehind}, onePodOnEachOf(stillResponding)...))
}

// The control plane's own Pod names are not this suite's to pin.
func onePodOnEachOf(nodes []scenario.Node) []scenario.Pod {
	placed := make([]scenario.Pod, 0, len(nodes))
	for at, node := range nodes {
		placed = append(placed, scenario.Pod{
			Name:  fmt.Sprintf("%s-%d", deployedDeploymentName, at),
			Node:  node.Name,
			Phase: scenario.PodRunning,
			Ready: true,
		})
	}

	return placed
}

// A cluster already settled needs a poll and two agreeing observations, so the
// least time a Wait may state has to leave room beyond one poll.
func TestThePollIntervalBetweenObservations_ShouldFitInTheLeastTimeAWaitMayStateWithRoomToSpare(t *testing.T) {
	expect.Expect(t, pollInterval < scenario.LeastStatedTime).To(expect.Equal(true))
}
