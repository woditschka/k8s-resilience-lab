package cluster

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"

	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

const (
	// No run input reaches the repository, so a run can never start an image
	// that is not a KWOK cluster image.
	clusterImageRepository = "registry.k8s.io/kwok/cluster"

	// defaultKubernetesVersion is written in the form ParseKubernetesVersion
	// normalises to, so the lab's own default needs no parsing.
	kwokRelease              = "v0.8.0"
	defaultKubernetesVersion = "v1.36.1"

	// Each Workload's cluster is its own, so it needs no namespace of its own.
	workloadNamespace = "default"

	defaultServiceAccount = "default"
)

// pollInterval is what makes "two consecutive observations agree" mean settled:
// a Deployment's Pods are created and then bound over several control-plane
// round trips, so two observations a few milliseconds apart can agree merely by
// falling inside one of them.
const pollInterval = time.Second

// DefaultKubernetesVersion is a code constant: the lab looks up no newest
// version anywhere.
func DefaultKubernetesVersion() KubernetesVersion {
	return KubernetesVersion{value: defaultKubernetesVersion}
}

// The repository and the KWOK release are constants and the version is
// validated, so no reference, path or scheme character can reach the tag.
func clusterImage(version KubernetesVersion) string {
	return fmt.Sprintf("%s:%s-k8s.%s", clusterImageRepository, kwokRelease, version)
}

// SimulatedCluster removes its container on Close, so a Scenario's cluster
// cannot outlive the Scenario.
type SimulatedCluster struct {
	container *ClusterContainer
	client    *kubernetes.Clientset
	// watches is the Timeline's streams' alone; every other call goes through
	// client and its deadline.
	watches *kubernetes.Clientset
	// The runner starts and stops recording on one goroutine, so the field is
	// never shared.
	recording *recording
}

// ObtainVersion lets a run learn once, before any Scenario starts, whether
// every Simulated Cluster it provisions can mirror the version. It pulls only
// when the engine lacks the image.
func ObtainVersion(ctx context.Context, endpoint Endpoint, version KubernetesVersion) error {
	if err := (Engine{endpoint: endpoint}).pullWhenAbsent(ctx, clusterImage(version)); err != nil {
		return fmt.Errorf("obtain Kubernetes version %s: %w", version, err)
	}

	return nil
}

// Provision makes the served Stages the lab's own set before any Node exists. A
// cluster it could not finish provisioning is removed rather than left behind.
func Provision(ctx context.Context, endpoint Endpoint, version KubernetesVersion, scenarioName string) (*SimulatedCluster, error) {
	engine, err := newEngine(endpoint, scenarioName)
	if err != nil {
		return nil, err
	}

	container, err := engine.StartCluster(ctx, version)
	if err != nil {
		return nil, err
	}

	provisioned, err := awaitControlPlane(ctx, container)
	if err != nil {
		return nil, errors.Join(err, container.Remove(ctx))
	}
	if err := provisioned.ownStages(ctx); err != nil {
		return nil, errors.Join(err, container.Remove(ctx))
	}

	return provisioned, nil
}

func awaitControlPlane(ctx context.Context, container *ClusterContainer) (*SimulatedCluster, error) {
	starting, cancel := context.WithTimeout(ctx, startTimeout)
	defer cancel()

	var unreached error
	for {
		client, watches, err := reachControlPlane(starting, container)
		if err == nil {
			return &SimulatedCluster{container: container, client: client, watches: watches}, nil
		}
		unreached = err

		select {
		case <-starting.Done():
			return nil, fmt.Errorf("the API server of %s did not answer within %s: %w", container.Name(), startTimeout, unreached)
		case <-time.After(pollInterval):
		}
	}
}

// One request is made through the request client, so the clients are returned
// only once the control plane is actually serving.
func reachControlPlane(ctx context.Context, container *ClusterContainer) (requests, watches *kubernetes.Clientset, err error) {
	address, err := container.APIAddress(ctx)
	if err != nil {
		return nil, nil, err
	}

	requests, watches, err = controlPlaneClients(address)
	if err != nil {
		return nil, nil, err
	}
	if _, err := requests.CoreV1().Nodes().List(ctx, metav1.ListOptions{Limit: 1}); err != nil {
		return nil, nil, fmt.Errorf("ask the control plane at %s for its Nodes: %w", address, err)
	}

	return requests, watches, nil
}

