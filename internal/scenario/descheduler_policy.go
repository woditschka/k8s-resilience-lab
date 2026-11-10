package scenario

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"
)

// DeschedulerPluginName is a plugin's name under the Descheduler's own
// spelling, which this package holds because it imports no Descheduler module.
type DeschedulerPluginName string

// The plugins a Scenario's policy can name, spelled as the Descheduler's
// plugin registry spells them.
const (
	DefaultEvictor                              DeschedulerPluginName = "DefaultEvictor"
	RemoveDuplicates                            DeschedulerPluginName = "RemoveDuplicates"
	RemovePodsViolatingInterPodAntiAffinity     DeschedulerPluginName = "RemovePodsViolatingInterPodAntiAffinity"
	RemovePodsViolatingTopologySpreadConstraint DeschedulerPluginName = "RemovePodsViolatingTopologySpreadConstraint"
)

// DeschedulerExtensionPoint is spelled as the policy file's key for it.
type DeschedulerExtensionPoint string

const (
	DescheduleExtensionPoint        DeschedulerExtensionPoint = "deschedule"
	BalanceExtensionPoint           DeschedulerExtensionPoint = "balance"
	FilterExtensionPoint            DeschedulerExtensionPoint = "filter"
	PreEvictionFilterExtensionPoint DeschedulerExtensionPoint = "preEvictionFilter"
)

// DeschedulerPolicy holds only what its definition states, so the lab adds no
// profile, plugin, argument or default the Descheduler would then apply. A
// limit of zero is one the definition leaves out.
type DeschedulerPolicy struct {
	maxPodsToEvictPerNode uint
	maxPodsToEvictTotal   uint
	profiles              []DeschedulerProfile
}

var errNoDeschedulerProfile = errors.New("a Descheduler policy holds the profiles it runs, " +
	"so a policy of no profile is refused")

// NewDeschedulerPolicy refuses two profiles of one name, since the policy is
// named by its profiles' names.
func NewDeschedulerPolicy(maxPodsToEvictPerNode uint, profiles ...DeschedulerProfile) (DeschedulerPolicy, error) {
	if len(profiles) == 0 {
		return DeschedulerPolicy{}, errNoDeschedulerProfile
	}
	named := make(map[string]bool, len(profiles))
	for _, profile := range profiles {
		if profile.name == "" {
			return DeschedulerPolicy{}, errNoDeschedulerProfileName
		}
		if named[profile.name] {
			return DeschedulerPolicy{}, fmt.Errorf("a Descheduler policy names each profile once, "+
				"so the profile %s named twice is refused", profile.name)
		}
		named[profile.name] = true
	}

	return DeschedulerPolicy{maxPodsToEvictPerNode: maxPodsToEvictPerNode, profiles: slices.Clone(profiles)}, nil
}

// DeschedulerPolicyName is the one rule naming a policy, since a policy file
// holds no name of its own: a definition the policy refuses is named by it too,
// so every line and refusal names one policy alike.
func DeschedulerPolicyName(profileNames ...string) string {
	return strings.Join(profileNames, ", ")
}

// Name is the DeschedulerPolicyName of the profile names in order.
func (p DeschedulerPolicy) Name() string {
	names := make([]string, 0, len(p.profiles))
	for _, profile := range p.profiles {
		names = append(names, profile.name)
	}

	return DeschedulerPolicyName(names...)
}

// MaxPodsToEvictPerNode is the policy file's maxNoOfPodsToEvictPerNode, zero
// where the definition leaves it out.
func (p DeschedulerPolicy) MaxPodsToEvictPerNode() uint {
	return p.maxPodsToEvictPerNode
}

// WithMaxPodsToEvictTotal states the policy file's maxNoOfPodsToEvictTotal,
// the limit on what one pass evicts across every Node.
func (p DeschedulerPolicy) WithMaxPodsToEvictTotal(total uint) DeschedulerPolicy {
	p.maxPodsToEvictTotal = total

	return p
}

// MaxPodsToEvictTotal is the policy file's maxNoOfPodsToEvictTotal, zero where
// the definition leaves it out.
func (p DeschedulerPolicy) MaxPodsToEvictTotal() uint {
	return p.maxPodsToEvictTotal
}

// Profiles are in definition order, as copies.
func (p DeschedulerPolicy) Profiles() []DeschedulerProfile {
	return slices.Clone(p.profiles)
}

// Equal is the equality go-cmp compares through, since the type's fields are
// unexported.
func (p DeschedulerPolicy) Equal(other DeschedulerPolicy) bool {
	return p.maxPodsToEvictPerNode == other.maxPodsToEvictPerNode &&
		p.maxPodsToEvictTotal == other.maxPodsToEvictTotal &&
		slices.EqualFunc(p.profiles, other.profiles, DeschedulerProfile.Equal)
}

// DeschedulerProfile holds its configuration entries and its enabled plugins
// apart, as the policy file does, so a plugin configured but enabled nowhere
// stays as written.
type DeschedulerProfile struct {
	name         string
	pluginConfig []DeschedulerPluginConfig
	enabled      []DeschedulerPluginSet
}

var errNoDeschedulerProfileName = errors.New("a Descheduler profile is named, " +
	"since a run reports the policy by its profiles' names, so a profile of no name is refused")

// NewDeschedulerProfile refuses a profile enabling no plugin at any extension
// point, which would evict nothing under the name of a profile that does.
func NewDeschedulerProfile(name string, pluginConfig []DeschedulerPluginConfig, enabled []DeschedulerPluginSet) (DeschedulerProfile, error) {
	if name == "" {
		return DeschedulerProfile{}, errNoDeschedulerProfileName
	}
	if !slices.ContainsFunc(enabled, func(set DeschedulerPluginSet) bool { return len(set.plugins) > 0 }) {
		return DeschedulerProfile{}, fmt.Errorf("a Descheduler profile enables the plugins it runs, "+
			"so the profile %s enabling none is refused", name)
	}

	return DeschedulerProfile{name: name, pluginConfig: slices.Clone(pluginConfig), enabled: slices.Clone(enabled)}, nil
}

