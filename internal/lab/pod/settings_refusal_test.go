// These tests read a setting as a Scenario writes it, a keyed literal: a field
// left out is refused at the specification holding it, and a complete setting
// is reported as a manifest writes it.
package pod

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

const (
	aZoneKey      = scenario.ZoneLabel
	aTolerationOn = scenario.TaintNodeNotReady
)

func aCompleteSpread() TopologySpreadConstraint {
	return TopologySpreadConstraint{TopologyKey: aZoneKey, MaxSkew: anEvenSkew, WhenUnsatisfiable: DoNotSchedule, NodeTaintsPolicy: Honor}
}

func aCompleteToleration() Toleration {
	return Toleration{Key: aTolerationOn, Operator: Exists, Effect: NoExecute, TolerationSeconds: 30}
}

func aCompleteRequest() Request {
	return Request{CPU: 2, Memory: 4 * Gi}
}

func TestASettingWithOneFieldLeftOut_ShouldBeRefusedNamingTheSettingAndTheField(t *testing.T) {
	spread, toleration, request := aCompleteSpread(), aCompleteToleration(), aCompleteRequest()
	tests := map[string]struct {
		setting Setting
		named   string
		field   string
	}{
		"a spread without its topologyKey":        {setting: withSpread(spread, func(s *TopologySpreadConstraint) { s.TopologyKey = "" }), named: "TopologySpreadConstraint", field: "topologyKey"},
		"a spread without its maxSkew":            {setting: withSpread(spread, func(s *TopologySpreadConstraint) { s.MaxSkew = 0 }), named: "TopologySpreadConstraint", field: "maxSkew"},
		"a spread without its whenUnsatisfiable":  {setting: withSpread(spread, func(s *TopologySpreadConstraint) { s.WhenUnsatisfiable = "" }), named: "TopologySpreadConstraint", field: "whenUnsatisfiable"},
		"a spread without its nodeTaintsPolicy":   {setting: withSpread(spread, func(s *TopologySpreadConstraint) { s.NodeTaintsPolicy = "" }), named: "TopologySpreadConstraint", field: "nodeTaintsPolicy"},
		"a toleration without its key":            {setting: withToleration(toleration, func(s *Toleration) { s.Key = "" }), named: "Toleration", field: "key"},
		"a toleration without its operator":       {setting: withToleration(toleration, func(s *Toleration) { s.Operator = "" }), named: "Toleration", field: "operator"},
		"a toleration without its effect":         {setting: withToleration(toleration, func(s *Toleration) { s.Effect = "" }), named: "Toleration", field: "effect"},
		"a toleration without its tolerationSecs": {setting: withToleration(toleration, func(s *Toleration) { s.TolerationSeconds = 0 }), named: "Toleration", field: "tolerationSeconds"},
		"a request without its cpu":               {setting: withRequest(request, func(s *Request) { s.CPU = 0 }), named: "Request", field: "cpu"},
		"a request without its memory":            {setting: withRequest(request, func(s *Request) { s.Memory = 0 }), named: "Request", field: "memory"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Predefined(defaultPlacementNamed, someStartupTime(), test.setting).Declared()

			expect.Expect(t, errors.Is(err, errAFieldLeftOut)).To(expect.Equal(true))
			expect.Expect(t, strings.Contains(err.Error(), fmt.Sprintf("pod.%s leaves out %s:", test.named, test.field))).
				To(expect.Equal(true))
		})
	}
}

func TestASettingWithTwoFieldsLeftOut_ShouldBeRefusedNamingTheFirstInTheOrderWritten(t *testing.T) {
	_, err := Predefined(defaultPlacementNamed, someStartupTime(), Toleration{Key: aTolerationOn, Effect: NoExecute}).Declared()

	expect.Expect(t, strings.Contains(err.Error(), "pod.Toleration leaves out operator:")).To(expect.Equal(true))
}

func TestASettingRefusedBeforeAnother_ShouldKeepTheFirstRefusal(t *testing.T) {
	_, err := Predefined(defaultPlacementNamed, someStartupTime(), Request{CPU: 2}, Toleration{}).Declared()

	expect.Expect(t, strings.Contains(err.Error(), "pod.Request leaves out memory:")).To(expect.Equal(true))
}

func TestASettingComplete_ShouldBeReadAsAManifestWritesIt(t *testing.T) {
	tests := map[string]struct {
		setting Setting
		carries string
	}{
		"a topology spread constraint": {
			setting: aCompleteSpread(),
			carries: "topologySpreadConstraints: {topologyKey: topology.kubernetes.io/zone, maxSkew: 1, whenUnsatisfiable: DoNotSchedule, nodeTaintsPolicy: Honor}",
		},
		"a toleration": {
			setting: aCompleteToleration(),
			carries: fmt.Sprintf("tolerations: {key: %s, operator: Exists, effect: NoExecute, tolerationSeconds: %d}",
				aTolerationOn, theLabsWaitInSeconds()),
		},
		"a request": {
			setting: aCompleteRequest(),
			carries: "resources.requests: {cpu: 2, memory: 4Gi}",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			defined := Predefined(defaultPlacementNamed, someStartupTime(), test.setting)

			_, err := defined.Declared()
			expect.Expect(t, err).To(expect.Equal[error](nil))
			expect.Expect(t, slices.Contains(defined.Carries(), test.carries)).To(expect.Equal(true))
		})
	}
}

