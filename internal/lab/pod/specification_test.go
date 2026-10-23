// These tests reach no cluster, because a definition is a value.
//
// The full-access tests write it as a Scenario does, which is why this file
// imports the Kubernetes package the pod package's own sources never name.
package pod

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/woditschka/k8s-resilience-lab/internal/cluster"
	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

const (
	defaultPlacementNamed   SpecificationName = "Default placement"
	resilientPlacementNamed SpecificationName = "Resilient placement"
)

// The wider skews differ from the definition's and from each other, so a
// customized value is read back as the Scenario's and the later of two
// Customizations is told from the earlier.
const (
	anEvenSkew      = 1
	aWiderSkew      = 2
	anEvenWiderSkew = 3
)

// No test of this package turns on the Startup time.
const (
	someShortestStartupMillis = 500
	someLongestStartupMillis  = 1000
)

func someStartupTime() StartupTime {
	return StartupMillisBetween(someShortestStartupMillis, someLongestStartupMillis)
}

// The lab defines no specification and a test of the pod package cannot import
// the Scenarios' place, so the tests define their own.
func defaultPlacement() Specification {
	return Predefined(defaultPlacementNamed, someStartupTime(), leavingAnUnresponsiveNode())
}

func resilientPlacement() Specification {
	return Predefined(resilientPlacementNamed, someStartupTime(),
		leavingAnUnresponsiveNode(),
		aZoneSpreadAt(anEvenSkew),
		TopologySpreadConstraint{TopologyKey: scenario.HostnameLabel, MaxSkew: anEvenSkew, WhenUnsatisfiable: DoNotSchedule, NodeTaintsPolicy: Honor},
	)
}

// The lab defines no Named setting and the test cannot import the Scenarios'
// place, so the two tolerations are written directly.
func leavingAnUnresponsiveNode() Customization {
	return Settings(
		Toleration{Key: scenario.TaintNodeNotReady, Operator: Exists, Effect: NoExecute, TolerationSeconds: theLabsWaitInSeconds()},
		Toleration{Key: scenario.TaintNodeUnreachable, Operator: Exists, Effect: NoExecute, TolerationSeconds: theLabsWaitInSeconds()},
	)
}

// Every specification holds these two tolerations at the lab's waiting time.
func theNotReadyToleration() scenario.Toleration {
	return scenario.NewToleration(
		scenario.TaintNodeNotReady, scenario.Exists, "", scenario.NoExecute, theLabsWaitInSeconds())
}

func theUnreachableToleration() scenario.Toleration {
	return scenario.NewToleration(
		scenario.TaintNodeUnreachable, scenario.Exists, "", scenario.NoExecute, theLabsWaitInSeconds())
}

func theLabsWaitInSeconds() int64 {
	return int64(scenario.UnresponsiveNodeWait.Seconds())
}

func theEvenZoneSpread() scenario.TopologySpread {
	return theZoneSpreadAt(anEvenSkew)
}

func theEvenNodeSpread() scenario.TopologySpread {
	return scenario.NewTopologySpread(scenario.HostnameLabel, anEvenSkew, scenario.DoNotSchedule, scenario.Honor)
}

func aZoneSpreadAt(maxSkew int) Setting {
	return TopologySpreadConstraint{
		TopologyKey: scenario.ZoneLabel, MaxSkew: maxSkew, WhenUnsatisfiable: DoNotSchedule, NodeTaintsPolicy: Honor,
	}
}

func theZoneSpreadAt(maxSkew int) scenario.TopologySpread {
	return scenario.NewTopologySpread(scenario.ZoneLabel, maxSkew, scenario.DoNotSchedule, scenario.Honor)
}

// The second cpu differs, so the later of two Requests is told from the
// earlier.
const (
	oneCorePerPod      = 1
	twoGibibytesPerPod = 2
	twoCoresPerPod     = 2
)

func aRequestOf(cores, gibibytes int) Setting {
	return Request{CPU: cores, Memory: scenario.Memory(gibibytes) * Gi}
}

func theRequestOf(cores, gibibytes int) scenario.Request {
	return scenario.NewRequest(scenario.CPU(cores*millicoresPerCore), scenario.Memory(gibibytes)*Gi)
}

// No test of this file turns on the priority class: a specification holds the
// place a full access was written in, not the change itself.
const somePriorityClass = "any-priority-class"

// What the change does is read off the deployed Pod template in
// internal/cluster's tests.
func aFullAccess() Customization {
	return FullAccess(func(spec *corev1.PodSpec) { spec.PriorityClassName = somePriorityClass })
}

