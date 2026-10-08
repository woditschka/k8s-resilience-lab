// The tests in this file read the lab's predefined Pod specifications the way an
// engineer opening one does: as the settings each sets and the value each sets
// them to. They reach no cluster, because a definition is a value.
//
// The tests of full access below write it the way a Scenario writes it, which is
// why this file names the one Kubernetes package such a Scenario imports.
// internal/lab's own sources name none: the Step surface carries a full access as
// the function type internal/cluster declares for it.
package lab

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/woditschka/k8s-resilience-lab/internal/cluster"
	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// The names the two predefined specifications of this slice carry, which is what
// the Step declaring one reports.
const (
	defaultPlacementNamed   = "Default placement"
	resilientPlacementNamed = "Resilient placement"
)

// The two Skews above the even one a Customization below is written at, so what
// a customized specification holds is read back as the Scenario's value and not
// as the definition's, and the later of two Customizations is told from the
// earlier.
const (
	aWiderSkew      = 2
	anEvenWiderSkew = 3
)

// theNotReadyToleration and theUnreachableToleration are the lab's own settings,
// which the base every specification starts from carries: a Pod leaves a Node
// that stopped responding after the lab's own waiting time, for each of the two
// taints the control plane adds to such a Node.
func theNotReadyToleration() scenario.Toleration {
	return scenario.NewToleration(
		scenario.TaintNodeNotReady, scenario.Exists, scenario.NoExecute, theLabsWaitInSeconds())
}

func theUnreachableToleration() scenario.Toleration {
	return scenario.NewToleration(
		scenario.TaintNodeUnreachable, scenario.Exists, scenario.NoExecute, theLabsWaitInSeconds())
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
	return scenario.NewTopologySpread(scenario.ZoneLabel, evenSkew, scenario.DoNotSchedule, scenario.Honor)
}

func theEvenNodeSpread() scenario.TopologySpread {
	return scenario.NewTopologySpread(scenario.HostnameLabel, evenSkew, scenario.DoNotSchedule, scenario.Honor)
}

