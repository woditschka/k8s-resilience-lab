//go:build !integration

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/lab"
	"github.com/woditschka/k8s-resilience-lab/internal/live"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
	"github.com/woditschka/k8s-resilience-lab/scenarios"
)

const aValueThatIsNotAVersion = "latest"

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

func TestAValueThatIsNotAVersion_ShouldStopTheRunWithoutReachingTheEngine(t *testing.T) {
	t.Parallel()
	reported, err := aRun(t, "--k8s-version", aValueThatIsNotAVersion)

	if err == nil {
		t.Fatalf("a value that is not a version stops the run, but it reported:\n%s", reported)
	}
	expect.Expect(t, reported).To(expect.Equal(""))
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

// A board of one Zone of one Node holding one Pod is enough to tell its lines
// from the Steps around it.
const (
	someNodePool          = "general"
	someNodeCPU           = scenario.CPU(16000)
	someNodeMemory        = scenario.Memory(65536)
	someNodePods          = 10
	oneZone               = 1
	oneNodePerZone        = 1
	theOnlyNode           = "zone-1-node-1"
	aDeploymentOnTheBoard = "checkout"
	aPodOnTheBoard        = "checkout-5fc87ff7c8-7vd48"
	theWaitStepName       = "all Pods Ready"
	theWhenStepName       = "1 Node fails, zone-1-node-1"
	aSecondThenStepName   = "1 Pod per Node"
	whyNoBoard            = "observe the Placement board: context canceled"
	aStepIndent           = "  "
	aSeparatorRune        = "-"

	// One rune, so an empty line does not pass for a separator.
	aSeparatorOfAnyWidth = 1
)

func aBoardOfOnePod(t *testing.T) scenario.PlacementBoard {
	t.Helper()

	return scenario.NewPlacementBoard(aTopologyOfOneNode(t),
		scenario.NewPlacement(aTopologyOfOneNode(t).Nodes(), []scenario.Pod{thePodOnTheBoard()}), scenario.PlacementBoard{})
}

func thePodOnTheBoard() scenario.Pod {
	return scenario.Pod{Name: aPodOnTheBoard, Deployment: aDeploymentOnTheBoard, Node: theOnlyNode, Phase: scenario.PodRunning}
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

func TestAScenarioReport_ShouldCloseWithOneSkewLineAndTheTimelinesLineAndNoCountTables(t *testing.T) {
	board := aBoardOfOnePod(t)
	waited := aWaitStepWith(board)
	settled := scenario.NewPlacement(aTopologyOfOneNode(t).Nodes(), []scenario.Pod{thePodOnTheBoard()})
	held := anOutcomeOf(waited, aThenStep())
	held.Placement, held.JudgedDeployment = settled, aDeploymentOnTheBoard

	reported := whatTheRunReportsFor(t, held, theReportAlone())

	theSkew := fmt.Sprintf("Skew of %s: %d over Zones, %d over Nodes",
		aDeploymentOnTheBoard, settled.Skew(scenario.SpreadOverZones), settled.Skew(scenario.SpreadOverNodes))
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
		{Elapsed: aNodeTurningReady, Change: scenario.ChangeAdded, Kind: scenario.DeploymentObject, Name: aDeploymentOnTheBoard, Detail: someDeploymentReady},
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
func TestAScenarioReport_ShouldPrintNoSummaryOfEachDeploymentsPods(t *testing.T) {
	held := anOutcomeOf(aWaitStepWith(aBoardOfOnePod(t)), aThenStep())

	reported := whatTheRunReportsFor(t, held, theReportAlone())

	lines := strings.Split(strings.TrimSuffix(reported, "\n"), "\n")
	expect.Expect(t, strings.HasPrefix(lines[len(lines)-1], reportedAsAStep(timelineReported))).To(expect.Equal(true))
}

func everyScenarioNameInOrder(t *testing.T) string {
	t.Helper()

	names, err := lab.ScenarioNames(scenarios.All())
	if err != nil {
		t.Fatalf("derive the name of every Scenario of the lab: %v", err)
	}

	return strings.Join(names, ", ")
}

func TestARunNamingAnUnknownScenario_ShouldStopBeforeProvisioningAndNameTheScenariosThatExist(t *testing.T) {
	cases := map[string][]string{
		"without --interactive": {aNameNoScenarioCarries},
		"with --interactive":    {interactive, aNameNoScenarioCarries},
	}

	for named, arguments := range cases {
		t.Run(named, func(t *testing.T) {
			_, err := aRun(t, arguments...)

			expectNamed(t, err, everyScenarioNameInOrder(t))
		})
	}
}

func TestScenarioNamesARunCannotTake_ShouldStopTheLabBeforeTheEngineNamingTheScenariosThatExist(t *testing.T) {
	refused := map[string][]string{
		"no name with --interactive":            {interactive},
		"two names with --interactive":          {interactive, theFirstScenarioOfTheLab, aSecondScenarioOfTheLab},
		"two names without --interactive":       {theFirstScenarioOfTheLab, aSecondScenarioOfTheLab},
		"a repeated name without --interactive": {theFirstScenarioOfTheLab, theFirstScenarioOfTheLab},
	}

	for named, arguments := range refused {
		t.Run(named, func(t *testing.T) {
			_, err := aRun(t, arguments...)

			expectNamed(t, err, everyScenarioNameInOrder(t))
		})
	}
}

func TestArgumentsNamingOneScenarioWithoutInteractive_ShouldSelectThatScenarioAloneAndHoldNone(t *testing.T) {
	setting, err := resolveSetting([]string{theFirstScenarioOfTheLab}, io.Discard)
	if err != nil {
		t.Fatalf("resolve the setting of a run naming %s: %v", theFirstScenarioOfTheLab, err)
	}

	expect.Expect(t, len(setting.selected)).To(expect.Equal(1))
	expect.Expect(t, setting.held == nil).To(expect.Equal(true))
}

func TestArgumentsNamingOneScenarioWithInteractive_ShouldCarryTheScenarioNameIntoTheSetting(t *testing.T) {
	cases := map[string]struct {
		arguments []string
		want      string
	}{
		"a Scenario held":  {arguments: []string{interactive, theFirstScenarioOfTheLab}, want: theFirstScenarioOfTheLab},
		"no Scenario held": {arguments: nil, want: ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			setting, err := resolveSetting(tc.arguments, io.Discard)
			if err != nil {
				t.Fatalf("resolve the setting of %v: %v", tc.arguments, err)
			}

			expect.Expect(t, setting.heldName).To(expect.Equal(tc.want))
		})
	}
}

const aKindOfFailureTheLabDoesNotInject live.FailureKind = "rack"

const anyNameClicked = "zone-1-node-1"

// The Live view hands over its closed set alone, so a kind beyond it is a
// defect that names itself rather than a click silently dropped.
func TestAClickedFailureOfAKindTheLabDoesNotInject_ShouldBeRefusedNamingTheKind(t *testing.T) {
	notHolding := lab.NewInjector()

	err := injected(notHolding, live.Failure{Kind: aKindOfFailureTheLabDoesNotInject, Name: anyNameClicked})

	expect.Expect(t, err != nil && strings.Contains(err.Error(), string(aKindOfFailureTheLabDoesNotInject))).To(expect.Equal(true))
}

// A run prints its When inside the Scenario's part, so the clicked When is
// held to the line that part gives the same Step.
func TestAClickedWhenFollowedInTheTerminal_ShouldStandIndentedAsARunsWhenDoes(t *testing.T) {
	clicked := lab.Step{Kind: lab.WhenStep, Name: "Node " + anyNameClicked + " stops responding"}
	var asARunPrintsIt reportPart
	writeSteps(&asARunPrintsIt, lab.Outcome{Steps: []lab.Step{clicked}})
	var reported bytes.Buffer

	(&runReport{to: &reported}).printFollowed(clicked.String())

	expect.Expect(t, reported.String()).To(expect.Equal(asARunPrintsIt[0] + "\n"))
}

// A run prints a narrative's line first under its Wait, so a moment followed is
// held to the line the run gives it.
func TestAMomentLineFollowedInTheTerminal_ShouldStandIndentedAsAWaitNarrativesLineDoes(t *testing.T) {
	narrated := aNarrativeOfOnePod()
	var asARunPrintsIt reportPart
	writeBoard(&asARunPrintsIt, lab.Step{Kind: lab.WaitStep, Narrative: narrated})
	var reported bytes.Buffer

	(&runReport{to: &reported}).printFollowed(narrated.Lines()[0])

	expect.Expect(t, reported.String()).To(expect.Equal(asARunPrintsIt[0] + "\n"))
}

const (
	theRepositoryRootFromThisPackage = "../.."
	aScenarioTheLabHolds             = "a-node-that-stops-responding"
	oneMakeInvocationAtMost          = 4 * time.Minute
	pipesAbandonedAfter              = 10 * time.Second
	theLabsScenarioCountRefusal      = "takes one Scenario name"
)

// Variables a gate run exports to its tests would make a nested make dry-run,
// keep going, or name a jobserver it does not hold; SCENARIO and ARGS would
// stand in for the assignments the test gives on the command line.
var variablesAMakeTargetMustNotInherit = []string{"MAKEFLAGS", "MFLAGS", "MAKELEVEL", "SCENARIO", "ARGS"}

func TestTheDemoCommandAskingForAVersion_ShouldHandItToTheInteractiveRunNotReadItAsASecondScenario(t *testing.T) {
	t.Parallel()

	output, err := makeAskingForAVersion(t, "demo")

	expectTheVersionRefusedAlone(t, output, err)
}

func TestTheInteractiveCommandAskingForAVersion_ShouldHandItToTheInteractiveRunNotReadItAsASecondScenario(t *testing.T) {
	t.Parallel()

	output, err := makeAskingForAVersion(t, "interactive")

	expectTheVersionRefusedAlone(t, output, err)
}

func expectTheVersionRefusedAlone(t *testing.T, output string, err error) {
	t.Helper()

	if err == nil {
		t.Errorf("a value that is not a version stops the command, but it reported:\n%s", output)
	}
	if !strings.Contains(output, aValueThatIsNotAVersion) {
		t.Errorf("the refusal names %q, but the command reported:\n%s", aValueThatIsNotAVersion, output)
	}
	reportLacks(t, output, theLabsScenarioCountRefusal)
	reportLacks(t, output, "Live view:")
}

// The real make target runs from the repository root, so the recipe's own
// argument order is what the lab receives.
func makeAskingForAVersion(t *testing.T, target string) (string, error) {
	t.Helper()
	for _, program := range []string{"make", "bash"} {
		if _, err := exec.LookPath(program); err != nil {
			t.Fatalf("the command is run through %s, which is not on PATH: %v", program, err)
		}
	}

	ctx, cancel := context.WithTimeout(t.Context(), oneMakeInvocationAtMost)
	defer cancel()
	command := exec.CommandContext(ctx, "make", target,
		"SCENARIO="+aScenarioTheLabHolds, "ARGS=--k8s-version "+aValueThatIsNotAVersion)
	command.Dir = theRepositoryRootFromThisPackage
	command.Env = slices.DeleteFunc(os.Environ(), func(variable string) bool {
		return slices.ContainsFunc(variablesAMakeTargetMustNotInherit, func(name string) bool {
			return strings.HasPrefix(variable, name+"=")
		})
	})
	command.WaitDelay = pipesAbandonedAfter

	output, err := command.CombinedOutput()

	return string(output), err
}
