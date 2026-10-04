package cluster

import (
	"fmt"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// controlPlaneTimeout is the budget one control-plane request runs within. The
// control plane is a container on this machine and the lab opens no watch, so a
// request still unanswered after this has wedged. Without a deadline of its own
// such a request holds up the Scenario indefinitely, and the removal that
// Scenario defers never runs — which leaves an unauthenticated API server on the
// machine (docs/system-design.md § Threat Model).
const controlPlaneTimeout = 30 * time.Second

// controlPlaneConfig is the one configuration the lab builds for a control plane:
// the loopback address the engine published for the cluster this run provisioned,
// the budget a request runs within, and nothing else.
//
// No credential, kubeconfig, exec provider or certificate file is set, and none
// is discovered — the lab drives only the Simulated Cluster it created itself,
// per docs/system-design.md § Threat Model.
func controlPlaneConfig(address string) *rest.Config {
	return &rest.Config{Host: address, Timeout: controlPlaneTimeout}
}

// newControlPlaneClient is the one construction path to a Simulated Cluster's
// control plane.
func newControlPlaneClient(address string) (*kubernetes.Clientset, error) {
	client, err := kubernetes.NewForConfig(controlPlaneConfig(address))
	if err != nil {
		return nil, fmt.Errorf("build a client for the control plane at %s: %w", address, err)
	}

	return client, nil
}
