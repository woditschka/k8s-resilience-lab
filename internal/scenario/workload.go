package scenario

import (
	"errors"
	"fmt"
)

// Workload is one declared group of Pod replicas, deployed together with the Pod
// specification that decides where its Pods may land and when they leave a Node.
//
// A Workload declares one of the lab's predefined specifications and no setting
// of its own, per
// docs/adr/2026-10-05-workload-declares-a-predefined-pod-specification.md.
type Workload struct {
	name          string
	pods          int
	specification PodSpecification
}

// NewWorkload builds the Workload a Scenario deploys, refusing a declaration the
// lab could not deploy and a specification that sets nothing.
func NewWorkload(name string, pods int, declared PodSpecification) (Workload, error) {
	if name == "" {
		return Workload{}, errors.New("a Workload carries the name its Pods are labelled with, so an empty name is refused")
	}
	if pods < 1 {
		return Workload{}, fmt.Errorf("a Workload declares at least one Pod, so a count of %d Pods is refused", pods)
	}
	if len(declared.settings) == 0 {
		return Workload{}, errors.New("a Workload's Pods are deployed with the specification it declares, so a specification holding no setting is refused")
	}

	return Workload{name: name, pods: pods, specification: declared}, nil
}

// Name is what this Workload's Pods are labelled with.
func (w Workload) Name() string {
	return w.name
}

// Pods is how many replicas this Workload declares.
func (w Workload) Pods() int {
	return w.pods
}

// PodSpecification is the specification this Workload's Pods are deployed with.
func (w Workload) PodSpecification() PodSpecification {
	return w.specification
}

// Equal reports whether two Workloads declare the same Pods under the same
// specification. It is the equality go-cmp compares through, since the type's
// fields are unexported.
func (w Workload) Equal(other Workload) bool {
	return w.name == other.name && w.pods == other.pods && w.specification.Equal(other.specification)
}
