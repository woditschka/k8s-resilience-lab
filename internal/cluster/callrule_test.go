package cluster

import (
	"strings"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/modulerules"
)

// inClusterConfigPackage and inClusterConfigCall name the call that would build a
// client from a service-account token mounted in a Pod, which is the one way the
// lab could reach a cluster it did not provision itself
// (docs/system-design.md § Threat Model).
const (
	inClusterConfigPackage = "k8s.io/client-go/rest"
	inClusterConfigCall    = "InClusterConfig"
)

// The .golangci.yml forbidigo rule denies the same call. This test asserts it
// from the module's own files, so a lint configuration that is scoped wrongly or
// never runs cannot make the rule pass silently
// (docs/system-design.md § Package Structure).
func TestNoFileInTheModule_ShouldCallInClusterConfiguration(t *testing.T) {
	offenders, err := modulerules.CallsTo(moduleRoot(t), inClusterConfigPackage, inClusterConfigCall)
	if err != nil {
		t.Fatalf("scan the module's own Go files: %v", err)
	}

	if len(offenders) > 0 {
		t.Errorf("no file may build a client from an in-cluster service account, but:\n%s", strings.Join(offenders, "\n"))
	}
}
