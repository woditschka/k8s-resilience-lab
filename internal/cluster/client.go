package cluster

import (
	"fmt"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// controlPlaneTimeout bounds every control-plane request, a watch's opening
// included, so a request that wedges cannot hold up the Scenario and its
// deferred removal, which would leave an unauthenticated API server on the
// machine. A watch's stream is not a request that wedges: it ends with its
// recording, which Close stops before removing the container.
const controlPlaneTimeout = 30 * time.Second

// noClientSideRateLimit is how client-go reads a negative QPS. Its default of
// five requests a second spaces a Zone's Nodes apart by most of a second each,
// so a large Zone would stop responding Node by Node rather than at once; the
// API server is the lab's own, and its limits stand.
const noClientSideRateLimit float32 = -1

// controlPlaneConfig sets no credential, kubeconfig, exec provider or
// certificate file and discovers none: the lab drives only the Simulated
// Cluster it created itself.
func controlPlaneConfig(address string) *rest.Config {
	return &rest.Config{Host: address, Timeout: controlPlaneTimeout, QPS: noClientSideRateLimit}
}

// watchConfig drops the deadline alone, since client-go applies it to a
// watch's whole stream and would close every stream after it.
func watchConfig(address string) *rest.Config {
	watching := rest.CopyConfig(controlPlaneConfig(address))
	watching.Timeout = 0

	return watching
}

// controlPlaneClients gives the client every request goes through and the one
// the Timeline's watch streams alone open on.
func controlPlaneClients(address string) (requests, watches *kubernetes.Clientset, err error) {
	requests, err = kubernetes.NewForConfig(controlPlaneConfig(address))
	if err != nil {
		return nil, nil, fmt.Errorf("build a client for the control plane at %s: %w", address, err)
	}
	watches, err = kubernetes.NewForConfig(watchConfig(address))
	if err != nil {
		return nil, nil, fmt.Errorf("build a watch client for the control plane at %s: %w", address, err)
	}

	return requests, watches, nil
}
