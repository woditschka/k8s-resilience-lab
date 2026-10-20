package cluster

import (
	"fmt"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// The lab opens no watch on this client, so a request still unanswered after
// this has wedged. Without a deadline it would hold up the Scenario
// indefinitely and the deferred removal would never run, leaving an
// unauthenticated API server on the machine.
const controlPlaneTimeout = 30 * time.Second

// controlPlaneConfig sets no credential, kubeconfig, exec provider or
// certificate file and discovers none: the lab drives only the Simulated
// Cluster it created itself.
func controlPlaneConfig(address string) *rest.Config {
	return &rest.Config{Host: address, Timeout: controlPlaneTimeout}
}

func newControlPlaneClient(address string) (*kubernetes.Clientset, error) {
	client, err := kubernetes.NewForConfig(controlPlaneConfig(address))
	if err != nil {
		return nil, fmt.Errorf("build a client for the control plane at %s: %w", address, err)
	}

	return client, nil
}
