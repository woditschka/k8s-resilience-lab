package scenario

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// This package names no Kubernetes type, so these are the lab's own spellings,
// and the mapping test pins each to the client library's constant.
const (
	ZoneLabel     = "topology.kubernetes.io/zone"
	HostnameLabel = "kubernetes.io/hostname"

	TaintNodeNotReady    = "node.kubernetes.io/not-ready"
	TaintNodeUnreachable = "node.kubernetes.io/unreachable"
)

type UnsatisfiableConstraintAction string

// DoNotSchedule leaves such a Pod Pending, so a spread the Topology cannot
// satisfy is visible rather than silently given up.
const DoNotSchedule UnsatisfiableConstraintAction = "DoNotSchedule"

type NodeInclusionPolicy string

// Honor counts only the Nodes whose taints these Pods tolerate, so a Node that
// stopped responding leaves the count its Pods were spread over.
const Honor NodeInclusionPolicy = "Honor"

// TolerationOperator is how a toleration matches a taint, under Kubernetes' own
// names: Equal takes a value and Exists none.
type TolerationOperator string

const (
	Exists TolerationOperator = "Exists"
	Equal  TolerationOperator = "Equal"
)

func (o TolerationOperator) TakesAValue() bool {
	return o == Equal
}

// TaintEffect is what a taint does to a Pod not tolerating it, under
// Kubernetes' own names.
type TaintEffect string

const NoExecute TaintEffect = "NoExecute"

// TakesAWaitingTime holds for NoExecute alone, as Kubernetes takes a waiting
// time for no other effect, since NoSchedule moves no Pod.
func (e TaintEffect) TakesAWaitingTime() bool {
	return e == NoExecute
}

// The lab's UnresponsiveNodeWait is short so a Scenario stays fast: above the
// poll interval and far below the default, which is longer than the whole
// settle wait. The control plane's noticing period adds to it, so a run prints
// the settings it makes and claims no total.
const (
	UnresponsiveNodeWait        = 2 * time.Second
	DefaultUnresponsiveNodeWait = 300 * time.Second
)

// StartupTime is no Setting, because no Kubernetes field holds it; the
// simulator reads it off the Pod. Equal bounds are one time every Pod takes.
type StartupTime struct {
	shortest time.Duration
	longest  time.Duration
}

// NewStartupTime refuses a negative time, because no Pod is Ready before it is
// placed, and a shortest above its longest. A time of none is admitted. A
// refusal names the bounds as durations, since a Scenario reaches it from
// seconds and milliseconds alike.
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

func (s StartupTime) Shortest() time.Duration {
	return s.shortest
}

func (s StartupTime) Longest() time.Duration {
	return s.longest
}

// Equal is the equality go-cmp compares through, since the type's fields are
// unexported.
func (s StartupTime) Equal(other StartupTime) bool {
	return s == other
}

// The kind is what makes one setting of a field replaceable by another of the
// same field.
const (
	spreadSetting     = "topologySpread"
	tolerationSetting = "toleration"
	requestSetting    = "request"
)

// Setting has one unexported method, so the three settings below are every
// setting there is.
type Setting interface {
	// A setting written for a field a specification already holds replaces it.
	field() settingField
}

// The key is the one Kubernetes holds one setting of that kind per: a topology
// key for a spread, a taint key for a toleration.
type settingField struct {
	kind string
	key  string
}

type TopologySpread struct {
	topologyKey       string
	maxSkew           int
	whenUnsatisfiable UnsatisfiableConstraintAction
	nodeTaintsPolicy  NodeInclusionPolicy
}

// NewTopologySpread refuses nothing while the lab's own definitions are its
// only callers; a Scenario supplying a value of its own brings a validated
// constructor with it.
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

func (s TopologySpread) TopologyKey() string {
	return s.topologyKey
}

func (s TopologySpread) MaxSkew() int {
	return s.maxSkew
}

func (s TopologySpread) WhenUnsatisfiable() UnsatisfiableConstraintAction {
	return s.whenUnsatisfiable
}

func (s TopologySpread) NodeTaintsPolicy() NodeInclusionPolicy {
	return s.nodeTaintsPolicy
}

// Equal is the equality go-cmp compares through, since the type's fields are
// unexported.
func (s TopologySpread) Equal(other TopologySpread) bool {
	return s == other
}

func (s TopologySpread) field() settingField {
	return settingField{kind: spreadSetting, key: s.topologyKey}
}

// Toleration holds the waiting time in the whole seconds Kubernetes holds that
// field as, so the setting is that field and not a reading of it.
type Toleration struct {
	key               string
	operator          TolerationOperator
	value             string
	effect            TaintEffect
	tolerationSeconds int64
}

