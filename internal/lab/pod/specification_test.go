// The tests in this file read a predefined Pod specification the way an engineer
// opening its definition does: as the Startup time it names, the settings it sets
// and the value it sets them to. They reach no cluster, because a definition is a
// value.
//
// The tests of full access below write it the way a Scenario writes it, which is
// why this file names the one Kubernetes package such a Scenario imports. The
// pod package's own sources name none: a full access is carried as the function
// type internal/cluster declares for it.
package pod

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/woditschka/k8s-resilience-lab/internal/cluster"
	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// The names the two specifications below are defined under, which is what the
// Step declaring one reports.
const (
	defaultPlacementNamed   SpecificationName = "Default placement"
	resilientPlacementNamed SpecificationName = "Resilient placement"
)

// anEvenSkew is the Skew that makes a spread even, and aWiderSkew and
// anEvenWiderSkew the two above it a Customization below is written at, so what a
// customized specification holds is read back as the Scenario's value and not as
// the definition's, and the later of two Customizations is told from the earlier.
const (
	anEvenSkew      = 1
	aWiderSkew      = 2
	anEvenWiderSkew = 3
)

// The Startup time the two specifications below name, in the whole milliseconds
// a definition states it in: a range above zero, as the lab's Scenarios define
// theirs, which no test of this package turns on.
const (
	someShortestStartupMillis = 500
	someLongestStartupMillis  = 1000
)

// someStartupTime is that Startup time as a definition writes it.
func someStartupTime() StartupTime {
	return StartupMillisBetween(someShortestStartupMillis, someLongestStartupMillis)
}

// defaultPlacement and resilientPlacement are the two specifications the
// machinery's tests declare, defined beside them through the mechanism the way the
// Scenarios' place defines its own: the lab defines none, and a test of
// the pod package cannot import that place, per
// docs/adr/2026-10-18-every-pod-specification-is-defined-by-the-scenarios.md.
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

// leavingAnUnresponsiveNode writes the two tolerations a Scenario's Named setting
// stands for, directly, where a test of the pod package needs them: the lab defines
// no Named setting, and the test cannot import the Scenarios' place.
func leavingAnUnresponsiveNode() Customization {
	return Settings(
		Toleration{Key: scenario.TaintNodeNotReady, Operator: Exists, Effect: NoExecute, TolerationSeconds: theLabsWaitInSeconds()},
		Toleration{Key: scenario.TaintNodeUnreachable, Operator: Exists, Effect: NoExecute, TolerationSeconds: theLabsWaitInSeconds()},
	)
}

// theNotReadyToleration and theUnreachableToleration are the lab's own settings,
// which every specification holds: a Pod leaves a Node that stopped responding
// after the lab's own waiting time, for each of the two taints the control plane
// adds to such a Node.
func theNotReadyToleration() scenario.Toleration {
	return scenario.NewToleration(
		scenario.TaintNodeNotReady, scenario.Exists, "", scenario.NoExecute, theLabsWaitInSeconds())
}

func theUnreachableToleration() scenario.Toleration {
	return scenario.NewToleration(
		scenario.TaintNodeUnreachable, scenario.Exists, "", scenario.NoExecute, theLabsWaitInSeconds())
}

// theLabsWaitInSeconds is the lab's own waiting time as the field Kubernetes
// holds it in, read from the constant that sets it rather than written out.
func theLabsWaitInSeconds() int64 {
	return int64(scenario.UnresponsiveNodeWait.Seconds())
}

// theEvenZoneSpread and theEvenNodeSpread are what Resilient placement sets on
// top of them: its Workload's Pods spread as evenly over the Zones of its
// Topology, and over its Nodes, as the control plane can place them.
func theEvenZoneSpread() scenario.TopologySpread {
	return theZoneSpreadAt(anEvenSkew)
}

func theEvenNodeSpread() scenario.TopologySpread {
	return scenario.NewTopologySpread(scenario.HostnameLabel, anEvenSkew, scenario.DoNotSchedule, scenario.Honor)
}

// aZoneSpreadAt is a Customization as a Scenario writes one, under Kubernetes' names: the Zone spread at the Skew named, which is the one
// setting a Scenario may change. theZoneSpreadAt is that same setting as the
// domain holds it, which is what a customized specification reads back as.
func aZoneSpreadAt(maxSkew int) Setting {
	return TopologySpreadConstraint{
		TopologyKey: scenario.ZoneLabel, MaxSkew: maxSkew, WhenUnsatisfiable: DoNotSchedule, NodeTaintsPolicy: Honor,
	}
}

