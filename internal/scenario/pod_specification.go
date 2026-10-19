package scenario

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// The Kubernetes names the lab spells itself: the labels a spread counts its
// domains over, and the taints the control plane adds to a Node it has stopped
// hearing from. This package names no Kubernetes type, so these are the lab's
// own spelling of them, and the mapping test pins each one to the client
// library's constant (docs/architecture-principles.md § Domain Core).
const (
	// ZoneLabel is the well-known label a Zone is a failure domain under, and
	// HostnameLabel the one a Node is one host under.
	ZoneLabel     = "topology.kubernetes.io/zone"
	HostnameLabel = "kubernetes.io/hostname"

	// TaintNodeNotReady is the taint a Node whose Ready condition is no longer
	// true carries, and TaintNodeUnreachable the one a Node the control plane
	// cannot reach carries.
	TaintNodeNotReady    = "node.kubernetes.io/not-ready"
	TaintNodeUnreachable = "node.kubernetes.io/unreachable"
)

// UnsatisfiableConstraintAction is what the scheduler does with a Pod no Node
// can take without widening a spread past the Skew it tolerates.
type UnsatisfiableConstraintAction string

// DoNotSchedule leaves such a Pod Pending, so a spread the Topology cannot
// satisfy is visible rather than silently given up.
const DoNotSchedule UnsatisfiableConstraintAction = "DoNotSchedule"

// NodeInclusionPolicy is whether a Node's taints are read when a spread counts
// the Pods of a domain.
type NodeInclusionPolicy string

// Honor counts only the Nodes whose taints these Pods tolerate, so a Node that
// stopped responding leaves the count its Pods were spread over.
const Honor NodeInclusionPolicy = "Honor"

// TolerationOperator is how a toleration matches the taint it names.
type TolerationOperator string

// Exists matches a taint of that key whatever value the control plane set it to,
// and Equal matches a taint of that key and the value the toleration names.
const (
	Exists TolerationOperator = "Exists"
	Equal  TolerationOperator = "Equal"
)

// TakesAValue reports whether a toleration matching this way names the value of
// the taint it tolerates: Equal does, and Exists matches every value.
func (o TolerationOperator) TakesAValue() bool {
	return o == Equal
}

// TaintEffect is what a taint does to a Pod that does not tolerate it.
type TaintEffect string

// NoExecute moves such a Pod off the Node.
const NoExecute TaintEffect = "NoExecute"

// TakesAWaitingTime reports whether a toleration of a taint of this effect states
// how long a Pod stays on the Node: NoExecute moves a Pod off after it, and
// NoSchedule moves no Pod at all, so Kubernetes takes a waiting time for
// NoExecute alone.
func (e TaintEffect) TakesAWaitingTime() bool {
	return e == NoExecute
}

// UnresponsiveNodeWait is how long a Pod stays on a Node that stopped
// responding, and DefaultUnresponsiveNodeWait the admission default the lab's
// own value replaces.
//
// The lab's value is short so a Scenario stays fast: it is above the interval a
// Placement is polled at and far below the default, which is longer than the whole
// settle wait. It is only part of what a run waits — the control plane's own
// period for noticing adds seconds on top, and the lab sets that period too,
// through the heartbeat factor its cluster container runs with — so every run
// prints the settings it makes and claims no total, per
// docs/adr/2026-10-05-node-failure-withholds-the-nodes-status.md.
const (
	UnresponsiveNodeWait        = 2 * time.Second
	DefaultUnresponsiveNodeWait = 300 * time.Second
)

// StartupTime is the time one placed Pod takes to become Ready: each Pod takes
// its own time between the shortest and the longest, both included, so equal
// bounds are one time every Pod takes. It is no Setting, because no Kubernetes
// field holds it; the simulator reads it off the Pod, per
// docs/adr/2026-10-08-the-lab-owns-the-simulators-stages-for-a-startup-time.md.
type StartupTime struct {
	shortest time.Duration
	longest  time.Duration
}

