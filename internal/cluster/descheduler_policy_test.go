//go:build !integration

package cluster

import (
	"encoding/json"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// The document is decoded generically and compared whole, so a key the lab
// added, an extension point, a configuration entry or a default, fails it by
// being there at all.
func TestADeschedulerPolicysEncoding_ShouldHoldEveryPluginAndArgumentItStatesAndNothingTheLabAdds(t *testing.T) {
	policy := aDeschedulerPolicyOf(t, oneEvictionPerNode,
		aDeschedulerProfileOf(t, someProfileName,
			[]scenario.DeschedulerPluginConfig{
				scenario.NewDeschedulerPluginConfig(scenario.DefaultEvictor, scenario.NewDeschedulerArgument(theNodeFitArgument, true)),
				scenario.NewDeschedulerPluginConfig(scenario.RemovePodsViolatingTopologySpreadConstraint,
					scenario.NewDeschedulerArgument(theConstraintsArgument, []string{aConstraintAction})),
			},
			scenario.NewDeschedulerPluginSet(scenario.BalanceExtensionPoint, scenario.RemovePodsViolatingTopologySpreadConstraint),
		),
		aDeschedulerProfileOf(t, anotherProfileName,
			[]scenario.DeschedulerPluginConfig{scenario.NewDeschedulerPluginConfig(scenario.RemoveDuplicates)},
			scenario.NewDeschedulerPluginSet(scenario.DescheduleExtensionPoint, scenario.RemovePodsViolatingInterPodAntiAffinity),
			scenario.NewDeschedulerPluginSet(scenario.BalanceExtensionPoint, scenario.RemoveDuplicates),
		),
	).WithMaxPodsToEvictTotal(oneEvictionPerPass)

	encoded, err := toDeschedulerPolicy(policy)

	if err != nil {
		t.Fatalf("encode the Descheduler policy %q: %v", policy.Name(), err)
	}
	expect.Expect(t, decoded(t, encoded)).To(expect.Equal(map[string]any{
		"apiVersion":                "descheduler/v1alpha2",
		"kind":                      "DeschedulerPolicy",
		"maxNoOfPodsToEvictPerNode": float64(oneEvictionPerNode),
		"maxNoOfPodsToEvictTotal":   float64(oneEvictionPerPass),
		"profiles": []any{
			map[string]any{
				"name": someProfileName,
				"pluginConfig": []any{
					map[string]any{"name": string(scenario.DefaultEvictor), "args": map[string]any{theNodeFitArgument: true}},
					map[string]any{
						"name": string(scenario.RemovePodsViolatingTopologySpreadConstraint),
						"args": map[string]any{theConstraintsArgument: []any{aConstraintAction}},
					},
				},
				"plugins": map[string]any{
					"balance": map[string]any{"enabled": []any{string(scenario.RemovePodsViolatingTopologySpreadConstraint)}},
				},
			},
			map[string]any{
				"name":         anotherProfileName,
				"pluginConfig": []any{map[string]any{"name": string(scenario.RemoveDuplicates)}},
				"plugins": map[string]any{
					"deschedule": map[string]any{"enabled": []any{string(scenario.RemovePodsViolatingInterPodAntiAffinity)}},
					"balance":    map[string]any{"enabled": []any{string(scenario.RemoveDuplicates)}},
				},
			},
		},
	}))
}

func TestADeschedulerPolicyLeavingItsLimitsAndEntriesOut_ShouldLeaveTheirKeysOutOfItsEncoding(t *testing.T) {
	policy := aDeschedulerPolicyOf(t, anyLimitPerNode, aDeschedulerProfileOf(t, someProfileName, nil,
		scenario.NewDeschedulerPluginSet(scenario.BalanceExtensionPoint, scenario.RemoveDuplicates)))

	encoded, err := toDeschedulerPolicy(policy)

	if err != nil {
		t.Fatalf("encode the Descheduler policy %q: %v", policy.Name(), err)
	}
	expect.Expect(t, decoded(t, encoded)).To(expect.Equal(map[string]any{
		"apiVersion": "descheduler/v1alpha2",
		"kind":       "DeschedulerPolicy",
		"profiles": []any{map[string]any{
			"name":    someProfileName,
			"plugins": map[string]any{"balance": map[string]any{"enabled": []any{string(scenario.RemoveDuplicates)}}},
		}},
	}))
}

func decoded(t *testing.T, encoded []byte) map[string]any {
	t.Helper()

	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("decode the encoded Descheduler policy %s: %v", encoded, err)
	}

	return document
}