func theZoneSpreadAt(maxSkew int) scenario.TopologySpread {
	return scenario.NewTopologySpread(scenario.ZoneLabel, maxSkew, scenario.DoNotSchedule, scenario.Honor)
}

// The cpu and memory a Request below asks for, in the whole cores and whole
// gibibytes a Scenario states them in, and the larger cpu a second Request asks
// for, so the later of two Requests is told from the earlier.
const (
	oneCorePerPod      = 1
	twoGibibytesPerPod = 2
	twoCoresPerPod     = 2
)

// aRequestOf is a Request as a Scenario writes one, under Kubernetes' names. theRequestOf is that same setting as the domain holds
// it, which is what a customized specification reads back as.
func aRequestOf(cores, gibibytes int) Setting {
	return Request{CPU: cores, Memory: scenario.Memory(gibibytes) * Gi}
}

func theRequestOf(cores, gibibytes int) scenario.Request {
	return scenario.NewRequest(scenario.CPU(cores*millicoresPerCore), scenario.Memory(gibibytes)*Gi)
}

// somePriorityClass is a Kubernetes Pod setting the lab does not
// name, which no test of this file turns on: what a specification holds of a full
// access is the place it was written in and not the change itself.
const somePriorityClass = "any-priority-class"

// aFullAccess is full access as a Scenario writes one, in the Scenario's own
// spelling. What the change does is read off the deployed Pod template, in
// internal/cluster/resources_test.go.
func aFullAccess() Customization {
	return FullAccess(func(spec *corev1.PodSpec) { spec.PriorityClassName = somePriorityClass })
}

// The places a full access holds in the tests below, as the number of settings
// transcribed before it runs: after the lab's two tolerations every specification
// holds, and after one further setting a Scenario wrote itself.
const (
	afterTheLabsOwnSettings        = 2
	afterASettingOfTheScenariosOwn = 3
)

// placesOfTheFullAccesses are the places these full accesses were written in.
// They are all a test can read of a function value, and all the mapper needs of
// one.
func placesOfTheFullAccesses(held []cluster.PodSpecAccessInPlace) []int {
	places := make([]int, 0, len(held))
	for _, written := range held {
		places = append(places, written.AfterSettings)
	}

	return places
}

// A group applies its settings in the order written, so the specification holds
// them in that order (docs/prd.md REQ-SIM-016).
func TestAGroupOfSettings_ShouldApplyEachSettingInTheOrderWritten(t *testing.T) {
	customized := defaultPlacement().Customize(Settings(aZoneSpreadAt(anEvenSkew), aRequestOf(oneCorePerPod, twoGibibytesPerPod)))

	expect.Expect(t, customized.declared.Settings()).To(expect.Equal([]scenario.Setting{
		theNotReadyToleration(), theUnreachableToleration(), theEvenZoneSpread(),
		theRequestOf(oneCorePerPod, twoGibibytesPerPod),
	}))
}

// Of two settings in a group naming one field the later one stands, as it does
// written one by one.
func TestAGroupOfSettingsNamingOneFieldTwice_ShouldHoldTheLaterSetting(t *testing.T) {
	const aLooserSkew = 3
	customized := defaultPlacement().Customize(Settings(aZoneSpreadAt(anEvenSkew), aZoneSpreadAt(aLooserSkew)))

	expect.Expect(t, customized.declared.TopologySpreads()).To(expect.Equal([]scenario.TopologySpread{theZoneSpreadAt(aLooserSkew)}))
}

// A group without a setting is no change.
func TestAGroupWithoutASetting_ShouldChangeNothing(t *testing.T) {
	expect.Expect(t, resilientPlacement().Customize(Settings()).Equal(resilientPlacement())).To(expect.Equal(true))
}

// A definition names its specification, the Startup time its Pods take, and
// every setting it holds with the value it sets (docs/prd.md REQ-SIM-009 and
// REQ-SIM-015).
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

// A specification that declares no spread of its own is not an empty one. Its
// definition writes the lab's own settings, so a reader of it sees the whole of
// what decides where its Workload's Pods sit and when they leave a Node.
func TestASpecificationDeclaringNoSpreadOfItsOwn_ShouldStillNameTheSettingsTheLabAdds(t *testing.T) {
	declared := defaultPlacement().declared

	expect.Expect(t, declared.TopologySpreads()).To(expect.HaveLen[scenario.TopologySpread](0))
	expect.Expect(t, declared.Tolerations()).
		To(expect.Equal([]scenario.Toleration{theNotReadyToleration(), theUnreachableToleration()}))
}

