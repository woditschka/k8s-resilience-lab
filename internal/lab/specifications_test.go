package lab

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/woditschka/k8s-resilience-lab/internal/cluster"
	"github.com/woditschka/k8s-resilience-lab/internal/lab/node"
	"github.com/woditschka/k8s-resilience-lab/internal/lab/pod"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// The names the two specifications below are defined under, which is what the
// Step declaring one reports.
const (
	defaultPlacementNamed   pod.SpecificationName = "Default placement"
	resilientPlacementNamed pod.SpecificationName = "Resilient placement"
)

// anEvenSkew is the Skew that makes a spread even, and aWiderSkew the one above
// it a Customization below is written at, so what a customized specification
// holds is read back as the Scenario's value and not as the definition's.
const (
	anEvenSkew = 1
	aWiderSkew = 2
)

// The Startup time the two specifications below name, in the whole milliseconds
// a definition states it in: a range above zero, as the lab's Scenarios define
// theirs, which no test of this package turns on.
const (
	someShortestStartupMillis = 500
	someLongestStartupMillis  = 1000
)

// someStartupTime is that Startup time as a definition writes it.
func someStartupTime() pod.StartupTime {
	return pod.StartupMillisBetween(someShortestStartupMillis, someLongestStartupMillis)
}

// defaultPlacement and resilientPlacement are the two specifications the
// machinery's tests declare, defined beside them the way the Scenarios' place
// defines its own: the lab defines none, and a test of internal/lab cannot
// import that place, per
// docs/adr/2026-10-18-every-pod-specification-is-defined-by-the-scenarios.md.
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

// leavingAnUnresponsiveNode writes the two tolerations a Scenario's Named setting
// stands for, directly, where a test of internal/lab needs them: the lab defines
// no Named setting, and the test cannot import the Scenarios' place.
func leavingAnUnresponsiveNode() pod.Customization {
	return pod.Settings(
		pod.Toleration{Key: node.NotReady, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: node.UnresponsiveWait},
		pod.Toleration{Key: node.Unreachable, Operator: pod.Exists, Effect: pod.NoExecute, TolerationSeconds: node.UnresponsiveWait},
	)
}

// theLabsWaitInSeconds is the lab's own waiting time as the field Kubernetes
// holds it in, read from the constant that sets it rather than written out.
func theLabsWaitInSeconds() int64 {
	return int64(scenario.UnresponsiveNodeWait.Seconds())
}

// aZoneSpreadAt is a Customization as a Scenario writes one: the Zone spread at
// the Skew named.
func aZoneSpreadAt(maxSkew int) pod.Setting {
	return pod.TopologySpreadConstraint{
		TopologyKey: node.Zone, MaxSkew: maxSkew, WhenUnsatisfiable: pod.DoNotSchedule, NodeTaintsPolicy: pod.Honor,
	}
}

// The cpu and memory a Request below asks for, in the whole cores and whole
// gibibytes a Scenario states them in.
const (
	oneCorePerPod      = 1
	twoGibibytesPerPod = 2
)

// aRequestOf is a Request as a Scenario writes one.
func aRequestOf(cores, gibibytes int) pod.Setting {
	return pod.Request{CPU: cores, Memory: scenario.Memory(gibibytes) * pod.Gi}
}

// millicoresPerCore is how many millicores the domain holds one whole core as,
// which is Kubernetes' own: coresOf and gibibytesOf are whole cores and
// gibibytes as the domain holds them, which the expected capacities below are
// built from.
const millicoresPerCore = 1000

func coresOf(cores int) scenario.CPU {
	return scenario.CPU(cores * millicoresPerCore)
}

func gibibytesOf(gibibytes int) scenario.Memory {
	return scenario.Memory(gibibytes) * node.Gi
}

// somePriorityClass is a Kubernetes Pod setting the lab does not name, which no
// test of this package turns on: what a specification holds of a full access is
// the place it was written in and not the change itself.
const somePriorityClass = "any-priority-class"

// aFullAccess is full access as a Scenario writes one, in the Scenario's own
// spelling. What the change does is read off the deployed Pod template, in
// internal/cluster/resources_test.go.
func aFullAccess() pod.Customization {
	return pod.FullAccess(func(spec *corev1.PodSpec) { spec.PriorityClassName = somePriorityClass })
}

// placesOfTheFullAccesses are the places these full accesses were written in.
// They are all a test can read of a function value, and all the mapper needs of
// one.
func placesOfTheFullAccesses(held []cluster.PodSpecAccessInPlace) []int {
	places := make([]int, 0, len(held))
	for _, written := range held {
		places = append(places, written.AfterSettings)
	}

	return places
}