// aZoneSpreadAt is a Customization as a Scenario writes one, in the vocabulary
// internal/lab offers it: the Zone spread at the Skew named, which is the one
// setting a Scenario may change. theZoneSpreadAt is that same setting as the
// domain holds it, which is what a customized specification reads back as.
func aZoneSpreadAt(maxSkew int) Customization {
	return TopologySpread(
		TopologyKey(ZoneLabel), MaxSkew(maxSkew), WhenUnsatisfiable(DoNotSchedule), NodeTaintsPolicy(Honor))
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

// aRequestOf is a Request as a Scenario writes one, in the vocabulary
// internal/lab offers it. theRequestOf is that same setting as the domain holds
// it, which is what a customized specification reads back as.
func aRequestOf(cores, gibibytes int) Customization {
	return Request(CPU(cores), MemoryGi(gibibytes))
}

func theRequestOf(cores, gibibytes int) scenario.Request {
	return scenario.NewRequest(CPU(cores), MemoryGi(gibibytes))
}

// somePriorityClass is a Kubernetes Pod setting the lab's own vocabulary does not
// name, which no test of this file turns on: what a specification holds of a full
// access is the place it was written in and not the change itself.
const somePriorityClass = "any-priority-class"

// aFullAccess is full access as a Scenario writes one, in the Scenario's own
// spelling. What the change does is read off the deployed Pod template, in
// internal/cluster/resources_test.go.
func aFullAccess() Customization {
	return FullAccess(func(pod *corev1.PodSpec) { pod.PriorityClassName = somePriorityClass })
}

// The places a full access holds in the tests below, as the number of settings
// transcribed before it runs: after the two settings the base of every
// specification carries, and after one further setting a Scenario wrote itself.
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

func TestAPredefinedPodSpecification_ShouldNameEverySettingItHoldsWithTheValueItSets(t *testing.T) {
	defined := map[string]struct {
		specification PodSpecification
		named         string
		settings      []scenario.Setting
	}{
		"Default placement": {
			specification: DefaultPlacement(),
			named:         defaultPlacementNamed,
			settings:      []scenario.Setting{theNotReadyToleration(), theUnreachableToleration()},
		},
		"Resilient placement": {
			specification: ResilientPlacement(),
			named:         resilientPlacementNamed,
			settings: []scenario.Setting{
				theNotReadyToleration(), theUnreachableToleration(), theEvenZoneSpread(), theEvenNodeSpread(),
			},
		},
	}

	for intent, definition := range defined {
		t.Run(intent, func(t *testing.T) {
			expect.Expect(t, definition.specification.name()).To(expect.Equal(definition.named))
			expect.Expect(t, definition.specification.declared.Settings()).To(expect.Equal(definition.settings))
		})
	}
}

// Edge case 4: a specification that declares no spread of its own is not an empty
// one. The base it starts from carries the lab's own settings, so a reader of the
// definition sees the whole of what decides where its Workload's Pods sit and
// when they leave a Node.
func TestASpecificationDeclaringNoSpreadOfItsOwn_ShouldStillNameTheSettingsTheLabAdds(t *testing.T) {
	declared := DefaultPlacement().declared

	expect.Expect(t, declared.TopologySpreads()).To(expect.HaveLen[scenario.TopologySpread](0))
	expect.Expect(t, declared.Tolerations()).
		To(expect.Equal([]scenario.Toleration{theNotReadyToleration(), theUnreachableToleration()}))
}

// A definition is built per call and handed back by copy, so two Scenarios
// declaring one specification declare the same settings and neither can change
// what the other gets.
func TestOnePredefinedSpecificationAskedForTwice_ShouldGiveTwoEqualSpecifications(t *testing.T) {
	expect.Expect(t, ResilientPlacement().Equal(ResilientPlacement())).To(expect.Equal(true))
	expect.Expect(t, ResilientPlacement().Equal(DefaultPlacement())).To(expect.Equal(false))
}

// A Customization sets the field it names and leaves every other setting of the
// specification standing, the lab's own included: it replaces the value a
// definition set, adds the setting a definition holds none of, and where two name
// one field the later one is what holds.
func TestASpecificationCustomizedByAScenario_ShouldHoldTheCustomizedSettingsAndKeepTheRest(t *testing.T) {
	written := map[string]struct {
		declares       PodSpecification
		customizations []Customization
		held           []scenario.Setting
	}{
		"a setting the specification sets": {
			declares:       ResilientPlacement(),
			customizations: []Customization{aZoneSpreadAt(aWiderSkew)},
			held: []scenario.Setting{
				theNotReadyToleration(), theUnreachableToleration(), theZoneSpreadAt(aWiderSkew), theEvenNodeSpread(),
			},
		},
		"two Customizations naming one setting": {
			declares:       ResilientPlacement(),
			customizations: []Customization{aZoneSpreadAt(aWiderSkew), aZoneSpreadAt(anEvenWiderSkew)},
			held: []scenario.Setting{
				theNotReadyToleration(), theUnreachableToleration(), theZoneSpreadAt(anEvenWiderSkew), theEvenNodeSpread(),
			},
		},
		"a setting the specification does not hold": {
			declares:       DefaultPlacement(),
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
	declaring := DefaultPlacement()

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
		"full access after a setting of the lab's own vocabulary": {
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
			customized := DefaultPlacement().Customize(taken.customizations...)

			expect.Expect(t, customized.declared.Settings()).To(expect.Equal(taken.held))
			expect.Expect(t, placesOfTheFullAccesses(customized.accesses)).To(expect.Equal(taken.places))
		})
	}
}

// Two specifications are equal where they name the same settings under the same
// name and hold as many full accesses in the same places.
func TestTwoSpecificationsHoldingFullAccess_ShouldBeEqualOnlyWhereItStandsInTheSamePlace(t *testing.T) {
	declared := DefaultPlacement()

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
	declaredByTheFirstWorkload := ResilientPlacement()

	customized := declaredByTheFirstWorkload.Customize(aZoneSpreadAt(aWiderSkew))

	expect.Expect(t, declaredByTheFirstWorkload.Equal(ResilientPlacement())).To(expect.Equal(true))
	expect.Expect(t, customized.Equal(ResilientPlacement())).To(expect.Equal(false))
}
