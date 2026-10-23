package scenario_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// No reading below turns on when the Pods first ran, only on the moments after
// the first When's note.
const (
	someTimeBeforeTheFailure = 4 * time.Second

	theFirstFailure      = 10 * time.Second
	theSecondFailure     = 11 * time.Second
	theFirstEviction     = 11 * time.Second
	theSecondEviction    = 12 * time.Second
	theFirstReplacement  = 15 * time.Second
	theSecondReplacement = 17 * time.Second

	theFirstFailureAsRead   = "+10.000s"
	theSecondFailureAsRead  = "+11.000s"
	theFirstEvictionAsRead  = "+11.000s"
	theSecondEvictionAsRead = "+12.000s"
	// From the second eviction to the first replacement, and from the first
	// eviction to the second replacement.
	theLowestLastedAsRead = "3.000s"
	theFallLastedAsRead   = "6.000s"
	// From the Failure to the first replacement.
	theFloorStoodAsRead = "5.000s"

	someWhenLine = "When 1 Zone fails, zone-1"
)

// Pods of the even spread a Zone or a Node holds, in the Topology's own order.
const (
	theFirstPodOfTheSecondZone  = nodesPerZoneOfTheTopology
	theSecondPodOfTheSecondZone = nodesPerZoneOfTheTopology + 1
	theFirstNode                = 0
	theSecondNode               = 1
)

func aPodNamed(at int) string {
	return fmt.Sprintf("%s-6d4f-%d", someDeploymentName, at)
}

func aReplacementNamed(at int) string {
	return fmt.Sprintf("%s-7e5a-%d", someDeploymentName, at)
}

// Two replicas surviving one Node of nine leave a Floor of one Ready Pod.
const twoReplicas = 2

func aDeploymentOfNineSurviving(t *testing.T, tolerance scenario.FailureTolerance) scenario.Deployment {
	t.Helper()

	return aDeploymentSurviving(t, nineReplicas, tolerance)
}

func aDeploymentSurviving(t *testing.T, replicas int, tolerance scenario.FailureTolerance) scenario.Deployment {
	t.Helper()

	deployment, err := aDeploymentOf(t, replicas).WithFailureTolerance(tolerance, aTopologyOfThreeZonesOfThreeNodes(t))
	if err != nil {
		t.Fatalf("declare a Failure tolerance of %s: %v", tolerance.Reads(), err)
	}

	return deployment
}

func survivingOneZone() scenario.FailureTolerance {
	return scenario.FailureTolerance{Lost: scenario.ZonesLost, Count: oneLost}
}

func survivingOneNode() scenario.FailureTolerance {
	return scenario.FailureTolerance{Lost: scenario.NodesLost, Count: oneLost}
}

func survivingNoNode() scenario.FailureTolerance {
	return scenario.FailureTolerance{Lost: scenario.NodesLost, Count: noLossAtAll}
}

func theNodeNamesOf(topology scenario.Topology) []string {
	nodes := topology.Nodes()
	names := make([]string, 0, len(nodes))
	for _, node := range nodes {
		names = append(names, node.Name)
	}

	return names
}

func onePodRunningPerNodeOf(topology scenario.Topology) []scenario.Entry {
	var running []scenario.Entry
	for at, node := range theNodeNamesOf(topology) {
		running = append(running, aPodRunning(someTimeBeforeTheFailure, aPodNamed(at), node))
	}

	return running
}

func aPodRunning(at time.Duration, pod, node string) scenario.Entry {
	return scenario.Entry{
		Elapsed: at, Change: scenario.ChangeModified, Kind: scenario.PodObject, Name: pod,
		Reason: string(scenario.PodRunning), Detail: node,
	}
}

func aPodPending(at time.Duration, pod string) scenario.Entry {
	return scenario.Entry{
		Elapsed: at, Change: scenario.ChangeAdded, Kind: scenario.PodObject, Name: pod, Reason: string(scenario.PodPending),
	}
}

