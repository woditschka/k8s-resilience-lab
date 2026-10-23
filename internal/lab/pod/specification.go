// Package pod holds what a Pod carries, as a Scenario writes it: the predefined
// Pod specification, its Customizations, settings under Kubernetes' own names,
// the Startup time and full access. The qualifier reads as the subject:
// pod.Predefined, pod.Toleration.
//
// It imports neither internal/lab nor the node package.
package pod

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/cluster"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// Specification carries the immutable domain value and, beside it, the full
// accesses its Deployment line wrote, because the domain keeps no Kubernetes type
// and no function value.
//
// A Customization carries no error of its own, so a refusal travels here, and
// the Given declaring the Deployment ends the Scenario with it.
type Specification struct {
	written  SpecificationName
	declared scenario.PodSpecification
	accesses []cluster.PodSpecAccessInPlace
	refused  error
}

// SpecificationName is also the intent the declaring Step reports.
type SpecificationName string

// Predefined takes the Startup time as a named argument, so a definition
// leaving it out does not compile. The domain value holds the lab's two
// tolerations from the start, so a definition naming LeaveUnresponsiveNodes
// first keeps them in that place.
func Predefined(name SpecificationName, startup StartupTime, settings ...Customization) Specification {
	defined := Specification{written: name}
	stated, err := startup.stated()
	if err != nil {
		defined.refused = err

		return defined
	}
	defined.declared, defined.refused = scenario.NewPodSpecification(string(name), stated)
	if defined.refused != nil {
		return defined
	}

	return defined.Customize(settings...)
}

// Customization has one unexported method, so the cases in this package are
// every Customization there is.
type Customization interface {
	customize(Specification) Specification
}

func (s Specification) holding(setting scenario.Setting) Specification {
	customized := s
	customized.declared = s.declared.With(setting)

	return customized
}

// The first refusal stands, since the Scenario ends at it whatever is written
// after.
func (s Specification) refusing(refused error) Specification {
	customized := s
	if customized.refused == nil {
		customized.refused = refused
	}

	return customized
}

// StartupTime is built by the Startup forms alone, so the zero value names none
// and is refused wherever it is written.
type StartupTime struct {
	built   bool
	taken   scenario.StartupTime
	refused error
}

var errNoStartupTime = errors.New("a Pod specification names the Startup time its Pods take, " +
	"and a pod.StartupTime no Startup form built names no Startup time, so it is refused")

func (s StartupTime) stated() (scenario.StartupTime, error) {
	if !s.built {
		return scenario.StartupTime{}, errNoStartupTime
	}

	return s.taken, s.refused
}

func (s StartupTime) customize(specification Specification) Specification {
	stated, err := s.stated()
	if err != nil {
		return specification.refusing(err)
	}
	customized := specification
	customized.declared = specification.declared.WithStartupTime(stated)

	return customized
}

// StartupSeconds states one time for every Pod, where StartupSecondsBetween
// draws each Pod's own time between the two bounds. A refused Startup time ends
// the Scenario at the Given declaring the Deployment.
func StartupSeconds(seconds int) StartupTime {
	return StartupSecondsBetween(seconds, seconds)
}

func StartupSecondsBetween(shortest, longest int) StartupTime {
	return StartupMillisBetween(shortest*millisPerSecond, longest*millisPerSecond)
}

// StartupMillis writes a time under a second or one that is no whole second.
// Every form states the one Startup time, so of two the later stands whichever
// unit each was written in.
func StartupMillis(millis int) StartupTime {
	return StartupMillisBetween(millis, millis)
}

func StartupMillisBetween(shortest, longest int) StartupTime {
	taken, err := scenario.NewStartupTime(shortest, longest)

	return StartupTime{built: true, taken: taken, refused: err}
}

const millisPerSecond = int(time.Second / time.Millisecond)

type fullAccess struct {
	change cluster.PodSpecAccess
}

func (f fullAccess) customize(specification Specification) Specification {
	customized := specification
	customized.accesses = append(slices.Clone(specification.accesses), cluster.PodSpecAccessInPlace{
		Access:        f.change,
		AfterSettings: len(specification.declared.Settings()),
	})

	return customized
}