func TestASpecificationReadOnAGivenLine_ShouldNameItsStartupTimeUnlessItIsRefusedOrUnbuilt(t *testing.T) {
	tests := map[string]struct {
		specification Specification
		reads         string
	}{
		"one Startup time": {
			specification: Predefined(defaultPlacementNamed, StartupSeconds(2)),
			reads:         "Default placement, each Pod starting in 2s",
		},
		"a range of Startup times": {
			specification: Predefined(defaultPlacementNamed, StartupMillisBetween(500, 1000)),
			reads:         fmt.Sprintf("Default placement, each Pod starting in %s to %s", 500*time.Millisecond, time.Second),
		},
		"a refused definition": {
			specification: Predefined(defaultPlacementNamed, StartupTime{}),
			reads:         "Default placement",
		},
		"the zero specification": {
			specification: Specification{},
			reads:         "",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			expect.Expect(t, test.specification.Reads()).To(expect.Equal(test.reads))
		})
	}
}

func withSpread(base TopologySpreadConstraint, change func(*TopologySpreadConstraint)) TopologySpreadConstraint {
	change(&base)

	return base
}

func withToleration(base Toleration, change func(*Toleration)) Toleration {
	change(&base)

	return base
}

func withRequest(base Request, change func(*Request)) Request {
	change(&base)

	return base
}

func aTolerationOfADedicatedNodePool() Toleration {
	return Toleration{Key: "nvidia.com/gpu", Operator: Equal, Value: "present", Effect: NoSchedule}
}

func TestATolerationsFields_ShouldFollowItsOperatorAndEffect(t *testing.T) {
	dedicated, unresponsive := aTolerationOfADedicatedNodePool(), aCompleteToleration()
	tests := map[string]struct {
		setting Toleration
		refused string
		reason  error
	}{
		"an Equal NoSchedule toleration": {setting: dedicated},
		"an Exists NoExecute toleration": {setting: unresponsive},
		"an Exists NoSchedule toleration": {
			setting: withToleration(dedicated, func(s *Toleration) { s.Operator, s.Value = Exists, "" }),
		},
		"an Equal NoExecute toleration": {
			setting: withToleration(unresponsive, func(s *Toleration) { s.Operator, s.Value = Equal, "present" }),
		},
		"an Equal toleration without its value": {
			setting: withToleration(dedicated, func(s *Toleration) { s.Value = "" }),
			refused: "pod.Toleration leaves out value:", reason: errAFieldLeftOut,
		},
		"an Exists toleration stating a value": {
			setting: withToleration(unresponsive, func(s *Toleration) { s.Value = "present" }),
			refused: "pod.Toleration states value, which the operator Exists takes none of:", reason: errAFieldStatedAgainst,
		},
		"a NoSchedule toleration stating a waiting time": {
			setting: withToleration(dedicated, func(s *Toleration) { s.TolerationSeconds = 30 }),
			refused: "pod.Toleration states tolerationSeconds, which the effect NoSchedule takes none of:", reason: errAFieldStatedAgainst,
		},
		"a toleration of an operator the lab does not offer": {
			setting: withToleration(dedicated, func(s *Toleration) { s.Operator = "In" }),
			refused: "pod.Toleration states operator In,", reason: errAFieldStatedAgainst,
		},
		"a toleration of an effect the lab does not offer": {
			setting: withToleration(dedicated, func(s *Toleration) { s.Effect = "PreferNoSchedule" }),
			refused: "pod.Toleration states effect PreferNoSchedule,", reason: errAFieldStatedAgainst,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Predefined(defaultPlacementNamed, someStartupTime(), test.setting).Declared()

			expect.Expect(t, errors.Is(err, test.reason)).To(expect.Equal(true))
			expect.Expect(t, err == nil || strings.Contains(err.Error(), test.refused)).To(expect.Equal(true))
		})
	}
}

func TestATolerationOfANoScheduleTaint_ShouldBeReadWithItsValueAndNoWaitingTime(t *testing.T) {
	defined := Predefined(defaultPlacementNamed, someStartupTime(), aTolerationOfADedicatedNodePool())

	expect.Expect(t, slices.Contains(defined.Carries(),
		"tolerations: {key: nvidia.com/gpu, operator: Equal, value: present, effect: NoSchedule}")).To(expect.Equal(true))
}
