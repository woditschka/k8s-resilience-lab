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
	required      requiredCapacity
}

// requiredCapacity is the fewest Pods a Workload must keep Running through a
// Failure, and whether the Workload declared one at all: a Workload declaring
// none is judged by no Failure tolerance, and a minimum of no Pod is a
// declaration rather than the absence of one, per
// docs/adr/2026-10-06-required-capacity-judged-at-two-moments.md.
type requiredCapacity struct {
	pods     int
	declared bool
}

// NewWorkload builds the Workload a Scenario deploys, refusing a declaration the
// lab could not deploy and a specification that sets nothing. The Workload it
// returns declares no Required Capacity.
func NewWorkload(name string, pods int, declared PodSpecification) (Workload, error) {
	return newWorkload(name, pods, declared, requiredCapacity{})
}

// WithRequiredCapacity is this Workload declaring the fewest Pods it must keep
// Running through a Failure: a copy routed through the one constructor, which
// refuses a minimum above the Pods declared and one below a single Pod.
func (w Workload) WithRequiredCapacity(pods int) (Workload, error) {
	return newWorkload(w.name, w.pods, w.specification, requiredCapacity{pods: pods, declared: true})
}

// newWorkload is the one validation home every Workload comes into being through,
// the copies declaring a Required Capacity included
// (docs/architecture-principles.md § Go Idioms).
func newWorkload(name string, pods int, declared PodSpecification, required requiredCapacity) (Workload, error) {
	if name == "" {
		return Workload{}, errors.New("a Workload carries the name its Pods are labelled with, so an empty name is refused")
	}
	if pods < 1 {
		return Workload{}, fmt.Errorf("a Workload declares at least one Pod, so a count of %d Pods is refused", pods)
	}
	if len(declared.settings) == 0 {
		return Workload{}, errors.New("a Workload's Pods are deployed with the specification it declares, so a specification holding no setting is refused")
	}
	if required.declared && required.pods < 1 {
		return Workload{}, fmt.Errorf("a Required Capacity names at least one Pod, so a minimum of %d Pods is refused", required.pods)
	}
	if required.declared && required.pods > pods {
		return Workload{}, fmt.Errorf("a Workload of %d Pods could never keep %d Pods Running, so that Required Capacity is refused", pods, required.pods)
	}

	return Workload{name: name, pods: pods, specification: declared, required: required}, nil
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

// RequiredCapacity is the fewest Pods this Workload must keep Running through a
// Failure, and whether it declared such a minimum at all.
func (w Workload) RequiredCapacity() (pods int, declared bool) {
	return w.required.pods, w.required.declared
}

// Equal reports whether two Workloads declare the same Pods under the same
// specification, at the same Required Capacity. It is the equality go-cmp
// compares through, since the type's fields are unexported.
func (w Workload) Equal(other Workload) bool {
	return w.name == other.name && w.pods == other.pods &&
		w.specification.Equal(other.specification) && w.required == other.required
}
