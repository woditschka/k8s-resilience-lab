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

// Kubernetes' own field names let a Scenario's literal read as the manifest it
// becomes.
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

// Kubernetes has no type for one Pod's requests.
func TestARequestsFields_ShouldBeTheResourceNamesAPodAsksForUnder(t *testing.T) {
	expect.Expect(t, lowercaseFieldsOf(reflect.TypeFor[pod.Request]())).
		To(expect.Equal([]string{string(corev1.ResourceCPU), string(corev1.ResourceMemory)}))
}

// internal/scenario's mapping test pins each spelling to the client library's
// constant. pod.Gi is the unit node.Gi spells, so a request and a capacity are
// written in one unit.
func TestThePodValues_ShouldBeTheDomainsOwnSpelling(t *testing.T) {
	expect.Expect(t, []string{string(pod.DoNotSchedule), string(pod.Honor), string(pod.Exists), string(pod.Equal), string(pod.NoExecute), string(pod.NoSchedule)}).
		To(expect.Equal([]string{
			string(scenario.DoNotSchedule), string(scenario.Honor), string(scenario.Exists), string(scenario.Equal), string(scenario.NoExecute), string(scenario.NoSchedule),
		}))
	expect.Expect(t, pod.Gi).To(expect.Equal(node.Gi))
}

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
