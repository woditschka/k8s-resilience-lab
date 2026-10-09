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
	// clusterImageRepository is the one repository every pull and create names. No
	// run input reaches it, so a run can never start an image that is not a KWOK
	// cluster image, per docs/adr/2026-10-03-kubernetes-version-pin.md.
	clusterImageRepository = "registry.k8s.io/kwok/cluster"

	// kwokRelease is the KWOK release the image tag carries, and
	// defaultKubernetesVersion the version a run mirrors when it names none. The
	// default is written in the form ParseKubernetesVersion normalises to, so the
	// lab's own default needs no parsing.
	kwokRelease              = "v0.8.0"
	defaultKubernetesVersion = "v1.36.1"

	// workloadNamespace is the namespace a Workload is deployed into. The lab
	// deploys one Workload at a time into a cluster of its own, so it needs no
	// namespace of its own.
	workloadNamespace = "default"

	// defaultServiceAccount is the ServiceAccount the control plane admits a Pod of
	// that namespace against.
	defaultServiceAccount = "default"
)

// pollInterval is the gap between the polls of a wait, which runs within the
// lab's own waiting time, scenario.WaitingTime.
//
// The interval is what makes "two consecutive observations agree" mean settled: a
// Deployment's Pods are created and then bound over several control-plane round
// trips, so two observations a few milliseconds apart can agree merely by falling
// inside one of them.
const pollInterval = time.Second

// DefaultKubernetesVersion is the version a run mirrors when it names none. It is
// a code constant: the lab looks up no newest version anywhere.
func DefaultKubernetesVersion() KubernetesVersion {
	return KubernetesVersion{value: defaultKubernetesVersion}
}

// clusterImage is the image a Simulated Cluster of version runs. The repository
// and the KWOK release are constants and the version is validated, so no
// reference, path or scheme character can reach the tag.
func clusterImage(version KubernetesVersion) string {
	return fmt.Sprintf("%s:%s-k8s.%s", clusterImageRepository, kwokRelease, version)
}

// SimulatedCluster is the provisioned cluster a run drives: the aggregate root
// through which a Topology, a Workload and an observation are reached. Its close
// removes the container, so a Scenario's cluster cannot outlive the Scenario.
type SimulatedCluster struct {
	container *ClusterContainer
	client    *kubernetes.Clientset
	// recording is this cluster's Timeline while it is being recorded, absent
	// until Record starts one. The runner starts and stops it on one goroutine,
	// so the field itself is never shared.
	recording *recording
}

// ObtainVersion makes the engine hold the cluster image of version, pulling it
// only when the engine does not hold it already, so a run learns once, before
// any Scenario starts, whether every Simulated Cluster it provisions can mirror
// that version (docs/prd.md REQ-SIM-001 edge case 4). Provision pulls the same
// image when absent, so after ObtainVersion it finds the image held.
func ObtainVersion(ctx context.Context, endpoint Endpoint, version KubernetesVersion) error {
	if err := (Engine{endpoint: endpoint}).pullWhenAbsent(ctx, clusterImage(version)); err != nil {
		return fmt.Errorf("obtain Kubernetes version %s: %w", version, err)
	}

	return nil
}

// Provision starts a Simulated Cluster for one Scenario on a resolved engine, for
// a resolved Kubernetes version, waits for its API server to answer, and makes
// the Stages it serves the lab's own set before any Node exists. It returns the
// handle whose close removes the cluster, or an error naming what it could not
// reach; a cluster it could not finish provisioning is removed rather than left
// behind.
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

// awaitControlPlane polls the started container until its API server answers,
// within the start timeout.
func awaitControlPlane(ctx context.Context, container *ClusterContainer) (*SimulatedCluster, error) {
	starting, cancel := context.WithTimeout(ctx, startTimeout)
	defer cancel()

	var unreached error
	for {
		client, err := reachControlPlane(starting, container)
		if err == nil {
			return &SimulatedCluster{container: container, client: client}, nil
		}
		unreached = err

		select {
		case <-starting.Done():
			return nil, fmt.Errorf("the API server of %s did not answer within %s: %w", container.Name(), startTimeout, unreached)
		case <-time.After(pollInterval):
		}
	}
}

