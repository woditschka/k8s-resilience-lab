package cluster

import (
	"strings"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/modulerules"
)

const ownPackageDir = "."

// everyPackageInModule is resolved against the module root, so a module-wide
// rule covers every package rather than this subtree.
const everyPackageInModule = "./..."

// Each of these could reach a cluster the lab did not provision itself.
var deniedCredentialImports = []string{
	"k8s.io/client-go/tools/clientcmd",
	"k8s.io/client-go/util/homedir",
	"k8s.io/client-go/plugin/pkg/client/auth",
}

// Only test files may import the test-support package, and this rule keeps it
// so without a linter rule of its own.
var testSupportPackage = []string{"github.com/woditschka/k8s-resilience-lab/internal/modulerules"}

func TestNoPackage_ShouldDiscoverClusterCredentials(t *testing.T) {
	offenders := offendingImporters(t, modulerules.EveryPackage, deniedCredentialImports)

	if len(offenders) > 0 {
		t.Errorf("no package may discover cluster credentials, but:\n%s", strings.Join(offenders, "\n"))
	}
}

func TestNoProductionPackage_ShouldImportTheTestSupportPackage(t *testing.T) {
	offenders := offendingImporters(t, modulerules.ProductionPackagesOnly, testSupportPackage)

	if len(offenders) > 0 {
		t.Errorf("the module-rule helpers are for test files alone, but:\n%s", strings.Join(offenders, "\n"))
	}
}

// The report is built from the package graph rather than the lint
// configuration, so a depguard rule scoped wrongly or never run cannot make a
// rule pass silently.
func offendingImporters(t *testing.T, scope modulerules.Scope, denied []string) []string {
	t.Helper()

	offenders, err := modulerules.OffendingImporters(t.Context(), moduleRoot(t), everyPackageInModule, scope, denied)
	if err != nil {
		t.Fatalf("walk the package graph of the module: %v", err)
	}

	return offenders
}

func moduleRoot(t *testing.T) string {
	t.Helper()

	root, err := modulerules.Root(t.Context(), ownPackageDir)
	if err != nil {
		t.Fatalf("locate the root of the module: %v", err)
	}

	return root
}
