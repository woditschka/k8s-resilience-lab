package main

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/cluster"
	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/lab"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// aVersionWithNoClusterImage is a Kubernetes version the lab cannot obtain: it is
// a well-formed version, so it passes validation and fails at the pull, which is
// the path edge case 4 describes.
const aVersionWithNoClusterImage = "1.0.0"

// aValueThatIsNotAVersion is refused by validation, before the run reaches the
// engine at all.
const aValueThatIsNotAVersion = "latest"

// versionReported and placementSettled are the lines of the run's report these
// tests read it by. The report is the whole surface a user of the lab sees, so a
// run is asserted through it rather than through anything the run holds inside.
const (
	versionReported  = "Kubernetes version: "
	placementSettled = "Placement settled"
)

// The lines a run of the lab's own Scenarios is read by: the Resilient placement
// Scenario's heading under the run name derived for it, the two Then Steps its
// Placement has to meet, and what a Finding is reported under.
const (
	// aScenarioHeading opens each Scenario's own part of a report, which is the
	// boundary a Timeline's count and its printed entries are read inside.
	aScenarioHeading = "Scenario "

	resilientPlacementReported = aScenarioHeading + "resilient-placement"
	threePodsPerZoneJudged     = "Then 3 Pods per Zone"
	onePodPerNodeJudged        = "Then 1 Pod per Node"
	aFindingReported           = "Finding"

	// resilientPlacementDeclared is the Workload's own Given line, which names the
	// predefined Pod specification its Pods are deployed with: the Step's name is
	// derived from that specification's name, so a run is where the two meet.
	resilientPlacementDeclared = "Given a Workload checkout of 9 Pods declaring Resilient placement"
)

// The two Wait lines every Scenario of the lab reports, read by the prefix each
// opens with: a Wait's line names how long it held, and how long that is belongs
// to the run rather than to what a Scenario asserts.
const (
	allNodesReadyAwaited = "Wait for all Nodes Ready in "
	allPodsReadyAwaited  = "Wait for all Pods Ready in "
)

// The lines both Failure Scenarios share: the Given line above renamed by the
// fewest Pods their Workload must keep Running through a Failure, and the opening
// of the Then that judges that minimum. A declaration renames the line it is
// written on, so the renamed Given is the one such a run reports, and the Then
// names the minimum before the Pods it counted at each moment.
const (
	theRequiredCapacityDeclared = resilientPlacementDeclared + ", keeping at least 6 Pods Running"
	theCapacityJudged           = "Then the Required Capacity held: a minimum of 6 Pods, "
)

// The lines a run of the Node stops responding Scenario is read by: its heading
// under the run name derived for it, the When Step naming the Node the lab
// picked, the Then that judges what the Failure left behind, and the counts its
// capacity judgement states — eight Pods left Running by a Node holding one,
// and the ninth placed again once the Placement has settled.
//
// theNodePickedToFail is the first Node the Scenario's Topology derives, which is
// the pick a run has to report and has to repeat on every run.
const (
	nodeStopsRespondingReported = aScenarioHeading + "node-stops-responding"
	theNodePickedToFail         = "zone-1-node-1"
	oneNodeFailsReported        = "When 1 Node fails, " + theNodePickedToFail
	noPodOnTheFailedNodesJudged = "Then no Pod on the Nodes that failed"

	capacityHeldWhenTheNodeStoppedJudged = theCapacityJudged + "8 at the moment of the Failure and 9 once Settled"
)

// The lines a run of the Zone stops responding Scenario is read by, in the same
// parts: its heading, the When Step naming the Zone the lab picked, the Then
// that judges what the Failure left in it, and the counts its capacity
// judgement states — a lost Zone takes three of the nine Pods, so exactly the
// minimum is left Running at that moment and all nine once the Placement has
// settled.
//
// theZonePickedToFail is the first Zone the Scenario's Topology derives, which is
// the pick a run has to report and has to repeat on every run.
const (
	zoneStopsRespondingReported = aScenarioHeading + "zone-stops-responding"
	theZonePickedToFail         = "zone-1"
	oneZoneFailsReported        = "When 1 Zone fails, " + theZonePickedToFail
	noPodInTheFailedZonesJudged = "Then no Pod in the Zones that failed"

	capacityHeldWhenTheZoneStoppedJudged = theCapacityJudged + "6 at the moment of the Failure and 9 once Settled"
)

// The labels a run opens each of its control-plane settings with. What such a
// line has to carry is the pair the declaring package holds — the value the lab
// sets and the default it replaces — rather than a wording this suite repeats.
const (
	unresponsiveNodeSetting = "unresponsive Node:"
	nodeHeartbeatSetting    = "Node heartbeats:"
	nodeEvictionSetting     = "Node eviction rate:"

	// The lab's own waiting time is reported beside them under a label of its own,
	// because it is the longest the lab itself waits rather than a setting it makes
	// on the control plane.
	labWaitingTime = "lab waiting time:"
)