// Name is unique within its policy, and names the policy alongside the others.
func (p DeschedulerProfile) Name() string {
	return p.name
}

// PluginConfig is the profile's pluginConfig entries, as copies.
func (p DeschedulerProfile) PluginConfig() []DeschedulerPluginConfig {
	return slices.Clone(p.pluginConfig)
}

// Enabled holds only the extension points the definition enables a plugin at,
// as copies.
func (p DeschedulerProfile) Enabled() []DeschedulerPluginSet {
	return slices.Clone(p.enabled)
}

// Equal is the equality go-cmp compares through, since the type's fields are
// unexported.
func (p DeschedulerProfile) Equal(other DeschedulerProfile) bool {
	return p.name == other.name &&
		slices.EqualFunc(p.pluginConfig, other.pluginConfig, DeschedulerPluginConfig.Equal) &&
		slices.EqualFunc(p.enabled, other.enabled, DeschedulerPluginSet.Equal)
}

// DeschedulerPluginConfig carries the arguments its definition states alone, in
// the order it states them.
type DeschedulerPluginConfig struct {
	name      DeschedulerPluginName
	arguments []DeschedulerArgument
}

// NewDeschedulerPluginConfig keeps the arguments in the order given, the order
// the policy file writes them in.
func NewDeschedulerPluginConfig(name DeschedulerPluginName, arguments ...DeschedulerArgument) DeschedulerPluginConfig {
	return DeschedulerPluginConfig{name: name, arguments: slices.Clone(arguments)}
}

// Name is the plugin the entry configures.
func (c DeschedulerPluginConfig) Name() DeschedulerPluginName {
	return c.name
}

// Arguments are in definition order, as copies.
func (c DeschedulerPluginConfig) Arguments() []DeschedulerArgument {
	return slices.Clone(c.arguments)
}

// Equal is the equality go-cmp compares through, since the type's fields are
// unexported.
func (c DeschedulerPluginConfig) Equal(other DeschedulerPluginConfig) bool {
	return c.name == other.name && slices.EqualFunc(c.arguments, other.arguments, DeschedulerArgument.Equal)
}

// DeschedulerPluginSet is the plugins one extension point enables, in the
// order written.
type DeschedulerPluginSet struct {
	point   DeschedulerExtensionPoint
	plugins []DeschedulerPluginName
}

// NewDeschedulerPluginSet keeps the plugins in the order given, the order the
// Descheduler runs them in.
func NewDeschedulerPluginSet(point DeschedulerExtensionPoint, plugins ...DeschedulerPluginName) DeschedulerPluginSet {
	return DeschedulerPluginSet{point: point, plugins: slices.Clone(plugins)}
}

// Point is the extension point the plugins are enabled at.
func (s DeschedulerPluginSet) Point() DeschedulerExtensionPoint {
	return s.point
}

// Plugins are in the order written, as a copy.
func (s DeschedulerPluginSet) Plugins() []DeschedulerPluginName {
	return slices.Clone(s.plugins)
}

// Equal is the equality go-cmp compares through, since the type's fields are
// unexported.
func (s DeschedulerPluginSet) Equal(other DeschedulerPluginSet) bool {
	return s.point == other.point && slices.Equal(s.plugins, other.plugins)
}

// DeschedulerArgumentValue holds the kinds of value the plugins the lab offers
// take: a switch, and a list of words.
type DeschedulerArgumentValue interface {
	bool | []string
}

// DeschedulerArgument is one argument of a plugin under its Kubernetes name.
type DeschedulerArgument struct {
	name  string
	value any
}

// NewDeschedulerArgument copies a list value, so the caller's list stays its own.
func NewDeschedulerArgument[V DeschedulerArgumentValue](name string, value V) DeschedulerArgument {
	return DeschedulerArgument{name: name, value: copied(value)}
}

// Name is the argument's key in the plugin's args, under Kubernetes' spelling.
func (a DeschedulerArgument) Name() string {
	return a.name
}

// Value is a bool or a []string, the kinds NewDeschedulerArgument admits.
func (a DeschedulerArgument) Value() any {
	return copied(a.value)
}

// Equal is the equality go-cmp compares through, since the type's fields are
// unexported.
func (a DeschedulerArgument) Equal(other DeschedulerArgument) bool {
	return a.name == other.name && reflect.DeepEqual(a.value, other.value)
}

// A list crosses every boundary as a copy, so no holder of one changes the
// argument it came from.
func copied(value any) any {
	if words, isAList := value.([]string); isAList {
		return slices.Clone(words)
	}

	return value
}

// DeschedulerInterval is the time from one pass's start to the next's, which a
// pass outlasting it stretches to its own exit.
type DeschedulerInterval struct {
	every time.Duration
}

// NewDeschedulerInterval refuses no seconds or fewer, since passes that close
// together would run back to back for the cluster's life.
func NewDeschedulerInterval(seconds int) (DeschedulerInterval, error) {
	if seconds < 1 {
		return DeschedulerInterval{}, fmt.Errorf("a Descheduler runs its passes at least %s apart, "+
			"so an interval of %d seconds is refused", time.Second, seconds)
	}

	return DeschedulerInterval{every: time.Duration(seconds) * time.Second}, nil
}

func (i DeschedulerInterval) Every() time.Duration {
	return i.every
}
