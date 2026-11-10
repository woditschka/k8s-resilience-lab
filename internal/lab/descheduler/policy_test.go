package descheduler_test

import (
	"strings"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/lab/descheduler"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

const (
	// No test here turns on a profile's name beyond telling two apart.
	someProfileName    = "a-descheduler-profile"
	anotherProfileName = "another-descheduler-profile"

	oneEvictionPerNode uint = 1
	oneEvictionPerPass uint = 1

	theNodeFitArgument     = "nodeFit"
	theConstraintsArgument = "constraints"
)

func TestADeschedulerPolicy_ShouldCarryEveryPluginAndArgumentItsDefinitionStatesAndNothingTheLabAdds(t *testing.T) {
	defined := descheduler.Policy{
		MaxNoOfPodsToEvictPerNode: oneEvictionPerNode,
		MaxNoOfPodsToEvictTotal:   oneEvictionPerPass,
		Profiles: []descheduler.Profile{{
			Name: someProfileName,
			PluginConfig: []descheduler.PluginConfig{
				{Name: descheduler.DefaultEvictor, Args: descheduler.DefaultEvictorArgs{NodeFit: true}},
				{
					Name: descheduler.RemovePodsViolatingTopologySpreadConstraint,
					Args: descheduler.RemovePodsViolatingTopologySpreadConstraintArgs{
						Constraints: []descheduler.UnsatisfiableConstraintAction{descheduler.DoNotSchedule, descheduler.ScheduleAnyway},
					},
				},
				{Name: descheduler.RemoveDuplicates},
			},
			Plugins: descheduler.Plugins{
				Balance: descheduler.PluginSet{Enabled: []descheduler.PluginName{
					descheduler.RemovePodsViolatingTopologySpreadConstraint, descheduler.RemoveDuplicates,
				}},
				Deschedule: descheduler.PluginSet{Enabled: []descheduler.PluginName{descheduler.RemovePodsViolatingInterPodAntiAffinity}},
			},
		}, {
			Name:         anotherProfileName,
			PluginConfig: []descheduler.PluginConfig{{Name: descheduler.DefaultEvictor, Args: descheduler.DefaultEvictorArgs{}}},
			Plugins: descheduler.Plugins{
				Filter: descheduler.PluginSet{Enabled: []descheduler.PluginName{descheduler.DefaultEvictor}},
			},
		}},
	}

	declared, err := defined.Declared()

	if err != nil {
		t.Fatalf("define a Descheduler policy of the profiles %s and %s: %v", someProfileName, anotherProfileName, err)
	}
	expect.Expect(t, declared).To(expect.Equal(aPolicyOf(t, oneEvictionPerNode,
		aProfileOf(t, someProfileName,
			[]scenario.DeschedulerPluginConfig{
				scenario.NewDeschedulerPluginConfig(scenario.DefaultEvictor, scenario.NewDeschedulerArgument(theNodeFitArgument, true)),
				scenario.NewDeschedulerPluginConfig(scenario.RemovePodsViolatingTopologySpreadConstraint,
					scenario.NewDeschedulerArgument(theConstraintsArgument, []string{
						string(descheduler.DoNotSchedule), string(descheduler.ScheduleAnyway),
					})),
				scenario.NewDeschedulerPluginConfig(scenario.RemoveDuplicates),
			},
			scenario.NewDeschedulerPluginSet(scenario.DescheduleExtensionPoint, scenario.RemovePodsViolatingInterPodAntiAffinity),
			scenario.NewDeschedulerPluginSet(scenario.BalanceExtensionPoint,
				scenario.RemovePodsViolatingTopologySpreadConstraint, scenario.RemoveDuplicates),
		),
		aProfileOf(t, anotherProfileName,
			[]scenario.DeschedulerPluginConfig{scenario.NewDeschedulerPluginConfig(scenario.DefaultEvictor)},
			scenario.NewDeschedulerPluginSet(scenario.FilterExtensionPoint, scenario.DefaultEvictor),
		),
	).WithMaxPodsToEvictTotal(oneEvictionPerPass)))
}

func TestADeschedulerPolicysEntryHoldingAnotherPluginsArgs_ShouldBeRefusedNamingTheEntryAndThatPlugin(t *testing.T) {
	defined := descheduler.Policy{Profiles: []descheduler.Profile{{
		Name: someProfileName,
		PluginConfig: []descheduler.PluginConfig{
			{Name: descheduler.RemoveDuplicates, Args: descheduler.DefaultEvictorArgs{NodeFit: true}},
		},
		Plugins: descheduler.Plugins{Balance: descheduler.PluginSet{Enabled: []descheduler.PluginName{descheduler.RemoveDuplicates}}},
	}}}

	_, refused := defined.Declared()

	if refused == nil || !strings.Contains(refused.Error(), string(descheduler.RemoveDuplicates)) ||
		!strings.Contains(refused.Error(), string(descheduler.DefaultEvictor)) {
		t.Fatalf("an entry holding another plugin's Args is refused naming %s and %s, got: %v",
			descheduler.RemoveDuplicates, descheduler.DefaultEvictor, refused)
	}
}

func TestADeschedulerProfileEnablingNoPlugin_ShouldCarryTheRefusal(t *testing.T) {
	defined := descheduler.Policy{Profiles: []descheduler.Profile{{
		Name:         someProfileName,
		PluginConfig: []descheduler.PluginConfig{{Name: descheduler.RemoveDuplicates}},
	}}}

	_, refused := defined.Declared()

	if refused == nil || !strings.Contains(refused.Error(), someProfileName) {
		t.Fatalf("a profile enabling no plugin is refused naming %s, got: %v", someProfileName, refused)
	}
}

func TestADeschedulerPolicysDefinition_ShouldBeNamedAsThePolicyItDeclares(t *testing.T) {
	defined := descheduler.Policy{Profiles: []descheduler.Profile{
		{Name: someProfileName, Plugins: descheduler.Plugins{
			Balance: descheduler.PluginSet{Enabled: []descheduler.PluginName{descheduler.RemoveDuplicates}},
		}},
		{Name: anotherProfileName, Plugins: descheduler.Plugins{
			Deschedule: descheduler.PluginSet{Enabled: []descheduler.PluginName{descheduler.RemovePodsViolatingInterPodAntiAffinity}},
		}},
	}}

	declared, err := defined.Declared()
	if err != nil {
		t.Fatalf("define a Descheduler policy of the profiles %s and %s: %v", someProfileName, anotherProfileName, err)
	}

	expect.Expect(t, defined.Name()).To(expect.Equal(declared.Name()))
}

func aProfileOf(t *testing.T, name string, configured []scenario.DeschedulerPluginConfig,
	enabled ...scenario.DeschedulerPluginSet,
) scenario.DeschedulerProfile {
	t.Helper()

	profile, err := scenario.NewDeschedulerProfile(name, configured, enabled)
	if err != nil {
		t.Fatalf("build the Descheduler profile %q: %v", name, err)
	}

	return profile
}

func aPolicyOf(t *testing.T, maxPodsToEvictPerNode uint, profiles ...scenario.DeschedulerProfile) scenario.DeschedulerPolicy {
	t.Helper()

	declared, err := scenario.NewDeschedulerPolicy(maxPodsToEvictPerNode, profiles...)
	if err != nil {
		t.Fatalf("build a Descheduler policy: %v", err)
	}

	return declared
}