// NewStartupTime builds the Startup time of that shortest and that longest, in
// whole milliseconds, the finest unit a Scenario states one in.
//
// A negative time is refused, because no Pod is Ready before it is placed, and so
// is a shortest above its longest, which leaves no time to take between them. A
// time of none is admitted: such a Pod is Ready as soon as it is placed. A
// refusal names the bounds as durations, since a Scenario reaches it from whole
// seconds and from whole milliseconds alike.
func NewStartupTime(shortestMillis, longestMillis int) (StartupTime, error) {
	shortest := time.Duration(shortestMillis) * time.Millisecond
	longest := time.Duration(longestMillis) * time.Millisecond
	if shortest < 0 || longest < 0 {
		return StartupTime{}, fmt.Errorf("no Pod is Ready before it is placed, so a Startup time between %s and %s is refused",
			shortest, longest)
	}
	if shortest > longest {
		return StartupTime{}, fmt.Errorf("a shortest Startup time of %s above its longest of %s leaves no time to take, so it is refused",
			shortest, longest)
	}

	return StartupTime{shortest: shortest, longest: longest}, nil
}

// Shortest is the least time a Pod taking this Startup time takes to become Ready.
func (s StartupTime) Shortest() time.Duration {
	return s.shortest
}

// Longest is the most time a Pod taking this Startup time takes to become Ready.
func (s StartupTime) Longest() time.Duration {
	return s.longest
}

// Equal reports whether two Startup times hold the same bounds. It is the
// equality go-cmp compares through, since the type's fields are unexported.
func (s StartupTime) Equal(other StartupTime) bool {
	return s == other
}

// The kinds of setting a Pod specification holds, which is what makes one
// setting of a field replaceable by another of the same field.
const (
	spreadSetting     = "topologySpread"
	tolerationSetting = "toleration"
	requestSetting    = "request"
)

// Setting is one thing a Pod specification sets: where a Workload's Pods land,
// when they leave a Node, or what one of them asks for. Its one method is
// unexported, so the three settings below are every setting there is and a
// specification can hold no value from outside this package.
type Setting interface {
	// field is the field of a Pod specification this setting occupies. A setting
	// written for a field a specification already holds replaces it.
	field() settingField
}

// settingField is the field one setting occupies: which kind of setting it is,
// and the key Kubernetes holds one of that kind per — a topology key for a
// spread, a taint key for a toleration.
type settingField struct {
	kind string
	key  string
}

// TopologySpread is the setting that decides where a Workload's Pods land: how
// evenly they sit over the domains one label names, stated as the Skew the
// Workload tolerates. Every field is named as Kubernetes names it, in a type of
// the lab's own, per
// docs/adr/2026-10-05-workload-declares-a-predefined-pod-specification.md.
type TopologySpread struct {
	topologyKey       string
	maxSkew           int
	whenUnsatisfiable UnsatisfiableConstraintAction
	nodeTaintsPolicy  NodeInclusionPolicy
}

// NewTopologySpread builds one spread setting. It refuses nothing while the
// lab's own definitions are the only builders of one: a Scenario supplying a
// value of its own brings a validated constructor with it, per the same ADR.
func NewTopologySpread(
	topologyKey string,
	maxSkew int,
	whenUnsatisfiable UnsatisfiableConstraintAction,
	nodeTaintsPolicy NodeInclusionPolicy,
) TopologySpread {
	return TopologySpread{
		topologyKey:       topologyKey,
		maxSkew:           maxSkew,
		whenUnsatisfiable: whenUnsatisfiable,
		nodeTaintsPolicy:  nodeTaintsPolicy,
	}
}

// TopologyKey is the label this spread counts its domains over.
func (s TopologySpread) TopologyKey() string {
	return s.topologyKey
}

// MaxSkew is the widest gap between the fullest and the emptiest domain this
// spread tolerates.
func (s TopologySpread) MaxSkew() int {
	return s.maxSkew
}

// WhenUnsatisfiable is what the scheduler does with a Pod this spread leaves no
// room for.
func (s TopologySpread) WhenUnsatisfiable() UnsatisfiableConstraintAction {
	return s.whenUnsatisfiable
}

// NodeTaintsPolicy is whether a Node's taints are read when this spread counts
// a domain's Pods.
func (s TopologySpread) NodeTaintsPolicy() NodeInclusionPolicy {
	return s.nodeTaintsPolicy
}

// Equal reports whether two spreads set every field to the same value. It is the
// equality go-cmp compares through, since the type's fields are unexported.
func (s TopologySpread) Equal(other TopologySpread) bool {
	return s == other
}

// field is the topology key this spread occupies: a specification spreads a
// Workload's Pods over one label once.
func (s TopologySpread) field() settingField {
	return settingField{kind: spreadSetting, key: s.topologyKey}
}

