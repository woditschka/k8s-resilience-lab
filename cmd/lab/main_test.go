//go:build !unit

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/cluster"
	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/lab"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
	"github.com/woditschka/k8s-resilience-lab/scenarios"
)

// A well-formed version, so it passes validation and fails at the image pull.
const aVersionWithNoClusterImage = "1.0.0"

// A reached Wait names the time it took, which belongs to the run, so these
// lines are read by prefix.
const (
	allNodesReadyAwaited = "Wait for all Nodes Ready, reached in "
	allPodsReadyAwaited  = "Wait for all Pods Ready, reached in "

	// The seconds are the ones the Failure Scenarios state on that line.
	theStatedRestoreTimeAwaited = " of the 30s stated"
)

// A declaration renames the line it is written on, so the renamed Given is the
// one a Failure Scenario reports.
const (
	aFailureToleranceDeclared   = ", surviving the loss of"
	theLossOfOneNodeDeclared    = resilientPlacementDeclared + aFailureToleranceDeclared + " 1 Node"
	theLossOfOneZoneDeclared    = resilientPlacementDeclared + ", surviving the loss of 1 Zone"
	theLossOfThreeNodesDeclared = resilientPlacementDeclared + ", surviving the loss of 3 Nodes"

	enoughPodsStayedReadyJudged = "Then enough Pods stayed Ready from the first Failure"
)

const aSettingCarried = "\n    "

func resilientPlacementCarries() string {
	return theLabsTolerationCarried("node.kubernetes.io/not-ready") +
		theLabsTolerationCarried("node.kubernetes.io/unreachable") +
		aSettingCarried + "topologySpreadConstraints: {topologyKey: topology.kubernetes.io/zone, maxSkew: 1, " +
		"whenUnsatisfiable: DoNotSchedule, nodeTaintsPolicy: Honor}" +
		aSettingCarried + "topologySpreadConstraints: {topologyKey: kubernetes.io/hostname, maxSkew: 1, " +
		"whenUnsatisfiable: DoNotSchedule, nodeTaintsPolicy: Honor}"
}

func theLabsTolerationCarried(taint string) string {
	return aSettingCarried + "tolerations: {key: " + taint + ", operator: Exists, effect: NoExecute, tolerationSeconds: " +
		strconv.FormatInt(int64(scenario.UnresponsiveNodeWait/time.Second), 10) + "}"
}

// A Deployment line's Request reports after the settings its specification
// holds.
const (
	theRequestCarried     = aSettingCarried + "resources.requests: {cpu: 3, memory: 12Gi}"
	theStandardPodCarried = aSettingCarried + "resources.requests: {cpu: 4, memory: 16Gi}"
	theGPUPodCarried      = aSettingCarried + "resources.requests: {cpu: 24, memory: 96Gi}"
)

// theNodePickedToFail is the first Node the Topology derives, the pick a run
// has to repeat on every run.
const (
	nodeStopsRespondingReported = aScenarioHeading + "a-node-that-stops-responding"
	theNodePickedToFail         = "zone-1-node-1"
	oneNodeFailsReported        = "When 1 Node fails, " + theNodePickedToFail
	noPodOnTheFailedNodesJudged = "Then no Pod on the Nodes that failed"

	// One Node holds one of the nine Pods, so the Floor is eight and the
	// Failure reaches it.
	theFloorKeptWhenTheNodeStopped = "checkout kept its Floor of 8 Ready Pods through the loss of 1 Node: lowest 8 of 9 at +"
)

// A lost Zone takes three of the nine Pods, so exactly the minimum is left
// Running at the moment of the Failure.
const (
	zoneStopsRespondingReported = aScenarioHeading + "a-zone-that-stops-responding"
	theZonePickedToFail         = "zone-1"
	oneZoneFailsReported        = "When 1 Zone fails, " + theZonePickedToFail
	noPodInTheFailedZonesJudged = "Then no Pod in the Zones that failed"

	theFloorKeptWhenTheZoneStopped = "checkout kept its Floor of 6 Ready Pods through the loss of 1 Zone: lowest 6 of 9 at +"
)

// Each Pod asks for more than half of what a Node holds, so the six Nodes still
// responding have room for six of the nine Pods, leaving three Pending.
const (
	noRoomAfterAZoneStopsRespondingReported = aScenarioHeading + "no-room-after-a-zone-stops-responding"
	thePlacementSettledAwaited              = "Wait for the Placement Settled, reached in "
	threePodsPendingJudged                  = "Then 3 Pods Pending"

	theFloorKeptWithNoRoomLeft = theFloorKeptWhenTheZoneStopped
)

// The spot Nodes are three of the nine Nodes, so the Failure costs exactly the
// three Pods the Deployment's Floor admits.
const (
	spotNodesStopRespondingReported = aScenarioHeading + "spot-nodes-that-stop-responding"
	theOnDemandAndSpotNodesDeclared = "Given a Topology of 3 Zones of " +
		"a NodePool on-demand of 2 Standard Nodes holding 16 cpu, 64Gi memory and 10 Pods each, " +
		"labelled capacity-type=on-demand; " +
		"a NodePool spot of 1 Standard Node holding 16 cpu, 64Gi memory and 10 Pods each, labelled capacity-type=spot"
	theSpotNodesFailReported = "When Nodes labelled capacity-type=spot fail, " +
		"zone-1-node-3, zone-2-node-3, zone-3-node-3"
	theFloorKeptWhenTheSpotNodesStopped = "checkout kept its Floor of 6 Ready Pods through the loss of 3 Nodes: lowest 6 of 9 at +"
)

// The control plane never moves a running Pod, so the Node or Zone that
// responds again stays empty: the Zone the Node is in keeps its three Pods on
// its other two Nodes, one of them holding two.
const (
	nodeRespondsAgainReported    = aScenarioHeading + "a-node-that-responds-again"
	theNodesRespondAgainReported = "When the stopped Nodes respond again, " + theNodePickedToFail
	theNodeTurningReadyNarrated  = "Node " + theNodePickedToFail + " Ready"

	zoneRespondsAgainReported    = aScenarioHeading + "a-zone-that-responds-again"
	theZonesRespondAgainReported = "When the stopped Zones respond again, " + theZonePickedToFail

	zoneRespondsAgainUnderADeschedulerReported = aScenarioHeading + "a-zone-that-responds-again-under-a-descheduler"

	noPodHeld   = 0
	onePodHeld  = 1
	twoPodsHeld = 2
	// Every Node of the three Zones of three but the one left empty and the one
	// holding two.
	nodesHoldingOne = 3*3 - 2
)