// reachControlPlane builds a client for the container's published address and
// makes one request through it, so a client is returned only once the control
// plane is actually serving.
func reachControlPlane(ctx context.Context, container *ClusterContainer) (*kubernetes.Clientset, error) {
	address, err := container.APIAddress(ctx)
	if err != nil {
		return nil, err
	}

	client, err := newControlPlaneClient(address)
	if err != nil {
		return nil, err
	}
	if _, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{Limit: 1}); err != nil {
		return nil, fmt.Errorf("ask the control plane at %s for its Nodes: %w", address, err)
	}

	return client, nil
}

// Name is the name of the container this cluster runs in.
func (c *SimulatedCluster) Name() string {
	return c.container.Name()
}

// ApplyTopology creates every Node of the Topology, each already Ready and
// carrying the capacity the Topology declared, so no Pod waits on a heartbeat. It
// returns once the scheduler has placed and released one probe Pod on every one
// of them, which is what the Topology-provisioned state means, or an error naming
// the Nodes it never placed one on.
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

// reportReady writes a created Node's Ready condition and declared capacity
// through the status subresource, which a create does not carry.
//
// The write is retried on a conflict because the KWOK controller maintains these
// same Nodes: it can modify one between the create and this write, which makes
// the version held here stale. Retrying from a fresh read is the client's own
// idiom for that, and the alternative — writing without a version — would let
// this call silently overwrite whatever the controller had just decided.
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

// awaitTopologyProvisioned returns once the scheduler has placed a probe Pod on
// every declared Node, and every probe Pod is gone again.
//
// Waiting matters because a spread rule is evaluated when a Pod is scheduled and
// never again (docs/adr/2026-10-04-scenario-declared-pod-spread.md). A Workload
// deployed over a Topology the scheduler holds only part of is spread perfectly
// over the Nodes it had seen, which is a wrong answer rather than a late one:
// nine Pods over six of nine Nodes settle at a Skew of one per Node and leave a
// whole Zone empty, and nothing rebalances them afterwards.
//
// The wait asks the scheduler rather than the API server, because the API server
// reports a Node Ready as soon as its status is committed while the scheduler
// places Pods from a snapshot of its own that no API call shows. A Running probe
// Pod is the one observable proof that the snapshot holds its Node
// (docs/adr/2026-10-04-scheduler-observable-provisioning.md).
//
// The probe Pods are removed whether or not every one of them ran, so a Workload
// is never deployed into a cluster still holding them.
func (c *SimulatedCluster) awaitTopologyProvisioned(ctx context.Context, topology scenario.Topology) error {
	placed := c.awaitProbePodOnEveryNode(ctx, topology)

	return errors.Join(placed, c.removeProbePods(ctx))
}

// awaitProbePodOnEveryNode creates one probe Pod per declared Node and polls at
// the poll interval, within the start timeout, until each of them is Running. It
// names the Nodes that never held one rather than reporting a count, because that
// name is what a failed provisioning leaves to look at. The scheduler itself
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

// awaitPodAdmission returns once the namespace holds the default ServiceAccount
// the control plane admits a Pod against, polling at the poll interval within the
// start timeout its caller's context carries.
//
// The control plane creates that ServiceAccount a moment after it starts serving,
// and admission refuses a Pod created before it exists rather than leaving the
// Pod Pending. A Deployment's own controller retries such a refusal, so a
// Workload never sees it; provisioning creates its probe Pods directly, so the
// wait belongs here.
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

// removeProbePods deletes every probe Pod and returns once the cluster reports
// none left, so no probe Pod can be part of the Placement a Workload reports.
//
// The delete names a zero grace period because a probe Pod is fake: no kubelet
// confirms it stopped, so a graceful delete would leave it Terminating for the
// default grace period and hold its Pod slot on the Node.
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

// probePods are the probe Pods the cluster currently holds, read by the label
// provisioning marks its own Pods with.
func (c *SimulatedCluster) probePods(ctx context.Context) ([]corev1.Pod, error) {
	listed, err := c.client.CoreV1().Pods(workloadNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: probeSelector(),
	})
	if err != nil {
		return nil, fmt.Errorf("list the probe Pods of %s: %w", c.Name(), err)
	}

	return listed.Items, nil
}

// nodesMissingFrom names the declared Nodes that are absent from holding, in
// declaration order. Two waits read it: the probe-Pod wait of provisioning, and
// a Wait for every Node Ready.
func nodesMissingFrom(declared []scenario.Node, holding []string) []string {
	unheld := make([]string, 0, len(declared))
	for _, node := range declared {
		if !slices.Contains(holding, node.Name) {
			unheld = append(unheld, node.Name)
		}
	}

	return unheld
}

