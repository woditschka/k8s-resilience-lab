// Package pod holds what a Pod carries, as a Scenario writes it: the predefined
// Pod specification a Workload declares, the Customizations a definition and a
// Workload line write, the settings under Kubernetes' own names, the Startup time
// its Pods take, and full access. A Scenario reaches each through the package
// name, so the qualifier reads as the subject: pod.Predefined, pod.Toleration,
// pod.StartupMillisBetween, per
// docs/adr/2026-10-18-scenario-code-stays-idiomatic-go-read-by-subject.md.
//
// It imports neither internal/lab nor the node package: internal/lab reads a
// specification through its exported read-only methods.
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

// Specification is the Pod specification a Workload Step declares: one the
// Scenarios define through Predefined, as the Startup time it names and the
// settings it sets under Kubernetes' own names, which a Scenario may customize
// before its Workload is deployed.
//
// It carries the immutable domain value rather than being one, and beside it the
// full accesses its Workload line wrote, each in the place it was written: the
// domain core keeps no Kubernetes type and no function value, per
// docs/adr/2026-10-05-workload-declares-a-predefined-pod-specification.md. The
// name it was defined under is kept beside the value too, so a line names what
// was written, refused or not.
//
// A Customization carries no error of its own, so a definition, a Startup time or
// a setting the lab refuses travels here instead, and the Given declaring a
// Workload of this specification ends the Scenario with it, per
// docs/adr/2026-10-18-pod-and-node-settings-are-keyed-literals-refused-at-their-given.md.
type Specification struct {
	written  SpecificationName
	declared scenario.PodSpecification
	accesses []cluster.PodSpecAccessInPlace
	refused  error
}

// SpecificationName is the name the Scenarios define a predefined Pod
// specification under, which is the intent the Step declaring it reports.
type SpecificationName string

// Predefined defines a predefined Pod specification: its name, the Startup time
// its Pods take, then the settings it sets. The lab defines none of its own: the
// Scenarios' place defines each one once, per
// docs/adr/2026-10-18-every-pod-specification-is-defined-by-the-scenarios.md.
//
// The Startup time is a named argument, so a definition leaving it out does not
// compile. The domain value holds the lab's two tolerations from the start, so a
// definition naming the Scenarios' LeaveUnresponsiveNodes first keeps them in
// that place.
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

// Customization is one change a definition or a Workload line writes: a setting
// of the specification under the name Kubernetes gives it, a group of settings
// the Scenarios name, the Startup time its Pods take, or full access to the
// Kubernetes Pod specification for a setting the lab does not name. Its one
// method is unexported, so the cases in this package are every Customization
// there is and a Scenario can write no value of its own.
type Customization interface {
	// customize is this change applied to a specification: the changed copy,
	// leaving the specification it was applied to as it was.
	customize(Specification) Specification
}

// holding is this specification holding one more setting, the later setting for
// a field standing.
func (s Specification) holding(setting scenario.Setting) Specification {
	customized := s
	customized.declared = s.declared.With(setting)

	return customized
}

// refusing is this specification carrying refused as its refusal. The first
// refusal stands, since the Scenario ends at it whatever is written after.
func (s Specification) refusing(refused error) Specification {
	customized := s
	if customized.refused == nil {
		customized.refused = refused
	}

	return customized
}

// StartupTime is the Startup time a definition names and a Customization
// states: the domain value its Pods take, or the refusal the domain gave the time
// written. Only the four Startup forms build one, so the zero value names no
// Startup time and is refused wherever it is written (docs/prd.md REQ-SIM-015
// edge case 1).
type StartupTime struct {
	built   bool
	taken   scenario.StartupTime
	refused error
}

// errNoStartupTime is the refusal of the zero StartupTime.
var errNoStartupTime = errors.New("a Pod specification names the Startup time its Pods take, " +
	"and a pod.StartupTime no Startup form built names no Startup time, so it is refused")

// stated is the domain value this Startup time holds, or why it holds none.
func (s StartupTime) stated() (scenario.StartupTime, error) {
	if !s.built {
		return scenario.StartupTime{}, errNoStartupTime
	}

	return s.taken, s.refused
}

// customize states this Startup time on a specification.
func (s StartupTime) customize(specification Specification) Specification {
	stated, err := s.stated()
	if err != nil {
		return specification.refusing(err)
	}
	customized := specification
	customized.declared = specification.declared.WithStartupTime(stated)

	return customized
}

// StartupSeconds is the Startup time every Pod of a Workload takes to become
// Ready once it is placed, and StartupSecondsBetween the shortest and the longest
// each Pod takes its own time between, both in the whole seconds a Wait's time is
// stated in. A definition names one as its second argument, and a Workload line
// may state another as a Customization (docs/prd.md REQ-SIM-012).
//
// A Startup time the domain refuses ends the Scenario at the Given declaring the
// Workload, because a definition and a Customization are both written inside
// that Given's line.
func StartupSeconds(seconds int) StartupTime {
	return StartupSecondsBetween(seconds, seconds)
}

func StartupSecondsBetween(shortest, longest int) StartupTime {
	return StartupMillisBetween(shortest*millisPerSecond, longest*millisPerSecond)
}

