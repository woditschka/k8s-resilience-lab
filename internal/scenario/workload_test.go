package scenario_test

import (
	"strings"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

const (
	// someWorkloadName and somePodCount are required to build a Workload but no
	// test below turns on either value.
	someWorkloadName = "web"
	somePodCount     = 9

	// evenSkew is the maximum Skew that makes a spread even: the fullest and the
	// emptiest domain differ by at most one Pod.
	evenSkew = 1
)

// aSpreadRule is the named default for a rule whose domain and Skew do not matter.
func aSpreadRule(t *testing.T) scenario.SpreadRule {
	t.Helper()

	return spreadRule(t, scenario.SpreadOverZones, evenSkew)
}

func spreadRule(t *testing.T, domain scenario.SpreadDomain, maxSkew int) scenario.SpreadRule {
	t.Helper()

	rule, err := scenario.NewSpreadRule(domain, maxSkew)
	if err != nil {
		t.Fatalf("build a spread rule over %s at a Skew of %d: %v", domain, maxSkew, err)
	}

	return rule
}

func TestAWorkload_ShouldKeepItsSpreadRulesInDeclarationOrder(t *testing.T) {
	overNodes := spreadRule(t, scenario.SpreadOverNodes, evenSkew)
	overZones := spreadRule(t, scenario.SpreadOverZones, evenSkew)

	workload, err := scenario.NewWorkload(someWorkloadName, somePodCount, overNodes, overZones)
	if err != nil {
		t.Fatalf("build a Workload of %d Pods: %v", somePodCount, err)
	}

	expect.Expect(t, workload.SpreadRules()).To(expect.Equal([]scenario.SpreadRule{overNodes, overZones}))
}

func TestAWorkloadDeclaringNoSpreadRule_ShouldReportNone(t *testing.T) {
	workload, err := scenario.NewWorkload(someWorkloadName, somePodCount)
	if err != nil {
		t.Fatalf("build a Workload declaring no spread rule: %v", err)
	}

	expect.Expect(t, workload.SpreadRules()).To(expect.HaveLen[scenario.SpreadRule](0))
}

func TestWhenTheReturnedRulesAreChanged_ShouldLeaveTheWorkloadUntouched(t *testing.T) {
	declared := aSpreadRule(t)
	workload, err := scenario.NewWorkload(someWorkloadName, somePodCount, declared)
	if err != nil {
		t.Fatalf("build a Workload of one spread rule: %v", err)
	}

	workload.SpreadRules()[0] = spreadRule(t, scenario.SpreadOverNodes, evenSkew+1)

	expect.Expect(t, workload.SpreadRules()).To(expect.Equal([]scenario.SpreadRule{declared}))
}

func TestAWorkload_ShouldRefuseTwoRulesForOneDomain(t *testing.T) {
	overZones := spreadRule(t, scenario.SpreadOverZones, evenSkew)
	overZonesAgain := spreadRule(t, scenario.SpreadOverZones, evenSkew+1)

	_, err := scenario.NewWorkload(someWorkloadName, somePodCount, overZones, overZonesAgain)

	if err == nil {
		t.Fatal("two rules for one domain are refused, but the Workload was admitted")
	}
	if !strings.Contains(err.Error(), string(scenario.SpreadOverZones)) {
		t.Errorf("the refusal names the repeated domain, but reads: %v", err)
	}
}

func TestAWorkload_ShouldRefuseADeclarationItCannotDeploy(t *testing.T) {
	refusals := map[string]struct {
		name  string
		pods  int
		named string
	}{
		"no name":        {name: "", pods: somePodCount, named: "name"},
		"no Pod at all":  {name: someWorkloadName, pods: 0, named: "Pod"},
		"a negative Pod": {name: someWorkloadName, pods: -1, named: "Pod"},
	}

	for refused, refusal := range refusals {
		t.Run(refused, func(t *testing.T) {
			_, err := scenario.NewWorkload(refusal.name, refusal.pods)

			if err == nil {
				t.Fatalf("a Workload named %q of %d Pods is refused, but it was admitted", refusal.name, refusal.pods)
			}
			if !strings.Contains(err.Error(), refusal.named) {
				t.Errorf("the refusal names %q, but reads: %v", refusal.named, err)
			}
		})
	}
}

func TestASpreadRule_ShouldRefuseADeclarationThatConstrainsNothing(t *testing.T) {
	refusals := map[string]struct {
		domain  scenario.SpreadDomain
		maxSkew int
	}{
		"a Skew below one":                    {domain: scenario.SpreadOverZones, maxSkew: 0},
		"a negative Skew":                     {domain: scenario.SpreadOverNodes, maxSkew: -1},
		"a domain that is neither of the two": {domain: scenario.SpreadDomain("Racks"), maxSkew: evenSkew},
	}

	for refused, refusal := range refusals {
		t.Run(refused, func(t *testing.T) {
			_, err := scenario.NewSpreadRule(refusal.domain, refusal.maxSkew)

			if err == nil {
				t.Fatalf("a rule over %q at a Skew of %d is refused, but it was admitted", refusal.domain, refusal.maxSkew)
			}
		})
	}
}
