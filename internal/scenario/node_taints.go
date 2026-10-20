package scenario

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// NoSchedule keeps every Pod not tolerating the taint off the Node and leaves
// the Pods already on it. It is the one effect a NodePool's taint may take.
const NoSchedule TaintEffect = "NoSchedule"

// A NodePool states no taint under this prefix, so no stated taint stands in
// for a Failure.
const controlPlanesTaintPrefix = "node.kubernetes.io/"

type Taint struct {
	Key    string
	Value  string
	Effect TaintEffect
}

type NodeTaints struct {
	taints []Taint
}

// NewNodeTaints gives the first refusal in key order, so the same taints are
// always refused alike.
func NewNodeTaints(stated ...Taint) (NodeTaints, error) {
	taints := slices.SortedStableFunc(slices.Values(stated), byKey)
	for at, taint := range taints {
		if err := refusalOfTaint(taint); err != nil {
			return NodeTaints{}, err
		}
		if at > 0 && taints[at-1].Key == taint.Key {
			return NodeTaints{}, taintStatedAgain(taint.Key)
		}
	}

	return NodeTaints{taints: taints}, nil
}

func byKey(a, b Taint) int {
	return cmp.Compare(a.Key, b.Key)
}

func refusalOfTaint(taint Taint) error {
	for _, field := range []struct {
		name    string
		leftOut bool
	}{
		{name: "key", leftOut: taint.Key == ""},
		{name: "value", leftOut: taint.Value == ""},
		{name: "effect", leftOut: taint.Effect == ""},
	} {
		if field.leftOut {
			return fmt.Errorf("a NodePool's taint %s leaves out its %s, and every field of a taint is stated, "+
				"so it is refused", taint, field.name)
		}
	}
	if taint.Effect != NoSchedule {
		return fmt.Errorf("a NodePool's taint takes the effect %s alone, so the taint %s is refused", NoSchedule, taint)
	}
	if IsControlPlanesTaint(taint.Key) {
		return fmt.Errorf("the control plane sets the taints under %q on a Node from its conditions, "+
			"so a NodePool stating the key %q is refused", controlPlanesTaintPrefix, taint.Key)
	}
	if !isLabelKey(taint.Key) {
		return fmt.Errorf("a taint key is an optional DNS subdomain of at most %d characters and '/', "+
			"then a name of at most %d letters, digits, '-', '_' or '.', opening and closing on a letter or digit, "+
			"so the key %q is refused", maxLabelKeyPrefixLength, maxLabelValueLength, taint.Key)
	}
	if !isLabelValue(taint.Value) {
		return fmt.Errorf("a taint value is at most %d letters, digits, '-', '_' or '.', "+
			"opening and closing on a letter or digit, so the value %q of the key %q is refused",
			maxLabelValueLength, taint.Value, taint.Key)
	}

	return nil
}

func IsControlPlanesTaint(key string) bool {
	return strings.HasPrefix(key, controlPlanesTaintPrefix)
}

func taintStatedAgain(key string) error {
	return fmt.Errorf("a NodePool states each taint key once, so the key %q stated again is refused", key)
}

// String writes the taint as kubectl does: key=value:Effect.
func (t Taint) String() string {
	return t.Key + "=" + t.Value + ":" + string(t.Effect)
}

func (t NodeTaints) Adding(added NodeTaints) (NodeTaints, error) {
	for _, taint := range added.taints {
		if slices.ContainsFunc(t.taints, func(held Taint) bool { return held.Key == taint.Key }) {
			return NodeTaints{}, taintStatedAgain(taint.Key)
		}
	}

	return NodeTaints{taints: slices.SortedStableFunc(slices.Values(slices.Concat(t.taints, added.taints)), byKey)}, nil
}

func (t NodeTaints) Taints() []Taint {
	return slices.Clone(t.taints)
}

// Equal is the equality go-cmp compares through, since the type's field is
// unexported.
func (t NodeTaints) Equal(other NodeTaints) bool {
	return slices.Equal(t.taints, other.taints)
}