// A definition is built per call and handed back by copy, so two Scenarios
// declaring one specification declare the same settings and neither can change
// what the other gets.
func TestOnePredefinedSpecificationAskedForTwice_ShouldGiveTwoEqualSpecifications(t *testing.T) {
	expect.Expect(t, resilientPlacement().Equal(resilientPlacement())).To(expect.Equal(true))
	expect.Expect(t, resilientPlacement().Equal(defaultPlacement())).To(expect.Equal(false))
}

// A Customization sets the field it names and leaves every other setting of the
// specification standing, the lab's own included: it replaces the value a
// definition set, adds the setting a definition holds none of, and where two name
// one field the later one is what holds.
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

// A specification holds one Request, so a Customization written after a Request
// states another one and the later of the two is the one that stands - the rule
// every setting of one field follows.
func TestARequestWrittenAfterAnother_ShouldStandAsTheLaterSetting(t *testing.T) {
	declaring := defaultPlacement()

	customized := declaring.Customize(
		aRequestOf(oneCorePerPod, twoGibibytesPerPod), aRequestOf(twoCoresPerPod, twoGibibytesPerPod))

	expect.Expect(t, customized.declared.Settings()).To(expect.Equal([]scenario.Setting{
		theNotReadyToleration(), theUnreachableToleration(), theRequestOf(twoCoresPerPod, twoGibibytesPerPod),
	}))
}

// A full access takes the place it was written in: the number of settings the
// specification holds when it is written, which is how many the mapper
// transcribes before running it. A setting of the lab's own written after a full
// access keeps the place its field was first written, so that full access stands
// where both name one Kubernetes field, per
// docs/adr/2026-10-05-workload-declares-a-predefined-pod-specification.md.
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

// Two specifications are equal where they name the same settings under the same
// name and hold as many full accesses in the same places.
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

// Customizing hands back a copy, so the predefined specification one Workload
// customized is the one a second Workload declaring it gets.
func TestAPredefinedSpecificationCustomizedByOneWorkload_ShouldStayAsItWasForAnother(t *testing.T) {
	declaredByTheFirstWorkload := resilientPlacement()

	customized := declaredByTheFirstWorkload.Customize(aZoneSpreadAt(aWiderSkew))

	expect.Expect(t, declaredByTheFirstWorkload.Equal(resilientPlacement())).To(expect.Equal(true))
	expect.Expect(t, customized.Equal(resilientPlacement())).To(expect.Equal(false))
}

// The Startup times a Customization below states, in the whole seconds and the
// whole milliseconds a Scenario writes them in: a shortest and a longest apart
// from each other and from the definitions' own, so the time a specification holds is
// read back as the Scenario's and the later of two is told from the earlier. The
// millisecond ones are no whole second, which no seconds form could state.
const (
	aShortStartup        = 3
	aLongerStartup       = 7
	aShortStartupMillis  = 300
	aLongerStartupMillis = 1700
)

// theStartupTimeBetween is a Startup time as the domain holds it, which is what
// a customized specification reads back as.
func theStartupTimeBetween(t *testing.T, shortest, longest time.Duration) scenario.StartupTime {
	t.Helper()

	stated, err := scenario.NewStartupTime(int(shortest.Milliseconds()), int(longest.Milliseconds()))
	if err != nil {
		t.Fatalf("build a Startup time between %s and %s: %v", shortest, longest, err)
	}

	return stated
}

// A Scenario states a Startup time as one time every Pod takes, or as a shortest
// and a longest each Pod takes its own time between (docs/prd.md REQ-SIM-012).
func TestAStartupTimeAScenarioStates_ShouldBeTheOneItsWorkloadsPodsTake(t *testing.T) {
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

// A Scenario states a Startup time under a second, or one that is no whole
// second, in whole milliseconds (docs/prd.md REQ-SIM-012).
func TestAStartupTimeInMilliseconds_ShouldBeTheOneItsWorkloadsPodsTake(t *testing.T) {
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

// Every form states the one Startup time, so of two the later stands whichever
// unit each was written in.
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

func TestAStartupTimeCustomizedForOneWorkload_ShouldLeaveTheSpecificationItCustomizedUnchanged(t *testing.T) {
	declaredByTheFirstWorkload := resilientPlacement()

	customized := declaredByTheFirstWorkload.Customize(StartupSeconds(aLongerStartup))

	expect.Expect(t, declaredByTheFirstWorkload.Equal(resilientPlacement())).To(expect.Equal(true))
	expect.Expect(t, customized.Equal(resilientPlacement())).To(expect.Equal(false))
	expect.Expect(t, declaredByTheFirstWorkload.declared.StartupTime()).To(expect.Equal(resilientPlacement().declared.StartupTime()))
}