// Every Pod is lost after the stopped Node responded again, so the replacements
// spread over every Node, the one that was empty included.
const (
	podsLostAfterANodeRespondsAgainReported = aScenarioHeading + "pods-lost-after-a-node-responds-again"
	everyPodOfCheckoutLostReported          = "When 9 Pods of checkout fail, "
	onePodOnEveryNodeHeld                   = 3 * 3
	everyPodOfCheckoutLost                  = 9
)

// One Descheduler pass after the stopped Node responded again evicts the one Pod
// past the spread, and its replacement takes the empty Node.
const (
	deschedulerPassAfterANodeRespondsAgainReported = aScenarioHeading + "a-descheduler-pass-after-a-node-responds-again"
	theDeschedulerPassReported                     = "When one Descheduler pass under even-spread-restored"
	evictedByTheDeschedulerNarrated                = " by the Descheduler"
	onePodDescheduled                              = 1
)

// Each Zone holds one on-demand and one spot Node, so six Pods spread evenly
// put two in each Zone and three on each capacity type.
const (
	spreadOverOnDemandAndSpotReported = aScenarioHeading + "a-deployment-spread-over-on-demand-and-spot"
	theLabelledNodePoolsDeclared      = "Given a Topology of 3 Zones of " +
		"a NodePool on-demand of 1 Standard Node holding 16 cpu, 64Gi memory and 10 Pods each, " +
		"labelled capacity-type=on-demand; " +
		"a NodePool spot of 1 Standard Node holding 16 cpu, 64Gi memory and 10 Pods each, labelled capacity-type=spot"
	spreadOverOnDemandAndSpotDeclared = "Given a Deployment checkout of 6 replicas declaring Spread over on-demand and spot, " +
		"each Pod starting in 500ms to 1s"
	everyPodRunningJudged          = "Then every Pod Running"
	twoPodsPerZoneJudged           = "Then 2 Pods per Zone"
	threePodsPerCapacityTypeJudged = "Then 3 Pods per value of capacity-type"
)

func spreadOverOnDemandAndSpotCarries() string {
	return theLabsTolerationCarried("node.kubernetes.io/not-ready") +
		theLabsTolerationCarried("node.kubernetes.io/unreachable") +
		aSettingCarried + "topologySpreadConstraints: {topologyKey: topology.kubernetes.io/zone, maxSkew: 1, " +
		"whenUnsatisfiable: DoNotSchedule, nodeTaintsPolicy: Honor}" +
		aSettingCarried + "topologySpreadConstraints: {topologyKey: capacity-type, maxSkew: 1, " +
		"whenUnsatisfiable: DoNotSchedule, nodeTaintsPolicy: Honor}"
}

// Six general Pods fill the six general Nodes, and three tolerating Pods, each
// asking for more than a general Node holds, take the three gpu Nodes.
const (
	generalPodsStayOffGpuNodesReported = aScenarioHeading + "general-pods-beside-tainted-gpu-nodes"
	theTaintedGpuNodePoolDeclared      = "Given a Topology of 3 Zones of " +
		"a NodePool general of 2 Standard Nodes holding 16 cpu, 64Gi memory and 10 Pods each; " +
		"a NodePool gpu of 1 GPU Node holding 32 cpu, 128Gi memory and 10 Pods each, " +
		"tainted nvidia.com/gpu=present:NoSchedule"
	sixGeneralPodsDeclared = "Given a Deployment checkout of 6 replicas declaring Resilient placement, " +
		"each Pod starting in 500ms to 1s"
	toleratingGpuNodesDeclared = "Given a Deployment inference of 3 replicas declaring Tolerating GPU Nodes, " +
		"each Pod starting in 500ms to 1s"
	sixPodsOnTheGeneralNodePoolJudged = "Then 6 Pods on the NodePool general"
	noPodOnTheGpuNodePoolJudged       = "Then 0 Pods on the NodePool gpu"
	threePodsOnTheGpuNodePoolJudged   = "Then 3 Pods on the NodePool gpu"
	noPodOnTheGeneralNodePoolJudged   = "Then 0 Pods on the NodePool general"
)

func toleratingGpuNodesCarries() string {
	return theLabsTolerationCarried("node.kubernetes.io/not-ready") +
		theLabsTolerationCarried("node.kubernetes.io/unreachable") +
		aSettingCarried + "tolerations: {key: nvidia.com/gpu, operator: Equal, value: present, effect: NoSchedule}"
}

const theNodeCapacityDeclared = "Given a Topology of 3 Zones of a NodePool general of 3 Nodes holding " +
	"4 cpu, 16Gi memory and 10 Pods each"

// aStandardNodeSetting is the run-start line the lab reported Standard on while
// it defined Standard itself.
const (
	theStandardNodeDeclared = "Given a Topology of 3 Zones of a NodePool general of 3 Standard Nodes holding " +
		"16 cpu, 64Gi memory and 10 Pods each"
	aStandardNodeSetting = "Standard Node:"
)

// A line is checked for the value and the default the declaring package holds
// rather than for a wording this suite repeats.
const (
	unresponsiveNodeSetting = "unresponsive Node:"
	nodeHeartbeatSetting    = "Node heartbeats:"
	nodeEvictionSetting     = "Node eviction rate:"

	labWaitingTime = "lab waiting time:"

	// A run no longer opens a line with this label: every Pod specification
	// names its own Startup time.
	aPodStartupTimeSetting = "Pod startup time:"
)

const (
	wholeTimelineAsked = "--whole-timeline"
	noEntryPrinted     = 0
)

const theDeploymentOfEveryScenario = "checkout-"

const noPodShared = 0

// requireEngine fails only the calling test where no engine answers, so every
// test that needs no engine still runs.
func requireEngine(t *testing.T) {
	t.Helper()

	if _, err := cluster.DetectEngine(t.Context(), slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("no container engine answers on this machine, so the lab cannot run: %v", err)
	}
}

// TestMain owns this folder, because the shared run outlives any single test.
var sharedRunTimelines string

func TestMain(m *testing.M) {
	folder, err := os.MkdirTemp("", "lab-shared-run-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "make the folder the shared run files its Timelines under: %v\n", err)
		os.Exit(1)
	}
	sharedRunTimelines = folder

	passed := m.Run()

	if err := os.RemoveAll(folder); err != nil {
		fmt.Fprintf(os.Stderr, "remove the folder the shared run filed its Timelines under: %v\n", err)
	}
	os.Exit(passed)
}

// aRunOfEveryScenario is shared so the suite provisions every Scenario's
// cluster once rather than once per test. Sharing is safe because the result is
// immutable. It runs under its own context, because it outlives the test that
// starts it.
var aRunOfEveryScenario = sync.OnceValues(func() (string, error) {
	var reported, warned bytes.Buffer
	err := run(context.Background(), nil, sharedRunTimelines, &reported, &warned)

	return reported.String(), err
})

