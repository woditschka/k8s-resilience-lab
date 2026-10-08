package lab

import (
	"slices"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/cluster"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// evenSkew is the maximum Skew that makes a spread even: the fullest and the
// emptiest domain differ by at most one Pod. It is the definitions' own value and
// no Scenario's, which writes the Skew it wants itself.
const evenSkew = 1

// The whole units a Scenario states a cpu and a memory amount in, as the lab's
// own quantities hold them: a core is this many millicores, a gibibyte this many
// mebibytes.
const (
	millicoresPerCore    = 1000
	mebibytesPerGibibyte = 1024
)

// PodSpecification is the Pod specification a Workload Step declares: one of the
// lab's predefined ones, each defined below as the settings it sets, under
// Kubernetes' own names, and each one a Scenario may customize before its
// Workload is deployed.
//
// It carries the immutable domain value rather than being one, and beside it the
// full accesses its Workload line wrote, each in the place it was written: the
// domain core keeps no Kubernetes type and no function value, per
// docs/adr/2026-10-05-workload-declares-a-predefined-pod-specification.md.
//
// A Customization carries no error of its own, so a Startup time the domain
// refuses travels here instead, and the Given declaring a Workload of this
// specification ends the Scenario with it (docs/prd.md REQ-SIM-012 edge case 2).
type PodSpecification struct {
	declared scenario.PodSpecification
	accesses []cluster.PodSpecAccessInPlace
	refused  error
}

// Customization is one change a Scenario writes on its Workload line: a setting
// of the specification it declares, named under the field names Kubernetes gives
// that setting, the Startup time its Pods take, or full access to the Kubernetes
// Pod specification for a setting the lab's own vocabulary does not name. Its one
// method is unexported, so the three cases below are every Customization there is
// and a Scenario can write no value of its own.
type Customization interface {
	// customize is this change applied to a specification: the changed copy,
	// leaving the specification it was applied to as it was.
	customize(PodSpecification) PodSpecification
}

// setting is the Customization one setting of the lab's own vocabulary is: the
// immutable domain value a specification holds that setting as.
type setting struct {
	declares scenario.Setting
}

func (s setting) customize(specification PodSpecification) PodSpecification {
	customized := specification
	customized.declared = specification.declared.With(s.declares)

	return customized
}

// startupTime is the Customization a Startup time is: the domain value its
// Pods take, or the refusal the domain gave the time a Scenario wrote. The first
// refusal stands, since the Scenario ends at it whatever is written after.
type startupTime struct {
	stated  scenario.StartupTime
	refused error
}

func (s startupTime) customize(specification PodSpecification) PodSpecification {
	customized := specification
	if s.refused != nil && customized.refused == nil {
		customized.refused = s.refused
	}
	if s.refused == nil {
		customized.declared = specification.declared.WithStartupTime(s.stated)
	}

	return customized
}

// fullAccess is the Customization full access is: the change a Scenario wrote,
// which the mapper runs once the settings written before it are transcribed.
type fullAccess struct {
	change cluster.PodSpecAccess
}

func (f fullAccess) customize(specification PodSpecification) PodSpecification {
	customized := specification
	customized.accesses = append(slices.Clone(specification.accesses), cluster.PodSpecAccessInPlace{
		Access:        f.change,
		AfterSettings: len(specification.declared.Settings()),
	})

	return customized
}

// FullAccess is the Customization a Scenario writes where the lab's own
// vocabulary names no setting it needs: its own change to the Kubernetes Pod
// specification, reaching every setting there is. It is written among the other
// Customizations and applies in its turn, so the later of two naming one
// Kubernetes field is the one that stands.
//
// The Scenario writing it names that specification under the one Kubernetes
// package holding it, `lab.FullAccess(func(pod *corev1.PodSpec) { ... })`, which
// is the stated exception to the one vocabulary a Scenario otherwise reads in.
// What the lab itself depends on holds whatever a full access leaves out: the
// label its Workload's Pods are counted by, and the waiting time before they
// leave an unresponsive Node, per
// docs/adr/2026-10-05-workload-declares-a-predefined-pod-specification.md.
func FullAccess(change cluster.PodSpecAccess) Customization {
	return fullAccess{change: change}
}

// specification is the base every predefined specification starts from: it names
// the specification and carries the lab's own settings, the tolerations that keep
// a Pod on a Node that stopped responding for the unresponsive-Node wait and no
// longer. They stand here rather than in the mapper, so a reader of a definition
// below sees the whole of what decides a Placement one hop away.
func specification(name string) PodSpecification {
	return PodSpecification{declared: scenario.NewPodSpecification(name,
		toleration(key(NodeNotReady), operator(Exists), effect(NoExecute), tolerationSeconds(scenario.UnresponsiveNodeWait)),
		toleration(key(NodeUnreachable), operator(Exists), effect(NoExecute), tolerationSeconds(scenario.UnresponsiveNodeWait)),
	)}
}

// DefaultPlacement is the specification that sets no spread at all. Where its
// Workload's Pods land is the scheduler's own choice, so a Scenario declaring it
// asserts only what holds of every Placement.
func DefaultPlacement() PodSpecification {
	return specification("Default placement")
}

// ResilientPlacement is the specification that spreads a Workload's Pods as
// evenly over the Zones of its Topology, and over its Nodes, as the control plane
// can place them, so the loss of one Node or one Zone takes the fewest Pods with
// it.
func ResilientPlacement() PodSpecification {
	return specification("Resilient placement").Customize(
		TopologySpread(TopologyKey(ZoneLabel), MaxSkew(evenSkew), WhenUnsatisfiable(DoNotSchedule), NodeTaintsPolicy(Honor)),
		TopologySpread(TopologyKey(HostnameLabel), MaxSkew(evenSkew), WhenUnsatisfiable(DoNotSchedule), NodeTaintsPolicy(Honor)),
	)
}

// Customize is this specification with the Customizations written: a copy holding
// them, keeping the last setting written for a field, and leaving the
// specification it was applied to as it was, so the predefined one a Scenario
// customized is the one the next Workload declaring it gets.
//
// The definitions above are written through it too, which is what makes each of
// them a worked example of the vocabulary a Scenario customizes in. A value the
// control plane refuses is refused there and not here: the lab does not own
// Kubernetes' admissible values, so such a Customization ends the Scenario at the
// Wait that deployed its Workload, which is the Step a declaration reaches the
// cluster through, per
// docs/adr/2026-10-05-workload-declares-a-predefined-pod-specification.md.
func (s PodSpecification) Customize(customizations ...Customization) PodSpecification {
	customized := s
	for _, customization := range customizations {
		customized = customization.customize(customized)
	}

	return customized
}

// name is the intent this specification stands for, which is what the Step
// declaring it reports.
func (s PodSpecification) name() string {
	return s.declared.Name()
}

// Equal reports whether two specifications name the same settings under the same
// name and hold as many full accesses in the same places. It is the equality
// go-cmp compares through, since the type's fields are unexported.
func (s PodSpecification) Equal(other PodSpecification) bool {
	return s.declared.Equal(other.declared) && slices.EqualFunc(s.accesses, other.accesses, inTheSamePlace)
}

// inTheSamePlace reports whether two full accesses were written in the same
// place. The changes themselves are never compared: Go cannot compare two
// function values, and what a full access does is read off the Pod template its
// Workload was deployed with.
func inTheSamePlace(one, another cluster.PodSpecAccessInPlace) bool {
	return one.AfterSettings == another.AfterSettings
}

// TopologySpread is the setting that decides where a Workload's Pods land, and
// Request what one of its Pods asks for: the two Customizations internal/lab
// offers a Scenario. toleration is the third setting a specification holds, and
// stays the lab's own, because a Scenario states no waiting time of its own
// (REQ-SIM-008).
//
// Each takes the fields Kubernetes gives that setting, in Kubernetes' own order.
// No two fields of one setting share a type, so a swapped argument does not
// compile.
func TopologySpread(
	topologyKey string,
	maxSkew int,
	whenUnsatisfiable scenario.UnsatisfiableConstraintAction,
	nodeTaintsPolicy scenario.NodeInclusionPolicy,
) Customization {
	return setting{declares: scenario.NewTopologySpread(topologyKey, maxSkew, whenUnsatisfiable, nodeTaintsPolicy)}
}

// Request is the setting that decides what one Pod of a Workload asks for: the
// cpu and the memory the scheduler fits against the room a Node reports. Both
// are stated, because a Pod asking for one of them and not the other is a
// request the lab's own vocabulary does not name.
func Request(cpu scenario.CPU, memory scenario.Memory) Customization {
	return setting{declares: scenario.NewRequest(cpu, memory)}
}

// StartupSeconds is the Startup time every Pod of a Workload takes to become
// Ready once it is placed, and StartupSecondsBetween the shortest and the longest
// each Pod takes its own time between. Both are written beside a Request, in the
// whole seconds a Wait's time is stated in, and a Workload whose specification
// states neither takes the lab's own, scenario.DefaultStartupTime (docs/prd.md
// REQ-SIM-012).
//
// A Startup time the domain refuses ends the Scenario at the Given declaring the
// Workload, because a Customization is written inside that Given's line.
func StartupSeconds(seconds int) Customization {
	return StartupSecondsBetween(seconds, seconds)
}

func StartupSecondsBetween(shortest, longest int) Customization {
	stated, err := scenario.NewStartupTime(shortest, longest)

	return startupTime{stated: stated, refused: err}
}

func toleration(
	key string,
	operator scenario.TolerationOperator,
	effect scenario.TaintEffect,
	tolerationSeconds int64,
) scenario.Toleration {
	return scenario.NewToleration(key, operator, effect, tolerationSeconds)
}

// The field names a setting sets, as Kubernetes names them. Each hands its value
// on unchanged and exists so a definition above, and a Scenario's own
// Customization, state which field they set and to which value. The four of the
// spread setting are offered to Scenarios with it; the four of the toleration
// below stay the lab's own.
func TopologyKey(label string) string {
	return label
}

func MaxSkew(pods int) int {
	return pods
}

func WhenUnsatisfiable(action scenario.UnsatisfiableConstraintAction) scenario.UnsatisfiableConstraintAction {
	return action
}

func NodeTaintsPolicy(policy scenario.NodeInclusionPolicy) scenario.NodeInclusionPolicy {
	return policy
}

// CPU and MemoryGi are the amounts a capacity and a request are stated in: whole
// cores and whole gibibytes, as the lab's own quantities hold them. They are the
// one further place the lab's vocabulary meets Kubernetes' unit, beside
// tolerationSeconds below, and a finer unit joins them as another function rather
// than changing a signature.
func CPU(cores int) scenario.CPU {
	return scenario.CPU(cores * millicoresPerCore)
}

func MemoryGi(gibibytes int) scenario.Memory {
	return scenario.Memory(gibibytes * mebibytesPerGibibyte)
}

func key(taint string) string {
	return taint
}

func operator(matching scenario.TolerationOperator) scenario.TolerationOperator {
	return matching
}

func effect(tolerated scenario.TaintEffect) scenario.TaintEffect {
	return tolerated
}

// tolerationSeconds is the lab's own waiting time as the whole seconds
// Kubernetes holds that field as, which is the one place the lab's vocabulary
// meets Kubernetes' unit.
func tolerationSeconds(leavingAfter time.Duration) int64 {
	return int64(leavingAfter.Seconds())
}
