package scenario

import (
	"strings"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/modulerules"
)

// scenarioPackageDir is where go test runs this suite: the package's own source
// directory, so the pattern below covers this module and nothing above it.
const scenarioPackageDir = "."

// everyPackageUnderScenario scopes the rule to this module's own packages, which
// is the subtree the layering rule governs.
const everyPackageUnderScenario = "./..."

// deniedKubernetesImport is the prefix of every Kubernetes package. A run's
// description and its observed outcome are immutable values that name no
// Kubernetes type; the mapping between the two vocabularies lives in
// internal/cluster (docs/system-design.md § Package Structure).
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
