package scenario

import (
	"errors"
	"fmt"
	"slices"
)

// SpreadDomain is what a spread rule spreads a Workload's Pods over.
type SpreadDomain string

const (
	// SpreadOverZones spreads a Workload's Pods over the Zones of its Topology.
	SpreadOverZones SpreadDomain = "Zones"
	// SpreadOverNodes spreads a Workload's Pods over the Nodes of its Topology.
	SpreadOverNodes SpreadDomain = "Nodes"
)

// spreadDomains are the domains a rule may name, which is what makes an unknown
// domain refusable at construction.
var spreadDomains = []SpreadDomain{SpreadOverZones, SpreadOverNodes}

// SpreadRule is one rule a Workload declares about where its Pods may land: how
// evenly they sit over one domain, stated as the Skew the Workload tolerates.
//
// A Workload declares its own spread rather than inheriting the lab's, per
// docs/adr/2026-10-04-scenario-declared-pod-spread.md.
type SpreadRule struct {
	domain  SpreadDomain
	maxSkew int
}

// NewSpreadRule builds one spread rule, refusing a declaration that would
// constrain nothing: an unknown domain, or a Skew no Placement could exceed.
func NewSpreadRule(domain SpreadDomain, maxSkew int) (SpreadRule, error) {
	if !slices.Contains(spreadDomains, domain) {
		return SpreadRule{}, fmt.Errorf("a spread rule names %v, so the domain %q is refused", spreadDomains, domain)
	}
	if maxSkew < 1 {
		return SpreadRule{}, fmt.Errorf("a spread rule tolerates a Skew of at least one, so a maximum Skew of %d over the %s is refused", maxSkew, domain)
	}

	return SpreadRule{domain: domain, maxSkew: maxSkew}, nil
}

// Domain is what this rule spreads a Workload's Pods over.
func (r SpreadRule) Domain() SpreadDomain {
	return r.domain
}

// MaxSkew is the widest gap between the fullest and the emptiest domain this
// rule tolerates.
func (r SpreadRule) MaxSkew() int {
	return r.maxSkew
}

// Equal reports whether two rules constrain the same domain to the same Skew. It
// is the equality go-cmp compares through, since the type's fields are unexported.
func (r SpreadRule) Equal(other SpreadRule) bool {
	return r.domain == other.domain && r.maxSkew == other.maxSkew
}

// Workload is one declared group of Pod replicas, deployed together with the
// rules that constrain where its Pods may land.
type Workload struct {
	name  string
	pods  int
	rules []SpreadRule
}

// NewWorkload builds the Workload a Scenario deploys, refusing a declaration the
// lab could not deploy and a domain constrained twice.
func NewWorkload(name string, pods int, rules ...SpreadRule) (Workload, error) {
	if name == "" {
		return Workload{}, errors.New("a Workload carries the name its Pods are labelled with, so an empty name is refused")
	}
	if pods < 1 {
		return Workload{}, fmt.Errorf("a Workload declares at least one Pod, so a count of %d Pods is refused", pods)
	}

	declared := make([]SpreadDomain, 0, len(rules))
	for _, rule := range rules {
		if slices.Contains(declared, rule.domain) {
			return Workload{}, fmt.Errorf("a Workload constrains one domain once, so a second rule over the %s is refused", rule.domain)
		}
		declared = append(declared, rule.domain)
	}

	return Workload{name: name, pods: pods, rules: slices.Clone(rules)}, nil
}

// Name is what this Workload's Pods are labelled with.
func (w Workload) Name() string {
	return w.name
}

// Pods is how many replicas this Workload declares.
func (w Workload) Pods() int {
	return w.pods
}

// SpreadRules are the rules this Workload declares, in declaration order.
func (w Workload) SpreadRules() []SpreadRule {
	return slices.Clone(w.rules)
}

// Equal reports whether two Workloads declare the same Pods under the same
// rules. It is the equality go-cmp compares through, since the type's fields are
// unexported.
func (w Workload) Equal(other Workload) bool {
	return w.name == other.name && w.pods == other.pods && slices.Equal(w.rules, other.rules)
}
