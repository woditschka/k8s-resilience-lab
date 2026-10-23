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
	"unicode/utf8"

	"github.com/woditschka/k8s-resilience-lab/internal/cluster"
	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/lab"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
	"github.com/woditschka/k8s-resilience-lab/scenarios"
)

// A well-formed version, so it passes validation and fails at the image pull.
const aVersionWithNoClusterImage = "1.0.0"

const aValueThatIsNotAVersion = "latest"

// The report is the whole surface a user of the lab sees, so a run is asserted
// through it rather than through anything the run holds inside.
const (
	versionReported  = "Kubernetes version: "
	placementSettled = "Placement settled"
	// A board's Zone line stands alone at the Steps' indent.
	theFirstZoneOnABoard = "\n  zone-1\n"
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

// A Workload line's Request reports after the settings its specification
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

	// The heavier separator above the next Scenario belongs to neither, so it is
	// read off the section it ends.
	for at := range sections {
		sections[at] = strings.TrimRight(sections[at], aHeavierSeparatorRune)
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

func TestAWorkloadNamingStandardPod_ShouldReportTheRequestBeneathItsGivenLine(t *testing.T) {
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
	reportShows(t, reported, theRequiredCapacityDeclared+resilientPlacementCarries()+theStandardPodCarried)
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
	reportShows(t, reported, theRequiredCapacityDeclared+resilientPlacementCarries()+theStandardPodCarried)
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
	reportShows(t, reported, theRequiredCapacityDeclared+resilientPlacementCarries()+theStandardPodCarried)
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

func TestATimelinesLine_ShouldEndAtItsFileBehindASeparator(t *testing.T) {
	settled := lab.Outcome{
		Name: theScenarioReported, Cluster: itsCluster,
		Steps:    []lab.Step{aThenStep()},
		Timeline: aTimelineOfTwoChanges(),
		FiledAt:  someTimelinePath,
	}
	theLine := timelineReported + strconv.Itoa(len(settled.Timeline.Entries())) + entriesReported + filedAt + someTimelinePath

	reported := whatTheRunReportsFor(t, settled, theReportAlone())

	closing := theLinesAfter(t, reported, reportedAsAStep(aThenStep().String()), 2)
	expect.Expect(t, spansAtLeast(closing[0], utf8.RuneCountInString(theLine))).To(expect.Equal(true))
	expect.Expect(t, closing[1]).To(expect.Equal(reportedAsAStep(theLine)))
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

	reported := whatTheRunReportsFor(t, endedAtItsTopology, theReportAlone())

	reportShows(t, reported, someTimelinePath)
	reportLacks(t, reported, aSkewReported)
}

func TestATimelineThatCannotBeWritten_ShouldBeNamedWhileTheScenarioOutcomeStands(t *testing.T) {
	heldButNotFiled := lab.Outcome{
		Name: theScenarioReported, Cluster: itsCluster,
		Steps:    []lab.Step{aThenStep()},
		Timeline: aTimelineOfTwoChanges(),
		NotFiled: errors.New(whyTheTimelineWasNotFiled),
	}

	reported, unfiled, found := whatTheRunReportsFor(t, heldButNotFiled, theReportAlone()), notFiled(heldButNotFiled), didNotHold(heldButNotFiled)

	reportShows(t, reported, aThenStep().String())
	reportShows(t, reported, filedNowhere+whyTheTimelineWasNotFiled)
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

// whatTheRunReportsForEach opens the report as a run does, so a heavier
// separator has the run's opening lines to divide the first Scenario from.
func whatTheRunReportsForEach(t *testing.T, carried []lab.Carried) (string, error) {
	t.Helper()

	var reported bytes.Buffer
	report := &runReport{to: &reported}
	report.printPart(reportPart{anOpeningLine})
	err := writeRun(report, carried, theReportAlone())

	return reported.String(), err
}

func thePartReportedFor(t *testing.T, carried lab.Carried) string {
	t.Helper()

	var reported bytes.Buffer
	var part reportPart
	writeCarried(&part, carried, theReportAlone())
	(&runReport{to: &reported}).printPart(part)

	return reported.String()
}

// Wider than any line a built Scenario reports, so the first heavier separator
// has to reach the opening lines to pass.
const anOpeningLine = "engine socket:      /a/socket/path/long/enough/to/be/wider/than/every/line/of/the/first/scenario/that/follows/it/in/the/run/report"

const (
	aHeavierSeparatorRune      = "="
	oneAboveEachOfTwoScenarios = 2
	oneAboveTheOnlyScenario    = 1
)

func theHeavierSeparatorsIn(reported string) []string {
	var separators []string
	for line := range strings.SplitSeq(reported, "\n") {
		if line != "" && strings.Trim(line, aHeavierSeparatorRune) == "" {
			separators = append(separators, line)
		}
	}

	return separators
}

func theWidestLineOf(reported string) int {
	widest := 0
	for line := range strings.SplitSeq(reported, "\n") {
		widest = max(widest, utf8.RuneCountInString(line))
	}

	return widest
}

func TestARunOfTwoScenarios_ShouldDivideItsPartsWithAHeavierSeparator(t *testing.T) {
	first, second := aScenarioThatHeld(), aScenarioWhoseClusterDidNotStart()
	firstPart, secondPart := thePartReportedFor(t, first), thePartReportedFor(t, second)

	reported, _ := whatTheRunReportsForEach(t, []lab.Carried{first, second})

	heavier := theHeavierSeparatorsIn(reported)
	expect.Expect(t, heavier).To(expect.HaveLen[string](oneAboveEachOfTwoScenarios))
	expect.Expect(t, reported).To(expect.Equal(anOpeningLine + "\n" + heavier[0] + "\n" + firstPart + heavier[1] + "\n" + secondPart))
	expect.Expect(t, len(heavier[0]) >= max(theWidestLineOf(anOpeningLine), theWidestLineOf(firstPart))).To(expect.Equal(true))
	expect.Expect(t, len(heavier[1]) >= max(theWidestLineOf(firstPart), theWidestLineOf(secondPart))).To(expect.Equal(true))
}

func TestARunOfOneScenario_ShouldPrintOneHeavierSeparatorAfterItsOpeningLines(t *testing.T) {
	only := aScenarioThatHeld()
	onlyPart := thePartReportedFor(t, only)

	reported, _ := whatTheRunReportsForEach(t, []lab.Carried{only})

	heavier := theHeavierSeparatorsIn(reported)
	expect.Expect(t, heavier).To(expect.HaveLen[string](oneAboveTheOnlyScenario))
	expect.Expect(t, reported).To(expect.Equal(anOpeningLine + "\n" + heavier[0] + "\n" + onlyPart))
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

const aStepFiled = `"kind":"Step"`

// An entry prints under the filing line, one level deeper than a narrative's
// lines, which also open with a time.
const anEntryIndent = "    "

const (
	onACluster       = ", on "
	timelineReported = "Timeline, "
	entriesReported  = " entries"
	filedAt          = ", filed at "
	filedNowhere     = ", filed nowhere: "
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
	var part reportPart
	writeOutcome(&part, outcome, asked)
	(&runReport{to: &reported}).printPart(part)

	return reported.String()
}

func theReportAlone() runSetting {
	return runSetting{}
}

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

func expectNamed(t *testing.T, err error, named string) {
	t.Helper()

	if err == nil {
		t.Fatalf("the run reports an error naming %s, but reports none", named)
	}
	if !strings.Contains(err.Error(), named) {
		t.Errorf("the error names %s, but reads: %v", named, err)
	}
}

// A board of one Zone of one Node holding one Pod is enough to tell its lines
// from the Steps around it.
const (
	someNodePool        = "general"
	someNodeCPU         = scenario.CPU(16000)
	someNodeMemory      = scenario.Memory(65536)
	someNodePods        = 10
	oneZone             = 1
	oneNodePerZone      = 1
	theOnlyNode         = "zone-1-node-1"
	aWorkloadOnTheBoard = "checkout"
	aPodOnTheBoard      = "checkout-5fc87ff7c8-7vd48"
	theWaitStepName     = "all Pods Ready"
	theWhenStepName     = "1 Node fails, zone-1-node-1"
	aSecondThenStepName = "1 Pod per Node"
	whyNoBoard          = "observe the Placement board: context canceled"
	aStepIndent         = "  "
	aSeparatorRune      = "-"

	// One rune, so an empty line does not pass for a separator.
	aSeparatorOfAnyWidth = 1
)

func aBoardOfOnePod(t *testing.T) scenario.PlacementBoard {
	t.Helper()

	return scenario.NewPlacementBoard(aTopologyOfOneNode(t),
		scenario.NewPlacement(aTopologyOfOneNode(t).Nodes(), []scenario.Pod{thePodOnTheBoard()}), scenario.PlacementBoard{})
}

func thePodOnTheBoard() scenario.Pod {
	return scenario.Pod{Name: aPodOnTheBoard, Workload: aWorkloadOnTheBoard, Node: theOnlyNode, Phase: scenario.PodRunning}
}

func aTopologyOfOneNode(t *testing.T) scenario.Topology {
	t.Helper()

	capacity, err := scenario.NewNodeCapacity(someNodePods, someNodeCPU, someNodeMemory)
	if err != nil {
		t.Fatalf("build a Node capacity: %v", err)
	}
	pool, err := scenario.NewNodePool(someNodePool, oneNodePerZone, capacity)
	if err != nil {
		t.Fatalf("build the NodePool %s: %v", someNodePool, err)
	}
	topology, err := scenario.NewTopology(oneZone, pool)
	if err != nil {
		t.Fatalf("build a Topology of one Node: %v", err)
	}

	return topology
}

func aWaitStepWith(board scenario.PlacementBoard) lab.Step {
	return lab.Step{Kind: lab.WaitStep, Name: theWaitStepName, Board: board}
}

func aSecondThenStep() lab.Step {
	return lab.Step{Kind: lab.ThenStep, Name: aSecondThenStepName}
}

func aWhenStep() lab.Step {
	return lab.Step{Kind: lab.WhenStep, Name: theWhenStepName}
}

func anOutcomeOf(steps ...lab.Step) lab.Outcome {
	return lab.Outcome{Name: theScenarioReported, Cluster: itsCluster, Steps: steps, Timeline: aTimelineOfTwoChanges(), FiledAt: someTimelinePath}
}

// theLinesAfter reads on from the first line equal to the one named, so a test
// pins what stands under a Step and not where the Step falls in the report.
// theLinesAfter fails the test rather than handing back fewer lines than asked
// for, so a short report reads as what it lacks instead of an index panic.
func theLinesAfter(t *testing.T, reported, line string, count int) []string {
	t.Helper()

	lines := strings.Split(reported, "\n")
	at := slices.Index(lines, line)
	if at < 0 {
		t.Fatalf("the run reports the line %q, but reported:\n%s", line, reported)
	}
	after := lines[at+1:]
	if len(after) < count {
		t.Fatalf("the run reports %d lines after %q, want at least %d, reported:\n%s", len(after), line, count, reported)
	}

	return after[:count]
}

func reportedAsAStep(read string) string {
	return aStepIndent + read
}

func reportedUnderAStep(lines []string) []string {
	indented := make([]string, 0, len(lines))
	for _, line := range lines {
		indented = append(indented, aStepIndent+line)
	}

	return indented
}

// A separator is read by its length alone, since it need only reach the widest board line.
func spansAtLeast(line string, width int) bool {
	dashes := strings.TrimPrefix(line, aStepIndent)

	return strings.Trim(dashes, aSeparatorRune) == "" && len(dashes) >= width
}

func TestAWaitWithNothingToNarrate_ShouldPrintTheSeparatorAndTheBoardDirectlyUnderItsLine(t *testing.T) {
	board := aBoardOfOnePod(t)
	waited := aWaitStepWith(board)

	reported := whatTheRunReportsFor(t, anOutcomeOf(waited), theReportAlone())

	under := theLinesAfter(t, reported, reportedAsAStep(waited.String()), 1+len(board.Lines()))
	expect.Expect(t, spansAtLeast(under[0], board.Width())).To(expect.Equal(true))
	expect.Expect(t, under[1:]).To(expect.Equal(reportedUnderAStep(board.Lines())))
}

func TestAWaitWithNoBoard_ShouldNameWhyInTheBoardsPlace(t *testing.T) {
	unobserved := lab.Step{Kind: lab.WaitStep, Name: theWaitStepName, BoardNotObserved: errors.New(whyNoBoard)}

	reported := whatTheRunReportsFor(t, anOutcomeOf(unobserved), theReportAlone())

	under := theLinesAfter(t, reported, reportedAsAStep(unobserved.String()), 2)
	expect.Expect(t, under[1]).To(expect.Equal(reportedAsAStep("no Placement board: " + whyNoBoard)))
}

func TestTheThensJudgingAWait_ShouldPrintAfterItsBoardBehindASecondSeparator(t *testing.T) {
	board := aBoardOfOnePod(t)
	waited := aWaitStepWith(board)

	reported := whatTheRunReportsFor(t, anOutcomeOf(waited, aThenStep(), aSecondThenStep()), theReportAlone())

	underTheBoard := theLinesAfter(t, reported, reportedAsAStep(waited.String()), 1+len(board.Lines())+3)[1+len(board.Lines()):]
	expect.Expect(t, spansAtLeast(underTheBoard[0], board.Width())).To(expect.Equal(true))
	expect.Expect(t, underTheBoard[1:]).To(expect.Equal([]string{
		reportedAsAStep(aThenStep().String()),
		reportedAsAStep(aSecondThenStep().String()),
	}))
}

func TestAPlacementBoard_ShouldCloseWithASeparatorBeforeTheNextStep(t *testing.T) {
	board := aBoardOfOnePod(t)
	waited := aWaitStepWith(board)

	reported := whatTheRunReportsFor(t, anOutcomeOf(waited, aWhenStep()), theReportAlone())

	underTheBoard := theLinesAfter(t, reported, reportedAsAStep(waited.String()), 1+len(board.Lines())+2)[1+len(board.Lines()):]
	expect.Expect(t, spansAtLeast(underTheBoard[0], board.Width())).To(expect.Equal(true))
	expect.Expect(t, underTheBoard[1]).To(expect.Equal(reportedAsAStep(aWhenStep().String())))
}

// A Wait the Scenario ended at is the last Step, so the Timeline's own
// separator and line follow its closing one.
func TestAWaitTheScenarioEndsAt_ShouldStillCloseItsBoardWithASeparator(t *testing.T) {
	board := aBoardOfOnePod(t)
	waited := aWaitStepWith(board)

	reported := whatTheRunReportsFor(t, anOutcomeOf(waited), theReportAlone())

	underTheBoard := theLinesAfter(t, reported, reportedAsAStep(waited.String()), 1+len(board.Lines())+3)[1+len(board.Lines()):]
	expect.Expect(t, spansAtLeast(underTheBoard[0], board.Width())).To(expect.Equal(true))
	expect.Expect(t, strings.HasPrefix(underTheBoard[2], reportedAsAStep(timelineReported))).To(expect.Equal(true))
}

func TestAThenFollowedByAWhen_ShouldStandWithNoSeparatorBetween(t *testing.T) {
	reported := whatTheRunReportsFor(t, anOutcomeOf(aWaitStepWith(aBoardOfOnePod(t)), aThenStep(), aWhenStep()), theReportAlone())

	afterTheThen := theLinesAfter(t, reported, reportedAsAStep(aThenStep().String()), 1)
	expect.Expect(t, afterTheThen).To(expect.Equal([]string{reportedAsAStep(aWhenStep().String())}))
}

const (
	aSkewReported = "Skew of "
	// A Then reads "Pods per Zone" too, so a count table is known by its
	// heading's opening.
	aZoneTableHead = "  Pods per Zone ("
	aNodeTableHead = "  Pods per Node ("
)

func TestAScenarioReport_ShouldCloseWithOneSkewLineAndTheTimelinesLineAndNoCountTables(t *testing.T) {
	board := aBoardOfOnePod(t)
	waited := aWaitStepWith(board)
	settled := scenario.NewPlacement(aTopologyOfOneNode(t).Nodes(), []scenario.Pod{thePodOnTheBoard()})
	held := anOutcomeOf(waited, aThenStep())
	held.Placement, held.JudgedWorkload = settled, aWorkloadOnTheBoard

	reported := whatTheRunReportsFor(t, held, theReportAlone())

	theSkew := fmt.Sprintf("Skew of %s: %d over Zones, %d over Nodes",
		aWorkloadOnTheBoard, settled.Skew(scenario.SpreadOverZones), settled.Skew(scenario.SpreadOverNodes))
	afterTheLastThen := theLinesAfter(t, reported, reportedAsAStep(aThenStep().String()), 3)
	expect.Expect(t, afterTheLastThen[0]).To(expect.Equal(reportedAsAStep(theSkew)))
	expect.Expect(t, spansAtLeast(afterTheLastThen[1], aSeparatorOfAnyWidth)).To(expect.Equal(true))
	expect.Expect(t, strings.HasPrefix(afterTheLastThen[2], reportedAsAStep(timelineReported))).To(expect.Equal(true))
	reportLacks(t, reported, aZoneTableHead)
	reportLacks(t, reported, aNodeTableHead)
	reportLacks(t, reported, placementSettled)
}

// The narrative's widest line is wider than the Wait line and the board, so the
// separator reaching it shows the separator spans the narrative too.
const (
	aNodeTurningReady   = 120 * time.Millisecond
	thePodRunning       = 1300 * time.Millisecond
	theWaitNoted        = 1500 * time.Millisecond
	aWaitNoteIsNamed    = scenario.WaitStepNamed
	theWaitLineNoted    = "Wait for all Pods Ready"
	aWideNodeName       = "a-node-whose-name-is-wider-than-the-wait-line-and-the-board"
	someDeploymentReady = "1 of 1 Pods ready"
)

func aNarrativeOfOnePod() scenario.WaitNarrative {
	narratives := scenario.NewTimeline([]scenario.Entry{
		{Elapsed: aNodeTurningReady, Change: scenario.ChangeAdded, Kind: scenario.NodeObject, Name: aWideNodeName, Reason: string(scenario.NodeReady)},
		{Elapsed: aNodeTurningReady, Change: scenario.ChangeAdded, Kind: scenario.DeploymentObject, Name: aWorkloadOnTheBoard, Detail: someDeploymentReady},
		{Elapsed: thePodRunning, Change: scenario.ChangeAdded, Kind: scenario.PodObject, Name: aPodOnTheBoard, Reason: string(scenario.PodRunning), Detail: theOnlyNode},
		{Elapsed: theWaitNoted, Change: scenario.ChangeNoted, Kind: scenario.StepObject, Name: aWaitNoteIsNamed, Detail: theWaitLineNoted},
	}).WaitNarratives()

	return narratives[0]
}

func TestAWaitsNarrative_ShouldPrintUnderItsLineAboveTheSeparatorAndTheBoard(t *testing.T) {
	board, narrative := aBoardOfOnePod(t), aNarrativeOfOnePod()
	waited := aWaitStepWith(board)
	waited.Narrative = narrative
	narrated := len(narrative.Lines())

	reported := whatTheRunReportsFor(t, anOutcomeOf(waited), theReportAlone())

	under := theLinesAfter(t, reported, reportedAsAStep(waited.String()), narrated+1+len(board.Lines()))
	expect.Expect(t, under[:narrated]).To(expect.Equal(reportedUnderAStep(narrative.Lines())))
	expect.Expect(t, spansAtLeast(under[narrated], narrative.Width())).To(expect.Equal(true))
	expect.Expect(t, under[narrated+1:]).To(expect.Equal(reportedUnderAStep(board.Lines())))
}

// The filing line ends the report: nothing of a summary follows it.
func TestAScenarioReport_ShouldPrintNoSummaryOfEachWorkloadsPods(t *testing.T) {
	held := anOutcomeOf(aWaitStepWith(aBoardOfOnePod(t)), aThenStep())

	reported := whatTheRunReportsFor(t, held, theReportAlone())

	lines := strings.Split(strings.TrimSuffix(reported, "\n"), "\n")
	expect.Expect(t, strings.HasPrefix(lines[len(lines)-1], reportedAsAStep(timelineReported))).To(expect.Equal(true))
}

const (
	interactive              = "--interactive"
	aNameNoScenarioCarries   = "a-condition-no-scenario-states"
	theLiveViewOnLoopback    = "Live view:          http://127.0.0.1:"
	theFirstScenarioOfTheLab = "a-workload-placed-for-resilience"
	aSecondScenarioOfTheLab  = "a-node-that-stops-responding"
	theTimelineFiled         = "Timeline, "
)

func everyScenarioNameInOrder(t *testing.T) string {
	t.Helper()

	names, err := lab.ScenarioNames(scenarios.All())
	if err != nil {
		t.Fatalf("derive the name of every Scenario of the lab: %v", err)
	}

	return strings.Join(names, ", ")
}

func TestAnInteractiveRunNamingAnUnknownScenario_ShouldStopBeforeProvisioningAndNameTheScenariosThatExist(t *testing.T) {
	_, err := aRun(t, interactive, aNameNoScenarioCarries)

	expectNamed(t, err, everyScenarioNameInOrder(t))
}

func TestScenarioNamesAnInteractiveRunCannotTake_ShouldStopTheLabBeforeTheEngineNamingTheScenariosThatExist(t *testing.T) {
	refused := map[string][]string{
		"no name":                      {interactive},
		"two names":                    {interactive, theFirstScenarioOfTheLab, aSecondScenarioOfTheLab},
		"a name without --interactive": {theFirstScenarioOfTheLab},
	}

	for named, arguments := range refused {
		t.Run(named, func(t *testing.T) {
			_, err := aRun(t, arguments...)

			expectNamed(t, err, everyScenarioNameInOrder(t))
		})
	}
}

// aTerminalStoppingTheLabOnceFiled stands for the engineer who reads the
// Outcome and then stops the lab: the filing line is the Outcome's last.
type aTerminalStoppingTheLabOnceFiled struct {
	bytes.Buffer

	stopTheLab context.CancelFunc
}

func (a *aTerminalStoppingTheLabOnceFiled) Write(line []byte) (int, error) {
	if strings.Contains(string(line), theTimelineFiled) {
		defer a.stopTheLab()
	}

	return a.Buffer.Write(line)
}

func TestAnInteractiveRunNamingAScenario_ShouldReportItsStepsBeforeItsFirstWhenAndWhereTheLiveViewIsReached(t *testing.T) {
	t.Parallel()
	requireEngine(t)
	running, stopTheLab := context.WithCancel(t.Context())
	terminal := &aTerminalStoppingTheLabOnceFiled{stopTheLab: stopTheLab}
	var warned bytes.Buffer

	err := run(running, []string{interactive, aSecondScenarioOfTheLab}, t.TempDir(), terminal, &warned)
	if err != nil {
		t.Fatalf("the Interactive run of %s: %v", aSecondScenarioOfTheLab, err)
	}

	reported := terminal.String()
	reportShows(t, reported, theLiveViewOnLoopback)
	reportShows(t, reported, nodeStopsRespondingReported)
	reportShows(t, reported, threePodsPerZoneJudged)
	reportLacks(t, reported, oneNodeFailsReported)
}