func (c *SimulatedCluster) Name() string {
	return c.container.Name()
}

// ApplyTopology creates every Node already Ready, so no Pod waits on a
// heartbeat, and returns once the scheduler has placed and released a probe Pod
// on each.
func (c *SimulatedCluster) ApplyTopology(ctx context.Context, topology scenario.Topology) error {
	for _, node := range topology.Nodes() {
		declared := toNode(node)

		if _, err := c.client.CoreV1().Nodes().Create(ctx, declared, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create Node %s: %w", node.Name, err)
		}
		if err := c.reportReady(ctx, declared); err != nil {
			return err
		}
	}

	return c.awaitTopologyProvisioned(ctx, topology)
}

// A create does not carry status, so it is written through the status
// subresource.
//
// The write is retried on a conflict because the KWOK controller maintains
// these same Nodes and can modify one between the create and this write.
// Writing without a version instead would silently overwrite whatever the
// controller had just decided.
func (c *SimulatedCluster) reportReady(ctx context.Context, declared *corev1.Node) error {
	reported := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest, err := c.client.CoreV1().Nodes().Get(ctx, declared.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		latest.Status = declared.Status
		_, err = c.client.CoreV1().Nodes().UpdateStatus(ctx, latest, metav1.UpdateOptions{})

		return err
	})
	if reported != nil {
		return fmt.Errorf("report Node %s Ready with its capacity: %w", declared.Name, reported)
	}

	return nil
}

// A spread rule is evaluated when a Pod is scheduled and never again. A
// Workload deployed over a Topology the scheduler holds only part of is spread
// perfectly over the Nodes it had seen, which is a wrong answer rather than a
// late one: nine Pods over six of nine Nodes leave a whole Zone empty, and
// nothing rebalances them.
//
// The wait asks the scheduler rather than the API server, because the API
// server reports a Node Ready as soon as its status is committed while the
// scheduler places Pods from a snapshot no API call shows. A Running probe Pod
// is the one observable proof that the snapshot holds its Node.
//
// The probe Pods are removed whether or not every one ran, so a Workload is
// never deployed into a cluster still holding them.
func (c *SimulatedCluster) awaitTopologyProvisioned(ctx context.Context, topology scenario.Topology) error {
	placed := c.awaitProbePodOnEveryNode(ctx, topology)

	return errors.Join(placed, c.removeProbePods(ctx))
}

// The Nodes that never held a probe Pod are named rather than counted, because
// that name is what a failed provisioning leaves to look at. The scheduler
// re-queues a probe Pod it cannot place yet, so the wait needs no retry of its
// own.
func (c *SimulatedCluster) awaitProbePodOnEveryNode(ctx context.Context, topology scenario.Topology) error {
	probing, cancel := context.WithTimeout(ctx, startTimeout)
	defer cancel()

	if err := c.awaitPodAdmission(probing); err != nil {
		return err
	}

	declared := topology.Nodes()
	for _, node := range declared {
		if _, err := c.client.CoreV1().Pods(workloadNamespace).Create(probing, toProbePod(node), metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create the probe Pod for Node %s: %w", node.Name, err)
		}
	}

	for {
		probes, err := c.probePods(probing)
		if err != nil {
			return err
		}
		unheld := nodesMissingFrom(declared, nodesHoldingRunningProbe(probes))
		if len(unheld) == 0 {
			return nil
		}

		select {
		case <-probing.Done():
			return fmt.Errorf("the scheduler of %s placed no Pod on %d of the %d Nodes of the Topology within %s: %s",
				c.Name(), len(unheld), len(declared), startTimeout, strings.Join(unheld, ", "))
		case <-time.After(pollInterval):
		}
	}
}

// The control plane creates the default ServiceAccount a moment after it starts
// serving, and admission refuses a Pod created before it exists rather than
// leaving it Pending. A Deployment's controller retries such a refusal, but
// provisioning creates its probe Pods directly, so the wait belongs here.
func (c *SimulatedCluster) awaitPodAdmission(ctx context.Context) error {
	for {
		_, err := c.client.CoreV1().ServiceAccounts(workloadNamespace).Get(ctx, defaultServiceAccount, metav1.GetOptions{})
		if err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("the %s namespace of %s held no %s ServiceAccount to admit a Pod against within %s: %w",
				workloadNamespace, c.Name(), defaultServiceAccount, startTimeout, err)
		case <-time.After(pollInterval):
		}
	}
}