func theRunOfEveryScenario(t *testing.T) string {
	t.Helper()

	requireEngine(t)
	reported, err := aRunOfEveryScenario()
	if err != nil {
		t.Fatalf("a Scenario of the lab reported a Finding, ended at a Step or could not be carried out: %v", err)
	}

	return reported
}

func theScenarioNamedIn(section string) string {
	named, _, _ := strings.Cut(section, ",")

	return named
}

// reportLineNames reads a setting by its values rather than as a whole line,
// because the run owes a reader the values and not the wording around them.
func reportLineNames(t *testing.T, reported, label string, named ...string) {
	t.Helper()

	line := theLineOpeningWith(reported, label)
	if line == "" {
		t.Fatalf("the run reports a line opening with %q, but reported:\n%s", label, reported)
	}

	for _, value := range named {
		if !strings.Contains(line, value) {
			t.Errorf("the line opening with %q names %q, but reads: %s", label, value, line)
		}
	}
}

func theLineOpeningWith(reported, label string) string {
	for line := range strings.SplitSeq(reported, "\n") {
		if strings.HasPrefix(line, label) {
			return line
		}
	}

	return ""
}

func TestADeploymentDeclaringResilientPlacement_ShouldReportBothSpreadsInFullBeneathItsGivenLine(t *testing.T) {
	t.Parallel()
	reported := theRunOfEveryScenario(t)

	reportShows(t, reported, resilientPlacementDeclared+resilientPlacementCarries())
}

func TestADeploymentNamingStandardPod_ShouldReportTheRequestBeneathItsGivenLine(t *testing.T) {
	t.Parallel()
	reported := theRunOfEveryScenario(t)

	reportShows(t, reported, resilientPlacementDeclared+resilientPlacementCarries()+theStandardPodCarried)
}

// A Scenario has no test of its own, so the command's suite is what fails when
// one reports a Finding or ends at a Step.
func TestResilientPlacementWrittenAsSteps_ShouldPlaceThreePodsPerZoneAndOnePerNode(t *testing.T) {
	t.Parallel()
	reported := theRunOfEveryScenario(t)

	reportShows(t, reported, resilientPlacementReported)
	reportShows(t, reported, allNodesReadyAwaited)
	reportShows(t, reported, allPodsReadyAwaited)
	reportShows(t, reported, threePodsPerZoneJudged)
	reportShows(t, reported, onePodPerNodeJudged)
	reportShows(t, reported, theFirstZoneOnABoard)
	reportShows(t, reported, aSkewReported)
	reportLacks(t, reported, aFindingReported)
}

func TestANodeThatStopsResponding_ShouldSettleAtThreePodsPerZoneWithNoneOnTheFailedNode(t *testing.T) {
	t.Parallel()
	reported := theRunOfEveryScenario(t)

	reportShows(t, reported, nodeStopsRespondingReported)
	reportShows(t, reported, theLossOfOneNodeDeclared+resilientPlacementCarries()+theStandardPodCarried)
	reportShows(t, reported, allNodesReadyAwaited)
	reportShows(t, reported, allPodsReadyAwaited)
	reportShows(t, reported, oneNodeFailsReported)
	reportShows(t, reported, theStatedRestoreTimeAwaited)
	reportShows(t, reported, noPodOnTheFailedNodesJudged)
	reportShows(t, reported, threePodsPerZoneJudged)
	reportLacks(t, reported, aFindingReported)
}

// Pods per Zone are not read after the Failure: two Zones carry nine Pods, so
// the Placement is skewed and the Scenario judges its Failure tolerance alone.
func TestARunOfAZoneThatStopsResponding_ShouldReportTheZoneItPickedToFail(t *testing.T) {
	t.Parallel()
	reported := theRunOfEveryScenario(t)

	reportShows(t, reported, zoneStopsRespondingReported)
	reportShows(t, reported, theLossOfOneZoneDeclared+resilientPlacementCarries()+theStandardPodCarried)
	reportShows(t, reported, allNodesReadyAwaited)
	reportShows(t, reported, allPodsReadyAwaited)
	reportShows(t, reported, oneZoneFailsReported)
	reportShows(t, reported, theStatedRestoreTimeAwaited)
	reportShows(t, reported, noPodInTheFailedZonesJudged)
	reportLacks(t, reported, aFindingReported)
}

func TestARunOfNoRoomAfterAZoneStopsResponding_ShouldSettleAtSixPodsRunningAndThreePending(t *testing.T) {
	t.Parallel()
	reported := theRunOfEveryScenario(t)

	reportShows(t, reported, noRoomAfterAZoneStopsRespondingReported)
	reportShows(t, reported, theNodeCapacityDeclared)
	reportShows(t, reported, theLossOfOneZoneDeclared+resilientPlacementCarries()+theRequestCarried)
	reportShows(t, reported, onePodPerNodeJudged)
	reportShows(t, reported, oneZoneFailsReported)
	reportShows(t, reported, thePlacementSettledAwaited)
	reportShows(t, reported, theStatedRestoreTimeAwaited)
	reportShows(t, reported, noPodInTheFailedZonesJudged)
	reportShows(t, reported, threePodsPendingJudged)
	reportLacks(t, reported, aFindingReported)
}

func TestSpotNodesThatStopResponding_ShouldSurviveLosingThreePodsAndRestoreNinePodsOnTheOnDemandNodes(t *testing.T) {
	t.Parallel()
	reported := theScenarioSectionHeaded(t, theRunOfEveryScenario(t), spotNodesStopRespondingReported)

	reportShows(t, reported, theOnDemandAndSpotNodesDeclared)
	reportShows(t, reported, theLossOfThreeNodesDeclared+resilientPlacementCarries()+theStandardPodCarried)
	reportShows(t, reported, onePodPerNodeJudged)
	reportShows(t, reported, theSpotNodesFailReported)
	reportShows(t, reported, theStatedRestoreTimeAwaited)
	reportShows(t, reported, threePodsPerZoneJudged)
	reportShows(t, reported, noPodOnTheFailedNodesJudged)
	reportLacks(t, reported, aFindingReported)
}