// StartupMillis and StartupMillisBetween state the same Startup time as the
// seconds forms, in whole milliseconds, which is how a time under a second or one
// that is no whole second is written. A range whose bounds would mix units is
// written here. Every form states the one Startup time, so of two Customizations
// stating one the later stands whichever unit each was written in.
func StartupMillis(millis int) StartupTime {
	return StartupMillisBetween(millis, millis)
}

func StartupMillisBetween(shortest, longest int) StartupTime {
	taken, err := scenario.NewStartupTime(shortest, longest)

	return StartupTime{built: true, taken: taken, refused: err}
}

// millisPerSecond turns a time a Scenario states in whole seconds into the whole
// milliseconds a Startup time is built from.
const millisPerSecond = int(time.Second / time.Millisecond)

// fullAccess is the Customization full access is: the change a Scenario wrote,
// which the mapper runs once the settings written before it are transcribed.
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

// FullAccess is the Customization a Scenario writes where the lab names no
// setting it needs: its own change to the Kubernetes Pod specification, reaching
// every setting there is. It is written among the other Customizations and
// applies in its turn, so the later of two naming one Kubernetes field is the one
// that stands.
//
// The Scenario writing it names that specification under the one Kubernetes
// package holding it, `pod.FullAccess(func(spec *corev1.PodSpec) { ... })`, which
// is the stated exception to the lab's own names a Scenario otherwise reads in.
// What the lab itself depends on holds whatever a full access leaves out: the
// label its Workload's Pods are counted by, and the waiting time before they
// leave an unresponsive Node, per
// docs/adr/2026-10-05-workload-declares-a-predefined-pod-specification.md.
func FullAccess(change cluster.PodSpecAccess) Customization {
	return fullAccess{change: change}
}

// Customize is this specification with the Customizations written: a copy holding
// them, keeping the last setting written for a field, and leaving the
// specification it was applied to as it was, so the predefined one a Scenario
// customized is the one the next Workload declaring it gets.
//
// Every definition is written through it too, which is what makes each one a
// worked example of what a Scenario customizes with. A value the control plane
// refuses is refused there and not here: the lab does not own Kubernetes'
// admissible values, so such a Customization ends the Scenario at the Wait that
// deployed its Workload, which is the Step a declaration reaches the cluster
// through, per
// docs/adr/2026-10-05-workload-declares-a-predefined-pod-specification.md.
func (s Specification) Customize(customizations ...Customization) Specification {
	customized := s
	for _, customization := range customizations {
		customized = customization.customize(customized)
	}

	return customized
}

// Declared is the domain's Pod specification this one declares, or the first
// refusal it carries, which the Given declaring its Workload ends the Scenario
// with.
func (s Specification) Declared() (scenario.PodSpecification, error) {
	return s.declared, s.refused
}

// Accesses are the full accesses this specification was customized with, each in
// the place it was written: what the domain holds of a specification is its
// settings alone, so they are deployed beside its Workload.
func (s Specification) Accesses() []cluster.PodSpecAccessInPlace {
	return slices.Clone(s.accesses)
}

// Reads is how the Step declaring this specification names it: the name it was
// defined under, then the Startup time its Pods take. A specification refused, or
// the zero one no definition built, holds no Startup time, so it reads as its
// name alone.
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

// aFullAccessApplies is how a Given reports a full access its Workload's Pods
// carry: the change itself is a function, which reads as nothing a reader could
// check, so the line says one applies.
const aFullAccessApplies = "a full access to the Kubernetes Pod specification applies"

// Carries is how the Given declaring this specification reports what its Pods
// carry: one line per setting the domain holds, in the order it holds them, then
// one line per full access, in the order written. A group of settings reports as
// the settings it stands for, since the domain holds those alone.
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

// asAManifestWritesIt is one setting as a Pod manifest writes it: the Pod-spec
// field it fills, then each of its fields under its Kubernetes API name, in the
// order the lab's setting takes them, with the value spelled as Kubernetes
// spells it.
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
		// The domain's Setting is sealed to the three cases above, so no setting
		// reaches here; one added there reads as its Go value until it is named.
		return fmt.Sprint(known)
	}
}

// asAManifestWritesTheToleration is one toleration as a Pod manifest writes it:
// its value where its operator takes one, and its waiting time where its effect
// takes one, so a toleration of a NoSchedule taint reads with no waiting time.
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

// Equal reports whether two specifications name the same settings under the same
// name and hold as many full accesses in the same places. It is the equality
// go-cmp compares through, since the type's fields are unexported.
func (s Specification) Equal(other Specification) bool {
	return s.declared.Equal(other.declared) && slices.EqualFunc(s.accesses, other.accesses, inTheSamePlace)
}

// inTheSamePlace reports whether two full accesses were written in the same
// place. The changes themselves are never compared: Go cannot compare two
// function values, and what a full access does is read off the Pod template its
// Workload was deployed with.
func inTheSamePlace(one, another cluster.PodSpecAccessInPlace) bool {
	return one.AfterSettings == another.AfterSettings
}