// The delete names a zero grace period because a probe Pod is fake: no kubelet
// confirms it stopped, so a graceful delete would leave it Terminating and
// holding its Pod slot on the Node.
func (c *SimulatedCluster) removeProbePods(ctx context.Context) error {
	removing, cancel := context.WithTimeout(ctx, startTimeout)
	defer cancel()

	immediately := int64(0)
	if err := c.client.CoreV1().Pods(workloadNamespace).DeleteCollection(removing,
		metav1.DeleteOptions{GracePeriodSeconds: &immediately},
		metav1.ListOptions{LabelSelector: probeSelector()}); err != nil {
		return fmt.Errorf("remove the probe Pods of %s: %w", c.Name(), err)
	}

	for {
		remaining, err := c.probePods(removing)
		if err != nil {
			return err
		}
		if len(remaining) == 0 {
			return nil
		}

		select {
		case <-removing.Done():
			return fmt.Errorf("%d probe Pods of %s were still there %s after their removal",
				len(remaining), c.Name(), startTimeout)
		case <-time.After(pollInterval):
		}
	}
}

func (c *SimulatedCluster) probePods(ctx context.Context) ([]corev1.Pod, error) {
	listed, err := c.client.CoreV1().Pods(workloadNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: probeSelector(),
	})
	if err != nil {
		return nil, fmt.Errorf("list the probe Pods of %s: %w", c.Name(), err)
	}

	return listed.Items, nil
}

func nodesMissingFrom(declared []scenario.Node, holding []string) []string {
	unheld := make([]string, 0, len(declared))
	for _, node := range declared {
		if !slices.Contains(holding, node.Name) {
			unheld = append(unheld, node.Name)
		}
	}

	return unheld
}

// FailNode stops one Node responding the way a hung machine does: the Node
// keeps its place and the control plane stops hearing from it.
//
// Removing the KWOK marking stops anything refreshing its heartbeat, and its
// Ready condition is then reported Unknown. Everything after that is the
// control plane's own doing: the node-lifecycle controller taints the Node
// unreachable, the taint-eviction controller deletes the Pods that stop
// tolerating that taint, and the ReplicaSet replaces them. The lab writes no
// taint and deletes no Pod, because watching Kubernetes do it is what the lab
// is for.
func (c *SimulatedCluster) FailNode(ctx context.Context, name string) error {
	if err := c.stopMaintaining(ctx, name); err != nil {
		return err
	}

	return c.reportUnresponsive(ctx, name)
}

// The KWOK controller maintains only marked Nodes, so once the marking is gone
// the status reported next stands.
func (c *SimulatedCluster) stopMaintaining(ctx context.Context, name string) error {
	unmarked := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest, err := c.client.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		latest.Labels = withoutTheKWOKMarking(latest.Labels)
		_, err = c.client.CoreV1().Nodes().Update(ctx, latest, metav1.UpdateOptions{})

		return err
	})
	if unmarked != nil {
		return fmt.Errorf("take Node %s off the Nodes the KWOK controller maintains: %w", name, unmarked)
	}

	return nil
}

// The write is retried on a conflict for the reason reportReady's is.
func (c *SimulatedCluster) reportUnresponsive(ctx context.Context, name string) error {
	reported := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest, err := c.client.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		latest.Status = toUnresponsive(latest.Status)
		_, err = c.client.CoreV1().Nodes().UpdateStatus(ctx, latest, metav1.UpdateOptions{})

		return err
	})
	if reported != nil {
		return fmt.Errorf("report Node %s as no longer responding: %w", name, reported)
	}

	return nil
}

// Deploy lets the control plane's own controllers produce the Placement from
// the Deployment.
func (c *SimulatedCluster) Deploy(
	ctx context.Context,
	workload scenario.Workload,
	accesses ...PodSpecAccessInPlace,
) error {
	deployed := toDeployment(workload, accesses...)
	if _, err := c.client.AppsV1().Deployments(workloadNamespace).Create(ctx, deployed, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("deploy the Workload %s: %w", workload.Name(), err)
	}

	return nil
}