// Each Failure Scenario's own test asserts its Placement outcome; this one
// reads every Floor judged beneath its Then.
func TestEveryFailureScenario_ShouldDeclareWhatItsWhenFailsAndKeepEveryFloor(t *testing.T) {
	t.Parallel()
	reported := theRunOfEveryScenario(t)
	cases := map[string]struct {
		kept string
	}{
		nodeStopsRespondingReported:                    {kept: theFloorKeptWhenTheNodeStopped},
		zoneStopsRespondingReported:                    {kept: theFloorKeptWhenTheZoneStopped},
		noRoomAfterAZoneStopsRespondingReported:        {kept: theFloorKeptWithNoRoomLeft},
		spotNodesStopRespondingReported:                {kept: theFloorKeptWhenTheSpotNodesStopped},
		nodeRespondsAgainReported:                      {kept: theFloorKeptWhenTheNodeStopped},
		zoneRespondsAgainReported:                      {kept: theFloorKeptWhenTheZoneStopped},
		deschedulerPassAfterANodeRespondsAgainReported: {kept: theFloorKeptWhenTheNodeStopped},
	}

	for heading, one := range cases {
		t.Run(heading, func(t *testing.T) {
			section := theScenarioSectionHeaded(t, reported, heading)

			reportShows(t, section, enoughPodsStayedReadyJudged+aSettingCarried+one.kept)
			reportLacks(t, section, aFindingReported)
		})
	}
}

func TestANodeThatRespondsAgain_ShouldHoldNoPodWhileEveryZoneKeepsThreeAndOneNodeHoldsTwo(t *testing.T) {
	t.Parallel()
	reported := theScenarioSectionHeaded(t, theRunOfEveryScenario(t), nodeRespondsAgainReported)

	reportShows(t, reported, oneNodeFailsReported)
	reportShows(t, reported, theNodesRespondAgainReported)
	reportShows(t, reported, noPodOnTheFailedNodesJudged)
	reportShows(t, reported, threePodsPerZoneJudged)
	reportLacks(t, reported, aFindingReported)
	expect.Expect(t, thePodsPerNodeOnTheLastBoardOf(reported)).To(expect.Equal(slices.Concat(
		[]int{noPodHeld}, slices.Repeat([]int{onePodHeld}, nodesHoldingOne), []int{twoPodsHeld})))
}

func TestAZoneThatRespondsAgain_ShouldHoldNoPodWhileTheNinePodsStayRunningOnTheOtherZones(t *testing.T) {
	t.Parallel()
	reported := theScenarioSectionHeaded(t, theRunOfEveryScenario(t), zoneRespondsAgainReported)

	reportShows(t, reported, oneZoneFailsReported)
	reportShows(t, reported, theZonesRespondAgainReported)
	reportShows(t, reported, noPodInTheFailedZonesJudged)
	reportShows(t, reported, everyPodRunningJudged)
	reportLacks(t, reported, aFindingReported)
}

func TestPodsLostAfterANodeRespondsAgain_ShouldDeclareNoFailureToleranceJudgeNoFloorAndPlaceOnePodOnEveryNodeWithNinePodsRunningAndThreePerZone(t *testing.T) {
	t.Parallel()
	reported := theScenarioSectionHeaded(t, theRunOfEveryScenario(t), podsLostAfterANodeRespondsAgainReported)

	reportShows(t, reported, everyPodOfCheckoutLostReported)
	reportLacks(t, reported, enoughPodsStayedReadyJudged)
	reportLacks(t, reported, aFailureToleranceDeclared)
	reportShows(t, reported, everyPodRunningJudged)
	reportShows(t, reported, threePodsPerZoneJudged)
	reportLacks(t, reported, aFindingReported)
	expect.Expect(t, thePodsPerNodeOnTheLastBoardOf(reported)).To(expect.Equal(slices.Repeat([]int{onePodHeld}, onePodOnEveryNodeHeld)))
}

func TestPodsLostToAWhen_ShouldBeReplacedAndNamedWithTheirNodeInTheNextWaitsNarrative(t *testing.T) {
	t.Parallel()
	reported := theScenarioSectionHeaded(t, theRunOfEveryScenario(t), podsLostAfterANodeRespondsAgainReported)

	next := theNextWaitAfter(reported, everyPodOfCheckoutLostReported)
	replacements := theSchedulingOfEachPodOnTheLastBoardOf(reported)

	expect.Expect(t, len(replacements)).To(expect.Equal(everyPodOfCheckoutLost))
	expect.Expect(t, theSchedulingNarratedIn(next, replacements)).To(expect.Equal(replacements))
}

func TestAWhenRunningADeschedulerPassesLine_ShouldNameTheDeclaredPolicy(t *testing.T) {
	t.Parallel()
	reported := theScenarioSectionHeaded(t, theRunOfEveryScenario(t), deschedulerPassAfterANodeRespondsAgainReported)

	reportShows(t, reported, "\n  "+theDeschedulerPassReported+"\n")
}

func TestADeschedulerPassAfterANodeRespondsAgain_ShouldEvictOnePodAndPlaceOnePodOnEveryNodeWithNinePodsRunningAndThreePerZone(t *testing.T) {
	t.Parallel()
	reported := theScenarioSectionHeaded(t, theRunOfEveryScenario(t), deschedulerPassAfterANodeRespondsAgainReported)

	next := theNextWaitAfter(reported, theDeschedulerPassReported)

	expect.Expect(t, strings.Count(next, evictedByTheDeschedulerNarrated)).To(expect.Equal(onePodDescheduled))
	reportShows(t, reported, everyPodRunningJudged)
	reportShows(t, reported, threePodsPerZoneJudged)
	reportLacks(t, reported, aFindingReported)
	expect.Expect(t, thePodsPerNodeOnTheLastBoardOf(reported)).To(expect.Equal(slices.Repeat([]int{onePodHeld}, onePodOnEveryNodeHeld)))
}

func TestNodesThatRespondAgain_ShouldBeNamedTurningReadyInTheNextWaitNarrative(t *testing.T) {
	t.Parallel()
	reported := theScenarioSectionHeaded(t, theRunOfEveryScenario(t), nodeRespondsAgainReported)

	reportShows(t, theNextWaitAfter(reported, theNodesRespondAgainReported), theNodeTurningReadyNarrated)
}

// theNextWaitAfter reads the Wait line and its narrative, which stand above the
// separator opening its board.
func theNextWaitAfter(section, line string) string {
	_, after, _ := strings.Cut(section, line)
	wait, _, _ := strings.Cut(after, "\n  "+separatorRune)

	return wait
}

// The board the section's last Wait drew is the one the Thens judged; its Node
// lines alone stand two levels in, opening on the Node's name, which begins
// with its Zone's, and its NodePool before its Pods. The counts are sorted, since which Node of the
// Zone takes the second Pod is the scheduler's pick.
func thePodsPerNodeOnTheLastBoardOf(section string) []int {
	var counts []int
	for _, line := range theNodeLinesOfTheLastBoardOf(section) {
		counts = append(counts, len(strings.Fields(line))-columnsBeforeThePods)
	}
	slices.Sort(counts)

	return counts
}

