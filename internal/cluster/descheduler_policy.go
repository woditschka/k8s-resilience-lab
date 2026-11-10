package cluster

import (
	"encoding/json"
	"fmt"

	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// These types mirror the Descheduler's v1alpha2 policy rather than importing
// it, so the module graph carries none of the Descheduler's k8s.io versions.
// Every optional key is omitted when empty, so the document holds what the
// policy states and nothing the lab adds.
type deschedulerPolicyDocument struct {
	APIVersion                string               `json:"apiVersion"`
	Kind                      string               `json:"kind"`
	MaxNoOfPodsToEvictPerNode uint                 `json:"maxNoOfPodsToEvictPerNode,omitempty"`
	MaxNoOfPodsToEvictTotal   uint                 `json:"maxNoOfPodsToEvictTotal,omitempty"`
	Profiles                  []deschedulerProfile `json:"profiles"`
}

type deschedulerProfile struct {
	Name         string                    `json:"name"`
	PluginConfig []deschedulerPluginConfig `json:"pluginConfig,omitempty"`
	// The extension points are keyed by the domain's spelling of them, which is
	// the policy file's own.
	Plugins map[scenario.DeschedulerExtensionPoint]deschedulerPluginSet `json:"plugins"`
}

type deschedulerPluginConfig struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

type deschedulerPluginSet struct {
	Enabled []scenario.DeschedulerPluginName `json:"enabled"`
}

const (
	deschedulerPolicyAPIVersion = "descheduler/v1alpha2"
	deschedulerPolicyKind       = "DeschedulerPolicy"
)

// toDeschedulerPolicy transcribes the policy: each profile, configuration entry
// and enabled plugin as stated, and nothing the policy leaves out.
func toDeschedulerPolicy(policy scenario.DeschedulerPolicy) ([]byte, error) {
	profiles := make([]deschedulerProfile, 0, len(policy.Profiles()))
	for _, profile := range policy.Profiles() {
		profiles = append(profiles, toDeschedulerProfile(profile))
	}

	encoded, err := json.Marshal(deschedulerPolicyDocument{
		APIVersion: deschedulerPolicyAPIVersion, Kind: deschedulerPolicyKind,
		MaxNoOfPodsToEvictPerNode: policy.MaxPodsToEvictPerNode(), MaxNoOfPodsToEvictTotal: policy.MaxPodsToEvictTotal(),
		Profiles: profiles,
	})
	if err != nil {
		return nil, fmt.Errorf("encode the Descheduler policy %s: %w", policy.Name(), err)
	}

	return encoded, nil
}

func toDeschedulerProfile(profile scenario.DeschedulerProfile) deschedulerProfile {
	transcribed := deschedulerProfile{
		Name:    profile.Name(),
		Plugins: make(map[scenario.DeschedulerExtensionPoint]deschedulerPluginSet, len(profile.Enabled())),
	}
	for _, entry := range profile.PluginConfig() {
		transcribed.PluginConfig = append(transcribed.PluginConfig, deschedulerPluginConfig{
			Name: string(entry.Name()), Args: argumentsByName(entry.Arguments()),
		})
	}
	for _, set := range profile.Enabled() {
		transcribed.Plugins[set.Point()] = deschedulerPluginSet{Enabled: set.Plugins()}
	}

	return transcribed
}

// An entry stating no argument has no args, so the Descheduler's own defaults
// apply to it.
func argumentsByName(arguments []scenario.DeschedulerArgument) map[string]any {
	if len(arguments) == 0 {
		return nil
	}

	byName := make(map[string]any, len(arguments))
	for _, argument := range arguments {
		byName[argument.Name()] = argument.Value()
	}

	return byName
}
