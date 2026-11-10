package scenario_test

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

const (
	// No test here turns on a profile's name beyond telling two apart.
	someProfileName    = "a-descheduler-profile"
	anotherProfileName = "another-descheduler-profile"

	theNodeFitArgument     = "nodeFit"
	theConstraintsArgument = "constraints"
	aConstraintAction      = "DoNotSchedule"

	twoPodsPerNode uint = 2
	onePodPerPass  uint = 1
	// No test here turns on the per-Node eviction limit but the one holding it.
	anyLimitPerNode uint = 0
)

func aProfileEnablingOnePlugin(t *testing.T, name string) scenario.DeschedulerProfile {
	t.Helper()

	profile, err := scenario.NewDeschedulerProfile(name, nil, []scenario.DeschedulerPluginSet{
		scenario.NewDeschedulerPluginSet(scenario.BalanceExtensionPoint, scenario.RemoveDuplicates),
	})
	if err != nil {
		t.Fatalf("build the Descheduler profile %q: %v", name, err)
	}

	return profile
}

func TestADeschedulerPolicy_ShouldHoldItsLimitAndEveryProfileWithItsEntriesAndEnabledPluginsInDefinitionOrder(t *testing.T) {
	balancing := scenario.NewDeschedulerPluginConfig(scenario.RemovePodsViolatingTopologySpreadConstraint,
		scenario.NewDeschedulerArgument(theConstraintsArgument, []string{aConstraintAction}))
	evicting := scenario.NewDeschedulerPluginConfig(scenario.DefaultEvictor,
		scenario.NewDeschedulerArgument(theNodeFitArgument, true))
	balanced := scenario.NewDeschedulerPluginSet(scenario.BalanceExtensionPoint,
		scenario.RemovePodsViolatingTopologySpreadConstraint, scenario.RemoveDuplicates)
	descheduled := scenario.NewDeschedulerPluginSet(scenario.DescheduleExtensionPoint,
		scenario.RemovePodsViolatingInterPodAntiAffinity)
	profile, err := scenario.NewDeschedulerProfile(someProfileName,
		[]scenario.DeschedulerPluginConfig{balancing, evicting}, []scenario.DeschedulerPluginSet{balanced, descheduled})
	if err != nil {
		t.Fatalf("build the Descheduler profile %q: %v", someProfileName, err)
	}

	declared, err := scenario.NewDeschedulerPolicy(twoPodsPerNode, profile)
	if err != nil {
		t.Fatalf("build a Descheduler policy of the profile %q: %v", someProfileName, err)
	}

	expect.Expect(t, declared.MaxPodsToEvictPerNode()).To(expect.Equal(twoPodsPerNode))
	expect.Expect(t, declared.Profiles()).To(expect.Equal([]scenario.DeschedulerProfile{profile}))
	expect.Expect(t, profile.PluginConfig()).To(expect.Equal([]scenario.DeschedulerPluginConfig{balancing, evicting}))
	expect.Expect(t, profile.Enabled()).To(expect.Equal([]scenario.DeschedulerPluginSet{balanced, descheduled}))
}

func TestADeschedulerPolicyWithATotalLimit_ShouldHoldItAndDifferFromThePolicyWithoutIt(t *testing.T) {
	withoutTotal, err := scenario.NewDeschedulerPolicy(anyLimitPerNode, aProfileEnablingOnePlugin(t, someProfileName))
	if err != nil {
		t.Fatalf("build a Descheduler policy of the profile %q: %v", someProfileName, err)
	}

	withTotal := withoutTotal.WithMaxPodsToEvictTotal(onePodPerPass)

	expect.Expect(t, withTotal.MaxPodsToEvictTotal()).To(expect.Equal(onePodPerPass))
	expect.Expect(t, withTotal.Equal(withoutTotal)).To(expect.Equal(false))
}

func TestADeschedulerPolicy_ShouldBeNamedByItsProfileNamesInOrder(t *testing.T) {
	declared, err := scenario.NewDeschedulerPolicy(anyLimitPerNode,
		aProfileEnablingOnePlugin(t, someProfileName), aProfileEnablingOnePlugin(t, anotherProfileName))
	if err != nil {
		t.Fatalf("build a Descheduler policy of two profiles: %v", err)
	}

	expect.Expect(t, declared.Name()).To(expect.Equal(someProfileName + ", " + anotherProfileName))
}

func TestADeschedulerPolicyOfNoProfileOrOfTwoProfilesOfOneName_ShouldBeRefused(t *testing.T) {
	refused := map[string][]scenario.DeschedulerProfile{
		"a policy of no profile": nil,
		"a policy of two profiles of one name": {
			aProfileEnablingOnePlugin(t, someProfileName), aProfileEnablingOnePlugin(t, someProfileName),
		},
	}

	for named, profiles := range refused {
		t.Run(named, func(t *testing.T) {
			declared, err := scenario.NewDeschedulerPolicy(anyLimitPerNode, profiles...)

			if err == nil {
				t.Fatalf("%s is refused, but it was built: %+v", named, declared)
			}
		})
	}
}

