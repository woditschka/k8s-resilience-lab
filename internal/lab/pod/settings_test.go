package pod_test

import (
	"reflect"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/lab/node"
	"github.com/woditschka/k8s-resilience-lab/internal/lab/pod"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// A setting the lab offers is Kubernetes' own type under its own name, holding
// fields of Kubernetes' names alone, so a Scenario's literal reads as the
// manifest it becomes (docs/adr/2026-10-18-pod-and-node-settings-are-keyed-literals-refused-at-their-given.md).
func TestASettingsFields_ShouldBeFieldsOfTheKubernetesTypeOfItsName(t *testing.T) {
	settings := map[string]struct {
		offered    reflect.Type
		kubernetes reflect.Type
	}{
		"the topology spread constraint": {
			offered:    reflect.TypeFor[pod.TopologySpreadConstraint](),
			kubernetes: reflect.TypeFor[corev1.TopologySpreadConstraint](),
		},
		"the toleration": {
			offered:    reflect.TypeFor[pod.Toleration](),
			kubernetes: reflect.TypeFor[corev1.Toleration](),
		},
	}

	for named, setting := range settings {
		t.Run(named, func(t *testing.T) {
			expect.Expect(t, setting.offered.Name()).To(expect.Equal(setting.kubernetes.Name()))
			expect.Expect(t, fieldsMissingFrom(setting.kubernetes, setting.offered)).To(expect.HaveLen[string](0))
		})
	}
}

// Kubernetes has no type for one Pod's requests, so a Request's fields are the
// resource names a Pod asks for under.
func TestARequestsFields_ShouldBeTheResourceNamesAPodAsksForUnder(t *testing.T) {
	expect.Expect(t, lowercaseFieldsOf(reflect.TypeFor[pod.Request]())).
		To(expect.Equal([]string{string(corev1.ResourceCPU), string(corev1.ResourceMemory)}))
}

// Every value the pod package offers is the domain's own spelling, which
// internal/scenario's mapping test pins to the client library's constant, and
// pod.Gi is the unit node.Gi spells, so a request and a capacity are written in
// one unit.
func TestThePodValues_ShouldBeTheDomainsOwnSpelling(t *testing.T) {
	expect.Expect(t, []string{string(pod.DoNotSchedule), string(pod.Honor), string(pod.Exists), string(pod.NoExecute)}).
		To(expect.Equal([]string{
			string(scenario.DoNotSchedule), string(scenario.Honor), string(scenario.Exists), string(scenario.NoExecute),
		}))
	expect.Expect(t, pod.Gi).To(expect.Equal(node.Gi))
}

// fieldsMissingFrom is every field of offered the Kubernetes type holds no field
// of that name for.
func fieldsMissingFrom(kubernetes, offered reflect.Type) []string {
	var missing []string
	for field := range offered.Fields() {
		if _, held := kubernetes.FieldByName(field.Name); !held {
			missing = append(missing, field.Name)
		}
	}

	return missing
}

func lowercaseFieldsOf(written reflect.Type) []string {
	var fields []string
	for field := range written.Fields() {
		fields = append(fields, strings.ToLower(field.Name))
	}

	return fields
}
