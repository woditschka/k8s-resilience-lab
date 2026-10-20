package cluster

import (
	"strings"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/modulerules"
)

// A service-account token mounted in a Pod is the one way the lab could reach a
// cluster it did not provision itself.
const (
	inClusterConfigPackage = "k8s.io/client-go/rest"
	inClusterConfigCall    = "InClusterConfig"
)

// The forbidigo rule denies the same call; asserting it from the module's own
// files means a lint configuration scoped wrongly or never run cannot make the
// rule pass silently.
func TestNoFileInTheModule_ShouldCallInClusterConfiguration(t *testing.T) {
	offenders, err := modulerules.CallsTo(moduleRoot(t), inClusterConfigPackage, inClusterConfigCall)
	if err != nil {
		t.Fatalf("scan the module's own Go files: %v", err)
	}

	if len(offenders) > 0 {
		t.Errorf("no file may build a client from an in-cluster service account, but:\n%s", strings.Join(offenders, "\n"))
	}
}
