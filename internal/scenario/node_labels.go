package scenario

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// These labels are spelled here so the domain refuses a NodePool stating one,
// and internal/cluster writes these spellings.
const (
	NodePoolLabel           = labsLabelPrefix + "node-pool"
	SimulatorSelectionLabel = "type"
)

// labsLabelPrefix covers every key the lab owns, whether it writes it on a Node
// today or not.
const labsLabelPrefix = "k8s-resilience-lab/"

// LabsOwnNodeLabelKeys are keys a NodePool states none of.
func LabsOwnNodeLabelKeys() []string {
	return []string{ZoneLabel, HostnameLabel, NodePoolLabel, SimulatorSelectionLabel}
}

func IsLabsOwnNodeLabel(key string) bool {
	return slices.Contains(LabsOwnNodeLabelKeys(), key) || strings.HasPrefix(key, labsLabelPrefix)
}

// The domain names no Kubernetes package, so the label-key syntax is held here
// and pinned to the client library's validation by a test in internal/cluster.
var labelKeyPrefixSyntax = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)

const maxLabelKeyPrefixLength = 253

type NodeLabel struct {
	Key   string
	Value string
}

type NodeLabels struct {
	pairs []NodeLabel
}

// NewNodeLabels admits an empty value, as Kubernetes does, and gives the first
// refusal in key order, so the same labels are always refused alike.
func NewNodeLabels(stated map[string]string) (NodeLabels, error) {
	pairs := make([]NodeLabel, 0, len(stated))
	for _, key := range slices.Sorted(maps.Keys(stated)) {
		value := stated[key]
		if err := refusalOfLabel(key, value); err != nil {
			return NodeLabels{}, err
		}
		pairs = append(pairs, NodeLabel{Key: key, Value: value})
	}

	return NodeLabels{pairs: pairs}, nil
}

func refusalOfLabel(key, value string) error {
	if IsLabsOwnNodeLabel(key) {
		return fmt.Errorf("the lab writes the label %q on every Node itself, and owns every key under %q, "+
			"so a NodePool stating it is refused", key, labsLabelPrefix)
	}
	if !isLabelKey(key) {
		return fmt.Errorf("a Node label key is an optional DNS subdomain of at most %d characters and '/', "+
			"then a name of at most %d letters, digits, '-', '_' or '.', opening and closing on a letter or digit, "+
			"so the key %q is refused", maxLabelKeyPrefixLength, maxLabelValueLength, key)
	}
	if !isLabelValue(value) {
		return fmt.Errorf("a Node label value is empty or at most %d letters, digits, '-', '_' or '.', "+
			"opening and closing on a letter or digit, so the value %q of the key %q is refused",
			maxLabelValueLength, value, key)
	}

	return nil
}

func isLabelKey(key string) bool {
	prefix, name, prefixed := strings.Cut(key, "/")
	if !prefixed {
		return isLabelName(key)
	}

	return len(prefix) <= maxLabelKeyPrefixLength && labelKeyPrefixSyntax.MatchString(prefix) && isLabelName(name)
}

func isLabelName(name string) bool {
	return name != "" && isLabelValue(name)
}

func isLabelValue(value string) bool {
	return value == "" || len(value) <= maxLabelValueLength && labelValueSyntax.MatchString(value)
}

func (l NodeLabels) Adding(added NodeLabels) (NodeLabels, error) {
	for _, label := range added.pairs {
		if _, held := l.Value(label.Key); held {
			return NodeLabels{}, fmt.Errorf("a NodePool states each Node label once, so the key %q stated again is refused",
				label.Key)
		}
	}

	pairs := slices.Concat(l.pairs, added.pairs)
	slices.SortFunc(pairs, func(a, b NodeLabel) int { return strings.Compare(a.Key, b.Key) })

	return NodeLabels{pairs: pairs}, nil
}

func (l NodeLabels) Value(key string) (string, bool) {
	for _, label := range l.pairs {
		if label.Key == key {
			return label.Value, true
		}
	}

	return "", false
}

func (l NodeLabels) Pairs() []NodeLabel {
	return slices.Clone(l.pairs)
}

// Equal is the equality go-cmp compares through, since the type's field is
// unexported.
func (l NodeLabels) Equal(other NodeLabels) bool {
	return slices.Equal(l.pairs, other.pairs)
}
