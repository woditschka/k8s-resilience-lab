package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/cluster"
	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/lab"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// A well-formed version, so it passes validation and fails at the image pull.
const aVersionWithNoClusterImage = "1.0.0"

const aValueThatIsNotAVersion = "latest"

// The report is the whole surface a user of the lab sees, so a run is asserted
// through it rather than through anything the run holds inside.
const (
	versionReported  = "Kubernetes version: "
	placementSettled = "Placement settled"
)

const (
	// A Timeline's count and its printed entries are read inside one Scenario's
	// part of the report.
	aScenarioHeading = "Scenario "

	resilientPlacementReported = aScenarioHeading + "a-workload-placed-for-resilience"
	threePodsPerZoneJudged     = "Then 3 Pods per Zone"
	onePodPerNodeJudged        = "Then 1 Pod per Node"
	aFindingReported           = "Finding"

	// The Step's name is derived from the predefined Pod specification and the
	// Startup time its definition names, so a run is where the definition and
	// the line meet.
	resilientPlacementDeclared = "Given a Workload checkout of 9 Pods declaring Resilient placement, " +
		"each Pod starting in 500ms to 1s"
)

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
	theRequiredCapacityDeclared = resilientPlacementDeclared + ", keeping at least 6 Pods Running"
	theCapacityJudged           = "Then the Required Capacity held: a minimum of 6 Pods, "
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

const theRequestCarried = aSettingCarried + "resources.requests: {cpu: 3, memory: 12Gi}"

// theNodePickedToFail is the first Node the Topology derives, the pick a run
// has to repeat on every run.
const (
	nodeStopsRespondingReported = aScenarioHeading + "a-node-that-stops-responding"
	theNodePickedToFail         = "zone-1-node-1"
	oneNodeFailsReported        = "When 1 Node fails, " + theNodePickedToFail
	noPodOnTheFailedNodesJudged = "Then no Pod on the Nodes that failed"

	capacityHeldWhenTheNodeStoppedJudged = theCapacityJudged + "8 at the moment of the Failure and 9 once Settled"
)

// A lost Zone takes three of the nine Pods, so exactly the minimum is left
// Running at the moment of the Failure.
const (
	zoneStopsRespondingReported = aScenarioHeading + "a-zone-that-stops-responding"
	theZonePickedToFail         = "zone-1"
	oneZoneFailsReported        = "When 1 Zone fails, " + theZonePickedToFail
	noPodInTheFailedZonesJudged = "Then no Pod in the Zones that failed"

	capacityHeldWhenTheZoneStoppedJudged = theCapacityJudged + "6 at the moment of the Failure and 9 once Settled"
)

// Each Pod asks for more than half of what a Node holds, so the six Nodes still
// responding have room for six of the nine Pods, leaving three Pending.
const (
	noRoomAfterAZoneStopsRespondingReported = aScenarioHeading + "no-room-after-a-zone-stops-responding"
	thePlacementSettledAwaited              = "Wait for the Placement Settled, reached in "
	threePodsPendingJudged                  = "Then 3 Pods Pending"

	capacityHeldWithNoRoomLeftJudged = theCapacityJudged + "6 at the moment of the Failure and 6 once Settled"
)

// The spot Nodes hold three of the nine Pods, so exactly the minimum of six is
// left Running at the moment of the Failure.
const (
	spotNodesStopRespondingReported = aScenarioHeading + "spot-nodes-that-stop-responding"
	theOnDemandAndSpotNodesDeclared = "Given a Topology of 3 Zones of " +
		"a NodePool on-demand of 2 Standard Nodes holding 16 cpu, 64Gi memory and 10 Pods each, " +
		"labelled capacity-type=on-demand; " +
		"a NodePool spot of 1 Standard Node holding 16 cpu, 64Gi memory and 10 Pods each, labelled capacity-type=spot"
	theSpotNodesFailReported = "When Nodes labelled capacity-type=spot fail, " +
		"zone-1-node-3, zone-2-node-3, zone-3-node-3"
	capacityHeldWhenTheSpotNodesStoppedJudged = theCapacityJudged + "6 at the moment of the Failure and 9 once Settled"
)