func aPodEvicted(at time.Duration, pod, node string) scenario.Entry {
	return scenario.Entry{
		Elapsed: at, Change: scenario.ChangeModified, Kind: scenario.PodObject, Name: pod,
		Reason: string(scenario.PodTerminating), Detail: node,
	}
}

// A Pod deleted while Running ends its Timeline with a removal still reading
// Running, so only the removal tells it is gone.
func aPodRemovedWhileRunning(at time.Duration, pod, node string) scenario.Entry {
	return scenario.Entry{
		Elapsed: at, Change: scenario.ChangeRemoved, Kind: scenario.PodObject, Name: pod,
		Reason: string(scenario.PodRunning), Detail: node,
	}
}

func aWhenNoteAt(at time.Duration) scenario.Entry {
	return scenario.Entry{
		Elapsed: at, Change: scenario.ChangeNoted, Kind: scenario.StepObject, Name: scenario.WhenStepNamed,
		Detail: someWhenLine,
	}
}

func theReadingOf(t *testing.T, judged scenario.Deployment, entries []scenario.Entry, stopped ...[]string) scenario.FloorReading {
	t.Helper()

	readings, err := scenario.NewTimeline(entries).Floors([]scenario.Deployment{judged}, []string{judged.Name()}, stopped)
	if err != nil {
		t.Fatalf("read the Floor of %s: %v", judged.Name(), err)
	}
	expect.Expect(t, readings).To(expect.HaveLen[scenario.FloorReading](1))

	return readings[0]
}

func lostOn(pods ...string) string {
	return strings.Join(pods, ", ")
}

func TestAPodOnAStoppedNode_ShouldCountAsLostFromTheFailureThoughStillReportedReady(t *testing.T) {
	topology := aTopologyOfThreeZonesOfThreeNodes(t)
	recorded := append(onePodRunningPerNodeOf(topology), aWhenNoteAt(theFirstFailure))

	reading := theReadingOf(t, aDeploymentOfNineSurviving(t, survivingOneNode()), recorded,
		theNodeNamesOf(topology)[:oneLost])

	expect.Expect(t, reading.Line()).To(expect.Equal(
		"web kept its Floor of 8 Ready Pods through the loss of 1 Node: lowest 8 of 9 at " +
			theFirstFailureAsRead + " for the rest of the Scenario"))
}

func TestAReadingKeepingItsFloor_ShouldStateItsLowestCountWhenAndForHowLongAndNoFall(t *testing.T) {
	topology := aTopologyOfThreeZonesOfThreeNodes(t)
	stopped := theNodesOfTheFirstZonesOf(topology, oneLost)
	recorded := append(onePodRunningPerNodeOf(topology),
		aWhenNoteAt(theFirstFailure),
		aPodPending(theFirstFailure, aReplacementNamed(theFirstNode)),
		aPodRunning(theFirstReplacement, aReplacementNamed(theFirstNode), theNodeNamesOf(topology)[theFirstPodOfTheSecondZone]),
	)

	reading := theReadingOf(t, aDeploymentOfNineSurviving(t, survivingOneZone()), recorded, stopped)

	expect.Expect(t, reading.Line()).To(expect.Equal(
		"web kept its Floor of 6 Ready Pods through the loss of 1 Zone: lowest 6 of 9 at " +
			theFirstFailureAsRead + " for " + theFloorStoodAsRead))
	expect.Expect(t, reading.Kept()).To(expect.Equal(true))
	expect.Expect(t, reading.Fall()).To(expect.Equal(""))
}

