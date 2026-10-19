package scenario

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// The Node labels the lab writes on every Node beside the Zone and hostname
// labels: the name of a Node's NodePool, and the label the simulator selects
// the Nodes it maintains by, which a Failure takes off its Node. They are spelled
// here so the domain refuses a NodePool stating one, and internal/cluster names
// these spellings when it writes them, per
// docs/adr/2026-10-18-a-nodepool-states-its-own-node-labels-beside-the-labs.md.
const (
	NodePoolLabel           = labsLabelPrefix + "node-pool"
	SimulatorSelectionLabel = "type"
)

// labsLabelPrefix is the prefix of every label key the lab owns, whether it
// writes it on a Node today or not.
const labsLabelPrefix = "k8s-resilience-lab/"

// LabsOwnNodeLabelKeys are the label keys the lab writes on every Node, in the
// order a reader of a Node finds them: its Zone, its hostname, its NodePool and
// the simulator's selection label. A NodePool states none of them.
func LabsOwnNodeLabelKeys() []string {
	return []string{ZoneLabel, HostnameLabel, NodePoolLabel, SimulatorSelectionLabel}
}

// IsLabsOwnNodeLabel reports whether key is one the lab owns: one it writes on
// every Node, or any under its prefix.
func IsLabsOwnNodeLabel(key string) bool {
	return slices.Contains(LabsOwnNodeLabelKeys(), key) || strings.HasPrefix(key, labsLabelPrefix)
}

// Kubernetes' label-key syntax: an optional prefix that is a DNS subdomain, then
// a name of the label-value syntax that is not empty. The domain names no
// Kubernetes package, so the rule is held here and pinned to the client
// library's own validation by a test in internal/cluster.
var labelKeyPrefixSyntax = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)

// maxLabelKeyPrefixLength is the longest DNS subdomain a label key's prefix is.
const maxLabelKeyPrefixLength = 253

// NodeLabel is one label a NodePool states its Nodes carry: a key and its value.
type NodeLabel struct {
	Key   string
	Value string
}

// NodeLabels are the labels a NodePool states its Nodes carry beside the lab's
// own, held in key order. The zero value holds none.
type NodeLabels struct {
	pairs []NodeLabel
}

// NewNodeLabels builds the labels stated, refusing a key the lab owns, a key
// Kubernetes cannot hold as a label key, and a value it cannot hold as a label
// value; an empty value is one it holds. The first refusal in key order is the
// one given, so the same labels are always refused alike.
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

// refusalOfLabel is why one stated label is refused, or nil where it is not.
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

// isLabelKey reports whether key meets Kubernetes' label-key syntax.
func isLabelKey(key string) bool {
	prefix, name, prefixed := strings.Cut(key, "/")
	if !prefixed {
		return isLabelName(key)
	}

	return len(prefix) <= maxLabelKeyPrefixLength && labelKeyPrefixSyntax.MatchString(prefix) && isLabelName(name)
}

// isLabelName reports whether name is a label key's name: a label value that is
// not empty.
func isLabelName(name string) bool {
	return name != "" && isLabelValue(name)
}

// isLabelValue reports whether value meets Kubernetes' label-value syntax, which
// admits the empty value.
func isLabelValue(value string) bool {
	return value == "" || len(value) <= maxLabelValueLength && labelValueSyntax.MatchString(value)
}

// Adding is these labels and the ones added, refusing a key both state.
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

// Value is the value these labels hold under key, and whether they hold it.
func (l NodeLabels) Value(key string) (string, bool) {
	for _, label := range l.pairs {
		if label.Key == key {
			return label.Value, true
		}
	}

	return "", false
}

// Pairs are every label held, in key order.
func (l NodeLabels) Pairs() []NodeLabel {
	return slices.Clone(l.pairs)
}

// Equal reports whether two sets of labels hold the same keys under the same
// values. It is the equality go-cmp compares through, since the type's field is
// unexported.
func (l NodeLabels) Equal(other NodeLabels) bool {
	return slices.Equal(l.pairs, other.pairs)
}