// NewToleration refuses nothing, for the reason NewTopologySpread does not.
func NewToleration(
	key string,
	operator TolerationOperator,
	value string,
	effect TaintEffect,
	tolerationSeconds int64,
) Toleration {
	return Toleration{key: key, operator: operator, value: value, effect: effect, tolerationSeconds: tolerationSeconds}
}

func (t Toleration) Key() string {
	return t.key
}

func (t Toleration) Operator() TolerationOperator {
	return t.operator
}

// Value is empty for an Exists toleration.
func (t Toleration) Value() string {
	return t.value
}

func (t Toleration) Effect() TaintEffect {
	return t.effect
}

func (t Toleration) TolerationSeconds() int64 {
	return t.tolerationSeconds
}

// Equal is the equality go-cmp compares through, since the type's fields are
// unexported.
func (t Toleration) Equal(other Toleration) bool {
	return t == other
}

func (t Toleration) field() settingField {
	return settingField{kind: tolerationSetting, key: t.key}
}

// Request states no limit: nothing runs in a Simulated Cluster for a limit to
// bind.
type Request struct {
	cpu    CPU
	memory Memory
}

// NewRequest refuses nothing: what a Kubernetes quantity admits is the control
// plane's to judge.
func NewRequest(cpu CPU, memory Memory) Request {
	return Request{cpu: cpu, memory: memory}
}

func (r Request) CPU() CPU {
	return r.cpu
}

func (r Request) Memory() Memory {
	return r.memory
}

// Equal is the equality go-cmp compares through, since the type's fields are
// unexported.
func (r Request) Equal(other Request) bool {
	return r == other
}

// A specification states what one Pod asks for once, so a request carries no
// key.
func (Request) field() settingField {
	return settingField{kind: requestSetting}
}

// PodSpecification holds one setting per field, in the order its definition
// wrote them, which is the order the mapper transcribes them in.
type PodSpecification struct {
	name        string
	settings    []Setting
	startupTime StartupTime
}

// NewPodSpecification refuses an empty name, keeps each field at the value
// written last, and holds the lab's two tolerations at the lab's waiting time
// whatever is written, because a Scenario states no waiting time of its own.
func NewPodSpecification(name string, startup StartupTime, settings ...Setting) (PodSpecification, error) {
	if name == "" {
		return PodSpecification{}, errors.New("a Pod specification is declared by its name, so one under no name is refused")
	}

	return PodSpecification{name: name, settings: held(settings), startupTime: startup}, nil
}

// With returns a copy. The name and Startup time were admitted when this one
// was built, so only the rule over its settings is entered again.
func (s PodSpecification) With(settings ...Setting) PodSpecification {
	customized := s
	customized.settings = held(slices.Concat(s.settings, settings))

	return customized
}

// WithStartupTime returns a copy.
func (s PodSpecification) WithStartupTime(stated StartupTime) PodSpecification {
	customized := s.With()
	customized.startupTime = stated

	return customized
}

func (s PodSpecification) StartupTime() StartupTime {
	return s.startupTime
}

// Name is also what the declaring Step reports.
func (s PodSpecification) Name() string {
	return s.name
}

func (s PodSpecification) Settings() []Setting {
	return slices.Clone(s.settings)
}

func (s PodSpecification) TopologySpreads() []TopologySpread {
	var spreads []TopologySpread
	for _, setting := range s.settings {
		if spread, isSpread := setting.(TopologySpread); isSpread {
			spreads = append(spreads, spread)
		}
	}

	return spreads
}

func (s PodSpecification) Tolerations() []Toleration {
	var tolerated []Toleration
	for _, setting := range s.settings {
		if toleration, isToleration := setting.(Toleration); isToleration {
			tolerated = append(tolerated, toleration)
		}
	}

	return tolerated
}

// Equal is the equality go-cmp compares through, since the type's fields are
// unexported.
func (s PodSpecification) Equal(other PodSpecification) bool {
	return s.name == other.name && slices.Equal(s.settings, other.settings) && s.startupTime == other.startupTime
}

func held(settings []Setting) []Setting {
	return withTheLabsTolerations(eachFieldOnce(settings))
}

var theLabsTaints = []string{TaintNodeNotReady, TaintNodeUnreachable}

const theLabsWaitInSeconds = int64(UnresponsiveNodeWait / time.Second)

// A lab toleration written by the definition keeps its place and takes the
// lab's time; one not written is added after the settings written.
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

func tolerationOf(taint string) settingField {
	return settingField{kind: tolerationSetting, key: taint}
}

// The setting written last for a field takes the place that field was first
// written, so a definition reads in the order it was written whichever setting
// a customization replaced.
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