// wholeTimelineAsked is the argument an engineer asks a run for every entry of
// its Timelines with, and noEntryPrinted is how many entry lines a run that was
// not asked prints.
const (
	wholeTimelineAsked = "--whole-timeline"
	noEntryPrinted     = 0
)

// requireEngine is the one guard every test that needs a container engine goes
// through; on a machine where none answers the calling test fails here, naming
// why, and it fails on its own, so every test that needs no engine still runs
// (docs/testing-principles.md § Mocking Policy).
func requireEngine(t *testing.T) {
	t.Helper()

	if _, err := cluster.DetectEngine(t.Context(), slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("no container engine answers on this machine, so the lab cannot run: %v", err)
	}
}

// aRun runs the lab with the arguments given and returns what it reported. It
// files its Timelines under a directory of the test's own, so a suite run leaves
// nothing in the repository.
func aRun(t *testing.T, arguments ...string) (string, error) {
	t.Helper()

	var reported, warned bytes.Buffer
	err := run(t.Context(), arguments, t.TempDir(), &reported, &warned)

	return reported.String(), err
}

// reportShows and reportLacks are how a run's report is read: a line that is
// missing, or one that should not be there, is named against the whole report,
// since the report is what a user would be looking at.
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

// theScenarioSectionsOf are the parts of a report that are one Scenario each. A
// run carries out every Scenario of the lab and reports each under its own
// heading, each with a Timeline of its own; a reported entry count and the
// entries printed under it are therefore only comparable inside one section.
func theScenarioSectionsOf(t *testing.T, reported string) []string {
	t.Helper()

	sections := strings.Split(reported, "\n"+aScenarioHeading)
	if len(sections) < 2 {
		t.Fatalf("a run reports each Scenario it carried out under its own heading, but reported:\n%s", reported)
	}

	return sections[1:]
}

// theScenarioNamedIn is the run name the heading of one such section carries,
// which is what names the case reading it.
func theScenarioNamedIn(section string) string {
	named, _, _ := strings.Cut(section, ",")

	return named
}

// reportLineNames asserts the line a run opens with label names every one of
// these values. A setting is read this way rather than as a whole line, because
// what the run owes a reader is the pair of values and not the wording around
// them.
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

// theLineOpeningWith is the reported line that starts with label, and empty when
// the report holds none.
func theLineOpeningWith(reported, label string) string {
	for line := range strings.SplitSeq(reported, "\n") {
		if strings.HasPrefix(line, label) {
			return line
		}
	}

	return ""
}

// This is where the lab's own Scenarios run: on a Simulated Cluster the run
// provisions for each of them, which is what makes the Placement the control
// plane's own doing. A Scenario has no test of its own — it is itself the check —
// so the command's suite is what fails when one reports a Finding or ends at a
// Step (docs/adr/2026-10-03-scenarios-as-go-functions.md).
func TestResilientPlacementWrittenAsSteps_ShouldPlaceThreePodsPerZoneAndOnePerNode(t *testing.T) {
	requireEngine(t)

	reported, err := aRun(t)
	if err != nil {
		t.Fatalf("a Scenario of the lab reported a Finding or ended at a Step: %v", err)
	}

	reportShows(t, reported, resilientPlacementReported)
	reportShows(t, reported, resilientPlacementDeclared)
	reportShows(t, reported, allNodesReadyAwaited)
	reportShows(t, reported, allPodsReadyAwaited)
	reportShows(t, reported, threePodsPerZoneJudged)
	reportShows(t, reported, onePodPerNodeJudged)
	reportShows(t, reported, placementSettled)
	reportLacks(t, reported, aFindingReported)
}

// The Node stops responding Scenario runs in the same suite and for the same
// reason: the Placement it judges after the Failure is the control plane's own
// doing, and the Scenario is itself the check. Its When Step is read too,
// because the Node the lab picked is something only the run can report.
func TestANodeThatStopsResponding_ShouldSettleAtThreePodsPerZoneWithNoneOnTheFailedNode(t *testing.T) {
	requireEngine(t)

	reported, err := aRun(t)
	if err != nil {
		t.Fatalf("a Scenario of the lab reported a Finding or ended at a Step: %v", err)
	}

	reportShows(t, reported, nodeStopsRespondingReported)
	reportShows(t, reported, theRequiredCapacityDeclared)
	reportShows(t, reported, allNodesReadyAwaited)
	reportShows(t, reported, allPodsReadyAwaited)
	reportShows(t, reported, oneNodeFailsReported)
	reportShows(t, reported, noPodOnTheFailedNodesJudged)
	reportShows(t, reported, threePodsPerZoneJudged)
	reportShows(t, reported, capacityHeldWhenTheNodeStoppedJudged)
	reportLacks(t, reported, aFindingReported)
}