func theNodeLinesOfTheLastBoardOf(section string) []string {
	board := section[strings.LastIndex(section, "\n  "+waitOpens):]

	var lines []string
	for line := range strings.SplitSeq(board, "\n") {
		if strings.HasPrefix(line, aNodeLineOnABoard) {
			lines = append(lines, line)
		}
	}

	return lines
}

// The replacements are the Pods standing once the Placement Settled, so the
// narrative's scheduling of each reads as it would print, sorted, since the
// order the lost Pods are replaced in is the control plane's.
func theSchedulingOfEachPodOnTheLastBoardOf(section string) []string {
	var scheduled []string
	for _, line := range theNodeLinesOfTheLastBoardOf(section) {
		fields := strings.Fields(line)
		for _, pod := range fields[columnsBeforeThePods:] {
			scheduled = append(scheduled, aPodScheduled(strings.TrimSuffix(pod, ","), fields[0]))
		}
	}
	slices.Sort(scheduled)

	return scheduled
}

// A narrative line reads its moment, then what happened, then when the Pod
// turned Running; only what happened names the Pod and its Node. A Pod
// scheduled that stands on no board after it is no replacement and is left
// out.
func theSchedulingNarratedIn(narrative string, replacements []string) []string {
	scheduled := []string{}
	for line := range strings.SplitSeq(narrative, "\n") {
		_, happened, _ := strings.Cut(strings.TrimSpace(line), "  ")
		happened, _, _ = strings.Cut(happened, ", Running at ")
		if slices.Contains(replacements, happened) {
			scheduled = append(scheduled, happened)
		}
	}
	slices.Sort(scheduled)

	return scheduled
}

func aPodScheduled(pod, node string) string {
	return "Pod " + pod + " scheduled on " + node
}

const (
	waitOpens            = "Wait for "
	aNodeLineOnABoard    = "    zone-"
	columnsBeforeThePods = 2
)

func TestSpreadOverOnDemandAndSpot_ShouldPlaceTwoPodsPerZoneAndThreePerLabelValue(t *testing.T) {
	t.Parallel()
	reported := theScenarioSectionHeaded(t, theRunOfEveryScenario(t), spreadOverOnDemandAndSpotReported)

	reportShows(t, reported, theLabelledNodePoolsDeclared)
	reportShows(t, reported, spreadOverOnDemandAndSpotDeclared+spreadOverOnDemandAndSpotCarries()+theStandardPodCarried)
	reportShows(t, reported, everyPodRunningJudged)
	reportShows(t, reported, twoPodsPerZoneJudged)
	reportShows(t, reported, threePodsPerCapacityTypeJudged)
	reportLacks(t, reported, aFindingReported)
}

func TestGeneralPodsStayOffGpuNodes_ShouldPlaceTheGeneralPodsOnGeneralNodesAndTheTolerantPodsOnGpuNodes(t *testing.T) {
	t.Parallel()
	reported := theScenarioSectionHeaded(t, theRunOfEveryScenario(t), generalPodsStayOffGpuNodesReported)

	reportShows(t, reported, theTaintedGpuNodePoolDeclared)
	reportShows(t, reported, sixGeneralPodsDeclared+resilientPlacementCarries()+theStandardPodCarried)
	reportShows(t, reported, sixPodsOnTheGeneralNodePoolJudged)
	reportShows(t, reported, noPodOnTheGpuNodePoolJudged)
	reportShows(t, reported, toleratingGpuNodesDeclared+toleratingGpuNodesCarries()+theGPUPodCarried)
	reportShows(t, reported, threePodsOnTheGpuNodePoolJudged)
	reportShows(t, reported, noPodOnTheGeneralNodePoolJudged)
	reportLacks(t, reported, aFindingReported)
}

func theScenarioSectionHeaded(t *testing.T, reported, heading string) string {
	t.Helper()

	for _, section := range theScenarioSectionsOf(t, reported) {
		if aScenarioHeading+theScenarioNamedIn(section) == heading {
			return section
		}
	}
	t.Fatalf("the run reports a section headed %q, but reported:\n%s", heading, reported)

	return ""
}

func TestARun_ShouldReportTheSettingsItMakesAgainstTheirDefaultsAndItsOwnWaitingTime(t *testing.T) {
	t.Parallel()
	reported := theRunOfEveryScenario(t)

	reportLineNames(t, reported, unresponsiveNodeSetting,
		scenario.UnresponsiveNodeWait.String(), scenario.DefaultUnresponsiveNodeWait.String())
	reportLineNames(t, reported, nodeHeartbeatSetting,
		cluster.NodeHeartbeatFactor, cluster.DefaultNodeHeartbeatFactor)
	reportLineNames(t, reported, nodeEvictionSetting,
		cluster.NodeEvictionRate, cluster.DefaultNodeEvictionRate)
	reportLineNames(t, reported, labWaitingTime, scenario.WaitingTime.String())
}

func TestARunsStart_ShouldReportNoStartupTimeOfTheLabsOwn(t *testing.T) {
	t.Parallel()
	reported := theRunOfEveryScenario(t)

	reportLacks(t, reported, aPodStartupTimeSetting)
}

func TestARunsStart_ShouldReportNoNodeCapacityOfTheLabsOwn(t *testing.T) {
	t.Parallel()
	reported := theRunOfEveryScenario(t)

	reportLacks(t, reported, aStandardNodeSetting)
}

func TestARunOfANodePoolNamingStandard_ShouldReportWhatStandardStandsForOnItsGivenLine(t *testing.T) {
	t.Parallel()
	reported := theRunOfEveryScenario(t)

	reportShows(t, reported, theStandardNodeDeclared)
}

func TestARunAskedForTheWholeTimeline_ShouldPrintEveryEntryInTheOrderItHappened(t *testing.T) {
	t.Parallel()
	requireEngine(t)

	reported, err := aRun(t, wholeTimelineAsked)
	if err != nil {
		t.Fatalf("a Scenario of the lab reported a Finding or ended at a Step: %v", err)
	}

	for _, section := range theScenarioSectionsOf(t, reported) {
		t.Run(theScenarioNamedIn(section), func(t *testing.T) {
			printed := theEntriesPrintedIn(t, section)
			expect.Expect(t, printed).To(expect.HaveLen[float64](theEntryCountReportedIn(t, section)))
			expect.Expect(t, slices.IsSorted(printed)).To(expect.Equal(true))
		})
	}
}

