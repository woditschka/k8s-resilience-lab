package scenario

import (
	"strings"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/modulerules"
)

const scenarioPackageDir = "."

const everyPackageUnderScenario = "./..."

// Kubernetes types stay in internal/cluster, which maps between the two
// vocabularies.
const deniedKubernetesImport = "k8s.io"

func TestTheScenarioPackage_ShouldImportNoKubernetesPackage(t *testing.T) {
	offenders, err := modulerules.OffendingImporters(t.Context(), scenarioPackageDir, everyPackageUnderScenario,
		modulerules.EveryPackage, []string{deniedKubernetesImport})
	if err != nil {
		t.Fatalf("walk the package graph under internal/scenario: %v", err)
	}

	if len(offenders) > 0 {
		t.Errorf("no package under internal/scenario may import a Kubernetes package, but:\n%s", strings.Join(offenders, "\n"))
	}
}

// The Live view shows the lab's own Placement board, so it reaches nothing that
// drives a Simulated Cluster.
const everyPackageUnderTheLiveView = "../live/..."

var whatDrivesACluster = []string{
	deniedKubernetesImport,
	"github.com/woditschka/k8s-resilience-lab/internal/cluster",
	"github.com/woditschka/k8s-resilience-lab/internal/lab",
	"github.com/woditschka/k8s-resilience-lab/internal/timeline",
}

// The command alone composes the Live view, so the server stays out of every
// package a run or a Scenario reaches.
const (
	everyPackageUnderInternal  = "../../internal/..."
	everyPackageUnderScenarios = "../../scenarios/..."
)

var theLiveView = []string{"github.com/woditschka/k8s-resilience-lab/internal/live"}

func TestTheLiveView_ShouldImportNothingThatDrivesASimulatedCluster(t *testing.T) {
	offenders, err := modulerules.OffendingImporters(t.Context(), scenarioPackageDir, everyPackageUnderTheLiveView,
		modulerules.EveryPackage, whatDrivesACluster)
	if err != nil {
		t.Fatalf("walk the package graph under internal/live: %v", err)
	}

	if len(offenders) > 0 {
		t.Errorf("internal/live imports internal/scenario and the standard library alone, but:\n%s", strings.Join(offenders, "\n"))
	}
}

func TestNoPackageButTheCommand_ShouldImportTheLiveView(t *testing.T) {
	underInternal, err := modulerules.OffendingImporters(t.Context(), scenarioPackageDir, everyPackageUnderInternal,
		modulerules.ProductionPackagesOnly, theLiveView)
	if err != nil {
		t.Fatalf("walk the package graph under internal: %v", err)
	}
	underScenarios, err := modulerules.OffendingImporters(t.Context(), scenarioPackageDir, everyPackageUnderScenarios,
		modulerules.ProductionPackagesOnly, theLiveView)
	if err != nil {
		t.Fatalf("walk the package graph under scenarios: %v", err)
	}

	if offenders := append(underInternal, underScenarios...); len(offenders) > 0 {
		t.Errorf("cmd/lab alone composes the Live view, but:\n%s", strings.Join(offenders, "\n"))
	}
}
