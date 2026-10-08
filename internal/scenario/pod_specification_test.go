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
	// someSpecificationName is the name a specification carries into the Step that
	// declares it, which no test in this file turns on.
	someSpecificationName = "a Pod specification"

	// anotherSpecificationName is a second such name, so two specifications of the
	// same settings can differ by name alone.
	anotherSpecificationName = "another Pod specification"

	// Two Skews apart, and two waiting times apart in the seconds a toleration
	// holds. No test here turns on either value: what they are for is that a
	// setting written twice is read back at the one written last, so the lab's own
	// Skew and waiting time are deliberately not the values used.
	someSkew       = 3
	someWiderSkew  = 4
	someWait       = 10
	someLongerWait = 30
)

// aZoneSpread and aNodeSpread are the two spread settings, at the maximum Skew
// named: the Zones of a Topology are counted over its Zone label and its Nodes
// over its hostname label.
func aZoneSpread(maxSkew int) scenario.TopologySpread {
	return scenario.NewTopologySpread(scenario.ZoneLabel, maxSkew, scenario.DoNotSchedule, scenario.Honor)
}

func aNodeSpread(maxSkew int) scenario.TopologySpread {
	return scenario.NewTopologySpread(scenario.HostnameLabel, maxSkew, scenario.DoNotSchedule, scenario.Honor)
}

// aNotReadyToleration is the toleration of the taint a Node that stopped
// responding carries, for the seconds named.
func aNotReadyToleration(leavingAfter int64) scenario.Toleration {
	return scenario.NewToleration(scenario.TaintNodeNotReady, scenario.Exists, scenario.NoExecute, leavingAfter)
}

// aPodSpecification is the named default for a specification whose name and
// settings no test turns on; it holds one setting, because a specification
// holding none is refused where it is declared.
func aPodSpecification() scenario.PodSpecification {
	return scenario.NewPodSpecification(someSpecificationName, aZoneSpread(someSkew))
}

func TestAPodSpecification_ShouldHoldEverySettingItWasDefinedWithInDefinitionOrder(t *testing.T) {
	tolerated, zoneSpread := aNotReadyToleration(someWait), aZoneSpread(someSkew)

	declared := scenario.NewPodSpecification(someSpecificationName, tolerated, zoneSpread)

	expect.Expect(t, declared.Name()).To(expect.Equal(someSpecificationName))
	expect.Expect(t, declared.Settings()).To(expect.Equal([]scenario.Setting{tolerated, zoneSpread}))
	expect.Expect(t, declared.TopologySpreads()).To(expect.Equal([]scenario.TopologySpread{zoneSpread}))
	expect.Expect(t, declared.Tolerations()).To(expect.Equal([]scenario.Toleration{tolerated}))
}

func TestASettingWrittenAgain_ShouldBeKeptAtTheValueWrittenLastInThePlaceItWasFirstWritten(t *testing.T) {
	tolerated := aNotReadyToleration(someWait)
	theZoneSpread, theZoneSpreadWrittenAgain := aZoneSpread(someSkew), aZoneSpread(someWiderSkew)

	written := map[string]scenario.PodSpecification{
		"written twice in one definition": scenario.NewPodSpecification(
			someSpecificationName, theZoneSpread, tolerated, theZoneSpreadWrittenAgain),
		"written again by a customization": scenario.NewPodSpecification(
			someSpecificationName, theZoneSpread, tolerated).With(theZoneSpreadWrittenAgain),
	}

	for how, declared := range written {
		t.Run(how, func(t *testing.T) {
			expect.Expect(t, declared.Settings()).To(expect.Equal([]scenario.Setting{theZoneSpreadWrittenAgain, tolerated}))
		})
	}
}

func TestACustomizedSpecification_ShouldLeaveTheOneItWasCustomizedFromAsItWas(t *testing.T) {
	theZoneSpread := aZoneSpread(someSkew)
	defined := scenario.NewPodSpecification(someSpecificationName, theZoneSpread)

	customized := defined.With(aNodeSpread(someSkew))

	expect.Expect(t, defined.Settings()).To(expect.Equal([]scenario.Setting{theZoneSpread}))
	expect.Expect(t, customized.Settings()).
		To(expect.Equal([]scenario.Setting{theZoneSpread, aNodeSpread(someSkew)}))
}

func TestWhenTheReturnedSettingsAreChanged_ShouldLeaveTheSpecificationUntouched(t *testing.T) {
	declared := aPodSpecification()

	declared.Settings()[0] = aNodeSpread(someWiderSkew)

	expect.Expect(t, declared.Settings()).To(expect.Equal([]scenario.Setting{aZoneSpread(someSkew)}))
}

func TestTwoPodSpecifications_ShouldBeEqualWhenTheyNameTheSameSettingsUnderTheSameName(t *testing.T) {
	compared := map[string]struct {
		one, other scenario.PodSpecification
		equal      bool
	}{
		"the same settings under the same name": {
			one:   scenario.NewPodSpecification(someSpecificationName, aZoneSpread(someSkew)),
			other: scenario.NewPodSpecification(someSpecificationName, aZoneSpread(someSkew)),
			equal: true,
		},
		"the same settings under another name": {
			one:   scenario.NewPodSpecification(someSpecificationName, aZoneSpread(someSkew)),
			other: scenario.NewPodSpecification(anotherSpecificationName, aZoneSpread(someSkew)),
			equal: false,
		},
		"one setting at another value": {
			one:   scenario.NewPodSpecification(someSpecificationName, aZoneSpread(someSkew)),
			other: scenario.NewPodSpecification(someSpecificationName, aZoneSpread(someWiderSkew)),
			equal: false,
		},
		"one setting of another field": {
			one:   scenario.NewPodSpecification(someSpecificationName, aZoneSpread(someSkew)),
			other: scenario.NewPodSpecification(someSpecificationName, aNodeSpread(someSkew)),
			equal: false,
		},
		"one toleration at another waiting time": {
			one:   scenario.NewPodSpecification(someSpecificationName, aNotReadyToleration(someWait)),
			other: scenario.NewPodSpecification(someSpecificationName, aNotReadyToleration(someLongerWait)),
			equal: false,
		},
	}

	for how, taken := range compared {
		t.Run(how, func(t *testing.T) {
			expect.Expect(t, taken.one.Equal(taken.other)).To(expect.Equal(taken.equal))
		})
	}
}

