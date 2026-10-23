package lab

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/woditschka/k8s-resilience-lab/internal/cluster"
	"github.com/woditschka/k8s-resilience-lab/internal/lab/node"
	"github.com/woditschka/k8s-resilience-lab/internal/lab/pod"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

const (
	defaultPlacementNamed   pod.SpecificationName = "Default placement"
	resilientPlacementNamed pod.SpecificationName = "Resilient placement"
)

// aWiderSkew differs from the definition's, so a customized specification is
// read back as the Scenario's value and not the definition's.
const (
	anEvenSkew = 1
	aWiderSkew = 2
)

// No test of this package turns on the Startup time.
const (
	someShortestStartupMillis = 500
	someLongestStartupMillis  = 1000
)

func someStartupTime() pod.StartupTime {
	return pod.StartupMillisBetween(someShortestStartupMillis, someLongestStartupMillis)
}

// The lab defines no specification and a test of internal/lab cannot import the
// Scenarios' place, so the tests define their own.
func defaultPlacement() pod.Specification {
	return pod.Predefined(defaultPlacementNamed, someStartupTime(), leavingAnUnresponsiveNode())
}

func resilientPlacement() pod.Specification {
	return pod.Predefined(resilientPlacementNamed, someStartupTime(),
		leavingAnUnresponsiveNode(),
		aZoneSpreadAt(anEvenSkew),
		pod.TopologySpreadConstraint{
			TopologyKey: node.Hostname, MaxSkew: anEvenSkew, WhenUnsatisfiable: pod.DoNotSchedule, NodeTaintsPolicy: pod.Honor,
		},
	)
}

// The lab defines no Named setting and the test cannot import the Scenarios'
// place, so the two tolerations are written directly.
func leavingAnUnresponsiveNode() pod.Customization {
	return pod.Settings(
		pod.Toleration{Key: node.NotReady, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: node.UnresponsiveWait},
		pod.Toleration{Key: node.Unreachable, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: node.UnresponsiveWait},
	)
}

func theLabsWaitInSeconds() int64 {
	return int64(scenario.UnresponsiveNodeWait.Seconds())
}

func aZoneSpreadAt(maxSkew int) pod.Setting {
	return pod.TopologySpreadConstraint{
		TopologyKey: node.Zone, MaxSkew: maxSkew, WhenUnsatisfiable: pod.DoNotSchedule, NodeTaintsPolicy: pod.Honor,
	}
}

const (
	oneCorePerPod      = 1
	twoGibibytesPerPod = 2

	// A Request the Deployment line's own stands over.
	overriddenCoresPerPod     = 3
	overriddenGibibytesPerPod = 6
)

// askingForNothing is the Deployment line's Request where the test asserts none,
// so its Pods ask for no cpu and no memory.
func askingForNothing() pod.Customization {
	return pod.Settings()
}

func aRequestOf(cores, gibibytes int) pod.Setting {
	return pod.Request{CPU: cores, Memory: scenario.Memory(gibibytes) * pod.Gi}
}

const millicoresPerCore = 1000

func coresOf(cores int) scenario.CPU {
	return scenario.CPU(cores * millicoresPerCore)
}

func gibibytesOf(gibibytes int) scenario.Memory {
	return scenario.Memory(gibibytes) * node.Gi
}

// No test of this package turns on the priority class: a specification holds
// the place a full access was written in, not the change itself.
const somePriorityClass = "any-priority-class"

// What the change does is read off the deployed Pod template in
// internal/cluster's tests.
func aFullAccess() pod.Customization {
	return pod.FullAccess(func(spec *corev1.PodSpec) { spec.PriorityClassName = somePriorityClass })
}

// The places are all a test can read of a function value, and all the mapper
// needs of one.
func placesOfTheFullAccesses(held []cluster.PodSpecAccessInPlace) []int {
	places := make([]int, 0, len(held))
	for _, written := range held {
		places = append(places, written.AfterSettings)
	}

	return places
}
