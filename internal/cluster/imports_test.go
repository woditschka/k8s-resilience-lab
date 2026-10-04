package cluster

import (
	"strings"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/modulerules"
)

// ownPackageDir is where go test runs this suite: the package's own source
// directory, which the module root is located from.
const ownPackageDir = "."

// everyPackageInModule is resolved against the module root, so a module-wide rule
// is asserted over every package rather than over this one subtree.
const everyPackageInModule = "./..."

// deniedCredentialImports are the import paths through which a package could reach
// a cluster the lab did not provision itself: a kubeconfig, the home directory one
// is discovered in, or a credential plugin that authenticates against a real
// cluster. The lab builds its client from the address it provisioned, with no
// credential at all (docs/system-design.md § Threat Model).
var deniedCredentialImports = []string{
	"k8s.io/client-go/tools/clientcmd",
	"k8s.io/client-go/util/homedir",
	"k8s.io/client-go/plugin/pkg/client/auth",
}

// testSupportPackage is the package the module-rule tests share their helpers
// through. Test files alone may import it, and this rule is what keeps it so
// without a linter rule of its own
// (docs/adr/2026-10-04-module-rule-test-support.md).
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

// offendingImporters is every package of this module, in scope, that imports one
// of the denied paths. The report is built from the package graph rather than
// from the lint configuration, so a depguard rule that is scoped wrongly or never
// runs cannot make a rule pass silently.
func offendingImporters(t *testing.T, scope modulerules.Scope, denied []string) []string {
	t.Helper()

	offenders, err := modulerules.OffendingImporters(t.Context(), moduleRoot(t), everyPackageInModule, scope, denied)
	if err != nil {
		t.Fatalf("walk the package graph of the module: %v", err)
	}

	return offenders
}

// moduleRoot is the directory holding the module file, so a pattern resolved
// against it covers the whole module.
func moduleRoot(t *testing.T) string {
	t.Helper()

	root, err := modulerules.Root(t.Context(), ownPackageDir)
	if err != nil {
		t.Fatalf("locate the root of the module: %v", err)
	}

	return root
}