// The Steps stand among the filed entries, so the file reads as the report's
// Steps did.
func TestATimeline_ShouldFileEveryEntryAndEachStepAtItsMomentWhileTheReportPrintsNone(t *testing.T) {
	t.Parallel()
	reported := theRunOfEveryScenario(t)

	filed := theTimelineFiledBy(t, reported)
	expect.Expect(t, theEntriesPrintedIn(t, reported)).To(expect.HaveLen[float64](noEntryPrinted))
	expect.Expect(t, filed[1:]).To(expect.HaveLen[string](theEntryCountReportedIn(t, reported)))
	reportShows(t, strings.Join(filed, "\n"), aStepFiled)
}

func TestARemovedSimulatedCluster_ShouldLeaveItsTimelineFileReadable(t *testing.T) {
	t.Parallel()

	// Every cluster is removed before the run returns, so the file below
	// outlives the cluster it describes.
	reported := theRunOfEveryScenario(t)

	filed := theTimelineFiledBy(t, reported)
	reportShows(t, filed[0], theScenarioRecorded)
	expect.Expect(t, filed[1:]).To(expect.HaveLen[string](theEntryCountReportedIn(t, reported)))
}

func TestARunNamingNoVersion_ShouldReportThePinnedDefaultVersion(t *testing.T) {
	t.Parallel()
	pinned := cluster.DefaultKubernetesVersion().String()

	reported := theRunOfEveryScenario(t)

	reportShows(t, reported, versionReported+pinned)
}

func TestARunNamingAVersion_ShouldMirrorAndReportThatVersion(t *testing.T) {
	t.Parallel()
	requireEngine(t)
	named := cluster.DefaultKubernetesVersion().String()
	// The version is named without its leading v, so the report shows the
	// validated version rather than the string given.
	asTheRunNamesIt := strings.TrimPrefix(named, "v")

	reported, err := aRun(t, "--k8s-version", asTheRunNamesIt)
	if err != nil {
		t.Fatalf("run the lab naming the version %s: %v", asTheRunNamesIt, err)
	}

	reportShows(t, reported, versionReported+named)
	reportShows(t, reported, aSkewReported)
}

func TestAVersionTheLabCannotObtain_ShouldStopTheRunBeforeAnyScenario(t *testing.T) {
	t.Parallel()
	requireEngine(t)

	reported, err := aRun(t, "--k8s-version", aVersionWithNoClusterImage)

	if err == nil {
		t.Fatalf("a version the lab cannot obtain stops the run, but it reported:\n%s", reported)
	}
	if !strings.Contains(err.Error(), aVersionWithNoClusterImage) {
		t.Errorf("the run names the version it could not obtain, but reads: %v", err)
	}
	reportLacks(t, reported, "\n"+aScenarioHeading)
}

func TestARunOfEveryScenario_ShouldReportEachScenariosLinesTogetherInDeclaredOrder(t *testing.T) {
	t.Parallel()
	declared := []string{
		resilientPlacementReported, nodeStopsRespondingReported,
		zoneStopsRespondingReported, noRoomAfterAZoneStopsRespondingReported,
		spreadOverOnDemandAndSpotReported, spotNodesStopRespondingReported,
		generalPodsStayOffGpuNodesReported, nodeRespondsAgainReported, zoneRespondsAgainReported,
		zoneRespondsAgainUnderADeschedulerReported,
		podsLostAfterANodeRespondsAgainReported, deschedulerPassAfterANodeRespondsAgainReported,
	}

	reported := theRunOfEveryScenario(t)

	expect.Expect(t, theHeadingsOf(t, reported)).To(expect.Equal(declared))
}

// Node names are derived from the Topology, so every cluster names them alike;
// Pod names are each control plane's own, which makes them what shows whether
// one Scenario's record reached into another's cluster.
func TestScenariosCarriedOutAtOnce_ShouldEachNameOnlyTheirOwnNodesAndPods(t *testing.T) {
	t.Parallel()

	reported := theRunOfEveryScenario(t)

	expect.Expect(t, theClustersFiledPerScenario(t, reported)).To(expect.Equal(theClustersReportedPerScenario(t, reported)))
	expect.Expect(t, thePodsFiledByMoreThanOneScenario(t, reported)).To(expect.HaveLen[string](noPodShared))
}

var aRunNamingOneScenario = sync.OnceValues(func() (string, error) {
	var reported, warned bytes.Buffer
	err := run(context.Background(), []string{aSecondScenarioOfTheLab}, sharedRunTimelines, &reported, &warned)

	return reported.String(), err
})

func TestARunNamingOneScenario_ShouldCarryOutOnlyThatScenarioToItsEnd(t *testing.T) {
	t.Parallel()
	requireEngine(t)

	reported, err := aRunNamingOneScenario()
	if err != nil {
		t.Fatalf("the run naming %s: %v", aSecondScenarioOfTheLab, err)
	}

	expect.Expect(t, theHeadingsOf(t, reported)).To(expect.Equal([]string{nodeStopsRespondingReported}))
}

func theHeadingsOf(t *testing.T, reported string) []string {
	t.Helper()

	var headings []string
	for _, section := range theScenarioSectionsOf(t, reported) {
		headings = append(headings, aScenarioHeading+theScenarioNamedIn(section))
	}

	return headings
}

func theClustersReportedPerScenario(t *testing.T, reported string) map[string]string {
	t.Helper()

	clusters := map[string]string{}
	for _, section := range theScenarioSectionsOf(t, reported) {
		heading, _, _ := strings.Cut(section, "\n")
		_, cluster, _ := strings.Cut(heading, onACluster)
		clusters[theScenarioNamedIn(section)] = cluster
	}

	return clusters
}

func theClustersFiledPerScenario(t *testing.T, reported string) map[string]string {
	t.Helper()

	clusters := map[string]string{}
	for _, section := range theScenarioSectionsOf(t, reported) {
		var header struct {
			Cluster string `json:"cluster"`
		}
		filed := theTimelineFiledBy(t, section)
		if err := json.Unmarshal([]byte(filed[0]), &header); err != nil {
			t.Fatalf("read the header of the Timeline of %s: %v", theScenarioNamedIn(section), err)
		}
		clusters[theScenarioNamedIn(section)] = header.Cluster
	}

	return clusters
}

