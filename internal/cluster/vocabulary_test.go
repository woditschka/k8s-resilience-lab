// The tests needing a container engine and those needing none share these
// names, so this file carries neither kind's build constraint.

package cluster

import (
	"bytes"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

func theDeschedulerPairedByDefault(t *testing.T) DeschedulerVersion {
	t.Helper()

	paired, err := DefaultKubernetesVersion().PairedDescheduler()
	if err != nil {
		t.Fatalf("pair a Descheduler with Kubernetes %s: %v", DefaultKubernetesVersion(), err)
	}

	return paired
}

const (
	// No test here turns on a profile's name beyond telling two apart.
	someProfileName    = "a-descheduler-profile"
	anotherProfileName = "another-descheduler-profile"

	oneEvictionPerNode uint = 1
	oneEvictionPerPass uint = 1
	// No test here turns on the per-Node eviction limit but the one encoding it.
	anyLimitPerNode uint = 0

	theNodeFitArgument     = "nodeFit"
	theConstraintsArgument = "constraints"
	aConstraintAction      = "DoNotSchedule"
)

func aDeschedulerProfileOf(t *testing.T, name string, configured []scenario.DeschedulerPluginConfig,
	enabled ...scenario.DeschedulerPluginSet,
) scenario.DeschedulerProfile {
	t.Helper()

	profile, err := scenario.NewDeschedulerProfile(name, configured, enabled)
	if err != nil {
		t.Fatalf("build the Descheduler profile %q: %v", name, err)
	}

	return profile
}

func aDeschedulerPolicyOf(t *testing.T, maxPodsToEvictPerNode uint, profiles ...scenario.DeschedulerProfile) scenario.DeschedulerPolicy {
	t.Helper()

	policy, err := scenario.NewDeschedulerPolicy(maxPodsToEvictPerNode, profiles...)
	if err != nil {
		t.Fatalf("build a Descheduler policy: %v", err)
	}

	return policy
}

const (
	deployedDeploymentName = "web"
	somePodsPerNode        = 10
	somePodCount           = 9
	evenSkew               = 1
	widerSkew              = 2

	someCPUPerNode      = scenario.CPU(4 * millicoresPerCore)
	someMemoryPerNode   = scenario.Memory(16 * mebibytesPerGibibyte)
	someRequestedCPU    = scenario.CPU(1 * millicoresPerCore)
	someRequestedMemory = scenario.Memory(2 * mebibytesPerGibibyte)

	// internal/lab converts whole units to the lab's quantities, and this suite
	// converts the same way rather than importing internal/lab, which imports
	// this package.
	millicoresPerCore    = 1000
	mebibytesPerGibibyte = 1024

	// No test of this suite turns on the specification name.
	someSpecificationName = "a Pod specification"
)

// The predefined specifications live in the Scenarios' place, which imports
// internal/lab, which imports this package, so this suite builds its own.
func aSpecificationOf(t *testing.T, settings ...scenario.Setting) scenario.PodSpecification {
	t.Helper()

	declared, err := scenario.NewPodSpecification(someSpecificationName,
		aStartupTimeBetween(t, someShortestStartup, someLongestStartup), settings...)
	if err != nil {
		t.Fatalf("build the Pod specification %q: %v", someSpecificationName, err)
	}

	return declared
}

const (
	someShortestStartup = 500 * time.Millisecond
	someLongestStartup  = time.Second
)

func anEvenSpreadOver(topologyKey string, maxSkew int) scenario.TopologySpread {
	return scenario.NewTopologySpread(topologyKey, maxSkew, scenario.DoNotSchedule, scenario.Honor)
}

func aNodeConditionToleration(taint string) scenario.Toleration {
	return scenario.NewToleration(taint, scenario.Exists, "", scenario.NoExecute, theLabsWaitInSeconds())
}

func theLabsWaitInSeconds() int64 {
	return int64(scenario.UnresponsiveNodeWait.Seconds())
}

const (
	capacityTypeLabel = "capacity-type"
	spotCapacity      = "spot"
)

func topologyOfTheGeneralNodePool(zones, nodesPerZone int, capacity scenario.NodeCapacity) (scenario.Topology, error) {
	pool, err := scenario.NewNodePool("general", nodesPerZone, capacity)
	if err != nil {
		return scenario.Topology{}, err
	}

	return scenario.NewTopology(zones, pool)
}

func aCapacity(t *testing.T) scenario.NodeCapacity {
	t.Helper()

	return aCapacityOf(t, somePodsPerNode, someCPUPerNode, someMemoryPerNode)
}

func aCapacityOf(t *testing.T, pods int, cpu scenario.CPU, memory scenario.Memory) scenario.NodeCapacity {
	t.Helper()

	capacity, err := scenario.NewNodeCapacity(pods, cpu, memory)
	if err != nil {
		t.Fatalf("build a Node capacity of %d Pods, %s cpu and %s memory: %v", pods, cpu, memory, err)
	}

	return capacity
}

func aStartupTimeBetween(t *testing.T, shortest, longest time.Duration) scenario.StartupTime {
	t.Helper()

	stated, err := scenario.NewStartupTime(int(shortest.Milliseconds()), int(longest.Milliseconds()))
	if err != nil {
		t.Fatalf("build a Startup time between %s and %s: %v", shortest, longest, err)
	}

	return stated
}

const (
	gpuTaintKey   = "nvidia.com/gpu"
	gpuTaintValue = "present"
)

func aGPUTaint() scenario.Taint {
	return scenario.Taint{Key: gpuTaintKey, Value: gpuTaintValue, Effect: scenario.NoSchedule}
}

func discardedWarnings() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func recordedWarnings() (*slog.Logger, *bytes.Buffer) {
	var written bytes.Buffer

	return slog.New(slog.NewTextHandler(&written, nil)), &written
}

func aSocketThatIsNotThere(t *testing.T) string {
	t.Helper()

	return filepath.Join(t.TempDir(), "absent-engine.sock")
}

// answeredBy stands in for the engine at the transport, for an engine
// behaviour the real one cannot be made to show on demand.
type answeredBy func(*http.Request) (*http.Response, error)

func (a answeredBy) RoundTrip(request *http.Request) (*http.Response, error) {
	return a(request)
}

// The socket is a name the errors carry; no request reaches it.
const aStandInsSocket = "stand-in.sock"

func anEngineStandIn(answers answeredBy) Endpoint {
	return newEndpoint(aStandInsSocket, &http.Client{Transport: answers})
}

const (
	// Each container on the engine says which Scenario it serves.
	resilientPlacementScenario = "resilient-placement"

	threeZones        = 3
	threeNodesPerZone = 3
	ninePods          = 9

	// The second Deployment has a label of its own, so each is spread over the
	// Nodes on its own.
	secondDeploymentName = "basket"
	threePods            = 3

	onePodPerNode = 1
	onePodTooMany = 1

	twoNodesInIt = 2
	noPodAtAll   = 0

	nodesStillResponding   = threeZones*threeNodesPerZone - 1
	onePodLeftBehind       = 1
	podLeftOnTheFailedNode = "checkout-left-behind"

	onePodNotYetCreated = 1

	onePodNotReadyYet = 1

	noPodStillStarting = 0
)

// Each Node can hold more Pods than the Deployment declares.
func anEvenSpreadTopology(t *testing.T) scenario.Topology {
	t.Helper()

	topology, err := topologyOfTheGeneralNodePool(threeZones, threeNodesPerZone, aCapacity(t))
	if err != nil {
		t.Fatalf("build a Topology of %d Zones of %d Nodes: %v", threeZones, threeNodesPerZone, err)
	}

	return topology
}

// The predefined specifications live in the Scenarios' place, which imports
// internal/lab, which imports this package, so this suite builds the same
// settings with internal/scenario's constructors.
func resilientPlacement(t *testing.T) scenario.PodSpecification {
	t.Helper()

	return defaultPlacement(t).With(
		anEvenSpreadOver(scenario.ZoneLabel, evenSkew),
		anEvenSpreadOver(scenario.HostnameLabel, evenSkew),
	)
}

func defaultPlacement(t *testing.T) scenario.PodSpecification {
	t.Helper()

	return aSpecificationOf(t,
		aNodeConditionToleration(scenario.TaintNodeNotReady),
		aNodeConditionToleration(scenario.TaintNodeUnreachable),
	)
}

func aDeploymentDeclaring(t *testing.T, pods int, declaring scenario.PodSpecification) scenario.Deployment {
	t.Helper()

	return aDeploymentNamed(t, deployedDeploymentName, pods, declaring)
}

func aDeploymentNamed(t *testing.T, name string, pods int, declaring scenario.PodSpecification) scenario.Deployment {
	t.Helper()

	deployment, err := scenario.NewDeployment(name, pods, declaring)
	if err != nil {
		t.Fatalf("build the Deployment %s of %d Pods: %v", name, pods, err)
	}

	return deployment
}

// The Node is derived rather than written out, so the expectation follows the
// Topology asked.
func theFirstNodeOf(topology scenario.Topology) string {
	return topology.Nodes()[0].Name
}

// entriesDelivered keeps the entries, so the delivery is asserted without the
// folder internal/timeline's File needs.
type entriesDelivered struct {
	entries []scenario.Entry
}

func (e *entriesDelivered) Write(entry scenario.Entry) {
	e.entries = append(e.entries, entry)
}

func expectNaming(t *testing.T, written string, named ...string) {
	t.Helper()

	for _, name := range named {
		if !strings.Contains(written, name) {
			t.Errorf("the lab wrote %q, which does not name %s", written, name)
		}
	}
}