// The Zone stops responding Scenario runs in the same suite for the same reason,
// and the Zone the lab picked is read out of its When Step: the Scenario names
// no Zone, so which one stopped is something only the run can report. The count
// of Pods per Zone is not read after the Failure — two Zones carry nine Pods,
// so the Placement is skewed and the Scenario judges the capacity alone, by
// hand and against the minimum its Workload declares (docs/prd.md REQ-SIM-008,
// REQ-SIM-010).
func TestARunOfAZoneThatStopsResponding_ShouldReportTheZoneItPickedToFail(t *testing.T) {
	requireEngine(t)

	reported, err := aRun(t)
	if err != nil {
		t.Fatalf("a Scenario of the lab reported a Finding or ended at a Step: %v", err)
	}

	reportShows(t, reported, zoneStopsRespondingReported)
	reportShows(t, reported, theRequiredCapacityDeclared)
	reportShows(t, reported, allNodesReadyAwaited)
	reportShows(t, reported, allPodsReadyAwaited)
	reportShows(t, reported, oneZoneFailsReported)
	reportShows(t, reported, noPodInTheFailedZonesJudged)
	reportShows(t, reported, capacityHeldWhenTheZoneStoppedJudged)
	reportLacks(t, reported, aFindingReported)
}

// Every control-plane setting the lab makes is reported against the default it
// replaces, so a reader of a run can tell which of the timings it watched are
// the lab's doing and which are Kubernetes' own. The lab's own waiting time is
// reported beside them, because a time a Wait states is refused against it and an
// engineer writing one cannot otherwise see the number. One run is read for all
// four, because each of them holds for the whole run.
func TestARun_ShouldReportTheSettingsItMakesAgainstTheirDefaultsAndItsOwnWaitingTime(t *testing.T) {
	requireEngine(t)

	reported, err := aRun(t)
	if err != nil {
		t.Fatalf("a Scenario of the lab reported a Finding or ended at a Step: %v", err)
	}

	reportLineNames(t, reported, unresponsiveNodeSetting,
		scenario.UnresponsiveNodeWait.String(), scenario.DefaultUnresponsiveNodeWait.String())
	reportLineNames(t, reported, nodeHeartbeatSetting,
		cluster.NodeHeartbeatFactor, cluster.DefaultNodeHeartbeatFactor)
	reportLineNames(t, reported, nodeEvictionSetting,
		cluster.NodeEvictionRate, cluster.DefaultNodeEvictionRate)
	reportLineNames(t, reported, labWaitingTime, scenario.WaitingTime.String())
}

func TestARunAskedForTheWholeTimeline_ShouldPrintEveryEntryInTheOrderItHappened(t *testing.T) {
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
	requireEngine(t)

	reported, err := aRun(t)
	if err != nil {
		t.Fatalf("a Scenario of the lab reported a Finding or ended at a Step: %v", err)
	}

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
	requireEngine(t)

	// Every Simulated Cluster a run provisions is removed before the run returns,
	// so by the time the file below is opened the cluster it describes is gone.
	reported, err := aRun(t)
	if err != nil {
		t.Fatalf("a Scenario of the lab reported a Finding or ended at a Step: %v", err)
	}

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
	requireEngine(t)
	pinned := cluster.DefaultKubernetesVersion().String()

	reported, err := aRun(t)
	if err != nil {
		t.Fatalf("run the lab with no version named: %v", err)
	}

	reportShows(t, reported, versionReported+pinned)
}

func TestARunNamingAVersion_ShouldMirrorAndReportThatVersion(t *testing.T) {
	requireEngine(t)
	named := cluster.DefaultKubernetesVersion().String()
	// The version is named without its leading v, so the report also shows that
	// the run mirrors the validated version rather than the string it was given.
	asTheRunNamesIt := strings.TrimPrefix(named, "v")

	reported, err := aRun(t, "--k8s-version", asTheRunNamesIt)
	if err != nil {
		t.Fatalf("run the lab naming the version %s: %v", asTheRunNamesIt, err)
	}

	reportShows(t, reported, versionReported+named)
	reportShows(t, reported, placementSettled)
}

func TestAVersionTheLabCannotObtain_ShouldStopTheRunBeforeAnyScenario(t *testing.T) {
	requireEngine(t)

	reported, err := aRun(t, "--k8s-version", aVersionWithNoClusterImage)

	if err == nil {
		t.Fatalf("a version the lab cannot obtain stops the run, but it reported:\n%s", reported)
	}
	if !strings.Contains(err.Error(), aVersionWithNoClusterImage) {
		t.Errorf("the run names the version it could not obtain, but reads: %v", err)
	}
	reportLacks(t, reported, placementSettled)
}