// Each Zone holds one on-demand and one spot Node, so six Pods spread evenly
// put two in each Zone and three on each capacity type.
const (
	spreadOverOnDemandAndSpotReported = aScenarioHeading + "a-workload-spread-over-on-demand-and-spot"
	theLabelledNodePoolsDeclared      = "Given a Topology of 3 Zones of " +
		"a NodePool on-demand of 1 Standard Node holding 16 cpu, 64Gi memory and 10 Pods each, " +
		"labelled capacity-type=on-demand; " +
		"a NodePool spot of 1 Standard Node holding 16 cpu, 64Gi memory and 10 Pods each, labelled capacity-type=spot"
	spreadOverOnDemandAndSpotDeclared = "Given a Workload checkout of 6 Pods declaring Spread over on-demand and spot, " +
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
	sixGeneralPodsDeclared = "Given a Workload checkout of 6 Pods declaring Resilient placement, " +
		"each Pod starting in 500ms to 1s"
	toleratingGpuNodesDeclared = "Given a Workload inference of 3 Pods declaring Tolerating GPU Nodes, " +
		"each Pod starting in 500ms to 1s"
	sixPodsOnTheGeneralNodePoolJudged = "Then 6 Pods on the NodePool general"
	noPodOnTheGpuNodePoolJudged       = "Then 0 Pods on the NodePool gpu"
	threePodsOnTheGpuNodePoolJudged   = "Then 3 Pods on the NodePool gpu"
	noPodOnTheGeneralNodePoolJudged   = "Then 0 Pods on the NodePool general"
)

func toleratingGpuNodesCarries() string {
	return theLabsTolerationCarried("node.kubernetes.io/not-ready") +
		theLabsTolerationCarried("node.kubernetes.io/unreachable") +
		aSettingCarried + "tolerations: {key: nvidia.com/gpu, operator: Equal, value: present, effect: NoSchedule}" +
		aSettingCarried + "resources.requests: {cpu: 24, memory: 96Gi}"
}

const theNodeCapacityDeclared = "Given a Topology of 3 Zones of a NodePool general of 3 Nodes holding " +
	"4 cpu, 16Gi memory and 10 Pods each"