// FailNode stops one Node responding, the way a machine that hangs or loses its
// network stops: the Node keeps its place in the cluster and the control plane
// stops hearing from it.
//
// Two writes do it. The marking the KWOK controller selects the Nodes it
// maintains by is taken off the Node, which stops anything refreshing its
// heartbeat, and its Ready condition is then reported Unknown with a lost
// heartbeat's reason. Everything after that is the control plane's own doing:
// its node-lifecycle controller taints the Node unreachable, its taint-eviction
// controller deletes the Pods that stop tolerating that taint, and the Workload's
// ReplicaSet replaces them on the Nodes still responding. The lab writes no taint
// and deletes no Pod, because what the lab is for is watching Kubernetes do it,
// per docs/adr/2026-10-05-node-failure-withholds-the-nodes-status.md.
func (c *SimulatedCluster) FailNode(ctx context.Context, name string) error {
	if err := c.stopMaintaining(ctx, name); err != nil {
		return err
	}

	return c.reportUnresponsive(ctx, name)
}

// stopMaintaining takes the KWOK marking off one Node, so the controller started
// to maintain the marked Nodes alone stops maintaining this one and the status
// reported next stands.
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

// reportUnresponsive reports one Node's Ready condition Unknown through the status
// subresource, which is what a control plane reports for a Node it has stopped
// hearing from.
//
// The write is retried on a conflict for the reason reportReady's is: the KWOK
// controller maintains these same Nodes and can modify one between the read and
// the write.
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

// Deploy creates the Workload's Deployment, so the control plane's own
// controllers produce the Placement. The accesses are the full accesses the
// Workload line wrote, each run on the Pod template in the place it was written.
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

// ReadyNodes are the Nodes this cluster reports Ready, each with the Zone read
// from its own label. They are the Nodes a Placement is read against.
func (c *SimulatedCluster) ReadyNodes(ctx context.Context) ([]scenario.Node, error) {
	listed, err := c.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list the Nodes of %s: %w", c.Name(), err)
	}

	return fromNodes(readyNodesIn(listed.Items))
}

// Observe reads one Placement of the Workload against the Nodes that could hold
// its Pods.
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

// AwaitSettled polls one Workload's Placement until it has settled, or the lab's
// waiting time expires. The control plane is eventually consistent, so every
// reported Placement comes from a wait rather than from a single observation.
//
// It is the settle-only wait: a Scenario writes it for nothing yet, and the two
// Waits beside it hold for one further clause over the same loop, per
// docs/adr/2026-10-07-a-scenario-writes-its-own-wait.md.
func (c *SimulatedCluster) AwaitSettled(ctx context.Context, workload scenario.Workload) (scenario.Placement, error) {
	return c.await(ctx, workload, aSettledPlacement(workload))
}

// AwaitPodsReady is the Wait a Scenario writes for a Workload: it polls that
// Workload's Placement until the control plane reports every Pod it declared
// Ready, which is the condition kubectl rollout status waits on.
func (c *SimulatedCluster) AwaitPodsReady(ctx context.Context, workload scenario.Workload) (scenario.Placement, error) {
	return c.await(ctx, workload, everyPodReady(workload))
}

// AwaitNodesReady is the Wait a Scenario writes for a Topology: it polls until the
// control plane reports every Node that Topology declared Ready. The Placement it
// reaches is read against those Nodes, and holds no Pod where the Scenario has
// declared no Workload yet, which is where this Wait is written.
func (c *SimulatedCluster) AwaitNodesReady(
	ctx context.Context,
	topology scenario.Topology,
	deployed scenario.Workload,
) (scenario.Placement, error) {
	return c.await(ctx, deployed, everyNodeReady(topology, deployed))
}

// awaited is what one wait holds for: whether an observation is the one it was
// waiting for, read against the one before it, and what it reports of the last
// observation it took once the lab's waiting time has expired.
type awaited struct {
	reached func(previous, current scenario.Placement) bool
	expired func(observed scenario.Placement) error
}

// await polls the Placement of this Workload at the poll interval, within the
// lab's waiting time, until the wait has what it holds for. An expired wait
// reports the last observation it took rather than waiting longer.
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

// aSettledPlacement is what the settle wait holds for, and what the two Waits
// hold for beneath their own clause, so the Thens after a Wait judge a Placement
// that has stopped changing.
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

// everyPodReady is what a Wait for a Workload holds for: a settled Placement in
// which the control plane reports every Pod Ready. An expired wait names the
// clause that held it open, which is the settle one while a Pod is still missing.
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

