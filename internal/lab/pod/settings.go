package pod

import (
	"errors"
	"fmt"
	"slices"

	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// internal/scenario holds the one spelling of each value, and its mapping test
// pins it to the client library's constant.
const (
	DoNotSchedule = scenario.DoNotSchedule
	Honor         = scenario.Honor

	Exists     = scenario.Exists
	Equal      = scenario.Equal
	NoExecute  = scenario.NoExecute
	NoSchedule = scenario.NoSchedule

	// Gi is the unit a Request's Memory is written in: Memory: 12 * pod.Gi.
	Gi = scenario.Memory(mebibytesPerGibibyte)
)

const (
	millicoresPerCore    = 1000
	mebibytesPerGibibyte = 1024
)

// Setting is a Customization that is one setting the lab offers. Its method is
// unexported, so a Startup time, a full access or another group does not
// compile inside a group.
//
// Every field a setting takes is required: its zero value is the field left
// out, which the specification carries as its refusal. A toleration evicting at
// once, or a request of none, is written through full access.
type Setting interface {
	Customization
	settles()
}

// No value deciding a Placement falls to a default, so a field left out is
// refused.
var errAFieldLeftOut = errors.New("every field of a setting the lab offers is stated, so one left out is refused")

// A toleration's operator and effect decide which of its fields Kubernetes
// takes, so a field they rule out is refused.
var errAFieldStatedAgainst = errors.New("a field of a setting the lab offers is stated as the setting's other fields take it, " +
	"so one stated against them is refused")

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

type namedField struct {
	name          string
	leftOut       bool
	statedAgainst string
}

func applied(specification Specification, setting string, fields []namedField, declares func() scenario.Setting) Specification {
	if refused := firstRefusal(setting, fields); refused != nil {
		return specification.refusing(refused)
	}

	return specification.holding(declares())
}

// TopologySpreadConstraint requires every field, so no value deciding how Pods
// spread is the lab's own.
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

// Toleration takes the fields its operator and effect take, as Kubernetes'
// does: Equal takes a Value and Exists none, NoExecute takes TolerationSeconds
// and NoSchedule none. The not-ready and unreachable tolerations are held at
// the lab's waiting time whatever is written for them, because a Scenario
// states no waiting time of its own.
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

func unlessOneOf[V ~string](stated V, offered ...V) string {
	if stated == "" || slices.Contains(offered, stated) {
		return ""
	}

	return fmt.Sprintf(" %s, which is none of the values the lab offers, %v", stated, offered)
}

func unlessTaken[V ~string](stated, taken bool, deciding string, decidedBy V) string {
	if !stated || taken {
		return ""
	}

	return fmt.Sprintf(", which the %s %s takes none of", deciding, decidedBy)
}

func (Toleration) settles() {}

// Request keeps the lab's term because Kubernetes has no type for one Pod's
// requests.
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

// Settings groups settings under one name of a Scenario's own, applied in the
// order written. A group without a setting changes nothing.
func Settings(settings ...Setting) Customization {
	return settingGroup{composes: slices.Clone(settings)}
}

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
