package timeline

import (
	"strings"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/modulerules"
)

// timelinePackageDir is where go test runs this suite: the package's own source
// directory, so the pattern below covers this module and nothing above it.
const timelinePackageDir = "."

// everyPackageUnderTimeline scopes the rule to this module's own packages, which
// is the subtree the layering rule governs.
const everyPackageUnderTimeline = "./..."

// deniedKubernetesImport is the prefix of every Kubernetes package. Where a
// Timeline is kept is the lab's own concern: the published line format would
// become the Kubernetes client's shape the moment one of its types reached here
// (docs/system-design.md § Package Structure).
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