// everyNodeReady is what a Wait for a Topology holds for: a settled Placement
// read against every Node that Topology declared. A Node that is not Ready is in
// no Placement the lab observes, so this clause reads the Nodes the observation
// is short of.
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

// nodesNotReadyIn are the Nodes the Topology declared that this observation was
// not read against, in declaration order.
func nodesNotReadyIn(topology scenario.Topology, observed scenario.Placement) []string {
	return nodesMissingFrom(topology.Nodes(), nodeNamesOf(observed.Nodes()))
}

// podsDidNotBecomeReady is what an expired Wait for every Pod Ready reports: the
// Pods the last observation did not hold Ready, by name, so a reader of an
// expired Wait sees which Pods held it open rather than a count alone.
func podsDidNotBecomeReady(workload scenario.Workload, observed scenario.Placement) error {
	notReady := observed.PodsNotReady()

	return fmt.Errorf("the Pods of the Workload %s were not all Ready within %s; it last held %d of %d Pods Ready, and these were not: %s",
		workload.Name(), scenario.WaitingTime, len(observed.Pods())-len(notReady), workload.Pods(),
		strings.Join(podNamesOf(notReady), ", "))
}

// nodesDidNotBecomeReady is what an expired Wait for every Node Ready reports:
// the declared Nodes the last observation was not read against, by name.
func nodesDidNotBecomeReady(topology scenario.Topology, observed scenario.Placement) error {
	return fmt.Errorf("the Nodes of the Topology were not all Ready within %s; these were not: %s",
		scenario.WaitingTime, strings.Join(nodesNotReadyIn(topology, observed), ", "))
}

// nodeNamesOf and podNamesOf are the names an observation holds, which is what a
// wait names rather than the values themselves.
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

// didNotSettle is what an expired settle wait reports: the Placement last
// observed, rather than a longer wait. It names one count per clause of
// hasSettled — the Pods created of the Pods declared, the Ready Nodes they were
// read against, the Pods on a Node the observation does not hold, and the placed
// Pods still inside their Startup time — so a reader of an expired wait can tell
// which clause held it open (PRD edge case 3).
func didNotSettle(workload scenario.Workload, observed scenario.Placement) error {
	return fmt.Errorf("the Placement of the Workload %s did not settle within %s; it last held %d of %d Pods over %d Ready Nodes, %d of them on a Node it does not hold and %d still starting",
		workload.Name(), scenario.WaitingTime, len(observed.Pods()), workload.Pods(), len(observed.Nodes()),
		len(observed.PodsItCannotAccountFor()), len(observed.PodsStarting()))
}

// hasSettled reports whether current can be the Workload's settled Placement:
// every Pod it declares has been created, every Pod it found is on a Node it was
// read against, and this observation agrees with the one before it.
//
// The Pod count is part of the condition because a Deployment's Pods appear a
// moment after the Deployment does. Two observations taken in that moment agree
// on finding nothing, which is an early Placement rather than a settled one. A
// Pod that cannot be placed still counts, because the control plane creates it
// and reports it Pending (edge case 1).
//
// A Pod on a Node the observation does not hold is the same kind of early
// Placement, after a Failure rather than after a Deployment: a Node that stopped
// responding leaves the Ready Nodes at once, while the Pod it holds is deleted
// only once the control plane has tainted the Node and the Pod has stopped
// tolerating that taint. Two observations inside that window agree without the
// Placement having stopped changing.
//
// A placed Pod still inside its Startup time is a third such window: it stands
// Pending on its Node until its time passes and it turns Running, so two
// observations within that time agree on a Placement that is about to change
// (docs/prd.md REQ-SIM-012). An unplaced Pod is no such Pod and still counts.
func hasSettled(previous, current scenario.Placement, workload scenario.Workload) bool {
	return len(current.Pods()) >= workload.Pods() &&
		len(current.PodsItCannotAccountFor()) == 0 &&
		len(current.PodsStarting()) == 0 &&
		current.Equal(previous)
}

// Close removes the Simulated Cluster this run provisioned. The runner defers it
// around each Scenario, so a Scenario that fails part-way through its steps still
// loses its cluster, per docs/adr/2026-10-04-cluster-per-scenario.md.
//
// A Timeline still recording is stopped first: the watch streams run against the
// cluster that is about to be removed, so a record outliving its cluster would
// only record its own disconnection.
func (c *SimulatedCluster) Close(ctx context.Context) error {
	c.StopRecording()

	return c.container.Remove(ctx)
}