func (c *SimulatedCluster) ReadyNodes(ctx context.Context) ([]scenario.Node, error) {
	listed, err := c.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list the Nodes of %s: %w", c.Name(), err)
	}

	return fromNodes(readyNodesIn(listed.Items))
}

func (c *SimulatedCluster) Observe(ctx context.Context, workload scenario.Workload) (scenario.Placement, error) {
	nodes, err := c.ReadyNodes(ctx)
	if err != nil {
		return scenario.Placement{}, err
	}

	listed, err := c.client.CoreV1().Pods(workloadNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: podSelectorOf(workload),
	})
	if err != nil {
		return scenario.Placement{}, fmt.Errorf("list the Pods of the Workload %s: %w", workload.Name(), err)
	}

	return scenario.NewPlacement(nodes, fromPods(listed.Items)), nil
}

// ObserveEveryWorkload reads every Node, Ready or not, so a Node that stopped
// responding keeps its place on a Placement board, marked NotReady. It selects
// Pods by the Workload label being present, which a probe Pod never carries.
func (c *SimulatedCluster) ObserveEveryWorkload(ctx context.Context) (scenario.Placement, error) {
	listedNodes, err := c.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return scenario.Placement{}, fmt.Errorf("list the Nodes of %s: %w", c.Name(), err)
	}
	nodes, err := fromNodes(listedNodes.Items)
	if err != nil {
		return scenario.Placement{}, err
	}

	listedPods, err := c.client.CoreV1().Pods(workloadNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: everyWorkloadsPods(),
	})
	if err != nil {
		return scenario.Placement{}, fmt.Errorf("list the Pods of every Workload of %s: %w", c.Name(), err)
	}

	return scenario.NewPlacement(nodes, fromPods(listedPods.Items)).WithNodesNotReady(notReadyNodeNamesIn(listedNodes.Items)), nil
}

// AwaitSettled reports a Placement from a wait rather than from a single
// observation, since the control plane is eventually consistent. It is the
// settle-only wait no Scenario writes yet, so it is kept for the first that
// does and is not dead code.
func (c *SimulatedCluster) AwaitSettled(ctx context.Context, workload scenario.Workload) (scenario.Placement, error) {
	return c.await(ctx, workload, aSettledPlacement(workload))
}

// AwaitPodsReady holds for the condition kubectl rollout status waits on.
func (c *SimulatedCluster) AwaitPodsReady(ctx context.Context, workload scenario.Workload) (scenario.Placement, error) {
	return c.await(ctx, workload, everyPodReady(workload))
}

// AwaitNodesReady reaches a Placement that holds no Pod where no Workload has
// been declared yet.
func (c *SimulatedCluster) AwaitNodesReady(
	ctx context.Context,
	topology scenario.Topology,
	deployed scenario.Workload,
) (scenario.Placement, error) {
	return c.await(ctx, deployed, everyNodeReady(topology, deployed))
}

type awaited struct {
	reached func(previous, current scenario.Placement) bool
	expired func(observed scenario.Placement) error
}

// An expired wait reports the last observation it took rather than waiting
// longer.
func (c *SimulatedCluster) await(
	ctx context.Context,
	workload scenario.Workload,
	holds awaited,
) (scenario.Placement, error) {
	waiting, cancel := context.WithTimeout(ctx, scenario.WaitingTime)
	defer cancel()

	previous, err := c.Observe(waiting, workload)
	if err != nil {
		return scenario.Placement{}, err
	}

	for {
		select {
		case <-waiting.Done():
			return scenario.Placement{}, holds.expired(previous)
		case <-time.After(pollInterval):
		}

		current, err := c.Observe(waiting, workload)
		if err != nil {
			return scenario.Placement{}, err
		}
		if holds.reached(previous, current) {
			return current, nil
		}
		previous = current
	}
}

func aSettledPlacement(workload scenario.Workload) awaited {
	return awaited{
		reached: func(previous, current scenario.Placement) bool {
			return hasSettled(previous, current, workload)
		},
		expired: func(observed scenario.Placement) error {
			return didNotSettle(workload, observed)
		},
	}
}

