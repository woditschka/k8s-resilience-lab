package scenario

import (
	"errors"
	"fmt"
)

// Workload is one group of Pod replicas deployed with a predefined Pod
// specification and no setting of its own.
type Workload struct {
	name          string
	pods          int
	specification PodSpecification
	required      requiredCapacity
}

// A Workload declaring no minimum is judged by no Failure tolerance, and a
// minimum of no Pod is a declaration rather than the absence of one.
type requiredCapacity struct {
	pods     int
	declared bool
}

// NewWorkload returns a Workload that declares no Required Capacity.
func NewWorkload(name string, pods int, declared PodSpecification) (Workload, error) {
	return newWorkload(name, pods, declared, requiredCapacity{})
}

// WithRequiredCapacity refuses a minimum above the Pods declared and one below
// a single Pod.
func (w Workload) WithRequiredCapacity(pods int) (Workload, error) {
	return newWorkload(w.name, w.pods, w.specification, requiredCapacity{pods: pods, declared: true})
}

// newWorkload is the one validation home, so the copies declaring a Required
// Capacity are checked alike.
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

func (w Workload) Name() string {
	return w.name
}

func (w Workload) Pods() int {
	return w.pods
}

func (w Workload) PodSpecification() PodSpecification {
	return w.specification
}

func (w Workload) RequiredCapacity() (pods int, declared bool) {
	return w.required.pods, w.required.declared
}

// Equal is the equality go-cmp compares through, since the type's fields are
// unexported.
func (w Workload) Equal(other Workload) bool {
	return w.name == other.name && w.pods == other.pods &&
		w.specification.Equal(other.specification) && w.required == other.required
}