// A place is the number of settings transcribed before the full access runs.
const (
	afterTheLabsOwnSettings        = 2
	afterASettingOfTheScenariosOwn = 3
)

// The places are all a test can read of a function value, and all the mapper
// needs of one.
func placesOfTheFullAccesses(held []cluster.PodSpecAccessInPlace) []int {
	places := make([]int, 0, len(held))
	for _, written := range held {
		places = append(places, written.AfterSettings)
	}

	return places
}

func TestAGroupOfSettings_ShouldApplyEachSettingInTheOrderWritten(t *testing.T) {
	customized := defaultPlacement().Customize(Settings(aZoneSpreadAt(anEvenSkew), aRequestOf(oneCorePerPod, twoGibibytesPerPod)))

	expect.Expect(t, customized.declared.Settings()).To(expect.Equal([]scenario.Setting{
		theNotReadyToleration(), theUnreachableToleration(), theEvenZoneSpread(),
		theRequestOf(oneCorePerPod, twoGibibytesPerPod),
	}))
}

func TestAGroupOfSettingsNamingOneFieldTwice_ShouldHoldTheLaterSetting(t *testing.T) {
	const aLooserSkew = 3
	customized := defaultPlacement().Customize(Settings(aZoneSpreadAt(anEvenSkew), aZoneSpreadAt(aLooserSkew)))

	expect.Expect(t, customized.declared.TopologySpreads()).To(expect.Equal([]scenario.TopologySpread{theZoneSpreadAt(aLooserSkew)}))
}

func TestAGroupWithoutASetting_ShouldChangeNothing(t *testing.T) {
	expect.Expect(t, resilientPlacement().Customize(Settings()).Equal(resilientPlacement())).To(expect.Equal(true))
}

func TestASpecificationDefinedThroughTheMechanism_ShouldHoldItsNameItsStartupTimeAndEverySettingWritten(t *testing.T) {
	defined := resilientPlacement()

	if defined.refused != nil {
		t.Fatalf("a definition naming its Startup time is admitted, but it was refused: %v", defined.refused)
	}
	expect.Expect(t, defined.declared.Name()).To(expect.Equal(string(resilientPlacementNamed)))
	expect.Expect(t, defined.declared.StartupTime()).
		To(expect.Equal(theStartupTimeBetween(t, someShortestStartupMillis*time.Millisecond, someLongestStartupMillis*time.Millisecond)))
	expect.Expect(t, defined.declared.Settings()).To(expect.Equal([]scenario.Setting{
		theNotReadyToleration(), theUnreachableToleration(), theEvenZoneSpread(), theEvenNodeSpread(),
	}))
}

func TestASpecificationDeclaringNoSpreadOfItsOwn_ShouldStillNameTheSettingsTheLabAdds(t *testing.T) {
	declared := defaultPlacement().declared

	expect.Expect(t, declared.TopologySpreads()).To(expect.HaveLen[scenario.TopologySpread](0))
	expect.Expect(t, declared.Tolerations()).
		To(expect.Equal([]scenario.Toleration{theNotReadyToleration(), theUnreachableToleration()}))
}

func TestOnePredefinedSpecificationAskedForTwice_ShouldGiveTwoEqualSpecifications(t *testing.T) {
	expect.Expect(t, resilientPlacement().Equal(resilientPlacement())).To(expect.Equal(true))
	expect.Expect(t, resilientPlacement().Equal(defaultPlacement())).To(expect.Equal(false))
}

func TestASpecificationCustomizedByAScenario_ShouldHoldTheCustomizedSettingsAndKeepTheRest(t *testing.T) {
	written := map[string]struct {
		declares       Specification
		customizations []Customization
		held           []scenario.Setting
	}{
		"a setting the specification sets": {
			declares:       resilientPlacement(),
			customizations: []Customization{aZoneSpreadAt(aWiderSkew)},
			held: []scenario.Setting{
				theNotReadyToleration(), theUnreachableToleration(), theZoneSpreadAt(aWiderSkew), theEvenNodeSpread(),
			},
		},
		"two Customizations naming one setting": {
			declares:       resilientPlacement(),
			customizations: []Customization{aZoneSpreadAt(aWiderSkew), aZoneSpreadAt(anEvenWiderSkew)},
			held: []scenario.Setting{
				theNotReadyToleration(), theUnreachableToleration(), theZoneSpreadAt(anEvenWiderSkew), theEvenNodeSpread(),
			},
		},
		"a setting the specification does not hold": {
			declares:       defaultPlacement(),
			customizations: []Customization{aZoneSpreadAt(aWiderSkew)},
			held: []scenario.Setting{
				theNotReadyToleration(), theUnreachableToleration(), theZoneSpreadAt(aWiderSkew),
			},
		},
	}

	for how, taken := range written {
		t.Run(how, func(t *testing.T) {
			customized := taken.declares.Customize(taken.customizations...)

			expect.Expect(t, customized.declared.Settings()).To(expect.Equal(taken.held))
		})
	}
}