// Toleration is the setting that decides when a Workload's Pods leave a Node:
// the taint they put up with, and for how long. Every field is named as
// Kubernetes names it, in a type of the lab's own, and the waiting time is the
// whole seconds Kubernetes holds that field as, so the setting is that field and
// not a reading of it.
type Toleration struct {
	key               string
	operator          TolerationOperator
	value             string
	effect            TaintEffect
	tolerationSeconds int64
}

// NewToleration builds one toleration setting, refusing nothing for the reason
// NewTopologySpread refuses nothing.
func NewToleration(
	key string,
	operator TolerationOperator,
	value string,
	effect TaintEffect,
	tolerationSeconds int64,
) Toleration {
	return Toleration{key: key, operator: operator, value: value, effect: effect, tolerationSeconds: tolerationSeconds}
}

// Key is the taint this toleration names.
func (t Toleration) Key() string {
	return t.key
}

// Operator is how this toleration matches that taint.
func (t Toleration) Operator() TolerationOperator {
	return t.operator
}

// Value is the value of that taint this toleration matches, which an Equal
// toleration names and an Exists one leaves empty.
func (t Toleration) Value() string {
	return t.value
}

// Effect is the effect of that taint this toleration puts up with.
func (t Toleration) Effect() TaintEffect {
	return t.effect
}

// TolerationSeconds is how many seconds a Pod stays on a Node carrying that
// taint.
func (t Toleration) TolerationSeconds() int64 {
	return t.tolerationSeconds
}

// Equal reports whether two tolerations set every field to the same value. It is
// the equality go-cmp compares through, since the type's fields are unexported.
func (t Toleration) Equal(other Toleration) bool {
	return t == other
}

// field is the taint key this toleration occupies: a specification tolerates one
// taint once.
func (t Toleration) field() settingField {
	return settingField{kind: tolerationSetting, key: t.key}
}

// Request is the setting that decides what one Pod of a Workload asks for: the
// cpu and the memory the scheduler fits against the room a Node reports. It is
// Kubernetes' resources.requests, in types of the lab's own, per
// docs/adr/2026-10-05-workload-declares-a-predefined-pod-specification.md, and it
// states no limit: nothing runs in a Simulated Cluster for a limit to bind.
type Request struct {
	cpu    CPU
	memory Memory
}

// NewRequest builds one request setting, refusing nothing for the reason
// NewTopologySpread refuses nothing: what a Kubernetes quantity admits is the
// control plane's to judge.
func NewRequest(cpu CPU, memory Memory) Request {
	return Request{cpu: cpu, memory: memory}
}

// CPU is how much processor time one Pod with this setting asks for.
func (r Request) CPU() CPU {
	return r.cpu
}

// Memory is how much memory one Pod with this setting asks for.
func (r Request) Memory() Memory {
	return r.memory
}

// Equal reports whether two requests ask for the same of both. It is the
// equality go-cmp compares through, since the type's fields are unexported.
func (r Request) Equal(other Request) bool {
	return r == other
}

// field is the one field a request occupies: a specification states what one Pod
// asks for once, so it carries no key of its own.
func (Request) field() settingField {
	return settingField{kind: requestSetting}
}

// PodSpecification is the settings one Workload's Pods are deployed with, under
// the intent they serve, and the Startup time each of them takes. It holds one
// setting per field, in the order its definition wrote them, which is the order
// the mapper transcribes them in.
//
// Each predefined specification is defined by the Scenarios as the settings it
// sets and the Startup time it names, so what a reader of a definition sees is
// the whole of what decides a Placement, per
// docs/adr/2026-10-18-every-pod-specification-is-defined-by-the-scenarios.md.
type PodSpecification struct {
	name        string
	settings    []Setting
	startupTime StartupTime
}

// NewPodSpecification builds the specification of that name, its Pods taking that
// Startup time, out of the settings written. A specification is declared by its
// name, so one under no name is refused.
//
// It keeps each field once at the value written last for it, and it holds the
// lab's two tolerations at the lab's waiting time whatever is written, because a
// Scenario states no waiting time of its own (docs/prd.md REQ-SIM-008).
func NewPodSpecification(name string, startup StartupTime, settings ...Setting) (PodSpecification, error) {
	if name == "" {
		return PodSpecification{}, errors.New("a Pod specification is declared by its name, so one under no name is refused")
	}

	return PodSpecification{name: name, settings: held(settings), startupTime: startup}, nil
}

