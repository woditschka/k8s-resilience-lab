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