func TestARequestWrittenAfterAnother_ShouldStandAsTheLaterSetting(t *testing.T) {
	declaring := defaultPlacement()

	customized := declaring.Customize(
		aRequestOf(oneCorePerPod, twoGibibytesPerPod), aRequestOf(twoCoresPerPod, twoGibibytesPerPod))

	expect.Expect(t, customized.declared.Settings()).To(expect.Equal([]scenario.Setting{
		theNotReadyToleration(), theUnreachableToleration(), theRequestOf(twoCoresPerPod, twoGibibytesPerPod),
	}))
}

// The place is the number of settings held when the full access is written,
// which the mapper transcribes before running it. A setting written after the
// full access keeps the place its field was first written, so the full access
// stands where both name one Kubernetes field.
func TestASpecificationCustomizedWithFullAccess_ShouldHoldItInThePlaceItWasWrittenIn(t *testing.T) {
	written := map[string]struct {
		customizations []Customization
		held           []scenario.Setting
		places         []int
	}{
		"full access alone": {
			customizations: []Customization{aFullAccess()},
			held:           []scenario.Setting{theNotReadyToleration(), theUnreachableToleration()},
			places:         []int{afterTheLabsOwnSettings},
		},
		"full access after a setting the lab offers": {
			customizations: []Customization{aZoneSpreadAt(aWiderSkew), aFullAccess()},
			held: []scenario.Setting{
				theNotReadyToleration(), theUnreachableToleration(), theZoneSpreadAt(aWiderSkew),
			},
			places: []int{afterASettingOfTheScenariosOwn},
		},
		"full access before it": {
			customizations: []Customization{aFullAccess(), aZoneSpreadAt(aWiderSkew)},
			held: []scenario.Setting{
				theNotReadyToleration(), theUnreachableToleration(), theZoneSpreadAt(aWiderSkew),
			},
			places: []int{afterTheLabsOwnSettings},
		},
		"two full accesses": {
			customizations: []Customization{aFullAccess(), aFullAccess()},
			held:           []scenario.Setting{theNotReadyToleration(), theUnreachableToleration()},
			places:         []int{afterTheLabsOwnSettings, afterTheLabsOwnSettings},
		},
	}

	for order, taken := range written {
		t.Run(order, func(t *testing.T) {
			customized := defaultPlacement().Customize(taken.customizations...)

			expect.Expect(t, customized.declared.Settings()).To(expect.Equal(taken.held))
			expect.Expect(t, placesOfTheFullAccesses(customized.accesses)).To(expect.Equal(taken.places))
		})
	}
}

func TestTwoSpecificationsHoldingFullAccess_ShouldBeEqualOnlyWhereItStandsInTheSamePlace(t *testing.T) {
	declared := defaultPlacement()

	expect.Expect(t, declared.Customize(aFullAccess()).Equal(declared.Customize(aFullAccess()))).
		To(expect.Equal(true))
	expect.Expect(t, declared.Customize(aFullAccess()).Equal(declared)).
		To(expect.Equal(false))
	expect.Expect(t, declared.Customize(aFullAccess(), aZoneSpreadAt(aWiderSkew)).
		Equal(declared.Customize(aZoneSpreadAt(aWiderSkew), aFullAccess()))).
		To(expect.Equal(false))
}

func TestAPredefinedSpecificationCustomizedByOneDeployment_ShouldStayAsItWasForAnother(t *testing.T) {
	declaredByTheFirstDeployment := resilientPlacement()

	customized := declaredByTheFirstDeployment.Customize(aZoneSpreadAt(aWiderSkew))

	expect.Expect(t, declaredByTheFirstDeployment.Equal(resilientPlacement())).To(expect.Equal(true))
	expect.Expect(t, customized.Equal(resilientPlacement())).To(expect.Equal(false))
}

