package cluster

import (
	"testing"

	"k8s.io/client-go/rest"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
)

// Only the fact that the client carries this address matters here.
const provisionedAddress = "http://127.0.0.1:38041"

// The whole configuration is compared, so a future credential, kubeconfig or
// exec-provider field fails this test by being there at all.
func TestTheControlPlaneClient_ShouldTargetTheProvisionedAddressUnderADeadlineAndCarryNoCredential(t *testing.T) {
	configured := controlPlaneConfig(provisionedAddress)

	expect.Expect(t, configured).To(expect.Equal(&rest.Config{
		Host:    provisionedAddress,
		Timeout: controlPlaneTimeout,
	}))
}