func TestADeschedulerProfileOfNoNameOrEnablingNoPlugin_ShouldBeRefused(t *testing.T) {
	somePlugins := []scenario.DeschedulerPluginSet{
		scenario.NewDeschedulerPluginSet(scenario.BalanceExtensionPoint, scenario.RemoveDuplicates),
	}
	refused := map[string]struct {
		name    string
		enabled []scenario.DeschedulerPluginSet
	}{
		"a profile of no name":             {name: noName, enabled: somePlugins},
		"a profile enabling no plugin":     {name: someProfileName},
		"a profile of an empty plugin set": {name: someProfileName, enabled: []scenario.DeschedulerPluginSet{scenario.NewDeschedulerPluginSet(scenario.FilterExtensionPoint)}},
	}

	for named, written := range refused {
		t.Run(named, func(t *testing.T) {
			declared, err := scenario.NewDeschedulerProfile(written.name, nil, written.enabled)

			if err == nil {
				t.Fatalf("%s is refused, but it was built: %+v", named, declared)
			}
		})
	}
}

func TestADeschedulerPolicy_ShouldNotChangeWhenAListItWasBuiltFromOrReadAsIsChanged(t *testing.T) {
	words := []string{aConstraintAction}
	configured := []scenario.DeschedulerPluginConfig{scenario.NewDeschedulerPluginConfig(
		scenario.RemovePodsViolatingTopologySpreadConstraint, scenario.NewDeschedulerArgument(theConstraintsArgument, words))}
	plugins := []scenario.DeschedulerPluginName{scenario.RemovePodsViolatingTopologySpreadConstraint}
	enabled := []scenario.DeschedulerPluginSet{scenario.NewDeschedulerPluginSet(scenario.BalanceExtensionPoint, plugins...)}
	profile, err := scenario.NewDeschedulerProfile(someProfileName, configured, enabled)
	if err != nil {
		t.Fatalf("build the Descheduler profile %q: %v", someProfileName, err)
	}
	built, err := scenario.NewDeschedulerPolicy(anyLimitPerNode, profile)
	if err != nil {
		t.Fatalf("build a Descheduler policy of the profile %q: %v", someProfileName, err)
	}
	unchangedProfile, err := scenario.NewDeschedulerProfile(someProfileName,
		[]scenario.DeschedulerPluginConfig{scenario.NewDeschedulerPluginConfig(scenario.RemovePodsViolatingTopologySpreadConstraint,
			scenario.NewDeschedulerArgument(theConstraintsArgument, []string{aConstraintAction}))},
		[]scenario.DeschedulerPluginSet{scenario.NewDeschedulerPluginSet(scenario.BalanceExtensionPoint,
			scenario.RemovePodsViolatingTopologySpreadConstraint)})
	if err != nil {
		t.Fatalf("build the Descheduler profile %q: %v", someProfileName, err)
	}
	unchanged, err := scenario.NewDeschedulerPolicy(anyLimitPerNode, unchangedProfile)
	if err != nil {
		t.Fatalf("build a Descheduler policy of the profile %q: %v", someProfileName, err)
	}

	words[0] = "changed"
	plugins[0] = scenario.DefaultEvictor
	configured[0] = scenario.NewDeschedulerPluginConfig(scenario.DefaultEvictor)
	enabled[0] = scenario.NewDeschedulerPluginSet(scenario.FilterExtensionPoint, scenario.DefaultEvictor)
	readProfiles := built.Profiles()
	readConfigured := readProfiles[0].PluginConfig()
	readArguments := readConfigured[0].Arguments()
	readArguments[0].Value().([]string)[0] = "changed"
	readArguments[0] = scenario.NewDeschedulerArgument(theNodeFitArgument, true)
	readConfigured[0] = scenario.NewDeschedulerPluginConfig(scenario.DefaultEvictor)
	readEnabled := readProfiles[0].Enabled()
	readEnabled[0].Plugins()[0] = scenario.DefaultEvictor
	readEnabled[0] = scenario.NewDeschedulerPluginSet(scenario.FilterExtensionPoint)
	readProfiles[0] = aProfileEnablingOnePlugin(t, anotherProfileName)

	expect.Expect(t, built).To(expect.Equal(unchanged))
}

func TestADeschedulerIntervalOfNoSecondsOrFewer_ShouldBeRefusedNamingTheSecondsStated(t *testing.T) {
	refusals := map[string]int{
		"no seconds at all": noSecondsAtAll,
		"a negative time":   aNegativeTime,
	}

	for refused, seconds := range refusals {
		t.Run(refused, func(t *testing.T) {
			_, err := scenario.NewDeschedulerInterval(seconds)

			if err == nil {
				t.Fatalf("a Descheduler interval of %d seconds is refused, but it was admitted", seconds)
			}
			if !strings.Contains(err.Error(), strconv.Itoa(seconds)) {
				t.Errorf("the refusal names %d, but reads: %v", seconds, err)
			}
		})
	}
}

func TestADeschedulerInterval_ShouldHoldTheSecondsStated(t *testing.T) {
	const fiveSeconds = 5

	interval, err := scenario.NewDeschedulerInterval(fiveSeconds)

	if err != nil {
		t.Fatalf("build a Descheduler interval of %d seconds: %v", fiveSeconds, err)
	}
	expect.Expect(t, interval.Every()).To(expect.Equal(fiveSeconds * time.Second))
}