// With is this specification customized by the settings named: a copy holding
// them, leaving the specification they were applied to as it was. The name and
// the Startup time it keeps were admitted when this one was built, so only the
// rule over its settings is entered again.
func (s PodSpecification) With(settings ...Setting) PodSpecification {
	customized := s
	customized.settings = held(slices.Concat(s.settings, settings))

	return customized
}

// WithStartupTime is this specification with its Pods taking the Startup time
// stated: a copy, leaving the specification it was stated on as it was.
func (s PodSpecification) WithStartupTime(stated StartupTime) PodSpecification {
	customized := s.With()
	customized.startupTime = stated

	return customized
}

// StartupTime is the time each Pod of this specification takes to become Ready:
// the one its definition named, or the one stated last on it.
func (s PodSpecification) StartupTime() StartupTime {
	return s.startupTime
}

// Name is the intent this specification stands for, which is what the Step
// declaring it reports.
func (s PodSpecification) Name() string {
	return s.name
}

// Settings are the settings this specification holds, in definition order.
func (s PodSpecification) Settings() []Setting {
	return slices.Clone(s.settings)
}

// TopologySpreads are the spread settings of this specification, in definition
// order, which is one topology spread constraint each on the Pod template.
func (s PodSpecification) TopologySpreads() []TopologySpread {
	var spreads []TopologySpread
	for _, setting := range s.settings {
		if spread, isSpread := setting.(TopologySpread); isSpread {
			spreads = append(spreads, spread)
		}
	}

	return spreads
}

// Tolerations are the toleration settings of this specification, in definition
// order, which is one toleration each on the Pod template.
func (s PodSpecification) Tolerations() []Toleration {
	var tolerated []Toleration
	for _, setting := range s.settings {
		if toleration, isToleration := setting.(Toleration); isToleration {
			tolerated = append(tolerated, toleration)
		}
	}

	return tolerated
}

// Equal reports whether two specifications name the same settings under the same
// name, and have their Pods take the same Startup time. It is the equality go-cmp
// compares through, since the type's fields are unexported.
func (s PodSpecification) Equal(other PodSpecification) bool {
	return s.name == other.name && slices.Equal(s.settings, other.settings) && s.startupTime == other.startupTime
}

// held are the settings a specification holds of those written: every field
// once, and the lab's two tolerations at the lab's waiting time.
func held(settings []Setting) []Setting {
	return withTheLabsTolerations(eachFieldOnce(settings))
}

// theLabsTaints are the two taints a Node that stopped responding carries, which
// every specification tolerates for the lab's waiting time and no other.
var theLabsTaints = []string{TaintNodeNotReady, TaintNodeUnreachable}

// theLabsWaitInSeconds is the lab's waiting time as the whole seconds Kubernetes
// holds a toleration's time in.
const theLabsWaitInSeconds = int64(UnresponsiveNodeWait / time.Second)

// withTheLabsTolerations are these settings with a toleration of each of the
// lab's taints at the lab's waiting time: one written keeps its place and takes
// that time, and one not written is added after the settings written.
func withTheLabsTolerations(settings []Setting) []Setting {
	holding := slices.Clone(settings)
	for at, setting := range holding {
		if toleration, isToleration := setting.(Toleration); isToleration && slices.Contains(theLabsTaints, toleration.key) {
			toleration.tolerationSeconds = theLabsWaitInSeconds
			holding[at] = toleration
		}
	}
	for _, taint := range theLabsTaints {
		if !slices.ContainsFunc(holding, func(setting Setting) bool { return setting.field() == tolerationOf(taint) }) {
			holding = append(holding, NewToleration(taint, Exists, "", NoExecute, theLabsWaitInSeconds))
		}
	}

	return holding
}

// tolerationOf is the field a toleration of that taint occupies.
func tolerationOf(taint string) settingField {
	return settingField{kind: tolerationSetting, key: taint}
}

// eachFieldOnce are these settings with every field held once: the setting
// written last for a field, in the place that field was first written, so a
// definition reads in the order it was written whichever of its settings a
// customization replaced.
func eachFieldOnce(settings []Setting) []Setting {
	held := make([]Setting, 0, len(settings))
	for _, setting := range settings {
		written := slices.IndexFunc(held, func(already Setting) bool {
			return already.field() == setting.field()
		})
		if written < 0 {
			held = append(held, setting)

			continue
		}
		held[written] = setting
	}

	return held
}