func TestADeploymentFallingBelowItsFloorAtAnyMoment_ShouldRecordAFindingNamingHowLowWhenAndForHowLong(t *testing.T) {
	topology := aTopologyOfThreeZonesOfThreeNodes(t)
	nodes := theNodeNamesOf(topology)
	stopped := theNodesOfTheFirstZonesOf(topology, oneLost)
	recorded := append(onePodRunningPerNodeOf(topology),
		aWhenNoteAt(theFirstFailure),
		aPodEvicted(theFirstEviction, aPodNamed(theFirstPodOfTheSecondZone), nodes[theFirstPodOfTheSecondZone]),
		aPodEvicted(theSecondEviction, aPodNamed(theSecondPodOfTheSecondZone), nodes[theSecondPodOfTheSecondZone]),
		aPodRunning(theFirstReplacement, aReplacementNamed(theFirstNode), nodes[theFirstPodOfTheSecondZone]),
		aPodRunning(theSecondReplacement, aReplacementNamed(theSecondNode), nodes[theSecondPodOfTheSecondZone]),
	)

	reading := theReadingOf(t, aDeploymentOfNineSurviving(t, survivingOneZone()), recorded, stopped)

	expect.Expect(t, reading.Fall()).To(expect.Equal(
		"web fell to 4 of 9 Pods Ready, below its Floor of 6 from " + theFirstEvictionAsRead + " for " +
			theFallLastedAsRead + ": 5 Pods lost to the Failure, where 3 is the most it survives losing; " +
			lostOn(
				aPodNamed(0)+" on "+nodes[0], aPodNamed(1)+" on "+nodes[1], aPodNamed(2)+" on "+nodes[2],
				aPodNamed(theFirstPodOfTheSecondZone)+" on "+nodes[theFirstPodOfTheSecondZone],
				aPodNamed(theSecondPodOfTheSecondZone)+" on "+nodes[theSecondPodOfTheSecondZone],
			)))
	expect.Expect(t, reading.Line()).To(expect.Equal(
		"web fell below its Floor of 6 Ready Pods through the loss of 1 Zone: lowest 4 of 9 at " +
			theSecondEvictionAsRead + " for " + theLowestLastedAsRead))
	expect.Expect(t, reading.Kept()).To(expect.Equal(false))
}

// The replacements stay Pending, so the fall never ends before the last entry.
func TestAFallLastingUntilTheThen_ShouldReadForTheRestOfTheScenario(t *testing.T) {
	topology := aTopologyOfThreeZonesOfThreeNodes(t)
	nodes := theNodeNamesOf(topology)
	stopped := theNodesOfTheFirstZonesOf(topology, oneLost)
	recorded := append(onePodRunningPerNodeOf(topology),
		aWhenNoteAt(theFirstFailure),
		aPodPending(theFirstReplacement, aReplacementNamed(theFirstNode)),
		aPodPending(theFirstReplacement, aReplacementNamed(theSecondNode)),
	)

	reading := theReadingOf(t, aDeploymentOfNineSurviving(t, survivingOneNode()), recorded, stopped)

	expect.Expect(t, reading.Fall()).To(expect.Equal(
		"web fell to 6 of 9 Pods Ready, below its Floor of 8 from " + theFirstFailureAsRead +
			" for the rest of the Scenario: 3 Pods lost to the Failure, where 1 is the most it survives losing; " +
			lostOn(aPodNamed(0)+" on "+nodes[0], aPodNamed(1)+" on "+nodes[1], aPodNamed(2)+" on "+nodes[2])))
}

func TestAPodRemovedFromTheCluster_ShouldStayLostFromItsFloor(t *testing.T) {
	topology := aTopologyOfThreeZonesOfThreeNodes(t)
	nodes := theNodeNamesOf(topology)
	recorded := append(onePodRunningPerNodeOf(topology),
		aWhenNoteAt(theFirstFailure),
		aPodRemovedWhileRunning(theFirstEviction, aPodNamed(theFirstPodOfTheSecondZone), nodes[theFirstPodOfTheSecondZone]),
	)

	reading := theReadingOf(t, aDeploymentOfNineSurviving(t, survivingOneNode()), recorded, []string{nodes[theFirstNode]})

	expect.Expect(t, reading.Line()).To(expect.Equal(
		"web fell below its Floor of 8 Ready Pods through the loss of 1 Node: lowest 7 of 9 at " +
			theFirstEvictionAsRead + " for the rest of the Scenario"))
	expect.Expect(t, reading.Fall()).To(expect.Equal(
		"web fell to 7 of 9 Pods Ready, below its Floor of 8 from " + theFirstEvictionAsRead +
			" for the rest of the Scenario: 2 Pods lost to the Failure, where 1 is the most it survives losing; " +
			lostOn(aPodNamed(theFirstNode)+" on "+nodes[theFirstNode],
				aPodNamed(theFirstPodOfTheSecondZone)+" on "+nodes[theFirstPodOfTheSecondZone])))
}