// An expired wait names the clause that held it open, which is the settle one
// while a Pod is still missing.
func everyPodReady(workload scenario.Workload) awaited {
	settled := aSettledPlacement(workload)

	return awaited{
		reached: func(previous, current scenario.Placement) bool {
			return settled.reached(previous, current) && len(current.PodsNotReady()) == 0
		},
		expired: func(observed scenario.Placement) error {
			if len(observed.PodsNotReady()) == 0 {
				return settled.expired(observed)
			}

			return podsDidNotBecomeReady(workload, observed)
		},
	}
}

// A Node that is not Ready is in no observed Placement, so this clause reads
// the Nodes the observation is short of.
func everyNodeReady(topology scenario.Topology, deployed scenario.Workload) awaited {
	settled := aSettledPlacement(deployed)

	return awaited{
		reached: func(previous, current scenario.Placement) bool {
			return len(nodesNotReadyIn(topology, current)) == 0 && settled.reached(previous, current)
		},
		expired: func(observed scenario.Placement) error {
			if len(nodesNotReadyIn(topology, observed)) == 0 {
				return settled.expired(observed)
			}

			return nodesDidNotBecomeReady(topology, observed)
		},
	}
}

func nodesNotReadyIn(topology scenario.Topology, observed scenario.Placement) []string {
	return nodesMissingFrom(topology.Nodes(), nodeNamesOf(observed.Nodes()))
}

// The Pods are named rather than counted, so a reader sees which ones held the
// Wait open.
func podsDidNotBecomeReady(workload scenario.Workload, observed scenario.Placement) error {
	notReady := observed.PodsNotReady()

	return fmt.Errorf("the Pods of the Workload %s were not all Ready within %s; it last held %d of %d Pods Ready, and these were not: %s",
		workload.Name(), scenario.WaitingTime, len(observed.Pods())-len(notReady), workload.Pods(),
		strings.Join(podNamesOf(notReady), ", "))
}

func nodesDidNotBecomeReady(topology scenario.Topology, observed scenario.Placement) error {
	return fmt.Errorf("the Nodes of the Topology were not all Ready within %s; these were not: %s",
		scenario.WaitingTime, strings.Join(nodesNotReadyIn(topology, observed), ", "))
}

func nodeNamesOf(nodes []scenario.Node) []string {
	names := make([]string, 0, len(nodes))
	for _, node := range nodes {
		names = append(names, node.Name)
	}

	return names
}

func podNamesOf(pods []scenario.Pod) []string {
	names := make([]string, 0, len(pods))
	for _, pod := range pods {
		names = append(names, pod.Name)
	}

	return names
}

// didNotSettle names one count per clause of hasSettled, so a reader of an
// expired wait can tell which clause held it open.
func didNotSettle(workload scenario.Workload, observed scenario.Placement) error {
	return fmt.Errorf("the Placement of the Workload %s did not settle within %s; it last held %d of %d Pods over %d Ready Nodes, %d of them on a Node it does not hold and %d still starting",
		workload.Name(), scenario.WaitingTime, len(observed.Pods()), workload.Pods(), len(observed.Nodes()),
		len(observed.PodsItCannotAccountFor()), len(observed.PodsStarting()))
}

// The Pod count is part of the condition because a Deployment's Pods appear a
// moment after the Deployment does, and two observations in that moment agree
// on finding nothing. A Pod that cannot be placed still counts, because the
// control plane creates it and reports it Pending.
//
// A Pod on a Node the observation does not hold is an early Placement after a
// Failure: a Node that stopped responding leaves the Ready Nodes at once, while
// its Pod is deleted only once the Node is tainted and the Pod stops tolerating
// that. Two observations inside that window agree without the Placement having
// stopped changing.
//
// A placed Pod still inside its Startup time is a third such window: it stands
// Pending until its time passes. An unplaced Pod is no such Pod and still
// counts.
func hasSettled(previous, current scenario.Placement, workload scenario.Workload) bool {
	return len(current.Pods()) >= workload.Pods() &&
		len(current.PodsItCannotAccountFor()) == 0 &&
		len(current.PodsStarting()) == 0 &&
		current.Equal(previous)
}

// Close is deferred by the runner around each Scenario, so a Scenario that
// fails part-way still loses its cluster.
//
// A Timeline still recording is stopped first, since a record outliving its
// cluster would only record its own disconnection.
func (c *SimulatedCluster) Close(ctx context.Context) error {
	c.StopRecording()

	return c.container.Remove(ctx)
}
