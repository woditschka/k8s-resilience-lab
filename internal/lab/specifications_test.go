//go:build !integration

package lab

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/woditschka/k8s-resilience-lab/internal/cluster"
	"github.com/woditschka/k8s-resilience-lab/internal/lab/pod"
)

const (
	oneCorePerPod      = 1
	twoGibibytesPerPod = 2

	// A Request the Deployment line's own stands over.
	overriddenCoresPerPod     = 3
	overriddenGibibytesPerPod = 6
)

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