func thePodsFiledByMoreThanOneScenario(t *testing.T, reported string) []string {
	t.Helper()

	filedBy := map[string]map[string]bool{}
	for _, section := range theScenarioSectionsOf(t, reported) {
		for _, line := range theTimelineFiledBy(t, section)[1:] {
			var entry struct {
				Kind string `json:"kind"`
				Name string `json:"name"`
			}
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				t.Fatalf("read an entry of the Timeline of %s: %v", theScenarioNamedIn(section), err)
			}
			if entry.Kind != string(scenario.PodObject) || !strings.HasPrefix(entry.Name, theDeploymentOfEveryScenario) {
				continue
			}
			if filedBy[entry.Name] == nil {
				filedBy[entry.Name] = map[string]bool{}
			}
			filedBy[entry.Name][theScenarioNamedIn(section)] = true
		}
	}

	var shared []string
	for pod, scenarios := range filedBy {
		if len(scenarios) > 1 {
			shared = append(shared, pod)
		}
	}

	return shared
}

const theScenarioRecorded = `"scenario":"` + theScenarioReported + `"`

const aStepFiled = `"kind":"Step"`

// An entry prints under the filing line, one level deeper than a narrative's
// lines, which also open with a time.
const anEntryIndent = "    "

func theEntriesPrintedIn(t *testing.T, reported string) []float64 {
	t.Helper()

	var received []float64
	for line := range strings.SplitSeq(reported, "\n") {
		if !strings.HasPrefix(line, anEntryIndent+"+") {
			continue
		}
		printed := strings.TrimSpace(line)

		elapsed, named := strings.CutSuffix(strings.Fields(printed)[0], "s")
		if !named {
			t.Fatalf("every printed entry opens with the time it was received, but one reads: %s", printed)
		}
		seconds, err := strconv.ParseFloat(strings.TrimPrefix(elapsed, "+"), 64)
		if err != nil {
			t.Fatalf("read the time %q the entry %s was received at: %v", elapsed, printed, err)
		}
		received = append(received, seconds)
	}

	return received
}

func theEntryCountReportedIn(t *testing.T, reported string) int {
	t.Helper()

	_, counted, named := strings.Cut(reported, timelineReported)
	if !named {
		t.Fatalf("the run reports how many entries its Timeline holds, but reported:\n%s", reported)
	}

	count, _, _ := strings.Cut(counted, entriesReported)
	held, err := strconv.Atoi(count)
	if err != nil {
		t.Fatalf("read the entry count %q the run reported: %v", count, err)
	}

	return held
}

// Reading the file back asserts the record outlives the Simulated Cluster it
// describes.
func theTimelineFiledBy(t *testing.T, reported string) []string {
	t.Helper()

	_, filing, named := strings.Cut(reported, filedAt)
	if !named {
		t.Fatalf("the run names the file it filed its Timeline at, but reported:\n%s", reported)
	}

	path, _, _ := strings.Cut(filing, "\n")
	filed, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the filed Timeline %s: %v", path, err)
	}

	return strings.Split(strings.TrimSuffix(string(filed), "\n"), "\n")
}

// aTerminalStoppingTheLabOnceHeld stands for the engineer who reads the
// Outcome and then stops the lab: the held Scenario's Skew line is its
// Outcome's last. What it printed by then is kept as printed while held.
type aTerminalStoppingTheLabOnceHeld struct {
	bytes.Buffer

	stopTheLab context.CancelFunc
	whileHeld  string
}

func (a *aTerminalStoppingTheLabOnceHeld) Write(line []byte) (int, error) {
	written, err := a.Buffer.Write(line)
	if strings.Contains(string(line), aSkewReported) {
		a.whileHeld = a.String()
		a.stopTheLab()
	}

	return written, err
}

// interactiveRunReported is the whole of what the Interactive run printed, and
// the part of it printed before the lab stopped.
type interactiveRunReported struct {
	whileHeld string
	whole     string
}

// theLiveViewLine is the whole line the demo recipe reads: its label, then the
// loopback address with a numeric port and its path, and nothing after.
var theLiveViewLine = regexp.MustCompile(`(?m)^Live view:\s+http://127\.0\.0\.1:[0-9]+/$`)

// anInteractiveRunPrintsItsSkewWithin bounds the shared Interactive run. A held
// Scenario whose Steps ended part-way prints no Skew line, so the terminal
// never stops the lab, which would otherwise hold its cluster until the test
// binary times out.
const anInteractiveRunPrintsItsSkewWithin = 10 * time.Minute

// anInteractiveRunNamingAScenario is shared so its tests hold one Simulated
// Cluster rather than one each. It runs under its own context, because it
// outlives the test that starts it.
var anInteractiveRunNamingAScenario = sync.OnceValues(func() (interactiveRunReported, error) {
	running, stopTheLab := context.WithTimeout(context.Background(), anInteractiveRunPrintsItsSkewWithin)
	defer stopTheLab()
	terminal := &aTerminalStoppingTheLabOnceHeld{stopTheLab: stopTheLab}
	var warned bytes.Buffer

	err := run(running, []string{interactive, aSecondScenarioOfTheLab}, sharedRunTimelines, terminal, &warned)
	reported := interactiveRunReported{whileHeld: terminal.whileHeld, whole: terminal.String()}
	if reported.whileHeld == "" {
		err = errors.Join(err, fmt.Errorf("no Skew line within %s, so the lab stopped there, having printed:\n%s",
			anInteractiveRunPrintsItsSkewWithin, reported.whole))
	}

	return reported, err
})

func theInteractiveRunNamingAScenario(t *testing.T) interactiveRunReported {
	t.Helper()

	requireEngine(t)
	reported, err := anInteractiveRunNamingAScenario()
	if err != nil {
		t.Fatalf("the Interactive run of %s: %v", aSecondScenarioOfTheLab, err)
	}

	return reported
}

func TestAnInteractiveRunNamingAScenario_ShouldReportItsStepsBeforeItsFirstWhen(t *testing.T) {
	t.Parallel()
	reported := theInteractiveRunNamingAScenario(t).whole

	reportShows(t, reported, nodeStopsRespondingReported)
	reportShows(t, reported, threePodsPerZoneJudged)
	reportLacks(t, reported, oneNodeFailsReported)
}

func TestAnInteractiveRunNamingAScenario_ShouldNameTheLiveViewAddressAloneOnItsLine(t *testing.T) {
	t.Parallel()
	reported := theInteractiveRunNamingAScenario(t).whole

	expect.Expect(t, theLiveViewLine.FindAllString(reported, -1)).To(expect.HaveLen[string](1))
}

func TestAnInteractiveRun_ShouldPrintItsTimelinesLineWhenStoppedNotWhenTheClusterIsHeld(t *testing.T) {
	t.Parallel()
	reported := theInteractiveRunNamingAScenario(t)

	reportLacks(t, reported.whileHeld, timelineReported)
	reportShows(t, strings.TrimPrefix(reported.whole, reported.whileHeld), timelineReported)
	reportShows(t, strings.TrimPrefix(reported.whole, reported.whileHeld), filedAt)
}