// The Startup times the tests below state, in whole seconds: a shortest and a
// longest apart from each other and from the lab's own, so a time read back is
// the one written; and the times a Pod cannot take to start.
const (
	aShortStartup    = 3
	aLongerStartup   = 7
	noStartupAtAll   = 0
	aNegativeStartup = -1
)

// aStartupTimeBetween is the Startup time of that shortest and that longest.
func aStartupTimeBetween(t *testing.T, shortest, longest int) scenario.StartupTime {
	t.Helper()

	stated, err := scenario.NewStartupTime(shortest, longest)
	if err != nil {
		t.Fatalf("build a Startup time between %d and %d seconds: %v", shortest, longest, err)
	}

	return stated
}

func TestAStartupTime_ShouldTakeEachPodBetweenItsShortestAndItsLongest(t *testing.T) {
	stated := map[string]struct {
		shortest, longest         int
		wantShortest, wantLongest time.Duration
	}{
		"a shortest and a longest": {
			shortest: aShortStartup, longest: aLongerStartup,
			wantShortest: aShortStartup * time.Second, wantLongest: aLongerStartup * time.Second,
		},
		"equal bounds, acting as one time": {
			shortest: aShortStartup, longest: aShortStartup,
			wantShortest: aShortStartup * time.Second, wantLongest: aShortStartup * time.Second,
		},
		"no time at all, Ready at placement": {
			shortest: noStartupAtAll, longest: noStartupAtAll,
			wantShortest: noStartupAtAll * time.Second, wantLongest: noStartupAtAll * time.Second,
		},
	}

	for how, startup := range stated {
		t.Run(how, func(t *testing.T) {
			taken := aStartupTimeBetween(t, startup.shortest, startup.longest)

			expect.Expect(t, taken.Shortest()).To(expect.Equal(startup.wantShortest))
			expect.Expect(t, taken.Longest()).To(expect.Equal(startup.wantLongest))
		})
	}
}

func TestAStartupTimeThatCannotHold_ShouldBeRefusedWhereItIsStated(t *testing.T) {
	refusals := map[string]struct {
		shortest, longest int
		named             string
	}{
		"a shortest above its longest": {shortest: aLongerStartup, longest: aShortStartup, named: strconv.Itoa(aLongerStartup)},
		"a negative shortest":          {shortest: aNegativeStartup, longest: aShortStartup, named: strconv.Itoa(aNegativeStartup)},
		"a negative time":              {shortest: aNegativeStartup, longest: aNegativeStartup, named: strconv.Itoa(aNegativeStartup)},
	}

	for refused, startup := range refusals {
		t.Run(refused, func(t *testing.T) {
			_, err := scenario.NewStartupTime(startup.shortest, startup.longest)

			if err == nil {
				t.Fatalf("a Startup time between %d and %d seconds is refused, but it was admitted", startup.shortest, startup.longest)
			}
			if !strings.Contains(err.Error(), startup.named) {
				t.Errorf("the refusal names %s, but reads: %v", startup.named, err)
			}
		})
	}
}

func TestAPodSpecificationStatingNoStartupTime_ShouldHaveItsPodsTakeTheLabsOwnStartupTime(t *testing.T) {
	declared := aPodSpecification()

	expect.Expect(t, declared.StartupTime().Shortest()).To(expect.Equal(scenario.DefaultStartupTime))
	expect.Expect(t, declared.StartupTime().Longest()).To(expect.Equal(scenario.DefaultStartupTime))
}

func TestAStartupTimeStatedOnASpecification_ShouldStandWhereverItsSettingsAreCustomizedAfter(t *testing.T) {
	stated := aStartupTimeBetween(t, aShortStartup, aLongerStartup)

	customized := aPodSpecification().WithStartupTime(stated).With(aNodeSpread(someSkew))

	expect.Expect(t, customized.StartupTime()).To(expect.Equal(stated))
}

func TestTwoStartupTimesStatedOnOneSpecification_ShouldLeaveTheLaterOneStanding(t *testing.T) {
	theEarlier, theLater := aStartupTimeBetween(t, aShortStartup, aShortStartup), aStartupTimeBetween(t, aLongerStartup, aLongerStartup)

	customized := aPodSpecification().WithStartupTime(theEarlier).WithStartupTime(theLater)

	expect.Expect(t, customized.StartupTime()).To(expect.Equal(theLater))
}

func TestAStartupTimeStatedOnASpecification_ShouldLeaveTheOneItWasStatedOnAsItWas(t *testing.T) {
	defined := aPodSpecification()

	customized := defined.WithStartupTime(aStartupTimeBetween(t, aShortStartup, aLongerStartup))

	expect.Expect(t, defined.Equal(aPodSpecification())).To(expect.Equal(true))
	expect.Expect(t, customized.Equal(defined)).To(expect.Equal(false))
}