func TestAValueThatIsNotAVersion_ShouldStopTheRunWithoutReachingTheEngine(t *testing.T) {
	reported, err := aRun(t, "--k8s-version", aValueThatIsNotAVersion)

	if err == nil {
		t.Fatalf("a value that is not a version stops the run, but it reported:\n%s", reported)
	}
	expect.Expect(t, reported).To(expect.Equal(""))
}

// The Outcomes the two tests above report are built here rather than run,
// because a Scenario whose Steps fail part-way cannot be written as a Scenario
// source: the Scenarios' place holds the lab's own experiments, which pass.
const (
	theScenarioReported = "resilient-placement"
	itsCluster          = "k8s-resilience-lab-resilient-placement-7f3a9c"
	theThenStepName     = "3 Pods per Zone"
	aTopologyStepName   = "a Topology of 3 Zones of 3 Nodes"

	whatEndedTheScenario      = "apply " + aTopologyStepName + ": create Node zone-1-node-1: already exists"
	whyTheTimelineWasNotFiled = "create the Timeline folder /read-only/timelines: permission denied"
	someTimelinePath          = "/tmp/a-run/timelines/k8s-resilience-lab-resilient-placement-7f3a9c.jsonl"
)

// The changes the Timelines above hold: a Node the Topology brought up, and the
// Placement the control plane reported for a Pod.
const (
	whenTheNodeWasAdded    = 120 * time.Millisecond
	theNodeAdded           = "zone-1-node-1"
	theNodeReady           = "Ready"
	theNodeCapacity        = "10 Pods"
	whenThePodWasScheduled = 3470 * time.Millisecond
	thePodReported         = "web-5d8f9b7c6-2xk4m"
	thePodScheduled        = "Scheduled"
	whereThePodWasPlaced   = "Successfully assigned default/web-5d8f9b7c6-2xk4m to zone-1-node-1"
)

// theScenarioRecorded is what the header of a filed Timeline names its Scenario
// in, which is how a file read back is recognised as the right one.
const theScenarioRecorded = `"scenario":"` + theScenarioReported + `"`

// What a Timeline is read out of a report by: the line it is reported under, and
// where that line names the file it was filed at.
const (
	timelineReported = "Timeline, "
	entriesReported  = " entries"
	filedAt          = ", filed at "
)

// aTopologyStep and aThenStep are the Steps the built Outcomes report, named the
// way the Resilient placement Scenario names its own.
func aTopologyStep() lab.Step {
	return lab.Step{Kind: lab.GivenStep, Name: aTopologyStepName}
}

func aThenStep() lab.Step {
	return lab.Step{Kind: lab.ThenStep, Name: theThenStepName}
}

// aTimelineOfTwoChanges is the record the built Outcomes carry: as much of a
// Timeline as a report needs to be read by.
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

// whatTheRunReportsFor is what a run asked for the setting given writes about
// one Scenario's Outcome, which is the surface a user reads a Timeline from.
func whatTheRunReportsFor(t *testing.T, outcome lab.Outcome, asked runSetting) string {
	t.Helper()

	var reported bytes.Buffer
	writeOutcome(&runReport{to: &reported}, outcome, asked)

	return reported.String()
}

// theSummaryAlone is what a run was asked to print when it was given no
// argument about its Timelines. It names no Kubernetes version, because no
// report reads one.
func theSummaryAlone() runSetting {
	return runSetting{}
}

// reportShowsTheSummary asserts every line of a Timeline's summary is in the
// report, in the form the summary itself reads as. What those lines say is
// internal/scenario's own to assert; this is that the run printed them.
func reportShowsTheSummary(t *testing.T, reported string, recorded scenario.Timeline) {
	t.Helper()

	for _, line := range recorded.Summarise().Lines() {
		reportShows(t, reported, line)
	}
}

// theEntriesPrintedIn are the times the entries of a printed Timeline were
// received, read out of the report line by line, because the report is the whole
// surface a user sees.
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

// theEntryCountReportedIn is how many entries the run says the Timeline holds,
// which every printed and every filed entry is counted against.
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

// theTimelineFiledBy are the lines of the file a run says it filed its Timeline
// at. Taking the path out of the report is what asserts the run names where the
// record went, and reading it afterwards is what asserts the record outlives the
// Simulated Cluster it describes.
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

// expectNamed asserts an error names what a reader needs to act on it.
func expectNamed(t *testing.T, err error, named string) {
	t.Helper()

	if err == nil {
		t.Fatalf("the run reports an error naming %s, but reports none", named)
	}
	if !strings.Contains(err.Error(), named) {
		t.Errorf("the error names %s, but reads: %v", named, err)
	}
}
