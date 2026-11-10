// Package descheduler holds what a Descheduler policy is, as a Scenario writes
// it: the Descheduler's own descheduler/v1alpha2 policy file as a keyed
// literal, every type, field and name Kubernetes' own. The qualifier reads as
// the subject: descheduler.Policy, descheduler.DefaultEvictor.
//
// It imports neither internal/lab nor the pod or node package.
package descheduler

import (
	"fmt"

	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// Policy is the policy file's DeschedulerPolicy. A field left at its zero value
// stays out of the policy the Descheduler receives, so its own default applies
// and the lab adds none.
type Policy struct {
	MaxNoOfPodsToEvictPerNode uint
	MaxNoOfPodsToEvictTotal   uint
	Profiles                  []Profile
}

// Profile holds its configuration entries and the plugins it enables apart, as
// the policy file does: an entry enables nothing, and the lab derives no
// extension point from a plugin.
type Profile struct {
	Name         string
	PluginConfig []PluginConfig
	Plugins      Plugins
}

// PluginConfig with no Args is an entry of its name alone.
type PluginConfig struct {
	Name PluginName
	Args Args
}

// Plugins holds the extension points a Scenario has needed so far.
type Plugins struct {
	Deschedule        PluginSet
	Balance           PluginSet
	Filter            PluginSet
	PreEvictionFilter PluginSet
}

// PluginSet is one extension point's entry under plugins.
type PluginSet struct {
	Enabled []PluginName
}

// PluginName is a plugin's name as the Descheduler's plugin registry spells it.
type PluginName = scenario.DeschedulerPluginName

// The plugins a Scenario's policy can name.
const (
	DefaultEvictor                              = scenario.DefaultEvictor
	RemoveDuplicates                            = scenario.RemoveDuplicates
	RemovePodsViolatingInterPodAntiAffinity     = scenario.RemovePodsViolatingInterPodAntiAffinity
	RemovePodsViolatingTopologySpreadConstraint = scenario.RemovePodsViolatingTopologySpreadConstraint
)

// Args has unexported methods alone, so the argument types in this package are
// every kind a Scenario can write, each naming the plugin it belongs to.
type Args interface {
	plugin() PluginName
	arguments() []scenario.DeschedulerArgument
}

// DefaultEvictorArgs with NodeFit left out is the Descheduler's own default of
// false.
type DefaultEvictorArgs struct {
	NodeFit bool
}

func (DefaultEvictorArgs) plugin() PluginName {
	return DefaultEvictor
}

func (a DefaultEvictorArgs) arguments() []scenario.DeschedulerArgument {
	if !a.NodeFit {
		return nil
	}

	return []scenario.DeschedulerArgument{scenario.NewDeschedulerArgument(nodeFit, a.NodeFit)}
}

// RemovePodsViolatingTopologySpreadConstraintArgs with Constraints left out is
// the Descheduler's own default.
type RemovePodsViolatingTopologySpreadConstraintArgs struct {
	Constraints []UnsatisfiableConstraintAction
}

func (RemovePodsViolatingTopologySpreadConstraintArgs) plugin() PluginName {
	return RemovePodsViolatingTopologySpreadConstraint
}

func (a RemovePodsViolatingTopologySpreadConstraintArgs) arguments() []scenario.DeschedulerArgument {
	if len(a.Constraints) == 0 {
		return nil
	}

	actions := make([]string, 0, len(a.Constraints))
	for _, action := range a.Constraints {
		actions = append(actions, string(action))
	}

	return []scenario.DeschedulerArgument{scenario.NewDeschedulerArgument(constraints, actions)}
}

// UnsatisfiableConstraintAction is a topology spread constraint's
// whenUnsatisfiable, under Kubernetes' own names.
type UnsatisfiableConstraintAction string

// The two whenUnsatisfiable actions Kubernetes defines.
const (
	DoNotSchedule  UnsatisfiableConstraintAction = "DoNotSchedule"
	ScheduleAnyway UnsatisfiableConstraintAction = "ScheduleAnyway"
)

const (
	nodeFit     = "nodeFit"
	constraints = "constraints"
)

// Name reads the definition the way its declared policy is named, so a
// definition the policy refuses is reported under the same name.
func (p Policy) Name() string {
	names := make([]string, 0, len(p.Profiles))
	for _, profile := range p.Profiles {
		names = append(names, profile.Name)
	}

	return scenario.DeschedulerPolicyName(names...)
}

// Declared carries the immutable domain value, or the first refusal the Given
// declaring the policy ends the Scenario with.
func (p Policy) Declared() (scenario.DeschedulerPolicy, error) {
	profiles := make([]scenario.DeschedulerProfile, 0, len(p.Profiles))
	for _, profile := range p.Profiles {
		declared, err := profile.declared()
		if err != nil {
			return scenario.DeschedulerPolicy{}, err
		}
		profiles = append(profiles, declared)
	}

	declared, err := scenario.NewDeschedulerPolicy(p.MaxNoOfPodsToEvictPerNode, profiles...)
	if err != nil {
		return scenario.DeschedulerPolicy{}, err
	}

	return declared.WithMaxPodsToEvictTotal(p.MaxNoOfPodsToEvictTotal), nil
}

func (p Profile) declared() (scenario.DeschedulerProfile, error) {
	configured := make([]scenario.DeschedulerPluginConfig, 0, len(p.PluginConfig))
	for _, entry := range p.PluginConfig {
		declared, err := entry.declared()
		if err != nil {
			return scenario.DeschedulerProfile{}, fmt.Errorf("the Descheduler profile %s: %w", p.Name, err)
		}
		configured = append(configured, declared)
	}

	return scenario.NewDeschedulerProfile(p.Name, configured, p.Plugins.declared())
}

// The Descheduler decodes an entry's args by the entry's name, so Args of
// another plugin would be read as that name's and are refused here.
func (c PluginConfig) declared() (scenario.DeschedulerPluginConfig, error) {
	if c.Args == nil {
		return scenario.NewDeschedulerPluginConfig(c.Name), nil
	}
	if belongsTo := c.Args.plugin(); belongsTo != c.Name {
		return scenario.DeschedulerPluginConfig{}, fmt.Errorf("the pluginConfig entry %s holds the Args of %s, "+
			"so the entry is refused", c.Name, belongsTo)
	}

	return scenario.NewDeschedulerPluginConfig(c.Name, c.Args.arguments()...), nil
}

// An extension point enabling no plugin is one the definition leaves out.
func (p Plugins) declared() []scenario.DeschedulerPluginSet {
	stated := []struct {
		point scenario.DeschedulerExtensionPoint
		set   PluginSet
	}{
		{point: scenario.DescheduleExtensionPoint, set: p.Deschedule},
		{point: scenario.BalanceExtensionPoint, set: p.Balance},
		{point: scenario.FilterExtensionPoint, set: p.Filter},
		{point: scenario.PreEvictionFilterExtensionPoint, set: p.PreEvictionFilter},
	}

	var enabled []scenario.DeschedulerPluginSet
	for _, each := range stated {
		if len(each.set.Enabled) > 0 {
			enabled = append(enabled, scenario.NewDeschedulerPluginSet(each.point, each.set.Enabled...))
		}
	}

	return enabled
}
