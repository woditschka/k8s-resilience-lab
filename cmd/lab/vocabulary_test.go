// The tests needing a container engine and those needing none share these
// names, so this file carries neither kind's build constraint.

package main

import (
	"bytes"
	"strings"
	"testing"
)

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

	resilientPlacementReported = aScenarioHeading + "a-deployment-placed-for-resilience"
	threePodsPerZoneJudged     = "Then 3 Pods per Zone"
	onePodPerNodeJudged        = "Then 1 Pod per Node"
	aFindingReported           = "Finding"

	// The Step's name is derived from the predefined Pod specification and the
	// Startup time its definition names, so a run is where the definition and
	// the line meet.
	resilientPlacementDeclared = "Given a Deployment checkout of 9 replicas declaring Resilient placement, " +
		"each Pod starting in 500ms to 1s"
)

// aRun files its Timelines under the test's own directory, so a suite run
// leaves nothing in the repository.
func aRun(t *testing.T, arguments ...string) (string, error) {
	t.Helper()

	var reported, warned bytes.Buffer
	err := run(t.Context(), arguments, t.TempDir(), &reported, &warned)

	return reported.String(), err
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

const (
	aHeavierSeparatorRune      = "="
	oneAboveEachOfTwoScenarios = 2
	oneAboveTheOnlyScenario    = 1
)

// These Outcomes are built rather than run, because a Scenario whose Steps fail
// part-way cannot be written as a Scenario source.
const (
	theScenarioReported = "a-deployment-placed-for-resilience"
	itsCluster          = "k8s-resilience-lab-a-deployment-placed-for-resilience-7f3a9c"
	theThenStepName     = "3 Pods per Zone"
	aTopologyStepName   = "a Topology of 3 Zones of 3 Nodes"

	whatEndedTheScenario      = "apply " + aTopologyStepName + ": create Node zone-1-node-1: already exists"
	whyTheTimelineWasNotFiled = "create the Timeline folder /read-only/timelines: permission denied"
	someTimelinePath          = "/tmp/a-run/timelines/k8s-resilience-lab-a-deployment-placed-for-resilience-7f3a9c.jsonl"

	theScenarioNotStarted    = "a-node-that-stops-responding"
	whyTheClusterDidNotStart = "the Scenario " + theScenarioNotStarted + ": start the Simulated Cluster: port already allocated"
)

const (
	onACluster       = ", on "
	timelineReported = "Timeline, "
	entriesReported  = " entries"
	filedAt          = ", filed at "
	filedNowhere     = ", filed nowhere: "
)

func expectNamed(t *testing.T, err error, named string) {
	t.Helper()

	if err == nil {
		t.Fatalf("the run reports an error naming %s, but reports none", named)
	}
	if !strings.Contains(err.Error(), named) {
		t.Errorf("the error names %s, but reads: %v", named, err)
	}
}

const (
	aSkewReported = "Skew of "
	// A Then reads "Pods per Zone" too, so a count table is known by its
	// heading's opening.
	aZoneTableHead = "  Pods per Zone ("
	aNodeTableHead = "  Pods per Node ("
)

const (
	interactive              = "--interactive"
	aNameNoScenarioCarries   = "a-condition-no-scenario-states"
	theFirstScenarioOfTheLab = "a-deployment-placed-for-resilience"
	aSecondScenarioOfTheLab  = "a-node-that-stops-responding"
)
