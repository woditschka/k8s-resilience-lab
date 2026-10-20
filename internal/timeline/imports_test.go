package timeline

import (
	"strings"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/modulerules"
)

const timelinePackageDir = "."

const everyPackageUnderTimeline = "./..."

// The published line format would become the Kubernetes client's shape the
// moment one of its types reached this package.
const deniedKubernetesImport = "k8s.io"

func TestTheTimelinePackage_ShouldImportNoKubernetesPackage(t *testing.T) {
	offenders, err := modulerules.OffendingImporters(t.Context(), timelinePackageDir, everyPackageUnderTimeline,
		modulerules.EveryPackage, []string{deniedKubernetesImport})
	if err != nil {
		t.Fatalf("walk the package graph under internal/timeline: %v", err)
	}

	if len(offenders) > 0 {
		t.Errorf("no package under internal/timeline may import a Kubernetes package, but:\n%s", strings.Join(offenders, "\n"))
	}
}