func TestAReadingOfASinglePod_ShouldNameItInTheSingular(t *testing.T) {
	topology := aTopologyOfThreeZonesOfThreeNodes(t)
	nodes := theNodeNamesOf(topology)
	tests := []struct {
		name     string
		judged   scenario.Deployment
		recorded []scenario.Entry
		wantLine string
		wantFall string
	}{
		{
			name:     "a Floor of one Ready Pod",
			judged:   aDeploymentSurviving(t, twoReplicas, survivingOneNode()),
			recorded: append(onePodRunningPerNodeOf(topology)[:twoReplicas], aWhenNoteAt(theFirstFailure)),
			wantLine: "web kept its Floor of 1 Ready Pod through the loss of 1 Node: lowest 1 of 2 at " +
				theFirstFailureAsRead + " for the rest of the Scenario",
			wantFall: "",
		},
		{
			name:     "a fall losing one Pod",
			judged:   aDeploymentOfNineSurviving(t, survivingNoNode()),
			recorded: append(onePodRunningPerNodeOf(topology), aWhenNoteAt(theFirstFailure)),
			wantLine: "web fell below its Floor of 9 Ready Pods through the loss of 0 Nodes: lowest 8 of 9 at " +
				theFirstFailureAsRead + " for the rest of the Scenario",
			wantFall: "web fell to 8 of 9 Pods Ready, below its Floor of 9 from " + theFirstFailureAsRead +
				" for the rest of the Scenario: 1 Pod lost to the Failure, where 0 is the most it survives losing; " +
				lostOn(aPodNamed(theFirstNode)+" on "+nodes[theFirstNode]),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reading := theReadingOf(t, tt.judged, tt.recorded, []string{nodes[theFirstNode]})

			expect.Expect(t, reading.Line()).To(expect.Equal(tt.wantLine))
			expect.Expect(t, reading.Fall()).To(expect.Equal(tt.wantFall))
		})
	}
}

func TestAFallAfterMoreThanOneFailure_ShouldNameThePodsLostToTheFailures(t *testing.T) {
	topology := aTopologyOfThreeZonesOfThreeNodes(t)
	nodes := theNodeNamesOf(topology)
	recorded := append(onePodRunningPerNodeOf(topology), aWhenNoteAt(theFirstFailure), aWhenNoteAt(theSecondFailure))

	reading := theReadingOf(t, aDeploymentOfNineSurviving(t, survivingOneNode()), recorded,
		[]string{nodes[theFirstNode]}, []string{nodes[theSecondNode]})

	expect.Expect(t, reading.Fall()).To(expect.Equal(
		"web fell to 7 of 9 Pods Ready, below its Floor of 8 from " + theSecondFailureAsRead +
			" for the rest of the Scenario: 2 Pods lost to the Failures, where 1 is the most it survives losing; " +
			lostOn(aPodNamed(theFirstNode)+" on "+nodes[theFirstNode], aPodNamed(theSecondNode)+" on "+nodes[theSecondNode])))
}

func TestATimelineHoldingNoWhensNote_ShouldJudgeNoFloor(t *testing.T) {
	topology := aTopologyOfThreeZonesOfThreeNodes(t)
	judged := aDeploymentOfNineSurviving(t, survivingOneZone())

	_, err := scenario.NewTimeline(onePodRunningPerNodeOf(topology)).
		Floors([]scenario.Deployment{judged}, []string{judged.Name()}, nil)

	if err == nil {
		t.Fatal("a Timeline holding no When's note judges no Floor, but a reading was given")
	}
	if !strings.Contains(err.Error(), "no When's note") {
		t.Errorf("the refusal names the missing When's note, but reads: %v", err)
	}
}
