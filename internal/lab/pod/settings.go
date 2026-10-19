package pod

import (
	"errors"
	"fmt"
	"slices"

	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// The values a setting takes, under Kubernetes' own names. internal/scenario
// holds the one spelling of each, and its mapping test pins it to the client
// library's constant.
const (
	// DoNotSchedule leaves a Pod no Node can take without widening a spread
	// Pending, and Honor counts only the Nodes whose taints a Workload's Pods
	// tolerate.
	DoNotSchedule = scenario.DoNotSchedule
	Honor         = scenario.Honor

	// Exists matches a taint of that key whatever its value, and Equal a taint of
	// that key and the value named. NoExecute is the effect a Pod is moved off a
	// Node by, and NoSchedule the one that keeps a Pod off a Node.
	Exists     = scenario.Exists
	Equal      = scenario.Equal
	NoExecute  = scenario.NoExecute
	NoSchedule = scenario.NoSchedule

	// Gi is one gibibyte, the unit a Request's Memory is written in, as a
	// manifest writes 12Gi: Memory: 12 * pod.Gi.
	Gi = scenario.Memory(mebibytesPerGibibyte)
)

// The lab's own quantities hold a cpu in millicores and a memory in mebibytes.
const (
	millicoresPerCore    = 1000
	mebibytesPerGibibyte = 1024
)

// Setting is a Customization that is one setting the lab offers: a
// TopologySpreadConstraint, a Toleration or a Request, and the only thing
// Settings groups. Its method is unexported, so a Startup time, a full access or
// another group is no Setting and does not compile inside a group.
//
// Each is a keyed literal under Kubernetes' names, and every field it takes is
// required: its zero value is the field left out, which the specification it is
// applied to carries as its refusal, naming the setting and the field, per
// docs/adr/2026-10-18-pod-and-node-settings-are-keyed-literals-refused-at-their-given.md.
// A toleration takes the fields its operator and effect take, and one stated
// against them is refused alike.
// A toleration evicting at once, or a request of none, is written through full
// access.
type Setting interface {
	Customization
	// settles marks the Customizations that are one setting.
	settles()
}

// errAFieldLeftOut is why a setting with a field left out is refused: the domain
// holds every field as a plain value, and no value deciding a Placement falls to
// a default, per
// docs/adr/2026-10-18-pod-and-node-settings-are-keyed-literals-refused-at-their-given.md.
var errAFieldLeftOut = errors.New("every field of a setting the lab offers is stated, so one left out is refused")

// errAFieldStatedAgainst is why a setting stating a field the others rule out is
// refused: a toleration's operator and effect decide which of its fields
// Kubernetes takes, and the lab offers the values whose fields it knows, per
// docs/adr/2026-10-19-a-nodepool-states-noschedule-taints-and-a-later-workload-deploys-beside-the-earlier.md.
var errAFieldStatedAgainst = errors.New("a field of a setting the lab offers is stated as the setting's other fields take it, " +
	"so one stated against them is refused")

// firstRefusal is the refusal of the setting named for the first field of fields
// left out or stated against the others, in the order a setting writes them, or
// nil where there is none.
func firstRefusal(setting string, fields []namedField) error {
	for _, field := range fields {
		switch {
		case field.leftOut:
			return fmt.Errorf("pod.%s leaves out %s: %w", setting, field.name, errAFieldLeftOut)
		case field.statedAgainst != "":
			return fmt.Errorf("pod.%s states %s%s: %w", setting, field.name, field.statedAgainst, errAFieldStatedAgainst)
		}
	}

	return nil
}

// namedField is one field of a setting: its Kubernetes name, whether it holds the
// zero value of a field the setting takes, and why it is refused where it is
// stated against the setting's other fields, read on after the field's name.
type namedField struct {
	name          string
	leftOut       bool
	statedAgainst string
}

// applied is the specification holding the setting that declares builds, or
// carrying the refusal of the first field left out or stated against the others.
func applied(specification Specification, setting string, fields []namedField, declares func() scenario.Setting) Specification {
	if refused := firstRefusal(setting, fields); refused != nil {
		return specification.refusing(refused)
	}

	return specification.holding(declares())
}

// TopologySpreadConstraint is the setting that decides where a Workload's Pods
// land: the label whose values are the domains the Pods spread over, the most
// they may differ by between two domains, what happens to a Pod no Node can take
// within it, and which Nodes' taints count.
type TopologySpreadConstraint struct {
	TopologyKey       string
	MaxSkew           int
	WhenUnsatisfiable scenario.UnsatisfiableConstraintAction
	NodeTaintsPolicy  scenario.NodeInclusionPolicy
}

func (c TopologySpreadConstraint) customize(specification Specification) Specification {
	return applied(specification, "TopologySpreadConstraint", []namedField{
		{name: "topologyKey", leftOut: c.TopologyKey == ""},
		{name: "maxSkew", leftOut: c.MaxSkew == 0},
		{name: "whenUnsatisfiable", leftOut: c.WhenUnsatisfiable == ""},
		{name: "nodeTaintsPolicy", leftOut: c.NodeTaintsPolicy == ""},
	}, func() scenario.Setting {
		return scenario.NewTopologySpread(c.TopologyKey, c.MaxSkew, c.WhenUnsatisfiable, c.NodeTaintsPolicy)
	})
}

func (TopologySpreadConstraint) settles() {}

// Toleration is the setting that decides which tainted Nodes a Workload's Pods
// may be placed on, and when they leave a Node: the taint they put up with, and
// for how long. The domain holds the lab's two tolerations, for the not-ready and
// the unreachable taint, at the lab's waiting time whatever is written for them,
// because a Scenario states no waiting time of its own (docs/prd.md REQ-SIM-008).
//
// The fields it takes follow its operator and effect, as Kubernetes' do: Equal
// takes a Value and Exists none, NoExecute takes TolerationSeconds and NoSchedule
// none, per
// docs/adr/2026-10-19-a-nodepool-states-noschedule-taints-and-a-later-workload-deploys-beside-the-earlier.md.
type Toleration struct {
	Key               string
	Operator          scenario.TolerationOperator
	Value             string
	Effect            scenario.TaintEffect
	TolerationSeconds int64
}

func (t Toleration) customize(specification Specification) Specification {
	return applied(specification, "Toleration", []namedField{
		{name: "key", leftOut: t.Key == ""},
		{name: "operator", leftOut: t.Operator == "", statedAgainst: unlessOneOf(t.Operator, Equal, Exists)},
		{
			name: "value", leftOut: t.Operator.TakesAValue() && t.Value == "",
			statedAgainst: unlessTaken(t.Value != "", t.Operator.TakesAValue(), "operator", t.Operator),
		},
		{name: "effect", leftOut: t.Effect == "", statedAgainst: unlessOneOf(t.Effect, NoSchedule, NoExecute)},
		{
			name: "tolerationSeconds", leftOut: t.Effect.TakesAWaitingTime() && t.TolerationSeconds == 0,
			statedAgainst: unlessTaken(t.TolerationSeconds != 0, t.Effect.TakesAWaitingTime(), "effect", t.Effect),
		},
	}, func() scenario.Setting {
		return scenario.NewToleration(t.Key, t.Operator, t.Value, t.Effect, t.TolerationSeconds)
	})
}

// unlessOneOf is why a stated value outside those the lab offers is refused, or
// nothing where it is one of them or left out.
func unlessOneOf[V ~string](stated V, offered ...V) string {
	if stated == "" || slices.Contains(offered, stated) {
		return ""
	}

	return fmt.Sprintf(" %s, which is none of the values the lab offers, %v", stated, offered)
}

// unlessTaken is why a field stated where the field deciding it takes none is
// refused, or nothing where it is taken or not stated.
func unlessTaken[V ~string](stated, taken bool, deciding string, decidedBy V) string {
	if !stated || taken {
		return ""
	}

	return fmt.Sprintf(", which the %s %s takes none of", deciding, decidedBy)
}

func (Toleration) settles() {}

// Request is the setting that decides what one Pod of a Workload asks for: the
// CPU in whole cores and the Memory in pod.Gi the scheduler fits against the room
// a Node reports. Kubernetes has no type for one Pod's requests, so the setting
// keeps the lab's term and its fields the resource names.
type Request struct {
	CPU    int
	Memory scenario.Memory
}

func (r Request) customize(specification Specification) Specification {
	return applied(specification, "Request", []namedField{
		{name: "cpu", leftOut: r.CPU == 0},
		{name: "memory", leftOut: r.Memory == 0},
	}, func() scenario.Setting {
		return scenario.NewRequest(scenario.CPU(r.CPU*millicoresPerCore), r.Memory)
	})
}

func (Request) settles() {}

// Settings groups settings under one name of a Scenario's own: the Customization
// applying each in the order written, so the later setting for a field stands as
// it does written one by one. A group without a setting changes nothing. The lab
// defines no group: a Named setting is a function of the Scenarios' place that
// calls it, per
// docs/adr/2026-10-18-the-scenarios-name-leaving-an-unresponsive-node-as-one-setting.md.
func Settings(settings ...Setting) Customization {
	return settingGroup{composes: slices.Clone(settings)}
}

// settingGroup is the Customization a group of settings is: the settings it
// stands for, in the order they apply. It holds settings alone, so a group
// carries no Startup time and no full access.
type settingGroup struct {
	composes []Setting
}

func (g settingGroup) customize(specification Specification) Specification {
	customized := specification
	for _, each := range g.composes {
		customized = each.customize(customized)
	}

	return customized
}
