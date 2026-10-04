package cluster

import (
	"testing"

	"k8s.io/client-go/rest"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
)

// provisionedAddress is the loopback address an engine publishes a Simulated
// Cluster's API server on. Only the fact that the client carries it matters here.
const provisionedAddress = "http://127.0.0.1:38041"

// The whole configuration is compared rather than its credential fields one by
// one: a field this comparison does not mention is a field the lab must not set,
// so a future credential, kubeconfig or exec-provider field fails this test by
// being there at all (docs/system-design.md § Threat Model). The deadline is read
// from the constant the lab sets it from, since what matters here is that a
// request cannot run without one.
func TestTheControlPlaneClient_ShouldTargetTheProvisionedAddressUnderADeadlineAndCarryNoCredential(t *testing.T) {
	configured := controlPlaneConfig(provisionedAddress)

	expect.Expect(t, configured).To(expect.Equal(&rest.Config{
		Host:    provisionedAddress,
		Timeout: controlPlaneTimeout,
	}))
}