// The times differ from each other and from the definitions' own, so the time
// held is read back as the Scenario's and the later of two is told from the
// earlier. The millisecond ones are no whole second, which no seconds form
// could state.
const (
	aShortStartup        = 3
	aLongerStartup       = 7
	aShortStartupMillis  = 300
	aLongerStartupMillis = 1700
)

func theStartupTimeBetween(t *testing.T, shortest, longest time.Duration) scenario.StartupTime {
	t.Helper()

	stated, err := scenario.NewStartupTime(int(shortest.Milliseconds()), int(longest.Milliseconds()))
	if err != nil {
		t.Fatalf("build a Startup time between %s and %s: %v", shortest, longest, err)
	}

	return stated
}

func TestAStartupTimeAScenarioStates_ShouldBeTheOneItsDeploymentsPodsTake(t *testing.T) {
	written := map[string]struct {
		customization Customization
		shortest      time.Duration
		longest       time.Duration
	}{
		"one time": {
			customization: StartupSeconds(aShortStartup),
			shortest:      aShortStartup * time.Second, longest: aShortStartup * time.Second,
		},
		"a shortest and a longest": {
			customization: StartupSecondsBetween(aShortStartup, aLongerStartup),
			shortest:      aShortStartup * time.Second, longest: aLongerStartup * time.Second,
		},
	}

	for how, taken := range written {
		t.Run(how, func(t *testing.T) {
			customized := resilientPlacement().Customize(taken.customization)

			expect.Expect(t, customized.declared.StartupTime()).To(expect.Equal(theStartupTimeBetween(t, taken.shortest, taken.longest)))
			expect.Expect(t, customized.declared.Settings()).To(expect.Equal(resilientPlacement().declared.Settings()))
		})
	}
}

func TestAStartupTimeInMilliseconds_ShouldBeTheOneItsDeploymentsPodsTake(t *testing.T) {
	written := map[string]struct {
		customization Customization
		shortest      time.Duration
		longest       time.Duration
	}{
		"one time": {
			customization: StartupMillis(aShortStartupMillis),
			shortest:      aShortStartupMillis * time.Millisecond, longest: aShortStartupMillis * time.Millisecond,
		},
		"a shortest and a longest": {
			customization: StartupMillisBetween(aShortStartupMillis, aLongerStartupMillis),
			shortest:      aShortStartupMillis * time.Millisecond, longest: aLongerStartupMillis * time.Millisecond,
		},
	}

	for how, taken := range written {
		t.Run(how, func(t *testing.T) {
			customized := resilientPlacement().Customize(taken.customization)

			expect.Expect(t, customized.declared.StartupTime()).To(expect.Equal(theStartupTimeBetween(t, taken.shortest, taken.longest)))
		})
	}
}

func TestTwoStartupTimeCustomizations_ShouldLeaveTheLaterOneStanding(t *testing.T) {
	written := map[string]struct {
		customized Specification
		theLater   time.Duration
	}{
		"both in seconds": {
			customized: defaultPlacement().Customize(StartupSeconds(aShortStartup), StartupSeconds(aLongerStartup)),
			theLater:   aLongerStartup * time.Second,
		},
		"seconds, then milliseconds": {
			customized: defaultPlacement().Customize(StartupSeconds(aLongerStartup), StartupMillis(aShortStartupMillis)),
			theLater:   aShortStartupMillis * time.Millisecond,
		},
		"milliseconds, then seconds": {
			customized: defaultPlacement().Customize(StartupMillis(aShortStartupMillis), StartupSeconds(aShortStartup)),
			theLater:   aShortStartup * time.Second,
		},
	}

	for order, startup := range written {
		t.Run(order, func(t *testing.T) {
			expect.Expect(t, startup.customized.declared.StartupTime()).To(expect.Equal(theStartupTimeBetween(t, startup.theLater, startup.theLater)))
		})
	}
}

func TestAStartupTimeCustomizedForOneDeployment_ShouldLeaveTheSpecificationItCustomizedUnchanged(t *testing.T) {
	declaredByTheFirstDeployment := resilientPlacement()

	customized := declaredByTheFirstDeployment.Customize(StartupSeconds(aLongerStartup))

	expect.Expect(t, declaredByTheFirstDeployment.Equal(resilientPlacement())).To(expect.Equal(true))
	expect.Expect(t, customized.Equal(resilientPlacement())).To(expect.Equal(false))
	expect.Expect(t, declaredByTheFirstDeployment.declared.StartupTime()).To(expect.Equal(resilientPlacement().declared.StartupTime()))
}