// FullAccess reaches a Pod specification setting the lab does not name. It
// applies in its turn among the other Customizations, so the later of two
// naming one field stands. Whatever it changes, the lab keeps the label its
// Pods are counted by and their waiting time before leaving an unresponsive
// Node.
func FullAccess(change cluster.PodSpecAccess) Customization {
	return fullAccess{change: change}
}

// Customize leaves the receiver unchanged, so the next Deployment declaring the
// predefined specification gets it as defined. A value only the control plane
// refuses ends the Scenario at the Wait that created the Deployment, because the
// lab does not own Kubernetes' admissible values.
func (s Specification) Customize(customizations ...Customization) Specification {
	customized := s
	for _, customization := range customizations {
		customized = customization.customize(customized)
	}

	return customized
}

func (s Specification) Declared() (scenario.PodSpecification, error) {
	return s.declared, s.refused
}

func (s Specification) Accesses() []cluster.PodSpecAccessInPlace {
	return slices.Clone(s.accesses)
}

// Reads reads a refused specification as its name alone, since it holds no
// Startup time.
func (s Specification) Reads() string {
	if s.refused != nil || s.declared.Name() == "" {
		return string(s.written)
	}
	startup := s.declared.StartupTime()
	if startup.Shortest() == startup.Longest() {
		return fmt.Sprintf("%s, each Pod starting in %s", s.written, startup.Shortest())
	}

	return fmt.Sprintf("%s, each Pod starting in %s to %s", s.written, startup.Shortest(), startup.Longest())
}

// A full access is a function, which reads as nothing a reader could check, so
// the line says one applies.
const aFullAccessApplies = "a full access to the Kubernetes Pod specification applies"

// Carries reports a group of settings as the settings it stands for, since the
// domain holds those alone.
func (s Specification) Carries() []string {
	var lines []string
	for _, held := range s.declared.Settings() {
		lines = append(lines, asAManifestWritesIt(held))
	}
	for range s.accesses {
		lines = append(lines, aFullAccessApplies)
	}

	return lines
}

func asAManifestWritesIt(declared scenario.Setting) string {
	switch known := declared.(type) {
	case scenario.TopologySpread:
		return fmt.Sprintf("topologySpreadConstraints: {topologyKey: %s, maxSkew: %d, whenUnsatisfiable: %s, nodeTaintsPolicy: %s}",
			known.TopologyKey(), known.MaxSkew(), known.WhenUnsatisfiable(), known.NodeTaintsPolicy())
	case scenario.Toleration:
		return asAManifestWritesTheToleration(known)
	case scenario.Request:
		return fmt.Sprintf("resources.requests: {cpu: %s, memory: %s}", known.CPU(), known.Memory())
	default:
		// The domain's Setting is sealed to the three cases above; one added
		// there reads as its Go value until it is named.
		return fmt.Sprint(known)
	}
}

// A toleration of a NoSchedule taint reads with no waiting time, because its
// effect takes none.
func asAManifestWritesTheToleration(toleration scenario.Toleration) string {
	fields := []string{"key: " + toleration.Key(), "operator: " + string(toleration.Operator())}
	if toleration.Operator().TakesAValue() {
		fields = append(fields, "value: "+toleration.Value())
	}
	fields = append(fields, "effect: "+string(toleration.Effect()))
	if toleration.Effect().TakesAWaitingTime() {
		fields = append(fields, fmt.Sprintf("tolerationSeconds: %d", toleration.TolerationSeconds()))
	}

	return "tolerations: {" + strings.Join(fields, ", ") + "}"
}

// Equal is the equality go-cmp compares through, since the type's fields are
// unexported.
func (s Specification) Equal(other Specification) bool {
	return s.declared.Equal(other.declared) && slices.EqualFunc(s.accesses, other.accesses, inTheSamePlace)
}

// The changes themselves are never compared: Go cannot compare two function
// values.
func inTheSamePlace(one, another cluster.PodSpecAccessInPlace) bool {
	return one.AfterSettings == another.AfterSettings
}