// aStandardNodeSetting is the run-start line the lab reported Standard on while
// it defined Standard itself.
const (
	theStandardNodesDeclared = "Given a Topology of 3 Zones of a NodePool general of 3 Standard Nodes holding " +
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

const theWorkloadOfEveryScenario = "checkout-"

const noPodShared = 0

// requireEngine fails only the calling test where no engine answers, so every
// test that needs no engine still runs.
func requireEngine(t *testing.T) {
	t.Helper()

	if _, err := cluster.DetectEngine(t.Context(), slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("no container engine answers on this machine, so the lab cannot run: %v", err)
	}
}

// aRun files its Timelines under the test's own directory, so a suite run
// leaves nothing in the repository.
func aRun(t *testing.T, arguments ...string) (string, error) {
	t.Helper()

	var reported, warned bytes.Buffer
	err := run(t.Context(), arguments, t.TempDir(), &reported, &warned)

	return reported.String(), err
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

func reportShows(t *testing.T, reported, line string) {
	t.Helper()

	if !strings.Contains(reported, line) {
		t.Errorf("the run reports %q, but reported:\n%s", line, reported)
	}
}

func reportLacks(t *testing.T, reported, line string) {
	t.Helper()

	if strings.Contains(reported, line) {
		t.Errorf("the run reports no %q, but reported:\n%s", line, reported)
	}
}

// A reported entry count and the entries printed under it are only comparable
// inside one Scenario's section.
func theScenarioSectionsOf(t *testing.T, reported string) []string {
	t.Helper()

	sections := strings.Split(reported, "\n"+aScenarioHeading)
	if len(sections) < 2 {
		t.Fatalf("a run reports each Scenario it carried out under its own heading, but reported:\n%s", reported)
	}

	return sections[1:]
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

func TestAWorkloadDeclaringResilientPlacement_ShouldReportBothSpreadsInFullBeneathItsGivenLine(t *testing.T) {
	t.Parallel()
	reported := theRunOfEveryScenario(t)

	reportShows(t, reported, resilientPlacementDeclared+resilientPlacementCarries())
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
	reportShows(t, reported, placementSettled)
	reportLacks(t, reported, aFindingReported)
}

func TestANodeThatStopsResponding_ShouldSettleAtThreePodsPerZoneWithNoneOnTheFailedNode(t *testing.T) {
	t.Parallel()
	reported := theRunOfEveryScenario(t)

	reportShows(t, reported, nodeStopsRespondingReported)
	reportShows(t, reported, theRequiredCapacityDeclared+resilientPlacementCarries())
	reportShows(t, reported, allNodesReadyAwaited)
	reportShows(t, reported, allPodsReadyAwaited)
	reportShows(t, reported, oneNodeFailsReported)
	reportShows(t, reported, theStatedRestoreTimeAwaited)
	reportShows(t, reported, noPodOnTheFailedNodesJudged)
	reportShows(t, reported, threePodsPerZoneJudged)
	reportShows(t, reported, capacityHeldWhenTheNodeStoppedJudged)
	reportLacks(t, reported, aFindingReported)
}

// Pods per Zone are not read after the Failure: two Zones carry nine Pods, so
// the Placement is skewed and the Scenario judges the capacity alone.
func TestARunOfAZoneThatStopsResponding_ShouldReportTheZoneItPickedToFail(t *testing.T) {
	t.Parallel()
	reported := theRunOfEveryScenario(t)

	reportShows(t, reported, zoneStopsRespondingReported)
	reportShows(t, reported, theRequiredCapacityDeclared+resilientPlacementCarries())
	reportShows(t, reported, allNodesReadyAwaited)
	reportShows(t, reported, allPodsReadyAwaited)
	reportShows(t, reported, oneZoneFailsReported)
	reportShows(t, reported, theStatedRestoreTimeAwaited)
	reportShows(t, reported, noPodInTheFailedZonesJudged)
	reportShows(t, reported, capacityHeldWhenTheZoneStoppedJudged)
	reportLacks(t, reported, aFindingReported)
}

func TestARunOfNoRoomAfterAZoneStopsResponding_ShouldSettleAtSixPodsRunningAndThreePending(t *testing.T) {
	t.Parallel()
	reported := theRunOfEveryScenario(t)

	reportShows(t, reported, noRoomAfterAZoneStopsRespondingReported)
	reportShows(t, reported, theNodeCapacityDeclared)
	reportShows(t, reported, theRequiredCapacityDeclared+resilientPlacementCarries()+theRequestCarried)
	reportShows(t, reported, onePodPerNodeJudged)
	reportShows(t, reported, oneZoneFailsReported)
	reportShows(t, reported, thePlacementSettledAwaited)
	reportShows(t, reported, theStatedRestoreTimeAwaited)
	reportShows(t, reported, noPodInTheFailedZonesJudged)
	reportShows(t, reported, threePodsPendingJudged)
	reportShows(t, reported, capacityHeldWithNoRoomLeftJudged)
	reportLacks(t, reported, aFindingReported)
}

func TestSpotNodesThatStopResponding_ShouldHoldTheRequiredCapacityAndRestoreNinePodsOnTheOnDemandNodes(t *testing.T) {
	t.Parallel()
	reported := theScenarioSectionHeaded(t, theRunOfEveryScenario(t), spotNodesStopRespondingReported)

	reportShows(t, reported, theOnDemandAndSpotNodesDeclared)
	reportShows(t, reported, theRequiredCapacityDeclared+resilientPlacementCarries())
	reportShows(t, reported, onePodPerNodeJudged)
	reportShows(t, reported, theSpotNodesFailReported)
	reportShows(t, reported, theStatedRestoreTimeAwaited)
	reportShows(t, reported, threePodsPerZoneJudged)
	reportShows(t, reported, noPodOnTheFailedNodesJudged)
	reportShows(t, reported, capacityHeldWhenTheSpotNodesStoppedJudged)
	reportLacks(t, reported, aFindingReported)
}

func TestSpreadOverOnDemandAndSpot_ShouldPlaceTwoPodsPerZoneAndThreePerLabelValue(t *testing.T) {
	t.Parallel()
	reported := theScenarioSectionHeaded(t, theRunOfEveryScenario(t), spreadOverOnDemandAndSpotReported)

	reportShows(t, reported, theLabelledNodePoolsDeclared)
	reportShows(t, reported, spreadOverOnDemandAndSpotDeclared+spreadOverOnDemandAndSpotCarries())
	reportShows(t, reported, everyPodRunningJudged)
	reportShows(t, reported, twoPodsPerZoneJudged)
	reportShows(t, reported, threePodsPerCapacityTypeJudged)
	reportLacks(t, reported, aFindingReported)
}

func TestGeneralPodsStayOffGpuNodes_ShouldPlaceTheGeneralPodsOnGeneralNodesAndTheTolerantPodsOnGpuNodes(t *testing.T) {
	t.Parallel()
	reported := theScenarioSectionHeaded(t, theRunOfEveryScenario(t), generalPodsStayOffGpuNodesReported)

	reportShows(t, reported, theTaintedGpuNodePoolDeclared)
	reportShows(t, reported, sixGeneralPodsDeclared+resilientPlacementCarries())
	reportShows(t, reported, sixPodsOnTheGeneralNodePoolJudged)
	reportShows(t, reported, noPodOnTheGpuNodePoolJudged)
	reportShows(t, reported, toleratingGpuNodesDeclared+toleratingGpuNodesCarries())
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

	reportShows(t, reported, theStandardNodesDeclared)
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

func TestACollapsedSummary_ShouldLeaveEveryEntryInTheTimelineFile(t *testing.T) {
	t.Parallel()
	reported := theRunOfEveryScenario(t)

	filed := theTimelineFiledBy(t, reported)
	expect.Expect(t, theEntriesPrintedIn(t, reported)).To(expect.HaveLen[float64](noEntryPrinted))
	expect.Expect(t, filed[1:]).To(expect.HaveLen[string](theEntryCountReportedIn(t, reported)))
}

func TestATimelineSummary_ShouldOpenWithTheEntryCountAndTheFileItWasFiledAt(t *testing.T) {
	settled := lab.Outcome{
		Name: theScenarioReported, Cluster: itsCluster,
		Steps:    []lab.Step{aThenStep()},
		Timeline: aTimelineOfTwoChanges(),
		FiledAt:  someTimelinePath,
	}

	reported := whatTheRunReportsFor(t, settled, theSummaryAlone())

	reportShows(t, reported, timelineReported+strconv.Itoa(len(settled.Timeline.Entries()))+entriesReported+filedAt+someTimelinePath)
	reportShowsTheSummary(t, reported, settled.Timeline)
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

func TestAScenarioThatFailsPartWay_ShouldStillPrintAndFileItsTimeline(t *testing.T) {
	endedAtItsTopology := lab.Outcome{
		Name: theScenarioReported, Cluster: itsCluster,
		Steps:    []lab.Step{aTopologyStep()},
		EndedAt:  aTopologyStep(),
		Cause:    errors.New(whatEndedTheScenario),
		Timeline: aTimelineOfTwoChanges(),
		FiledAt:  someTimelinePath,
	}

	reported := whatTheRunReportsFor(t, endedAtItsTopology, theSummaryAlone())

	reportShows(t, reported, someTimelinePath)
	reportShowsTheSummary(t, reported, endedAtItsTopology.Timeline)
	reportLacks(t, reported, placementSettled)
}

func TestATimelineThatCannotBeWritten_ShouldBeNamedWhileTheScenarioOutcomeStands(t *testing.T) {
	heldButNotFiled := lab.Outcome{
		Name: theScenarioReported, Cluster: itsCluster,
		Steps:    []lab.Step{aThenStep()},
		Timeline: aTimelineOfTwoChanges(),
		NotFiled: errors.New(whyTheTimelineWasNotFiled),
	}

	reported, unfiled, found := whatTheRunReportsFor(t, heldButNotFiled, theSummaryAlone()), notFiled(heldButNotFiled), didNotHold(heldButNotFiled)

	reportShows(t, reported, aThenStep().String())
	reportShows(t, reported, whyTheTimelineWasNotFiled)
	reportShowsTheSummary(t, reported, heldButNotFiled.Timeline)
	expectNamed(t, unfiled, theScenarioReported)
	if found != nil {
		t.Errorf("the Scenario's own outcome stands when its Timeline could not be filed, but the run reports: %v", found)
	}
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
	reportShows(t, reported, placementSettled)
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

func TestAValueThatIsNotAVersion_ShouldStopTheRunWithoutReachingTheEngine(t *testing.T) {
	t.Parallel()
	reported, err := aRun(t, "--k8s-version", aValueThatIsNotAVersion)

	if err == nil {
		t.Fatalf("a value that is not a version stops the run, but it reported:\n%s", reported)
	}
	expect.Expect(t, reported).To(expect.Equal(""))
}

func TestARunOfEveryScenario_ShouldReportEachScenariosLinesTogetherInDeclaredOrder(t *testing.T) {
	t.Parallel()
	declared := []string{
		resilientPlacementReported, nodeStopsRespondingReported,
		zoneStopsRespondingReported, noRoomAfterAZoneStopsRespondingReported,
		spreadOverOnDemandAndSpotReported, spotNodesStopRespondingReported,
		generalPodsStayOffGpuNodesReported,
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
			if entry.Kind != string(scenario.PodObject) || !strings.HasPrefix(entry.Name, theWorkloadOfEveryScenario) {
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

func TestASimulatedClusterTheRuntimeCannotStart_ShouldEndOnlyItsOwnScenario(t *testing.T) {
	carried := []lab.Carried{aScenarioThatHeld(), aScenarioWhoseClusterDidNotStart()}

	reported, err := whatTheRunReportsForEach(t, carried)

	reportShows(t, reported, aScenarioHeading+theScenarioReported)
	reportShows(t, reported, aThenStep().String())
	reportShows(t, reported, aScenarioHeading+theScenarioNotStarted)
	reportShows(t, reported, whyTheClusterDidNotStart)
	expectNamed(t, err, whyTheClusterDidNotStart)
	reportLacks(t, err.Error(), theScenarioReported)
}

func TestARunNamingOneScenario_ShouldReportItAsARunOfEveryScenarioReportsIt(t *testing.T) {
	alone, _ := whatTheRunReportsForEach(t, []lab.Carried{aScenarioThatHeld()})
	amongOthers, _ := whatTheRunReportsForEach(t, []lab.Carried{aScenarioThatHeld(), aScenarioWhoseClusterDidNotStart()})

	expect.Expect(t, theScenarioSectionsOf(t, alone)[0]).To(expect.Equal(theScenarioSectionsOf(t, amongOthers)[0]))
}

func aScenarioThatHeld() lab.Carried {
	return lab.Carried{Outcome: lab.Outcome{
		Name: theScenarioReported, Cluster: itsCluster,
		Steps:    []lab.Step{aThenStep()},
		Timeline: aTimelineOfTwoChanges(),
		FiledAt:  someTimelinePath,
	}}
}

func aScenarioWhoseClusterDidNotStart() lab.Carried {
	return lab.Carried{
		Outcome: lab.Outcome{Name: theScenarioNotStarted},
		Err:     errors.New(whyTheClusterDidNotStart),
	}
}

func whatTheRunReportsForEach(t *testing.T, carried []lab.Carried) (string, error) {
	t.Helper()

	var reported bytes.Buffer
	err := writeRun(&runReport{to: &reported}, carried, theSummaryAlone())

	return reported.String(), err
}

// These Outcomes are built rather than run, because a Scenario whose Steps fail
// part-way cannot be written as a Scenario source.
const (
	theScenarioReported = "a-workload-placed-for-resilience"
	itsCluster          = "k8s-resilience-lab-a-workload-placed-for-resilience-7f3a9c"
	theThenStepName     = "3 Pods per Zone"
	aTopologyStepName   = "a Topology of 3 Zones of 3 Nodes"

	whatEndedTheScenario      = "apply " + aTopologyStepName + ": create Node zone-1-node-1: already exists"
	whyTheTimelineWasNotFiled = "create the Timeline folder /read-only/timelines: permission denied"
	someTimelinePath          = "/tmp/a-run/timelines/k8s-resilience-lab-a-workload-placed-for-resilience-7f3a9c.jsonl"

	theScenarioNotStarted    = "a-node-that-stops-responding"
	whyTheClusterDidNotStart = "the Scenario " + theScenarioNotStarted + ": start the Simulated Cluster: port already allocated"
)

const (
	whenTheNodeWasAdded    = 120 * time.Millisecond
	theNodeAdded           = "zone-1-node-1"
	theNodeReady           = "Ready"
	theNodeCapacity        = "4 cpu, 16Gi memory and 10 Pods"
	whenThePodWasScheduled = 3470 * time.Millisecond
	thePodReported         = "web-5d8f9b7c6-2xk4m"
	thePodScheduled        = "Scheduled"
	whereThePodWasPlaced   = "Successfully assigned default/web-5d8f9b7c6-2xk4m to zone-1-node-1"
)

const theScenarioRecorded = `"scenario":"` + theScenarioReported + `"`

const (
	onACluster       = ", on "
	timelineReported = "Timeline, "
	entriesReported  = " entries"
	filedAt          = ", filed at "
)

func aTopologyStep() lab.Step {
	return lab.Step{Kind: lab.GivenStep, Name: aTopologyStepName}
}

func aThenStep() lab.Step {
	return lab.Step{Kind: lab.ThenStep, Name: theThenStepName}
}

func aTimelineOfTwoChanges() scenario.Timeline {
	return scenario.NewTimeline([]scenario.Entry{
		{
			Elapsed: whenTheNodeWasAdded, Change: scenario.ChangeAdded, Kind: scenario.NodeObject,
			Name: theNodeAdded, Reason: theNodeReady, Detail: theNodeCapacity,
		},
		{
			Elapsed: whenThePodWasScheduled, Change: scenario.ChangeReported, Kind: scenario.PodObject,
			Name: thePodReported, Reason: thePodScheduled, Detail: whereThePodWasPlaced,
		},
	})
}

func whatTheRunReportsFor(t *testing.T, outcome lab.Outcome, asked runSetting) string {
	t.Helper()

	var reported bytes.Buffer
	writeOutcome(&runReport{to: &reported}, outcome, asked)

	return reported.String()
}

func theSummaryAlone() runSetting {
	return runSetting{}
}

// What the summary lines say is internal/scenario's to assert; this asserts
// only that the run printed them.
func reportShowsTheSummary(t *testing.T, reported string, recorded scenario.Timeline) {
	t.Helper()

	for _, line := range recorded.Summarise().Lines() {
		reportShows(t, reported, line)
	}
}

func theEntriesPrintedIn(t *testing.T, reported string) []float64 {
	t.Helper()

	var received []float64
	for line := range strings.SplitSeq(reported, "\n") {
		printed := strings.TrimSpace(line)
		if !strings.HasPrefix(printed, "+") {
			continue
		}

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

	path, _, _ := strings.Cut(filing, ":\n")
	filed, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the filed Timeline %s: %v", path, err)
	}

	return strings.Split(strings.TrimSuffix(string(filed), "\n"), "\n")
}

func expectNamed(t *testing.T, err error, named string) {
	t.Helper()

	if err == nil {
		t.Fatalf("the run reports an error naming %s, but reports none", named)
	}
	if !strings.Contains(err.Error(), named) {
		t.Errorf("the error names %s, but reads: %v", named, err)
	}
}