// A stopped run is watched on the engine itself, because what the run reports
// cannot show a container it lost track of. These tests do not run in
// parallel, so every container of a shipped Scenario on the engine while they
// run is their own run's.
const (
	aContainerCreated      = "create"
	aContainerStarted      = "start"
	aDeschedulerPassSuffix = "-descheduler"
	// The engineer finds a run's containers by this name on the engine, as the
	// troubleshooting guidance does.
	aContainerOfTheLab = "k8s-resilience-lab-"
)

func TestARunStoppedWhileItsScenariosProvision_ShouldRemoveEverySimulatedClusterStartedSoFar(t *testing.T) {
	requireEngine(t)
	engine := theEngineTheLabDials(t)
	leftBefore := engine.containersOfTheShippedScenarios(t)
	running, stopTheRun := context.WithCancel(t.Context())
	defer stopTheRun()
	engine.onFirst(t, aContainerStarted, aSimulatedCluster, stopTheRun)

	var reported, warned bytes.Buffer
	err := run(running, nil, t.TempDir(), &reported, &warned)

	expectNamed(t, err, context.Canceled.Error())
	expect.Expect(t, engine.containersOfTheShippedScenarios(t)).To(expect.Equal(leftBefore))
}

func TestARunOfEveryScenarioTheEngineerStops_ShouldEndEachScenarioPartWayAndLeaveNoSimulatedClusterOrDescheduler(t *testing.T) {
	requireEngine(t)
	engine := theEngineTheLabDials(t)
	leftBefore := engine.containersOfTheShippedScenarios(t)
	engine.onFirst(t, aContainerCreated, aDeschedulerPass, theEngineerStopsTheLab(t))

	var reported, warned bytes.Buffer
	err := run(t.Context(), nil, t.TempDir(), &reported, &warned)

	expectNamed(t, err, context.Canceled.Error())
	expect.Expect(t, theSectionsFilingNoTimeline(theScenarioSectionsOf(t, reported.String()))).To(expect.HaveLen[string](0))
	expect.Expect(t, engine.containersOfTheShippedScenarios(t)).To(expect.Equal(leftBefore))
}

func aSimulatedCluster(container string) bool {
	return !strings.HasSuffix(container, aDeschedulerPassSuffix)
}

func aDeschedulerPass(container string) bool {
	return strings.HasSuffix(container, aDeschedulerPassSuffix)
}

// theEngineerStopsTheLab interrupts the test binary itself, as the engineer's
// Ctrl-C does the lab. The test listens as well, so a lab that does not listen
// leaves the binary running to fail the test rather than killing it.
func theEngineerStopsTheLab(t *testing.T) func() {
	t.Helper()

	heard := make(chan os.Signal, 1)
	signal.Notify(heard, os.Interrupt)
	t.Cleanup(func() { signal.Stop(heard) })

	return func() {
		// A refused signal leaves the run unstopped, which fails the test.
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	}
}

func theSectionsFilingNoTimeline(sections []string) []string {
	return slices.DeleteFunc(sections, func(section string) bool {
		return strings.Contains(section, filedAt)
	})
}

// engineWatch is the test's own client to the engine the lab dials, since the
// lab holds no call listing containers or following the engine's events.
type engineWatch struct {
	client  *http.Client
	shipped []string
}

func theEngineTheLabDials(t *testing.T) engineWatch {
	t.Helper()

	endpoint, err := cluster.DetectEngine(t.Context(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("detect the engine the lab dials: %v", err)
	}
	socket := endpoint.Socket()
	endpoint.Close()
	shipped, err := lab.ScenarioNames(scenarios.All())
	if err != nil {
		t.Fatalf("name the shipped Scenarios: %v", err)
	}

	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socket)
		},
	}}
	t.Cleanup(client.CloseIdleConnections)

	return engineWatch{client: client, shipped: shipped}
}

// theEngineHost is ignored by the dialer, which reaches the socket alone.
const theEngineHost = "http://engine"

func (e engineWatch) containersOfTheShippedScenarios(t *testing.T) []string {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
		theEngineHost+"/containers/json?all=true", nil)
	if err != nil {
		t.Fatalf("compose the request listing the engine's containers: %v", err)
	}
	answer, err := e.client.Do(request)
	if err != nil {
		t.Fatalf("list the engine's containers: %v", err)
	}
	defer func() {
		if err := answer.Body.Close(); err != nil {
			t.Errorf("close the engine's list of containers: %v", err)
		}
	}()

	var listed []struct {
		Names []string `json:"Names"`
	}
	if err := json.NewDecoder(answer.Body).Decode(&listed); err != nil {
		t.Fatalf("read the engine's containers: %v", err)
	}

	var ours []string
	for _, container := range listed {
		for _, name := range container.Names {
			if named := strings.TrimPrefix(name, "/"); e.ofAShippedScenario(named) {
				ours = append(ours, named)
			}
		}
	}
	slices.Sort(ours)

	return ours
}

func (e engineWatch) ofAShippedScenario(container string) bool {
	return slices.ContainsFunc(e.shipped, func(scenarioName string) bool {
		return strings.HasPrefix(container, aContainerOfTheLab+scenarioName+"-")
	})
}

// onFirst follows the engine's events from before the run starts, so the stop
// lands at a moment the engine itself reports rather than after a guessed
// sleep. It calls then once, for the first container of a shipped Scenario the
// action happens to whose name matches.
func (e engineWatch) onFirst(t *testing.T, action string, matches func(string) bool, then func()) {
	t.Helper()

	watching, stopWatching := context.WithCancel(context.WithoutCancel(t.Context()))
	t.Cleanup(stopWatching)
	filters := url.Values{"filters": {`{"type":["container"],"event":["` + action + `"]}`}}
	request, err := http.NewRequestWithContext(watching, http.MethodGet, theEngineHost+"/events?"+filters.Encode(), nil)
	if err != nil {
		t.Fatalf("compose the request following the engine's events: %v", err)
	}
	answer, err := e.client.Do(request)
	if err != nil {
		t.Fatalf("follow the engine's events: %v", err)
	}

	go func() {
		// The stream ends by its context being cancelled, so its close reports
		// nothing a test could act on.
		defer func() { _ = answer.Body.Close() }()
		events := json.NewDecoder(answer.Body)
		for {
			var event struct {
				Actor struct {
					Attributes map[string]string `json:"Attributes"`
				} `json:"Actor"`
			}
			if err := events.Decode(&event); err != nil {
				return
			}
			if named := event.Actor.Attributes["name"]; e.ofAShippedScenario(named) && matches(named) {
				then()
				return
			}
		}
	}()
}
