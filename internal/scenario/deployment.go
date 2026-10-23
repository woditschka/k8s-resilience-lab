package scenario

import (
	"errors"
	"fmt"
)

// Deployment is one group of Pod replicas deployed with a predefined Pod
// specification and no setting of its own.
type Deployment struct {
	name          string
	replicas      int
	specification PodSpecification
	tolerance     failureTolerance
}

// A Deployment declaring no Failure tolerance is judged by none, and a
// tolerance of no Pod is a declaration rather than the absence of one.
type failureTolerance struct {
	pods     int
	declared bool
}

// NewDeployment returns a Deployment that declares no Failure tolerance.
func NewDeployment(name string, replicas int, declared PodSpecification) (Deployment, error) {
	return newDeployment(name, replicas, declared, failureTolerance{})
}

// WithFailureTolerance refuses a tolerance below zero and one of every replica
// or more, since a Deployment losing every Pod has survived nothing.
func (d Deployment) WithFailureTolerance(pods int) (Deployment, error) {
	return newDeployment(d.name, d.replicas, d.specification, failureTolerance{pods: pods, declared: true})
}

// newDeployment is the one validation home, so the copies declaring a Failure
// tolerance are checked alike.
func newDeployment(name string, replicas int, declared PodSpecification, tolerance failureTolerance) (Deployment, error) {
	if name == "" {
		return Deployment{}, errors.New("a Deployment carries the name its Pods are labelled with, so an empty name is refused")
	}
	if replicas < 1 {
		return Deployment{}, fmt.Errorf("a Deployment declares at least one replica, so a count of %d replicas is refused", replicas)
	}
	if len(declared.settings) == 0 {
		return Deployment{}, errors.New("a Deployment's Pods are deployed with the specification it declares, so a specification holding no setting is refused")
	}
	if tolerance.declared && tolerance.pods < 0 {
		return Deployment{}, fmt.Errorf("a Failure tolerance counts the Pods a Deployment survives losing, from zero up, so a tolerance of %d Pods is refused", tolerance.pods)
	}
	if tolerance.declared && tolerance.pods >= replicas {
		return Deployment{}, fmt.Errorf("a Deployment of %d replicas survives losing fewer Pods than its replicas, so a Failure tolerance of %d Pods is refused", replicas, tolerance.pods)
	}

	return Deployment{name: name, replicas: replicas, specification: declared, tolerance: tolerance}, nil
}

func (d Deployment) Name() string {
	return d.name
}

func (d Deployment) Replicas() int {
	return d.replicas
}

func (d Deployment) PodSpecification() PodSpecification {
	return d.specification
}

func (d Deployment) FailureTolerance() (pods int, declared bool) {
	return d.tolerance.pods, d.tolerance.declared
}

// Equal is the equality go-cmp compares through, since the type's fields are
// unexported.
func (d Deployment) Equal(other Deployment) bool {
	return d.name == other.name && d.replicas == other.replicas &&
		d.specification.Equal(other.specification) && d.tolerance == other.tolerance
}
