// The Kubernetes names internal/lab offers a Scenario: the labels a spread
// counts its domains over, the taints a Node that stopped responding carries,
// and the values a setting takes. internal/scenario holds the one spelling of
// each, and the mapping test pins it to the client library's own constant.
//
// They sit together in this one file because a Scenario reads as one vocabulary
// under one prefix, and this is the curated list of what it may name — one line
// per name. It is not a screen in front of the machinery: internal/scenario and
// internal/cluster are there to be read, and this file only keeps a Scenario's
// own language small (docs/system-design.md § Package Structure).
package lab

import "github.com/woditschka/k8s-resilience-lab/internal/scenario"

const (
	// ZoneLabel is the label a Zone is a failure domain under, and HostnameLabel
	// the one a Node is a single host under.
	ZoneLabel     = scenario.ZoneLabel
	HostnameLabel = scenario.HostnameLabel

	// NodeNotReady and NodeUnreachable are the two taints the control plane adds
	// to a Node it has stopped hearing from.
	NodeNotReady    = scenario.TaintNodeNotReady
	NodeUnreachable = scenario.TaintNodeUnreachable

	// DoNotSchedule leaves a Pod no Node can take without widening a spread
	// Pending, and Honor counts only the Nodes whose taints a Workload's Pods
	// tolerate.
	DoNotSchedule = scenario.DoNotSchedule
	Honor         = scenario.Honor

	// Exists matches a taint of that key whatever its value, and NoExecute is the
	// effect a Pod is moved off a Node by.
	Exists    = scenario.Exists
	NoExecute = scenario.NoExecute
)
