package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/cluster"
	"github.com/woditschka/k8s-resilience-lab/internal/expect"
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

// aRun runs the lab with the arguments given and returns what it reported.
func aRun(t *testing.T, arguments ...string) (string, error) {
	t.Helper()

	var reported, warned bytes.Buffer
	err := run(t.Context(), arguments, &reported, &warned)

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
