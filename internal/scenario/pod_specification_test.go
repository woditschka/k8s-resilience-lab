package scenario_test

import (
	"slices"
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

	// noName is the name a specification cannot be declared under.
	noName = ""

	// Two Skews apart, and two waiting times apart in the seconds a toleration
	// holds. No test here turns on either value: what they are for is that a
	// setting written twice is read back at the one written last, so the lab's own
	// Skew and waiting time are deliberately not the values used.
	someSkew       = 3
	someWiderSkew  = 4
	someWait       = 10
	someLongerWait = 30

	// someTaint is a taint of no Node condition the lab waits on, so a toleration
	// of it keeps the waiting time written for it.
	someTaint = "example.com/some-taint"
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

// aNotReadyToleration and anUnreachableToleration are the tolerations of the two
// taints a Node that stopped responding carries, for the seconds named, and
// aTolerationOfSomeTaint one of a taint that is neither.
func aNotReadyToleration(leavingAfter int64) scenario.Toleration {
	return scenario.NewToleration(scenario.TaintNodeNotReady, scenario.Exists, "", scenario.NoExecute, leavingAfter)
}

func anUnreachableToleration(leavingAfter int64) scenario.Toleration {
	return scenario.NewToleration(scenario.TaintNodeUnreachable, scenario.Exists, "", scenario.NoExecute, leavingAfter)
}

func aTolerationOfSomeTaint(leavingAfter int64) scenario.Toleration {
	return scenario.NewToleration(someTaint, scenario.Exists, "", scenario.NoExecute, leavingAfter)
}

// theLabsWaitInSeconds is the lab's own waiting time as the field Kubernetes
// holds it in, read from the constant that sets it rather than written out.
func theLabsWaitInSeconds() int64 {
	return int64(scenario.UnresponsiveNodeWait.Seconds())
}

// theLabsTolerations are the two tolerations every specification holds at the
// lab's own waiting time, after the settings written where none names them.
func theLabsTolerations() []scenario.Setting {
	return []scenario.Setting{aNotReadyToleration(theLabsWaitInSeconds()), anUnreachableToleration(theLabsWaitInSeconds())}
}

// aSpecificationOf is a specification of the settings written, under a name and
// a Startup time no test calling it turns on.
func aSpecificationOf(t *testing.T, name string, settings ...scenario.Setting) scenario.PodSpecification {
	t.Helper()

	declared, err := scenario.NewPodSpecification(name, aStartupTimeBetween(t, aShortStartupMillis, aLongerStartupMillis), settings...)
	if err != nil {
		t.Fatalf("build the Pod specification %q: %v", name, err)
	}

	return declared
}

// aPodSpecification is the named default for a specification whose name and
// settings no test turns on.
func aPodSpecification(t *testing.T) scenario.PodSpecification {
	t.Helper()

	return aSpecificationOf(t, someSpecificationName, aZoneSpread(someSkew))
}

func TestAPodSpecification_ShouldHoldEverySettingItWasDefinedWithInDefinitionOrder(t *testing.T) {
	tolerated, zoneSpread := aTolerationOfSomeTaint(someWait), aZoneSpread(someSkew)
	startup := aStartupTimeBetween(t, aShortStartupMillis, aLongerStartupMillis)

	declared, err := scenario.NewPodSpecification(someSpecificationName, startup, tolerated, zoneSpread)

	if err != nil {
		t.Fatalf("a Pod specification under a name is admitted, but it was refused: %v", err)
	}
	expect.Expect(t, declared.Name()).To(expect.Equal(someSpecificationName))
	expect.Expect(t, declared.StartupTime()).To(expect.Equal(startup))
	expect.Expect(t, declared.Settings()).To(expect.Equal(slices.Concat([]scenario.Setting{tolerated, zoneSpread}, theLabsTolerations())))
	expect.Expect(t, declared.TopologySpreads()).To(expect.Equal([]scenario.TopologySpread{zoneSpread}))
}

// A Pod specification is declared by its name, so one under no name is refused
// where it is defined (docs/prd.md REQ-SIM-015).
func TestAPodSpecificationUnderNoName_ShouldBeRefused(t *testing.T) {
	_, err := scenario.NewPodSpecification(noName, aStartupTimeBetween(t, aShortStartupMillis, aLongerStartupMillis), aZoneSpread(someSkew))

	if err == nil {
		t.Fatal("a Pod specification under no name is refused, but it was admitted")
	}
}

// A Scenario states no waiting time of its own (docs/prd.md REQ-SIM-008), so
// every specification tolerates both taints of a Node that stopped responding for
// the lab's waiting time: a time written for either is replaced by it, and one
// not written is added after the settings written.
func TestAPodSpecification_ShouldHoldTheLabsTwoTolerationsAtTheLabsWaitWhateverIsWritten(t *testing.T) {
	zoneSpread := aZoneSpread(someSkew)

	written := map[string]struct {
		declared scenario.PodSpecification
		held     []scenario.Setting
	}{
		"neither written": {
			declared: aSpecificationOf(t, someSpecificationName, zoneSpread),
			held:     slices.Concat([]scenario.Setting{zoneSpread}, theLabsTolerations()),
		},
		"both written at another waiting time": {
			declared: aSpecificationOf(t, someSpecificationName,
				aNotReadyToleration(someWait), zoneSpread, anUnreachableToleration(someLongerWait)),
			held: []scenario.Setting{
				aNotReadyToleration(theLabsWaitInSeconds()), zoneSpread, anUnreachableToleration(theLabsWaitInSeconds()),
			},
		},
		"one written again by a customization": {
			declared: aSpecificationOf(t, someSpecificationName, zoneSpread).With(aNotReadyToleration(someWait)),
			held:     slices.Concat([]scenario.Setting{zoneSpread}, theLabsTolerations()),
		},
		"a toleration of another taint": {
			declared: aSpecificationOf(t, someSpecificationName, aTolerationOfSomeTaint(someWait)),
			held:     slices.Concat([]scenario.Setting{aTolerationOfSomeTaint(someWait)}, theLabsTolerations()),
		},
	}

	for how, taken := range written {
		t.Run(how, func(t *testing.T) {
			expect.Expect(t, taken.declared.Settings()).To(expect.Equal(taken.held))
		})
	}
}

func TestASettingWrittenAgain_ShouldBeKeptAtTheValueWrittenLastInThePlaceItWasFirstWritten(t *testing.T) {
	tolerated := aTolerationOfSomeTaint(someWait)
	theZoneSpread, theZoneSpreadWrittenAgain := aZoneSpread(someSkew), aZoneSpread(someWiderSkew)

	written := map[string]scenario.PodSpecification{
		"written twice in one definition": aSpecificationOf(t,
			someSpecificationName, theZoneSpread, tolerated, theZoneSpreadWrittenAgain),
		"written again by a customization": aSpecificationOf(t,
			someSpecificationName, theZoneSpread, tolerated).With(theZoneSpreadWrittenAgain),
	}

	for how, declared := range written {
		t.Run(how, func(t *testing.T) {
			expect.Expect(t, declared.Settings()).
				To(expect.Equal(slices.Concat([]scenario.Setting{theZoneSpreadWrittenAgain, tolerated}, theLabsTolerations())))
		})
	}
}

func TestACustomizedSpecification_ShouldLeaveTheOneItWasCustomizedFromAsItWas(t *testing.T) {
	theZoneSpread := aZoneSpread(someSkew)
	defined := aSpecificationOf(t, someSpecificationName, theZoneSpread)

	customized := defined.With(aNodeSpread(someSkew))

	expect.Expect(t, defined.Settings()).To(expect.Equal(slices.Concat([]scenario.Setting{theZoneSpread}, theLabsTolerations())))
	expect.Expect(t, customized.Settings()).
		To(expect.Equal(slices.Concat([]scenario.Setting{theZoneSpread}, theLabsTolerations(), []scenario.Setting{aNodeSpread(someSkew)})))
}

func TestWhenTheReturnedSettingsAreChanged_ShouldLeaveTheSpecificationUntouched(t *testing.T) {
	declared := aPodSpecification(t)

	declared.Settings()[0] = aNodeSpread(someWiderSkew)

	expect.Expect(t, declared.Settings()).To(expect.Equal(slices.Concat([]scenario.Setting{aZoneSpread(someSkew)}, theLabsTolerations())))
}

func TestTwoPodSpecifications_ShouldBeEqualWhenTheyNameTheSameSettingsUnderTheSameName(t *testing.T) {
	compared := map[string]struct {
		one, other scenario.PodSpecification
		equal      bool
	}{
		"the same settings under the same name": {
			one:   aSpecificationOf(t, someSpecificationName, aZoneSpread(someSkew)),
			other: aSpecificationOf(t, someSpecificationName, aZoneSpread(someSkew)),
			equal: true,
		},
		"the same settings under another name": {
			one:   aSpecificationOf(t, someSpecificationName, aZoneSpread(someSkew)),
			other: aSpecificationOf(t, anotherSpecificationName, aZoneSpread(someSkew)),
			equal: false,
		},
		"one setting at another value": {
			one:   aSpecificationOf(t, someSpecificationName, aZoneSpread(someSkew)),
			other: aSpecificationOf(t, someSpecificationName, aZoneSpread(someWiderSkew)),
			equal: false,
		},
		"one setting of another field": {
			one:   aSpecificationOf(t, someSpecificationName, aZoneSpread(someSkew)),
			other: aSpecificationOf(t, someSpecificationName, aNodeSpread(someSkew)),
			equal: false,
		},
		"one toleration at another waiting time": {
			one:   aSpecificationOf(t, someSpecificationName, aTolerationOfSomeTaint(someWait)),
			other: aSpecificationOf(t, someSpecificationName, aTolerationOfSomeTaint(someLongerWait)),
			equal: false,
		},
	}

	for how, taken := range compared {
		t.Run(how, func(t *testing.T) {
			expect.Expect(t, taken.one.Equal(taken.other)).To(expect.Equal(taken.equal))
		})
	}
}

// The Startup times the tests below state, in the whole milliseconds a Startup
// time is built from: a shortest under a second and a longest that is no whole
// second, apart from each other, so a time read back is the one written; and the
// times a Pod cannot take to start.
const (
	aShortStartupMillis    = 300
	aLongerStartupMillis   = 1700
	noStartupAtAll         = 0
	aNegativeStartupMillis = -1
)

// aStartupTimeBetween is the Startup time of that shortest and that longest.
func aStartupTimeBetween(t *testing.T, shortestMillis, longestMillis int) scenario.StartupTime {
	t.Helper()

	stated, err := scenario.NewStartupTime(shortestMillis, longestMillis)
	if err != nil {
		t.Fatalf("build a Startup time between %d and %d milliseconds: %v", shortestMillis, longestMillis, err)
	}

	return stated
}

func TestAStartupTime_ShouldTakeEachPodBetweenItsShortestAndItsLongest(t *testing.T) {
	stated := map[string]struct {
		shortestMillis, longestMillis int
		wantShortest, wantLongest     time.Duration
	}{
		"a shortest and a longest": {
			shortestMillis: aShortStartupMillis, longestMillis: aLongerStartupMillis,
			wantShortest: aShortStartupMillis * time.Millisecond, wantLongest: aLongerStartupMillis * time.Millisecond,
		},
		"equal bounds, acting as one time": {
			shortestMillis: aShortStartupMillis, longestMillis: aShortStartupMillis,
			wantShortest: aShortStartupMillis * time.Millisecond, wantLongest: aShortStartupMillis * time.Millisecond,
		},
		"no time at all, Ready at placement": {
			shortestMillis: noStartupAtAll, longestMillis: noStartupAtAll,
			wantShortest: noStartupAtAll, wantLongest: noStartupAtAll,
		},
	}

	for how, startup := range stated {
		t.Run(how, func(t *testing.T) {
			taken := aStartupTimeBetween(t, startup.shortestMillis, startup.longestMillis)

			expect.Expect(t, taken.Shortest()).To(expect.Equal(startup.wantShortest))
			expect.Expect(t, taken.Longest()).To(expect.Equal(startup.wantLongest))
		})
	}
}

// A refusal names the bounds as durations rather than in a unit word, because a
// Scenario reaches it from whole seconds and from whole milliseconds alike.
func TestAStartupTimeThatCannotHold_ShouldBeRefusedWhereItIsStated(t *testing.T) {
	refusals := map[string]struct {
		shortestMillis, longestMillis int
		named                         time.Duration
	}{
		"a shortest above its longest": {
			shortestMillis: aLongerStartupMillis, longestMillis: aShortStartupMillis, named: aLongerStartupMillis * time.Millisecond,
		},
		"a negative shortest": {
			shortestMillis: aNegativeStartupMillis, longestMillis: aShortStartupMillis, named: aNegativeStartupMillis * time.Millisecond,
		},
		"a negative time": {
			shortestMillis: aNegativeStartupMillis, longestMillis: aNegativeStartupMillis, named: aNegativeStartupMillis * time.Millisecond,
		},
	}

	for refused, startup := range refusals {
		t.Run(refused, func(t *testing.T) {
			_, err := scenario.NewStartupTime(startup.shortestMillis, startup.longestMillis)

			if err == nil {
				t.Fatalf("a Startup time between %d and %d milliseconds is refused, but it was admitted", startup.shortestMillis, startup.longestMillis)
			}
			if !strings.Contains(err.Error(), startup.named.String()) {
				t.Errorf("the refusal names %s, but reads: %v", startup.named, err)
			}
		})
	}
}

func TestAStartupTimeStatedOnASpecification_ShouldStandWhereverItsSettingsAreCustomizedAfter(t *testing.T) {
	stated := aStartupTimeBetween(t, aShortStartupMillis, aLongerStartupMillis)

	customized := aPodSpecification(t).WithStartupTime(stated).With(aNodeSpread(someSkew))

	expect.Expect(t, customized.StartupTime()).To(expect.Equal(stated))
}

func TestTwoStartupTimesStatedOnOneSpecification_ShouldLeaveTheLaterOneStanding(t *testing.T) {
	theEarlier, theLater := aStartupTimeBetween(t, aShortStartupMillis, aShortStartupMillis), aStartupTimeBetween(t, aLongerStartupMillis, aLongerStartupMillis)

	customized := aPodSpecification(t).WithStartupTime(theEarlier).WithStartupTime(theLater)

	expect.Expect(t, customized.StartupTime()).To(expect.Equal(theLater))
}

func TestAStartupTimeStatedOnASpecification_ShouldLeaveTheOneItWasStatedOnAsItWas(t *testing.T) {
	defined := aPodSpecification(t)

	customized := defined.WithStartupTime(aStartupTimeBetween(t, aLongerStartupMillis, aLongerStartupMillis))

	expect.Expect(t, defined.Equal(aPodSpecification(t))).To(expect.Equal(true))
	expect.Expect(t, customized.Equal(defined)).To(expect.Equal(false))
}
