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
	tolerance     FailureTolerance
	// floor is derived once, from the Topology the tolerance was declared
	// against, so it never follows where the Pods landed.
	floor    int
	declares bool
}

// ToleratedLoss is which part of a Topology a Failure tolerance counts.
type ToleratedLoss string

const (
	// ZonesLost counts the Zones a Deployment survives losing.
	ZonesLost ToleratedLoss = "Zone"
	// NodesLost counts the Nodes a Deployment survives losing.
	NodesLost ToleratedLoss = "Node"
)

// FailureTolerance carries no invariant of its own: the Deployment declaring it
// refuses a count its Topology cannot lose.
type FailureTolerance struct {
	Lost  ToleratedLoss
	Count int
}

// Reads is the count with what it counts, as a report line states it.
func (f FailureTolerance) Reads() string {
	if f.Count == 1 {
		return "1 " + string(f.Lost)
	}

	return fmt.Sprintf("%d %ss", f.Count, f.Lost)
}

// NewDeployment returns a Deployment that declares no Failure tolerance.
func NewDeployment(name string, replicas int, declared PodSpecification) (Deployment, error) {
	return newDeployment(name, replicas, declared)
}

// WithFailureTolerance derives the Floor from the Topology the tolerance is
// declared against. A count below zero is refused, and so is one reaching every
// Zone or Node the Topology holds, since a Deployment losing all of them has
// survived nothing.
func (d Deployment) WithFailureTolerance(tolerance FailureTolerance, against Topology) (Deployment, error) {
	floor, err := floorOf(d.replicas, tolerance, against)
	if err != nil {
		return Deployment{}, err
	}
	d.tolerance, d.floor, d.declares = tolerance, floor, true

	return d, nil
}

func newDeployment(name string, replicas int, declared PodSpecification) (Deployment, error) {
	if name == "" {
		return Deployment{}, errors.New("a Deployment carries the name its Pods are labelled with, so an empty name is refused")
	}
	if replicas < 1 {
		return Deployment{}, fmt.Errorf("a Deployment declares at least one replica, so a count of %d replicas is refused", replicas)
	}
	if len(declared.settings) == 0 {
		return Deployment{}, errors.New("a Deployment's Pods are deployed with the specification it declares, so a specification holding no setting is refused")
	}

	return Deployment{name: name, replicas: replicas, specification: declared, floor: replicas}, nil
}

// floorOf takes the Pods an even spread puts on the declared Zones or Nodes
// holding the most of them: each holds the quotient, and the first of them one
// of the remainder each.
func floorOf(replicas int, tolerance FailureTolerance, against Topology) (int, error) {
	var parts int
	switch tolerance.Lost {
	case ZonesLost:
		parts = len(against.Zones())
	case NodesLost:
		parts = len(against.Nodes())
	default:
		return 0, fmt.Errorf("a Failure tolerance counts Zones or Nodes, so one counting %q is refused", tolerance.Lost)
	}
	if tolerance.Count < 0 {
		return 0, fmt.Errorf("a Failure tolerance counts the %ss a Deployment survives losing, from zero up, "+
			"so a tolerance of %d %ss is refused", tolerance.Lost, tolerance.Count, tolerance.Lost)
	}
	if tolerance.Count >= parts {
		return 0, fmt.Errorf("a Deployment survives losing fewer %ss than the %d its Topology holds, "+
			"so a Failure tolerance of %s is refused", tolerance.Lost, parts, tolerance.Reads())
	}

	each, remaining := replicas/parts, replicas%parts

	return replicas - (tolerance.Count*each + min(tolerance.Count, remaining)), nil
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

func (d Deployment) FailureTolerance() (tolerance FailureTolerance, declared bool) {
	return d.tolerance, d.declares
}

// Floor is every replica for a Deployment declaring no Failure tolerance.
func (d Deployment) Floor() int {
	return d.floor
}

// Equal is the equality go-cmp compares through, since the type's fields are
// unexported.
func (d Deployment) Equal(other Deployment) bool {
	return d.name == other.name && d.replicas == other.replicas &&
		d.specification.Equal(other.specification) && d.tolerance == other.tolerance &&
		d.floor == other.floor && d.declares == other.declares
}
